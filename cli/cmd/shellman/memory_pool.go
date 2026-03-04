package main

import (
	"bytes"
	"sync"
)

const (
	pooledBytes4K   = 4 * 1024
	pooledBytes16K  = 16 * 1024
	pooledBytes64K  = 64 * 1024
	pooledBytes256K = 256 * 1024

	pooledBufferMaxCap = 256 * 1024
)

var (
	bytePool4K   = sync.Pool{New: func() any { return make([]byte, pooledBytes4K) }}
	bytePool16K  = sync.Pool{New: func() any { return make([]byte, pooledBytes16K) }}
	bytePool64K  = sync.Pool{New: func() any { return make([]byte, pooledBytes64K) }}
	bytePool256K = sync.Pool{New: func() any { return make([]byte, pooledBytes256K) }}

	bufferPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}
)

func acquirePooledBytes(size int) []byte {
	if size <= 0 {
		return nil
	}
	switch {
	case size <= pooledBytes4K:
		buf := bytePool4K.Get().([]byte)
		return buf[:size]
	case size <= pooledBytes16K:
		buf := bytePool16K.Get().([]byte)
		return buf[:size]
	case size <= pooledBytes64K:
		buf := bytePool64K.Get().([]byte)
		return buf[:size]
	case size <= pooledBytes256K:
		buf := bytePool256K.Get().([]byte)
		return buf[:size]
	default:
		return make([]byte, size)
	}
}

func releasePooledBytes(buf []byte) {
	if buf == nil {
		return
	}
	switch cap(buf) {
	case pooledBytes4K:
		bytePool4K.Put(buf[:pooledBytes4K])
	case pooledBytes16K:
		bytePool16K.Put(buf[:pooledBytes16K])
	case pooledBytes64K:
		bytePool64K.Put(buf[:pooledBytes64K])
	case pooledBytes256K:
		bytePool256K.Put(buf[:pooledBytes256K])
	}
}

func acquirePooledBuffer(growHint int) *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	if growHint > 0 && growHint <= pooledBufferMaxCap {
		buf.Grow(growHint)
	}
	return buf
}

func releasePooledBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	if buf.Cap() > pooledBufferMaxCap {
		return
	}
	buf.Reset()
	bufferPool.Put(buf)
}
