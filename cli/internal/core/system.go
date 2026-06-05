package core

import (
	"context"
	"fmt"
	"sort"
)

// GetConnectionInfo returns information about the NATS connection.
type ConnectionInfo struct {
	Connected  bool
	ServerID   string
	ServerName string
	Version    string
	MaxPayload int64
	RTT        string
}

// GetConnectionInfo retrieves NATS connection information.
func (c *ChattoCore) GetConnectionInfo() *ConnectionInfo {
	info := &ConnectionInfo{
		Connected: c.nc.IsConnected(),
	}

	if c.nc.IsConnected() {
		info.ServerID = c.nc.ConnectedServerId()
		info.ServerName = c.nc.ConnectedServerName()
		info.Version = c.nc.ConnectedServerVersion()
		info.MaxPayload = c.nc.MaxPayload()

		if rtt, err := c.nc.RTT(); err == nil {
			info.RTT = rtt.String()
		}
	}

	return info
}

// AccountInfo contains JetStream account limits and usage.
type AccountInfo struct {
	Memory        uint64
	MemoryUsed    uint64
	Storage       uint64
	StorageUsed   uint64
	Streams       int
	StreamsUsed   int
	Consumers     int
	ConsumersUsed int
}

// JetStreamStats contains operator-facing stream and consumer diagnostics.
type JetStreamStats struct {
	Streams              []StreamStats
	Consumers            []ConsumerStats
	TotalMessages        uint64
	TotalBytes           uint64
	TotalConsumerPending uint64
	TotalAckPending      int
}

// StreamStats contains basic JetStream stream state.
type StreamStats struct {
	Name          string
	Description   string
	Subjects      []string
	Storage       string
	Messages      uint64
	Bytes         uint64
	FirstSeq      uint64
	LastSeq       uint64
	ConsumerCount int
	Replicas      int
	ClusterLeader string
}

// ConsumerStats contains basic JetStream consumer state.
type ConsumerStats struct {
	Stream               string
	Name                 string
	Durable              string
	FilterSubject        string
	FilterSubjects       []string
	AckPolicy            string
	PullBased            bool
	PushBound            bool
	Pending              uint64
	AckPending           int
	Redelivered          int
	Waiting              int
	DeliveredConsumerSeq uint64
	DeliveredStreamSeq   uint64
	AckFloorConsumerSeq  uint64
	AckFloorStreamSeq    uint64
}

// GetAccountInfo retrieves JetStream account information.
func (c *ChattoCore) GetAccountInfo(ctx context.Context) (*AccountInfo, error) {
	acc, err := c.js.AccountInfo(ctx)
	if err != nil {
		return nil, err
	}

	return &AccountInfo{
		Memory:        uint64(acc.Limits.MaxMemory),
		MemoryUsed:    acc.Memory,
		Storage:       uint64(acc.Limits.MaxStore),
		StorageUsed:   acc.Store,
		Streams:       acc.Limits.MaxStreams,
		StreamsUsed:   acc.Streams,
		Consumers:     acc.Limits.MaxConsumers,
		ConsumersUsed: acc.Consumers,
	}, nil
}

// GetJetStreamStats lists current JetStream streams and consumers.
func (c *ChattoCore) GetJetStreamStats(ctx context.Context) (*JetStreamStats, error) {
	lister := c.js.ListStreams(ctx)
	stats := &JetStreamStats{}

	for info := range lister.Info() {
		if info == nil {
			continue
		}
		stream := StreamStats{
			Name:          info.Config.Name,
			Description:   info.Config.Description,
			Subjects:      append([]string(nil), info.Config.Subjects...),
			Storage:       info.Config.Storage.String(),
			Messages:      info.State.Msgs,
			Bytes:         info.State.Bytes,
			FirstSeq:      info.State.FirstSeq,
			LastSeq:       info.State.LastSeq,
			ConsumerCount: info.State.Consumers,
			Replicas:      info.Config.Replicas,
		}
		if info.Cluster != nil {
			stream.ClusterLeader = info.Cluster.Leader
		}
		stats.Streams = append(stats.Streams, stream)
		stats.TotalMessages += info.State.Msgs
		stats.TotalBytes += info.State.Bytes

		handle, err := c.js.Stream(ctx, info.Config.Name)
		if err != nil {
			return nil, fmt.Errorf("open stream %s: %w", info.Config.Name, err)
		}
		consumers := handle.ListConsumers(ctx)
		for consumerInfo := range consumers.Info() {
			if consumerInfo == nil {
				continue
			}
			consumer := ConsumerStats{
				Stream:               consumerInfo.Stream,
				Name:                 consumerInfo.Name,
				Durable:              consumerInfo.Config.Durable,
				FilterSubject:        consumerInfo.Config.FilterSubject,
				FilterSubjects:       append([]string(nil), consumerInfo.Config.FilterSubjects...),
				AckPolicy:            consumerInfo.Config.AckPolicy.String(),
				PullBased:            consumerInfo.Config.DeliverSubject == "",
				PushBound:            consumerInfo.PushBound,
				Pending:              consumerInfo.NumPending,
				AckPending:           consumerInfo.NumAckPending,
				Redelivered:          consumerInfo.NumRedelivered,
				Waiting:              consumerInfo.NumWaiting,
				DeliveredConsumerSeq: consumerInfo.Delivered.Consumer,
				DeliveredStreamSeq:   consumerInfo.Delivered.Stream,
				AckFloorConsumerSeq:  consumerInfo.AckFloor.Consumer,
				AckFloorStreamSeq:    consumerInfo.AckFloor.Stream,
			}
			stats.Consumers = append(stats.Consumers, consumer)
			stats.TotalConsumerPending += consumer.Pending
			stats.TotalAckPending += consumer.AckPending
		}
		if err := consumers.Err(); err != nil {
			return nil, fmt.Errorf("list consumers for %s: %w", info.Config.Name, err)
		}
	}
	if err := lister.Err(); err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}

	sort.Slice(stats.Streams, func(i, j int) bool {
		return stats.Streams[i].Name < stats.Streams[j].Name
	})
	sort.Slice(stats.Consumers, func(i, j int) bool {
		if stats.Consumers[i].Stream != stats.Consumers[j].Stream {
			return stats.Consumers[i].Stream < stats.Consumers[j].Stream
		}
		return stats.Consumers[i].Name < stats.Consumers[j].Name
	})

	return stats, nil
}
