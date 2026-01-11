// Package embedded provides direct FFI access to ToonDB native library
//
// This package uses CGO to bind directly to libtoondb_storage for embedded,
// single-process deployments. No server required.
//
// Usage:
//
//	db, err := embedded.Open("./mydb")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer db.Close()
//
//	err = db.Put([]byte("key"), []byte("value"))
//	value, err := db.Get([]byte("key"))
package embedded

/*
#cgo CFLAGS: -I${SRCDIR}/../include
#cgo darwin LDFLAGS: -L${SRCDIR}/../../toondb/target/release -ltoondb_storage -Wl,-rpath,${SRCDIR}/../../toondb/target/release
#cgo linux LDFLAGS: -L${SRCDIR}/../../toondb/target/release -ltoondb_storage -Wl,-rpath,${SRCDIR}/../../toondb/target/release
#cgo windows LDFLAGS: -L${SRCDIR}/../../toondb/target/release -ltoondb_storage

#include <stdlib.h>
#include <stdint.h>

// Database handle
typedef void* DatabasePtr;

// Transaction handle
typedef struct {
    uint64_t txn_id;
    uint64_t snapshot_ts;
} TxnHandle;

// Commit result
typedef struct {
    uint64_t commit_ts;
    int32_t error_code;
} CommitResult;

// Storage stats
typedef struct {
    uint64_t memtable_size_bytes;
    uint64_t wal_size_bytes;
    size_t active_transactions;
    uint64_t min_active_snapshot;
    uint64_t last_checkpoint_lsn;
} StorageStats;

// Database lifecycle
extern DatabasePtr toondb_open(const char* path);
extern void toondb_close(DatabasePtr db);

// Transaction API
extern TxnHandle toondb_begin_txn(DatabasePtr db);
extern CommitResult toondb_commit(DatabasePtr db, TxnHandle txn);
extern int toondb_abort(DatabasePtr db, TxnHandle txn);

// Key-Value API
extern int toondb_put(DatabasePtr db, TxnHandle txn, const uint8_t* key, size_t key_len, const uint8_t* val, size_t val_len);
extern int toondb_get(DatabasePtr db, TxnHandle txn, const uint8_t* key, size_t key_len, uint8_t** val_out, size_t* len_out);
extern int toondb_delete(DatabasePtr db, TxnHandle txn, const uint8_t* key, size_t key_len);
extern void toondb_free_bytes(uint8_t* ptr, size_t len);

// Path API
extern int toondb_put_path(DatabasePtr db, TxnHandle txn, const char* path, const uint8_t* val, size_t val_len);
extern int toondb_get_path(DatabasePtr db, TxnHandle txn, const char* path, uint8_t** val_out, size_t* len_out);

// Scan API
typedef void* ScanIteratorPtr;
extern ScanIteratorPtr toondb_scan_prefix(DatabasePtr db, TxnHandle txn, const uint8_t* prefix, size_t prefix_len);
extern int toondb_scan_next(ScanIteratorPtr iter, uint8_t** key_out, size_t* key_len_out, uint8_t** val_out, size_t* val_len_out);
extern void toondb_scan_free(ScanIteratorPtr iter);

// Checkpoint & Stats
extern uint64_t toondb_checkpoint(DatabasePtr db);
extern StorageStats toondb_stats(DatabasePtr db);

// Index policy
extern int toondb_set_table_index_policy(DatabasePtr db, const char* table, uint8_t policy);
extern uint8_t toondb_get_table_index_policy(DatabasePtr db, const char* table);
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

// Database represents an embedded ToonDB instance with direct FFI access
type Database struct {
	ptr  C.DatabasePtr
	path string
}

// Open opens a ToonDB database at the specified path
//
// The database is created if it doesn't exist. Returns an error if the
// database cannot be opened.
func Open(path string) (*Database, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	ptr := C.toondb_open(cpath)
	if ptr == nil {
		return nil, fmt.Errorf("failed to open database at %s", path)
	}

	return &Database{
		ptr:  ptr,
		path: path,
	}, nil
}

// Close closes the database and releases all resources
func (db *Database) Close() error {
	if db.ptr != nil {
		C.toondb_close(db.ptr)
		db.ptr = nil
	}
	return nil
}

// Put stores a key-value pair (auto-transaction)
func (db *Database) Put(key, value []byte) error {
	txn := db.Begin()
	defer txn.Abort()

	if err := txn.Put(key, value); err != nil {
		return err
	}

	return txn.Commit()
}

// Get retrieves a value by key (auto-transaction)
func (db *Database) Get(key []byte) ([]byte, error) {
	txn := db.Begin()
	defer txn.Abort()

	value, err := txn.Get(key)
	if err != nil {
		return nil, err
	}

	_ = txn.Commit()
	return value, nil
}

// Delete removes a key (auto-transaction)
func (db *Database) Delete(key []byte) error {
	txn := db.Begin()
	defer txn.Abort()

	if err := txn.Delete(key); err != nil {
		return err
	}

	return txn.Commit()
}

// PutPath stores a value at a path (auto-transaction)
func (db *Database) PutPath(path string, value []byte) error {
	txn := db.Begin()
	defer txn.Abort()

	if err := txn.PutPath(path, value); err != nil {
		return err
	}

	return txn.Commit()
}

// GetPath retrieves a value by path (auto-transaction)
func (db *Database) GetPath(path string) ([]byte, error) {
	txn := db.Begin()
	defer txn.Abort()

	value, err := txn.GetPath(path)
	if err != nil {
		return nil, err
	}

	_ = txn.Commit()
	return value, nil
}

// Begin starts a new transaction
func (db *Database) Begin() *Transaction {
	handle := C.toondb_begin_txn(db.ptr)
	return &Transaction{
		db:        db,
		handle:    handle,
		committed: false,
		aborted:   false,
	}
}

// WithTransaction executes a function within a transaction
//
// The transaction is automatically committed if the function returns nil,
// or aborted if it returns an error.
func (db *Database) WithTransaction(fn func(*Transaction) error) error {
	txn := db.Begin()
	defer txn.Abort()

	if err := fn(txn); err != nil {
		return err
	}

	return txn.Commit()
}

// Checkpoint forces a checkpoint and returns the LSN
func (db *Database) Checkpoint() (uint64, error) {
	lsn := C.toondb_checkpoint(db.ptr)
	return uint64(lsn), nil
}

// Stats returns storage statistics
func (db *Database) Stats() (*Stats, error) {
	cstats := C.toondb_stats(db.ptr)

	return &Stats{
		MemtableSizeBytes:  uint64(cstats.memtable_size_bytes),
		WalSizeBytes:       uint64(cstats.wal_size_bytes),
		ActiveTransactions: uint(cstats.active_transactions),
		MinActiveSnapshot:  uint64(cstats.min_active_snapshot),
		LastCheckpointLsn:  uint64(cstats.last_checkpoint_lsn),
	}, nil
}

// SetTableIndexPolicy sets the index policy for a table
func (db *Database) SetTableIndexPolicy(table string, policy IndexPolicy) error {
	ctable := C.CString(table)
	defer C.free(unsafe.Pointer(ctable))

	result := C.toondb_set_table_index_policy(db.ptr, ctable, C.uint8_t(policy))
	if result != 0 {
		return fmt.Errorf("failed to set index policy for table %s", table)
	}

	return nil
}

// GetTableIndexPolicy gets the index policy for a table
func (db *Database) GetTableIndexPolicy(table string) (IndexPolicy, error) {
	ctable := C.CString(table)
	defer C.free(unsafe.Pointer(ctable))

	policy := C.toondb_get_table_index_policy(db.ptr, ctable)
	if policy == 255 {
		return 0, fmt.Errorf("failed to get index policy for table %s", table)
	}

	return IndexPolicy(policy), nil
}

// Stats represents database storage statistics
type Stats struct {
	MemtableSizeBytes  uint64
	WalSizeBytes       uint64
	ActiveTransactions uint
	MinActiveSnapshot  uint64
	LastCheckpointLsn  uint64
}

// IndexPolicy represents the indexing strategy for a table
type IndexPolicy uint8

const (
	IndexWriteOptimized IndexPolicy = 0 // O(1) insert, O(N) scan
	IndexBalanced       IndexPolicy = 1 // O(1) amortized insert, O(log K) scan
	IndexScanOptimized  IndexPolicy = 2 // O(log N) insert, O(log N + K) scan
	IndexAppendOnly     IndexPolicy = 3 // O(1) insert, O(N) scan (time-series)
)

// Transaction represents a database transaction
type Transaction struct {
	db        *Database
	handle    C.TxnHandle
	committed bool
	aborted   bool
}

// ID returns the transaction ID
func (txn *Transaction) ID() uint64 {
	return uint64(txn.handle.txn_id)
}

// SnapshotTS returns the snapshot timestamp
func (txn *Transaction) SnapshotTS() uint64 {
	return uint64(txn.handle.snapshot_ts)
}

// Put stores a key-value pair within the transaction
func (txn *Transaction) Put(key, value []byte) error {
	if err := txn.ensureActive(); err != nil {
		return err
	}

	var keyPtr, valPtr *C.uint8_t
	if len(key) > 0 {
		keyPtr = (*C.uint8_t)(unsafe.Pointer(&key[0]))
	}
	if len(value) > 0 {
		valPtr = (*C.uint8_t)(unsafe.Pointer(&value[0]))
	}

	result := C.toondb_put(
		txn.db.ptr,
		txn.handle,
		keyPtr,
		C.size_t(len(key)),
		valPtr,
		C.size_t(len(value)),
	)

	if result != 0 {
		return errors.New("failed to put value")
	}

	return nil
}

// Get retrieves a value by key within the transaction
func (txn *Transaction) Get(key []byte) ([]byte, error) {
	if err := txn.ensureActive(); err != nil {
		return nil, err
	}

	var keyPtr *C.uint8_t
	if len(key) > 0 {
		keyPtr = (*C.uint8_t)(unsafe.Pointer(&key[0]))
	}

	var valOut *C.uint8_t
	var lenOut C.size_t

	result := C.toondb_get(
		txn.db.ptr,
		txn.handle,
		keyPtr,
		C.size_t(len(key)),
		&valOut,
		&lenOut,
	)

	if result == 1 {
		// Not found
		return nil, nil
	} else if result != 0 {
		return nil, errors.New("failed to get value")
	}

	if valOut == nil || lenOut == 0 {
		return nil, nil
	}

	// Copy data to Go slice
	value := C.GoBytes(unsafe.Pointer(valOut), C.int(lenOut))

	// Free Rust memory
	C.toondb_free_bytes(valOut, lenOut)

	return value, nil
}

// Delete removes a key within the transaction
func (txn *Transaction) Delete(key []byte) error {
	if err := txn.ensureActive(); err != nil {
		return err
	}

	var keyPtr *C.uint8_t
	if len(key) > 0 {
		keyPtr = (*C.uint8_t)(unsafe.Pointer(&key[0]))
	}

	result := C.toondb_delete(
		txn.db.ptr,
		txn.handle,
		keyPtr,
		C.size_t(len(key)),
	)

	if result != 0 {
		return errors.New("failed to delete key")
	}

	return nil
}

// PutPath stores a value at a path within the transaction
func (txn *Transaction) PutPath(path string, value []byte) error {
	if err := txn.ensureActive(); err != nil {
		return err
	}

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var valPtr *C.uint8_t
	if len(value) > 0 {
		valPtr = (*C.uint8_t)(unsafe.Pointer(&value[0]))
	}

	result := C.toondb_put_path(
		txn.db.ptr,
		txn.handle,
		cpath,
		valPtr,
		C.size_t(len(value)),
	)

	if result != 0 {
		return errors.New("failed to put path")
	}

	return nil
}

// GetPath retrieves a value by path within the transaction
func (txn *Transaction) GetPath(path string) ([]byte, error) {
	if err := txn.ensureActive(); err != nil {
		return nil, err
	}

	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	var valOut *C.uint8_t
	var lenOut C.size_t

	result := C.toondb_get_path(
		txn.db.ptr,
		txn.handle,
		cpath,
		&valOut,
		&lenOut,
	)

	if result == 1 {
		return nil, nil
	} else if result != 0 {
		return nil, errors.New("failed to get path")
	}

	if valOut == nil || lenOut == 0 {
		return nil, nil
	}

	value := C.GoBytes(unsafe.Pointer(valOut), C.int(lenOut))
	C.toondb_free_bytes(valOut, lenOut)

	return value, nil
}

// ScanPrefix returns an iterator for keys with the given prefix
func (txn *Transaction) ScanPrefix(prefix []byte) *ScanIterator {
	if err := txn.ensureActive(); err != nil {
		return &ScanIterator{err: err}
	}

	var prefixPtr *C.uint8_t
	if len(prefix) > 0 {
		prefixPtr = (*C.uint8_t)(unsafe.Pointer(&prefix[0]))
	}

	iterPtr := C.toondb_scan_prefix(
		txn.db.ptr,
		txn.handle,
		prefixPtr,
		C.size_t(len(prefix)),
	)

	if iterPtr == nil {
		return &ScanIterator{err: errors.New("failed to create scan iterator")}
	}

	return &ScanIterator{
		ptr: iterPtr,
		err: nil,
	}
}

// Commit commits the transaction
func (txn *Transaction) Commit() error {
	if err := txn.ensureActive(); err != nil {
		return err
	}

	result := C.toondb_commit(txn.db.ptr, txn.handle)

	if result.error_code != 0 {
		if result.error_code == -2 {
			return errors.New("SSI conflict: transaction aborted due to serialization failure")
		}
		return errors.New("failed to commit transaction")
	}

	txn.committed = true
	return nil
}

// Abort aborts the transaction
func (txn *Transaction) Abort() error {
	if txn.committed || txn.aborted {
		return nil
	}

	C.toondb_abort(txn.db.ptr, txn.handle)
	txn.aborted = true
	return nil
}

func (txn *Transaction) ensureActive() error {
	if txn.committed {
		return errors.New("transaction already committed")
	}
	if txn.aborted {
		return errors.New("transaction already aborted")
	}
	return nil
}

// ScanIterator iterates over scan results
type ScanIterator struct {
	ptr C.ScanIteratorPtr
	err error
}

// Next returns the next key-value pair, or false if done
func (iter *ScanIterator) Next() ([]byte, []byte, bool) {
	if iter.err != nil || iter.ptr == nil {
		return nil, nil, false
	}

	var keyOut, valOut *C.uint8_t
	var keyLen, valLen C.size_t

	result := C.toondb_scan_next(iter.ptr, &keyOut, &keyLen, &valOut, &valLen)

	if result == 1 {
		// End of scan
		return nil, nil, false
	} else if result != 0 {
		iter.err = errors.New("scan iteration failed")
		return nil, nil, false
	}

	key := C.GoBytes(unsafe.Pointer(keyOut), C.int(keyLen))
	value := C.GoBytes(unsafe.Pointer(valOut), C.int(valLen))

	C.toondb_free_bytes(keyOut, keyLen)
	C.toondb_free_bytes(valOut, valLen)

	return key, value, true
}

// Close closes the iterator and releases resources
func (iter *ScanIterator) Close() {
	if iter.ptr != nil {
		C.toondb_scan_free(iter.ptr)
		iter.ptr = nil
	}
}

// Err returns any error that occurred during iteration
func (iter *ScanIterator) Err() error {
	return iter.err
}
