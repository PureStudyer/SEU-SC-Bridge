package browser

import (
	"context"
	"sync"
	"time"

	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"github.com/chromedp/chromedp"
)

// Controller owns the isolated browser for one agent. Login and terminal tabs
// share its site session, while each SSH channel owns a separate terminal tab.
type Controller struct {
	Lifetime     context.Context
	mu           sync.Mutex
	root         context.Context
	closeBrowser func()
	profile      string
	visible      bool
}

func (b *Controller) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reset()
}

func (b *Controller) reset() {
	if b.root != nil {
		b.closeBrowser()
		b.root = nil
	}
}

func (b *Controller) loginTab(ctx context.Context, executable, profile string, visible bool) (context.Context, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.root != nil && (b.root.Err() != nil || profile != b.profile || visible && !b.visible) {
		b.reset()
	}
	if b.root == nil {
		opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
		opts = append(opts, chromedp.ExecPath(executable), chromedp.UserDataDir(profile), chromedp.Flag("headless", !visible), chromedp.Flag("disable-gpu", !visible))
		process, cancelProcess := context.WithCancel(b.Lifetime)
		ac, free := chromedp.NewExecAllocator(process, opts...)
		root, closeRoot := chromedp.NewContext(ac, chromedp.WithLogf(func(string, ...any) {}), chromedp.WithErrorf(func(string, ...any) {}))
		stop := context.AfterFunc(ctx, cancelProcess)
		err := chromedp.Run(root)
		if !stop() || err != nil {
			cancelProcess()
			closeRoot()
			free()
			return nil, nil, apperr.New("BROWSER_CONTROL_FAILED", "无法启动或控制登录浏览器")
		}
		b.root, b.profile, b.visible = root, profile, visible
		b.closeBrowser = func() {
			shutdown, cancel := context.WithTimeout(root, 5*time.Second)
			defer cancel()
			_ = chromedp.Cancel(shutdown)
			closeRoot()
			free()
			cancelProcess()
		}
	}
	tab, closeTab := linkedTab(b.root, ctx)
	return tab, closeTab, nil
}

func linkedTab(root, parent context.Context) (context.Context, func()) {
	life, cancel := context.WithCancel(root)
	tab, closeTab := chromedp.NewContext(life)
	stop := context.AfterFunc(parent, cancel)
	return tab, func() { stop(); cancel(); closeTab() }
}

// CheckSite uses a temporary browser profile so doctor can check page loading
// even before the user has logged in.
func CheckSite(ctx context.Context, browserPath, site string) (int, error) {
	executable, err := Find(browserPath)
	if err != nil {
		return 0, err
	}
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocator, free := chromedp.NewExecAllocator(ctx, append(opts, chromedp.ExecPath(executable))...)
	defer free()
	tab, closeTab := chromedp.NewContext(allocator, chromedp.WithErrorf(func(string, ...any) {}))
	defer closeTab()
	response, err := chromedp.RunResponse(tab, chromedp.Navigate(site))
	if err != nil || response == nil {
		return 0, apperr.New("SEU_UNREACHABLE", "浏览器无法加载 SEU 网站")
	}
	return int(response.Status), nil
}
