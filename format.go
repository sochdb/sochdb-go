// Copyright 2025 Sushanth (https://github.com/sushanthpy)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package toondb provides unified output format semantics.
//
// This module provides format enums for query results and LLM context packaging.
// It mirrors the Rust toondb-client format module for consistency.
package toondb

import "fmt"

// FormatConversionError represents an error when format conversion fails.
type FormatConversionError struct {
	FromFormat string
	ToFormat   string
	Reason     string
}

func (e *FormatConversionError) Error() string {
	return fmt.Sprintf("Cannot convert %s to %s: %s", e.FromFormat, e.ToFormat, e.Reason)
}

// WireFormat represents output format for query results sent to clients.
//
// These formats are optimized for transmission efficiency and
// client-side processing.
type WireFormat int

const (
	// WireFormatToon is TOON format (default, 40-66% fewer tokens than JSON).
	// Optimized for LLM consumption.
	WireFormatToon WireFormat = iota

	// WireFormatJSON is standard JSON for compatibility.
	WireFormatJSON

	// WireFormatColumnar is raw columnar format for analytics.
	// More efficient for large result sets with projection pushdown.
	WireFormatColumnar
)

// String returns the string representation of WireFormat.
func (f WireFormat) String() string {
	switch f {
	case WireFormatToon:
		return "toon"
	case WireFormatJSON:
		return "json"
	case WireFormatColumnar:
		return "columnar"
	default:
		return "unknown"
	}
}

// ParseWireFormat parses a WireFormat from a string.
func ParseWireFormat(s string) (WireFormat, error) {
	switch s {
	case "toon":
		return WireFormatToon, nil
	case "json":
		return WireFormatJSON, nil
	case "columnar", "column":
		return WireFormatColumnar, nil
	default:
		return 0, &FormatConversionError{
			FromFormat: s,
			ToFormat:   "WireFormat",
			Reason:     fmt.Sprintf("Unknown format '%s'. Valid: toon, json, columnar", s),
		}
	}
}

// ContextFormat represents output format for LLM context packaging.
//
// These formats are optimized for readability and token efficiency
// when constructing prompts for language models.
type ContextFormat int

const (
	// ContextFormatToon is TOON format (default, token-efficient).
	// Structured data with minimal syntax overhead.
	ContextFormatToon ContextFormat = iota

	// ContextFormatJSON is JSON format.
	// Widely understood by LLMs, good for structured data.
	ContextFormatJSON

	// ContextFormatMarkdown is Markdown format.
	// Best for human-readable context with formatting.
	ContextFormatMarkdown
)

// String returns the string representation of ContextFormat.
func (f ContextFormat) String() string {
	switch f {
	case ContextFormatToon:
		return "toon"
	case ContextFormatJSON:
		return "json"
	case ContextFormatMarkdown:
		return "markdown"
	default:
		return "unknown"
	}
}

// ParseContextFormat parses a ContextFormat from a string.
func ParseContextFormat(s string) (ContextFormat, error) {
	switch s {
	case "toon":
		return ContextFormatToon, nil
	case "json":
		return ContextFormatJSON, nil
	case "markdown", "md":
		return ContextFormatMarkdown, nil
	default:
		return 0, &FormatConversionError{
			FromFormat: s,
			ToFormat:   "ContextFormat",
			Reason:     fmt.Sprintf("Unknown format '%s'. Valid: toon, json, markdown", s),
		}
	}
}

// CanonicalFormat represents canonical storage format (server-side only).
//
// This is the format used for internal storage and is optimized
// for storage efficiency and query performance.
type CanonicalFormat int

const (
	// CanonicalFormatToon is TOON canonical format.
	CanonicalFormatToon CanonicalFormat = iota
)

// String returns the string representation of CanonicalFormat.
func (f CanonicalFormat) String() string {
	return "toon"
}

// FormatCapabilities provides helpers to check format capabilities and conversions.
type FormatCapabilities struct{}

// WireToContext converts WireFormat to ContextFormat if compatible.
// Returns nil if the conversion is not possible.
func (FormatCapabilities) WireToContext(wire WireFormat) *ContextFormat {
	switch wire {
	case WireFormatToon:
		ctx := ContextFormatToon
		return &ctx
	case WireFormatJSON:
		ctx := ContextFormatJSON
		return &ctx
	default:
		return nil
	}
}

// ContextToWire converts ContextFormat to WireFormat if compatible.
// Returns nil if the conversion is not possible.
func (FormatCapabilities) ContextToWire(ctx ContextFormat) *WireFormat {
	switch ctx {
	case ContextFormatToon:
		wire := WireFormatToon
		return &wire
	case ContextFormatJSON:
		wire := WireFormatJSON
		return &wire
	default:
		return nil
	}
}

// SupportsRoundTrip checks if format supports round-trip: decode(encode(x)) = x.
func (FormatCapabilities) SupportsRoundTrip(fmt WireFormat) bool {
	return fmt == WireFormatToon || fmt == WireFormatJSON
}
