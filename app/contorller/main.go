package main

import (
	"context"
	"fmt"
	"hack-a-tone/internal/adapters"
	"log/slog"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	slog.SetDefault(adapters.SetupLogger(adapters.EnvLocal))

	controller := adapters.NewKubeRuntimeController()
	err := controller.Start(ctx)
	if err != nil {
		slog.Error("Не удалось запустить контроллер", "error", err)
		return
	}

	p, err := controller.GetAllPods(ctx)
	if err != nil {
		slog.Error("Не удалось получить список подов", "error", err)
		return
	}

	for _, pod := range p.Items {
		fmt.Printf("Pod: %s, Namespace: %s, Status: %s\n", pod.Name, pod.Namespace, pod.Status.Phase)
	}
}
