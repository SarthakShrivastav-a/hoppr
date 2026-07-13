// Package forward runs and supervises SSH port forwards.
package forward

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind is which direction a forward runs.
type Kind string

const (
	// Local maps a port on this machine to one reachable from the remote host.
	// This is the one everyone means: reach a database that only listens on the
	// server's loopback.
	Local Kind = "local"

	// Remote maps a port on the remote host back to one here.
	Remote Kind = "remote"

	// Dynamic runs a SOCKS proxy through the host.
	//
	// It is not an afterthought — it is as useful as the other two, and tools
	// that ship only -L and -R get asked for it immediately.
	Dynamic Kind = "dynamic"
)

// Spec describes one forward.
type Spec struct {
	Kind Kind

	// BindAddr is optional. Empty means localhost, which is the safe default:
	// binding to 0.0.0.0 exposes the tunnel to your whole network, and nobody
	// should get that by accident.
	BindAddr string
	BindPort int

	// DestHost and DestPort are unused for Dynamic.
	DestHost string
	DestPort int
}

// ParseSpec reads the shorthand people already know from the ssh command line.
//
//	8080:localhost:80              local
//	127.0.0.1:8080:localhost:80    local, explicit bind address
//	1080                           dynamic (SOCKS)
func ParseSpec(kind Kind, s string) (Spec, error) {
	spec := Spec{Kind: kind}

	if kind == Dynamic {
		p, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return spec, fmt.Errorf("a dynamic forward takes just a port, got %q", s)
		}
		if err := validPort(p); err != nil {
			return spec, err
		}
		spec.BindPort = p
		return spec, nil
	}

	parts := strings.Split(s, ":")
	switch len(parts) {
	case 3:
		// bindPort:destHost:destPort
	case 4:
		// bindAddr:bindPort:destHost:destPort
		spec.BindAddr = parts[0]
		parts = parts[1:]
	default:
		return spec, fmt.Errorf("want bindPort:destHost:destPort, got %q", s)
	}

	bp, err := strconv.Atoi(parts[0])
	if err != nil {
		return spec, fmt.Errorf("bind port %q is not a number", parts[0])
	}
	if err := validPort(bp); err != nil {
		return spec, err
	}

	dp, err := strconv.Atoi(parts[2])
	if err != nil {
		return spec, fmt.Errorf("destination port %q is not a number", parts[2])
	}
	if err := validPort(dp); err != nil {
		return spec, err
	}

	spec.BindPort = bp
	spec.DestHost = parts[1]
	spec.DestPort = dp
	return spec, nil
}

func validPort(p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("port %d is out of range", p)
	}
	return nil
}

// Flag returns the ssh command-line flag for this forward.
func (s Spec) Flag() string {
	switch s.Kind {
	case Remote:
		return "-R"
	case Dynamic:
		return "-D"
	default:
		return "-L"
	}
}

// Arg returns the ssh command-line argument for this forward.
func (s Spec) Arg() string {
	if s.Kind == Dynamic {
		if s.BindAddr != "" {
			return s.BindAddr + ":" + strconv.Itoa(s.BindPort)
		}
		return strconv.Itoa(s.BindPort)
	}

	bind := strconv.Itoa(s.BindPort)
	if s.BindAddr != "" {
		bind = s.BindAddr + ":" + bind
	}
	return bind + ":" + s.DestHost + ":" + strconv.Itoa(s.DestPort)
}

// String renders the forward the way a human reads it.
func (s Spec) String() string {
	switch s.Kind {
	case Dynamic:
		return fmt.Sprintf("SOCKS proxy on localhost:%d", s.BindPort)
	case Remote:
		return fmt.Sprintf("remote :%d → %s:%d", s.BindPort, s.DestHost, s.DestPort)
	default:
		return fmt.Sprintf("localhost:%d → %s:%d", s.BindPort, s.DestHost, s.DestPort)
	}
}

// ConfigKeyword returns the ssh_config keyword and value for this forward, so a
// forward can be stored on the host rather than retyped every time.
func (s Spec) ConfigKeyword() (keyword string, args []string) {
	bind := strconv.Itoa(s.BindPort)
	if s.BindAddr != "" {
		bind = s.BindAddr + ":" + bind
	}
	dest := s.DestHost + ":" + strconv.Itoa(s.DestPort)

	switch s.Kind {
	case Dynamic:
		return "DynamicForward", []string{bind}
	case Remote:
		return "RemoteForward", []string{bind, dest}
	default:
		return "LocalForward", []string{bind, dest}
	}
}
