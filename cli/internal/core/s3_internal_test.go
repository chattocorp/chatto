package core

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/config"
)

func TestS3ClientUsesRequiredOnlyRequestChecksums(t *testing.T) {
	cfg := config.S3Config{
		Endpoint:        "r2.example.com",
		Bucket:          "test-bucket",
		AccessKeyID:     "test-key",
		SecretAccessKey: "test-secret",
	}

	client, err := NewS3Client(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)
	require.Equal(t, aws.RequestChecksumCalculationWhenRequired, client.client.Options().RequestChecksumCalculation)
}
