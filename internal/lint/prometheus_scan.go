package lint

import (
	"context"
	"fmt"
	"slices"

	"github.com/dnswlt/swcat/internal/prometheus"
)

// PromWorkload represents a single workload found in Prometheus.
type PromWorkload struct {
	Name        string
	LabelValues map[string]string // Maps DisplayLabel.Key to its value from the metric.
	Value       float64           // Only populated if ShowMetrics is true.
}

// ScanWorkloads runs the configured query and returns the results for rendering.
func (l *Linter) ScanPrometheusWorkloads(ctx context.Context, client *prometheus.Client) ([]PromWorkload, error) {
	samples, err := client.Query(ctx, l.config.Prometheus.WorkloadsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query prometheus: %w", err)
	}

	var workloads []PromWorkload
	for _, sample := range samples {
		name := sample.Labels[l.config.Prometheus.WorkloadNameLabel]

		if slices.Contains(l.config.Prometheus.ExcludedWorkloads, name) {
			continue
		}

		w := PromWorkload{
			Name:        name,
			LabelValues: make(map[string]string),
			Value:       sample.Value,
		}

		for _, dl := range l.config.Prometheus.DisplayLabels {
			if val, ok := sample.Labels[dl.Key]; ok {
				w.LabelValues[dl.Key] = val
			}
		}
		workloads = append(workloads, w)
	}

	return workloads, nil
}
