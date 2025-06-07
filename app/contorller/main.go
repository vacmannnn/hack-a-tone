package main

import (
	"context"
	"fmt"
	"hack-a-tone/internal/adapters"
	"log/slog"
	"os"
	"os/signal"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	namespace := ""

	defer stop()

	slog.SetDefault(adapters.SetupLogger(adapters.EnvLocal))

	controller := adapters.NewKubeRuntimeController()
	err := controller.Start(ctx)

	time.Sleep(2 * time.Second)
	if err != nil {
		slog.Error("Не удалось запустить контроллер", "error", err)
		return
	}

	p, err := controller.GetAllPods(ctx, namespace)
	if err != nil {
		slog.Error("Не удалось получить список подов", "error", err)
		return
	}

	for _, pod := range p.Items {
		d, err := controller.GetDeploymentFromPod(ctx, &pod)
		if err != nil {
			slog.Error("Не удалось получить имя деплоймента", "error", err)
			d = "unknown"
		}
		fmt.Printf("Pod: %s, Namespace: %s, DeployName %s, Status: %s\n", pod.Name, pod.Namespace, d, pod.Status.Phase)
	}

	// -------------

	//err = controller.ScalePod(ctx, "my-nginx", "default", 2)
	//if err != nil {
	//	slog.Error("Не удалось увеличить количество подов", "error", err)
	//	return
	//}

	//err = controller.RestartDeployment(ctx, "my-nginx", "default")
	//if err != nil {
	//	slog.Error("Не удалось перезагрузить деплоймент", "error", err)
	//	return
	//}
}
