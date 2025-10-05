package testutil

import (
	"bytes"
	"encoding/xml"
	"io"
	"sort"
	"strings"
)

// ExtractSVGIDs returns all id="" attribute values (entity-decoded).
func ExtractSVGIDs(svg []byte) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(svg))
	var ids []string
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			for _, a := range se.Attr {
				if a.Name.Local == "id" {
					ids = append(ids, a.Value)
				}
			}
		}
	}
	return ids, nil
}

// ExtractSVGClasses returns a deduped, sorted list of all CSS classes used.
func ExtractSVGClasses(svg []byte) ([]string, error) {
	dec := xml.NewDecoder(bytes.NewReader(svg))
	set := make(map[string]struct{})
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			for _, a := range se.Attr {
				if a.Name.Local == "class" {
					for _, c := range strings.Fields(a.Value) {
						if c != "" {
							set[c] = struct{}{}
						}
					}
				}
			}
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out, nil
}
