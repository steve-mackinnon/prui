package verify

import (
	"encoding/json"
	"testing"
)

func TestReportJSONContainsOnlyRelativeArtifactPaths(t *testing.T) {
	r := Report{
		Status:    Passed,
		Checks:    []Check{{Name: "open", Status: Passed}},
		Artifacts: Artifacts{Report: "report.json", Transcript: "terminal.txt"},
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"schema_version":1,"status":"passed","checks":[{"name":"open","status":"passed"}],"artifacts":{"report":"report.json","transcript":"terminal.txt"}}` {
		t.Fatalf("report JSON = %s", b)
	}
}

func TestReportRejectsAbsoluteOrTraversingArtifactPaths(t *testing.T) {
	for _, path := range []string{"/tmp/report.json", "../report.json", "screens/../../report.json"} {
		t.Run(path, func(t *testing.T) {
			if err := validateArtifactPath(path); err == nil {
				t.Fatal("accepted unsafe artifact path")
			}
		})
	}
}
