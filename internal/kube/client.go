package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Client queries a Kubernetes cluster for workloads.
type Client struct {
	clientset kubernetes.Interface
	config    Config
}

// NewClientFromConfig creates a Client from a Config.
func NewClientFromConfig(cfg Config) (*Client, error) {
	var restConfig *rest.Config
	var err error
	if cfg.Kubeconfig != "" {
		rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: cfg.Kubeconfig}
		overrides := &clientcmd.ConfigOverrides{}
		if cfg.Context != "" {
			overrides.CurrentContext = cfg.Context
		}
		restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("load kubeconfig %q: %w", cfg.Kubeconfig, err)
		}
	} else {
		// Standard "Staff" move: use the service account token mounted in the pod
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config: %w", err)
		}
	}

	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return &Client{clientset: cs, config: cfg}, nil
}

// Workloads returns all workloads in all configured namespaces.
func (c *Client) AllWorkloads(ctx context.Context) ([]Workload, error) {
	var allWorkloads []Workload
	for _, ns := range c.config.Namespaces {
		workloads, err := c.Workloads(ctx, ns)
		if err != nil {
			return nil, fmt.Errorf("get workloads for namespace %s: %w", ns, err)
		}
		allWorkloads = append(allWorkloads, workloads...)
	}
	return allWorkloads, nil
}

// Workloads returns all workloads in the given namespace.
func (c *Client) Workloads(ctx context.Context, namespace string) ([]Workload, error) {
	var result []Workload

	// Deployments
	deployments, err := c.clientset.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list deployments: %w", err)
	}
	for _, d := range deployments.Items {
		result = append(result, Workload{
			Kind:        KindDeployment,
			Name:        d.Name,
			Namespace:   d.Namespace,
			Labels:      d.Labels,
			Annotations: d.Annotations,
		})
	}

	// StatefulSets
	statefulSets, err := c.clientset.AppsV1().StatefulSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list statefulsets: %w", err)
	}
	for _, s := range statefulSets.Items {
		result = append(result, Workload{
			Kind:        KindStatefulSet,
			Name:        s.Name,
			Namespace:   s.Namespace,
			Labels:      s.Labels,
			Annotations: s.Annotations,
		})
	}

	// DaemonSets
	daemonSets, err := c.clientset.AppsV1().DaemonSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list daemonsets: %w", err)
	}
	for _, d := range daemonSets.Items {
		result = append(result, Workload{
			Kind:        KindDaemonSet,
			Name:        d.Name,
			Namespace:   d.Namespace,
			Labels:      d.Labels,
			Annotations: d.Annotations,
		})
	}

	// CronJobs
	cronJobs, err := c.clientset.BatchV1().CronJobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list cronjobs: %w", err)
	}
	for _, j := range cronJobs.Items {
		result = append(result, Workload{
			Kind:        KindCronJob,
			Name:        j.Name,
			Namespace:   j.Namespace,
			Labels:      j.Labels,
			Annotations: j.Annotations,
		})
	}

	// Jobs
	jobs, err := c.clientset.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list jobs: %w", err)
	}
	for _, j := range jobs.Items {
		result = append(result, Workload{
			Kind:        KindJob,
			Name:        j.Name,
			Namespace:   j.Namespace,
			Labels:      j.Labels,
			Annotations: j.Annotations,
		})
	}

	return result, nil
}
