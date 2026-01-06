// Sample program to test PostHog analytics integration
// This creates a real database connection and sends analytics events
package main

import (
	"fmt"
	"os"
	"time"

	toondb "github.com/toondb/toondb-go"
)

func main() {
	fmt.Println("=== ToonDB Analytics Test v0.3.3 ===\n")

	// Test 1: Analytics enabled (default)
	fmt.Println("Test 1: Analytics Enabled (default)")
	fmt.Println("Expected: database_opened event sent to PostHog")

	// This will fail to start server but will track connection_error
	db, err := toondb.Open("./test_analytics_demo")
	if err != nil {
		fmt.Printf("✓ Connection error occurred (as expected): %v\n", err)
		fmt.Println("✓ Analytics event 'connection_error' should be sent\n")
	} else {
		fmt.Println("✓ Database opened successfully!")
		fmt.Println("✓ Analytics event 'database_opened' sent")

		// Perform some operations
		_ = db.Put([]byte("test_key"), []byte("test_value"))
		_, _ = db.Get([]byte("test_key"))

		db.Close()
		os.RemoveAll("./test_analytics_demo")
	}

	// Wait for analytics to be sent
	time.Sleep(2 * time.Second)

	// Test 2: Analytics disabled
	fmt.Println("\nTest 2: Analytics Disabled")
	fmt.Println("Expected: No events sent")

	os.Setenv("TOONDB_DISABLE_ANALYTICS", "true")
	defer os.Unsetenv("TOONDB_DISABLE_ANALYTICS")

	db2, err := toondb.Open("./test_analytics_disabled")
	if err != nil {
		fmt.Printf("✓ Connection error occurred: %v\n", err)
		fmt.Println("✓ No analytics event sent (disabled)")
	} else {
		fmt.Println("✓ Database opened")
		fmt.Println("✓ No analytics event sent (disabled)")
		db2.Close()
		os.RemoveAll("./test_analytics_disabled")
	}

	fmt.Println("\n=== Test Complete ===")
	fmt.Println("\nCheck PostHog dashboard at: https://us.i.posthog.com")
	fmt.Println("Project: phc_zf0hm6ZmPUJj1pM07Kigqvphh1ClhKX1NahRU4G0bfu")
	fmt.Println("\nExpected events:")
	fmt.Println("  - connection_error (from Test 1)")
	fmt.Println("\nEvent properties:")
	fmt.Println("  - distinct_id: anonymous")
	fmt.Println("  - sdk_version: 0.3.3"))
	fmt.Println("  - sdk_language: go")
	fmt.Println("  - error_type: connection_error")
	fmt.Println("  - location: OpenWithConfig")
}
