package main

import (
	"reflect"
	"testing"
)

func TestFlatten(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "empty map",
			input:    map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "flat map",
			input: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "nested map",
			input: map[string]interface{}{
				"admin": map[string]interface{}{
					"oauth2": map[string]interface{}{
						"clientID":     "abc123",
						"clientSecret": "secret",
					},
					"publicAddress": "https://example.com",
				},
				"db": map[string]interface{}{
					"url": "postgres://localhost",
				},
			},
			expected: map[string]interface{}{
				"admin.oauth2.clientID":     "abc123",
				"admin.oauth2.clientSecret": "secret",
				"admin.publicAddress":       "https://example.com",
				"db.url":                    "postgres://localhost",
			},
		},
		{
			name: "mixed types",
			input: map[string]interface{}{
				"string": "value",
				"number": 42,
				"bool":   true,
				"nested": map[string]interface{}{
					"inner": "innerValue",
				},
			},
			expected: map[string]interface{}{
				"string":       "value",
				"number":       42,
				"bool":         true,
				"nested.inner": "innerValue",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Flatten(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("Flatten() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
