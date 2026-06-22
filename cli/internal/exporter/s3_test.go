package exporter

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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

	currentObjects, currentBytes, err := scanner.scan(ctx, false)
	require.NoError(t, err)
	require.Equal(t, int64(2), currentObjects)
	require.Equal(t, int64(10), currentBytes)

	allObjects, allBytes, err := scanner.scan(ctx, true)
	require.NoError(t, err)
	require.Equal(t, int64(2), allObjects)
	require.Equal(t, int64(10), allBytes)
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

	objects, totalBytes, err := scanner.scan(ctx, false)
	require.NoError(t, err)
	require.Equal(t, int64(1), objects)
	require.Equal(t, int64(3), totalBytes)
}
