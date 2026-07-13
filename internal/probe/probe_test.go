package probe

import (
	"context"
	"net"
	"testing"
	"time"
)

// listener opens a real TCP listener and returns its address.
func listener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	// Accept and immediately close, so a probe's dial actually completes.
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return ln.Addr().String()
}

// deadAddr returns an address nothing is listening on.
func deadAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close() // now nothing is there
	return addr
}

func TestCheckReportsUpForAListeningPort(t *testing.T) {
	res := Check(context.Background(), listener(t), time.Second)

	if res.State != Up {
		t.Fatalf("State = %v (err=%v), want up", res.State, res.Err)
	}
	if res.Err != nil {
		t.Errorf("Err = %v, want nil", res.Err)
	}
	if res.Latency <= 0 {
		t.Error("a successful probe should record a latency")
	}
}

func TestCheckReportsDownForANonListeningPort(t *testing.T) {
	res := Check(context.Background(), deadAddr(t), time.Second)

	if res.State != Down {
		t.Fatalf("State = %v, want down", res.State)
	}
	if res.Err == nil {
		t.Error("a failed probe should carry the reason")
	}
}

func TestCheckHonoursTheTimeout(t *testing.T) {
	// A routable but black-holed address: the dial hangs rather than being
	// refused, which is exactly the case the timeout exists for.
	start := time.Now()
	res := Check(context.Background(), "10.255.255.1:22", 200*time.Millisecond)
	elapsed := time.Since(start)

	if res.State != Down {
		t.Errorf("State = %v, want down", res.State)
	}
	// Allow generous slack for a slow CI box, but it must not have waited for
	// the OS-level TCP timeout, which is tens of seconds.
	if elapsed > 5*time.Second {
		t.Errorf("probe took %s, so the timeout was not applied", elapsed)
	}
}

func TestCheckAllProbesEveryAddress(t *testing.T) {
	up1, up2 := listener(t), listener(t)
	down := deadAddr(t)

	results := map[string]State{}
	for r := range CheckAll(context.Background(), []string{up1, up2, down}, time.Second) {
		results[r.Addr] = r.State
	}

	if len(results) != 3 {
		t.Fatalf("want a result per address, got %d", len(results))
	}
	if results[up1] != Up || results[up2] != Up {
		t.Errorf("listening addresses reported as %v / %v", results[up1], results[up2])
	}
	if results[down] != Down {
		t.Errorf("dead address reported as %v", results[down])
	}
}

func TestCheckAllClosesItsChannel(t *testing.T) {
	ch := CheckAll(context.Background(), []string{listener(t)}, time.Second)

	// Draining must terminate. A channel that is never closed hangs the caller,
	// which in the TUI means a spinner that never stops.
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("CheckAll never closed its channel")
	}
}

func TestCheckAllStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	done := make(chan struct{})
	go func() {
		for range CheckAll(ctx, []string{"10.255.255.1:22"}, 10*time.Second) {
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("a cancelled sweep should return promptly, not wait out the timeout")
	}
}

func TestStateString(t *testing.T) {
	// The TUI must be able to distinguish "not checked yet" from "unreachable",
	// so they cannot collapse to the same value.
	if Unknown == Down {
		t.Fatal("unknown and down must be distinct states")
	}
	for s, want := range map[State]string{
		Unknown:  "unknown",
		Checking: "checking",
		Up:       "up",
		Down:     "down",
	} {
		if got := s.String(); got != want {
			t.Errorf("State(%d).String() = %q, want %q", s, got, want)
		}
	}
}
