package web

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/dnswlt/swcat/internal/catalog"
	"github.com/dnswlt/swcat/internal/config"
	"github.com/tidwall/gjson"
)

type ccAttr struct {
	Name  string
	Value string
}

type ccRow struct {
	Columns []string
}

type ccTable struct {
	Headers []string
	Rows    []*ccRow
}

// CustomContentGroup is one <details> section in the entity detail view.
// It contains one or more rendered Blocks plus optional Meta footer.
type CustomContentGroup struct {
	Heading string
	Open    bool
	Rank    int
	Blocks  []*CustomContent
	Meta    []ccAttr // Optional metadata fields rendered at the bottom of the section.
}

// CustomContent is one rendered block within a CustomContentGroup.
type CustomContent struct {
	Heading string   // Optional sub-heading rendered above the block.
	Text    string   // Text to be presented as-is.
	Items   []string // Items to be rendered as an <ul> list.
	Attrs   []ccAttr // Items to be rendered as a key-value <table>.
	Table   *ccTable // Items to be rendered in a custom <table>.
	Code    string   // Preformatted code (typically JSON)
}

func customContentForEntity(entity catalog.Entity, cfg *config.UIConfig) ([]*CustomContentGroup, error) {
	var result []*CustomContentGroup
	// Annotations
	if len(entity.GetMetadata().Annotations) > 0 && len(cfg.AnnotationBasedContent) > 0 {
		meta := entity.GetMetadata()
		for k, abc := range cfg.AnnotationBasedContent {
			anno, ok := meta.Annotations[k]
			if !ok {
				continue
			}
			g, err := newCustomContentGroup(abc, anno)
			if err != nil {
				return nil, fmt.Errorf("invalid custom content: %v", err)
			}
			result = append(result, g)
		}
	}
	// Status
	if status := entity.GetStatus(); status != nil && len(status.Observations) > 0 && len(cfg.StatusBasedContent) > 0 {
		for k, sbc := range cfg.StatusBasedContent {
			obs, ok := status.Observations[k]
			if !ok {
				continue
			}
			g, err := newCustomContentGroupObservation(sbc, obs)
			if err != nil {
				return nil, fmt.Errorf("invalid custom content: %v", err)
			}
			result = append(result, g)
		}
	}

	slices.SortFunc(result, func(a, b *CustomContentGroup) int {
		if c := cmp.Compare(a.Rank, b.Rank); c != 0 {
			return c
		}
		return cmp.Compare(a.Heading, b.Heading)
	})

	return result, nil
}

func newCustomContentGroup(s *config.CustomContentSection, value string) (*CustomContentGroup, error) {
	g := &CustomContentGroup{
		Heading: s.Heading,
		Open:    s.Open,
		Rank:    s.Rank,
	}
	blocks, err := buildBlocks(&s.CustomContent, s.Blocks, value)
	if err != nil {
		return nil, err
	}
	g.Blocks = blocks
	return g, nil
}

func newCustomContentGroupObservation(s *config.CustomContentSection, obs catalog.Observation) (*CustomContentGroup, error) {
	g, err := newCustomContentGroup(s, string(obs.Value))
	if err != nil {
		return nil, err
	}
	g.Meta = append(g.Meta, ccAttr{Name: "updatedAt", Value: obs.UpdatedAt.Format(time.RFC3339)})
	if obs.Version != "" {
		g.Meta = append(g.Meta, ccAttr{Name: "version", Value: obs.Version})
	}
	return g, nil
}

// buildBlocks renders the configured Blocks for a group. If Blocks is empty,
// the inline CustomContent is rendered as a single block (its Heading is
// cleared because at the inline level it represents the group heading, not
// a block sub-heading).
func buildBlocks(inline *config.CustomContent, blocks []*config.CustomContent, value string) ([]*CustomContent, error) {
	if len(blocks) == 0 {
		cc, err := newCustomContent(inline, value)
		if err != nil {
			return nil, err
		}
		cc.Heading = ""
		return []*CustomContent{cc}, nil
	}
	out := make([]*CustomContent, 0, len(blocks))
	for _, b := range blocks {
		cc, err := newCustomContent(b, value)
		if err != nil {
			return nil, err
		}
		out = append(out, cc)
	}
	return out, nil
}

func setCCText(cc *CustomContent, value string) {
	var v any
	if err := json.Unmarshal([]byte(value), &v); err != nil {
		cc.Text = value // not valid JSON; use value as-is.
		return
	}
	cc.Text = formatJSONValue(v)
}

// formatJSONValue renders a JSON-decoded value as a display string: strings
// are used unquoted, everything else (numbers, bools, objects, arrays, null)
// is rendered as JSON so nested structures don't come out as Go's map/slice
// default format.
func formatJSONValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func setCCList(cc *CustomContent, value string) error {
	var items []any
	if err := json.Unmarshal([]byte(value), &items); err != nil {
		return fmt.Errorf("list style requires a JSON array: %v", err)
	}
	cc.Items = make([]string, len(items))
	for i, v := range items {
		cc.Items[i] = formatJSONValue(v)
	}
	return nil
}

func setCCTable(cc *CustomContent, columns []*config.CustomColumn, value string) error {
	var rows []map[string]any
	if err := json.Unmarshal([]byte(value), &rows); err != nil {
		return fmt.Errorf("table style requires a JSON array of objects: %v", err)
	}
	t := &ccTable{
		Headers: make([]string, len(columns)),
		Rows:    make([]*ccRow, len(rows)),
	}
	hasHeaders := false
	for i, c := range columns {
		t.Headers[i] = c.Header
		if c.Header != "" {
			hasHeaders = true
		}
	}
	if !hasHeaders {
		t.Headers = nil
	}
	for i, m := range rows {
		r := &ccRow{
			Columns: make([]string, len(columns)),
		}
		for j, c := range columns {
			var buf bytes.Buffer
			if err := c.DataTemplate().Execute(&buf, m); err != nil {
				r.Columns[j] = fmt.Sprintf("template error: %v", err)
			} else {
				r.Columns[j] = buf.String()
			}
		}
		t.Rows[i] = r
	}
	cc.Table = t
	return nil
}

func setCCCode(cc *CustomContent, value string) error {
	var parsed any
	if err := json.Unmarshal([]byte(value), &parsed); err != nil {
		return fmt.Errorf("json style requires valid JSON: %v", err)
	}
	// json.MarshalIndent can't fail for values produced by json.Unmarshal.
	indentedJSON, _ := json.MarshalIndent(parsed, "", "  ")
	cc.Code = string(indentedJSON)
	return nil
}

func setCCAttrs(cc *CustomContent, fields []string, value string) error {
	if len(fields) == 0 {
		// Use all fields
		var parsed map[string]any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return fmt.Errorf("attrs style requires a JSON object: %v", err)
		}
		keys := slices.Sorted(maps.Keys(parsed))
		for _, k := range keys {
			cc.Attrs = append(cc.Attrs, ccAttr{
				Name:  k,
				Value: formatJSONValue(parsed[k]),
			})
		}
		return nil
	}

	if !gjson.Valid(value) {
		return fmt.Errorf("attrs style requires valid JSON: %v", value)
	}
	for _, k := range fields {
		r := gjson.Get(value, k)
		if r.Exists() {
			cc.Attrs = append(cc.Attrs, ccAttr{
				Name:  k,
				Value: r.String(),
			})
		}
	}
	return nil
}

func newCustomContent(ccc *config.CustomContent, value string) (*CustomContent, error) {
	cc := &CustomContent{
		Heading: ccc.Heading,
	}
	if ccc.Selector != "" {
		res := gjson.Get(value, ccc.Selector)
		if !res.Exists() {
			return nil, fmt.Errorf("selector %q matched nothing", ccc.Selector)
		}
		value = res.Raw
	}

	switch ccc.Style {
	case "text", "":
		setCCText(cc, value)
	case "list":
		if err := setCCList(cc, value); err != nil {
			return nil, err
		}
	case "attrs":
		if err := setCCAttrs(cc, ccc.Fields, value); err != nil {
			return nil, err
		}
	case "table":
		if err := setCCTable(cc, ccc.Columns, value); err != nil {
			return nil, err
		}
	case "json":
		if err := setCCCode(cc, value); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid custom content style (must be text|list|attrs|table|json): %s", ccc.Style)
	}
	return cc, nil
}
