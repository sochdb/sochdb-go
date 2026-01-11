package embedded_test

import (
	"os"
	"testing"

	"github.com/toondb/toondb-go/embedded"
)

func TestEmbeddedDatabase(t *testing.T) {
	dbPath := "./test_embedded_db"

	// Clean up
	defer os.RemoveAll(dbPath)

	// Open database
	db, err := embedded.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Test Put/Get
	t.Run("PutGet", func(t *testing.T) {
		err := db.Put([]byte("test_key"), []byte("test_value"))
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		value, err := db.Get([]byte("test_key"))
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if string(value) != "test_value" {
			t.Errorf("Expected 'test_value', got '%s'", string(value))
		}
	})

	// Test Path operations
	t.Run("PathOperations", func(t *testing.T) {
		err := db.PutPath("users/alice", []byte("Alice"))
		if err != nil {
			t.Fatalf("PutPath failed: %v", err)
		}

		value, err := db.GetPath("users/alice")
		if err != nil {
			t.Fatalf("GetPath failed: %v", err)
		}

		if string(value) != "Alice" {
			t.Errorf("Expected 'Alice', got '%s'", string(value))
		}
	})

	// Test Transactions
	t.Run("Transactions", func(t *testing.T) {
		err := db.WithTransaction(func(txn *embedded.Transaction) error {
			if err := txn.Put([]byte("txn_key1"), []byte("value1")); err != nil {
				return err
			}
			if err := txn.Put([]byte("txn_key2"), []byte("value2")); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			t.Fatalf("Transaction failed: %v", err)
		}

		value, err := db.Get([]byte("txn_key1"))
		if err != nil {
			t.Fatalf("Get after transaction failed: %v", err)
		}

		if string(value) != "value1" {
			t.Errorf("Expected 'value1', got '%s'", string(value))
		}
	})

	// Test Scan
	t.Run("Scan", func(t *testing.T) {
		// Add some keys
		db.Put([]byte("scan_1"), []byte("val1"))
		db.Put([]byte("scan_2"), []byte("val2"))
		db.Put([]byte("scan_3"), []byte("val3"))

		txn := db.Begin()
		defer txn.Abort()

		iter := txn.ScanPrefix([]byte("scan_"))
		defer iter.Close()

		count := 0
		for {
			key, value, ok := iter.Next()
			if !ok {
				break
			}
			t.Logf("Scanned: %s = %s", key, value)
			count++
		}

		if err := iter.Err(); err != nil {
			t.Fatalf("Scan error: %v", err)
		}

		if count != 3 {
			t.Errorf("Expected 3 keys, found %d", count)
		}
	})

	// Test Stats
	t.Run("Stats", func(t *testing.T) {
		stats, err := db.Stats()
		if err != nil {
			t.Fatalf("Stats failed: %v", err)
		}

		t.Logf("Stats: %+v", stats)
	})

	// Test Checkpoint
	t.Run("Checkpoint", func(t *testing.T) {
		lsn, err := db.Checkpoint()
		if err != nil {
			t.Fatalf("Checkpoint failed: %v", err)
		}

		t.Logf("Checkpoint LSN: %d", lsn)
	})
}

func TestConcurrentTransactions(t *testing.T) {
	dbPath := "./test_concurrent_db"
	defer os.RemoveAll(dbPath)

	db, err := embedded.Open(dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Initialize counter
	db.Put([]byte("counter"), []byte("0"))

	// Two concurrent transactions trying to update the same key
	done1 := make(chan error)
	done2 := make(chan error)

	go func() {
		done1 <- db.WithTransaction(func(txn *embedded.Transaction) error {
			// Read current value
			val, err := txn.Get([]byte("counter"))
			if err != nil {
				return err
			}

			// Simulate some work
			// time.Sleep(10 * time.Millisecond)

			// Write new value
			return txn.Put([]byte("counter"), []byte(string(val)+"_A"))
		})
	}()

	go func() {
		done2 <- db.WithTransaction(func(txn *embedded.Transaction) error {
			val, err := txn.Get([]byte("counter"))
			if err != nil {
				return err
			}

			// Simulate some work
			// time.Sleep(10 * time.Millisecond)

			return txn.Put([]byte("counter"), []byte(string(val)+"_B"))
		})
	}()

	err1 := <-done1
	err2 := <-done2

	// One should succeed, one might get SSI conflict
	if err1 != nil && err2 != nil {
		t.Logf("Both transactions errored (possible SSI): err1=%v, err2=%v", err1, err2)
	}

	// Check final value
	finalVal, _ := db.Get([]byte("counter"))
	t.Logf("Final counter value: %s", string(finalVal))
}
