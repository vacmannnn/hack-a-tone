package adapters

import (
	"context"
	"hack-a-tone/internal/core/port"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"log/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const TlsOFF = true

type KubeRuntimeController struct {
	client client.Client
	mgr    manager.Manager
}

func NewKubeRuntimeController() port.KubeController {
	return &KubeRuntimeController{}
}

func (ctrl *KubeRuntimeController) GetAllPods(ctx context.Context) (*corev1.PodList, error) {
	response := &corev1.PodList{}
	err := ctrl.client.List(ctx, response, &client.ListOptions{})

	// TODO: example options field client.InNamespace("default")
	return response, err
}

func (ctrl *KubeRuntimeController) RestartPod() {
	//TODO implement me
	panic("implement me")
}

func (ctrl *KubeRuntimeController) ScalePod(name string, scaleNumber int) {
	//TODO implement me
	panic("implement me")
}

func offTLS(cfg *rest.Config) {
	cfg.TLSClientConfig.Insecure = true
	cfg.TLSClientConfig.CAData = nil
	cfg.TLSClientConfig.CAFile = ""
}
func (ctrl *KubeRuntimeController) Start(ctx context.Context) error {
	cfg, err := config.GetConfig()
	if err != nil {
		slog.Error("Не удалось получить конфигурацию", "error", err)
		return err
	}

	if TlsOFF {
		offTLS(cfg)
	}

	mgr, err := manager.New(cfg, manager.Options{})
	if err != nil {
		slog.Error("Не удалось создать manager", "error", err)
		return err
	}

	go func() {
		slog.Debug("Создание manager и запуск...")
		if err = mgr.Start(ctx); err != nil {
			slog.Error("Ошибка запуска manager", "error", err)
		}
	}()

	ctrl.client = mgr.GetClient()
	ctrl.mgr = mgr

	return err
}
