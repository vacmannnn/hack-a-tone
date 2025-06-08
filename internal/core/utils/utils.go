package utils

import (
	v1 "k8s.io/api/apps/v1"
)

func GetReplicasCountForDeployment(deployment *v1.Deployment) int32 {
	return *deployment.Spec.Replicas
}
