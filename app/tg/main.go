package main

import (
	"context"
	"encoding/json"
	"hack-a-tone/internal/adapters"
	"hack-a-tone/internal/adapters/storage"
	"hack-a-tone/internal/core/domain"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const waitTime = 2 * time.Second

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	db, err := storage.NewSQLRepo()
	if err != nil {
		slog.Error("Не удалось создать репозиторий", "error", err)
	}

	slog.SetDefault(adapters.SetupLogger(adapters.EnvLocal))

	controller := adapters.NewKubeRuntimeController()
	err = controller.Start(ctx)
	time.Sleep(waitTime)
	if err != nil {
		slog.Error("Не удалось запустить контроллер", "error", err)
		return
	}

	b := NewBot(os.Getenv("TG_BOT_KEY"), controller, db)

	go func() {
		http.HandleFunc("/alert", func(w http.ResponseWriter, r *http.Request) {
			slog.Info("Got alert ! Trying ro read body")
			body, err := io.ReadAll(r.Body)
			if err != nil {
				slog.Error("reading alert request body:", err)
				http.Error(w, "Error reading request body", http.StatusBadRequest)
				return
			}
			defer r.Body.Close()

			var alerts domain.Alerts
			err = json.Unmarshal(body, &alerts)
			if err != nil {
				slog.Error("unmarshalling alert:", err, "alert body:", string(body))
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
