package port

import (
	"context"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

type KubeController interface {
	GetAllPods(ctx context.Context, nameSpace string) (*corev1.PodList, error)
	GetDeployments(ctx context.Context, nameSpace string) (*v1.DeploymentList, error)
	RestartDeployment(ctx context.Context, deployName, nameSpace string) error
	ScalePod(ctx context.Context, deployName, nameSpace string, scaleNumber int32) error
	Start(ctx context.Context) error
}
