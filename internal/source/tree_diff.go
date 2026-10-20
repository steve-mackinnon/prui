package source

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

const emptyBlobOID = "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391"

type TreeDiffRequest struct {
	BlobDiffRequest
	Path, Status string
}

// TreeDiffBatch uses one pinned tree comparison for simple non-renamed blob
// paths. Requests must carry inventory's conservative byte reservations, and
// A/D paths must not contain other changed paths. Omitted indices were not read
// and may use BlobDiffBatch. Failed attempted reads must not be retried.
func (v *View) TreeDiffBatch(ctx context.Context, oldSHA, newSHA string, requests []TreeDiffRequest) map[int]BlobResult {
	results := map[int]BlobResult{}
	if !shaPattern.MatchString(oldSHA) || !shaPattern.MatchString(newSHA) {
		return results
	}
	var selected []int
	headers := map[string]int{}
	outputLimit, argumentBytes := 0, 0
	for i, request := range requests {
		if err := ctx.Err(); err != nil {
			results[i] = BlobResult{Err: err}
			continue
		}
		if !treeDiffEligible(request) {
			continue
		}
		if cached, ok := v.cachedDiff(request.OldOID+request.NewOID, request.Limit); ok {
			results[i] = cached
			continue
		}
		if request.Limit > v.limits.ContentBytes-outputLimit || argumentBytes+len(request.Path)+1 > 64<<10 || len(selected) >= v.limits.Entries {
			continue
		}
		header := "diff --git a/" + request.Path + " b/" + request.Path
		if _, duplicate := headers[header]; duplicate {
			continue
		}
		headers[header] = i
		selected = append(selected, i)
		outputLimit += request.Limit
		argumentBytes += len(request.Path) + 1
	}
	if len(selected) < 2 {
		return results
	}
	fail := func(err error) map[int]BlobResult {
		for _, i := range selected {
			results[i] = BlobResult{Err: err}
		}
		return results
	}
	args := []string{"--literal-pathspecs", "diff-tree", "-r", "-p", "--no-commit-id"}
	args = append(args, blobDiffPrefix[1:]...)
	args = append(args, oldSHA, newSHA, "--")
	for _, i := range selected {
		args = append(args, requests[i].Path)
	}
	patch, err := v.Git(ctx, outputLimit, args...)
	if err != nil {
		return fail(err)
	}
	prepared := map[int][]byte{}
	for len(patch) > 0 {
		end := len(patch)
		if next := bytes.Index(patch, []byte("\ndiff --git ")); next >= 0 {
			end = next + 1
		}
		file := patch[:end]
		patch = patch[end:]
		header, _, ok := bytes.Cut(file, []byte{'\n'})
		i, expected := headers[string(header)]
		if !ok || !expected {
			return fail(ErrCommand)
		}
		if _, duplicate := prepared[i]; duplicate {
			return fail(ErrCommand)
		}
		canonical, ok := canonicalTreePatch(file, requests[i])
		if !ok {
			return fail(ErrCommand)
		}
		prepared[i] = canonical
	}
	if len(prepared) != len(selected) {
		return fail(ErrCommand)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	// Validate every frame before retaining any result from this process.
	for _, i := range selected {
		data, request := prepared[i], requests[i]
		if len(data) > request.Limit {
			results[i] = BlobResult{Err: ErrLimit}
			continue
		}
		v.cacheDiff(request.OldOID+request.NewOID, data)
		results[i] = BlobResult{Data: data}
	}
	return results
}

func treeDiffEligible(r TreeDiffRequest) bool {
	if r.Limit < 2048 || r.OldOID == r.NewOID || !shaPattern.MatchString(r.OldOID) || !shaPattern.MatchString(r.NewOID) || len(r.Path) == 0 || len(r.Path) > 256 {
		return false
	}
	validStatus := r.Status == "M" || r.Status == "A" && r.OldOID == emptyBlobOID || r.Status == "D" && r.NewOID == emptyBlobOID
	if !validStatus {
		return false
	}
	for _, component := range strings.Split(r.Path, "/") {
		if component == "" || component == "." || component == ".." {
			return false
		}
	}
	for _, c := range []byte(r.Path) {
		validCharacter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-' || c == '/'
		if !validCharacter {
			return false
		}
	}
	return true
}

func canonicalTreePatch(file []byte, request TreeDiffRequest) ([]byte, bool) {
	body := bytes.Index(file, []byte("\n@@ "))
	if body < 0 || len(file) == 0 || file[len(file)-1] != '\n' {
		return nil, false
	}
	old, next := request.OldOID, request.NewOID
	minus, plus := "--- a/"+request.Path, "+++ b/"+request.Path
	if request.Status == "A" {
		old, minus = strings.Repeat("0", 40), "--- /dev/null"
	}
	if request.Status == "D" {
		next, plus = strings.Repeat("0", 40), "+++ /dev/null"
	}
	index, haveMinus, havePlus := false, false, false
	var indexMode, oldMode, newMode, addedMode, deletedMode string
	lines := strings.Split(string(file[:body]), "\n")
	for _, line := range lines[1:] {
		switch {
		case strings.HasPrefix(line, "index ") && !index:
			fields := strings.Fields(line)
			if len(fields) < 2 || len(fields) > 3 || fields[1] != old+".."+next {
				return nil, false
			}
			index = true
			if len(fields) == 3 {
				indexMode = fields[2]
			}
		case line == minus && !haveMinus:
			haveMinus = true
		case line == plus && !havePlus:
			havePlus = true
		case strings.HasPrefix(line, "old mode ") && oldMode == "":
			oldMode = strings.TrimPrefix(line, "old mode ")
		case strings.HasPrefix(line, "new mode ") && newMode == "":
			newMode = strings.TrimPrefix(line, "new mode ")
		case strings.HasPrefix(line, "new file mode ") && addedMode == "":
			addedMode = strings.TrimPrefix(line, "new file mode ")
		case strings.HasPrefix(line, "deleted file mode ") && deletedMode == "":
			deletedMode = strings.TrimPrefix(line, "deleted file mode ")
		default:
			return nil, false
		}
	}
	validMode := func(mode string) bool { return mode == "100644" || mode == "100755" || mode == "120000" }
	validModes := false
	switch request.Status {
	case "M":
		validModes = addedMode == "" && deletedMode == "" && (validMode(indexMode) && oldMode == "" && newMode == "" || indexMode == "" && validMode(oldMode) && validMode(newMode))
	case "A":
		validModes = validMode(addedMode) && indexMode == "" && oldMode == "" && newMode == "" && deletedMode == ""
	case "D":
		validModes = validMode(deletedMode) && indexMode == "" && oldMode == "" && newMode == "" && addedMode == ""
	}
	if !index || !haveMinus || !havePlus || !validModes {
		return nil, false
	}
	canonical := []byte(fmt.Sprintf("diff --git a/%s b/%s\nindex %s..%s 100644\n--- a/%s\n+++ b/%s\n", request.OldOID, request.NewOID, request.OldOID, request.NewOID, request.OldOID, request.NewOID))
	return append(canonical, file[body+1:]...), true
}
