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
	"time"
)

const TlsOFF = true

type KubeRuntimeController struct {
	client client.Client
	mgr    manager.Manager
}

func NewKubeRuntimeController() port.KubeController {
	return &KubeRuntimeController{}
}

func (ctrl *KubeRuntimeController) GetDeploymentFromPod(ctx context.Context, pod *corev1.Pod) (string, error) {
	var replicaSet *v1.ReplicaSet
	for _, ownerRef := range pod.OwnerReferences {
		if ownerRef.Kind == "ReplicaSet" {
			rs := &v1.ReplicaSet{}
			err := ctrl.client.Get(ctx, client.ObjectKey{
				Namespace: pod.Namespace,
				Name:      ownerRef.Name,
			}, rs)
			if err != nil {
				return "", fmt.Errorf("failed to get ReplicaSet %s: %w", ownerRef.Name, err)
			}
			replicaSet = rs
			break
		}
	}
	if replicaSet == nil {
		return "", fmt.Errorf("no ReplicaSet owner reference found for Pod %s", pod.Name)
	}
	for _, ownerRef := range replicaSet.OwnerReferences {
		if ownerRef.Kind == "Deployment" {
			return ownerRef.Name, nil
		}
	}
	return "", fmt.Errorf("no Deployment owner reference found for ReplicaSet %s", replicaSet.Name)
}

func (ctrl *KubeRuntimeController) GetAllPods(ctx context.Context, nameSpace string) (*corev1.PodList, error) {
	response := &corev1.PodList{}

	opt := &client.ListOptions{}
	if nameSpace != "" {
		opt.Namespace = nameSpace
	}
	err := ctrl.client.List(ctx, response, opt)

	return response, err
}

func (ctrl *KubeRuntimeController) GetDeployments(ctx context.Context, nameSpace string) (*v1.DeploymentList, error) {
	response := &v1.DeploymentList{}

	opt := &client.ListOptions{}
	if nameSpace != "" {
		opt.Namespace = nameSpace
	}

	err := ctrl.client.List(ctx, response, opt)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployments: %w", err)
	}

	return response, err
}

func (ctrl *KubeRuntimeController) RestartDeployment(ctx context.Context, deployName, nameSpace string) error {
	deployment := &v1.Deployment{}
	if err := ctrl.client.Get(ctx, types.NamespacedName{Name: deployName, Namespace: nameSpace}, deployment); err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = map[string]string{}
	}
	deployment.Spec.Template.Annotations["kubectl.kubernetes.io/restartedAt"] = time.Now().Format(time.RFC3339)

	if err := ctrl.client.Update(ctx, deployment); err != nil {
		return fmt.Errorf("failed to update deployment: %w", err)
	}

	return nil
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
