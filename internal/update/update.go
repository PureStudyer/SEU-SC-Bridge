package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const (
	repository   = "PureStudyer/SEU-SC-Bridge"
	latestAPI    = "https://api.github.com/repos/" + repository + "/releases/latest"
	maxJSONBytes = 2 << 20
	maxSumBytes  = 1 << 20
	maxAssetSize = 1 << 30
)

var versionPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)$`)

type Info struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	ReleaseURL      string `json:"releaseUrl"`
	Notes           string `json:"notes"`
	UpdateAvailable bool   `json:"updateAvailable"`
	CanInstall      bool   `json:"canInstall"`
}

type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Draft   bool   `json:"draft"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func Check(ctx context.Context, current string) (Info, error) {
	if _, ok := parseVersion(current); !ok {
		return Info{}, errors.New("当前软件版本号无效")
	}
	r, err := fetchLatest(ctx)
	if err != nil {
		return Info{}, err
	}
	latest := strings.TrimPrefix(r.TagName, "v")
	cmp, ok := compareVersions(current, latest)
	if !ok {
		return Info{}, errors.New("GitHub Release 版本号无效")
	}
	info := Info{
		CurrentVersion: current, LatestVersion: latest, ReleaseURL: r.HTMLURL,
		Notes: truncate(r.Body, 6000), UpdateAvailable: cmp < 0,
	}
	if runtime.GOOS == "windows" && assetFor(r, runtime.GOOS, runtime.GOARCH) != nil {
		info.CanInstall = true
	}
	return info, nil
}

// DownloadAndLaunch re-fetches the latest release, verifies the Windows
// installer against the release checksum manifest, then launches its UI.
func DownloadAndLaunch(ctx context.Context, current string) error {
	if runtime.GOOS != "windows" {
		return errors.New("自动安装目前只支持 Windows，请打开发布页面手动更新")
	}
	r, err := fetchLatest(ctx)
	if err != nil {
		return err
	}
	cmp, ok := compareVersions(current, strings.TrimPrefix(r.TagName, "v"))
	if !ok || cmp >= 0 {
		return errors.New("当前已是最新版本")
	}
	installer := assetFor(r, runtime.GOOS, runtime.GOARCH)
	sums := assetNamed(r, "SHA256SUMS.txt")
	if installer == nil || sums == nil {
		return errors.New("此版本缺少适用于本机的安装文件或校验清单")
	}
	if err = validateReleaseURL(r, installer.BrowserDownloadURL); err != nil {
		return err
	}
	if err = validateReleaseURL(r, sums.BrowserDownloadURL); err != nil {
		return err
	}
	httpClient := client(15 * time.Minute)
	manifest, err := fetchBytes(ctx, httpClient, sums.BrowserDownloadURL, maxSumBytes)
	if err != nil {
		return fmt.Errorf("无法下载更新校验清单: %w", err)
	}
	want, ok := checksumFor(string(manifest), installer.Name)
	if !ok {
		return errors.New("更新校验清单中没有安装文件的 SHA-256")
	}
	assetURL, _ := url.Parse(installer.BrowserDownloadURL)
	fileName := filepath.Base(assetURL.Path)
	if fileName != installer.Name {
		return errors.New("安装文件名与 Release 元数据不一致")
	}
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return errors.New("无法找到用户缓存目录")
	}
	dir := filepath.Join(cacheDir, "SEUSC", "updates")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return errors.New("无法创建更新缓存目录")
	}
	path := filepath.Join(dir, installer.Name)
	if !matchesSHA256(path, want) {
		_ = os.Remove(path)
		if err = downloadAsset(ctx, httpClient, installer.BrowserDownloadURL, path, want); err != nil {
			return err
		}
	}
	if err = exec.Command(path).Start(); err != nil {
		return errors.New("已下载更新，但无法启动安装程序")
	}
	return nil
}

func fetchLatest(ctx context.Context) (release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestAPI, nil)
	if err != nil {
		return release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "SEU-SC-Bridge-Updater")
	resp, err := client(20 * time.Second).Do(req)
	if err != nil {
		return release{}, errors.New("无法连接 GitHub 检查更新")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return release{}, fmt.Errorf("GitHub 更新服务返回 HTTP %d", resp.StatusCode)
	}
	var r release
	if err = json.NewDecoder(io.LimitReader(resp.Body, maxJSONBytes)).Decode(&r); err != nil {
		return release{}, errors.New("GitHub 更新信息格式无效")
	}
	if r.Draft || !strings.HasPrefix(r.HTMLURL, "https://github.com/"+repository+"/releases/tag/") {
		return release{}, errors.New("GitHub Release 来源无效")
	}
	if _, ok := parseVersion(strings.TrimPrefix(r.TagName, "v")); !ok {
		return release{}, errors.New("GitHub Release 版本号无效")
	}
	return r, nil
}

func client(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		switch req.URL.Hostname() {
		case "github.com", "api.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
			if req.URL.Scheme == "https" {
				return nil
			}
		}
		return errors.New("update redirect host is not trusted")
	}}
}

func fetchBytes(ctx context.Context, c *http.Client, target string, limit int64) ([]byte, error) {
	if !trustedURL(target) {
		return nil, errors.New("update URL is not trusted")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "SEU-SC-Bridge-Updater")
	resp, err := c.Do(req)
	if err != nil {
		return nil, errors.New("更新文件下载失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("更新服务器返回 HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(b)) > limit {
		return nil, errors.New("更新文件过大或下载不完整")
	}
	return b, nil
}

func downloadAsset(ctx context.Context, c *http.Client, target, path, want string) error {
	if !trustedURL(target) {
		return errors.New("更新 URL 不可信")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "SEU-SC-Bridge-Updater")
	resp, err := c.Do(req)
	if err != nil {
		return errors.New("更新安装文件下载失败")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("更新服务器返回 HTTP %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".update-*.tmp")
	if err != nil {
		return errors.New("无法写入更新安装文件")
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, maxAssetSize+1))
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil || n == 0 || n > maxAssetSize {
		return errors.New("更新安装文件下载失败或文件过大")
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want) {
		return errors.New("更新安装文件 SHA-256 校验失败")
	}
	if err = os.Rename(tmpName, path); err != nil {
		return errors.New("无法保存更新安装文件")
	}
	return nil
}

func assetFor(r release, goos, goarch string) *struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
} {
	want := ""
	switch {
	case goos == "windows" && goarch == "amd64":
		want = "SEU-SC-Bridge-Setup-x64.exe"
	case goos == "windows" && goarch == "arm64":
		want = "SEU-SC-Bridge-Setup-arm64.exe"
	case goos == "darwin" && goarch == "amd64":
		want = "SEU-SC-Bridge-amd64.dmg"
	case goos == "darwin" && goarch == "arm64":
		want = "SEU-SC-Bridge-arm64.dmg"
	}
	if want == "" {
		return nil
	}
	return assetNamed(r, want)
}

func assetNamed(r release, name string) *struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
} {
	for i := range r.Assets {
		if r.Assets[i].Name == name {
			return &r.Assets[i]
		}
	}
	return nil
}

func validateReleaseURL(r release, target string) error {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.Hostname() != "github.com" || !strings.HasPrefix(u.Path, "/"+repository+"/releases/download/"+r.TagName+"/") {
		return errors.New("Release 文件 URL 不可信")
	}
	return nil
}

func trustedURL(target string) bool {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" {
		return false
	}
	switch u.Hostname() {
	case "github.com", "api.github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com":
		return true
	default:
		return false
	}
}

func checksumFor(manifest, name string) (string, bool) {
	for _, line := range strings.Split(manifest, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == name && len(fields[0]) == 64 {
			if _, err := hex.DecodeString(fields[0]); err == nil {
				return fields[0], true
			}
		}
	}
	return "", false
}

func matchesSHA256(path, want string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return false
	}
	return strings.EqualFold(hex.EncodeToString(h.Sum(nil)), want)
}

func parseVersion(s string) ([3]uint64, bool) {
	var v [3]uint64
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return v, false
	}
	for i := range v {
		var n uint64
		if _, err := fmt.Sscan(m[i+1], &n); err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

func compareVersions(a, b string) (int, bool) {
	va, ok := parseVersion(a)
	if !ok {
		return 0, false
	}
	vb, ok := parseVersion(b)
	if !ok {
		return 0, false
	}
	for i := range va {
		if va[i] < vb[i] {
			return -1, true
		}
		if va[i] > vb[i] {
			return 1, true
		}
	}
	return 0, true
}

func truncate(s string, limit int) string {
	if len(s) > limit {
		return s[:limit] + "…"
	}
	return s
}
