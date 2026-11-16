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

// LaptopURL is the browser URL of the laptop endpoint.
func (f Forward) LaptopURL() string {
	return "http://" + f.LaptopAddr().String()
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
