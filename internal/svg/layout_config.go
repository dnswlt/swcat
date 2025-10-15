package svg

type LayoutConfig struct {
	// Labels whose values should be displayed as <<stereotypes>> in node labels.
	StereotypeLabels []string `yaml:"stereotypeLabels"`
	// Maps label keys and label values to node colors.
	// Can be used to override the default node colors per label value.
	NodeColorsByLabel map[string]map[string]string `yaml:"nodeColorsByLabel"`
}
