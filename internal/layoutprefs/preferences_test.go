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
