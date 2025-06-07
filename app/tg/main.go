package main

import (
	"context"
	"hack-a-tone/internal/adapters"
	"log/slog"
	"os"
	"os/signal"
	"time"
)

const token = "8000937203:AAHC8ZofmbGMGFw5gbOVPnfLqwdrgOarjYs"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	slog.SetDefault(adapters.SetupLogger(adapters.EnvLocal))

	controller := adapters.NewKubeRuntimeController()
	err := controller.Start(ctx)

	time.Sleep(2 * time.Second)
	if err != nil {
		slog.Error("Не удалось запустить контроллер", "error", err)
		return
	}

	b := NewBot(token, controller)
	b.start()
}
