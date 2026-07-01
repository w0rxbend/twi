package storage

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryAssetCacheStoresRecordsWithoutNetwork(t *testing.T) {
	cache := NewMemoryAssetCache()
	record := AssetRecord{
		Key:         AssetKey{Kind: "twitch_emote", ID: "25"},
		Path:        "emotes/25.png",
		MediaType:   "image/png",
		WidthCells:  6,
		HeightCells: 1,
		FetchedAt:   time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC),
	}

	if err := cache.PutAsset(context.Background(), record); err != nil {
		t.Fatalf("PutAsset returned error: %v", err)
	}
	got, ok, err := cache.GetAsset(context.Background(), record.Key)
	if err != nil {
		t.Fatalf("GetAsset returned error: %v", err)
	}
	if !ok {
		t.Fatal("GetAsset ok = false, want true")
	}
	if got != record {
		t.Fatalf("record = %#v, want %#v", got, record)
	}
}

func TestMemoryAssetCacheHonorsContextCancellation(t *testing.T) {
	cache := NewMemoryAssetCache()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	key := AssetKey{Kind: "avatar", ID: "user-1"}
	if _, _, err := cache.GetAsset(ctx, key); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetAsset error = %v, want context.Canceled", err)
	}
	if err := cache.PutAsset(ctx, AssetRecord{Key: key}); !errors.Is(err, context.Canceled) {
		t.Fatalf("PutAsset error = %v, want context.Canceled", err)
	}
}
