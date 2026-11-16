package workspace

import (
	"fmt"
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

// ForwardID identifies a forward within a workspace.
type ForwardID int

// End is one end of a forward: a host and TCP port.
type End struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func (e End) String() string {
	return net.JoinHostPort(e.Host, strconv.Itoa(e.Port))
}

// Forward is one ssh port forward through one host.
type Forward struct {
	ID        ForwardID `json:"id"`
	Host      string    `json:"host"`
	Direction Direction `json:"direction"`
	// BindPort is the port that listens: on the local machine for Local, on the remote host for Remote.
	BindPort int `json:"bind_port"`
	// Target is resolved on the remote host for Local, on the local machine for Remote.
	Target End `json:"target"`
}

// LocalEnd is the forward's end on the local machine.
func (f Forward) LocalEnd() End {
	if f.Direction == Local {
		return End{Host: "localhost", Port: f.BindPort}
	}
	return f.Target
}

// RemoteEnd is the forward's end on the remote host.
func (f Forward) RemoteEnd() End {
	if f.Direction == Local {
		return f.Target
	}
	return End{Host: "localhost", Port: f.BindPort}
}

// LocalURL is the browser URL of the local end, with https for a
// target on 443 or 8443. It is false for a target on a known non-web port,
// such as PostgreSQL's 5432.
func (f Forward) LocalURL() (string, bool) {
	scheme, isWeb := webScheme(f.Target.Port)
	if !isWeb {
		return "", false
	}
	return scheme + "://" + f.LocalEnd().String(), true
}

// SSHArgs are the ssh arguments that request this forward, such as
// "-L 4966:localhost:4966".
func (f Forward) SSHArgs() []string {
	flag := "-L"
	if f.Direction == Remote {
		flag = "-R"
	}
	return []string{flag, fmt.Sprintf("%d:%s", f.BindPort, f.Target)}
}
