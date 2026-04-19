package plugins

import (
	"reflect"
	"testing"
)

func TestReplacePropertyPlaceholders(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		props map[string]string
		want  string
	}{
		{
			name:  "all placeholders resolved",
			data:  "host: @@host@@\nport: @@port@@",
			props: map[string]string{"host": "kafka.example.com", "port": "9092"},
			want:  "host: kafka.example.com\nport: 9092",
		},
		{
			name:  "unresolved placeholder replaced with MISSING",
			data:  "host: @@host@@\nport: @@port@@",
			props: map[string]string{"host": "kafka.example.com"},
			want:  "host: kafka.example.com\nport: MISSING",
		},
		{
			name:  "no props leaves all placeholders as MISSING",
			data:  "host: @@host@@",
			props: nil,
			want:  "host: MISSING",
		},
		{
			name:  "empty props leaves all placeholders as MISSING",
			data:  "host: @@host@@",
			props: map[string]string{},
			want:  "host: MISSING",
		},
		{
			name:  "no placeholders at all",
			data:  "host: kafka.example.com\nport: 9092",
			props: map[string]string{"host": "nope"},
			want:  "host: kafka.example.com\nport: 9092",
		},
		{
			name:  "dotted key is resolved",
			data:  "topic: @@kafka.topic.name@@",
			props: map[string]string{"kafka.topic.name": "orders"},
			want:  "topic: orders",
		},
		{
			name:  "same placeholder used multiple times",
			data:  "a: @@x@@\nb: @@x@@\nc: @@x@@",
			props: map[string]string{"x": "1"},
			want:  "a: 1\nb: 1\nc: 1",
		},
		{
			name:  "mixed resolved and unresolved",
			data:  "a: @@known@@\nb: @@unknown@@\nc: @@known@@",
			props: map[string]string{"known": "42"},
			want:  "a: 42\nb: MISSING\nc: 42",
		},
		{
			name:  "lone @@ without closing token is left untouched",
			data:  "note: @@incomplete",
			props: map[string]string{"incomplete": "ignored"},
			want:  "note: @@incomplete",
		},
		{
			name:  "empty placeholder @@@@ is not treated as a placeholder",
			data:  "note: @@@@",
			props: nil,
			want:  "note: @@@@",
		},
		{
			name:  "empty input",
			data:  "",
			props: map[string]string{"x": "y"},
			want:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(replacePropertyPlaceholders([]byte(tc.data), tc.props))
			if got != tc.want {
				t.Errorf("replacePropertyPlaceholders() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseProperties(t *testing.T) {
	tests := []struct {
		name string
		data string
		want map[string]string
	}{
		{
			name: "simple equals separator",
			data: "host=kafka.example.com\nport=9092",
			want: map[string]string{"host": "kafka.example.com", "port": "9092"},
		},
		{
			name: "colon separator",
			data: "host: kafka.example.com\nport: 9092",
			want: map[string]string{"host": "kafka.example.com", "port": "9092"},
		},
		{
			name: "mixed separators",
			data: "host=kafka\nport: 9092",
			want: map[string]string{"host": "kafka", "port": "9092"},
		},
		{
			name: "whitespace around keys and values is trimmed",
			data: "  host  =  kafka.example.com  \n  port:9092",
			want: map[string]string{"host": "kafka.example.com", "port": "9092"},
		},
		{
			name: "comments with # and ! are ignored",
			data: "# this is a comment\n! also a comment\nhost=kafka",
			want: map[string]string{"host": "kafka"},
		},
		{
			name: "blank lines are skipped",
			data: "\n\nhost=kafka\n\n\nport=9092\n",
			want: map[string]string{"host": "kafka", "port": "9092"},
		},
		{
			name: "lines without separator are skipped",
			data: "this is not a property\nhost=kafka",
			want: map[string]string{"host": "kafka"},
		},
		{
			name: "empty value allowed",
			data: "host=",
			want: map[string]string{"host": ""},
		},
		{
			name: "value with = inside keeps the rest after first separator",
			data: "query=a=b&c=d",
			want: map[string]string{"query": "a=b&c=d"},
		},
		{
			name: "duplicate keys: last wins",
			data: "host=first\nhost=second",
			want: map[string]string{"host": "second"},
		},
		{
			name: "dotted keys",
			data: "kafka.topic.name=orders",
			want: map[string]string{"kafka.topic.name": "orders"},
		},
		{
			name: "empty input",
			data: "",
			want: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseProperties([]byte(tc.data))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parseProperties() = %v, want %v", got, tc.want)
			}
		})
	}
}
