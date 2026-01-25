// Context Query Builder for LLM Context Assembly
//
// Token-aware context assembly with priority-based truncation.

package sochdb

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ContextOutputFormat represents the output format for context
type ContextOutputFormat string

const (
	FormatTOON     ContextOutputFormat = "toon"
	FormatJSON     ContextOutputFormat = "json"
	FormatMarkdown ContextOutputFormat = "markdown"
)

// TruncationStrategy represents how to truncate context
type TruncationStrategy string

const (
	TailDrop     TruncationStrategy = "tail_drop"    // Drop from end
	HeadDrop     TruncationStrategy = "head_drop"    // Drop from beginning
	Proportional TruncationStrategy = "proportional" // Proportional across sections
)

// Section represents a context section
type section struct {
	Name       string
	Priority   int
	Content    string
	TokenCount int
}

// ContextSection represents a section in the result
type ContextSection struct {
	Name       string `json:"name"`
	TokenCount int    `json:"token_count"`
	Truncated  bool   `json:"truncated"`
}

// ContextResult represents the built context
type ContextResult struct {
	Text       string           `json:"text"`
	TokenCount int              `json:"token_count"`
	Sections   []ContextSection `json:"sections"`
	Truncated  bool             `json:"truncated"`
}

// ContextQueryBuilder builds LLM context with token awareness
type ContextQueryBuilder struct {
	sessionID   string
	tokenBudget int
	format      ContextOutputFormat
	truncation  TruncationStrategy
	sections    []section
}

// NewContextQueryBuilder creates a new context builder
func NewContextQueryBuilder() *ContextQueryBuilder {
	return &ContextQueryBuilder{
		tokenBudget: 4096,
		format:      FormatTOON,
		truncation:  TailDrop,
		sections:    []section{},
	}
}

// ForSession sets the session ID
func (b *ContextQueryBuilder) ForSession(sessionID string) *ContextQueryBuilder {
	b.sessionID = sessionID
	return b
}

// WithBudget sets the token budget
func (b *ContextQueryBuilder) WithBudget(tokens int) *ContextQueryBuilder {
	b.tokenBudget = tokens
	return b
}

// SetFormat sets the output format
func (b *ContextQueryBuilder) SetFormat(format ContextOutputFormat) *ContextQueryBuilder {
	b.format = format
	return b
}

// SetTruncation sets the truncation strategy
func (b *ContextQueryBuilder) SetTruncation(strategy TruncationStrategy) *ContextQueryBuilder {
	b.truncation = strategy
	return b
}

// Literal adds a literal text section
func (b *ContextQueryBuilder) Literal(name string, priority int, text string) *ContextQueryBuilder {
	tokenCount := b.estimateTokens(text)
	b.sections = append(b.sections, section{
		Name:       name,
		Priority:   priority,
		Content:    text,
		TokenCount: tokenCount,
	})
	return b
}

// estimateTokens estimates token count (simple approximation)
func (b *ContextQueryBuilder) estimateTokens(text string) int {
	// Simple estimation: ~4 characters per token (English text)
	return len(text) / 4
}

// Execute builds the context
func (b *ContextQueryBuilder) Execute() (*ContextResult, error) {
	// Sort sections by priority (lower = higher priority)
	sortedSections := make([]section, len(b.sections))
	copy(sortedSections, b.sections)
	sort.Slice(sortedSections, func(i, j int) bool {
		return sortedSections[i].Priority < sortedSections[j].Priority
	})

	// Calculate total tokens
	totalTokens := 0
	for _, s := range sortedSections {
		totalTokens += s.TokenCount
	}

	// Apply truncation if needed
	truncated := false
	if totalTokens > b.tokenBudget {
		truncated = true
		sortedSections = b.applyTruncation(sortedSections)
	}

	// Format output
	text, err := b.formatOutput(sortedSections)
	if err != nil {
		return nil, err
	}

	// Build result sections
	resultSections := make([]ContextSection, len(sortedSections))
	for i, s := range sortedSections {
		resultSections[i] = ContextSection{
			Name:       s.Name,
			TokenCount: s.TokenCount,
			Truncated:  false, // Individual section truncation not tracked in this impl
		}
	}

	// Recalculate token count
	finalTokens := 0
	for _, s := range sortedSections {
		finalTokens += s.TokenCount
	}

	return &ContextResult{
		Text:       text,
		TokenCount: finalTokens,
		Sections:   resultSections,
		Truncated:  truncated,
	}, nil
}

// applyTruncation applies truncation strategy
func (b *ContextQueryBuilder) applyTruncation(sections []section) []section {
	switch b.truncation {
	case TailDrop:
		return b.tailDropTruncation(sections)
	case HeadDrop:
		return b.headDropTruncation(sections)
	case Proportional:
		return b.proportionalTruncation(sections)
	default:
		return sections
	}
}

// tailDropTruncation drops sections from the end
func (b *ContextQueryBuilder) tailDropTruncation(sections []section) []section {
	result := []section{}
	tokenCount := 0

	for _, s := range sections {
		if tokenCount+s.TokenCount <= b.tokenBudget {
			result = append(result, s)
			tokenCount += s.TokenCount
		} else {
			break
		}
	}

	return result
}

// headDropTruncation drops sections from the beginning
func (b *ContextQueryBuilder) headDropTruncation(sections []section) []section {
	totalTokens := 0
	for _, s := range sections {
		totalTokens += s.TokenCount
	}

	// Calculate how many tokens to drop
	toDrop := totalTokens - b.tokenBudget
	if toDrop <= 0 {
		return sections
	}

	result := []section{}
	dropped := 0

	for _, s := range sections {
		if dropped+s.TokenCount <= toDrop {
			dropped += s.TokenCount
		} else {
			result = append(result, s)
		}
	}

	return result
}

// proportionalTruncation truncates proportionally
func (b *ContextQueryBuilder) proportionalTruncation(sections []section) []section {
	totalTokens := 0
	for _, s := range sections {
		totalTokens += s.TokenCount
	}

	if totalTokens <= b.tokenBudget {
		return sections
	}

	// Calculate reduction factor
	factor := float64(b.tokenBudget) / float64(totalTokens)

	result := make([]section, len(sections))
	for i, s := range sections {
		newTokenCount := int(float64(s.TokenCount) * factor)
		if newTokenCount < 1 {
			newTokenCount = 1
		}

		// Truncate content proportionally
		newLength := int(float64(len(s.Content)) * factor)
		if newLength > len(s.Content) {
			newLength = len(s.Content)
		}

		result[i] = section{
			Name:       s.Name,
			Priority:   s.Priority,
			Content:    s.Content[:newLength],
			TokenCount: newTokenCount,
		}
	}

	return result
}

// formatOutput formats sections according to output format
func (b *ContextQueryBuilder) formatOutput(sections []section) (string, error) {
	switch b.format {
	case FormatTOON:
		return b.formatTOON(sections), nil
	case FormatJSON:
		return b.formatJSON(sections)
	case FormatMarkdown:
		return b.formatMarkdown(sections), nil
	default:
		return "", fmt.Errorf("unknown format: %s", b.format)
	}
}

// formatTOON formats as TOON (section-based format)
func (b *ContextQueryBuilder) formatTOON(sections []section) string {
	var builder strings.Builder

	for _, s := range sections {
		builder.WriteString(fmt.Sprintf("[%s]\n", s.Name))
		builder.WriteString(s.Content)
		builder.WriteString("\n\n")
	}

	return strings.TrimSpace(builder.String())
}

// formatJSON formats as JSON
func (b *ContextQueryBuilder) formatJSON(sections []section) (string, error) {
	data := make(map[string]string)
	for _, s := range sections {
		data[s.Name] = s.Content
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	return string(bytes), nil
}

// formatMarkdown formats as Markdown
func (b *ContextQueryBuilder) formatMarkdown(sections []section) string {
	var builder strings.Builder

	for _, s := range sections {
		builder.WriteString(fmt.Sprintf("## %s\n\n", s.Name))
		builder.WriteString(s.Content)
		builder.WriteString("\n\n")
	}

	return strings.TrimSpace(builder.String())
}
