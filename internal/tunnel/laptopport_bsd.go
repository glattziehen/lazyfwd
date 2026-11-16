//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package tunnel

import "syscall"

// exclusiveBind turns off the SO_REUSEADDR that Go sets on listeners. With it,
// BSD-family kernels let 127.0.0.1:port bind while another process listens on
// *:port, so a taken port would look free and ssh would shadow that server.
// The cost: a port with connections in TIME_WAIT also looks taken, and
// lazyfwd picks a neighbour.
func exclusiveBind(network, address string, conn syscall.RawConn) error {
	var err error
	if controlErr := conn.Control(func(fd uintptr) {
		err = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 0)
	}); controlErr != nil {
		return controlErr
	}
	return err
}
