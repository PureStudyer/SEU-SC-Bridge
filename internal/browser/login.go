package browser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/auth"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/config"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func Find(explicit string) (string, error) {
	if explicit != "" {
		if s, e := os.Stat(explicit); e == nil && !s.IsDir() {
			return explicit, nil
		}
		return "", apperr.New("BROWSER_NOT_FOUND", "配置的浏览器路径无效")
	}
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = append(candidates, `C:\Program Files\Google\Chrome\Application\chrome.exe`, `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`)
	}
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("PROGRAMFILES"), os.Getenv("PROGRAMFILES(X86)"), os.Getenv("LOCALAPPDATA")} {
			candidates = append(candidates, filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
		}
		for _, base := range []string{os.Getenv("PROGRAMFILES(X86)"), os.Getenv("PROGRAMFILES"), os.Getenv("LOCALAPPDATA")} {
			candidates = append(candidates, filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"))
		}
	} else if runtime.GOOS == "darwin" {
		candidates = []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Chromium.app/Contents/MacOS/Chromium"}
		if h, e := os.UserHomeDir(); e == nil {
			candidates = append(candidates, filepath.Join(h, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"))
		}
	}
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser", "chrome", "msedge"} {
		if p, e := exec.LookPath(name); e == nil {
			candidates = append(candidates, p)
		}
	}
	if executable, e := os.Executable(); e == nil {
		base := filepath.Dir(executable)
		if runtime.GOOS == "windows" {
			candidates = append(candidates, filepath.Join(base, "chromium", "chrome.exe"))
		} else {
			candidates = append(candidates, filepath.Join(base, "chromium", "chrome"))
		}
	}
	for _, p := range candidates {
		if s, e := os.Stat(p); e == nil && !s.IsDir() {
			return p, nil
		}
	}
	return "", apperr.New("BROWSER_NOT_FOUND", "请安装 Chrome、Edge 或 Chromium，或在配置中指定 browser.path")
}

type Login struct {
	Profile            string
	Config             config.Config
	Username, Password string
	Visible            bool
	Progress           func(string)
}

func (l Login) Acquire(ctx context.Context) (auth.Credentials, error) {
	executable, e := Find(l.Config.Browser.Path)
	if e != nil {
		return auth.Credentials{}, e
	}
	if !l.Visible {
		c, e := l.attempt(ctx, executable, false, 35*time.Second)
		if e == nil {
			return c, nil
		}
		if ctx.Err() != nil {
			return auth.Credentials{}, ctx.Err()
		}
	}
	return l.attempt(ctx, executable, true, 3*time.Minute)
}
func (l Login) attempt(parent context.Context, executable string, visible bool, timeout time.Duration) (auth.Credentials, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.ExecPath(executable), chromedp.UserDataDir(l.Profile), chromedp.Flag("headless", !visible), chromedp.Flag("disable-gpu", !visible), chromedp.Flag("no-first-run", true), chromedp.Flag("no-default-browser-check", true))
	ac, closeAllocator := chromedp.NewExecAllocator(ctx, opts...)
	defer closeAllocator()
	bc, closeBrowser := chromedp.NewContext(ac, chromedp.WithLogf(func(string, ...any) {}), chromedp.WithErrorf(func(string, ...any) {}))
	defer closeBrowser()
	defer func() {
		shutdown, stop := context.WithTimeout(bc, 5*time.Second)
		defer stop()
		_ = chromedp.Cancel(shutdown)
	}()
	tokens := make(chan string, 1)
	var mu sync.Mutex
	allowed := map[network.RequestID]bool{}
	extra := map[network.RequestID]network.Headers{}
	offer := func(h network.Headers) {
		for k, v := range h {
			if strings.EqualFold(k, "Authorization") {
				if value, ok := v.(string); ok && strings.HasPrefix(value, "Bearer ") && len(value) > 7 {
					select {
					case tokens <- strings.TrimPrefix(value, "Bearer "):
					default:
					}
				}
			}
		}
	}
	base, _ := url.Parse(l.Config.SEU.BaseURL)
	chromedp.ListenTarget(bc, func(ev any) {
		mu.Lock()
		defer mu.Unlock()
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			u, err := url.Parse(e.Request.URL)
			if err == nil && u.Scheme == "https" && u.Host == base.Host {
				allowed[e.RequestID] = true
				offer(e.Request.Headers)
				if h, ok := extra[e.RequestID]; ok {
					offer(h)
					delete(extra, e.RequestID)
				}
			}
		case *network.EventRequestWillBeSentExtraInfo:
			if allowed[e.RequestID] {
				offer(e.Headers)
			} else if len(extra) < 256 {
				extra[e.RequestID] = e.Headers
			}
		case *network.EventLoadingFinished:
			delete(allowed, e.RequestID)
			delete(extra, e.RequestID)
		case *network.EventLoadingFailed:
			delete(allowed, e.RequestID)
			delete(extra, e.RequestID)
		}
	})
	if e := chromedp.Run(bc, network.Enable(), chromedp.Navigate(l.Config.SEU.BaseURL)); e != nil {
		return auth.Credentials{}, apperr.New("SEU_UNREACHABLE", "无法打开 SEU 登录网站")
	}
	filled := false
	lastProgress := ""
	tick := time.NewTicker(400 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case token := <-tokens:
			var cookies []*http.Cookie
			e := chromedp.Run(bc, chromedp.ActionFunc(func(c context.Context) error {
				items, e := network.GetCookies().WithURLs([]string{l.Config.SEU.BaseURL + "/finder/v2/webshell"}).Do(c)
				if e != nil {
					return e
				}
				for _, v := range items {
					cookies = append(cookies, &http.Cookie{Name: v.Name, Value: v.Value})
				}
				return nil
			}))
			if e != nil {
				return auth.Credentials{}, apperr.New("AUTH_FAILED", "无法读取登录会话")
			}
			return auth.Credentials{Token: token, Cookies: cookies}, nil
		case <-ctx.Done():
			return auth.Credentials{}, apperr.New("AUTH_REQUIRED", "登录未完成，请重试并在浏览器中完成验证码或统一认证")
		case <-tick.C:
			if l.Progress != nil {
				var state string
				_ = chromedp.Run(bc, chromedp.Evaluate(`JSON.stringify({path:location.pathname,form:!!document.querySelector("#form_item_username"),error:/账号或密码错误|用户名或密码错误|登录失败/.test(document.body.innerText),captcha:/请输入验证码/.test(document.body.innerText)})`, &state))
				if state != lastProgress {
					l.Progress(state)
					lastProgress = state
				}
			}
			if filled || l.Username == "" || l.Password == "" {
				continue
			}
			input, _ := json.Marshal(map[string]string{"username": l.Username, "password": l.Password, "userSelector": l.Config.Browser.UsernameSelector, "passwordSelector": l.Config.Browser.PasswordSelector, "submitSelector": l.Config.Browser.SubmitSelector, "origin": l.Config.SEU.BaseURL})
			// Only fill the trusted SEU origin. Redirected SSO pages remain user-controlled.
			script := `(function(o){if(location.origin!==new URL(o.origin).origin)return false;const u=document.querySelector(o.userSelector),p=document.querySelector(o.passwordSelector),b=document.querySelector(o.submitSelector);if(!u||!p||!b||!u.getClientRects().length)return false;for(const [el,value] of [[u,o.username],[p,o.password]]){Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(el,value);el.dispatchEvent(new Event('input',{bubbles:true}));el.dispatchEvent(new Event('change',{bubbles:true}));}b.click();return true;})(` + string(input) + ")"
			if e := chromedp.Run(bc, chromedp.Evaluate(script, &filled)); e != nil && ctx.Err() != nil {
				return auth.Credentials{}, apperr.New("AUTH_FAILED", "浏览器登录中断")
			}
		}
	}
}
