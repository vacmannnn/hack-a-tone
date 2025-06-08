package utils

import (
	"fmt"
	"hack-a-tone/internal/core/domain"
	v1 "k8s.io/api/apps/v1"
	"strings"
)

func GetReplicasCountForDeployment(deployment *v1.Deployment) int32 {
	return *deployment.Spec.Replicas
}

func PrettyPrintStatus(deploys []domain.DeployStatus) string {
	var sb strings.Builder

	for i, deploy := range deploys {
		sb.WriteString(fmt.Sprintf("Deployment #%d - Status: %s\n", i+1, deploy.Status))
		if len(deploy.Pods) == 0 {
			sb.WriteString("\tNo pods found\n")
			continue
		}

		for podName, pod := range deploy.Pods {
			sb.WriteString(fmt.Sprintf("\tPod: %s\n", podName))
			sb.WriteString(fmt.Sprintf("\t\tTotal CPU: %.3f cores\n", pod.TotalCPU))
			sb.WriteString(fmt.Sprintf("\t\tTotal Memory: %.3f MB\n", pod.TotalMem))

			if len(pod.Containers) == 0 {
				sb.WriteString("\t\tNo containers found\n")
				continue
			}

			for containerName, container := range pod.Containers {
				sb.WriteString(fmt.Sprintf("\t\tContainer: %s\n", containerName))
				sb.WriteString(fmt.Sprintf("\t\t\tCPU: %.3f cores\n", container.CPU))
				sb.WriteString(fmt.Sprintf("\t\t\tMemory: %.3f MB\n", container.Memory))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
