// Package probe answers one question quickly: is this host reachable right now?
package probe

import (
	"context"
	"net"
	"sync"
	"time"
)

// State is a host's reachability.
type State int

const (
	// Unknown means we have not looked yet. It is the honest starting state,
	// and it must be visually distinct from Down — a manager that renders
	// "not checked yet" the same as "unreachable" teaches you to ignore it.
	Unknown State = iota
	Checking
	Up
	Down
)

func (s State) String() string {
	switch s {
	case Checking:
		return "checking"
	case Up:
		return "up"
	case Down:
		return "down"
	default:
		return "unknown"
	}
}

// Result is one probe's outcome.
type Result struct {
	Addr    string
	State   State
	Latency time.Duration
	Err     error
}

// DefaultTimeout bounds a single probe.
//
// Two seconds is long enough for a TCP handshake to a box on another continent
// and short enough that a wall of dead hosts does not make the list feel hung.
const DefaultTimeout = 2 * time.Second

// maxConcurrent bounds how many sockets we open at once.
//
// A user with two hundred hosts should not have hoppr open two hundred
// simultaneous connections the moment it starts: some corporate networks read
// that as a port scan, and some laptops read it as a reason to melt.
const maxConcurrent = 16

// Check reports whether a TCP connection to addr succeeds.
//
// A TCP handshake is deliberately all this does. Completing an SSH handshake
// would be a stronger signal, but it means presenting a key, which shows up in
// the server's auth log — and a manager that generates a failed-login entry for
// every host in your list every time you open it is a manager you will be asked
// to stop using.
func Check(ctx context.Context, addr string, timeout time.Duration) Result {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start)

	if err != nil {
		return Result{Addr: addr, State: Down, Latency: elapsed, Err: err}
	}
	conn.Close()
	return Result{Addr: addr, State: Up, Latency: elapsed}
}

// CheckAll probes every address concurrently and streams results as they land.
//
// Results are streamed rather than collected because the point is that the list
// fills in progressively. Waiting for the slowest host before showing any status
// at all would mean staring at a blank column for the length of the timeout.
//
// The returned channel is closed when every probe has finished.
func CheckAll(ctx context.Context, addrs []string, timeout time.Duration) <-chan Result {
	out := make(chan Result, len(addrs))

	go func() {
		defer close(out)

		sem := make(chan struct{}, maxConcurrent)
		var wg sync.WaitGroup

		for _, addr := range addrs {
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}

			wg.Add(1)
			go func(a string) {
				defer wg.Done()
				defer func() { <-sem }()

				res := Check(ctx, a, timeout)
				select {
				case out <- res:
				case <-ctx.Done():
				}
			}(addr)
		}
		wg.Wait()
	}()

	return out
}
