# 参与开发

欢迎通过 Issue 报告问题，通过 Pull Request 提交改进。

## 本地开发

1. 安装 Go 1.26+；桌面运行需要浏览器和平台 WebView。
2. 运行 go test ./... -timeout 120s 与 go vet ./...。
3. 核心 CLI：go build ./cmd/seusc；桌面版本使用 scripts/build.ps1 或 scripts/build-macos.sh。
4. 并发变更运行 go test -race ./...（需要 C 编译器）。

测试默认使用模拟终端，不需要真实账号。SEUSC_HOME 可隔离本地配置和 SSH 密钥；不要在测试环境开启正式自启动。真实 SEU 验证必须显式选择，见 docs/testing.md。

## 提交要求

- 说明问题触发条件、变更后的行为及测试结果。
- 对协议、认证和资源生命周期变更增加针对性回归测试。
- 不提交账号、密码、Token、Cookie、日志、浏览器 profile、私钥或个人绝对路径。
- 不禁用 TLS 校验，不开放公网 SSH，不扩大 IPC 访问范围。
- 保留用户的非 SEUSC SSH 配置；上游 PTY 断线不静默重连。
- UI 或协议变更同步更新文档；未验证的平台明确注明。

PR 请基于 main 创建分支，保持修改聚焦。
