package inventory

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"prui/internal/source"
)

func TestSafetyHostileRepository(t *testing.T) {
	old, p, r := fixture(t)
	_ = old.Close()
	sentinel := filepath.Join(t.TempDir(), "executed")
	script := filepath.Join(r.Dir, "hostile.sh")
	r.Write("hostile.sh", "#!/bin/sh\nprintf executed > '"+sentinel+"'\nexit 97\n")
	if e := os.Chmod(script, 0700); e != nil {
		t.Fatal(e)
	}
	r.Write(".git/config", "[core]\nrepositoryformatversion=0\nbare=false\nhooksPath="+r.Dir+"/hooks\n[diff]\nexternal="+script+"\n[diff \"hostile\"]\ncommand="+script+"\ntextconv="+script+"\n[filter \"hostile\"]\nclean="+script+"\nsmudge="+script+"\n[credential]\nhelper=!"+script+"\n[remote \"origin\"]\npromisor=true\nurl=ext::"+script+"\n[extensions]\npartialClone=origin\n[url \"ext::"+script+"\"]\ninsteadOf=https://github.com/\n")
	r.Write(".git/info/attributes", "* diff=hostile filter=hostile\n")
	r.Write(".gitattributes", "* diff=hostile filter=hostile\n")
	for _, hook := range []string{"post-checkout", "post-merge", "pre-auto-gc", "reference-transaction"} {
		r.Write("hooks/"+hook, "#!/bin/sh\nexec '"+script+"'\n")
		if e := os.Chmod(filepath.Join(r.Dir, "hooks", hook), 0700); e != nil {
			t.Fatal(e)
		}
	}
	r.Write(".git/objects/info/alternates", r.Dir+"/hostile-alternate\n")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "diff.external")
	t.Setenv("GIT_CONFIG_VALUE_0", script)
	t.Setenv("GIT_EXTERNAL_DIFF", script)
	t.Setenv("GIT_EXEC_PATH", r.Dir)
	before := r.Snapshot()
	v, e := source.NewView(context.Background(), r.Dir, source.NewRunner(), source.Defaults())
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	inv, e := Build(context.Background(), v, p, source.Defaults())
	if e != nil || !inv.Complete {
		t.Fatal("hostile fixture failed", e)
	}
	if _, e = os.Stat(sentinel); !errors.Is(e, fs.ErrNotExist) {
		t.Fatal("repository code executed")
	}
	if !reflect.DeepEqual(before, r.Snapshot()) {
		t.Fatal("HEAD/refs/index/worktree/config/objects changed")
	}
}
