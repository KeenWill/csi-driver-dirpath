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
	volumes       string
	metadata      string
	removeAll     func(string) error
	syncDirectory func(string) error
}

func newVolumeStore(basePath string) *volumeStore {
	return &volumeStore{
		volumes:       filepath.Join(basePath, "volumes"),
		metadata:      filepath.Join(basePath, ".dirpath-meta", "volumes"),
		removeAll:     os.RemoveAll,
		syncDirectory: syncDirectory,
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
	if err := s.mkdirAllDurable(s.metadata, 0o700); err != nil {
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
	if err := s.syncDirectory(s.metadata); err != nil {
		return fmt.Errorf("sync metadata directory: %w", err)
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
	if err := s.mkdirAllDurable(s.volumePath(id), 0o750); err != nil {
		return fmt.Errorf("create volume directory: %w", err)
	}
	return nil
}

func (s *volumeStore) delete(id string) error {
	if !validVolumeID(id) {
		return fmt.Errorf("invalid volume id %q", id)
	}
	if err := s.removeAll(s.volumePath(id)); err != nil {
		return fmt.Errorf("remove volume directory: %w", err)
	}
	if err := s.syncIfExists(s.volumes); err != nil {
		return fmt.Errorf("sync volume directory: %w", err)
	}
	if err := os.Remove(s.metadataPath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove volume metadata: %w", err)
	}
	if err := s.syncIfExists(s.metadata); err != nil {
		return fmt.Errorf("sync metadata directory: %w", err)
	}
	return nil
}

func (s *volumeStore) deleteEntry(name string) error {
	if filepath.Base(name) != name || name == "." || name == ".." {
		return fmt.Errorf("invalid directory entry %q", name)
	}
	if err := s.removeAll(filepath.Join(s.volumes, name)); err != nil {
		return err
	}
	if err := s.syncIfExists(s.volumes); err != nil {
		return err
	}
	if validVolumeID(name) {
		if err := os.Remove(s.metadataPath(name)); err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := s.syncIfExists(s.metadata); err != nil {
			return err
		}
	}
	return nil
}

func (s *volumeStore) mkdirAllDurable(path string, mode os.FileMode) error {
	missing := make([]string, 0, 2)
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s is not a directory", current)
			}
			break
		}
		if !os.IsNotExist(err) {
			return err
		}
		missing = append(missing, current)
		if parent := filepath.Dir(current); parent == current {
			break
		}
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	for index := len(missing) - 1; index >= 0; index-- {
		if err := s.syncDirectory(filepath.Dir(missing[index])); err != nil {
			return err
		}
	}
	return nil
}

func (s *volumeStore) syncIfExists(path string) error {
	if err := s.syncDirectory(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (s *volumeStore) volumePath(id string) string {
	return filepath.Join(s.volumes, id)
}

func (s *volumeStore) metadataPath(id string) string {
	return filepath.Join(s.metadata, id+".json")
}
