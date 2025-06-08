package port

import (
	"context"
	"hack-a-tone/internal/core/domain"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type KubeController interface {
	GetAllPods(ctx context.Context, nameSpace string) (*corev1.PodList, error)
	GetDeploymentFromPod(ctx context.Context, pod *corev1.Pod) (string, error)
	GetDeployments(ctx context.Context, nameSpace string) (*v1.DeploymentList, error)
	RestartDeployment(ctx context.Context, deployName, nameSpace string) error
	RestartPod(ctx context.Context, nameSpace, podName string) error
	StatusAll(ctx context.Context) ([]domain.DeployStatus, error)
	ScalePod(ctx context.Context, deployName, nameSpace string, replicasCount int32) error
	GetAvailableRevisions(ctx context.Context, deployName, nameSpace string) ([]string, error)
	SetRevision(ctx context.Context, deployName, namespace string, revision string) error
	Start(ctx context.Context) error
}
