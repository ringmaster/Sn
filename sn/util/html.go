package util

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// GenerateSummaryFromHTML creates a summary from HTML content
func GenerateSummaryFromHTML(htmlContent string) string {
	if htmlContent == "" {
		return ""
	}

	// Remove HTML tags and get plain text
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return ""
	}

	// Get the text content, removing extra whitespace
	text := strings.TrimSpace(doc.Text())
	if text == "" {
		return ""
	}

	// Split into sentences and take first 2-3 sentences or 200 characters, whichever is shorter
	sentences := strings.Split(text, ". ")
	var summaryParts []string
	totalLength := 0
	maxLength := 200

	for i, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}

		// Add period back if it's not the last sentence and doesn't already end with punctuation
		if i < len(sentences)-1 && !strings.HasSuffix(sentence, ".") && !strings.HasSuffix(sentence, "!") && !strings.HasSuffix(sentence, "?") {
			sentence += "."
		}

		// Check if adding this sentence would exceed our limits
		if totalLength+len(sentence) > maxLength || len(summaryParts) >= 3 {
			break
		}

		summaryParts = append(summaryParts, sentence)
		totalLength += len(sentence)
	}

	summary := strings.Join(summaryParts, " ")

	// Ensure we don't cut off mid-word if we hit the length limit
	if len(summary) > maxLength {
		words := strings.Fields(summary)
		summary = ""
		for _, word := range words {
			if len(summary)+len(word)+1 > maxLength {
				break
			}
			if summary != "" {
				summary += " "
			}
			summary += word
		}
		if summary != "" && !strings.HasSuffix(summary, ".") && !strings.HasSuffix(summary, "!") && !strings.HasSuffix(summary, "?") {
			summary += "..."
		}
	}

	return summary
}
