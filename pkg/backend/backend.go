package backend

import (
	"context"
	"io"
	"time"
)

// FileInfo represents metadata about a file or directory
type FileInfo struct {
	Name    string
	Size    int64
	Mode    uint32 // File permissions and type
	ModTime time.Time
	IsDir   bool
	ETag    string // For optimistic concurrency control
}

// Backend defines the interface that all storage backends must implement
// This maps filesystem operations to REST/API operations
type Backend interface {
	// Read operations (GET)

	// Stat returns metadata about a file or directory
	// Maps to: HEAD request or metadata API call
	Stat(ctx context.Context, path string) (*FileInfo, error)

	// ReadFile reads the entire contents of a file
	// Maps to: GET request
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// ReadFileRange reads a range of bytes from a file
	// Maps to: GET request with Range header
	ReadFileRange(ctx context.Context, path string, offset int64, size int64) ([]byte, error)

	// ListDir returns the contents of a directory
	// Maps to: GET/LIST request on collection
	ListDir(ctx context.Context, path string) ([]*FileInfo, error)

	// Write operations (POST/PUT)

	// WriteFile writes data to a file, creating or overwriting
	// Maps to: PUT request (idempotent)
	WriteFile(ctx context.Context, path string, data []byte, mode uint32) error

	// WriteFileStream writes data from a reader
	// Maps to: PUT request with streaming body
	WriteFileStream(ctx context.Context, path string, r io.Reader, mode uint32) error

	// CreateDir creates a directory
	// Maps to: PUT/POST to create collection
	CreateDir(ctx context.Context, path string, mode uint32) error

	// Delete operations (DELETE)

	// Delete removes a file or empty directory
	// Maps to: DELETE request
	Delete(ctx context.Context, path string) error

	// DeleteDir removes a directory and its contents
	// Maps to: Recursive DELETE operations
	DeleteDir(ctx context.Context, path string) error

	// Metadata operations

	// Exists checks if a path exists
	// Maps to: HEAD request
	Exists(ctx context.Context, path string) (bool, error)

	// Rename moves/renames a file or directory
	// Note: This is challenging for REST APIs - may require copy+delete
	Rename(ctx context.Context, oldPath, newPath string) error

	// UpdateMode changes file permissions
	// Maps to: PATCH/POST to update metadata
	UpdateMode(ctx context.Context, path string, mode uint32) error

	// Lifecycle

	// Close cleans up any resources held by the backend
	Close() error
}

// XattrBackend is an optional interface for backends that support extended attributes
// Backends that implement this can store xattrs in their native storage:
//   - S3: user-defined object metadata
//   - HTTP: custom headers
//   - Local filesystem: native xattrs
// Backends that don't implement this will silently ignore xattr operations
type XattrBackend interface {
	Backend

	// GetXattr retrieves an extended attribute value
	GetXattr(ctx context.Context, path string, name string) ([]byte, error)

	// SetXattr sets an extended attribute value
	SetXattr(ctx context.Context, path string, name string, value []byte) error

	// ListXattr lists all extended attribute names
	ListXattr(ctx context.Context, path string) ([]string, error)

	// RemoveXattr removes an extended attribute
	RemoveXattr(ctx context.Context, path string, name string) error
}

// BackendFactory creates a backend from configuration
type BackendFactory func(config map[string]interface{}) (Backend, error)

// Registry holds all registered backend factories
var registry = make(map[string]BackendFactory)

// Register adds a backend factory to the registry
func Register(name string, factory BackendFactory) {
	registry[name] = factory
}

// Create creates a backend by name using the provided configuration
func Create(name string, config map[string]interface{}) (Backend, error) {
	factory, ok := registry[name]
	if !ok {
		return nil, &BackendError{
			Op:      "create",
			Backend: name,
			Err:     ErrBackendNotFound,
		}
	}
	return factory(config)
}
