//go:build unix

package main

import "syscall"

// raiseFdLimit lifts the soft RLIMIT_NOFILE toward the hard limit. macOS
// defaults the soft limit to 256, and kqueue-backed file watching costs one
// descriptor per watched directory, so the default is easy to exhaust.
func raiseFdLimit() {
	const want = 8192
	var lim syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		return
	}
	target := uint64(want)
	if lim.Max > 0 && target > lim.Max {
		target = lim.Max
	}
	if lim.Cur >= target {
		return
	}
	lim.Cur = target
	_ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim)
}
