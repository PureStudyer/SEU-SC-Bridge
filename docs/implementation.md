# Implementation and validation status

Validated on Windows amd64, 2026-09-21.

Implemented: Go WebShell transport (binary + current JSON input), SSH public-key server, interactive/exec bridge, resize/signals, concurrent sessions, configurable nodes, managed SSH config and pinned host key, browser login and persistent profile restore, coalesced token refresh, OS credential abstraction, private IPC, background process, autostart, Wails desktop/tray, logs, CLI/doctor, Windows installers, macOS bundle scripts, CI.

Checks completed:
- go test ./... and go vet ./... pass.
- Mock integration: unauthorized key rejection, raw terminal bytes, resize, signal, exec markers/status, five concurrent clients, upstream disconnect, SFTP rejection.
- Native Windows Credential Manager roundtrip passes with synthetic test credentials and cleanup.
- Real SEU: automatic username/password login, interactive UTF-8, PTY resize, Ctrl+C, hostname/pwd/squeue, exit 7, browser session persistence.
- Installed per-user Windows application: normal ssh seusc hostname/pwd/squeue without -F; doctor passes; agent restart followed by normal ssh seusc hostname passes.
- Wails Windows GUI starts and displays real READY status.
- Windows amd64/arm64 desktop executables and both NSIS installers generated.
- Darwin amd64/arm64 CLI cross-compilation passes.

Not yet verified: Windows ARM64 native execution, macOS GUI/Keychain/LaunchAgent/DMG on actual macOS, machine reboot, full terminal application matrix, public signing/notarization. Local race tests require a C compiler (not available); CI has race testing configured. Check Actions for current cross-platform CI results. Publishing is tracked in the repository history and Releases.

Current platform differs from the supplied spec: input must be JSON {type:input,data:...}; binary terminal output and JSON resize remain. The default uses JSON input, with binary mode retained. See README for exec/PTY and UTF-8 limits.

The supplied account password was used through process stdin and memory only; it was not placed in source, config, logs, or the OS password store. Normal browser session data is persisted in the isolated Chromium profile. Initial tests used SEUSC_HOME and explicit ssh -F; final installed tests use the actual user SSH config and no -F. The isolated test account was logged out and that agent stopped.

Branding: repository and distributed application renamed SEU SC Bridge; native multi-resolution ICO/ICNS, executable resource ID 3, installer icons and tray artwork integrated. The seusc command, SSH alias, IPC/state directory identifiers are retained for compatibility.

File transfer: added a standard SFTP subsystem backed by authenticated SEU Finder HTTP APIs. Real OpenSSH scp upload, overwrite and download passed SHA-256 comparison; a native SFTP batch roundtrip passed for >10 MB binary, empty and Chinese/space-named JSON files. Automated regressions cover multipart intermediate acknowledgements, merge, API business errors, authentication retry, open/truncate semantics and cancellation without publishing partial data.
