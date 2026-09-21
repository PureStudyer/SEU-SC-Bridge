package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/agent"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/browser"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/config"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/credential"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/gui"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/ipc"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(args []string) error {
	command := "gui"
	if len(args) > 0 {
		command = args[0]
		args = args[1:]
	}
	if command == "version" || command == "--version" {
		fmt.Println("SEU SC Bridge", agent.Version)
		return nil
	}
	if command == "help" || command == "--help" || command == "-h" {
		fmt.Println("SEU SC Bridge " + agent.Version + "\nUsage: seusc [gui|agent|login|logout|status|doctor|start|stop|restart|node|logs|autostart|init|version]\nlogin [--username ACCOUNT] [--remember] [--visible] [--password-stdin]\nnode [NAME [ID]]    autostart on|off    status [--json]\nPasswords are never accepted as command-line arguments.")
		return nil
	}
	p, e := platform.DefaultPaths()
	if e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	switch command {
	case "close-gui":
		gp := p
		gp.DataDir = p.DataDir + "-gui"
		gp.Socket = filepath.Join(p.DataDir, "gui.sock")
		probe, c := context.WithTimeout(ctx, 3*time.Second)
		defer c()
		_ = ipc.Call(probe, gp, ipc.Request{Op: "quit"}, nil)
		return nil
	case "tray":
		return gui.RunTray(p)
	case "gui":
		return gui.Run(p)
	case "agent":
		return agent.Run(ctx, p)
	case "start":
		if e := agent.Start(ctx, p); e != nil {
			return e
		}
		fmt.Println("SEUSC agent started")
		return nil
	case "stop":
		stop, c := context.WithTimeout(ctx, 15*time.Second)
		defer c()
		return agent.Stop(stop, p)
	case "restart":
		stop, c := context.WithTimeout(ctx, 15*time.Second)
		defer c()
		_ = agent.Stop(stop, p)
		return agent.Start(ctx, p)
	case "init":
		cfg, e := config.Load(p.ConfigDir)
		if e != nil {
			return e
		}
		_, _, e = config.EnsureSSH(p, cfg)
		if e != nil {
			return e
		}
		return config.Save(p.ConfigDir, cfg)
	case "login":
		flags := flag.NewFlagSet("login", flag.ContinueOnError)
		user := flags.String("username", "", "SEU account")
		remember := flags.Bool("remember", false, "save password in OS credential store")
		visible := flags.Bool("visible", false, "open visible login browser")
		stdin := flags.Bool("password-stdin", false, "read password from standard input")
		if e = flags.Parse(args); e != nil {
			return e
		}
		cfg, e := config.Load(p.ConfigDir)
		if e != nil {
			return e
		}
		reader := bufio.NewReader(os.Stdin)
		if *user == "" {
			*user = cfg.Username
		}
		if *user == "" {
			fmt.Fprint(os.Stderr, "SEU account: ")
			line, e := reader.ReadString('\n')
			if e != nil {
				return e
			}
			*user = strings.TrimSpace(line)
		}
		password := ""
		if *stdin {
			line, e := reader.ReadString('\n')
			if e != nil && len(line) == 0 {
				return e
			}
			password = strings.TrimRight(line, "\r\n")
		} else if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprint(os.Stderr, "Password (empty = browser session): ")
			b, e := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			if e != nil {
				return e
			}
			password = string(b)
		}
		if e = agent.Start(ctx, p); e != nil {
			return e
		}
		var st agent.Status
		if e = ipc.Call(ctx, p, ipc.Request{Op: "login", Username: *user, Password: password, Remember: *remember, Visible: *visible}, &st); e != nil {
			return e
		}
		printStatus(st)
		return nil
	case "logout":
		return ipc.Call(ctx, p, ipc.Request{Op: "logout"}, nil)
	case "status":
		probe, c := context.WithTimeout(ctx, 2*time.Second)
		defer c()
		var st agent.Status
		if e = ipc.Call(probe, p, ipc.Request{Op: "status"}, &st); e != nil {
			return e
		}
		if len(args) > 0 && args[0] == "--json" {
			return json.NewEncoder(os.Stdout).Encode(st)
		}
		printStatus(st)
		return nil
	case "node":
		if len(args) == 0 {
			var st agent.Status
			if e = ipc.Call(ctx, p, ipc.Request{Op: "status"}, &st); e != nil {
				return e
			}
			for name, id := range st.Nodes {
				mark := ""
				if name == st.Node {
					mark = " *"
				}
				fmt.Printf("%s %d%s\n", name, id, mark)
			}
			return nil
		}
		id := 0
		if len(args) > 1 {
			id, e = strconv.Atoi(args[1])
			if e != nil || id < 1 {
				return fmt.Errorf("node ID must be positive")
			}
		}
		return ipc.Call(ctx, p, ipc.Request{Op: "set-default-node", Node: args[0], NodeID: id}, nil)
	case "logs":
		var logs string
		if e = ipc.Call(ctx, p, ipc.Request{Op: "get-logs"}, &logs); e != nil {
			return e
		}
		fmt.Print(logs)
		return nil
	case "autostart":
		if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
			return fmt.Errorf("usage: seusc autostart on|off")
		}
		if e = agent.Start(ctx, p); e != nil {
			return e
		}
		on := args[0] == "on"
		return ipc.Call(ctx, p, ipc.Request{Op: "settings", AutoStart: &on}, nil)
	case "doctor":
		return doctor(ctx, p)
	default:
		return fmt.Errorf("unknown command %q; run seusc help", command)
	}
}
func printStatus(st agent.Status) {
	fmt.Printf("SEU SC Bridge %s\n\nAgent          running (PID %d)\nState          %s\nAccount        %s\nDefault node   %s\nLocal SSH      %s\nError          %s\n\nssh %s\n", st.Version, st.PID, st.State, st.Account, st.Node, st.Address, st.ErrorCode, st.Alias)
}
func doctor(ctx context.Context, p platform.Paths) error {
	failed := false
	check := func(name string, e error) {
		if e != nil {
			failed = true
			fmt.Printf("[FAIL] %s: %s\n", name, e)
		} else {
			fmt.Println("[OK]", name)
		}
	}
	cfg, e := config.Load(p.ConfigDir)
	check("configuration", e)
	if e != nil {
		return e
	}
	_, e = browser.Find(cfg.Browser.Path)
	check("browser", e)
	for name, file := range map[string]string{"SSH host key": filepath.Join(p.DataDir, "ssh_host_ed25519_key"), "local SSH key": filepath.Join(p.SSHDir, "seusc_ed25519")} {
		b, e := os.ReadFile(file)
		if e == nil {
			_, e = ssh.ParsePrivateKey(b)
		}
		check(name, e)
	}
	b, e := os.ReadFile(filepath.Join(p.SSHDir, "config"))
	if e == nil && !strings.Contains(string(b), "# BEGIN SEUSC MANAGED BLOCK") {
		e = fmt.Errorf("managed block missing")
	}
	check("SSH config", e)
	_, e = (credential.System{}).Get(cfg.Username)
	check("credential store", e)
	probe, c := context.WithTimeout(ctx, 3*time.Second)
	var st agent.Status
	e = ipc.Call(probe, p, ipc.Request{Op: "status"}, &st)
	c()
	check("agent IPC", e)
	if e == nil {
		clientBytes, kerr := os.ReadFile(filepath.Join(p.SSHDir, "seusc_ed25519"))
		hostBytes, herr := os.ReadFile(filepath.Join(p.DataDir, "ssh_host_ed25519_key"))
		if kerr == nil && herr == nil {
			client, ke := ssh.ParsePrivateKey(clientBytes)
			host, he := ssh.ParsePrivateKey(hostBytes)
			if ke == nil && he == nil {
				conn, ce := ssh.Dial("tcp", st.Address, &ssh.ClientConfig{User: "seusc", Auth: []ssh.AuthMethod{ssh.PublicKeys(client)}, HostKeyCallback: ssh.FixedHostKey(host.PublicKey()), Timeout: 3 * time.Second})
				if ce == nil {
					conn.Close()
				}
				check("local SSH port and key authentication", ce)
			}
		}
	}
	if e == nil {
		authErr := error(nil)
		if st.State != "READY" {
			authErr = fmt.Errorf("state %s", st.State)
		}
		check("authentication", authErr)
		site, c := context.WithTimeout(ctx, 15*time.Second)
		e = ipc.Call(site, p, ipc.Request{Op: "check-site"}, nil)
		c()
		check("SEU website", e)
		if st.State == "READY" {
			ws, c := context.WithTimeout(ctx, 45*time.Second)
			e = ipc.Call(ws, p, ipc.Request{Op: "check-webshell"}, nil)
			c()
			check("WebSocket handshake", e)
		}
	}
	if failed {
		return fmt.Errorf("some checks failed")
	}
	return nil
}
