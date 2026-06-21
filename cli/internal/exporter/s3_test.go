package exporter

import (
	"testing"

	"github.com/stretchr/testify/require"
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
