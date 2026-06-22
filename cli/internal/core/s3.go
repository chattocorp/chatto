package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"hmans.de/chatto/internal/config"
)

// S3Client wraps the AWS S3 client for S3-compatible storage operations.
type S3Client struct {
	client      *s3.Client
	presign     *s3.PresignClient
	bucket      string
	pathPrefix  string
	awsEndpoint bool
}

// NewS3Client creates a new S3 client from configuration.
// Returns nil if S3 storage is not configured.
func NewS3Client(cfg config.S3Config) (*S3Client, error) {
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, nil
	}
	cfg.NormalizePathPrefix()
	if err := cfg.ValidatePathPrefix(); err != nil {
		return nil, err
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	endpoint := s3EndpointURL(cfg)

	client := s3.New(s3.Options{
		Credentials:                credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Region:                     region,
		BaseEndpoint:               aws.String(endpoint),
		UsePathStyle:               cfg.UsePathStyleForEndpoint(),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
	})

	return &S3Client{
		client:      client,
		presign:     s3.NewPresignClient(client),
		bucket:      cfg.Bucket,
		pathPrefix:  cfg.PathPrefix,
		awsEndpoint: cfg.IsAWSEndpoint(),
	}, nil
}

func s3EndpointURL(cfg config.S3Config) string {
	if strings.HasPrefix(cfg.Endpoint, "http://") || strings.HasPrefix(cfg.Endpoint, "https://") {
		return cfg.Endpoint
	}
	if cfg.UseSSLOrDefault() {
		return "https://" + cfg.Endpoint
	}
	return "http://" + cfg.Endpoint
}

// Bucket returns the configured bucket name.
func (s *S3Client) Bucket() string {
	return s.bucket
}

// PathPrefix returns the configured S3 object-key prefix after normalization.
func (s *S3Client) PathPrefix() string {
	return s.pathPrefix
}

func (s *S3Client) physicalKey(logicalKey string) string {
	if s.pathPrefix == "" {
		return logicalKey
	}
	if logicalKey == "" {
		return s.pathPrefix
	}
	return s.pathPrefix + "/" + logicalKey
}

func (s *S3Client) logicalKey(physicalKey string) string {
	if s.pathPrefix == "" {
		return physicalKey
	}
	return strings.TrimPrefix(physicalKey, s.pathPrefix+"/")
}

// EnsureBucket creates the bucket if it doesn't exist.
func (s *S3Client) EnsureBucket(ctx context.Context) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s.bucket),
	})
	if err == nil {
		return nil
	}
	if !isNoSuchBucketError(err) {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}

	input := &s3.CreateBucketInput{
		Bucket: aws.String(s.bucket),
	}
	if region := s.client.Options().Region; region != "" && region != "us-east-1" && s.awsEndpoint {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}
	if _, err := s.client.CreateBucket(ctx, input); err != nil {
		return fmt.Errorf("failed to create bucket: %w", err)
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
	physicalKey := s.physicalKey(key)
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(physicalKey),
		Body:          reader,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload object: %w", err)
	}

	return &S3ObjectInfo{
		Key:         s.logicalKey(physicalKey),
		Size:        size,
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
	physicalKey := s.physicalKey(key)
	obj, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(physicalKey),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get object: %w", err)
	}

	return obj.Body, &S3ObjectInfo{
		Key:         s.logicalKey(physicalKey),
		Size:        aws.ToInt64(obj.ContentLength),
		ContentType: aws.ToString(obj.ContentType),
	}, nil
}

// GetObjectFromBucket retrieves an object from a specific bucket (for multi-bucket support).
func (s *S3Client) GetObjectFromBucket(ctx context.Context, bucket, key string) (io.ReadCloser, *S3ObjectInfo, error) {
	if bucket == "" {
		bucket = s.bucket
	}

	physicalKey := s.physicalKey(key)
	obj, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(physicalKey),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get object: %w", err)
	}

	return obj.Body, &S3ObjectInfo{
		Key:         s.logicalKey(physicalKey),
		Size:        aws.ToInt64(obj.ContentLength),
		ContentType: aws.ToString(obj.ContentType),
	}, nil
}

// DeleteObject deletes an object from S3.
func (s *S3Client) DeleteObject(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.physicalKey(key)),
	})
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

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s.physicalKey(key)),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}
	return nil
}

// StatObject returns metadata about an object without downloading it.
func (s *S3Client) StatObject(ctx context.Context, key string) (*S3ObjectInfo, error) {
	physicalKey := s.physicalKey(key)
	stat, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(physicalKey),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	return &S3ObjectInfo{
		Key:         s.logicalKey(physicalKey),
		Size:        aws.ToInt64(stat.ContentLength),
		ContentType: aws.ToString(stat.ContentType),
	}, nil
}

// IsNoSuchKeyError reports whether err came from S3 returning a definitive
// "object does not exist" response (404 / NoSuchKey). All other errors —
// network failures, timeouts, auth errors, misconfigured endpoint — return
// false so callers can distinguish "we know it's gone" from "we can't tell".
func IsNoSuchKeyError(err error) bool {
	if err == nil {
		return false
	}
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return true
		}
	}
	var respErr *smithyhttp.ResponseError
	return errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404
}

// PresignedGetURL generates a presigned GET URL for an S3 object.
// The URL is valid for the specified duration (max 7 days).
func (s *S3Client) PresignedGetURL(ctx context.Context, key string, expiry time.Duration) (*url.URL, error) {
	resp, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(s.physicalKey(key)),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiry
	})
	if err != nil {
		return nil, err
	}
	return url.Parse(resp.URL)
}

func isNoSuchBucketError(err error) bool {
	var noSuchBucket *types.NoSuchBucket
	if errors.As(err, &noSuchBucket) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchBucket", "NotFound", "404":
			return true
		}
	}
	var respErr *smithyhttp.ResponseError
	return errors.As(err, &respErr) && respErr.HTTPStatusCode() == 404
}

// S3 key helpers for organizing assets in S3.

// S3KeyAttachment returns the S3 key for an attachment uploaded after
// ADR-030 Phase 4. Format: attachments/{attachmentId}. Pre-Phase-4
// attachments live at `spaces/{server|DM}/attachments/{id}`; that key
// shape is no longer constructed by Go code — the canonical key for
// every attachment comes from `Attachment.Storage.S3.Key` on the proto.
func S3KeyAttachment(attachmentID string) string {
	return fmt.Sprintf("attachments/%s", attachmentID)
}

// S3KeyServerAsset returns the S3 key for a server asset (avatar, logo, banner, link preview image).
// Format: instance/{assetId}
// This matches the NATS key format so HTTP handlers can probe both backends with the same key.
func S3KeyServerAsset(assetID string) string {
	return fmt.Sprintf("instance/%s", assetID)
}
