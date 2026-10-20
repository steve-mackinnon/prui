package source

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/hex"
	"reflect"
	"sort"
	"testing"

	"github.com/pjbgf/sha1cd"
)

// Resource eligibility only: the deliberately synthetic OIDs still need Git's
// independent identity verification before any loose object could be omitted.
func repeatedPreflightFixture(t testing.TB, n int) ([]byte, []byte) {
	t.Helper()
	_, one := preflightFixture(t, 3, 2, 0, 0)
	record := one[12 : len(one)-20]
	pack := bytes.Clone(one[:12])
	binary.BigEndian.PutUint32(pack[8:], uint32(n))
	for range n {
		pack = append(pack, record...)
	}
	sum, _ := sha1cd.Sum(pack)
	pack = append(pack, sum[:]...)
	index := make([]byte, 8+1024+28*n+40)
	copy(index, []byte{255, 't', 'O', 'c', 0, 0, 0, 2})
	for i := range 256 {
		binary.BigEndian.PutUint32(index[8+4*i:], uint32(n))
	}
	for i := range n {
		binary.BigEndian.PutUint32(index[8+1024+20*i+16:], uint32(i+1))
		binary.BigEndian.PutUint32(index[8+1024+24*n+4*i:], uint32(12+i*len(record)))
	}
	copy(index[len(index)-40:], sum[:])
	sum, _ = sha1cd.Sum(index[:len(index)-20])
	copy(index[len(index)-20:], sum[:])
	return index, pack
}

func TestParallelPackPreflightMatchesSerial(t *testing.T) {
	for _, n := range []int{1, 31, 32, 64, 128} {
		index, pack := repeatedPreflightFixture(t, n)
		serial := preflightPackWorkers(context.Background(), index, pack, Defaults(), 1)
		if serial == nil {
			t.Fatal("bounded fixture rejected")
		}
		for _, workers := range []int{0, 2, 4, 16} {
			parallel := preflightPackWorkers(context.Background(), index, pack, Defaults(), workers)
			if !reflect.DeepEqual(serial, parallel) {
				t.Fatal("parallel resource eligibility differs", n, workers)
			}
		}
	}
}

func TestParallelInflationSharesExactWorkBudget(t *testing.T) {
	var payload [20]byte
	n := binary.PutUvarint(payload[:], 1)
	n += binary.PutUvarint(payload[n:], 10)
	var compressed bytes.Buffer
	z := zlib.NewWriter(&compressed)
	if _, err := z.Write(payload[:n]); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	checks := make([]packInflation, 64)
	for i := range checks {
		checks[i] = packInflation{start: 0, end: compressed.Len(), delta: true, size: uint64(n)}
	}
	declared := uint64(n * len(checks))
	exact := declared + uint64(10*len(checks))
	for _, workers := range []int{1, 2, 4, 16} {
		for _, limit := range []uint64{exact - 1, exact, exact + 1} {
			if got := checkPackInflations(context.Background(), compressed.Bytes(), checks, declared, limit, 32, workers); got != (limit >= exact) {
				t.Fatal("worker budgets multiplied or exact boundary rejected", workers, limit)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if checkPackInflations(ctx, compressed.Bytes(), checks, declared, exact, 32, 4) {
		t.Fatal("canceled parallel inflation succeeded")
	}
}

func TestParallelInflationRejectsCorruptFrames(t *testing.T) {
	index, pack := repeatedPreflightFixture(t, 64)
	// Repair outer checksums after corrupting the final stream's checksum.
	pack[len(pack)-21] ^= 1
	sum, _ := sha1cd.Sum(pack[:len(pack)-20])
	copy(pack[len(pack)-20:], sum[:])
	copy(index[len(index)-40:], sum[:])
	sum, _ = sha1cd.Sum(index[:len(index)-20])
	copy(index[len(index)-20:], sum[:])
	for _, workers := range []int{1, 2, 4} {
		if preflightPackWorkers(context.Background(), index, pack, Defaults(), workers) != nil {
			t.Fatal("corrupt stream accepted", workers)
		}
	}
}

// Tiny records stress worker/setup overhead rather than large-output inflation.
// This measures resource preflight only, not Git's independent verification.
func BenchmarkPackPreflight(b *testing.B) {
	index, pack := repeatedPreflightFixture(b, 4096)
	for name, check := range map[string]func(context.Context, []byte, []byte, Limits) *packPreflight{
		"original": originalPreflightPack,
		"parallel": preflightPack,
	} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				if check(context.Background(), index, pack, Defaults()) == nil {
					b.Fatal("bounded fixture rejected")
				}
			}
		})
	}
}

// Original resource-eligibility parser retained as a differential oracle.
func originalPreflightPack(ctx context.Context, index, data []byte, limits Limits) *packPreflight {
	if ctx.Err() != nil || len(data) < 32 || len(data) > min(limits.ContentBytes, maxDedupPackBytes)-len(index) || !bytes.Equal(data[:4], []byte("PACK")) {
		return nil
	}
	plan := preflightIndex(index, limits)
	count := binary.BigEndian.Uint32(data[8:])
	if count > maxDedupObjects || plan == nil || int(count) != len(plan.records) {
		return nil
	}
	version := binary.BigEndian.Uint32(data[4:])
	if version != 2 && version != 3 {
		return nil
	}
	digest, collision := sha1cd.Sum(data[:len(data)-20])
	if collision || !bytes.Equal(digest[:], data[len(data)-20:]) || !bytes.Equal(digest[:], plan.digest) {
		return nil
	}
	sort.Slice(plan.records, func(i, j int) bool { return plan.records[i].offset < plan.records[j].offset })
	byOID, byOffset := map[string]int{}, map[int]int{}
	for i, record := range plan.records {
		if record.offset >= len(data)-20 || i == 0 && record.offset != 12 {
			return nil
		}
		if _, duplicate := byOffset[record.offset]; duplicate {
			return nil
		}
		byOID[record.oid], byOffset[record.offset] = i, i
	}
	work, workLimit := uint64(0), uint64(maxDedupWorkBytes)
	objectLimit := uint64(max(0, min(limits.BlobBytes, 1<<20)))
	var inflater packInflater
	for i := range plan.records {
		if ctx.Err() != nil {
			return nil
		}
		record := &plan.records[i]
		end := len(data) - 20
		if i+1 < len(plan.records) {
			end = plan.records[i+1].offset
		}
		body := bytes.NewReader(data[record.offset:end])
		kind, size, ok := packObjectSize(body, objectLimit)
		if !ok {
			return nil
		}
		work += size
		if work > workLimit {
			return nil
		}
		switch kind {
		case 1, 2, 3, 4:
		case 6:
			distance, ok := packBaseDistance(body, record.offset)
			base, found := byOffset[record.offset-distance]
			if !ok || !found {
				return nil
			}
			record.base = base
		case 7:
			var oid [20]byte
			if n, _ := body.Read(oid[:]); n != len(oid) {
				return nil
			}
			base, found := byOID[hex.EncodeToString(oid[:])]
			if !found {
				return nil // Never resolve a thin pack through another object store.
			}
			record.base = base
		default:
			return nil
		}
		resultSize, ok := inflater.check(body, kind == 6 || kind == 7, size, objectLimit)
		if !ok {
			return nil
		}
		work += resultSize
		if work > workLimit {
			return nil
		}
	}
	if !boundedDeltaDepth(plan.records) {
		return nil
	}
	return plan
}
