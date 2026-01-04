// Copyright 2025 Sushanth (https://github.com/sushanthpy)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package toondb

import (
	"os"
	"testing"
)

// TestAnalyticsInitialization tests that analytics can be initialized
func TestAnalyticsInitialization(t *testing.T) {
	// Test with analytics disabled
	os.Setenv("TOONDB_DISABLE_ANALYTICS", "true")
	defer os.Unsetenv("TOONDB_DISABLE_ANALYTICS")

	initAnalytics()

	if analyticsEnabled {
		t.Error("Analytics should be disabled when TOONDB_DISABLE_ANALYTICS=true")
	}
}

// TestAnalyticsEnabled tests analytics when enabled
func TestAnalyticsEnabled(t *testing.T) {
	os.Unsetenv("TOONDB_DISABLE_ANALYTICS")

	// Initialize analytics
	initAnalytics()

	// Just verify it doesn't panic - we can't easily test the actual initialization
	// in a unit test without mocking
	t.Log("Analytics initialization completed without panic")
}

// TestTrackEvent tests that tracking events doesn't panic
func TestTrackEvent(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("trackEvent panicked: %v", r)
		}
	}()

	// Test with analytics disabled
	os.Setenv("TOONDB_DISABLE_ANALYTICS", "true")
	defer os.Unsetenv("TOONDB_DISABLE_ANALYTICS")

	trackEvent("test_event", map[string]interface{}{
		"test_property": "test_value",
	})

	trackDatabaseOpened()
	trackError("connection_error", "test_location")
}

// TestErrorHelpers tests the error detection helper functions
func TestErrorHelpers(t *testing.T) {
	tests := []struct {
		name     string
		str      string
		substr   string
		expected bool
	}{
		{"exact match", "timeout", "timeout", true},
		{"contains", "connection timeout error", "timeout", true},
		{"case insensitive", "Connection TIMEOUT", "timeout", true},
		{"not found", "connection error", "timeout", false},
		{"empty", "", "timeout", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.str, tt.substr)
			if result != tt.expected {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.str, tt.substr, result, tt.expected)
			}
		})
	}
}

// TestIsTimeoutError tests timeout error detection
func TestIsTimeoutError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"timeout error", &ToonDBError{Message: "connection timeout"}, true},
		{"deadline error", &ToonDBError{Message: "context deadline exceeded"}, true},
		{"other error", &ToonDBError{Message: "connection failed"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTimeoutError(tt.err)
			if result != tt.expected {
				t.Errorf("isTimeoutError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
