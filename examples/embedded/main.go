package main

import (
	"fmt"
	"log"
	"os"

	"github.com/toondb/toondb-go/embedded"
)

func main() {
	dbPath := "./example_embedded_db"

	// Clean up old database
	os.RemoveAll(dbPath)

	fmt.Println("=== ToonDB Go SDK - Embedded Mode (FFI) ===\n")

	// Open database
	fmt.Println("Opening database...")
	db, err := embedded.Open(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// 1. Basic KV operations
	fmt.Println("\n1. Basic KV Operations:")
	err = db.Put([]byte("user:1"), []byte("Alice"))
	if err != nil {
		log.Fatalf("Put failed: %v", err)
	}
	err = db.Put([]byte("user:2"), []byte("Bob"))
	if err != nil {
		log.Fatalf("Put failed: %v", err)
	}

	user1, err := db.Get([]byte("user:1"))
	if err != nil {
		log.Fatalf("Get failed: %v", err)
	}
	fmt.Printf("  user:1 = %s\n", string(user1))

	// 2. Path operations
	fmt.Println("\n2. Path Operations:")
	err = db.PutPath("users/alice/email", []byte("alice@example.com"))
	if err != nil {
		log.Fatalf("PutPath failed: %v", err)
	}
	err = db.PutPath("users/alice/age", []byte("30"))
	if err != nil {
		log.Fatalf("PutPath failed: %v", err)
	}

	email, err := db.GetPath("users/alice/email")
	if err != nil {
		log.Fatalf("GetPath failed: %v", err)
	}
	fmt.Printf("  alice email = %s\n", string(email))

	// 3. Transactions
	fmt.Println("\n3. Transactions:")
	err = db.WithTransaction(func(txn *embedded.Transaction) error {
		if err := txn.Put([]byte("counter"), []byte("1")); err != nil {
			return err
		}
		if err := txn.Put([]byte("last_update"), []byte("now")); err != nil {
			return err
		}
		fmt.Println("  Transaction committed")
		return nil
	})
	if err != nil {
		log.Fatalf("Transaction failed: %v", err)
	}

	// 4. Scan operations
	fmt.Println("\n4. Scan Operations:")
	db.Put([]byte("scan:1"), []byte("val1"))
	db.Put([]byte("scan:2"), []byte("val2"))
	db.Put([]byte("scan:3"), []byte("val3"))

	txn := db.Begin()
	defer txn.Abort()

	iter := txn.ScanPrefix([]byte("scan:"))
	defer iter.Close()

	count := 0
	for {
		key, value, ok := iter.Next()
		if !ok {
			break
		}
		fmt.Printf("  %s = %s\n", string(key), string(value))
		count++
	}
	fmt.Printf("  Found %d keys\n", count)

	// 5. Stats
	fmt.Println("\n5. Database Stats:")
	stats, err := db.Stats()
	if err != nil {
		log.Fatalf("Stats failed: %v", err)
	}
	fmt.Printf("  Active transactions: %d\n", stats.ActiveTransactions)
	fmt.Printf("  Memtable size: %d bytes\n", stats.MemtableSizeBytes)

	// 6. Checkpoint
	fmt.Println("\n6. Checkpoint:")
	lsn, err := db.Checkpoint()
	if err != nil {
		log.Fatalf("Checkpoint failed: %v", err)
	}
	fmt.Printf("  Checkpoint LSN: %d\n", lsn)

	fmt.Println("\n✅ All operations completed successfully!")
}
