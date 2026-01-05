package database

import (
	"fmt"
)

// DBError represents a database operation error with context
type DBError struct {
	Operation string
	Err       error
}

// Error implements the error interface
func (e *DBError) Error() string {
	return fmt.Sprintf("database error in %s: %v", e.Operation, e.Err)
}

// Unwrap returns the underlying error
func (e *DBError) Unwrap() error {
	return e.Err
}

// NotFoundError represents a resource not found error
type NotFoundError struct {
	Resource string
	ID       interface{}
}

// Error implements the error interface
func (e *NotFoundError) Error() string {
	if e.ID != nil {
		return fmt.Sprintf("%s not found: %v", e.Resource, e.ID)
	}
	return fmt.Sprintf("%s not found", e.Resource)
}

// ValidationError represents a validation error
type ValidationError struct {
	Field  string
	Reason string
	Value  interface{}
}

// Error implements the error interface
func (e *ValidationError) Error() string {
	if e.Value != nil {
		return fmt.Sprintf("validation failed for field '%s': %s (value: %v)", e.Field, e.Reason, e.Value)
	}
	return fmt.Sprintf("validation failed for field '%s': %s", e.Field, e.Reason)
}

// WrapDBError wraps a database error with operation context
// This provides better error messages and makes debugging easier
func WrapDBError(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &DBError{
		Operation: operation,
		Err:       err,
	}
}

// NewNotFoundError creates a new NotFoundError
func NewNotFoundError(resource string) error {
	return &NotFoundError{
		Resource: resource,
	}
}

// NewNotFoundErrorWithID creates a new NotFoundError with an ID
func NewNotFoundErrorWithID(resource string, id interface{}) error {
	return &NotFoundError{
		Resource: resource,
		ID:       id,
	}
}

// NewValidationError creates a new ValidationError
func NewValidationError(field, reason string) error {
	return &ValidationError{
		Field:  field,
		Reason: reason,
	}
}

// NewValidationErrorWithValue creates a new ValidationError with a value
func NewValidationErrorWithValue(field, reason string, value interface{}) error {
	return &ValidationError{
		Field:  field,
		Reason: reason,
		Value:  value,
	}
}
