package domain

import (
	"fmt"
	"time"
)

type AlertDB struct {
	Namespace string
	Status    string
	Labels    Labels
}

func (a Alert) ConvertToDB(namespace string) AlertDB {
	return AlertDB{
		Namespace: namespace,
		Status:    a.Status,
		Labels:    a.Labels,
	}
}

type Alerts []Alert

type Alert struct {
	Status       string                 `json:"Status"`
	Labels       Labels                 `json:"Labels"`
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
	return fmt.Sprintf("Alert: %s🚨\n\tPod: %s\n\tProblem: %s", a.Labels.Alertname, a.Labels.Pod, a.Annotations.Summary)
}
