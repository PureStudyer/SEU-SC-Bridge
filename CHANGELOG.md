# Changelog

## 1.1.0 — 2026-11-24

- `ssh seusc` 现在通过 Bridge 管理的浏览器打开官方 WebSSH 页面。每个 SSH 会话使用独立标签页，退出时自动关闭。
- 本地终端和命令行 agent 可继续使用原有 SSH 别名。已在 Windows x64 实机验证并发命令、远端退出码、中文输入输出、Ctrl+C 和窗口缩放。
- 登录页为已保存的密码显示掩码，支持显示或隐藏已保存和正在输入的密码。
- `seusc doctor` 使用浏览器检查站点和终端。官方页面拒绝连接时，Bridge 显示 HTTP 403 并停止自动重试。

## 1.0.3 — 2026-10-26

- Automatically recover an unresponsive background agent without leaving its single-instance lock stuck.
- Reauthenticate after expired WebShell tokens and verify the connection during background recovery.
- Keep the recovery browser profile between reconnects and show whether a password is actually stored.

## 1.0.2 — 2026-10-04

- Add in-app release checks and verified Windows update downloads.
- Show the actual application version in the desktop interface.
- Recover expired sessions and retry transient SEU and WebShell outages automatically.
- Restore remembered-password sign-in and recover from stale browser profile locks.
- Improve startup diagnostics when an existing background agent cannot be reached.

## 1.0.0 — 2026-09-21 · Public Preview

### Added

- Go SSH → SEU WebShell 桥接，独立多会话、PTY 缩放、控制字符和 exec 退出码。
- Chromium/CDP 自动登录、隔离浏览器 profile、会话恢复和一次鉴权刷新重试。
- Windows Credential Manager / macOS Keychain 凭据接口。
- 专用 SSH 客户端和主机密钥、受控 SSH config 区块与 known_hosts。
- 回环监听、私有 Named Pipe / Unix socket IPC、单实例后台代理。
- Wails 桌面登录、状态、设置、日志、托盘 / 菜单栏和自启动。
- CLI 状态、登录、退出、启停、节点、日志和 doctor 命令。
- Windows x64 / ARM64 安装器，macOS 原生打包脚本，跨平台 CI。
- 项目封面、中文手册、测试说明和 MIT 许可证。

### Compatibility

- 当前 SEU 前端发送 JSON input；默认采用该协议，同时保留 binary 模式。
- Windows x64 完成真实 OpenSSH 和代理重启恢复验证。其他平台的实机验收未完成。
- 暂不支持 旧版 SCP（scp -O）、端口转发及 VS Code Remote SSH；exec 的 PTY 限制见 README。

## SFTP / scp 文件传输

支持标准 SFTP 与现代 OpenSSH scp，通过 SEU Finder 接口上传和下载。已实测 1 MB 随机二进制 scp 往返，以及超过 10 MB 的分块文件、空文件、中文空格文件名的 SFTP 往返，SHA-256 一致。支持目录查询、创建、重命名、删除；rmdir 使用非递归空目录删除。暂不支持追加、权限/时间戳保留（scp -p）、链接创建及旧 scp -O 协议。本机需要文件暂存空间，具体命令与限制见 README。
