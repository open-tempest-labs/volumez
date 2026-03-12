package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/lmccay/volumez/pkg/backend"
)

// S3Backend implements the Backend interface for AWS S3
type S3Backend struct {
	client *s3.Client
	bucket string
	prefix string // Optional prefix for all operations
}

// Config holds S3 backend configuration
type Config struct {
	Bucket   string `json:"bucket"`
	Region   string `json:"region"`
	Endpoint string `json:"endpoint"` // Optional: for S3-compatible services
	Prefix   string `json:"prefix"`   // Optional: prefix for all keys
}

func init() {
	backend.Register("s3", New)
}

// New creates a new S3 backend from configuration
func New(cfg map[string]interface{}) (backend.Backend, error) {
	bucketName, ok := cfg["bucket"].(string)
	if !ok || bucketName == "" {
		return nil, &backend.BackendError{
			Op:      "init",
			Backend: "s3",
			Err:     fmt.Errorf("%w: bucket is required", backend.ErrInvalidConfig),
		}
	}

	region, _ := cfg["region"].(string)
	if region == "" {
		region = "us-east-1" // Default region
	}

	endpoint, _ := cfg["endpoint"].(string)
	prefix, _ := cfg["prefix"].(string)

	// Load AWS configuration
	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, &backend.BackendError{
			Op:      "init",
			Backend: "s3",
			Err:     fmt.Errorf("failed to load AWS config: %w", err),
		}
	}

	// Create S3 client
	clientOpts := []func(*s3.Options){}
	if endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}

	client := s3.NewFromConfig(awsCfg, clientOpts...)

	return &S3Backend{
		client: client,
		bucket: bucketName,
		prefix: strings.TrimSuffix(prefix, "/"),
	}, nil
}

// toS3Key converts a filesystem path to an S3 key
func (s *S3Backend) toS3Key(p string) string {
	// Clean the path and remove leading slash
	p = strings.TrimPrefix(path.Clean(p), "/")

	if s.prefix != "" {
		if p == "" || p == "." {
			return s.prefix + "/"
		}
		return s.prefix + "/" + p
	}

	if p == "" || p == "." {
		return ""
	}
	return p
}

// Stat returns metadata about a file or directory
func (s *S3Backend) Stat(ctx context.Context, p string) (*backend.FileInfo, error) {
	key := s.toS3Key(p)

	// Try as a file first
	headResp, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	if err == nil {
		// It's a file
		return &backend.FileInfo{
			Name:    path.Base(p),
			Size:    aws.ToInt64(headResp.ContentLength),
			Mode:    0666, // Default file permissions (readable/writable by all)
			ModTime: aws.ToTime(headResp.LastModified),
			IsDir:   false,
			ETag:    aws.ToString(headResp.ETag),
		}, nil
	}

	// Check if it's a directory by listing with prefix
	dirKey := key
	if !strings.HasSuffix(dirKey, "/") && dirKey != "" {
		dirKey += "/"
	}
	// Ensure we're checking the right prefix
	if dirKey == "" {
		dirKey = s.prefix
		if dirKey != "" && !strings.HasSuffix(dirKey, "/") {
			dirKey += "/"
		}
	}

	listResp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(dirKey),
		MaxKeys:   aws.Int32(1),
		Delimiter: aws.String("/"),
	})

	if err != nil {
		return nil, s.wrapError("stat", p, err)
	}

	if aws.ToInt32(listResp.KeyCount) > 0 || len(listResp.CommonPrefixes) > 0 {
		// It's a directory
		return &backend.FileInfo{
			Name:    path.Base(p),
			Size:    0,
			Mode:    0777 | syscall.S_IFDIR, // Directory with full permissions
			ModTime: time.Now(),
			IsDir:   true,
		}, nil
	}

	return nil, s.wrapError("stat", p, backend.ErrNotFound)
}

// ReadFile reads the entire contents of a file
func (s *S3Backend) ReadFile(ctx context.Context, p string) ([]byte, error) {
	key := s.toS3Key(p)

	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, s.wrapError("read", p, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, s.wrapError("read", p, err)
	}

	return data, nil
}

// ReadFileRange reads a range of bytes from a file
func (s *S3Backend) ReadFileRange(ctx context.Context, p string, offset int64, size int64) ([]byte, error) {
	key := s.toS3Key(p)

	rangeHeader := fmt.Sprintf("bytes=%d-%d", offset, offset+size-1)

	resp, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Range:  aws.String(rangeHeader),
	})
	if err != nil {
		return nil, s.wrapError("read", p, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, s.wrapError("read", p, err)
	}

	return data, nil
}

// ListDir returns the contents of a directory
func (s *S3Backend) ListDir(ctx context.Context, p string) ([]*backend.FileInfo, error) {
	key := s.toS3Key(p)
	if key != "" && !strings.HasSuffix(key, "/") {
		key += "/"
	}

	var results []*backend.FileInfo
	var continuationToken *string

	for {
		resp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(key),
			Delimiter:         aws.String("/"),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, s.wrapError("listdir", p, err)
		}

		// Add files
		for _, obj := range resp.Contents {
			name := strings.TrimPrefix(aws.ToString(obj.Key), key)
			if name == "" {
				continue // Skip the directory marker itself
			}

			results = append(results, &backend.FileInfo{
				Name:    name,
				Size:    aws.ToInt64(obj.Size),
				Mode:    0666,
				ModTime: aws.ToTime(obj.LastModified),
				IsDir:   false,
				ETag:    aws.ToString(obj.ETag),
			})
		}

		// Add subdirectories
		for _, prefix := range resp.CommonPrefixes {
			name := strings.TrimSuffix(strings.TrimPrefix(aws.ToString(prefix.Prefix), key), "/")
			if name == "" {
				continue
			}

			results = append(results, &backend.FileInfo{
				Name:    name,
				Size:    0,
				Mode:    0777 | syscall.S_IFDIR,
				ModTime: time.Now(),
				IsDir:   true,
			})
		}

		if !aws.ToBool(resp.IsTruncated) {
			break
		}
		continuationToken = resp.NextContinuationToken
	}

	return results, nil
}

// WriteFile writes data to a file
func (s *S3Backend) WriteFile(ctx context.Context, p string, data []byte, mode uint32) error {
	return s.WriteFileStream(ctx, p, bytes.NewReader(data), mode)
}

// WriteFileStream writes data from a reader
func (s *S3Backend) WriteFileStream(ctx context.Context, p string, r io.Reader, mode uint32) error {
	key := s.toS3Key(p)

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   r,
	})

	if err != nil {
		return s.wrapError("write", p, err)
	}

	return nil
}

// CreateDir creates a directory (in S3, this creates a marker object)
func (s *S3Backend) CreateDir(ctx context.Context, p string, mode uint32) error {
	key := s.toS3Key(p)
	if !strings.HasSuffix(key, "/") {
		key += "/"
	}

	// Create a directory marker object
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader([]byte{}),
	})

	if err != nil {
		return s.wrapError("mkdir", p, err)
	}

	return nil
}

// Delete removes a file
func (s *S3Backend) Delete(ctx context.Context, p string) error {
	key := s.toS3Key(p)

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return s.wrapError("delete", p, err)
	}

	return nil
}

// DeleteDir removes a directory and its contents
func (s *S3Backend) DeleteDir(ctx context.Context, p string) error {
	key := s.toS3Key(p)
	if !strings.HasSuffix(key, "/") {
		key += "/"
	}

	// List all objects with this prefix
	var objectsToDelete []types.ObjectIdentifier
	var continuationToken *string

	for {
		resp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(key),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return s.wrapError("deletedir", p, err)
		}

		for _, obj := range resp.Contents {
			objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
				Key: obj.Key,
			})
		}

		if !aws.ToBool(resp.IsTruncated) {
			break
		}
		continuationToken = resp.NextContinuationToken
	}

	// Delete all objects
	if len(objectsToDelete) > 0 {
		_, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(s.bucket),
			Delete: &types.Delete{
				Objects: objectsToDelete,
				Quiet:   aws.Bool(true),
			},
		})
		if err != nil {
			return s.wrapError("deletedir", p, err)
		}
	}

	return nil
}

// Exists checks if a path exists
func (s *S3Backend) Exists(ctx context.Context, p string) (bool, error) {
	_, err := s.Stat(ctx, p)
	if err != nil {
		if errors.Is(err, backend.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Rename moves/renames a file or directory
func (s *S3Backend) Rename(ctx context.Context, oldPath, newPath string) error {
	oldKey := s.toS3Key(oldPath)
	newKey := s.toS3Key(newPath)

	// Check if it's a directory
	info, err := s.Stat(ctx, oldPath)
	if err != nil {
		return err
	}

	if info.IsDir {
		// Rename directory: copy all objects with prefix, then delete originals
		return s.renameDir(ctx, oldKey, newKey)
	}

	// Copy object
	_, err = s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(s.bucket),
		CopySource: aws.String(fmt.Sprintf("%s/%s", s.bucket, oldKey)),
		Key:        aws.String(newKey),
	})
	if err != nil {
		return s.wrapError("rename", oldPath, err)
	}

	// Delete original
	return s.Delete(ctx, oldPath)
}

func (s *S3Backend) renameDir(ctx context.Context, oldKey, newKey string) error {
	if !strings.HasSuffix(oldKey, "/") {
		oldKey += "/"
	}
	if !strings.HasSuffix(newKey, "/") {
		newKey += "/"
	}

	// List all objects
	var continuationToken *string
	for {
		resp, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(oldKey),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return s.wrapError("rename", oldKey, err)
		}

		// Copy each object
		for _, obj := range resp.Contents {
			oldObjKey := aws.ToString(obj.Key)
			newObjKey := strings.Replace(oldObjKey, oldKey, newKey, 1)

			_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
				Bucket:     aws.String(s.bucket),
				CopySource: aws.String(fmt.Sprintf("%s/%s", s.bucket, oldObjKey)),
				Key:        aws.String(newObjKey),
			})
			if err != nil {
				return s.wrapError("rename", oldObjKey, err)
			}
		}

		if !aws.ToBool(resp.IsTruncated) {
			break
		}
		continuationToken = resp.NextContinuationToken
	}

	// Delete original directory
	return s.DeleteDir(ctx, oldKey)
}

// UpdateMode changes file permissions (no-op for S3)
func (s *S3Backend) UpdateMode(ctx context.Context, p string, mode uint32) error {
	// S3 doesn't support file permissions
	// We could store this in object metadata if needed
	return nil
}

// Close cleans up resources
func (s *S3Backend) Close() error {
	// AWS SDK v2 client doesn't require explicit cleanup
	return nil
}

// wrapError wraps an error with backend context
func (s *S3Backend) wrapError(op, p string, err error) error {
	// Convert AWS errors to backend errors
	if strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "NotFound") {
		err = backend.ErrNotFound
	}

	return &backend.BackendError{
		Op:      op,
		Path:    p,
		Backend: "s3",
		Err:     err,
	}
}
