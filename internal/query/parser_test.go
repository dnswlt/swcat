package query

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "single simple term",
			input:    "bana",
			expected: "bana",
		},
		{
			name:     "single attribute term",
			input:    "tag:foo",
			expected: "tag:foo",
		},
		{
			name:     "single attribute term with tilde",
			input:    "label~bar",
			expected: "label~bar",
		},
		{
			name:     "single attribute term with quoted value",
			input:    "owner:'helly r'",
			expected: "owner:'helly r'",
		},
		{
			name:     "simple OR",
			input:    "a OR b",
			expected: "(a OR b)",
		},
		{
			name:     "simple AND",
			input:    "a AND b",
			expected: "(a AND b)",
		},
		{
			name:     "simple implicit AND",
			input:    "a b",
			expected: "(a AND b)",
		},
		{
			name:     "negation",
			input:    "!bana",
			expected: "!bana",
		},
		{
			name:     "negation with attribute",
			input:    "!tag:foo",
			expected: "!tag:foo",
		},
		{
			name:     "grouped expression",
			input:    "(a OR b)",
			expected: "(a OR b)",
		},
		{
			name:     "AND and OR precedence",
			input:    "a AND b OR c",
			expected: "((a AND b) OR c)",
		},
		{
			name:     "OR and AND precedence",
			input:    "a OR b AND c",
			expected: "(a OR (b AND c))",
		},
		{
			name:     "grouped with surrounding terms",
			input:    "x (a OR b) y",
			expected: "((x AND (a OR b)) AND y)",
		},
		{
			name:     "negated group",
			input:    "!(a OR b)",
			expected: "!(a OR b)",
		},
		{
			name:     "original complex query",
			input:    "bana (tag:foo OR tag:bar) label~yankee.*doodle owner:'helly r'",
			expected: "(((bana AND (tag:foo OR tag:bar)) AND label~yankee.*doodle) AND owner:'helly r')",
		},
		{
			name:     "deeply nested",
			input:    "a AND (b OR (c AND d))",
			expected: "(a AND (b OR (c AND d)))",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ast, err := Parse(tc.input)
			if err != nil {
				t.Fatalf("Parse() returned an unexpected error: %v", err)
			}
			if ast == nil {
				t.Fatalf("Parse() returned a nil AST without an error")
			}
			if ast.String() != tc.expected {
				t.Errorf("\nInput:    %s\nExpected: %s\nGot:      %s", tc.input, tc.expected, ast.String())
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	testCases := []struct {
		name        string
		input       string
		expectedErr string
	}{
		{
			name:        "unclosed parenthesis",
			input:       "bana (tag:foo",
			expectedErr: "expected ')' to close group",
		},
		{
			name:        "unexpected closing parenthesis",
			input:       "bana tag:foo)",
			expectedErr: "unexpected token at start of expression: RPAREN",
		},
		{
			name:        "missing value for attribute",
			input:       "tag:",
			expectedErr: "expected identifier or string for attribute value, got EOF",
		},
		{
			name:        "operator at start",
			input:       "OR bana",
			expectedErr: "unexpected token at start of expression: OR",
		},
		{
			// String literals are only supported on explicit tags for now.
			name:        "string literal only",
			input:       "'some thing''",
			expectedErr: "unexpected token at start of expression: STRING",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.input)
			if err == nil {
				t.Fatalf("Expected an error but got none")
			}
			if !strings.Contains(err.Error(), tc.expectedErr) {
				t.Errorf("\nInput:       %s\nExpected err: %s\nGot err:      %v", tc.input, tc.expectedErr, err)
			}
		})
	}
}
