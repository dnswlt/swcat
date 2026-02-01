package api

// MetadataExtensions represents supplemental metadata derived from automated processes.
// These are merged into the core entity during catalog ingestion.
type MetadataExtensions struct {
	// Annotations contains non-identifying metadata used for display or tool integration.
	Annotations map[string]any `json:"annotations,omitempty"`
}

// CatalogExtensions represents the root of a sidecar document.
// The map key should follow the canonical entity reference format.
type CatalogExtensions struct {
	// Entities maps entity references to their auto-generated extensions.
	Entities map[string]*MetadataExtensions `json:"entities"`
}

// Merge merges other into c.
// It overwrites the MetadataExtensions for any entity present in 'other'.
func (c *CatalogExtensions) Merge(other *CatalogExtensions) {
	if other == nil || len(other.Entities) == 0 {
		return
	}
	if c.Entities == nil {
		c.Entities = make(map[string]*MetadataExtensions)
	}
	for ref, otherMeta := range other.Entities {
		c.Entities[ref] = otherMeta
	}
}
