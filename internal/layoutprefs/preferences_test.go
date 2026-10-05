package layoutprefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreferencesRestartAndCorruption(t *testing.T) {
	p := filepath.Join(t.TempDir(), "preferences.json")
	want := Preferences{Version: 1, Split: true, RailWidth: 81, CommitWidth: 42, GroupFiles: true, CollapseGenerated: true}
	if e := Save(p, want); e != nil {
		t.Fatal(e)
	}
	got, e := Load(p)
	if e != nil || got != want {
		t.Fatalf("%+v %v", got, e)
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	for _, bad := range []string{`{"version":2}`, `{"version":1,"rail_width":-4}`, `{"version":1,"unknown":true}`, `{"version":1} garbage`} {
		if e := os.WriteFile(p, []byte(bad), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := Load(p); e == nil {
			t.Fatal("accepted bad prefs")
		}
		if e := Save(p, want); e == nil {
			t.Fatal("overwrote invalid preferences")
		}
		b, _ := os.ReadFile(p)
		if string(b) != bad {
			t.Fatal("corruption replaced")
		}
	}
}

func TestPreferencesRejectNonregularAndOversized(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "layout.json")
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, p); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := Save(p, Preferences{Version: 1}); err == nil {
		t.Fatal("symlink overwritten")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(p, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("directory accepted")
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, make([]byte, 4097), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("oversized file accepted")
	}
}
