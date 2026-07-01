package storage

import (
	"context"
	"path/filepath"
	"sync"
	"time"
)

type Cache struct {
	Root string
}

func NewCache(root string) Cache {
	return Cache{Root: root}
}

func (c Cache) Path(parts ...string) string {
	items := append([]string{c.Root}, parts...)
	return filepath.Join(items...)
}

// AssetKey identifies a cached image or metadata asset without embedding a
// remote URL or any credential-bearing request data.
type AssetKey struct {
	Kind string
	ID   string
}

// AssetRecord describes a cached asset that can be handed to an image
// renderer by an asynchronous caller. Path may be empty for metadata-only
// entries or tests.
type AssetRecord struct {
	Key         AssetKey
	Path        string
	MediaType   string
	WidthCells  int
	HeightCells int
	FetchedAt   time.Time
	ExpiresAt   time.Time
}

// AssetCache is the minimal context-aware cache boundary for image assets.
// Implementations must not require network access for reads or writes.
type AssetCache interface {
	GetAsset(ctx context.Context, key AssetKey) (AssetRecord, bool, error)
	PutAsset(ctx context.Context, record AssetRecord) error
}

// MemoryAssetCache is a deterministic in-memory AssetCache for tests and
// early fallback-only wiring. It performs no file or network I/O.
type MemoryAssetCache struct {
	mu      sync.RWMutex
	records map[AssetKey]AssetRecord
}

var _ AssetCache = (*MemoryAssetCache)(nil)

// NewMemoryAssetCache returns an empty context-aware in-memory asset cache.
func NewMemoryAssetCache() *MemoryAssetCache {
	return &MemoryAssetCache{
		records: make(map[AssetKey]AssetRecord),
	}
}

func (c *MemoryAssetCache) GetAsset(ctx context.Context, key AssetKey) (AssetRecord, bool, error) {
	if err := ctx.Err(); err != nil {
		return AssetRecord{}, false, err
	}
	if c == nil {
		return AssetRecord{}, false, nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	record, ok := c.records[key]
	return record, ok, nil
}

func (c *MemoryAssetCache) PutAsset(ctx context.Context, record AssetRecord) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.records == nil {
		c.records = make(map[AssetKey]AssetRecord)
	}
	c.records[record.Key] = record
	return nil
}
