# 验证与复现

## 自动化（默认离线）

~~~sh
go test ./... -timeout 120s
go vet ./...
go test -race ./... -timeout 120s
~~~

race 需要 C 编译器。模拟 WebShell 集成测试启动随机回环端口，验证公钥拒绝、交互字节、缩放、SSH signal、exec 输出/退出码、五路并发、上游断线和不支持的 SFTP 请求。协议测试分别覆盖 binary 和 json 输入及 UTF-8 跨写入边界。

## 真实 SEU（默认禁用）

测试仅应使用自己有权限的账号，不把密码写入脚本或命令参数。PowerShell：

~~~powershell
$env:SEUSC_HOME = Join-Path $env:TEMP 'seusc-live-test'
./dist/windows-amd64/seusc.exe login
$env:SEUSC_LIVE_TEST = '1'
go test ./tests -run TestLiveInteractive -v -timeout 60s
ssh -F "$env:SEUSC_HOME/.ssh/config" seusc hostname
ssh -F "$env:SEUSC_HOME/.ssh/config" seusc pwd
ssh -F "$env:SEUSC_HOME/.ssh/config" seusc squeue
ssh -F "$env:SEUSC_HOME/.ssh/config" seusc 'exit 7'
$LASTEXITCODE
./dist/windows-amd64/seusc.exe restart
./dist/windows-amd64/seusc.exe status
./dist/windows-amd64/seusc.exe doctor
./dist/windows-amd64/seusc.exe logout
./dist/windows-amd64/seusc.exe stop
~~~

测试代理默认监听 24822；真实交互测试当前要求该端口可用。测试会运行 printf、stty、sleep、Ctrl+C 和 exit，不写入远端文件、不提交作业。

原生凭据存储测试仅使用随机名称的合成密码并清理：设置 SEUSC_CREDENTIAL_TEST=1 后运行 go test ./internal/credential -v。

## 手动平台验收

- Windows 10/11 x64、ARM64：安装、GUI 登录、关闭到托盘、重新打开、复制命令、电脑重启后的 ssh seusc。
- macOS Intel/Apple Silicon：DMG 拖入 Applications、Keychain 授权、菜单栏与 LaunchAgent、重启恢复。
- Shell 应用：bash/zsh、vim/nano/less/top/htop/tmux、Tab、方向键、Home/End、Ctrl+Z 后恢复作业、多终端及中文。
- 网络/认证：断网后关闭当前 SSH 会话；下一次连接可重新认证；验证码交由用户完成。
- 安装包升级、卸载保留配置、已有 Host seusc 冲突、端口被占用。

模拟测试不能证明所有交互式程序在所有系统上的行为。macOS GUI、ARM64 原生运行、真实重启及签名公证必须在目标机器验收。

## Live SFTP / native scp

Set SEUSC_LIVE_SFTP=1 only after authenticating the agent, then run go test ./tests -run TestLiveSFTP -v -timeout 150s. The test creates one uniquely named remote test directory, verifies SHA-256 for >10 MB binary, zero-byte and Chinese/space-named JSON files, checks listing and overwrite, and removes only its own test data. Normal CI skips it.

Native Windows OpenSSH 9.5p2 scp and sftp -b were separately verified against the installed application without -F.
