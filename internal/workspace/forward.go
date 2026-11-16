package workspace

import (
	"net"
	"strconv"
)

// Direction says which side of the ssh connection listens.
type Direction string

const (
	// Local listens on the local machine and connects to Target from the remote host (ssh -L).
	Local Direction = "local"
	// Remote listens on the remote host and connects to Target from the local machine (ssh -R).
	Remote Direction = "remote"
)

// BindHost is where a TCP bind listens; ssh binds it when a forward names
// only a port.
const BindHost = "localhost"

// ForwardID identifies a forward within a workspace.
type ForwardID int

// End is one end of a forward: a host and TCP port, or a unix socket path.
// A socket end has only a Path.
type End struct {
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
	Path string `json:"path,omitempty"`
}

// IsSocket reports whether the end is a unix socket.
func (e End) IsSocket() bool {
	return e.Path != ""
}

// String is the end as ssh writes it: host:port, or the socket path.
func (e End) String() string {
	if e.IsSocket() {
		return e.Path
	}
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}

// Forward is one ssh forward through one host.
type Forward struct {
	ID        ForwardID
	Host      string
	Direction Direction
	// Bind is where the forward listens: on the local machine for Local, on
	// the remote host for Remote. A TCP bind listens on localhost.
	Bind End
	// Target is where the forward connects: from the remote host for Local,
	// from the local machine for Remote.
	Target End
}

// LocalEnd is the forward's end on the local machine.
func (f Forward) LocalEnd() End {
	if f.Direction == Local {
		return f.Bind
	}
	return f.Target
}

// RemoteEnd is the forward's end on the remote host.
func (f Forward) RemoteEnd() End {
	if f.Direction == Local {
		return f.Target
	}
	return f.Bind
}

// LocalURL is the browser URL of a TCP local end, with https for a target
// on 443 or 8443. It is false for a socket local end and for a target on a
// known non-web port, such as PostgreSQL's 5432.
func (f Forward) LocalURL() (string, bool) {
	local := f.LocalEnd()
	if local.IsSocket() {
		return "", false
	}
	scheme, isWeb := webScheme(f.Target.Port)
	if !isWeb {
		return "", false
	}
	return scheme + "://" + local.String(), true
}

// SSHArgs are the ssh arguments that request this forward, such as
// "-L 4966:localhost:4966" or "-L 2375:/var/run/docker.sock". A TCP bind
// names only its port, so ssh listens on localhost.
func (f Forward) SSHArgs() []string {
	flag := "-L"
	if f.Direction == Remote {
		flag = "-R"
	}
	bind := f.Bind.Path
	if !f.Bind.IsSocket() {
		bind = strconv.Itoa(f.Bind.Port)
	}
	return []string{flag, bind + ":" + f.Target.String()}
}
