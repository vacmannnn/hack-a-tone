package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hack-a-tone/internal/adapters"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const token = "8000937203:AAHC8ZofmbGMGFw5gbOVPnfLqwdrgOarjYs"

type Alerts []Alert

type Alert struct {
	Status       string                 `json:"status"`
	Labels       Labels                 `json:"labels"`
	Annotations  Annotations            `json:"annotations"`
	StartsAt     time.Time              `json:"startsAt"`
	EndsAt       time.Time              `json:"endsAt"`
	GeneratorURL string                 `json:"generatorURL"`
	Fingerprint  string                 `json:"fingerprint"`
	SilenceURL   string                 `json:"silenceURL"`
	DashboardURL string                 `json:"dashboardURL"`
	PanelURL     string                 `json:"panelURL"`
	Values       map[string]interface{} `json:"values"`
	ValueString  string                 `json:"valueString"`
	OrgId        int                    `json:"orgId"`
}

type Labels struct {
	Alertname     string `json:"alertname"`
	GrafanaFolder string `json:"grafana_folder"`
	Pod           string `json:"pod"`
}

type Annotations struct {
	Summary string `json:"summary"`
}

func (a Alert) String() string {
	return fmt.Sprintf("Alert %s.\nВ поде %s проблема: %s.", a.Labels.Alertname, a.Labels.Pod, a.Annotations.Summary)
}

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

	go func() {
		http.HandleFunc("/alert", func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				fmt.Println(err)
				http.Error(w, "Error reading request body", http.StatusBadRequest)
				return
			}
			defer r.Body.Close()

			var alerts Alerts
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
