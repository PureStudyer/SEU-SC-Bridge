package logging

import (
	"gopkg.in/natefinch/lumberjack.v2"
	"log/slog"
	"path/filepath"
)

func New(dir, level string) (*slog.Logger, *lumberjack.Logger) {
	r := &lumberjack.Logger{Filename: filepath.Join(dir, "seusc.log"), MaxSize: 10, MaxBackups: 5, MaxAge: 30, Compress: true}
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewJSONHandler(r, &slog.HandlerOptions{Level: l})), r
}
