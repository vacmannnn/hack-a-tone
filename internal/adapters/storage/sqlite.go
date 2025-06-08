package storage

import (
	"database/sql"
	_ "database/sql"
	"encoding/json"
	_ "github.com/mattn/go-sqlite3"
	"hack-a-tone/internal/core/domain"
	"log/slog"
)

type SQLRepo struct {
	db *sql.DB
}

func NewSQLRepo() (*SQLRepo, error) {
	db, err := sql.Open("sqlite3", "./alerts.db")
	if err != nil {
		slog.Error("Не удалось открыть базу данных", "error", err)
		return nil, err
	}

	_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS alerts (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            namespace TEXT,
            status TEXT,
            labels TEXT,
            summary    TEXT,
            created_at DATETIME DEFAULT CURRENT_TIMESTAMP
        )`,
	)

	return &SQLRepo{
		db: db,
	}, nil
}

func (r *SQLRepo) GetLastNAlerts(n int, namespaces []string) ([]domain.Alert, error) {
	res := make([]domain.Alert, 0)

	for _, namespace := range namespaces {
		alerts, err := r.getLastNAlerts(n, namespace)
		if err != nil {
			slog.Error("Не удалось получить алерты из базы по namespace: "+namespace, "error", err)
		} else {
			res = append(res, alerts...)
		}
	}

	return res, nil
}

func (r *SQLRepo) getLastNAlerts(n int, namespace string) ([]domain.Alert, error) {
	rows, err := r.db.Query(`
        SELECT status, labels, summary, created_at
        FROM alerts
        WHERE namespace = ?
        ORDER BY created_at DESC
        LIMIT ?
    `, namespace, n)
	if err != nil {
		slog.Error("Ошибка выборки алертов из базы", "error", err)
		return nil, err
	}
	defer rows.Close()

	var alerts []domain.Alert

	for rows.Next() {
		var status string
		var labelsJSON string
		var createdAt string
		var summary string

		err = rows.Scan(&status, &labelsJSON, &summary, &createdAt)
		if err != nil {
			slog.Error("Ошибка чтения строки из базы", "error", err)
			return nil, err
		}

		var labels domain.Labels
		err = json.Unmarshal([]byte(labelsJSON), &labels)
		if err != nil {
			slog.Error("Ошибка десериализации меток", "error", err)
			return nil, err
		}

		alert := domain.Alert{
			Status:      status,
			Labels:      labels,
			Annotations: domain.Annotations{Summary: summary},
		}

		alerts = append(alerts, alert)
	}

	if err = rows.Err(); err != nil {
		slog.Error("Ошибка после итерации по строкам", "error", err)
		return nil, err
	}

	return alerts, nil
}

func (r *SQLRepo) WriteAlert(alert domain.Alert, namespace string) error {
	alertDB := alert.ConvertToDB(namespace)

	labelsJson, err := json.Marshal(alertDB.Labels)
	if err != nil {
		slog.Error("Не удалось сериализовать метки", "error", err)
		return err
	}

	_, err = r.db.Exec(
		"INSERT INTO alerts (namespace, status, labels, summary) VALUES (?, ?, ?, ?)",
		alertDB.Namespace, alertDB.Status, string(labelsJson), alert.Annotations.Summary,
	)

	return err
}
