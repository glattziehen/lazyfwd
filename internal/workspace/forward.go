package workspace

import (
	"fmt"
	"net"
	"strconv"
)

// Direction says which side of the ssh connection listens.
type Direction string

const (
	// Local listens on the laptop and connects to Target from the remote host (ssh -L).
	Local Direction = "local"
	// Remote listens on the remote host and connects to Target from the laptop (ssh -R).
	Remote Direction = "remote"
)

// ForwardID identifies a forward within a workspace.
type ForwardID int

// Addr is a host and TCP port.
type Addr struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func (a Addr) String() string {
	return net.JoinHostPort(a.Host, strconv.Itoa(a.Port))
}

// Forward is one ssh port forward through one host.
type Forward struct {
	ID        ForwardID `json:"id"`
	Host      string    `json:"host"`
	Direction Direction `json:"direction"`
	// BindPort is the port that listens: on the laptop for Local, on the remote host for Remote.
	BindPort int `json:"bind_port"`
	// Target is resolved on the remote host for Local, on the laptop for Remote.
	Target Addr `json:"target"`
}

// LaptopAddr is the forward's endpoint on the laptop.
func (f Forward) LaptopAddr() Addr {
	if f.Direction == Local {
		return Addr{Host: "localhost", Port: f.BindPort}
	}
	return f.Target
}

// RemoteAddr is the forward's endpoint on the remote host.
func (f Forward) RemoteAddr() Addr {
	if f.Direction == Local {
		return f.Target
	}
	return Addr{Host: "localhost", Port: f.BindPort}
}

// LaptopURL is the browser URL of the laptop endpoint, with https for a
// target on 443 or 8443. It is false for a target on a known non-web port,
// such as PostgreSQL's 5432.
func (f Forward) LaptopURL() (string, bool) {
	scheme, isWeb := webScheme(f.Target.Port)
	if !isWeb {
		return "", false
	}
	return scheme + "://" + f.LaptopAddr().String(), true
}

// SSHArgs are the ssh arguments that open this forward and nothing else.
func (f Forward) SSHArgs() []string {
	flag := "-L"
	if f.Direction == Remote {
		flag = "-R"
	}
	spec := fmt.Sprintf("%d:%s", f.BindPort, f.Target)
	return []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		flag, spec,
		f.Host,
	}
}
