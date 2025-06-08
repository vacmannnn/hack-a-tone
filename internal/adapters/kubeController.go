package adapters

import (
	"context"
	"fmt"
	"hack-a-tone/internal/core/domain"
	"hack-a-tone/internal/core/port"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/client/clientset/versioned"
	"log/slog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"time"
)

const TlsOFF = true

type KubeRuntimeController struct {
	client       client.Client
	metricClient *versioned.Clientset
	mgr          manager.Manager
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

	if scaleNumber < 0 {
		return fmt.Errorf("count replicas less than zero")
	}

	*deployment.Spec.Replicas = scaleNumber

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

	c, err := versioned.NewForConfig(cfg)
	if err != nil {
		slog.Error("Не удалось создать client", "error", err)
		return err
	}

	ctrl.metricClient = c
	ctrl.mgr = mgr

	return err
}

func (ctrl *KubeRuntimeController) GetAvailableRevisions(ctx context.Context, deployName, nameSpace string) ([]string, error) {
	var deployment v1.Deployment
	err := ctrl.client.Get(ctx, client.ObjectKey{
		Namespace: nameSpace,
		Name:      deployName,
	}, &deployment)
	if err != nil {
		slog.Error("Failed to get deployment", "name", deployName, "namespace", nameSpace, "error", err)
		return nil, fmt.Errorf("failed to get deployment %s: %w", deployName, err)
	}

	var replicaSetList v1.ReplicaSetList
	err = ctrl.client.List(ctx, &replicaSetList, &client.ListOptions{
		Namespace: nameSpace,
	})
	if err != nil {
		slog.Error("Failed to list replicasets", "namespace", nameSpace, "error", err)
		return nil, fmt.Errorf("failed to list replicasets in namespace %s: %w", nameSpace, err)
	}

	revisions := make([]string, 0)
	for _, rs := range replicaSetList.Items {
		for _, owner := range rs.OwnerReferences {
			if owner.Kind == "Deployment" && owner.Name == deployName {
				revision, ok := rs.Annotations["deployment.kubernetes.io/revision"]
				if ok {
					revisions = append(revisions, revision)
				}
			}
		}
	}

	return revisions, nil
}
func (ctrl *KubeRuntimeController) SetRevision(ctx context.Context, deployName, namespace string, revision string) error {
	var deployment v1.Deployment
	err := ctrl.client.Get(ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      deployName,
	}, &deployment)
	if err != nil {
		slog.Error("Failed to get deployment", "name", deployName, "namespace", namespace, "error", err)
		return fmt.Errorf("failed to get deployment %s: %w", deployName, err)
	}

	var rsList v1.ReplicaSetList
	err = ctrl.client.List(ctx, &rsList, &client.ListOptions{
		Namespace: namespace,
	})
	if err != nil {
		slog.Error("Failed to list replicasets", "namespace", namespace, "error", err)
		return fmt.Errorf("failed to list replicasets in namespace %s: %w", namespace, err)
	}

	var targetRS *v1.ReplicaSet
	for _, rs := range rsList.Items {
		for _, owner := range rs.OwnerReferences {
			if owner.Kind == "Deployment" && owner.Name == deployName {
				if rs.Annotations["deployment.kubernetes.io/revision"] == revision {
					targetRS = rs.DeepCopy()
					break
				}
			}
		}
		if targetRS != nil {
			break
		}
	}

	if targetRS == nil {
		slog.Warn("No ReplicaSet found with the specified revision", "revision", revision)
		return fmt.Errorf("no ReplicaSet found with revision %s", revision)
	}

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var updatedDeployment v1.Deployment
		err = ctrl.client.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      deployName,
		}, &updatedDeployment)
		if err != nil {
			return fmt.Errorf("failed to get latest deployment %s: %w", deployName, err)
		}
		updatedDeployment.Spec.Template = targetRS.Spec.Template
		if updatedDeployment.Annotations == nil {
			updatedDeployment.Annotations = make(map[string]string)
		}
		updatedDeployment.Annotations["kubernetes.io/change-cause"] = fmt.Sprintf("Rollback to revision %s", revision)
		if err = ctrl.client.Update(ctx, &updatedDeployment); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		slog.Error("Failed to set revision", "revision", revision, "error", err)
		return fmt.Errorf("failed to set revision %s: %w", revision, err)
	}
	slog.Info("Successfully set revision", "revision", revision, "deployment", deployName)

	return nil
}

func (ctrl *KubeRuntimeController) RestartPod(ctx context.Context, nameSpace, podName string) error {
	var pod *corev1.Pod
	pod.Namespace = nameSpace
	pod.Name = podName

	if err := ctrl.client.Delete(ctx, pod); err != nil {
		return fmt.Errorf("failed to delete pod %s/%s: %w", nameSpace, podName, err)
	}

	return nil
}

func (ctrl *KubeRuntimeController) StatusAll(ctx context.Context) ([]domain.DeployStatus, error) {
	var deployments v1.DeploymentList
	if err := ctrl.client.List(ctx, &deployments); err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	var result []domain.DeployStatus

	for _, deploy := range deployments.Items {
		selector := client.MatchingLabels(deploy.Spec.Selector.MatchLabels)
		var podList corev1.PodList
		if err := ctrl.client.List(ctx, &podList, selector); err != nil {
			return nil, fmt.Errorf("failed to list pods for deployment %s: %w", deploy.Name, err)
		}

		pods := make(map[string]domain.PodStatus)

		deployStatus := "Unknown"
		if len(podList.Items) > 0 {
			deployStatus = string(podList.Items[0].Status.Phase)
		}

		for _, pod := range podList.Items {
			containers := make(map[string]domain.ContainerStatus)
			var totalCPU, totalMem float64

			for _, cs := range pod.Status.ContainerStatuses {
				cpuUsage, memUsage, err := ctrl.getContainerResourceUsage(ctx, cs.Name, pod.Name, pod.Namespace)
				if err != nil {
					slog.Error("Failed to get resource usage", "container", cs.Name, "pod", pod.Name, "namespace", pod.Namespace, "error", err)
				}

				containers[cs.Name] = domain.ContainerStatus{
					CPU:    cpuUsage,
					Memory: memUsage,
				}
				totalCPU += cpuUsage
				totalMem += memUsage
			}

			pods[pod.Name] = domain.PodStatus{
				Containers: containers,
				TotalCPU:   totalCPU,
				TotalMem:   totalMem,
			}
		}

		result = append(result, domain.DeployStatus{
			Status: deployStatus,
			Pods:   pods,
		})
	}

	return result, nil
}

func (ctrl *KubeRuntimeController) getContainerResourceUsage(ctx context.Context, containerName, podName, namespace string) (cpu float64, mem float64, err error) {
	podMetrics, err := ctrl.metricClient.MetricsV1beta1().PodMetricses(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return 0, 0, err
	}

	// Ищем нужный контейнер в метриках
	for _, c := range podMetrics.Containers {
		if c.Name == containerName {
			// CPU в наносекундах (обычно в формате Quantity)
			cpuQuantity := c.Usage.Cpu()
			memQuantity := c.Usage.Memory()

			// Преобразуем CPU в float64 — количество ядер (например, 0.1 = 100m)
			cpu = float64(cpuQuantity.MilliValue()) / 1000.0

			// Преобразуем память в мегабайты
			mem = float64(memQuantity.Value()) / (1024 * 1024)

			return cpu, mem, nil
		}
	}
	// Контейнер не найден в метриках
	return 0, 0, fmt.Errorf("container %s not found in pod metrics %s/%s", containerName, namespace, podName)
}
