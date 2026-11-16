package tunnel

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// soExclusiveAddrUse is SO_EXCLUSIVEADDRUSE, which x/sys/windows lacks.
const soExclusiveAddrUse = ^syscall.SO_REUSEADDR

// errAddrInUse is what Winsock returns for a taken port; syscall.EADDRINUSE
// is a different number on Windows.
const errAddrInUse = windows.WSAEADDRINUSE

// exclusiveBind sets SO_EXCLUSIVEADDRUSE. Without it Windows lets
// 127.0.0.1:port bind while another socket listens on *:port, so a taken port
// would look free and ssh would shadow that server.
func exclusiveBind(network, address string, conn syscall.RawConn) error {
	var err error
	if controlErr := conn.Control(func(fd uintptr) {
		err = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, soExclusiveAddrUse, 1)
	}); controlErr != nil {
		return controlErr
	}
	return err
}
