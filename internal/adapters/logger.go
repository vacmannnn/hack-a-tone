package adapters

import (
	"log/slog"
	"os"
)

type LogLevel int

const (
	EnvLocal LogLevel = iota
	EndDev
	EnvProd
)

func SetupLogger(level LogLevel) *slog.Logger {
	var log *slog.Logger
	switch level {
	case EnvLocal:
		log = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case EndDev:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case EnvProd:
		log = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return log
}
