package sn

import (
	"testing"

	"github.com/spf13/viper"
)

// TestReplaceBasePath tests the BASE_PATH and UNSPLASH replacement
func TestReplaceBasePath(t *testing.T) {
	// Set up viper config for unsplash
	viper.Reset()
	viper.Set("unsplash", "test-unsplash-key")

	tests := []struct {
		name     string
		content  []byte
		basePath string
		expected []byte
	}{
		{
			name:     "replace BASE_PATH only",
			content:  []byte("<a href='{{BASE_PATH}}/about'>About</a>"),
			basePath: "/blog",
			expected: []byte("<a href='/blog/about'>About</a>"),
		},
		{
			name:     "replace multiple BASE_PATH",
			content:  []byte("<a href='{{BASE_PATH}}/a'>A</a><a href='{{BASE_PATH}}/b'>B</a>"),
			basePath: "/site",
			expected: []byte("<a href='/site/a'>A</a><a href='/site/b'>B</a>"),
		},
		{
			name:     "replace UNSPLASH",
			content:  []byte("?client_id={{UNSPLASH}}"),
			basePath: "",
			expected: []byte("?client_id=test-unsplash-key"),
		},
		{
			name:     "replace both BASE_PATH and UNSPLASH",
			content:  []byte("<a href='{{BASE_PATH}}'><img src='https://api.unsplash.com?client_id={{UNSPLASH}}'></a>"),
			basePath: "/mysite",
			expected: []byte("<a href='/mysite'><img src='https://api.unsplash.com?client_id=test-unsplash-key'></a>"),
		},
		{
			name:     "no placeholders",
			content:  []byte("<p>Hello world</p>"),
			basePath: "/anything",
			expected: []byte("<p>Hello world</p>"),
		},
		{
			name:     "empty content",
			content:  []byte(""),
			basePath: "/path",
			expected: []byte(""),
		},
		{
			name:     "empty base path",
			content:  []byte("<a href='{{BASE_PATH}}/page'>Page</a>"),
			basePath: "",
			expected: []byte("<a href='/page'>Page</a>"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceBasePath(tt.content, tt.basePath)
			if string(result) != string(tt.expected) {
				t.Errorf("replaceBasePath() = %q, want %q", string(result), string(tt.expected))
			}
		})
	}
}
