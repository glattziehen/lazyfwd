package tunnel

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows a bind cannot tell whether a port is free: a bind on 127.0.0.1
// succeeds next to another program's listener on the wildcard address, even
// with SO_EXCLUSIVEADDRUSE. lazyfwd reads the system's table of TCP listeners
// instead, which neither binds nor counts ports in TIME_WAIT.

var procGetExtendedTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

const (
	// tcpTableOwnerPIDListener is TCP_TABLE_OWNER_PID_LISTENER.
	tcpTableOwnerPIDListener = 3
	// tcpRowSize and tcp6RowSize are the sizes of MIB_TCPROW_OWNER_PID and
	// MIB_TCP6ROW_OWNER_PID; rows follow the table's uint32 entry count.
	tcpRowSize  = 24
	tcp6RowSize = 56
)

// IsLocalPortFree reports whether no program listens on the TCP port on the
// local loopback or the wildcard address, on IPv4 or IPv6: ssh binds
// "localhost" on both, and a forward next to a wildcard listener would
// shadow it on loopback. It reports false when it cannot read the table.
func IsLocalPortFree(port int) bool {
	listeners, err := tcpListeners()
	if err != nil {
		return false
	}
	for _, listener := range listeners {
		address := listener.Addr().Unmap()
		isLocalhost := address == netip.IPv4Unspecified() || address == netip.AddrFrom4([4]byte{127, 0, 0, 1}) ||
			address == netip.IPv6Unspecified() || address == netip.IPv6Loopback()
		if int(listener.Port()) == port && isLocalhost {
			return false
		}
	}
	return true
}

// tcpListeners lists the local addresses of every listening TCP socket.
func tcpListeners() ([]netip.AddrPort, error) {
	table4, err := tcpTable(windows.AF_INET)
	if err != nil {
		return nil, err
	}
	table6, err := tcpTable(windows.AF_INET6)
	if err != nil {
		return nil, err
	}
	var listeners []netip.AddrPort
	for row := range rows(table4, tcpRowSize) {
		// dwState, dwLocalAddr, dwLocalPort, ...; address and port are in
		// network byte order.
		address := netip.AddrFrom4([4]byte(row[4:8]))
		listeners = append(listeners, netip.AddrPortFrom(address, binary.BigEndian.Uint16(row[8:10])))
	}
	for row := range rows(table6, tcp6RowSize) {
		// ucLocalAddr[16], dwLocalScopeId, dwLocalPort, ...
		address := netip.AddrFrom16([16]byte(row[0:16]))
		listeners = append(listeners, netip.AddrPortFrom(address, binary.BigEndian.Uint16(row[20:22])))
	}
	return listeners, nil
}

// tcpTable returns the listener table of family as GetExtendedTcpTable
// writes it, growing the buffer while the table grows.
func tcpTable(family int) ([]byte, error) {
	size := uint32(4)
	for {
		table := make([]byte, size)
		result, _, _ := procGetExtendedTCPTable.Call(
			uintptr(unsafe.Pointer(&table[0])), uintptr(unsafe.Pointer(&size)),
			0, uintptr(family), tcpTableOwnerPIDListener, 0)
		switch err := syscall.Errno(result); {
		case result == 0:
			return table, nil
		case errors.Is(err, windows.ERROR_INSUFFICIENT_BUFFER):
			continue
		default:
			return nil, err
		}
	}
}

// rows yields each row of a table that starts with a uint32 entry count.
func rows(table []byte, rowSize int) func(yield func([]byte) bool) {
	return func(yield func([]byte) bool) {
		count := int(binary.LittleEndian.Uint32(table[:4]))
		for i := range count {
			start := 4 + i*rowSize
			if start+rowSize > len(table) || !yield(table[start:start+rowSize]) {
				return
			}
		}
	}
}
