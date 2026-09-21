# SEUSC

SEU 超算 WebShell 的本地 SSH 桥接工具。Go 核心，Wails 桌面界面，Windows / macOS 原生运行，无需 Python 或 WSL。

## 使用

1. Windows 运行对应架构的安装器；macOS 将 DMG 内的 SEU SC Bridge.app 拖入 Applications。
2. 打开 SEUSC，输入 SEU 超算账号和密码，点击“登录并启动”。遇到验证码/统一认证时，在弹出的浏览器中自行完成。
3. 打开终端运行：

~~~sh
ssh seusc
ssh seusc hostname
ssh seusc pwd
ssh seusc squeue
~~~

支持多会话、PTY 缩放、Ctrl+C / Ctrl+Z、ANSI 和 UTF-8。关闭窗口后托盘和后台代理继续运行；托盘“退出 SEUSC”会停止后台代理。需要安装 Chrome / Edge / Chromium；Windows 桌面界面需要 Microsoft Edge WebView2 Runtime。

记住密码默认关闭。开启后使用 Windows Credential Manager 或 macOS Keychain。浏览器登录会话保存在独立配置目录，代理重启后自动恢复；服务器撤销登录后会重新认证。首次启动自动填写仅限 sc.seu.edu.cn，统一认证跨域页面由用户完成。

## 已验证与平台范围

2026-09-21 在 Windows amd64 完成真实 SEU 自动登录、OpenSSH hostname/pwd/squeue、中文输出、退出码和重启恢复测试。自动化模拟测试覆盖二进制协议、SSH 公钥认证、缩放、并发会话、断线及 exec 标记跨帧解析。

Windows amd64/arm64 提供构建脚本和 NSIS 安装器。macOS amd64/arm64 提供原生构建、app/DMG 打包及签名/公证接口；macOS GUI 必须在 macOS 构建，实际机器验收仍需相应平台。CI 不运行真实账号测试。

## 协议说明：与原始 spec 的差异

实测当前 SEU 站点前端使用 JSON 文本帧输入：

~~~json
{"type":"input","data":"hostname\r"}
~~~

终端输出仍是二进制帧；缩放是 JSON。原 spec 的纯二进制输入在当前服务上被忽略，因此默认 seu.input_mode 为 json，同时保留 binary 供旧版平台使用。JSON 模式按 UTF-8 跨帧拼接输入，保留控制字符；不支持任意非 UTF-8 输入字节。输出始终原样传送。

exec 使用随机控制字符标记截取输出、返回退出码，并隔离启动横幅/命令回显。平台本质是 PTY，输出可能带 CRLF，stdout/stderr 无法分开；不支持输入管道、大于约 3 KB 的命令、旧版 SCP（scp -O）、VS Code Remote、端口/agent/X11 转发。30 秒内没有收到 exec 启动标记会报错；已启动的长命令不施加时间限制。交互式会话不会自动重连丢失的 PTY。

## CLI

~~~text
seusc                  打开 GUI（桌面构建）
seusc gui              打开 GUI
seusc agent            前台运行代理
seusc login            交互式账号/隐藏密码输入
seusc login --username ACCOUNT --remember
seusc login --visible  打开可见浏览器
seusc logout           删除系统密码和隔离登录会话、断开 SSH
seusc status [--json]
seusc doctor
seusc start | stop | restart
seusc node             列出节点
seusc node login02 7    添加并选择节点
seusc logs
seusc autostart on|off
seusc init             初始化密钥与 SSH 配置
seusc version
~~~

自动化登录可使用 --password-stdin 从 stdin 读取密码；不接受密码命令行参数。未加入 PATH 时，使用安装目录中 seusc.exe 的完整路径。macOS CLI 为 /Applications/SEU SC Bridge.app/Contents/MacOS/seusc。

## 配置与安全

Windows 配置位于 %APPDATA%/SEUSC/config.json；数据/浏览器位于 %LOCALAPPDATA%/SEUSC；macOS 为 ~/Library/Application Support/SEUSC。日志分别为 %LOCALAPPDATA%/SEUSC/logs 和 ~/Library/Logs/SEUSC，10 MB × 5 备份。

- SSH 仅接受回环地址和专用客户端公钥，拒绝密码/无认证连接。
- 专用客户端密钥 ~/.ssh/seusc_ed25519；服务端密钥保存在应用数据目录。
- SSH config 仅管理 BEGIN/END SEUSC 区块，首次修改保存 .seusc-backup；同名自定义 Host 会报告冲突。
- 独立 seusc_known_hosts 固定主机公钥，默认严格校验，避免首次连接询问。
- Windows 目录/私钥收紧 ACL；IPC Named Pipe 仅当前用户可访问；macOS socket 位于私有目录、权限 0600。
- 密码仅保存在系统凭据管理器（选择记住后），Bearer/Cookie 不写入应用 JSON 或日志。隔离 Chromium profile 自身管理站点会话数据，应按敏感数据保护。
- TLS 验证保持开启；默认节点 login01=6 可修改，端口冲突自动尝试之后的 19 个端口。
- 卸载保留用户配置和 SSH 密钥；如需清除登录数据，请先退出账号。

配置示例见 config.example.json。高级用户可修改 browser.path 和表单选择器。SEUSC_HOME 可将应用全部状态和 .ssh 重定向到测试目录；请勿用于正式安装的自启动配置。

## 构建

需要 Go 1.26+。前端为嵌入式 HTML/CSS/JavaScript，无需 Node/npm 构建步骤。

~~~sh
go test ./... -timeout 120s
go vet ./...
go build -o seusc ./cmd/seusc
~~~

CLI 构建可用于测试/服务器开发；GUI 需要 desktop,production 两个 build tags。

Windows PowerShell：

~~~powershell
./scripts/build.ps1 -Arch amd64
./scripts/build.ps1 -Arch arm64
makensis /DARCH=amd64 packaging/windows/installer.nsi
makensis /DARCH=arm64 packaging/windows/installer.nsi
~~~

macOS（需要 Xcode Command Line Tools）：

~~~sh
bash scripts/build-macos.sh arm64
bash scripts/build-macos.sh amd64
~~~

APPLE_SIGNING_IDENTITY 配置 Developer ID 签名，APPLE_NOTARY_PROFILE 配置已保存的 notarytool 凭据；未配置时产出 ad-hoc 签名的开发构建，不能当作已公证的正式发布版本。

## 测试

单元与集成测试无需账号、浏览器或联网。race 检查需要 C 编译器：go test -race ./...。真实测试必须显式设置 SEUSC_LIVE_TEST=1 和 SEUSC_HOME，并先通过 CLI 登录；详见 testing.md。

完整需求保存在 SEUSC_SPEC.md；实现与验收说明见 implementation.md。

## SFTP / scp 文件传输

支持标准 SFTP 与现代 OpenSSH scp，通过 SEU Finder 接口上传和下载。已实测 1 MB 随机二进制 scp 往返，以及超过 10 MB 的分块文件、空文件、中文空格文件名的 SFTP 往返，SHA-256 一致。支持目录查询、创建、重命名、删除；rmdir 使用非递归空目录删除。暂不支持追加、权限/时间戳保留（scp -p）、链接创建及旧 scp -O 协议。本机需要文件暂存空间，具体命令与限制见 README。
