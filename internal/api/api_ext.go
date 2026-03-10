package api

import "time"

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

// LintFinding can be used as a JSON-as-string annotation value (e.g. by plugins)
// to store an indication that an entity has issues.
// The lintAnnotation custom lint rule can interpret these.
type LintFinding struct {
	CreateTime time.Time `json:"createTime"`
	Message    string    `json:"message"`
}

func NewCatalogExtensions() *CatalogExtensions {
	return &CatalogExtensions{
		Entities: make(map[string]*MetadataExtensions),
	}
}

func (c *CatalogExtensions) Get(ref string) *MetadataExtensions {
	if c.Entities == nil {
		return nil
	}
	return c.Entities[ref]
}

// Merge merges other into c at the annotation key level.
// A nil annotation value deletes the key; absent keys are preserved.
func (c *CatalogExtensions) Merge(other *CatalogExtensions) {
	if other == nil || len(other.Entities) == 0 {
		return
	}
	if c.Entities == nil {
		c.Entities = make(map[string]*MetadataExtensions)
	}
	for ref, otherMeta := range other.Entities {
		existing := c.Entities[ref]
		if existing == nil {
			existing = &MetadataExtensions{}
			c.Entities[ref] = existing
		}
		if existing.Annotations == nil {
			existing.Annotations = make(map[string]any)
		}
		for k, v := range otherMeta.Annotations {
			if v == nil {
				delete(existing.Annotations, k)
			} else {
				existing.Annotations[k] = v
			}
		}
	}
}

// WrapAnnotation wraps the given annotation value in a $data and $meta
// envelope
func WrapAnnotation(value any, updateTime time.Time) map[string]any {
	return map[string]any{
		"$data": value,
		"$meta": map[string]string{
			"updateTime": updateTime.Format("2006-01-02 15:04:05"),
		},
	}
}

func UnwrapAnnotation(value any) (data any, meta map[string]any, found bool) {
	m, ok := value.(map[string]any)
	if !ok {
		return value, nil, false
	}
	metaRaw, hasMeta := m["$meta"]
	dataRaw, hasValue := m["$data"]
	if !hasMeta || !hasValue || len(m) != 2 {
		return value, nil, false
	}
	metaMap, ok := metaRaw.(map[string]any)
	if !ok {
		return value, nil, false
	}
	return dataRaw, metaMap, true
}
