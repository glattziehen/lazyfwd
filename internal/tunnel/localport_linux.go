package tunnel

import "syscall"

// exclusiveBind is nil on Linux: there SO_REUSEADDR, which Go sets, never
// lets a bind share a port with a listening socket, wildcard included.
var exclusiveBind func(network, address string, conn syscall.RawConn) error
