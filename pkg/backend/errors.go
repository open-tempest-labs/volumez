package backend

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound indicates the requested path does not exist
	ErrNotFound = errors.New("not found")

	// ErrExists indicates the path already exists
	ErrExists = errors.New("already exists")

	// ErrNotEmpty indicates a directory is not empty
	ErrNotEmpty = errors.New("directory not empty")

	// ErrIsDirectory indicates the path is a directory when a file was expected
	ErrIsDirectory = errors.New("is a directory")

	// ErrNotDirectory indicates the path is a file when a directory was expected
	ErrNotDirectory = errors.New("not a directory")

	// ErrBackendNotFound indicates the requested backend type is not registered
	ErrBackendNotFound = errors.New("backend not found")

	// ErrPermission indicates a permission error
	ErrPermission = errors.New("permission denied")

	// ErrInvalidConfig indicates invalid backend configuration
	ErrInvalidConfig = errors.New("invalid configuration")
)

// BackendError provides context for backend errors
type BackendError struct {
	Op      string // Operation being performed
	Path    string // Path involved (if applicable)
	Backend string // Backend name
	Err     error  // Underlying error
}

func (e *BackendError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s %s: %s: %v", e.Backend, e.Op, e.Path, e.Err)
	}
	return fmt.Sprintf("%s %s: %v", e.Backend, e.Op, e.Err)
}

func (e *BackendError) Unwrap() error {
	return e.Err
}
