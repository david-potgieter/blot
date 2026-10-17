// Package queries persists saved scan queries as JSON.
package queries

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
)

// Query is a saved set of form values, stored as typed.
type Query struct {
	Label string `json:"label"`
	Roots string `json:"roots"`
	Min   string `json:"min"`
	Show  string `json:"show"`
	Name  string `json:"name"`
}

// Store reads and writes queries at Path.
type Store struct {
	Path string
}

// DefaultPath is os.UserConfigDir()/blot/queries.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "blot", "queries.json"), nil
}

func defaultStore() (Store, error) {
	p, err := DefaultPath()
	return Store{Path: p}, err
}

// Save stores q under label in the default file.
func Save(label string, q Query) error {
	s, err := defaultStore()
	if err != nil {
		return err
	}
	return s.Save(label, q)
}

// Load returns the query saved under label in the default file.
func Load(label string) (Query, error) {
	s, err := defaultStore()
	if err != nil {
		return Query{}, err
	}
	return s.Load(label)
}

// Delete removes label from the default file.
func Delete(label string) error {
	s, err := defaultStore()
	if err != nil {
		return err
	}
	return s.Delete(label)
}

// List returns the sorted labels in the default file.
func List() ([]string, error) {
	s, err := defaultStore()
	if err != nil {
		return nil, err
	}
	return s.List()
}

// List returns all saved query labels, sorted.
func (s Store) List() ([]string, error) {
	all, err := s.read()
	if err != nil {
		return nil, err
	}
	return slices.Sorted(maps.Keys(all)), nil
}

// Load returns the query saved under label.
func (s Store) Load(label string) (Query, error) {
	all, err := s.read()
	if err != nil {
		return Query{}, err
	}
	q, ok := all[label]
	if !ok {
		return Query{}, fmt.Errorf("no saved query %q", label)
	}
	return q, nil
}

// Save stores q under label, replacing any existing query with that label.
func (s Store) Save(label string, q Query) error {
	all, err := s.read()
	if err != nil {
		return err
	}
	q.Label = label
	all[label] = q
	return s.write(all)
}

// Delete removes the query saved under label.
func (s Store) Delete(label string) error {
	all, err := s.read()
	if err != nil {
		return err
	}
	delete(all, label)
	return s.write(all)
}

func (s Store) read() (map[string]Query, error) {
	all := map[string]Query{}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return all, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, fmt.Errorf("corrupt queries file %s: %w", s.Path, err)
	}
	return all, nil
}

// write replaces the file atomically: temp file in the same dir, then rename.
func (s Store) write(all map[string]Query) error {
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".queries-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.Path)
}
