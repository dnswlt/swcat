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

// CustomContent represents content that is displayed in the detail view
// and is specified in an entity annotation.
type CustomContent struct {
	Heading string
	Text    string   // Text to be presented as-is.
	Items   []string // Items to be rendered as an <ul> list.
	Attrs   []ccAttr // Items to be rendered as a key-value <table>.
	Table   *ccTable // Items to be rendered in a custom <table>.
	Code    string   // Preformatted code (typically JSON)
	Meta    []ccAttr // Optional metadata fields rendered at the bottom (from $meta wrapper).
	Rank    int      // Used to order multiple custom content items.
	Open    bool     // If true, the custom content will be open (expanded).
}

func customContentForEntity(entity catalog.Entity, cfg *config.UIConfig) ([]*CustomContent, error) {
	var result []*CustomContent
	// Annotations
	if len(entity.GetMetadata().Annotations) > 0 && len(cfg.AnnotationBasedContent) > 0 {
		meta := entity.GetMetadata()
		for k, abc := range cfg.AnnotationBasedContent {
			anno, ok := meta.Annotations[k]
			if !ok {
				continue
			}
			cc, err := newCustomContent(&abc.CustomContent, anno)
			if err != nil {
				return nil, fmt.Errorf("invalid custom content: %v", err)
			}
			cc.Rank = abc.Rank
			result = append(result, cc)
		}
	}
	// Status
	if status := entity.GetStatus(); status != nil && len(status.Observations) > 0 && len(cfg.StatusBasedContent) > 0 {
		for k, sbc := range cfg.StatusBasedContent {
			obs, ok := status.Observations[k]
			if !ok {
				continue
			}
			cc, err := newCustomContentObservation(&sbc.CustomContent, obs)
			if err != nil {
				return nil, fmt.Errorf("invalid custom content: %v", err)
			}
			cc.Rank = sbc.Rank
			result = append(result, cc)
		}
	}

	slices.SortFunc(result, func(a, b *CustomContent) int {
		if c := cmp.Compare(a.Rank, b.Rank); c != 0 {
			return c
		}
		return cmp.Compare(a.Heading, b.Heading)
	})

	return result, nil
}

func newCustomContentObservation(ccc *config.CustomContent, obs catalog.Observation) (*CustomContent, error) {
	cc, err := newCustomContent(ccc, string(obs.Value))
	if err != nil {
		return nil, err
	}
	// Add metadata
	cc.Meta = append(cc.Meta, ccAttr{Name: "updatedAt", Value: obs.UpdatedAt.Format(time.RFC3339)})
	if obs.Version != "" {
		cc.Meta = append(cc.Meta, ccAttr{Name: "version", Value: obs.Version})
	}
	return cc, nil
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
		Open:    ccc.Open,
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
