package main

import (
	"testing"
)

func TestAcquireReleasePooledBytes_Buckets(t *testing.T) {
	tests := []struct {
		size    int
		wantCap int
	}{
		{size: 1, wantCap: pooledBytes4K},
		{size: 4096, wantCap: pooledBytes4K},
		{size: 4097, wantCap: pooledBytes16K},
		{size: 16384, wantCap: pooledBytes16K},
		{size: 16385, wantCap: pooledBytes64K},
		{size: 65536, wantCap: pooledBytes64K},
		{size: 65537, wantCap: pooledBytes256K},
		{size: 262144, wantCap: pooledBytes256K},
	}
	for _, tc := range tests {
		buf := acquirePooledBytes(tc.size)
		if len(buf) != tc.size {
			t.Fatalf("size=%d: len mismatch got=%d", tc.size, len(buf))
		}
		if cap(buf) != tc.wantCap {
			t.Fatalf("size=%d: cap mismatch got=%d want=%d", tc.size, cap(buf), tc.wantCap)
		}
		releasePooledBytes(buf)
	}
}

func TestAcquirePooledBytes_LargeBypassesPool(t *testing.T) {
	buf := acquirePooledBytes(pooledBytes256K + 1)
	if len(buf) != pooledBytes256K+1 {
		t.Fatalf("len mismatch got=%d", len(buf))
	}
	if cap(buf) != pooledBytes256K+1 {
		t.Fatalf("cap mismatch got=%d want=%d", cap(buf), pooledBytes256K+1)
	}
	releasePooledBytes(buf)
}

func TestAcquireReleasePooledBuffer(t *testing.T) {
	buf := acquirePooledBuffer(1024)
	if buf.Len() != 0 {
		t.Fatalf("expected empty buffer, got len=%d", buf.Len())
	}
	_, _ = buf.WriteString("hello")
	if buf.Len() != 5 {
		t.Fatalf("expected len=5, got %d", buf.Len())
	}
	releasePooledBuffer(buf)

	buf2 := acquirePooledBuffer(0)
	if buf2.Len() != 0 {
		t.Fatalf("expected reset buffer, got len=%d", buf2.Len())
	}
	releasePooledBuffer(buf2)
}
