package source

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"encoding/hex"
	"io"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/pjbgf/sha1cd"
)

const maxDedupPackBytes = 16 << 20
const maxDedupObjects = 10000
const maxDedupDepth = 64

// Cumulative verifier work is discarded, not returned source. Keep it bounded
// independently of the unchanged materialized-source quotas. Private retained
// input bytes still fit ContentBytes and the 16 MiB compressed-copy ceiling.
const maxDedupWorkBytes = 96 << 20

type packRecord struct {
	oid    string
	offset int
	base   int
}

// Eligibility is only a resource preflight. No object may be omitted until
// Git independently verifies the private pack and its complete index.
type packPreflight struct {
	oids    []string
	records []packRecord
	digest  []byte
}

// Restrict the optimization to small SHA-1 v2 indices without large offsets.
// Other storage shapes retain ordinary mirroring, without a verification read.
func preflightIndex(data []byte, limits Limits) *packPreflight {
	const prefix = 8 + 1024
	if len(data) < prefix+40 || len(data) > min(limits.ContentBytes, 1<<20) || !bytes.Equal(data[:8], []byte{255, 't', 'O', 'c', 0, 0, 0, 2}) {
		return nil
	}
	n := int(binary.BigEndian.Uint32(data[prefix-4:]))
	if n < 1 || n > min(limits.Entries, maxDedupObjects) || len(data) != prefix+28*n+40 {
		return nil
	}
	digest, collision := sha1cd.Sum(data[:len(data)-20])
	if collision || !bytes.Equal(digest[:], data[len(data)-20:]) {
		return nil
	}
	plan := &packPreflight{digest: bytes.Clone(data[len(data)-40 : len(data)-20])}
	var counts [256]uint32
	var previous []byte
	for i := range n {
		oid := data[prefix+20*i : prefix+20*(i+1)]
		if previous != nil && bytes.Compare(previous, oid) >= 0 {
			return nil
		}
		previous = oid
		counts[oid[0]]++
		offset := binary.BigEndian.Uint32(data[prefix+24*n+4*i:])
		if offset < 12 || offset >= 1<<31 {
			return nil
		}
		name := hex.EncodeToString(oid)
		plan.oids = append(plan.oids, name)
		plan.records = append(plan.records, packRecord{oid: name, offset: int(offset), base: -1})
	}
	var cumulative uint32
	for i, count := range counts {
		cumulative += count
		if binary.BigEndian.Uint32(data[8+4*i:]) != cumulative {
			return nil
		}
	}
	return plan
}

func preflightPack(ctx context.Context, index, data []byte, limits Limits) *packPreflight {
	return preflightPackWorkers(ctx, index, data, limits, min(4, runtime.GOMAXPROCS(0)))
}

func preflightPackWorkers(ctx context.Context, index, data []byte, limits Limits, workers int) *packPreflight {
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
	checks := make([]packInflation, len(plan.records))
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
		checks[i] = packInflation{start: end - body.Len(), end: end, delta: kind == 6 || kind == 7, size: size}
	}
	if !boundedDeltaDepth(plan.records) || !checkPackInflations(ctx, data, checks, work, workLimit, objectLimit, workers) {
		return nil
	}
	return plan
}

type packInflation struct {
	start, end int
	size       uint64
	delta      bool
}

// All declared inflation bytes were reserved before starting any worker. Delta
// result sizes share the same aggregate eligibility budget, never a per-worker
// allowance. Every worker joins before verification or ordinary mirroring.
func checkPackInflations(parent context.Context, data []byte, checks []packInflation, declared, workLimit, objectLimit uint64, workers int) bool {
	if declared > workLimit {
		return false
	}
	workers = max(1, min(4, workers, len(checks)))
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var spent atomic.Uint64
	spent.Store(declared)
	reserve := func(n uint64) bool {
		for {
			previous := spent.Load()
			if n > workLimit-previous {
				return false
			}
			if spent.CompareAndSwap(previous, previous+n) {
				return true
			}
		}
	}
	run := func(worker int) {
		var inflater packInflater
		for i := worker; i < len(checks); i += workers {
			if ctx.Err() != nil {
				return
			}
			item := checks[i]
			result, ok := inflater.check(bytes.NewReader(data[item.start:item.end]), item.delta, item.size, objectLimit)
			if !ok || result > 0 && !reserve(result) {
				cancel()
				return
			}
		}
	}
	if workers == 1 {
		run(0)
	} else {
		var group sync.WaitGroup
		for i := range workers {
			group.Go(func() { run(i) })
		}
		group.Wait()
	}
	return ctx.Err() == nil
}

func boundedDeltaDepth(records []packRecord) bool {
	for i := range records {
		depth := 0
		for base := records[i].base; base >= 0; base = records[base].base {
			depth++
			if depth > maxDedupDepth {
				return false // Also rejects cycles before starting the Git verifier.
			}
		}
	}
	return true
}

// Inflate at most the declared bounded size plus one byte. Checking the exact
// stream boundary is essential: an unverified index could otherwise point at
// fake headers inside another record and hide an oversized sequential record.
// This checks resource eligibility and framing, not object identity.
type packInflater struct {
	stream  io.ReadCloser
	limited io.LimitedReader
	buffer  *bufio.Reader
}

func (p *packInflater) check(body *bytes.Reader, delta bool, size, limit uint64) (uint64, bool) {
	if size > 1<<20 || size > limit {
		return 0, false
	}
	if p.stream == nil {
		stream, err := zlib.NewReader(body)
		if err != nil {
			return 0, false
		}
		p.stream = stream
	} else if err := p.stream.(zlib.Resetter).Reset(body, nil); err != nil {
		_ = p.stream.Close()
		return 0, false
	}
	p.limited = io.LimitedReader{R: p.stream, N: int64(size) + 1}
	if p.buffer == nil {
		p.buffer = bufio.NewReaderSize(&p.limited, 16)
	} else {
		p.buffer.Reset(&p.limited)
	}
	resultSize := uint64(0)
	if delta {
		baseSize, baseErr := binary.ReadUvarint(p.buffer)
		var resultErr error
		resultSize, resultErr = binary.ReadUvarint(p.buffer)
		if baseErr != nil || resultErr != nil || baseSize > limit || resultSize > limit {
			_ = p.stream.Close()
			return 0, false
		}
	}
	_, readErr := io.Copy(io.Discard, p.buffer)
	closeErr := p.stream.Close()
	return resultSize, readErr == nil && closeErr == nil && p.limited.N == 1 && body.Len() == 0
}

func packObjectSize(r *bytes.Reader, limit uint64) (byte, uint64, bool) {
	c, err := r.ReadByte()
	if err != nil {
		return 0, 0, false
	}
	kind, size := (c>>4)&7, uint64(c&15)
	for shift := uint(4); c&128 != 0; shift += 7 {
		c, err = r.ReadByte()
		if err != nil || shift >= 64 || uint64(c&127) > limit>>shift {
			return 0, 0, false
		}
		size |= uint64(c&127) << shift
	}
	return kind, size, size <= limit
}

func packBaseDistance(r *bytes.Reader, offset int) (int, bool) {
	if offset <= 0 {
		return 0, false
	}
	unsignedOffset := uint64(offset)
	c, err := r.ReadByte()
	if err != nil {
		return 0, false
	}
	distance := uint64(c & 127)
	for c&128 != 0 {
		c, err = r.ReadByte()
		if err != nil || distance+1 > unsignedOffset>>7 {
			return 0, false
		}
		distance = (distance+1)<<7 | uint64(c&127)
	}
	if distance == 0 || distance >= unsignedOffset {
		return 0, false
	}
	return int(distance), true
}
