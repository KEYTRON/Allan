package secrets

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

// A keychain that reads fine but refuses to write (locked macOS keychain over
// SSH) must not make pairing fail: the secret lands in the 0600 file instead.
func TestLayeredStoreFallsBackWhenKeyringWriteFails(t *testing.T) {
	keyring.MockInitWithError(errors.New("User interaction is not allowed"))
	dir := t.TempDir()
	l := &layeredStore{keyring: keyringStore{}, file: &fileStore{path: filepath.Join(dir, "secrets.json")}}

	if err := l.Set("keytron-worker", "tok-123"); err != nil {
		t.Fatalf("Set must fall back to the file, got %v", err)
	}
	got, err := l.Get("keytron-worker")
	if err != nil || got != "tok-123" {
		t.Fatalf("Get = %q, %v; want the file copy", got, err)
	}
	names, _ := l.Names()
	if len(names) != 1 || names[0] != "keytron-worker" {
		t.Fatalf("Names = %v", names)
	}
	if err := l.Delete("keytron-worker"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := l.Get("keytron-worker"); err == nil {
		t.Fatal("secret still readable after Delete")
	}
}

func TestLayeredStorePrefersKeyringWhenItWorks(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	file := &fileStore{path: filepath.Join(dir, "secrets.json")}
	_ = file.Set("k", "old-file-copy")
	l := &layeredStore{keyring: keyringStore{}, file: file}

	if err := l.Set("k", "from-keyring"); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.Get("k"); got != "from-keyring" {
		t.Fatalf("Get = %q", got)
	}
	// the stale file copy is removed once the keyring accepted the value
	if _, err := file.Get("k"); err == nil {
		t.Fatal("old file copy was left behind")
	}
}
