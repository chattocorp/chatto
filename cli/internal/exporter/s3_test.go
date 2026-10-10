package exporter

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/testutil/fakes3"
)

func TestS3NonCurrentStats(t *testing.T) {
	stats := s3Stats{
		CurrentObjects:    2,
		CurrentBytes:      100,
		AllVersionObjects: 5,
		AllVersionBytes:   175,
	}
	stats.deriveNonCurrent()

	require.Equal(t, int64(3), stats.NonCurrentObjects)
	require.Equal(t, int64(75), stats.NonCurrentBytes)
}

func TestS3NonCurrentStatsNeverNegative(t *testing.T) {
	stats := s3Stats{
		CurrentObjects:    5,
		CurrentBytes:      175,
		AllVersionObjects: 2,
		AllVersionBytes:   100,
	}
	stats.deriveNonCurrent()

	require.Equal(t, int64(0), stats.NonCurrentObjects)
	require.Equal(t, int64(0), stats.NonCurrentBytes)
}

func TestS3ScannerScansCurrentAndVersionedObjectsWithPrefix(t *testing.T) {
	s3Server := fakes3.NewServer(t)
	useSSL := false
	pathStyle := true
	assets := config.AssetsConfig{
		StorageBackend: config.StorageBackendS3,
		S3: config.S3Config{
			Endpoint:        s3Server.EndpointHost(),
			Bucket:          "test-bucket",
			PathPrefix:      "tenant-a/chatto",
			AccessKeyID:     "test-key",
			SecretAccessKey: "test-secret",
			UseSSL:          &useSSL,
			PathStyle:       &pathStyle,
		},
	}

	scanner, err := newS3Scanner(assets, time.Second)
	require.NoError(t, err)
	require.NotNil(t, scanner)
	require.Equal(t, aws.RequestChecksumCalculationWhenRequired, scanner.client.Options().RequestChecksumCalculation)

	ctx := context.Background()
	_, err = scanner.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(scanner.bucket),
	})
	require.NoError(t, err)
	putObject := func(key, body string) {
		t.Helper()
		_, err := scanner.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(scanner.bucket),
			Key:           aws.String(key),
			Body:          bytes.NewReader([]byte(body)),
			ContentLength: aws.Int64(int64(len(body))),
			ContentType:   aws.String("text/plain"),
		})
		require.NoError(t, err)
	}
	putObject("tenant-a/chatto/one.txt", "one")
	putObject("tenant-a/chatto/two.txt", "two-two")
	putObject("tenant-b/chatto/ignored.txt", "ignored")

	current, err := scanner.scan(ctx, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), current.objects)
	require.Equal(t, int64(10), current.bytes)

	all, err := scanner.scan(ctx, true)
	require.NoError(t, err)
	require.Equal(t, int64(2), all.objects)
	require.Equal(t, int64(10), all.bytes)
}

func TestS3ScannerDefaultsToPathStyleForCustomEndpoint(t *testing.T) {
	s3Server := fakes3.NewServer(t)
	useSSL := false
	assets := config.AssetsConfig{
		StorageBackend: config.StorageBackendS3,
		S3: config.S3Config{
			Endpoint:        s3Server.EndpointHost(),
			Bucket:          "test-bucket",
			AccessKeyID:     "test-key",
			SecretAccessKey: "test-secret",
			UseSSL:          &useSSL,
		},
	}

	scanner, err := newS3Scanner(assets, time.Second)
	require.NoError(t, err)
	require.NotNil(t, scanner)

	ctx := context.Background()
	_, err = scanner.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(scanner.bucket),
	})
	require.NoError(t, err)
	_, err = scanner.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(scanner.bucket),
		Key:           aws.String("one.txt"),
		Body:          bytes.NewReader([]byte("one")),
		ContentLength: aws.Int64(3),
		ContentType:   aws.String("text/plain"),
	})
	require.NoError(t, err)

	inventory, err := scanner.scan(ctx, false)
	require.NoError(t, err)
	require.Equal(t, int64(1), inventory.objects)
	require.Equal(t, int64(3), inventory.bytes)
}

// One listing supplies both bucket and snapshot totals. Versioned totals include
// old versions and delete markers, which must not disappear from storage metrics.
func TestSnapshotInventoryPaginationAndUnavailableMetrics(t *testing.T) {
	for _, failure := range []string{"none", "current", "versions", "pagination"} {
		t.Run(failure, func(t *testing.T) {
			requests := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/xml")
				if r.URL.Query().Has("versions") && failure == "versions" {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `<Error><Code>AccessDenied</Code></Error>`)
				} else if r.URL.Query().Has("versions") {
					fmt.Fprint(w, `<ListVersionsResult><IsTruncated>false</IsTruncated><Version><Key>tenant/internal/projection-snapshots/a</Key><Size>5</Size></Version><Version><Key>tenant/internal/projection-snapshots/a</Key><Size>3</Size></Version><DeleteMarker><Key>tenant/internal/projection-snapshots/old</Key></DeleteMarker></ListVersionsResult>`)
				} else if r.URL.Query().Get("continuation-token") == "" {
					fmt.Fprint(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken><Contents><Key>tenant/asset</Key><Size>100</Size></Contents></ListBucketResult>`)
				} else if failure == "pagination" {
					fmt.Fprint(w, `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>next</NextContinuationToken></ListBucketResult>`)
				} else if failure == "current" {
					w.WriteHeader(http.StatusForbidden)
					fmt.Fprint(w, `<Error><Code>AccessDenied</Code></Error>`)
				} else {
					fmt.Fprint(w, `<ListBucketResult><IsTruncated>false</IsTruncated><Contents><Key>tenant/internal/projection-snapshots/a</Key><Size>5</Size></Contents><Contents><Key>tenant/internal/projection-snapshots-other/a</Key><Size>40</Size></Contents></ListBucketResult>`)
				}
			}))
			defer provider.Close()
			scanner := &s3Scanner{client: s3.New(s3.Options{BaseEndpoint: aws.String(provider.URL), Region: "test", UsePathStyle: true, Credentials: credentials.NewStaticCredentialsProvider("test", "test", "")}), bucket: "test", pathPrefix: "tenant/", timeout: time.Second, stats: s3Stats{Configured: true}}
			collector := newCollector(&Server{s3: scanner})
			// Scrapes use only the cache, including before any inventory is available.
			require.NotContains(t, snapshotInventoryText(t, collector), "chatto_projection_snapshot_storage_objects{")
			require.Zero(t, requests)
			scanner.refresh(context.Background())
			before := requests
			output := snapshotInventoryText(t, collector)
			require.Equal(t, before, requests)
			if failure != "none" {
				require.False(t, scanner.snapshot().LastSuccess)
				require.Contains(t, output, `chatto_projection_snapshot_storage_refresh_success{backend="s3"} 0`)
				require.NotContains(t, output, "chatto_projection_snapshot_storage_objects{")
			} else {
				require.True(t, scanner.snapshot().LastSuccess)
				require.Equal(t, 3, requests)
				require.Contains(t, output, `chatto_projection_snapshot_storage_objects{backend="s3",scope="current"} 1`)
				require.Contains(t, output, `chatto_projection_snapshot_storage_objects{backend="s3",scope="all_versions"} 3`)
				require.Contains(t, output, `chatto_projection_snapshot_storage_bytes{backend="s3",scope="non_current"} 3`)
			}
		})
	}
}

// snapshotInventoryCollector isolates the cached S3 collector from unrelated
// exporter NATS/presence reads while preserving Prometheus descriptor checks.
type snapshotInventoryCollector struct{ c *collector }

func (c snapshotInventoryCollector) Describe(ch chan<- *prometheus.Desc) { c.c.Describe(ch) }
func (c snapshotInventoryCollector) Collect(ch chan<- prometheus.Metric) { c.c.collectS3(ch) }
func snapshotInventoryText(t *testing.T, c *collector) string {
	t.Helper()
	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(snapshotInventoryCollector{c})
	families, err := registry.Gather()
	require.NoError(t, err)
	var out bytes.Buffer
	for _, family := range families {
		_, err := expfmt.MetricFamilyToText(&out, family)
		require.NoError(t, err)
	}
	return out.String()
}
