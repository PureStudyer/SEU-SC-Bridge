//go:build darwin

package platform

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
)

func AutoStart(exe string, enabled bool) error {
	h, e := os.UserHomeDir()
	if e != nil {
		return e
	}
	p := filepath.Join(h, "Library", "LaunchAgents", "cn.seu.seusc.plist")
	if !enabled {
		e = os.Remove(p)
		if os.IsNotExist(e) {
			return nil
		}
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(exe))
	return os.WriteFile(p, []byte(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"><plist version="1.0"><dict><key>Label</key><string>cn.seu.seusc</string><key>ProgramArguments</key><array><string>`+b.String()+`</string><string>agent</string></array><key>RunAtLoad</key><true/></dict></plist>`), 0600)
}
