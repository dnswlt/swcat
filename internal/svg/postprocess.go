package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
)

// PostprocessSVG transforms the SVG output by graphviz by filtering out all
// <title> elements, which browsers render as native tooltips. We render rich
// custom tooltips in the frontend instead.
func PostprocessSVG(svg []byte) ([]byte, error) {
	in := bytes.NewReader(svg)
	var out bytes.Buffer

	decoder := xml.NewDecoder(in)
	encoder := xml.NewEncoder(&out)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error decoding XML token: %w", err)
		}

		if se, ok := token.(xml.StartElement); ok && se.Name.Local == "title" {
			// Skip tokens until we find the corresponding </title> end element.
			for {
				nextToken, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					return nil, err
				}
				if ee, ok := nextToken.(xml.EndElement); ok && ee.Name.Local == "title" {
					break
				}
			}
			continue
		}

		if err := encoder.EncodeToken(token); err != nil {
			return nil, fmt.Errorf("error encoding XML token: %w", err)
		}
	}

	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
