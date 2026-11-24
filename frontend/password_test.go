package frontend_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PureStudyer/SEU-SC-Bridge/frontend"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/browser"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func TestRememberedPasswordForm(t *testing.T) {
	exe, err := browser.Find("")
	if err != nil {
		t.Skip("Chromium not installed")
	}
	server := httptest.NewServer(http.FileServer(http.FS(frontend.Assets)))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocator, free := chromedp.NewExecAllocator(ctx, append(opts, chromedp.ExecPath(exe))...)
	defer free()
	tab, closeTab := chromedp.NewContext(allocator)
	defer closeTab()
	const mock = `
	  window.calls = [];
	  window.statusFixture = {state:'READY',account:'alice',remember:true,credential_stored:true};
	  window.go = {gui:{App:{
	    Status: async () => statusFixture, CheckForUpdates: async () => ({}),
	    RevealSavedPassword: async () => {calls.push('read');return 'fixture-only'},
	    Login: async (name,password) => calls.push([name,password])
	  }}};
	`
	err = chromedp.Run(tab, chromedp.ActionFunc(func(c context.Context) error {
		_, err := page.AddScriptToEvaluateOnNewDocument(mock).Do(c)
		return err
	}), chromedp.Navigate(server.URL), chromedp.Click("button[data-page=account]", chromedp.ByQuery))
	if err != nil {
		t.Fatal(err)
	}
	steps := []struct{ name, js string }{
		{"saved mask without reading", `return !passwordField.value && passwordField.placeholder==='••••••••' && !calls.length`},
		{"show saved", `await revealButton.onclick();return passwordField.type==='text' && passwordField.value==='fixture-only'`},
		{"hide saved", `revealButton.click();return passwordField.type==='password' && !passwordField.value`},
		{"switch account during read", `const pending=revealButton.onclick();$('username').value='bob';$('username').dispatchEvent(new Event('input'));await pending;return !passwordField.value && passwordField.type==='password' && passwordField.placeholder!=='••••••••'`},
		{"show typed password", `passwordField.value='edited';passwordField.dispatchEvent(new Event('input'));revealButton.click();return passwordField.type==='text' && passwordField.value==='edited'`},
		{"preserve edits on refresh", `await refresh();revealButton.click();return passwordField.value==='edited' && passwordField.type==='password'`},
		{"submit draft", `$('login-form').requestSubmit();return JSON.stringify(calls.at(-1))==='["bob","edited"]'`},
		{"reuse stored credential", `page('account');$('username').value='alice';$('username').dispatchEvent(new Event('input'));$('login-form').requestSubmit();return JSON.stringify(calls.at(-1))==='["alice",""]'`},
	}
	for _, s := range steps {
		var ok bool
		err := chromedp.Run(tab, chromedp.Evaluate("(async()=>{"+s.js+"})()", &ok, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) }))
		if err != nil || !ok {
			t.Fatalf("%s: %v", s.name, err)
		}
	}
}
