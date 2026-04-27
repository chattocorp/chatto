package core

import (
	"context"
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
