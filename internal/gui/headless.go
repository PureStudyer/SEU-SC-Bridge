//go:build !desktop

package gui

import (
	"errors"
	"github.com/PureStudyer/SEU-SC-Bridge/internal/platform"
)

func Run(p platform.Paths) error {
	return errors.New("this is a CLI build; run seusc login or use the desktop installer (build with -tags desktop)")
}

func RunTray(p platform.Paths) error { return nil }
