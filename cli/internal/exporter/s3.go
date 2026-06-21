package exporter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"hmans.de/chatto/internal/config"
)

type s3Scanner struct {
	client     *minio.Client
	bucket     string
	pathPrefix string
	timeout    time.Duration

	mu    sync.RWMutex
	stats s3Stats
}

type s3Stats struct {
	Configured             bool
	CurrentObjects         int64
	CurrentBytes           int64
	NonCurrentObjects      int64
	NonCurrentBytes        int64
	AllVersionObjects      int64
	AllVersionBytes        int64
	LastRefreshUnixSeconds int64
	LastDurationSeconds    float64
	LastSuccess            bool
	LastError              string
}

func newS3Scanner(assets config.AssetsConfig, timeout time.Duration) (*s3Scanner, error) {
	if assets.StorageBackend != config.StorageBackendS3 {
		return nil, nil
	}
	cfg := assets.S3
	if cfg.Endpoint == "" || cfg.Bucket == "" {
		return nil, nil
	}
	cfg.NormalizePathPrefix()
	if err := cfg.ValidatePathPrefix(); err != nil {
		return nil, err
	}
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
		return nil, fmt.Errorf("create S3 client: %w", err)
	}
	return &s3Scanner{
		client:     client,
		bucket:     cfg.Bucket,
		pathPrefix: listPrefix(cfg.PathPrefix),
		timeout:    timeout,
		stats:      s3Stats{Configured: true},
	}, nil
}

func listPrefix(pathPrefix string) string {
	pathPrefix = strings.Trim(pathPrefix, "/")
	if pathPrefix == "" {
		return ""
	}
	return pathPrefix + "/"
}

func (s *s3Scanner) run(ctx context.Context, interval time.Duration) {
	if s == nil {
		return
	}
	s.refresh(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refresh(ctx)
		}
	}
}

func (s *s3Scanner) refresh(parent context.Context) {
	if s == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, s.timeout)
	defer cancel()

	started := time.Now()
	stats := s3Stats{Configured: true}
	currentObjects, currentBytes, err := s.scan(ctx, false)
	if err != nil {
		stats.LastSuccess = false
		stats.LastError = err.Error()
		stats.LastDurationSeconds = time.Since(started).Seconds()
		stats.LastRefreshUnixSeconds = time.Now().Unix()
		s.store(stats)
		return
	}
	stats.CurrentObjects = currentObjects
	stats.CurrentBytes = currentBytes

	allObjects, allBytes, err := s.scan(ctx, true)
	if err != nil {
		stats.AllVersionObjects = currentObjects
		stats.AllVersionBytes = currentBytes
		stats.NonCurrentObjects = 0
		stats.NonCurrentBytes = 0
		stats.LastSuccess = false
		stats.LastError = err.Error()
		stats.LastDurationSeconds = time.Since(started).Seconds()
		stats.LastRefreshUnixSeconds = time.Now().Unix()
		s.store(stats)
		return
	}
	stats.AllVersionObjects = allObjects
	stats.AllVersionBytes = allBytes
	stats.deriveNonCurrent()
	stats.LastSuccess = true
	stats.LastDurationSeconds = time.Since(started).Seconds()
	stats.LastRefreshUnixSeconds = time.Now().Unix()
	s.store(stats)
}

func (s *s3Scanner) scan(ctx context.Context, withVersions bool) (objects int64, bytes int64, err error) {
	opts := minio.ListObjectsOptions{
		Prefix:       s.pathPrefix,
		Recursive:    true,
		WithVersions: withVersions,
	}
	for object := range s.client.ListObjects(ctx, s.bucket, opts) {
		if object.Err != nil {
			return 0, 0, object.Err
		}
		if object.Key == "" {
			continue
		}
		objects++
		bytes += object.Size
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	return objects, bytes, nil
}

func (s *s3Scanner) store(stats s3Stats) {
	s.mu.Lock()
	s.stats = stats
	s.mu.Unlock()
}

func (s *s3Scanner) snapshot() s3Stats {
	if s == nil {
		return s3Stats{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

func (s *s3Stats) deriveNonCurrent() {
	s.NonCurrentObjects = maxInt64(0, s.AllVersionObjects-s.CurrentObjects)
	s.NonCurrentBytes = maxInt64(0, s.AllVersionBytes-s.CurrentBytes)
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
