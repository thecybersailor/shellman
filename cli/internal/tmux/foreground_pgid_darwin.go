//go:build darwin

package tmux

import "golang.org/x/sys/unix"

func foregroundProcessGroupIDForPID(pid int) (int, bool) {
	if pid <= 0 {
		return 0, false
	}
	kproc, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, false
	}
	tpgid := int(kproc.Eproc.Tpgid)
	if tpgid <= 0 {
		return 0, false
	}
	return tpgid, true
}
