package sochdb

import (
	"errors"
	"fmt"
)

// Common errors
var (
	// ErrClosed is returned when operating on a closed connection.
	ErrClosed = errors.New("connection closed")

	// ErrNotFound is returned when a key is not found.
	ErrNotFound = errors.New("key not found")

	// ErrInvalidResponse is returned when the server response is invalid.
	ErrInvalidResponse = errors.New("invalid server response")

	// ErrDatabaseLocked is returned when the database is locked by another process.
	ErrDatabaseLocked = errors.New("database locked by another process")

	// ErrLockTimeout is returned when timed out waiting for database lock.
	ErrLockTimeout = errors.New("timed out waiting for database lock")

	// ErrEpochMismatch is returned when WAL epoch mismatch detected.
	ErrEpochMismatch = errors.New("epoch mismatch: stale writer detected")

	// ErrSplitBrain is returned when split-brain condition detected.
	ErrSplitBrain = errors.New("split-brain: multiple active writers")
)

// ConnectionError represents a connection failure.
type ConnectionError struct {
	Address string
	Err     error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("failed to connect to %s: %v", e.Address, e.Err)
}

func (e *ConnectionError) Unwrap() error {
	return e.Err
}

// ProtocolError represents a protocol-level error.
type ProtocolError struct {
	Message string
}

func (e *ProtocolError) Error() string {
	return fmt.Sprintf("protocol error: %s", e.Message)
}

// ServerError represents an error returned by the server.
type ServerError struct {
	Message string
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("server error: %s", e.Message)
}

// TransactionError represents a transaction-related error.
type TransactionError struct {
	Message string
}

func (e *TransactionError) Error() string {
	return fmt.Sprintf("transaction error: %s", e.Message)
}

// SochDBError represents a general SochDB error.
type SochDBError struct {
	Op      string
	Message string
}

func (e *SochDBError) Error() string {
	if e.Op != "" {
		return fmt.Sprintf("sochdb %s: %s", e.Op, e.Message)
	}
	return e.Message
}

// ============================================================================
// Lock/Concurrency Errors (v0.4.1)
// ============================================================================

// LockError represents a lock-related error.
type LockError struct {
	Path        string
	Message     string
	Remediation string
}

func (e *LockError) Error() string {
	return fmt.Sprintf("lock error on %s: %s", e.Path, e.Message)
}

// DatabaseLockedError is returned when the database is locked by another process.
type DatabaseLockedError struct {
	Path      string
	HolderPID int
}

func (e *DatabaseLockedError) Error() string {
	if e.HolderPID > 0 {
		return fmt.Sprintf("database at '%s' is locked by process %d", e.Path, e.HolderPID)
	}
	return fmt.Sprintf("database at '%s' is locked", e.Path)
}

func (e *DatabaseLockedError) Is(target error) bool {
	return target == ErrDatabaseLocked
}

// LockTimeoutError is returned when timed out waiting for database lock.
type LockTimeoutError struct {
	Path        string
	TimeoutSecs float64
}

func (e *LockTimeoutError) Error() string {
	return fmt.Sprintf("timed out after %.1fs waiting for lock on '%s'", e.TimeoutSecs, e.Path)
}

func (e *LockTimeoutError) Is(target error) bool {
	return target == ErrLockTimeout
}

// EpochMismatchError is returned when WAL epoch mismatch detected.
type EpochMismatchError struct {
	Expected uint64
	Actual   uint64
}

func (e *EpochMismatchError) Error() string {
	return fmt.Sprintf("epoch mismatch: expected %d, found %d", e.Expected, e.Actual)
}

func (e *EpochMismatchError) Is(target error) bool {
	return target == ErrEpochMismatch
}

// SplitBrainError is returned when split-brain condition detected.
type SplitBrainError struct {
	Message string
}

func (e *SplitBrainError) Error() string {
	if e.Message == "" {
		return "split-brain detected: multiple active writers"
	}
	return fmt.Sprintf("split-brain: %s", e.Message)
}

func (e *SplitBrainError) Is(target error) bool {
	return target == ErrSplitBrain
}
