package fuse

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"syscall"

	"github.com/hanwen/go-fuse/v2/fs"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/lmccay/volumez/internal/pathmap"
	"github.com/lmccay/volumez/pkg/backend"
)

// FS is the root node of the filesystem
type FS struct {
	fs.Inode
	mapper *pathmap.PathMapper
	debug  bool
	uid    uint32
	gid    uint32
}

var _ = (fs.InodeEmbedder)((*FS)(nil))
var _ = (fs.NodeReaddirer)((*FS)(nil))
var _ = (fs.NodeLookuper)((*FS)(nil))
var _ = (fs.NodeCreater)((*FS)(nil))
var _ = (fs.NodeMkdirer)((*FS)(nil))
var _ = (fs.NodeUnlinker)((*FS)(nil))
var _ = (fs.NodeRmdirer)((*FS)(nil))
var _ = (fs.NodeRenamer)((*FS)(nil))
var _ = (fs.NodeGetattrer)((*FS)(nil))
// Xattr support is disabled to avoid macOS fcopyfile() issues
// Uncomment these to enable backend-specific xattr support
// var _ = (fs.NodeGetxattrer)((*FS)(nil))
// var _ = (fs.NodeSetxattrer)((*FS)(nil))
// var _ = (fs.NodeListxattrer)((*FS)(nil))
// var _ = (fs.NodeRemovexattrer)((*FS)(nil))

// NewFS creates a new FUSE filesystem
func NewFS(mapper *pathmap.PathMapper, debug bool) fs.InodeEmbedder {
	return &FS{
		mapper: mapper,
		debug:  debug,
		uid:    uint32(syscall.Getuid()),
		gid:    uint32(syscall.Getgid()),
	}
}

// Getattr returns attributes for the root directory
func (f *FS) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	out.Mode = 0755 | syscall.S_IFDIR
	out.Uid = f.uid
	out.Gid = f.gid
	return 0
}

// Readdir lists the root directory (mount points)
func (f *FS) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	mounts := f.mapper.ListMounts()
	entries := make([]fuse.DirEntry, 0, len(mounts))

	for _, mountPath := range mounts {
		name := strings.TrimPrefix(mountPath, "/")
		if name == "" {
			continue
		}

		entries = append(entries, fuse.DirEntry{
			Name: name,
			Mode: syscall.S_IFDIR,
		})
	}

	return fs.NewListDirStream(entries), 0
}

// Lookup finds a child node in the root
func (f *FS) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childPath := "/" + name

	b, relPath, err := f.mapper.Resolve(childPath)
	if err != nil {
		return nil, syscall.ENOENT
	}

	info, err := b.Stat(ctx, relPath)
	if err != nil {
		if isNotFound(err) {
			return nil, syscall.ENOENT
		}
		return nil, syscall.EIO
	}

	var node fs.InodeEmbedder
	if info.IsDir {
		node = &Dir{
			mapper: f.mapper,
			path:   childPath,
			rootFS: f,
		}
		out.Mode = uint32(info.Mode) | syscall.S_IFDIR
	} else {
		node = &File{
			mapper: f.mapper,
			path:   childPath,
			rootFS: f,
		}
		out.Mode = uint32(info.Mode)
	}

	out.Size = uint64(info.Size)
	out.Uid = f.uid
	out.Gid = f.gid
	out.SetTimes(nil, &info.ModTime, nil)

	child := f.NewInode(ctx, node, fs.StableAttr{
		Mode: info.Mode,
	})

	return child, 0
}

// Create is not supported on root
func (f *FS) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return nil, nil, 0, syscall.EACCES
}

// Mkdir is not supported on root
func (f *FS) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	return nil, syscall.EACCES
}

// Unlink is not supported on root
func (f *FS) Unlink(ctx context.Context, name string) syscall.Errno {
	return syscall.EACCES
}

// Rmdir is not supported on root
func (f *FS) Rmdir(ctx context.Context, name string) syscall.Errno {
	return syscall.EACCES
}

// Rename is not supported on root
func (f *FS) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	return syscall.EACCES
}

// Dir represents a directory node
type Dir struct {
	fs.Inode
	mapper *pathmap.PathMapper
	path   string
	rootFS *FS
}

var _ = (fs.InodeEmbedder)((*Dir)(nil))
var _ = (fs.NodeReaddirer)((*Dir)(nil))
var _ = (fs.NodeLookuper)((*Dir)(nil))
var _ = (fs.NodeCreater)((*Dir)(nil))
var _ = (fs.NodeMkdirer)((*Dir)(nil))
var _ = (fs.NodeUnlinker)((*Dir)(nil))
var _ = (fs.NodeRmdirer)((*Dir)(nil))
var _ = (fs.NodeRenamer)((*Dir)(nil))
var _ = (fs.NodeGetattrer)((*Dir)(nil))
// Xattr support is disabled to avoid macOS fcopyfile() issues
// var _ = (fs.NodeGetxattrer)((*Dir)(nil))
// var _ = (fs.NodeSetxattrer)((*Dir)(nil))
// var _ = (fs.NodeListxattrer)((*Dir)(nil))
// var _ = (fs.NodeRemovexattrer)((*Dir)(nil))

// Getattr returns attributes for the directory
func (d *Dir) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	b, relPath, err := d.mapper.Resolve(d.path)
	if err != nil {
		return syscall.ENOENT
	}

	info, err := b.Stat(ctx, relPath)
	if err != nil {
		if isNotFound(err) {
			return syscall.ENOENT
		}
		return syscall.EIO
	}

	out.Mode = uint32(info.Mode) | syscall.S_IFDIR
	out.Size = uint64(info.Size)
	out.Uid = d.rootFS.uid
	out.Gid = d.rootFS.gid
	out.SetTimes(nil, &info.ModTime, nil)

	return 0
}

// Readdir lists directory contents
func (d *Dir) Readdir(ctx context.Context) (fs.DirStream, syscall.Errno) {
	b, relPath, err := d.mapper.Resolve(d.path)
	if err != nil {
		return nil, syscall.ENOENT
	}

	infos, err := b.ListDir(ctx, relPath)
	if err != nil {
		if isNotFound(err) {
			return nil, syscall.ENOENT
		}
		return nil, syscall.EIO
	}

	entries := make([]fuse.DirEntry, len(infos))
	for i, info := range infos {
		mode := uint32(syscall.S_IFREG)
		if info.IsDir {
			mode = syscall.S_IFDIR
		}

		entries[i] = fuse.DirEntry{
			Name: info.Name,
			Mode: mode,
		}
	}

	return fs.NewListDirStream(entries), 0
}

// Lookup finds a child node
func (d *Dir) Lookup(ctx context.Context, name string, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childPath := path.Join(d.path, name)

	b, relPath, err := d.mapper.Resolve(childPath)
	if err != nil {
		return nil, syscall.ENOENT
	}

	info, err := b.Stat(ctx, relPath)
	if err != nil {
		if isNotFound(err) {
			return nil, syscall.ENOENT
		}
		return nil, syscall.EIO
	}

	var node fs.InodeEmbedder
	if info.IsDir {
		node = &Dir{
			mapper: d.mapper,
			path:   childPath,
			rootFS: d.rootFS,
		}
		out.Mode = uint32(info.Mode) | syscall.S_IFDIR
	} else {
		node = &File{
			mapper: d.mapper,
			path:   childPath,
			rootFS: d.rootFS,
		}
		out.Mode = uint32(info.Mode)
	}

	out.Size = uint64(info.Size)
	out.Uid = d.rootFS.uid
	out.Gid = d.rootFS.gid
	out.SetTimes(nil, &info.ModTime, nil)

	child := d.NewInode(ctx, node, fs.StableAttr{
		Mode: info.Mode,
	})

	return child, 0
}

// Create creates a new file
func (d *Dir) Create(ctx context.Context, name string, flags uint32, mode uint32, out *fuse.EntryOut) (node *fs.Inode, fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	childPath := path.Join(d.path, name)

	b, relPath, err := d.mapper.Resolve(childPath)
	if err != nil {
		return nil, nil, 0, syscall.ENOENT
	}

	// Create empty file
	if err := b.WriteFile(ctx, relPath, []byte{}, mode); err != nil {
		return nil, nil, 0, syscall.EIO
	}

	fileNode := &File{
		mapper: d.mapper,
		path:   childPath,
		rootFS: d.rootFS,
	}

	child := d.NewInode(ctx, fileNode, fs.StableAttr{
		Mode: mode,
	})

	handle := &FileHandle{
		file: fileNode,
	}

	out.Mode = mode
	out.Size = 0
	out.Uid = d.rootFS.uid
	out.Gid = d.rootFS.gid

	return child, handle, 0, 0
}

// Mkdir creates a new directory
func (d *Dir) Mkdir(ctx context.Context, name string, mode uint32, out *fuse.EntryOut) (*fs.Inode, syscall.Errno) {
	childPath := path.Join(d.path, name)

	b, relPath, err := d.mapper.Resolve(childPath)
	if err != nil {
		return nil, syscall.ENOENT
	}

	if err := b.CreateDir(ctx, relPath, mode); err != nil {
		return nil, syscall.EIO
	}

	dirNode := &Dir{
		mapper: d.mapper,
		path:   childPath,
		rootFS: d.rootFS,
	}

	child := d.NewInode(ctx, dirNode, fs.StableAttr{
		Mode: mode | syscall.S_IFDIR,
	})

	out.Mode = mode | syscall.S_IFDIR
	out.Uid = d.rootFS.uid
	out.Gid = d.rootFS.gid

	return child, 0
}

// Unlink removes a file
func (d *Dir) Unlink(ctx context.Context, name string) syscall.Errno {
	childPath := path.Join(d.path, name)

	b, relPath, err := d.mapper.Resolve(childPath)
	if err != nil {
		return syscall.ENOENT
	}

	if err := b.Delete(ctx, relPath); err != nil {
		if isNotFound(err) {
			return syscall.ENOENT
		}
		return syscall.EIO
	}

	return 0
}

// Rmdir removes a directory
func (d *Dir) Rmdir(ctx context.Context, name string) syscall.Errno {
	childPath := path.Join(d.path, name)

	b, relPath, err := d.mapper.Resolve(childPath)
	if err != nil {
		return syscall.ENOENT
	}

	if err := b.DeleteDir(ctx, relPath); err != nil {
		if isNotFound(err) {
			return syscall.ENOENT
		}
		return syscall.EIO
	}

	return 0
}

// Rename renames a file or directory
func (d *Dir) Rename(ctx context.Context, name string, newParent fs.InodeEmbedder, newName string, flags uint32) syscall.Errno {
	oldPath := path.Join(d.path, name)

	var newPath string
	if newDir, ok := newParent.(*Dir); ok {
		newPath = path.Join(newDir.path, newName)
	} else if _, ok := newParent.(*FS); ok {
		newPath = "/" + newName
	} else {
		return syscall.ENOTSUP
	}

	b, oldRelPath, err := d.mapper.Resolve(oldPath)
	if err != nil {
		return syscall.ENOENT
	}

	newB, newRelPath, err := d.mapper.Resolve(newPath)
	if err != nil {
		return syscall.ENOENT
	}

	// Check if same backend
	if b != newB {
		return syscall.EXDEV // Cross-device link
	}

	if err := b.Rename(ctx, oldRelPath, newRelPath); err != nil {
		if isNotFound(err) {
			return syscall.ENOENT
		}
		return syscall.EIO
	}

	return 0
}

// File represents a file node
type File struct {
	fs.Inode
	mapper *pathmap.PathMapper
	path   string
	rootFS *FS
}

var _ = (fs.InodeEmbedder)((*File)(nil))
var _ = (fs.NodeOpener)((*File)(nil))
var _ = (fs.NodeGetattrer)((*File)(nil))
var _ = (fs.NodeSetattrer)((*File)(nil))
// Xattr support is disabled to avoid macOS fcopyfile() issues
// var _ = (fs.NodeGetxattrer)((*File)(nil))
// var _ = (fs.NodeSetxattrer)((*File)(nil))
// var _ = (fs.NodeListxattrer)((*File)(nil))
// var _ = (fs.NodeRemovexattrer)((*File)(nil))

// Getattr returns attributes for the file
func (f *File) Getattr(ctx context.Context, fh fs.FileHandle, out *fuse.AttrOut) syscall.Errno {
	b, relPath, err := f.mapper.Resolve(f.path)
	if err != nil {
		return syscall.ENOENT
	}

	info, err := b.Stat(ctx, relPath)
	if err != nil {
		if isNotFound(err) {
			return syscall.ENOENT
		}
		return syscall.EIO
	}

	out.Mode = uint32(info.Mode)
	out.Size = uint64(info.Size)
	out.Uid = f.rootFS.uid
	out.Gid = f.rootFS.gid
	out.SetTimes(nil, &info.ModTime, nil)

	return 0
}

// Setattr sets file attributes
func (f *File) Setattr(ctx context.Context, fh fs.FileHandle, in *fuse.SetAttrIn, out *fuse.AttrOut) syscall.Errno {
	b, relPath, err := f.mapper.Resolve(f.path)
	if err != nil {
		return syscall.ENOENT
	}

	// Handle mode changes
	if mode, ok := in.GetMode(); ok {
		if err := b.UpdateMode(ctx, relPath, mode); err != nil {
			return syscall.EIO
		}
	}

	// Handle size changes (truncate)
	if size, ok := in.GetSize(); ok {
		if size == 0 {
			// Truncate to zero
			if err := b.WriteFile(ctx, relPath, []byte{}, 0644); err != nil {
				return syscall.EIO
			}
		} else {
			// Truncate to specific size
			data, err := b.ReadFile(ctx, relPath)
			if err != nil {
				return syscall.EIO
			}

			if uint64(len(data)) > size {
				data = data[:size]
			} else {
				extended := make([]byte, size)
				copy(extended, data)
				data = extended
			}

			if err := b.WriteFile(ctx, relPath, data, 0644); err != nil {
				return syscall.EIO
			}
		}
	}

	// Return updated attributes
	return f.Getattr(ctx, fh, out)
}

// Open opens the file
func (f *File) Open(ctx context.Context, flags uint32) (fh fs.FileHandle, fuseFlags uint32, errno syscall.Errno) {
	return &FileHandle{
		file: f,
	}, 0, 0
}

// FileHandle represents an open file handle
type FileHandle struct {
	file *File
}

var _ = (fs.FileHandle)((*FileHandle)(nil))
var _ = (fs.FileReader)((*FileHandle)(nil))
var _ = (fs.FileWriter)((*FileHandle)(nil))
var _ = (fs.FileFlusher)((*FileHandle)(nil))

// Read reads from the file
func (fh *FileHandle) Read(ctx context.Context, dest []byte, off int64) (fuse.ReadResult, syscall.Errno) {
	b, relPath, err := fh.file.mapper.Resolve(fh.file.path)
	if err != nil {
		return nil, syscall.ENOENT
	}

	data, err := b.ReadFileRange(ctx, relPath, off, int64(len(dest)))
	if err != nil {
		if isNotFound(err) {
			return nil, syscall.ENOENT
		}
		return nil, syscall.EIO
	}

	return fuse.ReadResultData(data), 0
}

// Write writes to the file
func (fh *FileHandle) Write(ctx context.Context, data []byte, off int64) (written uint32, errno syscall.Errno) {
	b, relPath, err := fh.file.mapper.Resolve(fh.file.path)
	if err != nil {
		return 0, syscall.ENOENT
	}

	// Read current file content
	currentData, err := b.ReadFile(ctx, relPath)
	if err != nil && !isNotFound(err) {
		return 0, syscall.EIO
	}

	// Calculate new size
	endOffset := off + int64(len(data))
	newSize := endOffset
	if int64(len(currentData)) > newSize {
		newSize = int64(len(currentData))
	}

	// Create new data buffer
	newData := make([]byte, newSize)
	copy(newData, currentData)
	copy(newData[off:], data)

	// Write back
	if err := b.WriteFile(ctx, relPath, newData, 0644); err != nil {
		return 0, syscall.EIO
	}

	return uint32(len(data)), 0
}

// Flush flushes any buffered data
func (fh *FileHandle) Flush(ctx context.Context) syscall.Errno {
	// For write-through mode, nothing to flush
	return 0
}

// isNotFound checks if an error is a "not found" error
func isNotFound(err error) bool {
	return errors.Is(err, backend.ErrNotFound) || fmt.Sprintf("%v", err) == "not found"
}

// Extended attribute operations for FS (root)
// These delegate to the backend if it supports XattrBackend, otherwise silently succeed

func (f *FS) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	// Root directory doesn't have xattrs
	return 0, syscall.ENODATA
}

func (f *FS) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	// Silently succeed - root doesn't store xattrs
	return 0
}

func (f *FS) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	// Return empty list
	return 0, 0
}

func (f *FS) Removexattr(ctx context.Context, attr string) syscall.Errno {
	// Silently succeed
	return 0
}

// Extended attribute operations for Dir
// Delegates to backend if it implements XattrBackend

func (d *Dir) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	b, relPath, err := d.mapper.Resolve(d.path)
	if err != nil {
		return 0, syscall.ENOENT
	}

	// Check if backend supports xattrs
	if xb, ok := b.(backend.XattrBackend); ok {
		value, err := xb.GetXattr(ctx, relPath, attr)
		if err != nil {
			if isNotFound(err) {
				return 0, syscall.ENODATA
			}
			return 0, syscall.EIO
		}
		if len(dest) > 0 {
			n := copy(dest, value)
			return uint32(n), 0
		}
		return uint32(len(value)), 0
	}

	// Backend doesn't support xattrs
	return 0, syscall.ENODATA
}

func (d *Dir) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	b, relPath, err := d.mapper.Resolve(d.path)
	if err != nil {
		return syscall.ENOENT
	}

	// Check if backend supports xattrs
	if xb, ok := b.(backend.XattrBackend); ok {
		if err := xb.SetXattr(ctx, relPath, attr, data); err != nil {
			return syscall.EIO
		}
		return 0
	}

	// Backend doesn't support xattrs - silently succeed
	return 0
}

func (d *Dir) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	b, relPath, err := d.mapper.Resolve(d.path)
	if err != nil {
		return 0, syscall.ENOENT
	}

	// Check if backend supports xattrs
	if xb, ok := b.(backend.XattrBackend); ok {
		names, err := xb.ListXattr(ctx, relPath)
		if err != nil {
			return 0, syscall.EIO
		}

		// Format as null-separated list
		var total uint32
		for _, name := range names {
			total += uint32(len(name)) + 1 // +1 for null terminator
		}

		if len(dest) > 0 {
			var written uint32
			for _, name := range names {
				n := copy(dest[written:], name)
				written += uint32(n)
				if int(written) < len(dest) {
					dest[written] = 0
					written++
				}
			}
			return written, 0
		}
		return total, 0
	}

	// Backend doesn't support xattrs - return empty list
	return 0, 0
}

func (d *Dir) Removexattr(ctx context.Context, attr string) syscall.Errno {
	b, relPath, err := d.mapper.Resolve(d.path)
	if err != nil {
		return syscall.ENOENT
	}

	// Check if backend supports xattrs
	if xb, ok := b.(backend.XattrBackend); ok {
		if err := xb.RemoveXattr(ctx, relPath, attr); err != nil {
			if isNotFound(err) {
				return syscall.ENODATA
			}
			return syscall.EIO
		}
		return 0
	}

	// Backend doesn't support xattrs - silently succeed
	return 0
}

// Extended attribute operations for File
// Same implementation as Dir

func (f *File) Getxattr(ctx context.Context, attr string, dest []byte) (uint32, syscall.Errno) {
	b, relPath, err := f.mapper.Resolve(f.path)
	if err != nil {
		return 0, syscall.ENOENT
	}

	if xb, ok := b.(backend.XattrBackend); ok {
		value, err := xb.GetXattr(ctx, relPath, attr)
		if err != nil {
			if isNotFound(err) {
				return 0, syscall.ENODATA
			}
			return 0, syscall.EIO
		}
		if len(dest) > 0 {
			n := copy(dest, value)
			return uint32(n), 0
		}
		return uint32(len(value)), 0
	}

	return 0, syscall.ENODATA
}

func (f *File) Setxattr(ctx context.Context, attr string, data []byte, flags uint32) syscall.Errno {
	b, relPath, err := f.mapper.Resolve(f.path)
	if err != nil {
		return syscall.ENOENT
	}

	if xb, ok := b.(backend.XattrBackend); ok {
		if err := xb.SetXattr(ctx, relPath, attr, data); err != nil {
			return syscall.EIO
		}
		return 0
	}

	return 0
}

func (f *File) Listxattr(ctx context.Context, dest []byte) (uint32, syscall.Errno) {
	b, relPath, err := f.mapper.Resolve(f.path)
	if err != nil {
		return 0, syscall.ENOENT
	}

	if xb, ok := b.(backend.XattrBackend); ok {
		names, err := xb.ListXattr(ctx, relPath)
		if err != nil {
			return 0, syscall.EIO
		}

		var total uint32
		for _, name := range names {
			total += uint32(len(name)) + 1
		}

		if len(dest) > 0 {
			var written uint32
			for _, name := range names {
				n := copy(dest[written:], name)
				written += uint32(n)
				if int(written) < len(dest) {
					dest[written] = 0
					written++
				}
			}
			return written, 0
		}
		return total, 0
	}

	return 0, 0
}

func (f *File) Removexattr(ctx context.Context, attr string) syscall.Errno {
	b, relPath, err := f.mapper.Resolve(f.path)
	if err != nil {
		return syscall.ENOENT
	}

	if xb, ok := b.(backend.XattrBackend); ok {
		if err := xb.RemoveXattr(ctx, relPath, attr); err != nil {
			if isNotFound(err) {
				return syscall.ENODATA
			}
			return syscall.EIO
		}
		return 0
	}

	return 0
}
