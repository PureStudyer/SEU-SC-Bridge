// Package finder adapts SEU's authenticated web file manager without exposing credentials to SSH clients.
package finder

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/auth"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL, Account string
	RemoveEmptyDir   func(context.Context, string) error
	Tokens           *auth.Manager
	HTTP             *http.Client
}
type Entry struct {
	Path        string    `json:"path"`
	Filename    string    `json:"name"`
	Bytes       int64     `json:"size"`
	Directory   bool      `json:"isDir"`
	Symlink     bool      `json:"isSymlink"`
	LinkPath    string    `json:"linkPath"`
	Permissions string    `json:"mode"`
	Modified    time.Time `json:"modTime"`
	Items       []Entry   `json:"items"`
	Total       int       `json:"itemTotal"`
}

func (e Entry) Name() string       { return e.Filename }
func (e Entry) Size() int64        { return e.Bytes }
func (e Entry) ModTime() time.Time { return e.Modified }
func (e Entry) IsDir() bool        { return e.Directory }
func (e Entry) Sys() any           { return nil }
func (e Entry) Mode() os.FileMode {
	v, _ := strconv.ParseUint(e.Permissions, 8, 32)
	m := os.FileMode(v) & 0777
	if e.Directory {
		m |= os.ModeDir
	}
	if e.Symlink {
		m |= os.ModeSymlink
	}
	return m
}
func (c *Client) request(ctx context.Context, method, endpoint string, q url.Values, body []byte, contentType string) (*http.Response, error) {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	for attempt := 0; attempt < 2; attempt++ {
		creds, e := c.Tokens.GetCredentials(ctx)
		if e != nil {
			return nil, e
		}
		target := strings.TrimRight(c.BaseURL, "/") + "/finder/v2" + endpoint
		if len(q) > 0 {
			target += "?" + q.Encode()
		}
		r, e := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
		if e != nil {
			return nil, errors.New("invalid file request")
		}
		r.Header.Set("Authorization", "Bearer "+creds.Token)
		r.Header.Set("Origin", c.BaseURL)
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		for _, cookie := range creds.Cookies {
			r.AddCookie(cookie)
		}
		res, e := hc.Do(r)
		if e != nil {
			return nil, errors.New("file service request failed; check network")
		}
		if res.StatusCode == 401 || res.StatusCode == 403 {
			res.Body.Close()
			c.Tokens.InvalidateToken(creds.Token)
			continue
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			res.Body.Close()
			if res.StatusCode == 404 {
				return nil, os.ErrNotExist
			}
			return nil, fmt.Errorf("file service returned HTTP %d", res.StatusCode)
		}
		return res, nil
	}
	return nil, errors.New("file service authentication expired")
}
func decode(res *http.Response, out any) error {
	defer res.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(res.Body, 16<<20))
	if e != nil {
		return errors.New("file service response interrupted")
	}
	if len(raw) > 0 && raw[0] == '"' {
		var inner string
		if json.Unmarshal(raw, &inner) == nil {
			raw = []byte(inner)
		}
	}
	var env struct {
		Success *bool           `json:"success"`
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &env) != nil {
		return errors.New("invalid file service response")
	}
	if (env.Success != nil && !*env.Success) || (env.Code != nil && string(env.Code) != "0" && string(env.Code) != "200" && string(env.Code) != "\"0\"") {
		if strings.Contains(env.Message, "不存在") {
			return os.ErrNotExist
		}
		if strings.Contains(env.Message, "权限") {
			return os.ErrPermission
		}
		return errors.New("file operation rejected by SEU: " + safeMessage(env.Message))
	}
	if out == nil {
		return nil
	}
	if env.Data != nil && string(env.Data) != "null" {
		raw = env.Data
	}
	if string(raw) == "\"\"" {
		return os.ErrNotExist
	}
	if e = json.Unmarshal(raw, out); e != nil {
		return errors.New("unexpected file service response")
	}
	return nil
}
func safeMessage(s string) string {
	if len(s) > 200 {
		return "operation failed"
	}
	if strings.Contains(strings.ToLower(s), "token") || strings.Contains(s, "Bearer") {
		return "authentication required"
	}
	return s
}
func (c *Client) json(ctx context.Context, method, ep string, q url.Values, v, out any) error {
	var b []byte
	if v != nil {
		b, _ = json.Marshal(v)
	}
	r, e := c.request(ctx, method, ep, q, b, "application/json")
	if e != nil {
		return e
	}
	return decode(r, out)
}
func (c *Client) Stat(ctx context.Context, p string) (Entry, error) {
	var out Entry
	e := c.json(ctx, "GET", "/files/getStat", url.Values{"path": {p}}, nil, &out)
	if e == nil && out.Filename == "" {
		e = os.ErrNotExist
	}
	out.Path = p
	return out, e
}
func (c *Client) List(ctx context.Context, p string) ([]Entry, error) {
	var all []Entry
	for page := 1; page <= 10000; page++ {
		var out Entry
		e := c.json(ctx, "POST", "/files/search", nil, map[string]any{"fileOption": map[string]any{"path": p, "ClusterAccount": c.Account, "clusterAccount": c.Account, "page": page, "pageSize": 300, "showHidden": true, "expand": true, "sort": "name", "reverse": "ascend", "containSub": false, "search": ""}}, &out)
		if e != nil {
			return nil, e
		}
		for i := range out.Items {
			if out.Items[i].Path == "" {
				out.Items[i].Path = path.Join(p, out.Items[i].Filename)
			}
		}
		all = append(all, out.Items...)
		if len(all) >= out.Total || len(out.Items) == 0 {
			return all, nil
		}
	}
	return nil, errors.New("directory exceeds supported pagination limit")
}
func (c *Client) Mkdir(ctx context.Context, p string) error {
	return c.json(ctx, "POST", "/files/CreateFile", nil, map[string]any{"path": p, "name": path.Base(p), "isDir": true, "mode": 493, "isLink": false, "ClusterAccount": c.Account}, nil)
}
func (c *Client) Rename(ctx context.Context, old, new string) error {
	return c.json(ctx, "POST", "/files/rename", nil, map[string]any{"path": path.Dir(old), "oldName": old, "newName": new, "ClusterAccount": c.Account}, nil)
}
func (c *Client) Remove(ctx context.Context, p string, isDir bool) error {
	return c.json(ctx, "POST", "/files/DeleteFiles", nil, map[string]any{"files": []any{map[string]any{"path": p, "isDir": isDir, "ClusterAccount": c.Account}}}, nil)
}
func (c *Client) Download(ctx context.Context, p string, w io.Writer) error {
	res, e := c.request(ctx, "GET", "/files/download", url.Values{"path": {p}, "name": {path.Base(p)}, "compress": {"false"}, "ClusterAccount": {c.Account}}, nil, "")
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if strings.Contains(res.Header.Get("Content-Type"), "application/json") && res.Header.Get("Content-Disposition") == "" {
		if e := decode(res, nil); e != nil {
			return e
		}
		return errors.New("download returned metadata instead of file bytes")
	}
	_, e = io.Copy(w, res.Body)
	if e != nil {
		return errors.New("file download interrupted")
	}
	return nil
}
func randomID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}

// Upload stages chunks on SEU, then renames a complete temporary remote file into place.
func (c *Client) Upload(ctx context.Context, dest string, r io.ReaderAt, size int64) error {
	id := randomID()
	name := ".seusc-upload-" + id
	stage := path.Join(path.Dir(dest), name)
	committed := false
	defer func() {
		if !committed {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = c.Remove(cleanup, stage, false)
		}
	}()
	const chunkSize int64 = 10 << 20
	total := (size + chunkSize - 1) / chunkSize
	if total == 0 {
		total = 1
	}
	for i := int64(0); i < total; i++ {
		n := chunkSize
		if left := size - i*chunkSize; left < n {
			n = left
		}
		q := url.Values{"chunkNumber": {strconv.FormatInt(i+1, 10)}, "chunkSize": {strconv.FormatInt(chunkSize, 10)}, "currentChunkSize": {strconv.FormatInt(n, 10)}, "totalSize": {strconv.FormatInt(size, 10)}, "identifier": {id}, "filename": {name}, "relativePath": {name}, "totalChunks": {strconv.FormatInt(total, 10)}, "clusterAccount": {c.Account}, "targetPath": {path.Dir(dest)}}
		var body bytes.Buffer
		multi := multipart.NewWriter(&body)
		for k, values := range q {
			for _, v := range values {
				_ = multi.WriteField(k, v)
			}
		}
		part, e := multi.CreateFormFile("file", name)
		if e != nil {
			return e
		}
		if _, e = io.CopyN(part, io.NewSectionReader(r, i*chunkSize, n), n); e != nil {
			return e
		}
		multi.Close()
		res, e := c.request(ctx, "POST", "/upload/global_upload", nil, body.Bytes(), multi.FormDataContentType())
		if e != nil {
			return e
		}
		raw, e := io.ReadAll(io.LimitReader(res.Body, 1<<20))
		res.Body.Close()
		if e != nil {
			return errors.New("upload response interrupted")
		}
		if i < total-1 && strings.TrimSpace(string(raw)) == "1" {
			continue
		}
		res.Body = io.NopCloser(bytes.NewReader(raw))
		var reply struct {
			Result    json.RawMessage `json:"result"`
			NeedMerge json.RawMessage `json:"needMerge"`
			TimeStamp json.RawMessage `json:"timeStamp"`
			Message   string          `json:"message"`
		}
		if e = decode(res, &reply); e != nil {
			return e
		}
		if string(reply.Result) != "true" && string(reply.Result) != "200" {
			return errors.New("SEU rejected upload chunk")
		}
		if string(reply.NeedMerge) == "true" || string(reply.NeedMerge) == "\"true\"" {
			stamp := strings.Trim(string(reply.TimeStamp), "\"")
			mq := url.Values{"clusterAccount": {c.Account}, "filename": {name}, "identifier": {id}, "totalSize": {strconv.FormatInt(size, 10)}, "timeStamp": {stamp}, "jobPath": {path.Dir(dest)}}
			if e = c.json(ctx, "GET", "/upload/file_merge", mq, nil, nil); e != nil {
				return e
			}
		}
	}
	stat, e := c.Stat(ctx, stage)
	if e != nil {
		return e
	}
	if stat.Size() != size {
		return errors.New("remote upload size verification failed")
	}
	if e = c.Rename(ctx, stage, dest); e != nil {
		return e
	}
	committed = true
	return nil
}

func (c *Client) Rmdir(ctx context.Context, p string) error {
	if c.RemoveEmptyDir == nil {
		return errors.New("safe empty-directory removal unavailable")
	}
	return c.RemoveEmptyDir(ctx, p)
}
