package svg

import "strings"

// joinWrap joins items into a single string, separated by sep.
// It inserts newlines after each item that reaches or exceeds
// the specified limit (# of characters) of the current line.
// Individual items always appear on one line and are never wrapped.
func joinWrap(items []string, sep string, limit int) string {
	if len(items) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(items[0])

	currentLineLen := len(items[0])
	sepLen := len(sep)

	for _, item := range items[1:] {
		itemLen := len(item)

		// We wrap if:
		// 1. The line is not empty (we don't wrap an already empty line).
		// 2. Adding the separator and the item would exceed the limit.
		if currentLineLen > 0 && (currentLineLen+sepLen+itemLen) > limit {
			// Wrap to a new line
			b.WriteString("\n")
			b.WriteString(item)
			currentLineLen = itemLen
		} else {
			// Append to the current line.
			// Only add a separator if the line isn't empty.
			if currentLineLen > 0 {
				b.WriteString(sep)
				b.WriteString(item)
				currentLineLen += sepLen + itemLen
			} else {
				b.WriteString(item)
				currentLineLen = itemLen
			}
		}
	}

	return b.String()
}
