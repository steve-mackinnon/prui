package source

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/pjbgf/sha1cd"
)

func preflightFixture(t testing.TB, kind byte, declared, baseSize, resultSize uint64) ([]byte, []byte) {
	t.Helper()
	pack := []byte("PACK\x00\x00\x00\x02\x00\x00\x00\x01")
	header := byte(declared&15) | kind<<4
	declared >>= 4
	if declared > 0 {
		header |= 128
	}
	pack = append(pack, header)
	for declared > 0 {
		next := byte(declared & 127)
		declared >>= 7
		if declared > 0 {
			next |= 128
		}
		pack = append(pack, next)
	}
	if kind == 6 {
		pack = append(pack, 1)
	}
	if kind == 7 {
		pack = append(pack, make([]byte, 20)...)
	}
	var compressed bytes.Buffer
	z := zlib.NewWriter(&compressed)
	if kind >= 6 {
		var data [20]byte
		n := binary.PutUvarint(data[:], baseSize)
		n += binary.PutUvarint(data[n:], resultSize)
		if _, err := z.Write(data[:n]); err != nil {
			t.Fatal(err)
		}
	} else if _, err := z.Write([]byte("x\n")); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	pack = append(pack, compressed.Bytes()...)
	digest, _ := sha1cd.Sum(pack)
	pack = append(pack, digest[:]...)
	index := make([]byte, 8+1024+28+40)
	copy(index, []byte{255, 't', 'O', 'c', 0, 0, 0, 2})
	for i := 0; i < 256; i++ {
		binary.BigEndian.PutUint32(index[8+4*i:], 1)
	}
	// One sorted zero OID with a small offset; Git verification must establish
	// identity later. This fixture only exercises resource eligibility.
	binary.BigEndian.PutUint32(index[8+1024+20+4:], 12)
	copy(index[len(index)-40:], digest[:])
	idxDigest, _ := sha1cd.Sum(index[:len(index)-20])
	copy(index[len(index)-20:], idxDigest[:])
	return index, pack
}

func FuzzPackPreflight(f *testing.F) {
	index, pack := preflightFixture(f, 3, 2, 0, 0)
	f.Add(index, pack)
	f.Add([]byte{}, []byte{})
	f.Fuzz(func(t *testing.T, index, pack []byte) {
		if len(index) > 1<<20 || len(pack) > 1<<20 {
			return
		}
		index, pack = bytes.Clone(index), bytes.Clone(pack)
		// Repair outer checksums so mutations can reach resource/framing logic.
		// Explicit unit tests independently cover bad-checksum rejection.
		if len(pack) >= 32 && len(index) >= 40 {
			digest, _ := sha1cd.Sum(pack[:len(pack)-20])
			copy(pack[len(pack)-20:], digest[:])
			copy(index[len(index)-40:], digest[:])
			sum, _ := sha1cd.Sum(index[:len(index)-20])
			copy(index[len(index)-20:], sum[:])
		}
		_ = preflightIndex(index, Defaults())
		_ = preflightPack(context.Background(), index, pack, Defaults())
	})
}

func TestPackPreflightBoundsBeforeVerification(t *testing.T) {
	index, pack := preflightFixture(t, 3, 2, 0, 0)
	if plan := preflightPack(context.Background(), index, pack, Defaults()); plan == nil || len(plan.oids) != 1 {
		t.Fatal("small bounded pack rejected")
	}
	for _, tc := range []struct {
		name string
		kind byte
		size uint64
	}{
		{"large blob", 3, 2 << 20}, {"large tree", 2, 2 << 20}, {"large delta instructions", 6, 2 << 20}, {"reserved kind", 5, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx, data := preflightFixture(t, tc.kind, tc.size, 1, 2)
			if preflightPack(context.Background(), idx, data, Defaults()) != nil {
				t.Fatal("unbounded or unsupported record became eligible")
			}
		})
	}
	for name, mutate := range map[string]func([]byte, []byte){
		"bad index checksum": func(idx, data []byte) { idx[len(idx)-1] ^= 1 },
		"bad pack checksum":  func(idx, data []byte) { data[len(data)-1] ^= 1 },
		"bad signature":      func(idx, data []byte) { data[0] = 'X' },
		"bad count":          func(idx, data []byte) { data[11] = 2 },
		"bad offset":         func(idx, data []byte) { binary.BigEndian.PutUint32(idx[8+1024+24:], 0xffffffff) },
	} {
		t.Run(name, func(t *testing.T) {
			idx, data := bytes.Clone(index), bytes.Clone(pack)
			mutate(idx, data)
			if preflightPack(context.Background(), idx, data, Defaults()) != nil {
				t.Fatal("malformed pack became eligible")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if preflightPack(ctx, index, pack, Defaults()) != nil {
		t.Fatal("canceled preflight succeeded")
	}
	limits := Defaults()
	limits.BlobBytes = 1
	if preflightPack(context.Background(), index, pack, limits) != nil {
		t.Fatal("caller blob limit ignored")
	}
}

func TestPackIndexPreflightRejectsMalformedTables(t *testing.T) {
	index, _ := preflightFixture(t, 3, 2, 0, 0)
	for _, offset := range []int{0, 7, 8, 8 + 4*255, 8 + 1024 + 24} {
		t.Run(fmt.Sprint(offset), func(t *testing.T) {
			idx := bytes.Clone(index)
			idx[offset] ^= 0x80
			sum, _ := sha1cd.Sum(idx[:len(idx)-20])
			copy(idx[len(idx)-20:], sum[:])
			if preflightIndex(idx, Defaults()) != nil {
				t.Fatal("malformed index accepted", hex.EncodeToString(idx[:8]))
			}
		})
	}
}

func TestDeltaResourcePreflight(t *testing.T) {
	for name, payload := range map[string][]byte{
		"bounded": {1, 2}, "large base": {0x80, 0x80, 0x80, 1, 2}, "large result": {1, 0x80, 0x80, 0x80, 1},
		"overflow": bytes.Repeat([]byte{255}, 20), "truncated": {128},
	} {
		t.Run(name, func(t *testing.T) {
			var compressed bytes.Buffer
			z := zlib.NewWriter(&compressed)
			if _, err := z.Write(payload); err != nil {
				t.Fatal(err)
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			size, ok := (&packInflater{}).check(bytes.NewReader(compressed.Bytes()), true, uint64(len(payload)), 1<<20)
			if ok != (name == "bounded") || ok && size != 2 {
				t.Fatal("delta result guard", size, ok)
			}
		})
	}
	for _, count := range []int{maxDedupDepth + 1, maxDedupDepth + 2} {
		records := make([]packRecord, count)
		for i := range records {
			records[i].base = i - 1
		}
		if boundedDeltaDepth(records) != (count == maxDedupDepth+1) {
			t.Fatal("delta depth bound", count)
		}
		records[0].base = count - 1
		if boundedDeltaDepth(records) {
			t.Fatal("delta cycle accepted")
		}
	}
}

func TestPackPreflightRejectsUntrustedRecordBoundaries(t *testing.T) {
	index, pack := preflightFixture(t, 3, 2, 0, 0)
	pack = append(pack[:len(pack)-20], 0) // Indexed span now includes bytes after the actual stream.
	digest, _ := sha1cd.Sum(pack)
	pack = append(pack, digest[:]...)
	copy(index[len(index)-40:], digest[:])
	sum, _ := sha1cd.Sum(index[:len(index)-20])
	copy(index[len(index)-20:], sum[:])
	if preflightPack(context.Background(), index, pack, Defaults()) != nil {
		t.Fatal("unverified index boundaries bypassed bounded sequential framing")
	}
	index, pack = preflightFixture(t, 3, 1, 0, 0) // Header lies about the actual two-byte output.
	if preflightPack(context.Background(), index, pack, Defaults()) != nil {
		t.Fatal("uncompressed output exceeded its resource reservation")
	}
}

func TestPackPreflightCapsDiscardedVerificationWork(t *testing.T) {
	// Many small compressed records can represent much more historical work
	// than their retained input size. Test the whole parser at that boundary.
	var compressed bytes.Buffer
	z := zlib.NewWriter(&compressed)
	if _, err := z.Write(make([]byte, 1<<20)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{96, 97} {
		pack := []byte("PACK\x00\x00\x00\x02\x00\x00\x00\x00")
		binary.BigEndian.PutUint32(pack[8:], uint32(n))
		index := make([]byte, 8+1024+28*n+40)
		copy(index, []byte{255, 't', 'O', 'c', 0, 0, 0, 2})
		for i := range 256 {
			binary.BigEndian.PutUint32(index[8+4*i:], uint32(n))
		}
		for i := range n {
			binary.BigEndian.PutUint32(index[8+1024+20*i+16:], uint32(i+1))
			binary.BigEndian.PutUint32(index[8+1024+24*n+4*i:], uint32(len(pack)))
			pack = append(pack, 0xb0, 0x80, 0x80, 0x04) // Blob header declaring exactly 1 MiB.
			pack = append(pack, compressed.Bytes()...)
		}
		digest, _ := sha1cd.Sum(pack)
		pack = append(pack, digest[:]...)
		copy(index[len(index)-40:], digest[:])
		sum, _ := sha1cd.Sum(index[:len(index)-20])
		copy(index[len(index)-20:], sum[:])
		eligible := preflightPack(context.Background(), index, pack, Defaults()) != nil
		if eligible != (n == 96) {
			t.Fatal("discarded historical work cap", n, eligible)
		}
	}
}
