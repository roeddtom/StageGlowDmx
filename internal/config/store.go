package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	mu      sync.RWMutex
	path    string
	current Config
}

func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("create configuration directory: %w", err)
	}
	s := &Store{path: filepath.Join(dataDir, "config.json")}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		// A backup without its primary is evidence of an interrupted/manual recovery,
		// not a new installation. Never hide that evidence with defaults.
		if _, backupErr := os.Stat(s.path + ".bak"); !errors.Is(backupErr, os.ErrNotExist) {
			return nil, fmt.Errorf("configuration missing but backup may exist at %s.bak; inspect and restore it explicitly", s.path)
		}
		s.current = Defaults()
		data, err = encode(s.current)
		if err == nil {
			err = atomicWrite(s.path, data)
		}
		if err != nil {
			return nil, fmt.Errorf("create safe default configuration: %w", err)
		}
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read configuration %s: %w", s.path, err)
	}
	s.current, err = Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid configuration %s: %w; inspect the file or explicitly restore a validated %s.bak backup", s.path, err, s.path)
	}
	return s, nil
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.current)
}

func (s *Store) Save(c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	// Own all slices and pointers before retaining the caller's configuration.
	c = clone(c)
	data, err := encode(c)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	previous, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read previous configuration: %w", err)
	}
	if _, err := Decode(bytes.NewReader(previous)); err != nil {
		return fmt.Errorf("current file is invalid; refusing to replace it or its backup: %w", err)
	}
	if err := atomicWrite(s.path+".bak", previous); err != nil {
		return fmt.Errorf("retain last-known-good backup: %w", err)
	}
	if err := atomicWrite(s.path, data); err != nil {
		return fmt.Errorf("replace configuration: %w", err)
	}
	s.current = c
	return nil
}

func encode(c Config) ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	return append(data, '\n'), err
}

func atomicWrite(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".stageglow-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Same-directory rename is atomic on the Pi's Unix filesystem. Go's Windows
	// implementation replaces existing files without deleting the primary first;
	// if a sharing violation prevents replacement, the old file remains intact.
	return os.Rename(name, path)
}
