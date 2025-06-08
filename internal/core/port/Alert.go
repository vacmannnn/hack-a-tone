package port

import (
	"hack-a-tone/internal/core/domain"
)

type AlertRepo interface {
	GetLastNAlerts(n int, namespaces []string) ([]domain.Alert, error)
	WriteAlert(alert domain.Alert, namespace string) error
}
