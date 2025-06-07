package adapters

import (
	"context"
	"fmt"
	"hack-a-tone/internal/core/port"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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

func (ctrl *KubeRuntimeController) ScalePod(ctx context.Context, deployName, nameSpace string, scaleNumber int32) error {
	deployment := &v1.Deployment{}
	if err := ctrl.client.Get(ctx, types.NamespacedName{Name: deployName, Namespace: nameSpace}, deployment); err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	if *deployment.Spec.Replicas+scaleNumber < 0 {
		return fmt.Errorf("count replicas less than zero")
	}

	*deployment.Spec.Replicas += scaleNumber

	if err := ctrl.client.Update(ctx, deployment); err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
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
