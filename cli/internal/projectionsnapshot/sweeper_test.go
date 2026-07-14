package projectionsnapshot

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

var sweepNow = time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)

func newSweepRepository(t *testing.T, blobs *memoryBlobStore, secret string) *Repository {
	t.Helper()
	blobs.now = func() time.Time { return sweepNow }
	repository, err := NewRepository(blobs, RepositoryOptions{
		SecretHex:       secret,
		ProducerVersion: "sweep-test",
		Now:             func() time.Time { return sweepNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	return repository
}

func sweepOptions() SweepOptions {
	return SweepOptions{
		ProjectionKeys: []string{"threads"},
		GracePeriod:    24 * time.Hour,
		MaxDeletes:     100,
		MaxDeleteBytes: 1 << 30,
	}
}

func putSweepObject(blobs *memoryBlobStore, id string, modified time.Time, size int) string {
	key := generationObjectKey(id)
	blobs.objects[key] = make([]byte, size)
	blobs.modified[key] = modified
	return key
}

func TestRepositorySweepRetainsReferencesAndRecentGenerations(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newSweepRepository(t, blobs, testSecret)
	first, err := repository.Save(ctx, testSaveInput(10, []byte("first")))
	if err != nil {
		t.Fatal(err)
	}
	second, err := repository.Save(ctx, testSaveInput(20, []byte("second")))
	if err != nil {
		t.Fatal(err)
	}
	blobs.modified[generationObjectKey(first.GenerationID)] = sweepNow.Add(-30 * 24 * time.Hour)
	blobs.modified[generationObjectKey(second.GenerationID)] = sweepNow.Add(-30 * 24 * time.Hour)
	oldOrphan := putSweepObject(blobs, strings.Repeat("a", 32), sweepNow.Add(-25*time.Hour), 17)
	recentOrphan := putSweepObject(blobs, strings.Repeat("b", 32), sweepNow.Add(-23*time.Hour), 19)
	invalid := objectPrefix + "objects/not-a-generation"
	blobs.objects[invalid] = []byte("unknown")
	blobs.modified[invalid] = sweepNow.Add(-30 * 24 * time.Hour)
	invalidPointer := objectPrefix + "pointers/future-format"
	blobs.objects[invalidPointer] = []byte("unknown")
	blobs.modified[invalidPointer] = sweepNow.Add(-30 * 24 * time.Hour)

	result, err := repository.Sweep(ctx, sweepOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.ScannedObjects != 7 || result.ReferencedObjects != 2 || result.ActivePointers != 1 || result.RecentObjects != 1 || result.EligibleObjects != 1 || result.IgnoredObjects != 2 {
		t.Fatalf("unexpected inventory result: %#v", result)
	}
	if result.DeletedObjects != 1 || result.DeletedBytes != 17 || result.DeleteLimitHit {
		t.Fatalf("unexpected deletion result: %#v", result)
	}
	if _, ok := blobs.objects[oldOrphan]; ok {
		t.Fatal("old unreferenced generation was retained")
	}
	for _, key := range []string{generationObjectKey(first.GenerationID), generationObjectKey(second.GenerationID), recentOrphan, invalid, invalidPointer, repository.pointerKey("threads")} {
		if _, ok := blobs.objects[key]; !ok {
			t.Fatalf("protected object %q was deleted", key)
		}
	}
}

func TestRepositorySweepProtectsEveryRegisteredProjection(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newSweepRepository(t, blobs, testSecret)
	threads, err := repository.Save(ctx, testSaveInput(10, []byte("threads")))
	if err != nil {
		t.Fatal(err)
	}
	roomsInput := testSaveInput(20, []byte("rooms"))
	roomsInput.ProjectionKey = "rooms"
	rooms, err := repository.Save(ctx, roomsInput)
	if err != nil {
		t.Fatal(err)
	}
	for key := range blobs.objects {
		blobs.modified[key] = sweepNow.Add(-48 * time.Hour)
	}
	orphan := putSweepObject(blobs, strings.Repeat("a", 32), sweepNow.Add(-48*time.Hour), 10)
	opts := sweepOptions()
	opts.ProjectionKeys = []string{"rooms", "threads", "rooms"}

	result, err := repository.Sweep(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.ReferencedObjects != 2 || result.ActivePointers != 2 || result.DeletedObjects != 1 {
		t.Fatalf("multi-projection result = %#v", result)
	}
	for _, key := range []string{generationObjectKey(threads.GenerationID), generationObjectKey(rooms.GenerationID), repository.pointerKey("threads"), repository.pointerKey("rooms")} {
		if _, ok := blobs.objects[key]; !ok {
			t.Fatalf("registered projection object %q was deleted", key)
		}
	}
	if _, ok := blobs.objects[orphan]; ok {
		t.Fatal("old orphan was retained")
	}
}

func TestRepositorySweepGraceProtectsGenerationPublishedAfterInventoryStarts(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	repository := newSweepRepository(t, blobs, testSecret)
	first, err := repository.Save(ctx, testSaveInput(10, []byte("first")))
	if err != nil {
		t.Fatal(err)
	}
	for key := range blobs.objects {
		blobs.modified[key] = sweepNow.Add(-48 * time.Hour)
	}
	orphan := putSweepObject(blobs, strings.Repeat("a", 32), sweepNow.Add(-48*time.Hour), 10)
	var published LoadedSnapshot
	blobs.walkHook = func(call int, _ string) {
		if call != 1 || published.GenerationID != "" {
			return
		}
		published, err = repository.Save(ctx, testSaveInput(20, []byte("published during sweep")))
		if err != nil {
			t.Fatal(err)
		}
	}

	result, err := repository.Sweep(ctx, sweepOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedObjects != 1 {
		t.Fatalf("concurrent publication result = %#v", result)
	}
	for _, generation := range []LoadedSnapshot{first, published} {
		if _, ok := blobs.objects[generationObjectKey(generation.GenerationID)]; !ok {
			t.Fatalf("generation %q was deleted", generation.GenerationID)
		}
	}
	if _, ok := blobs.objects[orphan]; ok {
		t.Fatal("old orphan was retained")
	}
	loaded, err := repository.Load(ctx, "threads", testCompatibilityID, "EVT", testStreamIdentity, 20)
	if err != nil || loaded.GenerationID != published.GenerationID {
		t.Fatalf("concurrently published generation did not remain loadable: loaded=%#v err=%v", loaded, err)
	}
}

func TestRepositorySweepTreatsGraceBoundaryAsEligible(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newSweepRepository(t, blobs, testSecret)
	key := putSweepObject(blobs, strings.Repeat("a", 32), sweepNow.Add(-24*time.Hour), 1)

	result, err := repository.Sweep(context.Background(), sweepOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedObjects != 1 {
		t.Fatalf("boundary result = %#v", result)
	}
	if _, ok := blobs.objects[key]; ok {
		t.Fatal("generation exactly at grace boundary was retained")
	}
}

func TestRepositorySweepPointerFailuresDeleteNothing(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*memoryBlobStore, *Repository)
	}{
		{
			name: "transient read",
			mutate: func(blobs *memoryBlobStore, repository *Repository) {
				blobs.failGet = func(key string) bool { return key == repository.pointerKey("threads") }
			},
		},
		{
			name: "invalid pointer",
			mutate: func(blobs *memoryBlobStore, repository *Repository) {
				blobs.objects[repository.pointerKey("threads")][envelopeHeaderSize] ^= 1
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			blobs := newMemoryBlobStore()
			repository := newSweepRepository(t, blobs, testSecret)
			if _, err := repository.Save(ctx, testSaveInput(1, []byte("current"))); err != nil {
				t.Fatal(err)
			}
			orphan := putSweepObject(blobs, strings.Repeat("a", 32), sweepNow.Add(-48*time.Hour), 10)
			test.mutate(blobs, repository)

			result, err := repository.Sweep(ctx, sweepOptions())
			if err == nil {
				t.Fatal("Sweep succeeded with unreadable pointer")
			}
			if result.DeletedObjects != 0 || blobs.walkCalls != 0 {
				t.Fatalf("pointer failure reached inventory or deletion: result=%#v walks=%d", result, blobs.walkCalls)
			}
			if _, ok := blobs.objects[orphan]; !ok {
				t.Fatal("pointer failure deleted orphan")
			}
		})
	}
}

func TestRepositorySweepIncompleteInventoryDeletesNothing(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newSweepRepository(t, blobs, testSecret)
	orphan := putSweepObject(blobs, strings.Repeat("a", 32), sweepNow.Add(-48*time.Hour), 10)
	blobs.failWalk = func(call int) error {
		if call == 1 {
			return errors.New("injected listing failure")
		}
		return nil
	}

	result, err := repository.Sweep(context.Background(), sweepOptions())
	if err == nil || !strings.Contains(err.Error(), "inventory projection snapshot objects") {
		t.Fatalf("Sweep error = %v", err)
	}
	if result.DeletedObjects != 0 || blobs.walkCalls != 1 {
		t.Fatalf("incomplete inventory result=%#v walks=%d", result, blobs.walkCalls)
	}
	if _, ok := blobs.objects[orphan]; !ok {
		t.Fatal("incomplete inventory deleted orphan")
	}
}

func TestRepositorySweepStopsWhenSecondPassListingFails(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newSweepRepository(t, blobs, testSecret)
	orphan := putSweepObject(blobs, strings.Repeat("a", 32), sweepNow.Add(-48*time.Hour), 10)
	blobs.failWalk = func(call int) error {
		if call == 2 {
			return errors.New("injected second-pass listing failure")
		}
		return nil
	}

	result, err := repository.Sweep(context.Background(), sweepOptions())
	if err == nil || !strings.Contains(err.Error(), "sweep projection snapshot objects") {
		t.Fatalf("Sweep error = %v", err)
	}
	if result.EligibleObjects != 1 || result.DeletedObjects != 0 || blobs.walkCalls != 2 {
		t.Fatalf("second-pass failure result=%#v walks=%d", result, blobs.walkCalls)
	}
	if _, ok := blobs.objects[orphan]; !ok {
		t.Fatal("second-pass listing failure deleted orphan")
	}
}

func TestRepositorySweepStopsOnOwnershipOrDeleteFailure(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*memoryBlobStore, *SweepOptions)
	}{
		{
			name: "ownership",
			mutate: func(_ *memoryBlobStore, opts *SweepOptions) {
				opts.BeforeDelete = func(context.Context) error { return errors.New("lease lost") }
			},
		},
		{
			name: "delete",
			mutate: func(blobs *memoryBlobStore, _ *SweepOptions) {
				blobs.failDelete = func(string) bool { return true }
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			blobs := newMemoryBlobStore()
			repository := newSweepRepository(t, blobs, testSecret)
			orphan := putSweepObject(blobs, strings.Repeat("a", 32), sweepNow.Add(-48*time.Hour), 10)
			opts := sweepOptions()
			test.mutate(blobs, &opts)

			result, err := repository.Sweep(context.Background(), opts)
			if err == nil {
				t.Fatal("Sweep succeeded despite injected failure")
			}
			if result.DeletedObjects != 0 {
				t.Fatalf("failure result = %#v", result)
			}
			if _, ok := blobs.objects[orphan]; !ok {
				t.Fatal("failed deletion removed orphan")
			}
		})
	}
}

func TestRepositorySweepBoundsEachPass(t *testing.T) {
	for _, test := range []struct {
		name     string
		maxCount int
		maxBytes int64
		want     int
	}{
		{name: "objects", maxCount: 2, maxBytes: 1000, want: 2},
		{name: "bytes", maxCount: 10, maxBytes: 15, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			blobs := newMemoryBlobStore()
			repository := newSweepRepository(t, blobs, testSecret)
			for _, id := range []string{strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)} {
				putSweepObject(blobs, id, sweepNow.Add(-48*time.Hour), 10)
			}
			opts := sweepOptions()
			opts.MaxDeletes = test.maxCount
			opts.MaxDeleteBytes = test.maxBytes

			result, err := repository.Sweep(context.Background(), opts)
			if err != nil {
				t.Fatal(err)
			}
			if result.DeletedObjects != test.want || !result.DeleteLimitHit {
				t.Fatalf("bounded result = %#v, want %d deletions", result, test.want)
			}
		})
	}
}

func TestRepositorySweepTreatsInvalidSizesConservativelyAndSaturatesTotals(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newSweepRepository(t, blobs, testSecret)
	largeA := putSweepObject(blobs, strings.Repeat("a", 32), sweepNow.Add(-48*time.Hour), 1)
	largeB := putSweepObject(blobs, strings.Repeat("b", 32), sweepNow.Add(-48*time.Hour), 1)
	invalid := putSweepObject(blobs, strings.Repeat("c", 32), sweepNow.Add(-48*time.Hour), 1)
	blobs.walkInfo = func(_ int, info BlobInfo) BlobInfo {
		switch info.Key {
		case largeA, largeB:
			info.Size = math.MaxInt64
		case invalid:
			info.Size = -1
		}
		return info
	}
	opts := sweepOptions()
	opts.MaxDeleteBytes = math.MaxInt64

	result, err := repository.Sweep(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if result.ScannedBytes != math.MaxInt64 || result.EligibleBytes != math.MaxInt64 {
		t.Fatalf("byte totals wrapped: %#v", result)
	}
	if result.EligibleObjects != 2 || result.IgnoredObjects != 1 || result.DeletedObjects != 1 || !result.DeleteLimitHit {
		t.Fatalf("size validation result = %#v", result)
	}
	if _, ok := blobs.objects[invalid]; !ok {
		t.Fatal("object with invalid negative size was deleted")
	}
}

func TestRepositorySweepAfterSecretRotationRemovesOldGenerationsAndPointer(t *testing.T) {
	ctx := context.Background()
	blobs := newMemoryBlobStore()
	oldRepository := newSweepRepository(t, blobs, testSecret)
	if _, err := oldRepository.Save(ctx, testSaveInput(1, []byte("old"))); err != nil {
		t.Fatal(err)
	}
	oldPointer := oldRepository.pointerKey("threads")
	for key := range blobs.objects {
		blobs.modified[key] = sweepNow.Add(-48 * time.Hour)
	}
	newRepository := newSweepRepository(t, blobs, strings.Repeat("11", 32))

	result, err := newRepository.Sweep(ctx, sweepOptions())
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedObjects != 2 {
		t.Fatalf("rotation result = %#v", result)
	}
	if _, ok := blobs.objects[oldPointer]; ok {
		t.Fatal("cleanup retained opaque pointer from prior key scheme")
	}
}

func TestRepositorySweepHonorsCancellationBeforeDeletion(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newSweepRepository(t, blobs, testSecret)
	orphan := putSweepObject(blobs, strings.Repeat("a", 32), sweepNow.Add(-48*time.Hour), 10)
	ctx, cancel := context.WithCancel(context.Background())
	blobs.walkHook = func(call int, _ string) {
		if call == 1 {
			cancel()
		}
	}

	result, err := repository.Sweep(ctx, sweepOptions())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Sweep error = %v, want context cancellation", err)
	}
	if result.DeletedObjects != 0 {
		t.Fatalf("cancelled result = %#v", result)
	}
	if _, ok := blobs.objects[orphan]; !ok {
		t.Fatal("cancelled inventory deleted orphan")
	}
}
