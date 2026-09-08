package evaluation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCorpus(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []Case
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	results := Run(cases)
	if len(results) != 5 {
		t.Fatalf("got %d corpus results", len(results))
	}
	for _, r := range results {
		if r.Status != "valid" || r.Owned != r.Total {
			t.Fatalf("incomplete corpus case: %+v", r)
		}
	}
	t.Log(Summary(results))
}
