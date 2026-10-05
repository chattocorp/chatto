package events

import (
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

func streamSequenceFromMsg(msg jetstream.Msg) (uint64, error) {
	return streamSequenceFromReply(msg.Reply())
}

func streamSequenceFromReply(reply string) (uint64, error) {
	const jsAckPrefix = "$JS.ACK."
	if len(reply) < len(jsAckPrefix) || reply[:len(jsAckPrefix)] != jsAckPrefix {
		return 0, fmt.Errorf("invalid JetStream ACK reply subject")
	}

	var v1Start, v1End int
	var v2Start, v2End int
	tokenStart := 0
	tokenIndex := 0
	for i := 0; i <= len(reply); i++ {
		if i != len(reply) && reply[i] != '.' {
			continue
		}
		switch tokenIndex {
		case 5:
			v1Start, v1End = tokenStart, i
		case 7:
			v2Start, v2End = tokenStart, i
		}
		tokenIndex++
		tokenStart = i + 1
	}

	switch {
	case tokenIndex == 9:
		return parseAckSequenceToken(reply[v1Start:v1End])
	case tokenIndex >= 11:
		return parseAckSequenceToken(reply[v2Start:v2End])
	default:
		return 0, fmt.Errorf("invalid JetStream ACK reply subject")
	}
}

func parseAckSequenceToken(token string) (uint64, error) {
	if token == "" {
		return 0, fmt.Errorf("invalid JetStream ACK stream sequence")
	}
	var n uint64
	for i := 0; i < len(token); i++ {
		c := token[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid JetStream ACK stream sequence")
		}
		digit := uint64(c - '0')
		if n > (^uint64(0)-digit)/10 {
			return 0, fmt.Errorf("invalid JetStream ACK stream sequence")
		}
		n = n*10 + digit
	}
	return n, nil
}
