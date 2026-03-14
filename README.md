# Volumez

A FUSE-based filesystem that translates POSIX operations into REST API calls and SDK implementations for cloud storage backends.

## Overview

Volumez allows you to mount cloud storage (like S3) and REST APIs as local filesystems, enabling standard file operations to be transparently mapped to backend API calls:

- **Read** → GET requests
- **Write** → PUT/POST requests
- **Delete** → DELETE requests
- **List directory** → Collection/prefix listings

## Features

- **Pluggable backends**: S3, HTTP REST APIs, and extensible to other services
- **Path-based mapping**: Map different filesystem paths to different backends
- **Full read-write support**: Create, read, update, and delete files
- **Standard POSIX operations**: Works with any tool that uses file I/O
- **Configurable**: JSON-based configuration for easy setup

## Architecture

```
┌─────────────────────────────────────┐
│   Applications (cp, cat, ls, etc)   │
└─────────────────┬───────────────────┘
                  │ POSIX syscalls
┌─────────────────▼───────────────────┐
│         FUSE Filesystem Layer       │
└─────────────────┬───────────────────┘
                  │
┌─────────────────▼───────────────────┐
│       Path Mapper & Router          │
└─────┬───────────────────────┬───────┘
      │                       │
┌─────▼──────┐        ┌───────▼─────┐
│ S3 Backend │        │ HTTP Backend│
└─────┬──────┘        └──────┬──────┘
      │                      │
┌─────▼──────┐        ┌──────▼──────┐
│   AWS S3   │        │  REST API   │
└────────────┘        └─────────────┘
```

## Installation

### Homebrew (macOS)

```bash
brew tap open-tempest-labs/volumez
brew install volumez
```

The formula automatically installs macFUSE as a dependency. After installation, you may need to:
1. Allow the macFUSE kernel extension in System Settings > Privacy & Security
2. Restart your computer

### Pre-built Binaries

Download the latest release from the [releases page](https://github.com/open-tempest-labs/volumez/releases).

See [INSTALL.md](INSTALL.md) for detailed installation instructions for all platforms.

### Build from Source

**Prerequisites:**
- Go 1.21 or later
- FUSE libraries:
  - **Linux**: Install `libfuse-dev` or `fuse-devel` package
  - **macOS**: Install macFUSE from https://osxfuse.github.io/ or use `brew install --cask macfuse`
- AWS credentials (for S3 backend)

```bash
git clone https://github.com/open-tempest-labs/volumez.git
cd volumez
go build -o volumez ./cmd/volumez
```

**Note**: The build produces a ~15MB binary. This is normal for Go applications with embedded dependencies.

## Quick Start

### 1. Generate example configuration

```bash
./volumez -gen-config
```

This creates `volumez.example.json` with sample configurations.

### 2. Configure your backends

Edit `volumez.json`:

```json
{
  "mounts": [
    {
      "path": "/s3-data",
      "backend": "s3",
      "config": {
        "bucket": "my-bucket",
        "region": "us-east-1",
        "prefix": "data"
      }
    },
    {
      "path": "/api",
      "backend": "http",
      "config": {
        "base_url": "https://api.example.com/files",
        "headers": {
          "Authorization": "Bearer YOUR_TOKEN_HERE"
        },
        "timeout": 30
      }
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

### 3. Mount the filesystem

```bash
mkdir /tmp/mymount
./volumez -config volumez.json -mount /tmp/mymount
```

### 4. Use it like any filesystem

```bash
# List files
ls /tmp/mymount/s3-data

# Read a file
cat /tmp/mymount/s3-data/myfile.txt

# Write a file
echo "Hello World" > /tmp/mymount/s3-data/newfile.txt

# Copy files
cp localfile.txt /tmp/mymount/s3-data/

# Remove files
rm /tmp/mymount/s3-data/oldfile.txt
```

### 5. Unmount

Press `Ctrl+C` in the terminal running volumez, or:

```bash
umount /tmp/mymount
```

## Configuration

### Mount Configuration

Each mount maps a filesystem path to a backend:

```json
{
  "path": "/mount-path",     // Where to mount in the filesystem
  "backend": "backend-type", // Backend type: "s3", "http"
  "config": {                // Backend-specific configuration
    // ... backend options ...
  }
}
```

### S3 Backend Configuration

```json
{
  "backend": "s3",
  "config": {
    "bucket": "my-bucket",          // Required: S3 bucket name
    "region": "us-east-1",          // Required: AWS region
    "endpoint": "",                 // Optional: Custom endpoint (for S3-compatible)
    "prefix": "path/in/bucket"      // Optional: Prefix for all keys
  }
}
```

**AWS Authentication**: Uses standard AWS credential chain (environment variables, ~/.aws/credentials, IAM roles).

### HTTP Backend Configuration

```json
{
  "backend": "http",
  "config": {
    "base_url": "https://api.example.com/files",  // Required: Base API URL
    "headers": {                                   // Optional: Custom headers
      "Authorization": "Bearer token",
      "X-Custom-Header": "value"
    },
    "timeout": 30                                  // Optional: Timeout in seconds
  }
}
```

**Expected API Format**:

- `HEAD /path` - Check file existence, return metadata
- `GET /path` - Read file contents (supports Range header)
- `PUT /path` - Create/update file
- `DELETE /path` - Delete file
- `GET /path` (directory) - List directory as JSON array
- `MOVE /path` with `Destination` header - Rename/move file

### Cache Configuration

```json
{
  "cache": {
    "enabled": true,           // Enable/disable caching
    "max_size": 1073741824,   // Maximum cache size in bytes (1GB)
    "ttl": 300,               // Data cache TTL in seconds (5 min)
    "metadata_ttl": 60        // Metadata cache TTL in seconds (1 min)
  }
}
```

## Command-Line Options

```
Usage: volumez [options]

Options:
  -config string
        Path to configuration file (default "volumez.json")
  -mount string
        Mount point (required)
  -gen-config
        Generate example configuration and exit
  -debug
        Enable debug output
  -allow-other
        Allow other users to access the filesystem
  -allow-root
        Allow root to access the filesystem
  -read-only
        Mount filesystem as read-only
  -version
        Show version information
```

## Implementing Custom Backends

To add a new backend:

1. Create a new package under `pkg/backend/`
2. Implement the `backend.Backend` interface
3. Register your backend in the `init()` function

Example:

```go
package mybackend

import (
    "github.com/lmccay/volumez/pkg/backend"
)

type MyBackend struct {
    // ... your fields ...
}

func init() {
    backend.Register("mybackend", New)
}

func New(cfg map[string]interface{}) (backend.Backend, error) {
    // Parse config and create backend
    return &MyBackend{}, nil
}

// Implement all Backend interface methods...
```

## Limitations and Considerations

### POSIX vs Object Storage Impedance

- **No atomic renames**: Object stores don't support atomic rename (copy+delete used instead)
- **Eventual consistency**: Some operations may not be immediately visible
- **No file locking**: Distributed locking not supported
- **Performance**: Network latency affects all operations

### Performance

- **Best for**: Large sequential files, read-heavy workloads
- **Not ideal for**: Small random I/O, databases, high-frequency writes
- **Caching helps**: But adds complexity for consistency

### Recommended Use Cases

✅ Good:
- Accessing cloud storage with standard tools
- Reading/processing large files from S3
- Backup/archive access
- Development/testing with cloud data

❌ Not recommended:
- Database files
- High-performance applications
- Applications requiring strict POSIX compliance
- Real-time/low-latency requirements

## Project Structure

```
volumez/
├── cmd/
│   └── volumez/        # Main application
│       └── main.go
├── pkg/
│   ├── backend/        # Backend interface and implementations
│   │   ├── backend.go  # Core interface
│   │   ├── errors.go   # Error types
│   │   ├── s3/         # S3 backend
│   │   └── http/       # HTTP REST backend
│   ├── config/         # Configuration management
│   │   └── config.go
│   ├── fuse/           # FUSE filesystem implementation
│   │   └── fs.go
│   └── cache/          # Caching layer (future)
├── internal/
│   └── pathmap/        # Path to backend mapping
│       └── pathmap.go
└── README.md
```

## Development

### Building

```bash
go build -o volumez ./cmd/volumez
```

### Testing

```bash
go test ./...
```

### Running in Debug Mode

```bash
./volumez -config volumez.json -mount /tmp/test -debug
```

## Troubleshooting

### Mount fails with "permission denied"

- On Linux, you may need to add your user to the `fuse` group
- Use `-allow-other` flag if needed

### AWS credential errors

- Ensure AWS credentials are configured: `aws configure`
- Check IAM permissions for S3 bucket access
- Verify region is correct

### "Transport endpoint not connected"

- The mount may have crashed. Unmount first: `umount /tmp/mymount`
- Check logs for errors

### Slow performance

- Enable caching in configuration
- Use larger read/write buffers
- Consider if use case is appropriate (see Limitations)

### macOS file copy issues

Volumez disables extended attribute (xattr) support to ensure compatibility with macOS `cp` and Finder:

- **Why**: macOS `fcopyfile()` requires full xattr support; partial implementation causes copy failures
- **Impact**: Standard `cp` and Finder drag-and-drop work correctly
- **Trade-off**: macOS metadata (Finder tags, quarantine flags) is not preserved when copying to volumez mounts
- **Future**: Backend-specific xattr support can be enabled by uncommenting xattr interfaces in `pkg/fuse/fs.go`

## Contributing

Contributions welcome! Areas for improvement:

- Additional backends (Azure Blob, Google Cloud Storage, etc.)
- Better caching strategies
- Write buffering and optimization
- Tests and benchmarks
- Documentation

## License

Apache License 2.0 - See LICENSE file for details

## Credits

Built with:
- [bazil.org/fuse](https://bazil.org/fuse) - FUSE library for Go
- [AWS SDK for Go v2](https://aws.github.io/aws-sdk-go-v2/) - S3 integration
