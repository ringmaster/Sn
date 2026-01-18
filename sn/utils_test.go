package sn

import (
	"strings"
	"testing"
)

func TestMinOf(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected int
	}{
		{"single value", []int{5}, 5},
		{"two values ascending", []int{1, 2}, 1},
		{"two values descending", []int{2, 1}, 1},
		{"multiple values", []int{5, 3, 8, 1, 9}, 1},
		{"negative values", []int{-5, -3, -8, -1}, -8},
		{"mixed values", []int{-5, 0, 5}, -5},
		{"all same", []int{3, 3, 3}, 3},
		{"large numbers", []int{1000000, 2000000, 500000}, 500000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MinOf(tt.input...)
			if result != tt.expected {
				t.Errorf("MinOf(%v) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMaxOf(t *testing.T) {
	tests := []struct {
		name     string
		input    []int
		expected int
	}{
		{"single value", []int{5}, 5},
		{"two values ascending", []int{1, 2}, 2},
		{"two values descending", []int{2, 1}, 2},
		{"multiple values", []int{5, 3, 8, 1, 9}, 9},
		{"negative values", []int{-5, -3, -8, -1}, -1},
		{"mixed values", []int{-5, 0, 5}, 5},
		{"all same", []int{3, 3, 3}, 3},
		{"large numbers", []int{1000000, 2000000, 500000}, 2000000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaxOf(tt.input...)
			if result != tt.expected {
				t.Errorf("MaxOf(%v) = %d, expected %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCopyMap(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]interface{}
	}{
		{
			name:  "empty map",
			input: map[string]interface{}{},
		},
		{
			name: "simple values",
			input: map[string]interface{}{
				"string": "hello",
				"int":    42,
				"bool":   true,
			},
		},
		{
			name: "with nested map that should be excluded",
			input: map[string]interface{}{
				"string": "hello",
				"nested": map[string]interface{}{"key": "value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CopyMap(tt.input)

			// Verify result is a new map
			if &result == &tt.input {
				t.Error("CopyMap should return a new map, not the same reference")
			}

			// Verify non-map values are copied
			for k, v := range tt.input {
				if _, isMap := v.(map[string]interface{}); !isMap {
					if result[k] != v {
						t.Errorf("CopyMap: key %s not copied correctly, got %v, expected %v", k, result[k], v)
					}
				}
			}

			// Verify nested maps are excluded
			for k, v := range result {
				if _, isMap := v.(map[string]interface{}); isMap {
					t.Errorf("CopyMap should not copy nested maps, but found one at key %s", k)
				}
			}
		})
	}
}

func TestCopyMapExcludesNestedMaps(t *testing.T) {
	input := map[string]interface{}{
		"string": "hello",
		"number": 42,
		"nested": map[string]interface{}{
			"inner": "value",
		},
	}

	result := CopyMap(input)

	// Should have 2 items (string and number), not 3
	if len(result) != 2 {
		t.Errorf("CopyMap should exclude nested maps, got %d items, expected 2", len(result))
	}

	if result["string"] != "hello" {
		t.Error("CopyMap should copy 'string' key")
	}

	if result["number"] != 42 {
		t.Error("CopyMap should copy 'number' key")
	}

	if _, exists := result["nested"]; exists {
		t.Error("CopyMap should not copy 'nested' key (nested map)")
	}
}

func TestPrintMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		indent   string
		contains []string
	}{
		{
			name:     "empty map",
			input:    map[string]interface{}{},
			indent:   "",
			contains: []string{},
		},
		{
			name: "simple map",
			input: map[string]interface{}{
				"key1": "value1",
			},
			indent:   "",
			contains: []string{"Key: key1", "Value: value1"},
		},
		{
			name: "with indent",
			input: map[string]interface{}{
				"key1": "value1",
			},
			indent:   "  ",
			contains: []string{"  Key: key1"},
		},
		{
			name: "nested map",
			input: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
			indent:   "",
			contains: []string{"Key: outer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PrintMap(tt.input, tt.indent)

			for _, expected := range tt.contains {
				if !strings.Contains(result, expected) {
					t.Errorf("PrintMap output should contain %q, got %q", expected, result)
				}
			}
		})
	}
}

func TestPrintMapWithNumbers(t *testing.T) {
	input := map[string]interface{}{
		"count": 42,
		"price": 19.99,
	}

	result := PrintMap(input, "")

	if !strings.Contains(result, "count") {
		t.Error("PrintMap should include 'count' key")
	}
	if !strings.Contains(result, "42") {
		t.Error("PrintMap should include value 42")
	}
}

func TestDirExists(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"root exists", "/", true},
		{"tmp usually exists", "/tmp", true},
		{"nonexistent path", "/definitely/does/not/exist/xyz123", false},
		{"empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DirExists(tt.path)
			if result != tt.expected {
				t.Errorf("DirExists(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}
