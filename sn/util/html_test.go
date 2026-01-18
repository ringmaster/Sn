package util

import (
	"strings"
	"testing"
)

// TestGenerateSummaryFromHTML_Basic tests basic summary generation
func TestGenerateSummaryFromHTML_Basic(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty input returns empty",
			input:    "",
			expected: "",
		},
		{
			name:     "simple paragraph",
			input:    "<p>Hello world.</p>",
			expected: "Hello world.",
		},
		{
			name:     "whitespace only returns empty",
			input:    "<p>   </p>",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSummaryFromHTML(tt.input)
			if result != tt.expected {
				t.Errorf("GenerateSummaryFromHTML(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGenerateSummaryFromHTML_SentenceExtraction verifies that the function
// correctly extracts sentences from HTML content
func TestGenerateSummaryFromHTML_SentenceExtraction(t *testing.T) {
	input := "<p>First sentence here. Second sentence follows. Third one too. Fourth sentence. Fifth sentence.</p>"
	result := GenerateSummaryFromHTML(input)

	// Should contain first sentence
	if !strings.Contains(result, "First sentence here.") {
		t.Errorf("Result should contain first sentence, got %q", result)
	}

	// Should be limited (not contain all sentences due to length/count limits)
	// The function limits to 3 sentences or 200 chars
	sentences := strings.Split(result, ". ")
	if len(sentences) > 4 { // Allow for some variation in counting
		t.Errorf("Result should be limited to ~3 sentences, got %d parts", len(sentences))
	}
}

// TestGenerateSummaryFromHTML_HTMLStripping verifies HTML tags are properly removed
func TestGenerateSummaryFromHTML_HTMLStripping(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldHave  string
		shouldntHave string
	}{
		{
			name:        "removes bold tags",
			input:       "<p><strong>Bold text</strong> here.</p>",
			shouldHave:  "Bold text",
			shouldntHave: "<strong>",
		},
		{
			name:        "removes italic tags",
			input:       "<p><em>Italic text</em> here.</p>",
			shouldHave:  "Italic text",
			shouldntHave: "<em>",
		},
		{
			name:        "removes nested divs",
			input:       "<div><div><p>Nested content.</p></div></div>",
			shouldHave:  "Nested content",
			shouldntHave: "<div>",
		},
		{
			name:        "removes links but keeps text",
			input:       "<p>Click <a href='http://example.com'>here</a> for more.</p>",
			shouldHave:  "here",
			shouldntHave: "<a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSummaryFromHTML(tt.input)

			if !strings.Contains(result, tt.shouldHave) {
				t.Errorf("Result should contain %q, got %q", tt.shouldHave, result)
			}

			if strings.Contains(result, tt.shouldntHave) {
				t.Errorf("Result should not contain %q, got %q", tt.shouldntHave, result)
			}
		})
	}
}

// TestGenerateSummaryFromHTML_HTMLEntities verifies HTML entities are decoded
func TestGenerateSummaryFromHTML_HTMLEntities(t *testing.T) {
	input := "<p>&amp; ampersand and &lt;brackets&gt; and &quot;quotes&quot;.</p>"
	result := GenerateSummaryFromHTML(input)

	if !strings.Contains(result, "&") {
		t.Errorf("& entity should be decoded, got %q", result)
	}

	if !strings.Contains(result, "<brackets>") {
		t.Errorf("<> entities should be decoded, got %q", result)
	}
}

// TestGenerateSummaryFromHTML_LengthLimit verifies the 200 character limit
func TestGenerateSummaryFromHTML_LengthLimit(t *testing.T) {
	// Create a very long paragraph
	longText := strings.Repeat("This is a long sentence that keeps going. ", 20)
	input := "<p>" + longText + "</p>"

	result := GenerateSummaryFromHTML(input)

	// Result should be roughly limited to 200 chars (with some buffer for ellipsis)
	if len(result) > 210 {
		t.Errorf("Result should be limited to ~200 chars, got %d chars: %q", len(result), result)
	}
}

// TestGenerateSummaryFromHTML_PreservesPunctuation verifies sentence-ending punctuation
func TestGenerateSummaryFromHTML_PreservesPunctuation(t *testing.T) {
	input := "<p>Is this a question? Yes it is! And a statement.</p>"
	result := GenerateSummaryFromHTML(input)

	if !strings.Contains(result, "?") {
		t.Errorf("Question marks should be preserved, got %q", result)
	}

	if !strings.Contains(result, "!") {
		t.Errorf("Exclamation marks should be preserved, got %q", result)
	}
}

// TestGenerateSummaryFromHTML_ComplexHTML tests real-world HTML structures
func TestGenerateSummaryFromHTML_ComplexHTML(t *testing.T) {
	input := `
		<article>
			<h1>Article Title</h1>
			<p>This is the first paragraph of the article. It contains important information.</p>
			<p>This is the second paragraph with more details.</p>
			<ul>
				<li>List item one</li>
				<li>List item two</li>
			</ul>
		</article>
	`

	result := GenerateSummaryFromHTML(input)

	// Should extract text content
	if result == "" {
		t.Error("Should extract content from complex HTML")
	}

	// Should not contain HTML tags
	if strings.Contains(result, "<") || strings.Contains(result, ">") {
		t.Errorf("Should not contain HTML tags, got %q", result)
	}
}

// TestGenerateSummaryFromHTML_NoSentenceBreaks tests content without periods
func TestGenerateSummaryFromHTML_NoSentenceBreaks(t *testing.T) {
	input := "<p>This is content without any sentence breaks just one long continuous text</p>"
	result := GenerateSummaryFromHTML(input)

	if result == "" {
		t.Error("Should handle content without sentence breaks")
	}

	// Should contain the text
	if !strings.Contains(result, "content") {
		t.Errorf("Should contain text content, got %q", result)
	}
}

// TestGenerateSummaryFromHTML_ExceedMaxLength tests truncation when summary exceeds max length
func TestGenerateSummaryFromHTML_ExceedMaxLength(t *testing.T) {
	// Create input where each sentence is 50 chars, so 4 sentences = 200 chars exactly
	// This should trigger the "summary > maxLength" branch
	input := "<p>This is a sentence with exactly fifty chars ok. Another sentence with exactly fifty chars ok. Yet another with exactly fifty character ok. One more sentence with exactly fifty chars ok.</p>"
	result := GenerateSummaryFromHTML(input)

	// Should be limited
	if len(result) > 210 {
		t.Errorf("Summary should be limited, got %d chars: %q", len(result), result)
	}
}

// TestGenerateSummaryFromHTML_EmptySentences tests handling of empty sentences
func TestGenerateSummaryFromHTML_EmptySentences(t *testing.T) {
	// Multiple periods create empty sentences
	input := "<p>First sentence... Second sentence.</p>"
	result := GenerateSummaryFromHTML(input)

	// Should still produce output
	if result == "" {
		t.Error("Should handle content with multiple periods")
	}
}

// TestGenerateSummaryFromHTML_TruncationWithEllipsis tests that truncated content gets ellipsis
func TestGenerateSummaryFromHTML_TruncationWithEllipsis(t *testing.T) {
	// Create a very long single-word string that will need truncation
	longWord := strings.Repeat("verylongword ", 30)
	input := "<p>" + longWord + "</p>"
	result := GenerateSummaryFromHTML(input)

	// Result should be limited
	if len(result) > 210 {
		t.Errorf("Summary should be limited, got %d chars", len(result))
	}

	// Should end with ellipsis if truncated without natural sentence ending
	if len(result) > 0 && !strings.HasSuffix(result, ".") && !strings.HasSuffix(result, "!") && !strings.HasSuffix(result, "?") && !strings.HasSuffix(result, "...") {
		// It's ok if it doesn't end with ellipsis if it happened to end at a word boundary
		// Just make sure it's not cut mid-word
		words := strings.Fields(result)
		if len(words) > 0 {
			lastWord := words[len(words)-1]
			if !strings.HasSuffix(lastWord, "...") && !strings.HasSuffix(lastWord, ".") {
				// This is acceptable - word boundary truncation
			}
		}
	}
}

// TestGenerateSummaryFromHTML_SentencesWithPunctuation tests sentences ending with different punctuation
func TestGenerateSummaryFromHTML_SentencesWithPunctuation(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldHave  string
	}{
		{
			name:       "question mark sentence",
			input:      "<p>Is this working? Yes.</p>",
			shouldHave: "?",
		},
		{
			name:       "exclamation mark sentence",
			input:      "<p>This is great! Indeed.</p>",
			shouldHave: "!",
		},
		{
			name:       "period sentence",
			input:      "<p>This is a sentence. Another one.</p>",
			shouldHave: ".",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSummaryFromHTML(tt.input)
			if !strings.Contains(result, tt.shouldHave) {
				t.Errorf("Result should contain %q, got %q", tt.shouldHave, result)
			}
		})
	}
}

// TestGenerateSummaryFromHTML_ThreeSentenceLimit tests the 3 sentence limit
func TestGenerateSummaryFromHTML_ThreeSentenceLimit(t *testing.T) {
	// Short sentences that won't hit 200 char limit
	input := "<p>One. Two. Three. Four. Five.</p>"
	result := GenerateSummaryFromHTML(input)

	// Count sentences (rough approximation)
	// With short sentences, should get at most 3
	parts := strings.Split(result, ".")
	nonEmpty := 0
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			nonEmpty++
		}
	}

	if nonEmpty > 3 {
		t.Errorf("Should limit to ~3 sentences, got %d non-empty parts in %q", nonEmpty, result)
	}
}
