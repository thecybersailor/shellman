package main

import (
	"crypto/sha1"
	"encoding/hex"
)

func sha1Text(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])
}

func normalizeTermSnapshot(text string) string {
	return text
}
