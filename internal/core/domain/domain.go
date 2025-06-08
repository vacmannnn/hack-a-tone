package domain

type ContainerStatus struct {
	CPU    float64
	Memory float64
}

type PodStatus struct {
	Containers map[string]ContainerStatus
	TotalCPU   float64
	TotalMem   float64
}

type DeployStatus struct {
	Status string
	Pods   map[string]PodStatus
}
