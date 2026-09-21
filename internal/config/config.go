package config

import (
	"encoding/json"
	"errors"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/apperr"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
)

type Config struct {
	Version  int    `json:"version"`
	Username string `json:"username"`
	Remember bool   `json:"remember_password"`
	SSH      struct {
		Listen string `json:"listen"`
		Port   int    `json:"port"`
		Alias  string `json:"alias"`
	} `json:"ssh"`
	SEU struct {
		InputMode   string `json:"input_mode"`
		BaseURL     string `json:"base_url"`
		DefaultNode string `json:"default_node"`
	} `json:"seu"`
	Nodes map[string]int `json:"nodes"`
	Agent struct {
		AutoStart bool `json:"autostart"`
	} `json:"agent"`
	Logging struct {
		Level string `json:"level"`
	} `json:"logging"`
	Browser struct {
		Path             string `json:"path,omitempty"`
		UsernameSelector string `json:"username_selector"`
		PasswordSelector string `json:"password_selector"`
		SubmitSelector   string `json:"submit_selector"`
	} `json:"browser"`
}

func Default() Config {
	var c Config
	c.Version = 1
	c.SSH.Listen = "127.0.0.1"
	c.SSH.Port = 24822
	c.SSH.Alias = "seusc"
	c.SEU.BaseURL = "https://sc.seu.edu.cn"
	c.SEU.InputMode = "json"
	c.SEU.DefaultNode = "login01"
	c.Nodes = map[string]int{"login01": 6}
	c.Agent.AutoStart = true
	c.Logging.Level = "info"
	c.Browser.UsernameSelector = "#form_item_username"
	c.Browser.PasswordSelector = "#form_item_password"
	c.Browser.SubmitSelector = "button.ant-btn-primary"
	return c
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func (c Config) Validate() error {
	if c.SEU.InputMode != "binary" && c.SEU.InputMode != "json" {
		return errors.New("invalid WebShell input mode")
	}
	if c.Version != 1 {
		return errors.New("unsupported configuration version")
	}
	ip := net.ParseIP(c.SSH.Listen)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("SSH listen must be a loopback IP")
	}
	if c.SSH.Port < 1024 || c.SSH.Port > 65535 || !safeName.MatchString(c.SSH.Alias) {
		return errors.New("invalid SSH port or alias")
	}
	u, e := url.Parse(c.SEU.BaseURL)
	if e != nil || u.Scheme != "https" || u.Host != "sc.seu.edu.cn" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("SEU base URL must be https://sc.seu.edu.cn")
	}
	if c.Nodes[c.SEU.DefaultNode] < 1 {
		return apperr.New("NODE_UNAVAILABLE", "默认节点不存在")
	}
	for name, id := range c.Nodes {
		if !safeName.MatchString(name) || id < 1 {
			return errors.New("invalid node mapping")
		}
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		return errors.New("invalid logging level")
	}
	return nil
}
func Load(dir string) (Config, error) {
	c := Default()
	b, e := os.ReadFile(filepath.Join(dir, "config.json"))
	if os.IsNotExist(e) {
		return c, nil
	}
	if e != nil {
		return c, e
	}
	if e = json.Unmarshal(b, &c); e != nil {
		return c, e
	}
	return c, c.Validate()
}
func Save(dir string, c Config) error {
	if e := c.Validate(); e != nil {
		return e
	}
	b, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	return AtomicWrite(filepath.Join(dir, "config.json"), append(b, '\n'), 0600)
}
func AtomicWrite(p string, b []byte, mode os.FileMode) error {
	f, e := os.CreateTemp(filepath.Dir(p), ".seusc-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(mode); e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, p)
}
