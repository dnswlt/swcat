package prometheus

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type DisplayLabel struct {
	Key   string `yaml:"key"`
	Label string `yaml:"label"`
}

type Config struct {
	URL            string         `yaml:"url"`
	WorkloadsQuery string         `yaml:"workloadsQuery"`
	NameLabel      string         `yaml:"nameLabel"`
	DisplayLabels  []DisplayLabel `yaml:"displayLabels"`
	ShowMetrics    bool           `yaml:"showMetrics"`
}

// ParseConfig reads a prometheus Config from YAML data.
func ParseConfig(data []byte) (*Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("invalid prometheus config YAML: %w", err)
	}

	if cfg.URL == "" || strings.TrimSpace(cfg.WorkloadsQuery) == "" || cfg.NameLabel == "" {
		return nil, fmt.Errorf("prometheus config is missing required fields [url, workloadsQuery, nameLabel]")
	}
	return &cfg, nil
}

type WorkloadScanner struct {
	client *Client
	cfg    Config
}

// Workload represents a single workload found in Prometheus.
type Workload struct {
	Name        string
	LabelValues map[string]string // Maps DisplayLabel.Key to its value from the metric.
	Value       float64           // Only populated if ShowMetrics is true.
}

// WorkloadResult contains the list of workloads and configuration for rendering.
type WorkloadResult struct {
	Workloads     []Workload
	DisplayLabels []DisplayLabel
	ShowMetrics   bool
}

// NewWorkloadScanner creates a new scanner for Prometheus workloads.
func NewWorkloadScanner(opts ClientOptions, cfg Config) *WorkloadScanner {
	return &WorkloadScanner{
		client: NewClient(cfg.URL, opts),
		cfg:    cfg,
	}
}

// ScanWorkloads runs the configured query and returns the results for rendering.
func (s *WorkloadScanner) ScanWorkloads(ctx context.Context) (*WorkloadResult, error) {
	samples, err := s.client.Query(ctx, s.cfg.WorkloadsQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query prometheus: %w", err)
	}

	var workloads []Workload
	for _, sample := range samples {
		w := Workload{
			Name:        sample.Labels[s.cfg.NameLabel],
			LabelValues: make(map[string]string),
			Value:       sample.Value,
		}

		for _, dl := range s.cfg.DisplayLabels {
			if val, ok := sample.Labels[dl.Key]; ok {
				w.LabelValues[dl.Key] = val
			}
		}
		workloads = append(workloads, w)
	}

	return &WorkloadResult{
		Workloads:     workloads,
		DisplayLabels: s.cfg.DisplayLabels,
		ShowMetrics:   s.cfg.ShowMetrics,
	}, nil
}
