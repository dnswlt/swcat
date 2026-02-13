package kube

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// DefaultLabels are shown when no labels are explicitly configured.
var DefaultLabels = []string{
	"app",
	"app.kubernetes.io/name",
	"app.kubernetes.io/version",
}

// Config holds the configuration for connecting to a Kubernetes cluster.
type Config struct {
	// Path to the kubeconfig file.
	Kubeconfig string `yaml:"kubeconfig"`
	// Optional context to use from the kubeconfig. If empty, the current-context is used.
	Context string `yaml:"context"`
	// Namespaces to query for workloads.
	Namespaces []string `yaml:"namespaces"`
}

// LoadConfig reads a kube Config from YAML data.
func LoadConfig(data []byte) (*Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("invalid kube config YAML: %w", err)
	}
	return &cfg, nil
}

// WorkloadKind represents the type of Kubernetes workload.
type WorkloadKind string

const (
	KindDeployment  WorkloadKind = "Deployment"
	KindStatefulSet WorkloadKind = "StatefulSet"
	KindCronJob     WorkloadKind = "CronJob"
	KindDaemonSet   WorkloadKind = "DaemonSet"
	KindJob         WorkloadKind = "Job"
)

// Workload is a simplified representation of a Kubernetes workload.
type Workload struct {
	Kind        WorkloadKind
	Name        string
	Namespace   string
	Labels      map[string]string
	Annotations map[string]string
}
