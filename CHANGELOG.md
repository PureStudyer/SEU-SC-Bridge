# Changelog

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
