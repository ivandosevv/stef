//go:build !windows

package main

import "syscall"

func residentProcessAttrs() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
