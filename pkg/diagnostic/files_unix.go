//go:build !windows

package diagnostic

import "syscall"

const nonBlock = syscall.O_NONBLOCK
