package sn

import (
	"strings"
	"testing"
)

func TestGenerateSummaryFromHTML_Empty(t *testing.T) {
	result := GenerateSummaryFromHTML("")
	if result != "" {
		t.Errorf("Expected empty string for empty input, got %q", result)
	}
}

func TestGenerateSummaryFromHTML_SimpleParagraph(t *testing.T) {
	result := GenerateSummaryFromHTML("<p>Hello world.</p>")
	if result != "Hello world." {
		t.Errorf("Expected 'Hello world.', got %q", result)
	}
}

func TestGenerateSummaryFromHTML_MultipleSentences(t *testing.T) {
	result := GenerateSummaryFromHTML("<p>First sentence. Second sentence. Third sentence. Fourth sentence.</p>")
	// Should contain first few sentences but not all
	if !strings.Contains(result, "First sentence.") {
		t.Errorf("Expected result to contain 'First sentence.', got %q", result)
	}
	if !strings.Contains(result, "Second sentence.") {
		t.Errorf("Expected result to contain 'Second sentence.', got %q", result)
	}
}

func TestGenerateSummaryFromHTML_HTMLEntities(t *testing.T) {
	result := GenerateSummaryFromHTML("<p>&amp; special &lt;chars&gt;.</p>")
	if !strings.Contains(result, "&") {
		t.Errorf("Expected HTML entity & to be decoded, got %q", result)
	}
	if !strings.Contains(result, "<chars>") {
		t.Errorf("Expected HTML entities to be decoded, got %q", result)
	}
}

func TestGenerateSummaryFromHTML_NestedTags(t *testing.T) {
	result := GenerateSummaryFromHTML("<div><p><strong>Bold</strong> text.</p></div>")
	if !strings.Contains(result, "Bold") {
		t.Errorf("Expected result to contain 'Bold', got %q", result)
	}
	if !strings.Contains(result, "text") {
		t.Errorf("Expected result to contain 'text', got %q", result)
	}
}

func TestGenerateSummaryFromHTML_MaxLength(t *testing.T) {
	// Create content longer than 200 chars
	longContent := "<p>" + strings.Repeat("Word ", 100) + "</p>"
	result := GenerateSummaryFromHTML(longContent)
	if len(result) > 210 { // Allow some buffer for ellipsis
		t.Errorf("Expected result to be limited to ~200 chars, got %d chars: %q", len(result), result)
	}
}

func TestGenerateSummaryFromHTML_MaxSentences(t *testing.T) {
	// Should only take up to 3 sentences
	result := GenerateSummaryFromHTML("<p>One. Two. Three. Four. Five. Six.</p>")
	sentences := strings.Split(result, ". ")
	// Count non-empty parts
	count := 0
	for _, s := range sentences {
		if strings.TrimSpace(s) != "" {
			count++
		}
	}
	if count > 3 {
		t.Errorf("Expected at most 3 sentences, got %d in %q", count, result)
	}
}

func TestGenerateSummaryFromHTML_NoSentences(t *testing.T) {
	// Content without period separators
	result := GenerateSummaryFromHTML("<p>Just some text without periods</p>")
	if result == "" {
		t.Errorf("Expected non-empty result for content without sentence breaks")
	}
}

func TestGenerateSummaryFromHTML_OnlyWhitespace(t *testing.T) {
	result := GenerateSummaryFromHTML("<p>   </p>")
	if result != "" {
		t.Errorf("Expected empty string for whitespace-only content, got %q", result)
	}
}

func TestGenerateSummaryFromHTML_PreservePunctuation(t *testing.T) {
	result := GenerateSummaryFromHTML("<p>Question? Exclamation! Statement.</p>")
	if !strings.Contains(result, "?") {
		t.Errorf("Expected question mark to be preserved, got %q", result)
	}
	if !strings.Contains(result, "!") {
		t.Errorf("Expected exclamation mark to be preserved, got %q", result)
	}
}

// TestAndSQL tests the SQL query builder helper
func TestAndSQL(t *testing.T) {
	tests := []struct {
		name         string
		paramName    string
		qryParam     *string
		initialSQL   string
		initialVals  []any
		expectedSQL  string
		expectedVals int
	}{
		{
			name:         "nil param - no change",
			paramName:    "slug",
			qryParam:     nil,
			initialSQL:   "SELECT * FROM items WHERE 1",
			initialVals:  []any{},
			expectedSQL:  "SELECT * FROM items WHERE 1",
			expectedVals: 0,
		},
		{
			name:         "non-nil param - adds condition",
			paramName:    "slug",
			qryParam:     strPtr("test-slug"),
			initialSQL:   "SELECT * FROM items WHERE 1",
			initialVals:  []any{},
			expectedSQL:  "SELECT * FROM items WHERE 1 AND slug = ?",
			expectedVals: 1,
		},
		{
			name:         "multiple params build up",
			paramName:    "repo",
			qryParam:     strPtr("blog"),
			initialSQL:   "SELECT * FROM items WHERE 1 AND slug = ?",
			initialVals:  []any{"test"},
			expectedSQL:  "SELECT * FROM items WHERE 1 AND slug = ? AND repo = ?",
			expectedVals: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultSQL, resultVals := andSQL(tt.paramName, tt.qryParam, tt.initialSQL, tt.initialVals)

			if resultSQL != tt.expectedSQL {
				t.Errorf("SQL mismatch: got %q, want %q", resultSQL, tt.expectedSQL)
			}

			if len(resultVals) != tt.expectedVals {
				t.Errorf("Values count mismatch: got %d, want %d", len(resultVals), tt.expectedVals)
			}
		})
	}
}

// strPtr is a helper to create string pointers
func strPtr(s string) *string {
	return &s
}

// TestReplaceParams tests parameter substitution in query parameters
func TestReplaceParams(t *testing.T) {
	tests := []struct {
		name     string
		values   map[string]interface{}
		params   map[string]string
		expected map[string]interface{}
	}{
		{
			name: "simple substitution",
			values: map[string]interface{}{
				"slug": "{myslug}",
			},
			params: map[string]string{
				"myslug": "test-post",
			},
			expected: map[string]interface{}{
				"slug": "test-post",
			},
		},
		{
			name: "multiple substitutions",
			values: map[string]interface{}{
				"title": "{prefix}-{suffix}",
			},
			params: map[string]string{
				"prefix": "hello",
				"suffix": "world",
			},
			expected: map[string]interface{}{
				"title": "hello-world",
			},
		},
		{
			name: "no substitution needed",
			values: map[string]interface{}{
				"static": "value",
			},
			params: map[string]string{
				"other": "param",
			},
			expected: map[string]interface{}{
				"static": "value",
			},
		},
		{
			name: "non-string value unchanged",
			values: map[string]interface{}{
				"count": 42,
			},
			params: map[string]string{
				"count": "100",
			},
			expected: map[string]interface{}{
				"count": 42, // Should remain integer
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := replaceParams(tt.values, tt.params)

			for k, expected := range tt.expected {
				if result[k] != expected {
					t.Errorf("Key %q: got %v, want %v", k, result[k], expected)
				}
			}
		})
	}
}

// TestItemQueryStruct tests ItemQuery struct initialization
func TestItemQueryStruct(t *testing.T) {
	slug := "test"
	repo := "blog"

	query := ItemQuery{
		PerPage:     10,
		Page:        2,
		Slug:        &slug,
		Repo:        &repo,
		Frontmatter: map[string]string{"key": "value"},
	}

	if query.PerPage != 10 {
		t.Errorf("PerPage = %d, want 10", query.PerPage)
	}
	if query.Page != 2 {
		t.Errorf("Page = %d, want 2", query.Page)
	}
	if *query.Slug != "test" {
		t.Errorf("Slug = %q, want 'test'", *query.Slug)
	}
	if *query.Repo != "blog" {
		t.Errorf("Repo = %q, want 'blog'", *query.Repo)
	}
	if query.Frontmatter["key"] != "value" {
		t.Error("Frontmatter not set correctly")
	}
}

// TestSchema tests that schema returns valid SQL
func TestSchema(t *testing.T) {
	schemaSQL := schema()

	// Verify it contains expected table creations
	if !strings.Contains(schemaSQL, "CREATE TABLE IF NOT EXISTS \"items\"") {
		t.Error("Schema should contain items table creation")
	}
	if !strings.Contains(schemaSQL, "CREATE TABLE IF NOT EXISTS \"authors\"") {
		t.Error("Schema should contain authors table creation")
	}
	if !strings.Contains(schemaSQL, "CREATE TABLE IF NOT EXISTS \"categories\"") {
		t.Error("Schema should contain categories table creation")
	}
	if !strings.Contains(schemaSQL, "CREATE TABLE IF NOT EXISTS \"frontmatter\"") {
		t.Error("Schema should contain frontmatter table creation")
	}
	if !strings.Contains(schemaSQL, "CREATE TABLE IF NOT EXISTS \"items_authors\"") {
		t.Error("Schema should contain items_authors table creation")
	}
	if !strings.Contains(schemaSQL, "CREATE TABLE IF NOT EXISTS \"items_categories\"") {
		t.Error("Schema should contain items_categories table creation")
	}

	// Verify indexes are created
	if !strings.Contains(schemaSQL, "CREATE INDEX IF NOT EXISTS") {
		t.Error("Schema should contain index creation statements")
	}
	if !strings.Contains(schemaSQL, "CREATE UNIQUE INDEX IF NOT EXISTS") {
		t.Error("Schema should contain unique index creation statements")
	}
}
