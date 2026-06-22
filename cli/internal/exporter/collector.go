package exporter

import (
	"context"

	"github.com/prometheus/client_golang/prometheus"
)

type collector struct {
	server *Server

	buildInfo      *prometheus.Desc
	ready          *prometheus.Desc
	replayComplete *prometheus.Desc
	scrapeError    *prometheus.Desc
	users          *prometheus.Desc
	presence       *prometheus.Desc
	rooms          *prometheus.Desc
	messages       *prometheus.Desc
	assets         *prometheus.Desc
	s3Objects      *prometheus.Desc
	s3Bytes        *prometheus.Desc
	s3Success      *prometheus.Desc
	s3Duration     *prometheus.Desc
	s3LastRefresh  *prometheus.Desc
}

func newCollector(server *Server) *collector {
	return &collector{
		server: server,
		buildInfo: prometheus.NewDesc(
			"chatto_exporter_build_info",
			"Build information for this Chatto exporter.",
			[]string{"version"},
			nil,
		),
		ready: prometheus.NewDesc(
			"chatto_exporter_ready",
			"Whether the Chatto exporter has completed initial EVT replay and can collect deployment-wide metrics.",
			nil,
			nil,
		),
		replayComplete: prometheus.NewDesc(
			"chatto_exporter_evt_replay_complete",
			"Whether the exporter has replayed EVT through the stream tail observed at startup.",
			nil,
			nil,
		),
		scrapeError: prometheus.NewDesc(
			"chatto_exporter_scrape_error",
			"Whether a Chatto exporter collector failed during this scrape.",
			[]string{"collector"},
			nil,
		),
		users: prometheus.NewDesc(
			"chatto_users_total",
			"Current number of registered, non-deleted users by verified-email status.",
			[]string{"email_status"},
			nil,
		),
		presence: prometheus.NewDesc(
			"chatto_presence_users",
			"Current number of users with live presence by status.",
			[]string{"status"},
			nil,
		),
		rooms: prometheus.NewDesc(
			"chatto_rooms_total",
			"Current number of rooms by type.",
			[]string{"type"},
			nil,
		),
		messages: prometheus.NewDesc(
			"chatto_messages_posted_total",
			"Lifetime number of MessagePosted facts by message type.",
			[]string{"type"},
			nil,
		),
		assets: prometheus.NewDesc(
			"chatto_assets_total",
			"Current number of known assets by backend, lifecycle state, and asset kind.",
			[]string{"backend", "state", "kind"},
			nil,
		),
		s3Objects: prometheus.NewDesc(
			"chatto_s3_bucket_objects",
			"Cached S3 bucket object count.",
			[]string{"scope"},
			nil,
		),
		s3Bytes: prometheus.NewDesc(
			"chatto_s3_bucket_bytes",
			"Cached S3 bucket byte size.",
			[]string{"scope"},
			nil,
		),
		s3Success: prometheus.NewDesc(
			"chatto_s3_bucket_refresh_success",
			"Whether the most recent S3 bucket-size refresh succeeded.",
			nil,
			nil,
		),
		s3Duration: prometheus.NewDesc(
			"chatto_s3_bucket_refresh_duration_seconds",
			"Duration of the most recent S3 bucket-size refresh.",
			nil,
			nil,
		),
		s3LastRefresh: prometheus.NewDesc(
			"chatto_s3_bucket_last_refresh_timestamp_seconds",
			"Unix timestamp of the most recent S3 bucket-size refresh.",
			nil,
			nil,
		),
	}
}

func (c *collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.buildInfo
	ch <- c.ready
	ch <- c.replayComplete
	ch <- c.scrapeError
	ch <- c.users
	ch <- c.presence
	ch <- c.rooms
	ch <- c.messages
	ch <- c.assets
	ch <- c.s3Objects
	ch <- c.s3Bytes
	ch <- c.s3Success
	ch <- c.s3Duration
	ch <- c.s3LastRefresh
}

func (c *collector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.buildInfo, prometheus.GaugeValue, 1, c.server.version)

	ctx, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()

	presence, err := c.server.presenceSnapshot(ctx)
	if err != nil {
		ch <- prometheus.MustNewConstMetric(c.scrapeError, prometheus.GaugeValue, 1, "presence")
		presence = map[string]int{}
	} else {
		ch <- prometheus.MustNewConstMetric(c.scrapeError, prometheus.GaugeValue, 0, "presence")
	}

	snapshot := c.server.stats.snapshot(presence)
	ready := snapshot.ReplayComplete && !c.server.replayFailed.Load()
	ch <- prometheus.MustNewConstMetric(c.ready, prometheus.GaugeValue, boolMetric(ready))
	ch <- prometheus.MustNewConstMetric(c.replayComplete, prometheus.GaugeValue, boolMetric(snapshot.ReplayComplete))

	for _, emailStatus := range sortedKeys(snapshot.Users) {
		ch <- prometheus.MustNewConstMetric(c.users, prometheus.GaugeValue, float64(snapshot.Users[emailStatus]), emailStatus)
	}
	for _, status := range sortedKeys(snapshot.Presence) {
		ch <- prometheus.MustNewConstMetric(c.presence, prometheus.GaugeValue, float64(snapshot.Presence[status]), status)
	}
	for _, roomType := range sortedKeys(snapshot.Rooms) {
		ch <- prometheus.MustNewConstMetric(c.rooms, prometheus.GaugeValue, float64(snapshot.Rooms[roomType]), roomType)
	}
	for _, messageType := range sortedKeys(snapshot.Messages) {
		ch <- prometheus.MustNewConstMetric(c.messages, prometheus.GaugeValue, float64(snapshot.Messages[messageType]), messageType)
	}
	for _, key := range sortedKeys(snapshot.Assets) {
		backend, lifecycle, kind := splitAssetKey(key)
		ch <- prometheus.MustNewConstMetric(c.assets, prometheus.GaugeValue, float64(snapshot.Assets[key]), backend, lifecycle, kind)
	}

	c.collectS3(ch)
}

func (c *collector) collectS3(ch chan<- prometheus.Metric) {
	stats := c.server.s3.snapshot()
	if !stats.Configured {
		return
	}
	ch <- prometheus.MustNewConstMetric(c.s3Objects, prometheus.GaugeValue, float64(stats.CurrentObjects), "current")
	ch <- prometheus.MustNewConstMetric(c.s3Bytes, prometheus.GaugeValue, float64(stats.CurrentBytes), "current")
	ch <- prometheus.MustNewConstMetric(c.s3Objects, prometheus.GaugeValue, float64(stats.NonCurrentObjects), "non_current")
	ch <- prometheus.MustNewConstMetric(c.s3Bytes, prometheus.GaugeValue, float64(stats.NonCurrentBytes), "non_current")
	ch <- prometheus.MustNewConstMetric(c.s3Objects, prometheus.GaugeValue, float64(stats.AllVersionObjects), "all_versions")
	ch <- prometheus.MustNewConstMetric(c.s3Bytes, prometheus.GaugeValue, float64(stats.AllVersionBytes), "all_versions")
	ch <- prometheus.MustNewConstMetric(c.s3Success, prometheus.GaugeValue, boolMetric(stats.LastSuccess))
	ch <- prometheus.MustNewConstMetric(c.s3Duration, prometheus.GaugeValue, stats.LastDurationSeconds)
	ch <- prometheus.MustNewConstMetric(c.s3LastRefresh, prometheus.GaugeValue, float64(stats.LastRefreshUnixSeconds))
	ch <- prometheus.MustNewConstMetric(c.scrapeError, prometheus.GaugeValue, inverseBoolMetric(stats.LastSuccess), "s3")
}

func boolMetric(v bool) float64 {
	if v {
		return 1
	}
	return 0
}

func inverseBoolMetric(v bool) float64 {
	if v {
		return 0
	}
	return 1
}

func splitAssetKey(key string) (string, string, string) {
	parts := make([]string, 0, 3)
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] != '|' {
			continue
		}
		parts = append(parts, key[start:i])
		start = i + 1
	}
	parts = append(parts, key[start:])
	for len(parts) < 3 {
		parts = append(parts, "unknown")
	}
	return parts[0], parts[1], parts[2]
}

var _ prometheus.Collector = (*collector)(nil)
