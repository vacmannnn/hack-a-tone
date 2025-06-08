package main

import (
	"context"
	"fmt"
	"hack-a-tone/internal/adapters"
	"hack-a-tone/internal/core/utils"
	"log/slog"
	"os"
	"os/signal"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	//namespace := ""

	defer stop()

	slog.SetDefault(adapters.SetupLogger(adapters.EnvLocal))

	controller := adapters.NewKubeRuntimeController()
	err := controller.Start(ctx)

	time.Sleep(2 * time.Second)
	if err != nil {
		slog.Error("Не удалось запустить контроллер", "error", err)
		return
	}

	ds, err := controller.GetDeployments(ctx, "")
	if err != nil {
		slog.Error("Не удалось получить список деплойментов", "error", err)
		return
	}

	for _, d := range ds.Items {
		fmt.Println(d.Name, "have replicas count", utils.GetReplicasCountForDeployment(&d))
	}
}
