// Package secrets keeps provider API keys out of config.toml.
//
// The system keyring (Secret Service / KWallet / GNOME Keyring on Linux,
// Keychain on macOS, Credential Manager on Windows) is preferred. Where no
// keyring is reachable (headless servers, SSH sessions without D-Bus) keys
// fall back to a separate secrets.json readable only by the owner (0600).
package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/zalando/go-keyring"
)

const service = "allan"

// indexUser is the keyring entry that lists stored provider names,
// since keyrings cannot enumerate entries portably.
const indexUser = "__index__"

var ErrNotFound = errors.New("secret not found")

type Store interface {
	Get(name string) (string, error)
	Set(name, secret string) error
	Delete(name string) error
	Names() ([]string, error)
	// Kind describes where secrets live, for display to the user.
	Kind() string
}

// Open returns the keyring store when it is usable, otherwise the file store
// in dir. Set ALLAN_SECRETS=file to force the file store.
func Open(dir string) Store {
	file := &fileStore{path: filepath.Join(dir, "secrets.json")}
	if os.Getenv("ALLAN_SECRETS") == "file" {
		return file
	}
	if _, err := keyring.Get(service, indexUser); err == nil || errors.Is(err, keyring.ErrNotFound) {
		return &keyringStore{}
	}
	return file
}

type keyringStore struct{}

func (keyringStore) Kind() string { return "system keyring" }

func (keyringStore) Get(name string) (string, error) {
	s, err := keyring.Get(service, name)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return s, err
}

func (k keyringStore) Set(name, secret string) error {
	if err := keyring.Set(service, name, secret); err != nil {
		return err
	}
	return k.updateIndex(name, true)
}

func (k keyringStore) Delete(name string) error {
	err := keyring.Delete(service, name)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return k.updateIndex(name, false)
}

func (keyringStore) Names() ([]string, error) {
	raw, err := keyring.Get(service, indexUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal([]byte(raw), &names); err != nil {
		return nil, err
	}
	return names, nil
}

func (k keyringStore) updateIndex(name string, present bool) error {
	names, err := k.Names()
	if err != nil {
		return err
	}
	set := map[string]bool{}
	for _, n := range names {
		set[n] = true
	}
	if present {
		set[name] = true
	} else {
		delete(set, name)
	}
	buf, _ := json.Marshal(sortedKeys(set))
	return keyring.Set(service, indexUser, string(buf))
}

type fileStore struct {
	path string
	mu   sync.Mutex
}

func (f *fileStore) Kind() string { return "file " + f.path + " (0600)" }

func (f *fileStore) load() (map[string]string, error) {
	data, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", f.path, err)
	}
	return m, nil
}

func (f *fileStore) save(m map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(f.path, buf, 0o600)
}

func (f *fileStore) Get(name string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return "", err
	}
	s, ok := m[name]
	if !ok {
		return "", ErrNotFound
	}
	return s, nil
}

func (f *fileStore) Set(name, secret string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	m[name] = secret
	return f.save(m)
}

func (f *fileStore) Delete(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := m[name]; !ok {
		return nil
	}
	delete(m, name)
	return f.save(m)
}

func (f *fileStore) Names() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for n := range m {
		set[n] = true
	}
	return sortedKeys(set), nil
}

// WriteFileAtomic writes via a temp file in the same directory and renames it,
// so a crash never leaves a half-written file and the mode applies from the start.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
