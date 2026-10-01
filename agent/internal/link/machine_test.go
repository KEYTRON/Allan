package link

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMachineIDIsStable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	a := MachineID()
	b := MachineID()
	if a != b {
		t.Fatalf("id changed between calls: %q vs %q", a, b)
	}
	if !validMachineID(a) {
		t.Fatalf("not a UUID: %q", a)
	}
	if short := ShortID(a); len(short) != 8 || !strings.HasPrefix(a, short) {
		t.Fatalf("bad short id %q for %q", short, a)
	}
}

func TestMachineIDRepairsGarbage(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	first := MachineID()
	path := filepath.Join(configDir(), "machine_id")
	if err := os.WriteFile(path, []byte("not-an-id\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second := MachineID()
	if !validMachineID(second) || second == first {
		t.Fatalf("garbage was not replaced: %q (was %q)", second, first)
	}
}

func TestValidMachineID(t *testing.T) {
	if validMachineID("") || validMachineID("123") || validMachineID(strings.Repeat("g", 36)) {
		t.Fatal("accepted an invalid id")
	}
}
