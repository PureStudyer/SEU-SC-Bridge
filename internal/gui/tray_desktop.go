//go:build desktop

package gui

import (
	"github.com/energye/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func configureTray(a *App) (func(), func()) {
	return systray.RunWithExternalLoop(func() {
		configureTrayAppearance()
		open := systray.AddMenuItem("打开 SEU SC Bridge", "打开主窗口")
		open.Click(func() { <-a.ready; runtime.WindowShow(a.ctx); runtime.WindowUnminimise(a.ctx) })
		copy := systray.AddMenuItem("复制 SSH 命令", "复制 ssh seusc")
		copy.Click(func() { <-a.ready; _ = a.CopyCommand() })
		restart := systray.AddMenuItem("重启代理", "重新启动后台进程")
		restart.Click(func() { go func() { <-a.ready; _ = a.Restart() }() })
		systray.AddSeparator()
		quit := systray.AddMenuItem("退出 SEU SC Bridge", "停止后台代理并退出")
		quit.Click(func() { go func() { <-a.ready; _ = a.Stop(); runtime.Quit(a.ctx) }() })
		finishTrayMenu()
	}, func() {})
}
