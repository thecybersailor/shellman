package main

import (
	"crypto/sha1"
	"encoding/hex"
	"hash"
	"sync"
)

var sha1HasherPool = sync.Pool{
	New: func() any { return sha1.New() },
}

func sha1Text(text string) string {
	hasher := sha1HasherPool.Get().(hash.Hash)
	hasher.Reset()
	if text != "" {
		input := acquirePooledBytes(len(text))
		copy(input, text)
		_, _ = hasher.Write(input)
		releasePooledBytes(input)
	}
	sumBuf := acquirePooledBytes(sha1.Size)
	sum := hasher.Sum(sumBuf[:0])
	sha1HasherPool.Put(hasher)

	hexBuf := acquirePooledBytes(sha1.Size * 2)
	hex.Encode(hexBuf[:sha1.Size*2], sum)
	out := string(hexBuf[:sha1.Size*2])
	releasePooledBytes(sumBuf)
	releasePooledBytes(hexBuf)
	return out
}

func normalizeTermSnapshot(text string) string {
	return text
}
