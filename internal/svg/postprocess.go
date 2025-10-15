package svg

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

const (
	// PrefixNodeLabel* MUST start with "|" and be 2 characters long.
	PrefixNodeLabelSmall = "|."
	PrefixNodeLabelEm    = "|,"
)

// classPrefixMap defines the mapping from short prefixes to CSS class names.
var classPrefixMap = map[string]string{
	PrefixNodeLabelSmall: "node-label-small",
	PrefixNodeLabelEm:    "node-label-em",
}

// PostprocessClassPrefixes reads SVG data from a byte slice, injects class attributes into <text>
// elements based on special prefixes, and returns the transformed SVG as a new byte slice.
// It returns an error if any part of the XML processing fails.
func PostprocessClassPrefixes(svg []byte) ([]byte, error) {
	in := bytes.NewReader(svg)
	var out bytes.Buffer

	decoder := xml.NewDecoder(in)
	encoder := xml.NewEncoder(&out)

	for {
		// Read the next XML token
		token, err := decoder.Token()
		if err == io.EOF {
			break // End of input
		}
		if err != nil {
			return nil, fmt.Errorf("error decoding XML token: %w", err)
		}

		switch se := token.(type) {
		case xml.StartElement:
			// Check if the element is a <text> tag
			if se.Name.Local == "text" {
				// We found a <text> element. Now, we peek at the next token
				// to see if it's the character data we need to modify.
				nextToken, err := decoder.Token()
				if err != nil {
					if err == io.EOF { // Handle case where <text> is the last element
						if err := encoder.EncodeToken(se); err != nil {
							return nil, err
						}
						break
					}
					return nil, fmt.Errorf("error getting token after <text>: %w", err)
				}

				if charData, ok := nextToken.(xml.CharData); ok {
					// We have the text content. Let's process it.
					text := string(charData)
					newText, classVal := processText(text)

					if classVal != "" {
						// A class was found. Modify the <text> element and its content.
						se.Attr = append(se.Attr, xml.Attr{
							Name:  xml.Name{Local: "class"},
							Value: classVal,
						})
						// Write the modified <text> start tag
						if err := encoder.EncodeToken(se); err != nil {
							return nil, err
						}
						// Write the modified text content
						if err := encoder.EncodeToken(xml.CharData(newText)); err != nil {
							return nil, err
						}
						continue
					}

					// No prefix found, so write the original tokens.
					if err := encoder.EncodeToken(se); err != nil {
						return nil, err
					}
					if err := encoder.EncodeToken(charData); err != nil {
						return nil, err
					}
					continue
				}

				// The next token wasn't CharData, so write both tokens as-is.
				if err := encoder.EncodeToken(se); err != nil {
					return nil, err
				}
				if err := encoder.EncodeToken(nextToken); err != nil {
					return nil, err
				}
				continue
			}
		}

		// For all other tokens, just write them to the output.
		if err := encoder.EncodeToken(token); err != nil {
			return nil, fmt.Errorf("error encoding XML token: %w", err)
		}
	}

	// Flush the encoder to ensure all buffered data is written.
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// processText checks if a string has a recognized prefix (PrefixNodeLabel*).
// If it does, it returns the processed text (without the prefix) and the
// corresponding class name from the classMap.
// If not, it returns the original text and an empty string for the class.
func processText(text string) (newText, classVal string) {
	trimmedText := strings.TrimSpace(text)

	// Efficiently check for the "|<digit>" pattern before doing a map lookup.
	if len(trimmedText) >= 2 && trimmedText[0] == '|' {
		prefix := trimmedText[:2]
		if className, ok := classPrefixMap[prefix]; ok {
			// Prefix found in map. The new text is the part after the prefix.
			newContent := trimmedText[len(prefix):]
			// Trim leading spaces from the content that remains.
			return strings.TrimLeft(newContent, " "), className
		}
	}

	// No recognized prefix found, return the original text.
	return text, ""
}
