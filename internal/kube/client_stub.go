//go:build !kube

package kube

import (
	"context"
	"fmt"
)

type noopClient struct{}

// NewClientFromConfig creates a Client from a ConnectConfig and a Config.
// In this stub implementation, it always returns an error indicating that Kubernetes support is disabled.
func NewClientFromConfig(cc ConnectConfig, cfg Config) (Client, error) {
	return nil, fmt.Errorf("kubernetes support is disabled in this build")
}

func (c *noopClient) AllWorkloads(ctx context.Context) ([]Workload, error) {
	return nil, nil
}

func (c *noopClient) Workloads(ctx context.Context, namespace string) ([]Workload, error) {
	return nil, nil
}
