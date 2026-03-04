//go:build !darwin && !linux

package tmux

func foregroundProcessGroupIDForPID(int) (int, bool) {
	return 0, false
}
