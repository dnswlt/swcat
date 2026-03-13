//go:build kube

package kube

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// k8sClient queries a Kubernetes cluster for workloads.
type k8sClient struct {
	clientset kubernetes.Interface
	config    Config
}

// NewClientFromConfig creates a Client from a ConnectConfig and a Config.
func NewClientFromConfig(cc ConnectConfig, cfg Config) (Client, error) {
	var restConfig *rest.Config
	var err error
	if cc.Kubeconfig != "" {
		rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: cc.Kubeconfig}
		overrides := &clientcmd.ConfigOverrides{}
		if cc.Context != "" {
			overrides.CurrentContext = cc.Context
		}
		restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
		if err != nil {
			return nil, fmt.Errorf("load kubeconfig %q: %w", cc.Kubeconfig, err)
		}
	} else if cc.InCluster {
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("in-cluster config: %w", err)
		}
	} else {
		return nil, fmt.Errorf("no kubeconfig path or in-cluster mode specified")
	}

	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	return &k8sClient{clientset: cs, config: cfg}, nil
}

// AllWorkloads returns all workloads from all configured namespaces.
func (c *k8sClient) AllWorkloads(ctx context.Context) ([]Workload, error) {
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
func (c *k8sClient) Workloads(ctx context.Context, namespace string) ([]Workload, error) {
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

	return result, nil
}
