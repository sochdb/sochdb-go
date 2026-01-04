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
	"sync"

	"github.com/posthog/posthog-go"
)

const (
	posthogAPIKey = "phc_zf0hm6ZmPUJj1pM07Kigqvphh1ClhKX1NahRU4G0bfu"
	posthogHost   = "https://us.i.posthog.com"
)

var (
	analyticsClient      posthog.Client
	analyticsOnce        sync.Once
	analyticsEnabled     = true
	analyticsInitialized = false
)

// initAnalytics initializes the PostHog client (lazy, called once).
func initAnalytics() {
	analyticsOnce.Do(func() {
		// Check if analytics is disabled via environment variable
		if os.Getenv("TOONDB_DISABLE_ANALYTICS") == "true" {
			analyticsEnabled = false
			return
		}

		client, err := posthog.NewWithConfig(
			posthogAPIKey,
			posthog.Config{
				Endpoint: posthogHost,
			},
		)
		if err != nil {
			// Failed to initialize, disable analytics
			analyticsEnabled = false
			return
		}

		analyticsClient = client
		analyticsInitialized = true
	})
}

// trackEvent sends an event to PostHog with static metadata only.
// distinctId is generated from a hash of the SDK path or a random UUID.
func trackEvent(eventName string, properties map[string]interface{}) {
	initAnalytics()

	if !analyticsEnabled || !analyticsInitialized {
		return
	}

	// Add SDK metadata to all events
	if properties == nil {
		properties = make(map[string]interface{})
	}
	properties["sdk_version"] = Version
	properties["sdk_language"] = "go"
	properties["$lib"] = "posthog-go" // Identifies library in PostHog UI

	// Use anonymous distinct ID (we don't track users)
	// Generate a unique ID based on machine or use "anonymous"
	distinctID := "anonymous"

	// Enqueue event (non-blocking)
	// PostHog client will batch and send events automatically
	_ = analyticsClient.Enqueue(posthog.Capture{
		DistinctId: distinctID,
		Event:      eventName,
		Properties: properties,
	})
}

// trackDatabaseOpened tracks when a database is successfully opened.
func trackDatabaseOpened() {
	trackEvent("database_opened", nil)
}

// trackError tracks error events with error type and location.
func trackError(errorType, location string) {
	trackEvent(errorType, map[string]interface{}{
		"error_type": errorType,
		"location":   location,
	})
}

// flushAnalytics flushes any pending events (should be called before program exit)
func flushAnalytics() {
	if analyticsInitialized && analyticsClient != nil {
		// Close will flush all pending events
		_ = analyticsClient.Close()
		analyticsInitialized = false
	}
}
