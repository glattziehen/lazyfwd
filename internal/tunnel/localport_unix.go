//go:build unix

package tunnel

import "syscall"

const errAddrInUse = syscall.EADDRINUSE
