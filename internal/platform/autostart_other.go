//go:build !windows && !darwin

package platform

import "errors"

func AutoStart(exe string, enabled bool) error {
	if !enabled {
		return nil
	}
	return errors.New("autostart is supported on Windows and macOS")
}
