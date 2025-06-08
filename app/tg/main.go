package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hack-a-tone/internal/adapters"
	"hack-a-tone/internal/core/domain"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const (
	token    = "8000937203:AAHC8ZofmbGMGFw5gbOVPnfLqwdrgOarjYs"
	waitTime = 2 * time.Second
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	//db, err := storage.NewSQLRepo()
	//if err != nil {
	//	slog.Error("Не удалось создать репозиторий", "error", err)
	//}

	slog.SetDefault(adapters.SetupLogger(adapters.EnvLocal))

	controller := adapters.NewKubeRuntimeController()
	err := controller.Start(ctx)
	time.Sleep(waitTime)
	if err != nil {
		slog.Error("Не удалось запустить контроллер", "error", err)
		return
	}

	b := NewBot(token, controller, nil)

	go func() {
		http.HandleFunc("/alert", func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				fmt.Println(err)
				http.Error(w, "Error reading request body", http.StatusBadRequest)
				return
			}
			defer r.Body.Close()

			var alerts domain.Alerts
			err = json.Unmarshal(body, &alerts)
			if err != nil {
				fmt.Println(err)
				return
			}

			for _, alert := range alerts {
				b.SendMsg(alert)
			}
		})

		http.ListenAndServe(":3030", nil)
	}()

	b.start()
}
