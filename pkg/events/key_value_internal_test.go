package events

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

func TestJetStreamAPIPrefixMatchesKeyValuePuts(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts jetstream.JetStreamOptions
		want string
	}{
		{"default API", jetstream.JetStreamOptions{APIPrefix: jetstream.DefaultAPIPrefix}, ""},
		{"no prefix", jetstream.JetStreamOptions{}, ""},
		{"default API without dot", jetstream.JetStreamOptions{APIPrefix: "$JS.API"}, ""},
		{"domain", jetstream.JetStreamOptions{Domain: "hub"}, "$JS.hub.API."},
		{"API prefix", jetstream.JetStreamOptions{APIPrefix: "tenant.API"}, "tenant.API."},
		{"API prefix with dot", jetstream.JetStreamOptions{APIPrefix: "tenant.API."}, "tenant.API."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := jetStreamAPIPrefix(tc.opts); got != tc.want {
				t.Fatalf("jetStreamAPIPrefix = %q, want %q", got, tc.want)
			}
		})
	}
}
