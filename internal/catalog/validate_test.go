package catalog

import "testing"

func TestIsValidLabel(t *testing.T) {
	testCases := []struct {
		name        string
		key         string
		value       string
		expectValid bool
	}{
		// --- Valid Cases ---
		{
			name:        "simple key and value",
			key:         "app",
			value:       "nginx",
			expectValid: true,
		},
		{
			name:        "qualified key with simple value",
			key:         "kubernetes.io/arch",
			value:       "amd64",
			expectValid: true,
		},
		{
			name:        "key with all allowed characters",
			key:         "my.app_key-v1",
			value:       "stable",
			expectValid: true,
		},
		{
			name:        "value with all allowed characters",
			key:         "version",
			value:       "v1.2_3-beta",
			expectValid: true,
		},
		{
			name:        "empty value",
			key:         "my-key",
			value:       "",
			expectValid: true,
		},
		{
			name:        "max length key name (63 chars)",
			key:         "a123456789a123456789a123456789a123456789a123456789a123456789ab1",
			value:       "ok",
			expectValid: true,
		},
		{
			name:        "max length value (63 chars)",
			key:         "data",
			value:       "a123456789a123456789a123456789a123456789a123456789a123456789ab1",
			expectValid: true,
		},
		{
			name: "max length key prefix (253 chars)",
			// This prefix is exactly 253 characters: 22 segments of "a123456789." (242 chars)
			// plus one final segment of "a123456789a" (11 chars).
			key:         "a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789a/name",
			value:       "ok",
			expectValid: true,
		},
		// --- Invalid Key Cases ---
		{
			name:        "key with more than one slash",
			key:         "a/b/c",
			value:       "ok",
			expectValid: false,
		},
		{
			name:        "key name starts with a dash",
			key:         "-app",
			value:       "ok",
			expectValid: false,
		},
		{
			name:        "key name ends with a dot",
			key:         "app.",
			value:       "ok",
			expectValid: false,
		},
		{
			name:        "key name is too long (64 chars)",
			key:         "a123456789a123456789a123456789a123456789a123456789a123456789ab12",
			value:       "ok",
			expectValid: false,
		},
		{
			name:        "empty key name",
			key:         "",
			value:       "ok",
			expectValid: false,
		},
		{
			name:        "empty prefix with slash",
			key:         "/key",
			value:       "ok",
			expectValid: false,
		},
		{
			name:        "key prefix is too long (254 chars)",
			key:         "a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a123456789.a1234/name",
			value:       "ok",
			expectValid: false,
		},
		{
			name:        "key prefix contains invalid characters (_)",
			key:         "k8s_io/app",
			value:       "ok",
			expectValid: false,
		},
		{
			name:        "key prefix contains uppercase letters",
			key:         "MyDomain.com/app",
			value:       "ok",
			expectValid: false,
		},

		// --- Invalid Value Cases ---
		{
			name:        "value is too long (64 chars)",
			key:         "app",
			value:       "a123456789a123456789a123456789a123456789a123456789a123456789ab12",
			expectValid: false,
		},
		{
			name:        "value starts with an underscore",
			key:         "app",
			value:       "_nginx",
			expectValid: false,
		},
		{
			name:        "value ends with a dash",
			key:         "app",
			value:       "nginx-",
			expectValid: false,
		},
		{
			name:        "invalid key and invalid value",
			key:         "App",
			value:       "nginx-",
			expectValid: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsValidLabel(tc.key, tc.value)
			if got != tc.expectValid {
				t.Errorf("IsValidLabel(%q, %q) = %v; want %v", tc.key, tc.value, got, tc.expectValid)
			}
		})
	}
}
