package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"syscall"
	"time"

	"github.com/lmccay/volumez/pkg/backend"
)

// HTTPBackend implements the Backend interface for generic REST APIs
type HTTPBackend struct {
	client  *http.Client
	baseURL string
	headers map[string]string
}

// Config holds HTTP backend configuration
type Config struct {
	BaseURL string            `json:"base_url"`
	Headers map[string]string `json:"headers"` // Optional: custom headers (e.g., auth tokens)
	Timeout int               `json:"timeout"` // Timeout in seconds
}

func init() {
	backend.Register("http", New)
}

// New creates a new HTTP backend from configuration
func New(cfg map[string]interface{}) (backend.Backend, error) {
	baseURL, ok := cfg["base_url"].(string)
	if !ok || baseURL == "" {
		return nil, &backend.BackendError{
			Op:      "init",
			Backend: "http",
			Err:     fmt.Errorf("%w: base_url is required", backend.ErrInvalidConfig),
		}
	}

	timeout := 30 // Default timeout in seconds
	if t, ok := cfg["timeout"].(float64); ok {
		timeout = int(t)
	}

	headers := make(map[string]string)
	if h, ok := cfg["headers"].(map[string]interface{}); ok {
		for k, v := range h {
			if s, ok := v.(string); ok {
				headers[k] = s
			}
		}
	}

	return &HTTPBackend{
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
		baseURL: baseURL,
		headers: headers,
	}, nil
}

// buildURL constructs the full URL for a path
func (h *HTTPBackend) buildURL(p string) string {
	return h.baseURL + "/" + path.Clean(p)
}

// doRequest performs an HTTP request with configured headers
func (h *HTTPBackend) doRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	// Add custom headers
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// Stat returns metadata about a file or directory
func (h *HTTPBackend) Stat(ctx context.Context, p string) (*backend.FileInfo, error) {
	url := h.buildURL(p)

	resp, err := h.doRequest(ctx, http.MethodHead, url, nil)
	if err != nil {
		return nil, h.wrapError("stat", p, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, h.wrapError("stat", p, backend.ErrNotFound)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, h.wrapError("stat", p, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	// Parse metadata from headers
	info := &backend.FileInfo{
		Name:    path.Base(p),
		Size:    resp.ContentLength,
		Mode:    0644,
		ModTime: time.Now(),
		IsDir:   false,
		ETag:    resp.Header.Get("ETag"),
	}

	// Check if it's a directory based on Content-Type
	if contentType := resp.Header.Get("Content-Type"); contentType == "application/json" || contentType == "application/directory" {
		info.IsDir = true
		info.Mode = 0755 | syscall.S_IFDIR
	}

	// Parse Last-Modified header
	if lastMod := resp.Header.Get("Last-Modified"); lastMod != "" {
		if t, err := time.Parse(http.TimeFormat, lastMod); err == nil {
			info.ModTime = t
		}
	}

	return info, nil
}

// ReadFile reads the entire contents of a file
func (h *HTTPBackend) ReadFile(ctx context.Context, p string) ([]byte, error) {
	url := h.buildURL(p)

	resp, err := h.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, h.wrapError("read", p, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, h.wrapError("read", p, backend.ErrNotFound)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, h.wrapError("read", p, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, h.wrapError("read", p, err)
	}

	return data, nil
}

// ReadFileRange reads a range of bytes from a file
func (h *HTTPBackend) ReadFileRange(ctx context.Context, p string, offset int64, size int64) ([]byte, error) {
	url := h.buildURL(p)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, h.wrapError("read", p, err)
	}

	// Set Range header
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, offset+size-1))

	// Add custom headers
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, h.wrapError("read", p, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, h.wrapError("read", p, backend.ErrNotFound)
	}

	// Accept both 200 OK and 206 Partial Content
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return nil, h.wrapError("read", p, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, h.wrapError("read", p, err)
	}

	return data, nil
}

// ListDir returns the contents of a directory
func (h *HTTPBackend) ListDir(ctx context.Context, p string) ([]*backend.FileInfo, error) {
	url := h.buildURL(p)

	resp, err := h.doRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, h.wrapError("listdir", p, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, h.wrapError("listdir", p, backend.ErrNotFound)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, h.wrapError("listdir", p, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	// Expect JSON array of file entries
	var entries []struct {
		Name    string    `json:"name"`
		Size    int64     `json:"size"`
		IsDir   bool      `json:"is_dir"`
		ModTime time.Time `json:"mod_time"`
		Mode    uint32    `json:"mode"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, h.wrapError("listdir", p, err)
	}

	results := make([]*backend.FileInfo, len(entries))
	for i, entry := range entries {
		mode := entry.Mode
		if mode == 0 {
			if entry.IsDir {
				mode = 0755 | syscall.S_IFDIR
			} else {
				mode = 0644
			}
		}

		results[i] = &backend.FileInfo{
			Name:    entry.Name,
			Size:    entry.Size,
			Mode:    mode,
			ModTime: entry.ModTime,
			IsDir:   entry.IsDir,
		}
	}

	return results, nil
}

// WriteFile writes data to a file
func (h *HTTPBackend) WriteFile(ctx context.Context, p string, data []byte, mode uint32) error {
	return h.WriteFileStream(ctx, p, bytes.NewReader(data), mode)
}

// WriteFileStream writes data from a reader
func (h *HTTPBackend) WriteFileStream(ctx context.Context, p string, r io.Reader, mode uint32) error {
	url := h.buildURL(p)

	// Use PUT for idempotent create/update
	resp, err := h.doRequest(ctx, http.MethodPut, url, r)
	if err != nil {
		return h.wrapError("write", p, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return h.wrapError("write", p, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	return nil
}

// CreateDir creates a directory
func (h *HTTPBackend) CreateDir(ctx context.Context, p string, mode uint32) error {
	url := h.buildURL(p)

	// Use PUT or POST with special content type to indicate directory
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader([]byte{}))
	if err != nil {
		return h.wrapError("mkdir", p, err)
	}

	req.Header.Set("Content-Type", "application/directory")

	// Add custom headers
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return h.wrapError("mkdir", p, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return h.wrapError("mkdir", p, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	return nil
}

// Delete removes a file
func (h *HTTPBackend) Delete(ctx context.Context, p string) error {
	url := h.buildURL(p)

	resp, err := h.doRequest(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return h.wrapError("delete", p, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return h.wrapError("delete", p, backend.ErrNotFound)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return h.wrapError("delete", p, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	return nil
}

// DeleteDir removes a directory and its contents
func (h *HTTPBackend) DeleteDir(ctx context.Context, p string) error {
	// For HTTP APIs, this is the same as Delete
	// The server should handle recursive deletion
	return h.Delete(ctx, p)
}

// Exists checks if a path exists
func (h *HTTPBackend) Exists(ctx context.Context, p string) (bool, error) {
	url := h.buildURL(p)

	resp, err := h.doRequest(ctx, http.MethodHead, url, nil)
	if err != nil {
		return false, h.wrapError("exists", p, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}

	return false, h.wrapError("exists", p, fmt.Errorf("HTTP %d", resp.StatusCode))
}

// Rename moves/renames a file or directory
func (h *HTTPBackend) Rename(ctx context.Context, oldPath, newPath string) error {
	// Use custom MOVE method or POST with special headers
	url := h.buildURL(oldPath)

	req, err := http.NewRequestWithContext(ctx, "MOVE", url, nil)
	if err != nil {
		return h.wrapError("rename", oldPath, err)
	}

	req.Header.Set("Destination", h.buildURL(newPath))

	// Add custom headers
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return h.wrapError("rename", oldPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return h.wrapError("rename", oldPath, backend.ErrNotFound)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return h.wrapError("rename", oldPath, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	return nil
}

// UpdateMode changes file permissions
func (h *HTTPBackend) UpdateMode(ctx context.Context, p string, mode uint32) error {
	// Use PATCH to update metadata
	url := h.buildURL(p)

	body, _ := json.Marshal(map[string]interface{}{
		"mode": mode,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
	if err != nil {
		return h.wrapError("chmod", p, err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add custom headers
	for k, v := range h.headers {
		req.Header.Set(k, v)
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return h.wrapError("chmod", p, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return h.wrapError("chmod", p, fmt.Errorf("HTTP %d", resp.StatusCode))
	}

	return nil
}

// Close cleans up resources
func (h *HTTPBackend) Close() error {
	h.client.CloseIdleConnections()
	return nil
}

// wrapError wraps an error with backend context
func (h *HTTPBackend) wrapError(op, p string, err error) error {
	return &backend.BackendError{
		Op:      op,
		Path:    p,
		Backend: "http",
		Err:     err,
	}
}
