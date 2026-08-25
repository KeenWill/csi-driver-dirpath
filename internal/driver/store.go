package driver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

var volumeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type volumeMetadata struct {
	ID            string            `json:"id"`
	Name          string            `json:"name"`
	NodeID        string            `json:"nodeID"`
	CapacityBytes int64             `json:"capacityBytes"`
	Parameters    map[string]string `json:"parameters,omitempty"`
}

type volumeStore struct {
	basePath string
	volumes  string
	metadata string
}

func newVolumeStore(basePath string) *volumeStore {
	return &volumeStore{
		basePath: basePath,
		volumes:  filepath.Join(basePath, "volumes"),
		metadata: filepath.Join(basePath, ".dirpath-meta", "volumes"),
	}
}

func validVolumeID(id string) bool {
	return volumeIDPattern.MatchString(id)
}

func (s *volumeStore) create(v volumeMetadata) error {
	if !validVolumeID(v.ID) {
		return fmt.Errorf("invalid volume id %q", v.ID)
	}
	if err := s.ensureDirectory(v.ID); err != nil {
		return err
	}
	if err := os.MkdirAll(s.metadata, 0o700); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	temporary, err := os.CreateTemp(s.metadata, ".volume-*")
	if err != nil {
		return fmt.Errorf("create metadata file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set metadata permissions: %w", err)
	}
	if err := json.NewEncoder(temporary).Encode(v); err != nil {
		temporary.Close()
		return fmt.Errorf("encode metadata: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync metadata: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close metadata: %w", err)
	}
	if err := os.Rename(temporaryName, s.metadataPath(v.ID)); err != nil {
		return fmt.Errorf("publish metadata: %w", err)
	}
	return nil
}

func (s *volumeStore) load(id string) (volumeMetadata, error) {
	if !validVolumeID(id) {
		return volumeMetadata{}, fmt.Errorf("invalid volume id %q", id)
	}
	file, err := os.Open(s.metadataPath(id))
	if err != nil {
		return volumeMetadata{}, err
	}
	defer file.Close()
	var v volumeMetadata
	if err := json.NewDecoder(file).Decode(&v); err != nil {
		return volumeMetadata{}, fmt.Errorf("decode metadata: %w", err)
	}
	if v.ID != id {
		return volumeMetadata{}, fmt.Errorf("metadata id %q does not match filename", v.ID)
	}
	return v, nil
}

func (s *volumeStore) ensureDirectory(id string) error {
	if !validVolumeID(id) {
		return fmt.Errorf("invalid volume id %q", id)
	}
	if err := os.MkdirAll(s.volumePath(id), 0o750); err != nil {
		return fmt.Errorf("create volume directory: %w", err)
	}
	return nil
}

func (s *volumeStore) delete(id string) error {
	if !validVolumeID(id) {
		return fmt.Errorf("invalid volume id %q", id)
	}
	if err := os.RemoveAll(s.volumePath(id)); err != nil {
		return fmt.Errorf("remove volume directory: %w", err)
	}
	if err := os.Remove(s.metadataPath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove volume metadata: %w", err)
	}
	return nil
}

func (s *volumeStore) deleteEntry(name string) error {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return fmt.Errorf("invalid directory entry %q", name)
	}
	if err := os.RemoveAll(filepath.Join(s.volumes, name)); err != nil {
		return err
	}
	if validVolumeID(name) {
		if err := os.Remove(s.metadataPath(name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (s *volumeStore) volumePath(id string) string {
	return filepath.Join(s.volumes, id)
}

func (s *volumeStore) metadataPath(id string) string {
	return filepath.Join(s.metadata, id+".json")
}
