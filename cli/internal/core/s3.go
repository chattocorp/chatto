package core

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"hmans.de/chatto/internal/config"
)

// S3Client wraps minio-go client for S3-compatible storage operations.
type S3Client struct {
	client *minio.Client
	bucket string
}

// NewS3Client creates a new S3 client from configuration.
// Returns nil if S3 storage is not configured.
func NewS3Client(cfg config.S3Config) (*S3Client, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, nil
	}

	// Set up bucket lookup type based on path-style config
	// Path-style: http://endpoint/bucket/key (required for MinIO and most S3-compatible services)
	// Virtual-hosted: http://bucket.endpoint/key (default for AWS S3)
	bucketLookup := minio.BucketLookupAuto
	if cfg.PathStyleOrDefault() {
		bucketLookup = minio.BucketLookupPath
	}

	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure:       cfg.UseSSLOrDefault(),
		Region:       cfg.Region,
		BucketLookup: bucketLookup,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	return &S3Client{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// Bucket returns the configured bucket name.
func (s *S3Client) Bucket() string {
	return s.bucket
}

// EnsureBucket creates the bucket if it doesn't exist.
func (s *S3Client) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		err = s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("failed to create bucket: %w", err)
		}
	}
	return nil
}

// S3ObjectInfo contains metadata about an S3 object.
type S3ObjectInfo struct {
	Key         string
	Size        int64
	ContentType string
}

// PutObject uploads an object to S3.
func (s *S3Client) PutObject(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (*S3ObjectInfo, error) {
	opts := minio.PutObjectOptions{
		ContentType: contentType,
	}

	info, err := s.client.PutObject(ctx, s.bucket, key, reader, size, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to upload object: %w", err)
	}

	return &S3ObjectInfo{
		Key:         info.Key,
		Size:        info.Size,
		ContentType: contentType,
	}, nil
}

// PutObjectFromBytes uploads bytes to S3.
func (s *S3Client) PutObjectFromBytes(ctx context.Context, key string, data []byte, contentType string) (*S3ObjectInfo, error) {
	return s.PutObject(ctx, key, bytes.NewReader(data), int64(len(data)), contentType)
}

// GetObject retrieves an object from S3.
// The returned reader must be closed by the caller.
func (s *S3Client) GetObject(ctx context.Context, key string) (io.ReadCloser, *S3ObjectInfo, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get object: %w", err)
	}

	stat, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, nil, fmt.Errorf("failed to stat object: %w", err)
	}

	return obj, &S3ObjectInfo{
		Key:         stat.Key,
		Size:        stat.Size,
		ContentType: stat.ContentType,
	}, nil
}

// GetObjectFromBucket retrieves an object from a specific bucket (for multi-bucket support).
func (s *S3Client) GetObjectFromBucket(ctx context.Context, bucket, key string) (io.ReadCloser, *S3ObjectInfo, error) {
	if bucket == "" {
		bucket = s.bucket
	}

	obj, err := s.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get object: %w", err)
	}

	stat, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, nil, fmt.Errorf("failed to stat object: %w", err)
	}

	return obj, &S3ObjectInfo{
		Key:         stat.Key,
		Size:        stat.Size,
		ContentType: stat.ContentType,
	}, nil
}

// DeleteObject deletes an object from S3.
func (s *S3Client) DeleteObject(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

// DeleteObjectFromBucket deletes an object from a specific bucket (for multi-bucket support).
func (s *S3Client) DeleteObjectFromBucket(ctx context.Context, bucket, key string) error {
	if bucket == "" {
		bucket = s.bucket
	}

	err := s.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

// StatObject returns metadata about an object without downloading it.
func (s *S3Client) StatObject(ctx context.Context, key string) (*S3ObjectInfo, error) {
	stat, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	return &S3ObjectInfo{
		Key:         stat.Key,
		Size:        stat.Size,
		ContentType: stat.ContentType,
	}, nil
}

// PresignedGetURL generates a presigned GET URL for an S3 object.
// The URL is valid for the specified duration (max 7 days).
func (s *S3Client) PresignedGetURL(ctx context.Context, key string, expiry time.Duration) (*url.URL, error) {
	return s.client.PresignedGetObject(ctx, s.bucket, key, expiry, nil)
}

// S3 key helpers for organizing assets in S3.
// Space attachments use hierarchical paths; server assets use flat keys matching NATS.

// S3KeySpaceAttachment returns the S3 key for a space attachment.
// Format: spaces/{spaceId}/attachments/{attachmentId}
// This format allows the HTTP handler to construct the key from URL parameters.
func S3KeySpaceAttachment(spaceID, attachmentID string) string {
	return fmt.Sprintf("spaces/%s/attachments/%s", spaceID, attachmentID)
}

// S3KeyServerAsset returns the S3 key for an server asset (avatar, logo, banner).
// Format: instance/{assetId}
// This matches the NATS key format so HTTP handlers can probe both backends with the same key.
func S3KeyServerAsset(assetID string) string {
	return fmt.Sprintf("instance/%s", assetID)
}
