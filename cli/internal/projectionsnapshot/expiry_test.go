package projectionsnapshot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRepositoryExpireDeletesOnlyOldMarkedPerProjectionGenerations(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	now := repository.now().UTC()

	old := putExpiryObject(t, repository, blobs, "threads", "v1", strings.Repeat("a", 32), now.Add(-8*24*time.Hour))
	recent := putExpiryObject(t, repository, blobs, "users", "v2", strings.Repeat("b", 32), now.Add(-6*24*time.Hour))
	unmarked := putExpiryObject(t, repository, blobs, "threads", "v1", strings.Repeat("c", 32), now.Add(-8*24*time.Hour))
	blobs.purposes[unmarked] = ""
	wrongType := putExpiryObject(t, repository, blobs, "threads", "v1", strings.Repeat("d", 32), now.Add(-8*24*time.Hour))
	blobs.contentTypes[wrongType] = "application/octet-stream"
	legacy := objectRootPrefix + "v1/objects/0123456789abcdef/" + strings.Repeat("e", 32)
	putRawExpiryObject(blobs, legacy, now.Add(-30*24*time.Hour))
	asset := "attachments/" + strings.Repeat("f", 32)
	putRawExpiryObject(blobs, asset, now.Add(-30*24*time.Hour))

	result, err := repository.Expire(context.Background(), ExpireOptions{
		Retention: 7 * 24 * time.Hour, MaxDeletes: 10, MaxDeleteBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedObjects != 1 || result.RecentObjects != 1 || result.IgnoredObjects != 3 {
		t.Fatalf("expiry result = %#v", result)
	}
	if _, ok := blobs.objects[old]; ok {
		t.Fatal("old marked snapshot was not deleted")
	}
	for _, key := range []string{recent, unmarked, wrongType, legacy, asset} {
		if _, ok := blobs.objects[key]; !ok {
			t.Fatalf("non-eligible object %q was deleted", key)
		}
	}
}

func TestRepositoryExpireInventoryOrStatFailureDeletesNothing(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*memoryBlobStore, string)
	}{
		{"inventory", func(blobs *memoryBlobStore, _ string) {
			blobs.failWalk = func(int) error { return errors.New("list failed") }
		}},
		{"stat", func(blobs *memoryBlobStore, key string) {
			blobs.failGet = func(candidate string) bool { return candidate == key }
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			blobs := newMemoryBlobStore()
			repository := newTestRepository(t, blobs, testSecret)
			first := putExpiryObject(t, repository, blobs, "threads", "v1", strings.Repeat("1", 32), repository.now().Add(-8*24*time.Hour))
			second := putExpiryObject(t, repository, blobs, "users", "v2", strings.Repeat("2", 32), repository.now().Add(-8*24*time.Hour))
			test.setup(blobs, first)
			if _, err := repository.Expire(context.Background(), ExpireOptions{Retention: 7 * 24 * time.Hour, MaxDeletes: 10, MaxDeleteBytes: 1024}); err == nil {
				t.Fatal("Expire succeeded despite injected failure")
			}
			for _, key := range []string{first, second} {
				if _, ok := blobs.objects[key]; !ok {
					t.Fatalf("failure deleted %q", key)
				}
			}
		})
	}
}

func TestRepositoryExpireBoundsDeletionBatch(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	for index, id := range []string{strings.Repeat("3", 32), strings.Repeat("4", 32), strings.Repeat("5", 32)} {
		key := putExpiryObject(t, repository, blobs, "threads", "v1", id, repository.now().Add(-8*24*time.Hour))
		blobs.objects[key] = make([]byte, index+1)
	}
	result, err := repository.Expire(context.Background(), ExpireOptions{
		Retention: 7 * 24 * time.Hour, MaxDeletes: 1, MaxDeleteBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedObjects != 1 || !result.DeleteLimitHit || result.EligibleObjects != 3 {
		t.Fatalf("bounded expiry result = %#v", result)
	}
}

func TestRepositoryExpireToleratesCandidateRemovedBeforeDelete(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	vanished := putExpiryObject(t, repository, blobs, "threads", "v1", strings.Repeat("6", 32), repository.now().Add(-8*24*time.Hour))
	remaining := putExpiryObject(t, repository, blobs, "users", "v2", strings.Repeat("7", 32), repository.now().Add(-8*24*time.Hour))
	blobs.beforeDelete = func(key string) {
		blobs.beforeDelete = nil
		if key == vanished {
			delete(blobs.objects, key)
		}
	}

	result, err := repository.Expire(context.Background(), ExpireOptions{
		Retention: 7 * 24 * time.Hour, MaxDeletes: 10, MaxDeleteBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedObjects != 1 {
		t.Fatalf("expiry result = %#v", result)
	}
	if _, ok := blobs.objects[remaining]; ok {
		t.Fatal("expiry stopped after an idempotent missing-object deletion")
	}
}

func TestRepositoryExpireCurrentAndPreviousCausesColdReplay(t *testing.T) {
	blobs := newMemoryBlobStore()
	repository := newTestRepository(t, blobs, testSecret)
	if _, err := repository.Save(context.Background(), testSaveInput(1, []byte("previous"))); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Save(context.Background(), testSaveInput(2, []byte("current"))); err != nil {
		t.Fatal(err)
	}
	for key := range blobs.objects {
		blobs.modified[key] = repository.now().Add(-8 * 24 * time.Hour)
	}

	result, err := repository.Expire(context.Background(), ExpireOptions{
		Retention: 7 * 24 * time.Hour, MaxDeletes: 10, MaxDeleteBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedObjects != 2 {
		t.Fatalf("expiry result = %#v", result)
	}
	if _, err := repository.Load(context.Background(), "threads", "v1", "EVT", testStreamIdentity, 2); !errors.Is(err, ErrBlobNotFound) {
		t.Fatalf("Load after expiry error = %v, want missing generation", err)
	}
}

func TestGenerationObjectKeyParserRejectsPrefixConfusion(t *testing.T) {
	valid := objectRootPrefix + "room_directory/tracking-v1/objects/0123456789abcdef/" + strings.Repeat("a", 32)
	if !isGenerationObjectKey(valid) {
		t.Fatal("valid generation key rejected")
	}
	for _, key := range []string{
		"attachments/" + valid,
		objectRootPrefix + "v1/objects/0123456789abcdef/" + strings.Repeat("a", 32),
		objectRootPrefix + "threads/v1/objects/0123456789abcdef/" + strings.Repeat("a", 32) + "/asset",
		objectRootPrefix + "threads/v1/objects/0123456789abcdeg/" + strings.Repeat("a", 32),
		objectRootPrefix + "threads/v1/objects/0123456789abcdef/" + strings.Repeat("A", 32),
		objectRootPrefix + "../assets/v1/objects/0123456789abcdef/" + strings.Repeat("a", 32),
	} {
		if isGenerationObjectKey(key) {
			t.Fatalf("unsafe generation key accepted: %q", key)
		}
	}
}

func putExpiryObject(t *testing.T, repository *Repository, blobs *memoryBlobStore, projection, version, id string, modified time.Time) string {
	t.Helper()
	key := repository.generationObjectKey(projection, version, id)
	putRawExpiryObject(blobs, key, modified)
	return key
}

func putRawExpiryObject(blobs *memoryBlobStore, key string, modified time.Time) {
	blobs.objects[key] = []byte("encrypted snapshot")
	blobs.modified[key] = modified
	blobs.contentTypes[key] = contentType
	blobs.purposes[key] = objectPurpose
}
