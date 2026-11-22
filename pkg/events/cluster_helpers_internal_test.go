package events

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
)

// testCluster is a three-node NATS cluster with JetStream on loopback ports.
// Production runs JetStream as a three-node cluster with replicated streams.
type testCluster struct {
	servers []*server.Server
}

// startTestCluster starts three clustered JetStream servers and waits until
// the cluster has a meta leader.
func startTestCluster(t *testing.T) *testCluster {
	t.Helper()
	const size = 3
	clusterPorts := make([]int, size)
	for i := range clusterPorts {
		clusterPorts[i] = freeTCPPort(t)
	}
	routes := make([]*url.URL, size)
	for i, port := range clusterPorts {
		routes[i] = &url.URL{Scheme: "nats", Host: fmt.Sprintf("127.0.0.1:%d", port)}
	}
	cluster := &testCluster{}
	for i := range size {
		natsServer, err := server.NewServer(&server.Options{
			ServerName: fmt.Sprintf("node-%d", i),
			Host:       "127.0.0.1",
			Port:       server.RANDOM_PORT,
			JetStream:  true,
			StoreDir:   t.TempDir(),
			NoSigs:     true,
			Cluster: server.ClusterOpts{
				Name: "events-test",
				Host: "127.0.0.1",
				Port: clusterPorts[i],
			},
			Routes: routes,
		})
		if err != nil {
			t.Fatalf("create NATS server %d: %v", i, err)
		}
		natsServer.Start()
		t.Cleanup(func() {
			natsServer.Shutdown()
			natsServer.WaitForShutdown()
		})
		if !natsServer.ReadyForConnections(10 * time.Second) {
			t.Fatalf("NATS server %d did not become ready", i)
		}
		cluster.servers = append(cluster.servers, natsServer)
	}
	deadline := time.Now().Add(20 * time.Second)
	for cluster.metaLeader() == nil {
		if time.Now().After(deadline) {
			t.Fatal("cluster has no JetStream meta leader")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cluster
}

// freeTCPPort returns a loopback port that was free when it was checked.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port
}

func (c *testCluster) metaLeader() *server.Server {
	for _, s := range c.servers {
		if s.Running() && s.JetStreamIsLeader() {
			return s
		}
	}
	return nil
}

// urls returns the client URLs of the running servers.
func (c *testCluster) urls() string {
	var urls []string
	for _, s := range c.servers {
		if s.Running() {
			urls = append(urls, s.ClientURL())
		}
	}
	return strings.Join(urls, ",")
}

// server returns the running server with name.
func (c *testCluster) server(name string) *server.Server {
	for _, s := range c.servers {
		if s.Running() && s.Name() == name {
			return s
		}
	}
	return nil
}
