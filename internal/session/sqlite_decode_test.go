package session

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestSourceCanonicalDecodeRejectsAmbiguousJSON(t *testing.T) {
	_, canonical, err := encodeSource(commitFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{`{`, `"inventory":{`, `"Comparison":{`, `"Metadata":{`, `"Files":[{`, `"Units":[{`, `"context":{`, `"commits":{`, `"Entries":[{`, `"Diff":{`} {
		t.Run("unknown "+needle, func(t *testing.T) {
			if !bytes.Contains(canonical, []byte(needle)) {
				t.Fatal("fixture does not exercise nested object")
			}
			data := bytes.Replace(canonical, []byte(needle), []byte(needle+`"unexpected":true,`), 1)
			if _, err := decodeSource(digest(data), data); !errors.Is(err, ErrInvalidRecord) {
				t.Fatalf("unknown field accepted: %v", err)
			}
		})
	}
	variants := map[string][]byte{
		"leading whitespace":  append([]byte(" "), canonical...),
		"trailing whitespace": append(append([]byte{}, canonical...), '\n'),
		"trailing value":      append(append([]byte{}, canonical...), []byte(`{}`)...),
		"duplicate version":   bytes.Replace(canonical, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1),
		"escaped field":       bytes.Replace(canonical, []byte(`"version"`), []byte(`"\u0076ersion"`), 1),
		"escaped value":       bytes.Replace(canonical, []byte(`"file"`), []byte(`"\u0066ile"`), 1),
		"wrong version":       bytes.Replace(canonical, []byte(`"version":1`), []byte(`"version":2`), 1),
		"invalid UTF8":        bytes.Replace(canonical, []byte(`"Alice"`), append(append([]byte{'"'}, 0xff), '"'), 1),
	}
	for name, data := range variants {
		t.Run(name, func(t *testing.T) {
			if bytes.Equal(data, canonical) {
				t.Fatal("mutation did not change fixture")
			}
			if _, err := decodeSource(digest(data), data); !errors.Is(err, ErrInvalidRecord) {
				t.Fatalf("noncanonical source accepted: %v", err)
			}
		})
	}
	if _, err := decodeSource(digest([]byte("other")), canonical); !errors.Is(err, ErrInvalidRecord) {
		t.Fatal("wrong source digest accepted")
	}
	for _, snapshot := range []Snapshot{fixture(), commitFixture()} {
		_, data, err := encodeSource(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := decodeSource(digest(data), data)
		if err != nil || !reflect.DeepEqual(sourcePayload(snapshot), sourcePayload(decoded)) {
			t.Fatalf("canonical source changed: %v", err)
		}
	}
}

// This decoder optimization is reviewed for a plain JSON schema. Future custom
// codecs require an explicit equivalence review rather than relying on the
// canonical round trip alone.
func TestSourceCanonicalDecodePlainSchema(t *testing.T) {
	codecs := []reflect.Type{
		reflect.TypeFor[json.Marshaler](), reflect.TypeFor[json.Unmarshaler](),
		reflect.TypeFor[encoding.TextMarshaler](), reflect.TypeFor[encoding.TextUnmarshaler](),
	}
	seen := map[reflect.Type]bool{}
	var visit func(reflect.Type)
	visit = func(typ reflect.Type) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		for _, codec := range codecs {
			if typ.Implements(codec) || reflect.PointerTo(typ).Implements(codec) {
				t.Fatalf("source codec %v requires equivalence review", typ)
			}
		}
		switch typ.Kind() {
		case reflect.Pointer, reflect.Array, reflect.Slice:
			visit(typ.Elem())
		case reflect.Map:
			visit(typ.Key())
			visit(typ.Elem())
		case reflect.Struct:
			for i := range typ.NumField() {
				field := typ.Field(i)
				if field.IsExported() {
					visit(field.Type)
				}
			}
		case reflect.Interface:
			t.Fatalf("unbounded source schema interface %v requires equivalence review", typ)
		}
	}
	visit(reflect.TypeFor[sqliteSourcePayload]())
}

// Keep the original streaming decoder as an independent API oracle for this
// parser substitution. Compare acceptance and complete payloads, not diagnostics.
func strictSourceDecode(data []byte) (sqliteSourcePayload, bool) {
	var payload sqliteSourcePayload
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&payload) != nil {
		return payload, false
	}
	if decoder.Decode(new(any)) != io.EOF || payload.Version != sqlitePayloadVersion {
		return payload, false
	}
	canonical, err := json.Marshal(payload)
	if err != nil || !bytes.Equal(canonical, data) {
		return payload, false
	}
	metadata := payload.Inventory.Comparison.Metadata
	return payload, validateCommits(payload.Commits, metadata.BaseSHA, metadata.HeadSHA) && validateIncremental(payload.Incremental, metadata)
}

func FuzzSourceDecodeEquivalence(f *testing.F) {
	for _, snapshot := range []Snapshot{fixture(), commitFixture()} {
		_, data, err := encodeSource(snapshot)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
		f.Add(bytes.Replace(data, []byte(`{`), []byte(`{"unknown":0,`), 1))
	}
	for _, data := range [][]byte{nil, []byte(`null`), []byte(`{}`), []byte(`{"version":1,"version":1}`), []byte{0xff}} {
		f.Add(data)
	}
	check := func(t *testing.T, data []byte) {
		t.Helper()
		want, accepted := strictSourceDecode(data)
		got, err := decodeSource(digest(data), data)
		if (err == nil) != accepted {
			t.Fatalf("decoder acceptance differs for %q: %v", data, err)
		}
		if err == nil && !reflect.DeepEqual(sourcePayload(got), want) {
			t.Fatal("accepted source payload changed")
		}
		if err != nil && !errors.Is(err, ErrInvalidRecord) {
			t.Fatal("corrupt source error classification changed")
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			return
		}
		check(t, data)
		if len(data) > 1024 {
			return
		}
		snapshot := commitFixture()
		description := strings.ToValidUTF8(string(data), "\ufffd")
		snapshot.PullRequestDescription = &description
		snapshot.Inventory.Comparison.Metadata.Description = description
		path := bytes.ReplaceAll(data, []byte{0}, []byte{'x'})
		snapshot.Inventory.Files[0].NewPath = path
		patch := append([]byte{}, data...)
		reference := digest(patch)
		snapshot.Inventory.Patches = map[string][]byte{reference: patch}
		snapshot.Inventory.Units[0].PatchReference = reference
		diff := snapshot.Commits.Entries[0].Diff
		diff.Files[0].NewPath = path
		diff.Patches = map[string][]byte{reference: patch}
		diff.Units[0].PatchReference = reference
		_, canonical, err := encodeSource(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		check(t, canonical)
	})
}

func TestSourceDecodeOwnsPayload(t *testing.T) {
	_, data, err := encodeSource(commitFixture())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeSource(digest(data), data)
	if err != nil {
		t.Fatal(err)
	}
	for i := range data {
		data[i] = 0
	}
	if !reflect.DeepEqual(sourcePayload(decoded), sourcePayload(commitFixture())) {
		t.Fatal("decoded source aliases input buffer")
	}
}

func TestSourceDecodeDoesNotExposeInvalidValues(t *testing.T) {
	data := []byte(`{"version":"source-secret-must-not-appear"}`)
	_, err := decodeSource(digest(data), data)
	if !errors.Is(err, ErrInvalidRecord) || bytes.Contains([]byte(err.Error()), []byte("source-secret-must-not-appear")) {
		t.Fatal("source decoder exposed invalid input value")
	}
}
