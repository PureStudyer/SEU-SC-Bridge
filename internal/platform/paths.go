package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Paths struct{ ConfigDir, DataDir, LogDir, Profile, SSHDir, Socket, Lock string }

func DefaultPaths() (Paths, error) {
	home, e := os.UserHomeDir()
	if e != nil {
		return Paths{}, e
	}
	var c, d, l string
	if override := os.Getenv("SEUSC_HOME"); override != "" {
		c = override
		d = override
		l = filepath.Join(override, "logs")
		home = override
	} else {
		switch runtime.GOOS {
		case "windows":
			c = filepath.Join(os.Getenv("APPDATA"), "SEUSC")
			d = filepath.Join(os.Getenv("LOCALAPPDATA"), "SEUSC")
			l = filepath.Join(d, "logs")
		case "darwin":
			c = filepath.Join(home, "Library", "Application Support", "SEUSC")
			d = c
			l = filepath.Join(home, "Library", "Logs", "SEUSC")
		default:
			c = filepath.Join(home, ".config", "seusc")
			d = c
			l = filepath.Join(c, "logs")
		}
	}
	p := Paths{ConfigDir: c, DataDir: d, LogDir: l, Profile: filepath.Join(d, "browser-profile"), SSHDir: filepath.Join(home, ".ssh"), Socket: filepath.Join(d, "agent.sock"), Lock: filepath.Join(d, "agent.lock")}
	for _, dir := range []string{c, d, l, p.SSHDir} {
		if e = os.MkdirAll(dir, 0700); e != nil {
			return p, e
		}
	}
	for _, dir := range []string{c, d, l} {
		if e = Protect(dir); e != nil {
			return p, fmt.Errorf("protect application directory: %w", e)
		}
	}
	return p, nil
}
