//go:build unix

package main

import "syscall"

// detachAttr puts the worker in its own session so closing the terminal does not kill it.
func detachAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }
