# Volumez Architecture

This document provides a detailed overview of the Volumez architecture and design decisions.

## Overview

Volumez is a FUSE-based filesystem that translates POSIX filesystem operations into REST API calls and cloud storage SDK operations. The goal is to provide a transparent filesystem interface to cloud storage backends.

## Core Components

### 1. Backend Interface (`pkg/backend/`)

The backend interface defines a contract that all storage backends must implement:

```go
type Backend interface {
    // Read operations
    Stat(ctx context.Context, path string) (*FileInfo, error)
    ReadFile(ctx context.Context, path string) ([]byte, error)
    ReadFileRange(ctx context.Context, path string, offset int64, size int64) ([]byte, error)
    ListDir(ctx context.Context, path string) ([]*FileInfo, error)

    // Write operations
    WriteFile(ctx context.Context, path string, data []byte, mode uint32) error
    WriteFileStream(ctx context.Context, path string, r io.Reader, mode uint32) error
    CreateDir(ctx context.Context, path string, mode uint32) error

    // Delete operations
    Delete(ctx context.Context, path string) error
    DeleteDir(ctx context.Context, path string) error

    // Metadata operations
    Exists(ctx context.Context, path string) (bool, error)
    Rename(ctx context.Context, oldPath, newPath string) error
    UpdateMode(ctx context.Context, path string, mode uint32) error

    // Lifecycle
    Close() error
}
```

#### Design Rationale

- **Context-aware**: All operations accept `context.Context` for cancellation and timeouts
- **Streaming support**: `WriteFileStream` allows efficient handling of large files
- **Range reads**: `ReadFileRange` enables partial file reads (critical for performance)
- **Explicit operations**: Each operation maps clearly to REST verbs or SDK calls

### 2. Implemented Backends

#### S3 Backend (`pkg/backend/s3/`)

**Mapping to S3 Operations:**

| Filesystem Op | S3 API | Notes |
|--------------|--------|-------|
| Stat | HeadObject | Returns metadata without downloading content |
| ReadFile | GetObject | Downloads entire object |
| ReadFileRange | GetObject + Range header | Efficient partial reads |
| ListDir | ListObjectsV2 | Uses delimiter="/" for directory simulation |
| WriteFile | PutObject | Idempotent upload |
| CreateDir | PutObject (marker) | Creates empty object with "/" suffix |
| Delete | DeleteObject | Single object deletion |
| DeleteDir | DeleteObjects | Batch deletion of all keys with prefix |
| Rename | CopyObject + DeleteObject | Not atomic - limitation of S3 |

**Key Features:**
- Prefix support for mounting subdirectories
- S3-compatible endpoint support (MinIO, Wasabi, etc.)
- ETag tracking for optimistic concurrency

**Limitations:**
- Directories are simulated via prefix listings
- Rename is not atomic (copy + delete)
- No support for file locking
- Eventual consistency on some operations

#### HTTP Backend (`pkg/backend/http/`)

Generic REST API backend that follows a standard HTTP semantic:

| Filesystem Op | HTTP Method | Headers/Body |
|--------------|-------------|--------------|
| Stat | HEAD | Metadata in response headers |
| ReadFile | GET | Body contains file data |
| ReadFileRange | GET | Range header for partial reads |
| ListDir | GET | JSON array of file entries |
| WriteFile | PUT | Body contains file data |
| CreateDir | PUT | Content-Type: application/directory |
| Delete | DELETE | - |
| Rename | MOVE | Destination header |
| UpdateMode | PATCH | JSON body with mode |

**Expected API Contract:**

Directory listings should return JSON:
```json
[
  {
    "name": "file.txt",
    "size": 1024,
    "is_dir": false,
    "mod_time": "2024-01-01T00:00:00Z",
    "mode": 0644
  }
]
```

### 3. Path Mapping (`internal/pathmap/`)

The PathMapper routes filesystem paths to appropriate backends:

```
Input: /s3-data/documents/file.txt

PathMapper logic:
1. Clean path: /s3-data/documents/file.txt
2. Find longest matching mount: /s3-data → S3 Backend
3. Calculate relative path: /documents/file.txt
4. Return: (S3Backend, "/documents/file.txt", nil)
```

**Features:**
- Longest-prefix matching (more specific mounts take precedence)
- Thread-safe with RWMutex
- Optional default backend for unmapped paths
- Clean path handling

**Example Configuration:**
```
Mounts:
  /s3-data   → S3 Backend (bucket: my-bucket)
  /api       → HTTP Backend (base_url: api.example.com)
  /s3-data/public → S3 Backend (bucket: public-bucket)

Path Resolution:
  /s3-data/file.txt        → my-bucket
  /s3-data/public/img.jpg  → public-bucket (longer match wins)
  /api/users/1.json        → api.example.com/users/1.json
```

### 4. FUSE Filesystem Layer (`pkg/fuse/`)

Implements the FUSE interface using bazil.org/fuse library:

#### Node Types

**FS (Root)**
- Entry point for FUSE
- Returns root directory node

**Dir (Directory Node)**
- Implements: Node, NodeStringLookuper, HandleReadDirAller, NodeCreater, NodeMkdirer, NodeRemover, NodeRenamer
- Operations:
  - `Attr()`: Returns directory metadata
  - `Lookup()`: Finds child nodes
  - `ReadDirAll()`: Lists directory contents
  - `Create()`: Creates new files
  - `Mkdir()`: Creates directories
  - `Remove()`: Deletes files/directories
  - `Rename()`: Renames/moves files

**File (File Node)**
- Implements: Node, NodeOpener, NodeSetattrer
- Operations:
  - `Attr()`: Returns file metadata
  - `Open()`: Opens file for reading/writing
  - `Setattr()`: Modifies file attributes (size, mode)

**FileHandle (Open File Handle)**
- Implements: Handle, HandleReader, HandleWriter, HandleFlusher
- Operations:
  - `Read()`: Reads file data (uses ReadFileRange for efficiency)
  - `Write()`: Writes file data (currently write-through)
  - `Flush()`: Flushes buffered data

#### Write Strategy

Current implementation uses **write-through**:
1. Read entire file
2. Apply changes to buffer
3. Write entire file back

**Advantages:**
- Simple implementation
- Consistency guaranteed
- No cache management needed

**Disadvantages:**
- Poor performance for large files
- High bandwidth usage
- Not suitable for append operations

**Future Improvements:**
- Write buffering with dirty tracking
- Block-level writes
- Append optimization
- Multi-part upload for S3

### 5. Configuration System (`pkg/config/`)

JSON-based configuration with validation:

```json
{
  "mounts": [
    {
      "path": "/s3-data",
      "backend": "s3",
      "config": { "bucket": "my-bucket", "region": "us-east-1" }
    }
  ],
  "cache": {
    "enabled": true,
    "max_size": 1073741824,
    "ttl": 300,
    "metadata_ttl": 60
  },
  "debug": false
}
```

**Validation:**
- No duplicate mount paths
- Required backend parameters
- Path cleaning and normalization

## Data Flow

### Read Operation

```
1. Application calls read("/s3-data/file.txt")
   ↓
2. FUSE kernel module forwards to volumez
   ↓
3. FileHandle.Read() receives request
   ↓
4. PathMapper.Resolve("/s3-data/file.txt")
   → Returns: (S3Backend, "/file.txt")
   ↓
5. S3Backend.ReadFileRange(ctx, "/file.txt", offset, size)
   ↓
6. AWS SDK GetObject with Range header
   ↓
7. Data returned to application
```

### Write Operation

```
1. Application calls write("/s3-data/file.txt", data)
   ↓
2. FUSE kernel module forwards to volumez
   ↓
3. FileHandle.Write() receives request
   ↓
4. PathMapper.Resolve("/s3-data/file.txt")
   → Returns: (S3Backend, "/file.txt")
   ↓
5. Read existing file content (if exists)
   ↓
6. Merge new data at offset
   ↓
7. S3Backend.WriteFile(ctx, "/file.txt", mergedData)
   ↓
8. AWS SDK PutObject
   ↓
9. Confirm write to application
```

## Design Decisions

### Why FUSE?

**Advantages:**
- No application modifications needed
- Works with any POSIX-compliant tool
- Familiar filesystem semantics
- Kernel-level integration

**Disadvantages:**
- Performance overhead
- Platform-specific (Linux/macOS)
- Complex error handling
- Limited async operations

### Why Write-Through Instead of Write-Back?

**Current: Write-Through**
- Simpler to implement
- No cache consistency issues
- No data loss risk
- Immediate durability

**Future: Write-Back Cache**
Would improve performance but adds:
- Cache invalidation complexity
- Potential data loss on crashes
- Consistency challenges with concurrent access
- Memory management for dirty buffers

### Backend Registration Pattern

Backends self-register in `init()`:

```go
func init() {
    backend.Register("s3", New)
}
```

**Benefits:**
- Compile-time plugin system
- Easy to add new backends
- Loose coupling
- Clear factory pattern

### Error Mapping

Backend errors are wrapped with context:

```go
type BackendError struct {
    Op      string // "read", "write", etc.
    Path    string
    Backend string // "s3", "http"
    Err     error  // Underlying error
}
```

FUSE layer maps to syscall errors:
- `backend.ErrNotFound` → `syscall.ENOENT`
- `backend.ErrPermission` → `syscall.EPERM`
- Generic errors → `syscall.EIO`

## Performance Considerations

### Current Bottlenecks

1. **Write operations**: Read-modify-write for every update
2. **Metadata operations**: Network round-trip for every stat/readdir
3. **Small files**: High overhead relative to data size
4. **No caching**: Every read hits the backend

### Optimization Opportunities

1. **Metadata caching**
   - Cache directory listings
   - Cache file attributes
   - TTL-based invalidation

2. **Data caching**
   - LRU cache for frequently accessed files
   - Configurable cache size
   - Read-ahead for sequential access

3. **Write buffering**
   - Buffer writes in memory
   - Flush on close or timeout
   - Coalesce multiple writes

4. **Parallel operations**
   - Concurrent directory listings
   - Multi-part uploads for large files
   - Prefetch on sequential reads

## Security Considerations

### Current Implementation

- Relies on backend authentication (AWS credentials, API tokens)
- No additional access control
- File permissions stored but not enforced by backends
- No encryption at rest (depends on backend)

### Recommendations

1. **Credential management**
   - Use IAM roles when possible
   - Rotate API tokens regularly
   - Avoid storing credentials in config

2. **Access control**
   - Use FUSE permissions
   - Implement `-allow-other` carefully
   - Consider read-only mode for production

3. **Encryption**
   - Use S3 server-side encryption
   - TLS for HTTP backends
   - Consider client-side encryption layer

## Testing Strategy

### Unit Tests

- Backend interface implementations
- Path mapping logic
- Configuration parsing and validation

### Integration Tests

- Real S3 operations (using localstack/minio)
- HTTP backend against test server
- FUSE operations (requires FUSE support)

### Performance Tests

- Large file operations
- Many small files
- Concurrent access
- Cache effectiveness

## Future Enhancements

### Short Term

1. Implement caching layer
2. Add more backends (Azure, GCS)
3. Improve error messages
4. Add metrics/monitoring
5. Write buffering

### Long Term

1. Distributed caching
2. Multi-backend redundancy
3. Snapshot support
4. Compression support
5. Deduplication
6. WebDAV backend
7. SFTP backend

## References

- FUSE documentation: https://www.kernel.org/doc/html/latest/filesystems/fuse.html
- bazil.org/fuse: https://bazil.org/fuse/
- AWS S3 API: https://docs.aws.amazon.com/s3/
- REST API best practices: https://restfulapi.net/

## Contributing

When adding a new backend:

1. Create package under `pkg/backend/yourbackend/`
2. Implement `backend.Backend` interface
3. Register in `init()` function
4. Add configuration example
5. Document API requirements
6. Add tests
7. Update README

When modifying FUSE layer:

1. Consider performance implications
2. Test on both Linux and macOS
3. Handle errors properly (map to syscall errors)
4. Update architecture docs
5. Consider caching impact
