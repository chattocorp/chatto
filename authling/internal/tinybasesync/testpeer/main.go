// testpeer exposes the experimental peer over newline-delimited JSON.
// It exists only for the pinned JavaScript interoperability test.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"hmans.de/authling/internal/tinybasesync"
)

func main() {
	statePath := flag.String("state", "", "path to durable peer state")
	peerCount := flag.Int("peers", 1, "number of live peers sharing the state")
	flag.Parse()
	if *statePath == "" {
		fmt.Fprintln(os.Stderr, "testpeer: -state is required")
		os.Exit(2)
	}
	if *peerCount < 1 || *peerCount > 8 {
		fmt.Fprintln(os.Stderr, "testpeer: -peers must be between 1 and 8")
		os.Exit(2)
	}
	store := &tinybasesync.FileStore{Path: *statePath}
	peers := make([]*tinybasesync.Peer, *peerCount)
	for index := range peers {
		peer, err := tinybasesync.NewPeer(context.Background(), store)
		if err != nil {
			fmt.Fprintf(os.Stderr, "testpeer: %v\n", err)
			os.Exit(1)
		}
		peers[index] = peer
	}

	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var input struct {
			Peer int `json:"peer"`
			tinybasesync.Envelope
		}
		if err := json.Unmarshal(scanner.Bytes(), &input); err != nil || input.Peer < 0 || input.Peer >= len(peers) {
			fmt.Fprintf(os.Stderr, "testpeer: decode message: %v\n", err)
			os.Exit(1)
		}
		outbound, err := peers[input.Peer].Handle(context.Background(), input.Envelope)
		if err != nil {
			fmt.Fprintf(os.Stderr, "testpeer: handle message: %v\n", err)
			os.Exit(1)
		}
		for _, message := range outbound {
			if err := encoder.Encode(message); err != nil {
				fmt.Fprintf(os.Stderr, "testpeer: encode message: %v\n", err)
				os.Exit(1)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "testpeer: read message: %v\n", err)
		os.Exit(1)
	}
}
