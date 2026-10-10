package exporter

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/projectionsnapshot"
)

type s3Scanner struct {
	client     *s3.Client
	bucket     string
	pathPrefix string
	timeout    time.Duration

	mu    sync.RWMutex
	stats s3Stats
}

// s3Inventory is one complete listing. Snapshot totals include only keys in
// the private snapshot namespace, without reading payloads or object metadata.
type s3Inventory struct {
	objects, bytes                 int64
	snapshotObjects, snapshotBytes int64
}

type s3Stats struct {
	SnapshotCurrentObjects, SnapshotCurrentBytes int64
	SnapshotAllObjects, SnapshotAllBytes         int64

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
		BaseEndpoint:               aws.String(cfg.EndpointURL()),
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
	current, err := s.scan(ctx, false)
	if err != nil {
		stats.LastSuccess = false
		stats.LastError = err.Error()
		stats.LastDurationSeconds = time.Since(started).Seconds()
		stats.LastRefreshUnixSeconds = time.Now().Unix()
		s.store(stats)
		return
	}
	stats.CurrentObjects = current.objects
	stats.SnapshotCurrentObjects = current.snapshotObjects
	stats.SnapshotCurrentBytes = current.snapshotBytes
	stats.CurrentBytes = current.bytes

	all, err := s.scan(ctx, true)
	if err != nil {
		stats.AllVersionObjects = current.objects
		stats.AllVersionBytes = current.bytes
		stats.NonCurrentObjects = 0
		stats.NonCurrentBytes = 0
		stats.LastSuccess = false
		stats.LastError = err.Error()
		stats.LastDurationSeconds = time.Since(started).Seconds()
		stats.LastRefreshUnixSeconds = time.Now().Unix()
		s.store(stats)
		return
	}
	stats.AllVersionObjects = all.objects
	stats.SnapshotAllObjects = all.snapshotObjects
	stats.SnapshotAllBytes = all.snapshotBytes
	stats.AllVersionBytes = all.bytes
	stats.deriveNonCurrent()
	stats.LastSuccess = true
	stats.LastDurationSeconds = time.Since(started).Seconds()
	stats.LastRefreshUnixSeconds = time.Now().Unix()
	s.store(stats)
}

func (s *s3Scanner) scan(ctx context.Context, withVersions bool) (inventory s3Inventory, err error) {
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
			return s3Inventory{}, err
		}
		for _, object := range output.Contents {
			if aws.ToString(object.Key) == "" {
				continue
			}
			inventory.objects++
			inventory.bytes += aws.ToInt64(object.Size)
			if projectionsnapshot.IsSnapshotObject(strings.TrimPrefix(aws.ToString(object.Key), s.pathPrefix)) {
				inventory.snapshotObjects++
				inventory.snapshotBytes += aws.ToInt64(object.Size)
			}
		}
		if !aws.ToBool(output.IsTruncated) {
			break
		}
		if output.NextContinuationToken == nil || aws.ToString(output.NextContinuationToken) == aws.ToString(input.ContinuationToken) {
			return s3Inventory{}, fmt.Errorf("S3 inventory pagination did not advance")
		}
		input.ContinuationToken = output.NextContinuationToken
	}
	if err := ctx.Err(); err != nil {
		return s3Inventory{}, err
	}
	return inventory, nil
}

func (s *s3Scanner) scanVersions(ctx context.Context) (inventory s3Inventory, err error) {
	input := &s3.ListObjectVersionsInput{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.pathPrefix),
	}
	for {
		output, err := s.client.ListObjectVersions(ctx, input)
		if err != nil {
			return s3Inventory{}, err
		}
		for _, object := range output.Versions {
			if aws.ToString(object.Key) == "" {
				continue
			}
			inventory.objects++
			inventory.bytes += aws.ToInt64(object.Size)
			if projectionsnapshot.IsSnapshotObject(strings.TrimPrefix(aws.ToString(object.Key), s.pathPrefix)) {
				inventory.snapshotObjects++
				inventory.snapshotBytes += aws.ToInt64(object.Size)
			}
		}
		for _, marker := range output.DeleteMarkers {
			if aws.ToString(marker.Key) == "" {
				continue
			}
			inventory.objects++
			if projectionsnapshot.IsSnapshotObject(strings.TrimPrefix(aws.ToString(marker.Key), s.pathPrefix)) {
				inventory.snapshotObjects++
			}
		}
		if !aws.ToBool(output.IsTruncated) {
			break
		}
		if (output.NextKeyMarker == nil && output.NextVersionIdMarker == nil) || (aws.ToString(output.NextKeyMarker) == aws.ToString(input.KeyMarker) && aws.ToString(output.NextVersionIdMarker) == aws.ToString(input.VersionIdMarker)) {
			return s3Inventory{}, fmt.Errorf("S3 version inventory pagination did not advance")
		}
		input.KeyMarker = output.NextKeyMarker
		input.VersionIdMarker = output.NextVersionIdMarker
	}
	if err := ctx.Err(); err != nil {
		return s3Inventory{}, err
	}
	return inventory, nil
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
