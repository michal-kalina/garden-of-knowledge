// Package storage abstracts object storage (MinIO in dev, any S3-compatible
// store in production) behind a narrow interface, so the rest of the system
// never depends on a concrete SDK.
package storage

import (
	"context"
	"io"
)

// ObjectStore is the minimal contract the application needs from object storage.
// Keeping it this small makes fakes trivial in tests and keeps the MinIO SDK
// contained in a single file.
type ObjectStore interface {
	// Put stores the object under key. size must match the reader's length
	// (S3 needs it up front for non-chunked uploads).
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	// Get returns a reader for the object. The caller must Close it.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
}
