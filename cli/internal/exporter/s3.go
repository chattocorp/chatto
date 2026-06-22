package exporter

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"hmans.de/chatto/internal/config"
)

type s3Scanner struct {
	client     *s3.Client
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

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	client := s3.New(s3.Options{
		Credentials:                credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Region:                     region,
		BaseEndpoint:               aws.String(s3EndpointURL(cfg)),
		UsePathStyle:               cfg.UsePathStyleForEndpoint(),
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
	})
	return &s3Scanner{
		client:     client,
		bucket:     cfg.Bucket,
		pathPrefix: listPrefix(cfg.PathPrefix),
		timeout:    timeout,
		stats:      s3Stats{Configured: true},
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
	if withVersions {
		return s.scanVersions(ctx)
	}

	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.pathPrefix),
	}
	for {
		output, err := s.client.ListObjectsV2(ctx, input)
		if err != nil {
			return 0, 0, err
		}
		for _, object := range output.Contents {
			if aws.ToString(object.Key) == "" {
				continue
			}
			objects++
			bytes += aws.ToInt64(object.Size)
		}
		if !aws.ToBool(output.IsTruncated) || output.NextContinuationToken == nil {
			break
		}
		input.ContinuationToken = output.NextContinuationToken
	}
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	return objects, bytes, nil
}

func (s *s3Scanner) scanVersions(ctx context.Context) (objects int64, bytes int64, err error) {
	input := &s3.ListObjectVersionsInput{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.pathPrefix),
	}
	for {
		output, err := s.client.ListObjectVersions(ctx, input)
		if err != nil {
			return 0, 0, err
		}
		for _, object := range output.Versions {
			if aws.ToString(object.Key) == "" {
				continue
			}
			objects++
			bytes += aws.ToInt64(object.Size)
		}
		for _, marker := range output.DeleteMarkers {
			if aws.ToString(marker.Key) == "" {
				continue
			}
			objects++
		}
		if !aws.ToBool(output.IsTruncated) {
			break
		}
		if output.NextKeyMarker == nil && output.NextVersionIdMarker == nil {
			break
		}
		input.KeyMarker = output.NextKeyMarker
		input.VersionIdMarker = output.NextVersionIdMarker
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
