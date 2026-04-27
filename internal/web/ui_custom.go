package web

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"html/template"
	"maps"
	"slices"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/config"
)

type ccAttr struct {
	Name  string
	Value string
}

// CustomContent is one rendered <details> section on an entity detail page.
// At most one of HTML, Code, and Err is set:
//   - HTML when a user template was rendered;
//   - Code when the value was rendered as raw pretty-printed JSON;
//   - Err when rendering failed for this section (the section is shown
//     with an error banner instead of content).
type CustomContent struct {
	Heading string
	Open    bool
	Rank    int
	HTML    template.HTML // user-template output (already safe HTML)
	Code    string        // pretty-printed JSON shown in the read-only viewer
	Meta    []ccAttr      // optional footer attributes (status observations only)
	Err     string        // non-empty when rendering this section failed
}

// customContentForEntity collects every configured section that applies
// to entity. A section that fails to render (invalid JSON, template
// error, …) is included with its Err set, so the rest of the page still
// renders — and the user is told which section is broken.
func customContentForEntity(entity catalog.Entity, cfg *config.UIConfig) []*CustomContent {
	var result []*CustomContent
	if len(entity.GetMetadata().Annotations) > 0 && len(cfg.AnnotationBasedContent) > 0 {
		annotations := entity.GetMetadata().Annotations
		for k, c := range cfg.AnnotationBasedContent {
			anno, ok := annotations[k]
			if !ok {
				continue
			}
			cc, err := newCustomContent(c, anno)
			if err != nil {
				cc = errorCustomContent(c, err)
			}
			result = append(result, cc)
		}
	}
	if status := entity.GetStatus(); status != nil && len(status.Observations) > 0 && len(cfg.StatusBasedContent) > 0 {
		for k, c := range cfg.StatusBasedContent {
			obs, ok := status.Observations[k]
			if !ok {
				continue
			}
			cc, err := newCustomContentObservation(c, obs)
			if err != nil {
				cc = errorCustomContent(c, err)
			}
			result = append(result, cc)
		}
	}

	slices.SortFunc(result, func(a, b *CustomContent) int {
		if c := cmp.Compare(a.Rank, b.Rank); c != 0 {
			return c
		}
		return cmp.Compare(a.Heading, b.Heading)
	})

	return result
}

// errorCustomContent builds a placeholder CustomContent that records a
// rendering failure. The section is force-opened so the user notices the
// error without having to click.
func errorCustomContent(c *config.CustomContent, err error) *CustomContent {
	return &CustomContent{
		Heading: c.Heading,
		Open:    true,
		Rank:    c.Rank,
		Err:     err.Error(),
	}
}

func newCustomContent(c *config.CustomContent, value string) (*CustomContent, error) {
	cc := &CustomContent{
		Heading: c.Heading,
		Open:    c.Open,
		Rank:    c.Rank,
	}
	if c.Tmpl() == nil {
		// Raw JSON view: validate and pretty-print.
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, fmt.Errorf("value is not valid JSON: %v", err)
		}
		// json.MarshalIndent can't fail for values produced by json.Unmarshal.
		indented, _ := json.MarshalIndent(parsed, "", "  ")
		cc.Code = string(indented)
		return cc, nil
	}
	// Template view: parse JSON if possible, otherwise pass the raw string.
	var data any
	if err := json.Unmarshal([]byte(value), &data); err != nil {
		data = value
	}
	var buf bytes.Buffer
	if err := c.Tmpl().Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("template execution failed: %v", err)
	}
	cc.HTML = template.HTML(buf.String())
	return cc, nil
}

func newCustomContentObservation(c *config.CustomContent, obs catalog.Observation) (*CustomContent, error) {
	cc, err := newCustomContent(c, string(obs.Value))
	if err != nil {
		return nil, err
	}
	cc.Meta = append(cc.Meta, ccAttr{Name: "updatedAt", Value: obs.UpdatedAt.Format(time.RFC3339)})
	if obs.Version != "" {
		cc.Meta = append(cc.Meta, ccAttr{Name: "version", Value: obs.Version})
	}
	for _, k := range slices.Sorted(maps.Keys(obs.Meta)) {
		cc.Meta = append(cc.Meta, ccAttr{Name: k, Value: obs.Meta[k]})
	}
	return cc, nil
}
