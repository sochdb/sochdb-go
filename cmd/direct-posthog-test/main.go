// Direct PostHog test - no ToonDB dependencies
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/posthog/posthog-go"
)

func main() {
	fmt.Println("Direct PostHog Go Client Test")
	fmt.Println("==============================\n")

	// Create PostHog client directly
	client, err := posthog.NewWithConfig(
		"phc_zf0hm6ZmPUJj1pM07Kigqvphh1ClhKX1NahRU4G0bfu",
		posthog.Config{
			Endpoint: "https://us.i.posthog.com",
		},
	)
	if err != nil {
		fmt.Printf("❌ Failed to create PostHog client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close() // CRITICAL: This flushes events

	fmt.Println("✓ PostHog client created")
	fmt.Println("✓ Endpoint: https://us.i.posthog.com")
	fmt.Println("✓ API Key: phc_zf0hm6ZmPUJj1pM07Kigqvphh1ClhKX1NahRU4G0bfu\n")

	// Send test event
	fmt.Println("Sending test event...")
	err = client.Enqueue(posthog.Capture{
		DistinctId: "test-user-golang",
		Event:      "golang_test_event",
		Properties: map[string]interface{}{
			"sdk_version":  "0.3.3",
			"sdk_language": "go",
			"test_type":    "direct_client",
			"$lib":         "posthog-go",
		},
	})
	if err != nil {
		fmt.Printf("❌ Failed to enqueue event: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ Event enqueued")
	fmt.Println("\nClosing client (this flushes events)...")

	// Close will flush - adding small delay first
	time.Sleep(500 * time.Millisecond)
	client.Close()

	fmt.Println("✓ Client closed, events flushed\n")
	fmt.Println("✅ Test complete!")
	fmt.Println("\nCheck PostHog dashboard:")
	fmt.Println("  https://us.i.posthog.com")
	fmt.Println("\nLook for event:")
	fmt.Println("  Event: golang_test_event")
	fmt.Println("  Distinct ID: test-user-golang")
	fmt.Println("  Library: posthog-go")
}
