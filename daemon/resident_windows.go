//go:build windows

package main

import "syscall"

func residentProcessAttrs() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x00000200 | 0x00000008}
}
