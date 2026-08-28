package evtstream

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/events"
)

// nopLogger satisfies events.Logger for worker-construction tests.
type nopLogger struct{}

func (nopLogger) Debug(interface{}, ...interface{}) {}
func (nopLogger) Info(interface{}, ...interface{})  {}
func (nopLogger) Warn(interface{}, ...interface{})  {}
func (nopLogger) Error(interface{}, ...interface{}) {}

func newEffectTestStream(t *testing.T) (context.Context, jetstream.Stream) {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "EVT_EFFECT_TEST",
		Subjects: []string{"evt.>", "notifications.>"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctx, stream
}

func TestCreateEffectConsumerAppliesStandardEffectPolicies(t *testing.T) {
	ctx, stream := newEffectTestStream(t)

	consumer, err := CreateEffectConsumer(ctx, stream, EffectConsumerConfig{
		Name:           "test-effect",
		Description:    "unit contract check",
		FilterSubjects: []string{"evt.room.*.message_posted", "evt.room.*.message_edited"},
		AckWait:        2 * time.Minute,
		MaxAckPending:  16,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := consumer.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	config := info.Config
	if config.Name != "test-effect" || config.Durable != "test-effect" {
		t.Fatalf("consumer name = %q durable = %q", config.Name, config.Durable)
	}
	if config.Description != "unit contract check" {
		t.Fatalf("description = %q", config.Description)
	}
	// Two subjects must persist via the plural field.
	if len(config.FilterSubjects) != 2 || config.FilterSubjects[0] != "evt.room.*.message_posted" || config.FilterSubjects[1] != "evt.room.*.message_edited" || config.FilterSubject != "" {
		t.Fatalf("filter subjects = %q / %q", config.FilterSubject, config.FilterSubjects)
	}
	if config.DeliverPolicy != jetstream.DeliverAllPolicy {
		t.Fatalf("deliver policy = %v", config.DeliverPolicy)
	}
	if config.AckPolicy != jetstream.AckExplicitPolicy {
		t.Fatalf("ack policy = %v", config.AckPolicy)
	}
	if config.MaxDeliver != -1 {
		t.Fatalf("max deliver = %d, want unlimited redelivery", config.MaxDeliver)
	}
	if config.ReplayPolicy != jetstream.ReplayInstantPolicy {
		t.Fatalf("replay policy = %v", config.ReplayPolicy)
	}
	if config.AckWait != 2*time.Minute {
		t.Fatalf("ack wait = %v", config.AckWait)
	}
	if config.MaxAckPending != 16 || config.MaxRequestBatch != 16 {
		t.Fatalf("max ack pending = %d max request batch = %d", config.MaxAckPending, config.MaxRequestBatch)
	}
}

func TestCreateEffectConsumerSupportsCreationBoundaryStart(t *testing.T) {
	ctx, stream := newEffectTestStream(t)

	consumer, err := CreateEffectConsumer(ctx, stream, EffectConsumerConfig{
		Name:           "test-effect-new",
		Description:    "creation-boundary consumer",
		FilterSubjects: []string{"notifications.signalled"},
		AckWait:        time.Minute,
		MaxAckPending:  1,
		DeliverPolicy:  jetstream.DeliverNewPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := consumer.Info(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if info.Config.DeliverPolicy != jetstream.DeliverNewPolicy {
		t.Fatalf("deliver policy = %v, want creation-boundary start", info.Config.DeliverPolicy)
	}
	// A single filter subject must persist via the singular field, matching
	// the historical single-subject effect consumers.
	if info.Config.FilterSubject != "notifications.signalled" || len(info.Config.FilterSubjects) != 0 {
		t.Fatalf("filter subject = %q subjects = %v", info.Config.FilterSubject, info.Config.FilterSubjects)
	}
}

// TestCreateEffectConsumerUpdatePathStable re-applies identical configs to an
// existing consumer, covering both filter-field representations. CreateOrUpdate
// must not flip a historical singular-filter consumer to plural representation
// (or vice versa) when an upgraded binary reproduces its contract.
func TestCreateEffectConsumerUpdatePathStable(t *testing.T) {
	ctx, stream := newEffectTestStream(t)

	singular := EffectConsumerConfig{
		Name:           "update-singular",
		Description:    "update path",
		FilterSubjects: []string{"evt.room.*.message_posted"},
		AckWait:        time.Minute,
		MaxAckPending:  2,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	}
	if _, err := CreateEffectConsumer(ctx, stream, singular); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		consumer, err := stream.Consumer(ctx, "update-singular")
		if err != nil {
			t.Fatal(err)
		}
		info, err := consumer.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if info.Config.FilterSubject != "evt.room.*.message_posted" || len(info.Config.FilterSubjects) != 0 {
			t.Fatalf("pass %d: singular consumer drifted: %+v", i, info.Config)
		}
		if _, err := CreateEffectConsumer(ctx, stream, singular); err != nil {
			t.Fatal(err)
		}
	}

	plural := EffectConsumerConfig{
		Name:           "update-plural",
		Description:    "update path",
		FilterSubjects: []string{"evt.room.*.message_posted", "evt.room.*.message_edited"},
		AckWait:        time.Minute,
		MaxAckPending:  2,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	}
	if _, err := CreateEffectConsumer(ctx, stream, plural); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		consumer, err := stream.Consumer(ctx, "update-plural")
		if err != nil {
			t.Fatal(err)
		}
		info, err := consumer.Info(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(info.Config.FilterSubjects) != 2 || info.Config.FilterSubject != "" {
			t.Fatalf("pass %d: plural consumer drifted: %+v", i, info.Config)
		}
		if _, err := CreateEffectConsumer(ctx, stream, plural); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreateEffectConsumerRejectsInvalidContracts(t *testing.T) {
	ctx, stream := newEffectTestStream(t)

	cases := []struct {
		name   string
		config EffectConsumerConfig
	}{
		{
			name:   "missing name",
			config: EffectConsumerConfig{FilterSubjects: []string{"evt.x"}, AckWait: time.Minute, MaxAckPending: 1, DeliverPolicy: jetstream.DeliverAllPolicy},
		},
		{
			name:   "no filter subjects",
			config: EffectConsumerConfig{Name: "nofilter", AckWait: time.Minute, MaxAckPending: 1, DeliverPolicy: jetstream.DeliverAllPolicy},
		},
		{
			name:   "empty ack wait",
			config: EffectConsumerConfig{Name: "noackwait", FilterSubjects: []string{"evt.x"}, MaxAckPending: 1, DeliverPolicy: jetstream.DeliverAllPolicy},
		},
		{
			name:   "zero max ack pending",
			config: EffectConsumerConfig{Name: "nomaxpending", FilterSubjects: []string{"evt.x"}, AckWait: time.Minute, DeliverPolicy: jetstream.DeliverAllPolicy},
		},
		{
			name:   "empty filter subject entry",
			config: EffectConsumerConfig{Name: "emptysubject", FilterSubjects: []string{"evt.x", ""}, AckWait: time.Minute, MaxAckPending: 1, DeliverPolicy: jetstream.DeliverAllPolicy},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := CreateEffectConsumer(ctx, stream, tc.config); err == nil {
				t.Fatalf("CreateEffectConsumer(%s) succeeded, want contract error", tc.name)
			}
		})
	}
}

func TestEffectWorkerOptionsPreserveDeliveryKnobs(t *testing.T) {
	ctx, stream := newEffectTestStream(t)

	consumer, err := CreateEffectConsumer(ctx, stream, EffectConsumerConfig{
		Name:           "test-worker-options",
		Description:    "worker option passthrough",
		FilterSubjects: []string{"evt.room.*.message_posted"},
		AckWait:        time.Minute,
		MaxAckPending:  4,
		DeliverPolicy:  jetstream.DeliverAllPolicy,
	})
	if err != nil {
		t.Fatal(err)
	}

	worker, err := NewEffectWorker(consumer, func(context.Context, events.DurableDelivery) error { return nil }, EffectWorkerOptions{
		MaxConcurrent:     4,
		RetryDelay:        30 * time.Second,
		AckTimeout:        5 * time.Second,
		HeartbeatInterval: 30 * time.Second,
		Logger:            nopLogger{},
	})
	if err != nil {
		t.Fatal(err)
	}
	opts := worker.Options()
	if opts.MaxConcurrent != 4 {
		t.Fatalf("max concurrent = %d", opts.MaxConcurrent)
	}
	// NewEffectWorker leaves FetchMaxWait unset; DurableWorker construction
	// resolves zero to its own default, which must remain one second to match
	// every historical effect site.
	if opts.FetchMaxWait != time.Second {
		t.Fatalf("resolved fetch max wait = %v, want framework default %v", opts.FetchMaxWait, time.Second)
	}
	if opts.RetryDelay != 30*time.Second || opts.AckTimeout != 5*time.Second || opts.HeartbeatInterval != 30*time.Second {
		t.Fatalf("delivery knobs = %+v", opts)
	}
}
