package port

import (
	"context"
	corev1 "k8s.io/api/core/v1"
)

type KubeController interface {
	GetAllPods(ctx context.Context) (*corev1.PodList, error)
	RestartPod()
	ScalePod(ctx context.Context, deployName, nameSpace string, scaleNumber int32) error
	Start(ctx context.Context) error
}
