// Priority Queue API Example (v0.4.1)
//
// This example demonstrates:
// - Creating priority queues
// - Enqueueing tasks with priorities
// - Dequeuing and processing tasks
// - Task acknowledgment and retry logic
// - Queue statistics and monitoring

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	sochdb "github.com/sochdb/sochdb-go"
	"github.com/sochdb/sochdb-go/embedded"
)

func main() {
	// Clean up previous test data
	dbPath := filepath.Join(os.TempDir(), "queue-example-db")
	os.RemoveAll(dbPath)

	fmt.Println("🚀 SochDB Priority Queue API Example\n")

	// Open embedded database
	db, err := embedded.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()
	fmt.Println("✅ Database opened\n")

	// Example 1: Create a priority queue for job processing
	fmt.Println("📋 Creating priority queue for background jobs...")
	jobQueue := sochdb.NewPriorityQueue(db, "background-jobs", &sochdb.QueueConfig{
		Name:              "background-jobs",
		VisibilityTimeout: 30000, // 30 seconds
		MaxRetries:        3,
		DeadLetterQueue:   "failed-jobs",
	})
	fmt.Println("✅ Queue created: background-jobs\n")

	// Example 2: Enqueue tasks with different priorities
	fmt.Println("📝 Enqueueing tasks (lower priority = higher urgency)...")

	type TaskData struct {
		Name string                 `json:"name"`
		Data map[string]interface{} `json:"data"`
	}

	tasks := []struct {
		Priority int64
		Name     string
		Data     map[string]interface{}
	}{
		{1, "Critical: Process payment", map[string]interface{}{"orderId": "12345", "amount": 99.99}},
		{5, "Normal: Send email", map[string]interface{}{"to": "user@example.com", "template": "welcome"}},
		{3, "High: Update inventory", map[string]interface{}{"productId": "SKU-789", "quantity": 50}},
		{10, "Low: Generate report", map[string]interface{}{"reportType": "daily", "date": "2026-01-24"}},
		{2, "Urgent: Verify fraud", map[string]interface{}{"transactionId": "TX-99999", "score": 0.85}},
	}

	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		payload, _ := json.Marshal(task.Data)
		metadata := map[string]interface{}{
			"name":       task.Name,
			"enqueuedAt": time.Now().Format(time.RFC3339),
			"source":     "api-server",
		}

		taskID, err := jobQueue.Enqueue(task.Priority, payload, metadata)
		if err != nil {
			log.Printf("Error enqueueing task: %v", err)
			taskID = fmt.Sprintf("task_%d", time.Now().UnixNano())
		}
		taskIDs = append(taskIDs, taskID)
		fmt.Printf("  ✓ Priority %d: %s (ID: %s...)\n", task.Priority, task.Name, taskID[:12])
	}
	fmt.Printf("✅ Enqueued %d tasks\n\n", len(tasks))

	// Example 3: Worker simulation - Dequeue and process tasks
	fmt.Println("👷 Worker #1 processing tasks...\n")
	simulateWorker(jobQueue, "worker-1", 3)

	// Example 4: Show queue statistics
	fmt.Println("\n📊 Queue Statistics:")
	stats, err := jobQueue.Stats()
	if err != nil {
		log.Printf("Error getting stats: %v", err)
		stats = &sochdb.QueueStats{}
	}
	fmt.Printf("  Pending:        %d\n", stats.Pending)
	fmt.Printf("  Claimed:        %d\n", stats.Claimed)
	fmt.Printf("  Completed:      %d\n", stats.Completed)
	fmt.Printf("  Dead-lettered:  %d\n", stats.DeadLettered)
	fmt.Printf("  Total enqueued: %d\n", stats.TotalEnqueued)
	fmt.Printf("  Total dequeued: %d\n", stats.TotalDequeued)
	fmt.Println()

	// Example 5: Create a high-throughput queue for webhooks
	fmt.Println("🔗 Creating webhook delivery queue...")
	webhookQueue := sochdb.NewPriorityQueue(db, "webhooks", &sochdb.QueueConfig{
		Name:              "webhooks",
		VisibilityTimeout: 10000, // 10 seconds for quick retries
		MaxRetries:        5,
	})

	// Enqueue webhook deliveries
	webhooks := []struct {
		URL   string
		Event string
	}{
		{"https://api.example.com/webhooks/order", "order.created"},
		{"https://api.example.com/webhooks/user", "user.registered"},
		{"https://api.example.com/webhooks/payment", "payment.received"},
	}

	fmt.Println("📝 Enqueueing webhook deliveries...")
	for _, webhook := range webhooks {
		payload, _ := json.Marshal(webhook)
		metadata := map[string]interface{}{"event": webhook.Event}
		_, err := webhookQueue.Enqueue(1, payload, metadata)
		if err != nil {
			log.Printf("Error enqueueing webhook: %v", err)
		}
		fmt.Printf("  ✓ Webhook: %s → %s\n", webhook.Event, webhook.URL)
	}
	fmt.Printf("✅ Enqueued %d webhooks\n\n", len(webhooks))

	// Example 6: Scheduled tasks (future execution)
	fmt.Println("⏰ Creating scheduled task queue...")
	scheduledQueue := sochdb.NewPriorityQueue(db, "scheduled-tasks", &sochdb.QueueConfig{
		Name:              "scheduled-tasks",
		VisibilityTimeout: 60000, // 1 minute
	})

	// Schedule tasks for future execution
	scheduledTasks := []struct {
		Name      string
		ExecuteAt int64
	}{
		{"Daily backup", time.Now().Add(1 * time.Hour).UnixMilli()},          // 1 hour from now
		{"Weekly report", time.Now().Add(7 * 24 * time.Hour).UnixMilli()},    // 7 days from now
		{"Monthly cleanup", time.Now().Add(30 * 24 * time.Hour).UnixMilli()}, // 30 days from now
	}

	fmt.Println("📝 Scheduling future tasks...")
	for _, task := range scheduledTasks {
		payload, _ := json.Marshal(map[string]string{"name": task.Name})
		executeDate := time.UnixMilli(task.ExecuteAt)
		metadata := map[string]interface{}{
			"name":         task.Name,
			"scheduledFor": executeDate.Format(time.RFC3339),
		}
		_, err := scheduledQueue.Enqueue(task.ExecuteAt, payload, metadata)
		if err != nil {
			log.Printf("Error scheduling task: %v", err)
		}
		fmt.Printf("  ✓ %s → %s\n", task.Name, executeDate.Format(time.RFC1123))
	}
	fmt.Printf("✅ Scheduled %d tasks\n\n", len(scheduledTasks))

	fmt.Println("✨ Example completed successfully!\n")
	fmt.Println("Key Features Demonstrated:")
	fmt.Println("  ✓ Priority-based task ordering")
	fmt.Println("  ✓ Worker task claiming and processing")
	fmt.Println("  ✓ Acknowledgment and retry logic")
	fmt.Println("  ✓ Queue statistics and monitoring")
	fmt.Println("  ✓ Multiple queues for different purposes")
	fmt.Println("  ✓ Scheduled/delayed task execution")
	fmt.Println("  ✓ Dead letter queue for failed tasks")
}

// simulateWorker processes tasks from the queue
func simulateWorker(queue *sochdb.PriorityQueue, workerID string, maxTasks int) {
	processed := 0

	for processed < maxTasks {
		task, err := queue.Dequeue(workerID)
		if err != nil {
			log.Printf("Error dequeuing: %v", err)
			break
		}

		if task == nil {
			fmt.Printf("  ℹ️  No tasks available for %s\n", workerID)
			break
		}

		var taskData map[string]interface{}
		json.Unmarshal(task.Payload, &taskData)

		taskName := "Unknown task"
		if task.Metadata != nil {
			if name, ok := task.Metadata["name"].(string); ok {
				taskName = name
			}
		}

		fmt.Printf("  ⚙️  Processing: %s\n", taskName)
		fmt.Printf("     Priority: %d, Worker: %s\n", task.Priority, workerID)
		fmt.Printf("     Data: %v\n", taskData)

		// Simulate processing time
		time.Sleep(100 * time.Millisecond)

		// Randomly succeed or retry (80% success rate)
		success := rand.Float64() > 0.2

		if success {
			err = queue.Ack(task.TaskID)
			if err != nil {
				log.Printf("Error acknowledging task: %v", err)
			}
			fmt.Printf("  ✅ Completed task: %s...\n", task.TaskID[:12])
		} else {
			err = queue.Nack(task.TaskID)
			if err != nil {
				log.Printf("Error nack'ing task: %v", err)
			}
			fmt.Printf("  ⚠️  Task failed, will retry: %s...\n", task.TaskID[:12])
		}

		processed++
		fmt.Println()
	}

	fmt.Printf("✅ Worker %s processed %d tasks\n", workerID, processed)
}
