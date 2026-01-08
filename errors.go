package toondb

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

// ToonDBError represents a general ToonDB error.
type ToonDBError struct {
	Op      string
	Message string
}

func (e *ToonDBError) Error() string {
	if e.Op != "" {
		return fmt.Sprintf("toondb %s: %s", e.Op, e.Message)
	}
	return e.Message
}
