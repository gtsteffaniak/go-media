//go:build windows

package main

import (
	"syscall"
)

func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	const access = syscall.STANDARD_RIGHTS_READ | syscall.PROCESS_QUERY_INFORMATION
	h, err := syscall.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return false
	}
	_ = syscall.CloseHandle(h)
	return true
}
