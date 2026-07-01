package storage

import "path/filepath"

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
