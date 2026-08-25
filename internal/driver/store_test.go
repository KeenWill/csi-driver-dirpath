package driver

import (
	"path/filepath"
	"testing"
)

func TestCreateDurablySyncsMetadataPath(t *testing.T) {
	basePath := t.TempDir()
	store := newVolumeStore(BasePath(basePath))
	originalSync := store.syncDirectory
	var synced []string
	store.syncDirectory = func(path string) error {
		synced = append(synced, path)
		return originalSync(path)
	}

	if err := store.create(volumeMetadata{ID: "vol-durable", Name: "durable", NodeID: "node-a"}); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		basePath,
		filepath.Join(basePath, ".dirpath-meta"),
		store.metadata,
	} {
		if !containsPath(synced, required) {
			t.Fatalf("directory %s was not synced; synced: %v", required, synced)
		}
	}
	if last := synced[len(synced)-1]; last != store.metadata {
		t.Fatalf("last synced directory = %s, want metadata directory %s", last, store.metadata)
	}
}

func containsPath(paths []string, target string) bool {
	for _, path := range paths {
		if path == target {
			return true
		}
	}
	return false
}
