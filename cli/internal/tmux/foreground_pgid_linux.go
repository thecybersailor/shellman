//go:build linux

package tmux

import (
	"os"
	"strconv"
	"strings"
)

func foregroundProcessGroupIDForPID(pid int) (int, bool) {
	if pid <= 0 {
		return 0, false
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return 0, false
	}
	// /proc/<pid>/stat: pid (comm) state ppid pgrp session tty_nr tpgid ...
	right := strings.LastIndex(text, ")")
	if right < 0 || right+1 >= len(text) {
		return 0, false
	}
	fields := strings.Fields(strings.TrimSpace(text[right+1:]))
	if len(fields) < 6 {
		return 0, false
	}
	tpgid, err := strconv.Atoi(fields[5])
	if err != nil || tpgid <= 0 {
		return 0, false
	}
	return tpgid, true
}
