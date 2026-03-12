package pathmap

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lmccay/volumez/pkg/backend"
)

// PathMapper maps filesystem paths to backend instances
type PathMapper struct {
	mu             sync.RWMutex
	mounts         []mount
	defaultBackend backend.Backend // Optional default backend
}

type mount struct {
	path    string
	backend backend.Backend
}

// New creates a new PathMapper
func New() *PathMapper {
	return &PathMapper{
		mounts:        make([]mount, 0),
		defaultBackend: nil,
	}
}

// AddMount registers a path prefix to a backend
// Paths should be absolute and cleaned (e.g., "/data", "/images")
func (pm *PathMapper) AddMount(path string, b backend.Backend) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Clean the path
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("mount path must be absolute: %s", path)
	}

	// Check for conflicts
	for _, m := range pm.mounts {
		if m.path == path {
			return fmt.Errorf("mount already exists for path: %s", path)
		}
	}

	// Insert in order (longest paths first for proper matching)
	pm.mounts = append(pm.mounts, mount{path: path, backend: b})
	pm.sortMounts()

	return nil
}

// SetDefault sets a default backend for unmapped paths
func (pm *PathMapper) SetDefault(b backend.Backend) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.defaultBackend = b
}

// Resolve returns the backend and relative path for a given absolute path
// Returns (backend, relativePath, error)
func (pm *PathMapper) Resolve(absPath string) (backend.Backend, string, error) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Clean the path
	absPath = filepath.Clean(absPath)

	// Special case: root
	if absPath == "/" {
		if pm.defaultBackend != nil {
			return pm.defaultBackend, "/", nil
		}
		// Return first mount if no default
		if len(pm.mounts) > 0 {
			return pm.mounts[0].backend, "/", nil
		}
		return nil, "", fmt.Errorf("no backend configured for path: %s", absPath)
	}

	// Find matching mount (mounts are sorted longest-first)
	for _, m := range pm.mounts {
		if absPath == m.path {
			// Exact match
			return m.backend, "/", nil
		}

		if strings.HasPrefix(absPath, m.path+"/") {
			// Path is under this mount
			relPath := strings.TrimPrefix(absPath, m.path)
			return m.backend, relPath, nil
		}
	}

	// No mount found
	if pm.defaultBackend != nil {
		return pm.defaultBackend, absPath, nil
	}

	return nil, "", fmt.Errorf("no backend configured for path: %s", absPath)
}

// ListMounts returns all configured mount points
func (pm *PathMapper) ListMounts() []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]string, len(pm.mounts))
	for i, m := range pm.mounts {
		result[i] = m.path
	}
	return result
}

// Close closes all backends
func (pm *PathMapper) Close() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var errs []error

	for _, m := range pm.mounts {
		if err := m.backend.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close backend for %s: %w", m.path, err))
		}
	}

	if pm.defaultBackend != nil {
		if err := pm.defaultBackend.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close default backend: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing backends: %v", errs)
	}

	return nil
}

// sortMounts sorts mounts by path length (longest first) for proper prefix matching
func (pm *PathMapper) sortMounts() {
	// Simple bubble sort (fine for small number of mounts)
	for i := 0; i < len(pm.mounts); i++ {
		for j := i + 1; j < len(pm.mounts); j++ {
			if len(pm.mounts[j].path) > len(pm.mounts[i].path) {
				pm.mounts[i], pm.mounts[j] = pm.mounts[j], pm.mounts[i]
			}
		}
	}
}
