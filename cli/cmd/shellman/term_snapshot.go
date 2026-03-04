package main

import (
	"encoding/binary"
	"encoding/hex"
	"hash"
	"hash/crc32"
	"io"
	"sync"
)

const (
	snapshotHashHeadSampleBytes = 2 * 1024
	snapshotHashMidSampleBytes  = 2 * 1024
	snapshotHashTailSampleBytes = 16 * 1024
)

var snapshotHashTable = crc32.MakeTable(crc32.Castagnoli)

var snapshotHasherPool = sync.Pool{
	New: func() any { return crc32.New(snapshotHashTable) },
}

func snapshotChangeHash(text string) string {
	hasher := snapshotHasherPool.Get().(hash.Hash32)
	hasher.Reset()
	if len(text) > 0 {
		writeSnapshotHashSamples(hasher, text)
		var lenBuf [8]byte
		binary.LittleEndian.PutUint64(lenBuf[:], uint64(len(text)))
		_, _ = hasher.Write(lenBuf[:])
	}
	sum := hasher.Sum32()
	snapshotHasherPool.Put(hasher)

	var sumBuf [4]byte
	binary.BigEndian.PutUint32(sumBuf[:], sum)
	var hexBuf [8]byte
	hex.Encode(hexBuf[:], sumBuf[:])
	return string(hexBuf[:])
}

func writeSnapshotHashSamples(hasher hash.Hash32, text string) {
	if hasher == nil || len(text) == 0 {
		return
	}
	n := len(text)
	fullThreshold := snapshotHashHeadSampleBytes + snapshotHashMidSampleBytes + snapshotHashTailSampleBytes
	if n <= fullThreshold {
		_, _ = io.WriteString(hasher, text)
		return
	}
	_, _ = io.WriteString(hasher, text[:snapshotHashHeadSampleBytes])
	midStart := n/2 - snapshotHashMidSampleBytes/2
	if midStart < 0 {
		midStart = 0
	}
	midEnd := midStart + snapshotHashMidSampleBytes
	if midEnd > n {
		midEnd = n
	}
	_, _ = io.WriteString(hasher, text[midStart:midEnd])
	_, _ = io.WriteString(hasher, text[n-snapshotHashTailSampleBytes:])
}

func normalizeTermSnapshot(text string) string {
	return text
}
