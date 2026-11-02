package core

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/stretchr/testify/require"
)

func TestS3ClientUsesRequiredOnlyRequestChecksums(t *testing.T) {
	t.Parallel()

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
