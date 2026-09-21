# SEUSC — Cross-Platform Local SSH Bridge for SEU Supercomputing Platform

Version: 1.0
Target platforms: Windows 10/11, macOS Intel, macOS Apple Silicon
Core language: Go
GUI: Wails
Status: Implementation Spec

---

## 1. Goal

SEUSC turns the SEU supercomputing platform WebShell into a local SSH-compatible endpoint.

The end-user workflow should be:

1. Install SEUSC.
2. Open the app.
3. Enter SEU account and password.
4. SEUSC logs in automatically and runs in the background.
5. The user can then run:

```bash
ssh seusc
```

and get a normal shell on the SEU login node.

The user should not need to:

- Open WSL.
- Run Python.
- Open browser DevTools.
- Copy Bearer tokens.
- Copy cookies.
- Manually start a WebSocket client.
- Manually edit SSH config.

---

## 2. Confirmed SEU WebShell Behavior

Main site:

```text
https://sc.seu.edu.cn
```

WebShell endpoint:

```text
wss://sc.seu.edu.cn/finder/v2/webshell
```

Observed query parameters:

```text
Rows
Cols
Authorization
NodeId
```

Example structure:

```text
wss://sc.seu.edu.cn/finder/v2/webshell
?Rows=51
&Cols=112
&Authorization=Bearer+<TOKEN>
&NodeId=6
```

Observed normal website requests use:

```http
Authorization: Bearer <TOKEN>
```

Observed WebSocket handshake uses:

```http
Origin: https://sc.seu.edu.cn
```

The WebSocket returns terminal output as binary frames.

Terminal input is sent as binary frames.

Resize messages are JSON:

```json
{
  "type": "resize",
  "rows": 40,
  "cols": 120
}
```

A Python proof of concept has already successfully connected to the WebSocket and reached:

```text
[YOUR_SEU_ACCOUNT@login01 ~]$
```

Therefore the core bridge is known to work.

---

## 3. Product Requirements

### 3.1 Must Work

SEUSC v1 must support:

- Windows 10/11.
- macOS x86_64.
- macOS arm64.
- Native background process.
- GUI login.
- Username/password login.
- Persistent login session.
- Automatic Bearer token acquisition.
- Automatic re-login when session expires.
- Local SSH server.
- `ssh seusc`.
- Interactive terminal.
- PTY resize.
- ANSI terminal output.
- Ctrl+C.
- Ctrl+Z.
- Tab completion.
- Arrow keys.
- `vim`.
- `nano`.
- `less`.
- `top`.
- `tmux`.
- Multiple simultaneous SSH sessions.
- Configurable login node.
- Background autostart.
- Automatic SSH config creation.
- CLI status commands.
- Useful logs.

### 3.2 Nice to Have Later

Not required for first release:

- SCP.
- SFTP.
- VS Code Remote SSH.
- Port forwarding.
- SSH agent forwarding.
- X11 forwarding.
- Multi-account support.
- Linux GUI build.

---

## 4. Technology Stack

Use Go for the core application.

Recommended stack:

```text
Core language        Go
SSH server           golang.org/x/crypto/ssh
WebSocket            github.com/coder/websocket
Browser automation   github.com/chromedp/chromedp
GUI                   Wails
Logging               log/slog
Windows secrets       Windows Credential Manager / DPAPI
macOS secrets         Keychain
```

Do not use Python in the final product.

Do not require WSL.

---

## 5. Final Architecture

```text
                 +----------------------+
                 |   sc.seu.edu.cn      |
                 |                      |
                 | Web UI / Auth / API  |
                 +----------+-----------+
                            |
                     Bearer token
                            |
                            v
              +---------------------------+
              | /finder/v2/webshell       |
              | WebSocket PTY gateway     |
              +-------------+-------------+
                            |
                     binary PTY stream
                            |
                            v
+--------------------------------------------------+
|                  SEUSC Agent                     |
|                                                  |
|  Auth Manager                                    |
|  Browser Login Manager                           |
|  Bearer Token Manager                            |
|  WebShell Client                                 |
|  SSH Server                                      |
|  SSH <-> WebSocket Bridge                        |
|  Config Manager                                  |
|  Credential Store                                |
|  Auto-start Manager                              |
|  Logging                                         |
+----------------------+---------------------------+
                       |
                 127.0.0.1:24822
                       |
                       v
                  OpenSSH Client
                       |
                       v
                    ssh seusc
```

---

## 6. User Experience

### 6.1 First Launch

The GUI should show:

```text
SEUSC

SEU Account
[________________________]

Password
[________________________]

[ ] Remember password
[x] Start automatically

[ Login and Start ]
```

After successful login:

```text
SEUSC

Status       Connected
Account      YOUR_SEU_ACCOUNT
Node         login01
Local SSH    127.0.0.1:24822

Command:
ssh seusc
```

### 6.2 Daily Use

The GUI does not need to stay open.

The background agent starts automatically.

Users can use:

```bash
ssh seusc
```

or:

```bash
ssh seusc "hostname"
ssh seusc "squeue"
ssh seusc "git status"
```

---

## 7. Process Model

One executable should support several modes:

```text
seusc
seusc gui
seusc agent
seusc login
seusc logout
seusc status
seusc doctor
seusc start
seusc stop
seusc restart
```

Windows:

```text
seusc.exe
```

macOS:

```text
SEUSC.app
```

The GUI starts or connects to the background agent.

The background agent runs continuously.

---

## 8. Project Layout

Recommended repository structure:

```text
seusc/
├── cmd/
│   └── seusc/
│       └── main.go
│
├── internal/
│   ├── agent/
│   ├── auth/
│   ├── browser/
│   ├── bridge/
│   ├── config/
│   ├── credential/
│   ├── logging/
│   ├── platform/
│   ├── sshserver/
│   └── webshell/
│
├── frontend/
├── packaging/
│   ├── windows/
│   └── macos/
├── scripts/
├── tests/
└── docs/
```

---

## 9. Authentication Design

The software must automatically perform the normal SEU website login flow.

Do not hard-code Bearer tokens.

Do not make the user manually copy tokens.

The login manager should use a controlled Chromium-based browser through CDP.

Recommended implementation:

```text
SEUSC
  |
  +--> launch Chrome/Edge/Chromium with isolated SEUSC profile
  |
  +--> open https://sc.seu.edu.cn
  |
  +--> fill account/password
  |
  +--> submit login
  |
  +--> wait for successful authenticated page
  |
  +--> monitor outgoing requests
  |
  +--> capture:
       Authorization: Bearer <token>
```

The captured token is kept in memory.

The browser profile stores the login session.

If the session later expires, perform login again automatically.

If login requires manual interaction, open the visible browser and let the user complete it.

---

## 10. Credential Storage

Passwords must not be stored as plaintext files.

Windows:

```text
Windows Credential Manager
```

or DPAPI-protected storage.

macOS:

```text
Keychain
```

Configuration files may store:

```text
username
preferences
node
port
```

but not:

```text
password
Bearer token
cookies
```

---

## 11. Browser Profile

Use an isolated SEUSC browser profile.

Windows:

```text
%LOCALAPPDATA%\SEUSC\browser-profile
```

macOS:

```text
~/Library/Application Support/SEUSC/browser-profile
```

Do not use the user's normal Chrome profile.

The isolated profile exists only for SEUSC login/session persistence.

---

## 12. Token Manager

The Bearer token should be treated as an opaque string.

Do not assume it is JWT.

Do not assume a fixed lifetime.

Recommended interface:

```go
type TokenManager interface {
    Get(ctx context.Context) (string, error)
    Invalidate()
    Refresh(ctx context.Context) (string, error)
}
```

Behavior:

```text
Get
 |
 +--> cached token exists
 |      |
 |      +--> return it
 |
 +--> no token
        |
        +--> restore browser session
        |
        +--> obtain Bearer
        |
        +--> return Bearer
```

If a WebSocket connection fails due to authentication:

```text
invalidate token
refresh token
retry once
```

Do not loop forever.

---

## 13. WebShell Client

Recommended interface:

```go
type ConnectOptions struct {
    Token  string
    NodeID int
    Rows   int
    Cols   int
}

type Session interface {
    Read(p []byte) (int, error)
    Write(p []byte) (int, error)
    Resize(ctx context.Context, rows, cols int) error
    Close() error
}
```

WebSocket URL:

```text
wss://sc.seu.edu.cn/finder/v2/webshell
```

Required query fields:

```text
Rows
Cols
Authorization=Bearer+<TOKEN>
NodeId
```

Use proper URL query encoding.

Do not build credentials by unsafe string concatenation.

Handshake should include:

```http
Origin: https://sc.seu.edu.cn
```

If cookies are required, obtain them from the managed browser session.

---

## 14. WebSocket Data Handling

Incoming binary frames:

```text
write directly to SSH channel
```

Outgoing SSH input:

```text
send directly as binary WebSocket frame
```

Do not parse terminal output.

Do not decode ANSI.

Do not convert encodings.

Treat PTY data as raw bytes.

Resize:

```json
{
  "type": "resize",
  "rows": 40,
  "cols": 120
}
```

---

## 15. Local SSH Server

Default listener:

```text
127.0.0.1:24822
```

Also optionally listen on:

```text
::1
```

Do not listen publicly by default.

Use:

```text
golang.org/x/crypto/ssh
```

The SSH server must support:

```text
session channels
pty-req
shell
window-change
signal
exec
```

---

## 16. Local SSH Authentication

Generate one local client key during installation:

```text
~/.ssh/seusc_ed25519
```

Generate one SSH server host key.

The local SSH server only accepts the generated SEUSC client key.

This avoids asking the user for another password every time.

The SSH config should include:

```sshconfig
Host seusc
    HostName 127.0.0.1
    Port 24822
    User seusc
    IdentityFile ~/.ssh/seusc_ed25519
    IdentitiesOnly yes
```

---

## 17. SSH Config Management

Windows SSH config:

```text
%USERPROFILE%\.ssh\config
```

macOS SSH config:

```text
~/.ssh/config
```

SEUSC must only manage its own block:

```sshconfig
# BEGIN SEUSC MANAGED BLOCK
Host seusc
    HostName 127.0.0.1
    Port 24822
    User seusc
    IdentityFile ~/.ssh/seusc_ed25519
    IdentitiesOnly yes
# END SEUSC MANAGED BLOCK
```

Do not overwrite unrelated SSH configuration.

---

## 18. Interactive SSH Flow

When the user runs:

```bash
ssh seusc
```

the flow is:

```text
OpenSSH
  |
  v
SEUSC local SSH server
  |
  v
receive PTY request
  |
  v
AuthManager.GetToken()
  |
  v
connect WebShell with requested rows/cols
  |
  v
start bidirectional stream
```

Bridge:

```text
SSH channel stdin
    |
    v
WebSocket binary send

WebSocket binary recv
    |
    v
SSH channel stdout
```

Use one goroutine per direction.

---

## 19. Terminal Resize

When SSH receives:

```text
window-change
```

convert it immediately to:

```json
{
  "type": "resize",
  "rows": <new_rows>,
  "cols": <new_cols>
}
```

This is required for:

```text
vim
less
top
tmux
full-screen terminal apps
```

---

## 20. Control Characters

Interactive mode must preserve raw terminal bytes.

Examples:

```text
Ctrl+C  0x03
Ctrl+Z  0x1A
Ctrl+\  0x1C
```

Do not intercept these unnecessarily.

---

## 21. Multiple Sessions

Every SSH connection creates its own WebSocket session.

Example:

```text
Terminal A -> WebSocket A
Terminal B -> WebSocket B
Terminal C -> WebSocket C
```

Do not share one PTY between multiple SSH connections.

---

## 22. Disconnect Handling

When the SSH client disconnects:

```text
cancel session context
close WebSocket
stop bridge goroutines
release all resources
```

No zombie WebSocket connections.

If the SEU WebSocket unexpectedly disconnects:

```text
close the current SSH session
```

Do not silently reconnect an existing interactive PTY because shell state may be lost.

The user can simply run:

```bash
ssh seusc
```

again.

---

## 23. SSH Exec Mode

Support:

```bash
ssh seusc "hostname"
ssh seusc "squeue"
ssh seusc "pwd"
```

The backend is still PTY-based, so implement exec mode by opening a WebShell session and sending the command.

Recommended command wrapper:

```bash
(command)
rc=$?
printf '\n__SEUSC_DONE_<UUID>__:%d\n' "$rc"
exit
```

The bridge reads output until the unique marker appears.

Return the parsed exit status to the SSH client.

This mode only needs to work reliably for normal shell commands.

---

## 24. Node Management

Do not permanently assume one fixed NodeId.

Maintain a node map:

```go
type Node struct {
    ID   int
    Name string
}
```

Config example:

```json
{
  "nodes": {
    "login01": 6
  }
}
```

Later, if SEU exposes a stable node-list API, discover nodes automatically.

The GUI should support:

```text
Default node:
[ login01 v ]
```

---

## 25. Agent

The background agent owns:

```text
SSH listener
auth state
token cache
browser session
WebSocket session creation
config
logs
GUI IPC
```

The agent must be single-instance.

---

## 26. Auto Start

Windows:

Use per-user startup.

Recommended:

```text
HKCU\Software\Microsoft\Windows\CurrentVersion\Run
```

or equivalent startup task.

macOS:

Use LaunchAgent:

```text
~/Library/LaunchAgents/cn.seu.seusc.plist
```

No administrator permission should be required for normal installation.

---

## 27. GUI and Agent IPC

GUI should communicate with the background agent using local IPC.

Windows:

```text
Named Pipe
\\.\pipe\seusc-agent
```

macOS:

```text
Unix socket
~/Library/Application Support/SEUSC/agent.sock
```

Required IPC operations:

```text
status
login
logout
restart
set-default-node
get-logs
```

---

## 28. Agent States

Use clear states:

```text
STARTING
AUTH_REQUIRED
AUTHENTICATING
READY
ERROR
STOPPED
```

GUI should reflect the current state.

Example:

```text
READY
Account: YOUR_SEU_ACCOUNT
Node: login01
SSH: 127.0.0.1:24822
```

---

## 29. GUI

Keep the GUI simple.

Pages:

```text
Status
Account
Settings
Logs
About
```

Main status:

```text
SEUSC

● Connected

Account       YOUR_SEU_ACCOUNT
Node          login01
Local SSH     127.0.0.1:24822

ssh seusc

[ Copy Command ]

Start at login   ON
Remember login   ON
```

Tray / menu bar:

```text
SEUSC ● Connected
Open
Copy SSH command
Restart Agent
Quit
```

---

## 30. Configuration

Windows:

```text
%APPDATA%\SEUSC\config.json
```

macOS:

```text
~/Library/Application Support/SEUSC/config.json
```

Example:

```json
{
  "version": 1,
  "ssh": {
    "listen": "127.0.0.1",
    "port": 24822,
    "alias": "seusc"
  },
  "seu": {
    "base_url": "https://sc.seu.edu.cn",
    "default_node": "login01"
  },
  "agent": {
    "autostart": true
  },
  "logging": {
    "level": "info"
  }
}
```

---

## 31. Logging

Use structured logs.

Example:

```text
INFO agent started
INFO ssh listener started address=127.0.0.1:24822
INFO ssh session opened session=31af
INFO webshell connected node=login01 session=31af
INFO ssh session closed session=31af
```

Never log:

```text
password
full Bearer token
cookies
Authorization header
browser storage contents
```

Log locations:

Windows:

```text
%LOCALAPPDATA%\SEUSC\logs\
```

macOS:

```text
~/Library/Logs/SEUSC/
```

Use rotation.

Suggested:

```text
10 MB x 5 files
```

---

## 32. Error Handling

Use stable internal error codes:

```text
AUTH_REQUIRED
AUTH_FAILED
TOKEN_EXPIRED
BROWSER_NOT_FOUND
SEU_UNREACHABLE
WEBSOCKET_FAILED
SSH_PORT_IN_USE
SSH_CONFIG_CONFLICT
CREDENTIAL_STORE_FAILED
NODE_UNAVAILABLE
INTERNAL_ERROR
```

User-facing messages should be simple.

Example:

```text
Unable to connect to the SEU WebShell.
SEUSC will refresh the login session and retry.
```

---

## 33. Port Conflict

Default:

```text
127.0.0.1:24822
```

If occupied, automatically try:

```text
24823
24824
24825
...
```

Then update the managed SSH config block automatically.

---

## 34. Browser Selection

Preferred order:

Windows:

```text
Chrome
Edge
Chromium
bundled Chromium fallback
```

macOS:

```text
Chrome
Chromium
bundled Chromium fallback
```

Use the chosen browser only with the SEUSC isolated profile.

---

## 35. Headless Behavior

If an existing session is valid:

```text
use headless browser
```

If the user must interact:

```text
use visible browser
```

After Bearer acquisition, the browser does not need to stay running unless required for session refresh.

---

## 36. Security Baseline

The implementation should satisfy:

```text
no plaintext password storage
no public SSH listener
TLS verification enabled
local SSH key authentication
no full token logging
no cookie logging
isolated browser profile
```

Do not disable TLS verification.

---

## 37. Performance Targets

Agent startup:

```text
< 1 second
```

Idle CPU:

```text
approximately 0%
```

Core idle memory without browser:

```text
< 50 MB target
```

Total agent memory:

```text
< 100 MB target
```

Interactive terminal latency should be visually indistinguishable from the browser WebShell under the same network conditions.

---

## 38. Testing

### 38.1 Unit Tests

Test:

```text
config parsing
managed SSH config block
URL construction
resize message construction
token cache
SSH request parsing
node config
credential abstraction
```

### 38.2 Integration Tests

Build a mock WebSocket terminal server.

Test:

```text
SSH client
  <->
SEUSC SSH server
  <->
mock WebShell
```

Cases:

```text
banner output
echo input
resize
disconnect
auth failure
multiple simultaneous clients
exec command
```

### 38.3 Live Test

Optional live test:

```text
SEUSC_LIVE_TEST=1
```

Live tests must never run in normal CI.

---

## 39. Terminal Compatibility Checklist

Manually test:

```text
bash
zsh
vim
nano
less
top
htop
tmux
Ctrl+C
Ctrl+Z
Tab
arrow keys
Home
End
UTF-8 Chinese
terminal resize
multiple terminals
```

---

## 40. Windows Packaging

Target outputs:

```text
SEUSC-Setup-x64.exe
SEUSC-Setup-arm64.exe
```

Preferred install location:

```text
%LOCALAPPDATA%\Programs\SEUSC\
```

Normal install should not require admin rights.

Installer responsibilities:

```text
install binary
install GUI assets
create startup entry
initialize SSH keys
write SSH config
start agent
```

---

## 41. macOS Packaging

Target:

```text
SEUSC.dmg
```

Containing:

```text
SEUSC.app
```

Architectures:

```text
x86_64
arm64
```

Prefer a universal app if practical.

For public distribution:

```text
codesign
notarization
```

---

## 42. Build Targets

CI should build:

```text
windows/amd64
windows/arm64
darwin/amd64
darwin/arm64
```

---

## 43. Versioning

Use semantic versioning:

```text
1.0.0
1.0.1
1.1.0
2.0.0
```

---

## 44. CLI

Required CLI commands:

```bash
seusc status
seusc login
seusc logout
seusc start
seusc stop
seusc restart
seusc doctor
seusc node
seusc logs
```

Example:

```bash
seusc status
```

Output:

```text
SEUSC 1.0.0

Agent          running
Authentication valid
Account        YOUR_SEU_ACCOUNT
Default node   login01
Local SSH      127.0.0.1:24822

ssh seusc
```

---

## 45. Doctor Command

```bash
seusc doctor
```

Example output:

```text
[OK] configuration
[OK] credential store
[OK] SSH host key
[OK] local SSH key
[OK] SSH config
[OK] local port
[OK] browser
[OK] SEU website
[OK] authentication
[OK] WebSocket handshake
```

---

## 46. Development Milestones

### M0 — Existing PoC

Already done:

```text
Python WebSocket client
  ->
SEU WebShell
  ->
login01
```

### M1 — Go WebShell Client

Goal:

```text
Go client with manually supplied token
```

Acceptance:

```text
interactive shell works
resize works
Ctrl+C works
```

### M2 — Local SSH Server

Goal:

```bash
ssh -p 24822 seusc@127.0.0.1
```

opens the SEU shell.

### M3 — `ssh seusc`

Generate local key and SSH config.

Goal:

```bash
ssh seusc
```

works directly.

### M4 — Browser Authentication

Goal:

```bash
seusc login
```

automatically logs in and captures Bearer.

No manual token copy.

### M5 — Persistent Authentication

Restart SEUSC and restore login automatically.

### M6 — Secure Credential Storage

Windows Credential Manager / macOS Keychain.

### M7 — Background Agent

Single-instance daemon with auto-start.

### M8 — GUI

Wails status/login/settings UI.

### M9 — Windows and macOS Installers

Produce:

```text
SEUSC-Setup.exe
SEUSC.dmg
```

---

## 47. Definition of Done

Windows:

```text
install SEUSC
open app
enter account/password
login succeeds
close GUI
open PowerShell
ssh seusc
shell works
reboot
ssh seusc
shell still works
```

macOS:

```text
install SEUSC.app
login once
open Terminal
ssh seusc
shell works
reboot
ssh seusc
shell still works
```

---

## 48. SSH Acceptance Tests

The following must work:

```bash
ssh seusc
ssh seusc "hostname"
ssh seusc "pwd"
ssh seusc "squeue"
```

Interactive terminal must support:

```text
cd
ls
vim
less
top
tmux
Ctrl+C
Ctrl+Z
resize
Chinese UTF-8 output
multiple simultaneous sessions
```

---

## 49. Authentication Acceptance Tests

The user must never need to:

```text
open DevTools
copy Bearer
copy Cookie
run export
edit Python
open WSL
```

If a token expires:

```text
SEUSC refreshes it automatically
```

If the website session expires:

```text
SEUSC re-authenticates automatically
```

If automatic login is impossible:

```text
SEUSC opens the login browser and asks the user to finish login
```

---

## 50. Implementation Rules

Keep these rules:

```text
Go core
No Python dependency
No WSL dependency
No hard-coded token
No hard-coded cookie
No hard-coded NodeId forever
No plaintext password file
No public local SSH listener
No overwriting unrelated SSH config
One WebSocket per SSH session
Raw byte terminal bridge
GUI last
Protocol first
```

---

## 51. Recommended Implementation Order

Implement in this exact order:

```text
1. Go WebShell protocol
2. Go interactive terminal PoC
3. Local SSH server
4. SSH <-> WebSocket bridge
5. PTY resize
6. ssh seusc config generation
7. exec command support
8. Auth manager
9. Chromium/CDP login
10. Bearer capture
11. Credential storage
12. Background agent
13. Auto-start
14. Wails GUI
15. Windows installer
16. macOS app/dmg
17. Stability fixes
18. Release
```

Do not start with the GUI.

---

## 52. Final Product Definition

SEUSC is a cross-platform local SSH compatibility layer that authenticates against the SEU Supercomputing Platform, converts local OpenSSH sessions into authenticated SEU WebShell WebSocket PTY sessions, and exposes the service through a persistent local SSH endpoint.

The only command the user should normally need after installation is:

```bash
ssh seusc
```
