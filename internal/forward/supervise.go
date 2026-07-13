package forward

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/SarthakShrivastav-a/hoppr/internal/connect"
	"github.com/SarthakShrivastav-a/hoppr/internal/host"
)

// Backoff bounds.
//
// A tunnel drops for two very different reasons: the laptop slept, or the
// server is gone. Reconnecting instantly is right for the first and a
// denial-of-service against your own bastion for the second, so back off — but
// cap it, because the laptop waking up should not then wait ten minutes.
const (
	minBackoff = 1 * time.Second
	maxBackoff = 30 * time.Second
)

// State is a tunnel's current condition.
type State int

const (
	Starting State = iota
	Up
	Retrying
	Stopped
)

func (s State) String() string {
	switch s {
	case Up:
		return "up"
	case Retrying:
		return "retrying"
	case Stopped:
		return "stopped"
	default:
		return "starting"
	}
}

// Status is a snapshot of a running tunnel.
type Status struct {
	Spec     Spec
	Host     string
	State    State
	Since    time.Time
	Restarts int

	// LastErr is why the tunnel last dropped. It is surfaced rather than
	// swallowed: "the tunnel is down" without a reason is the exact complaint
	// people make about GUI tunnel managers.
	LastErr error
}

// Tunnel is a supervised port forward.
//
// It runs `ssh -N` — no remote command, no shell — and restarts it when it
// dies, with exponential backoff and jitter.
type Tunnel struct {
	spec Spec
	host *host.Host
	opts connect.Options

	mu       sync.Mutex
	state    State
	since    time.Time
	restarts int
	lastErr  error

	cancel context.CancelFunc
	done   chan struct{}
}

// New builds a tunnel. It does not start it.
func New(h *host.Host, spec Spec, opts connect.Options) *Tunnel {
	return &Tunnel{
		spec:  spec,
		host:  h,
		opts:  opts,
		state: Stopped,
		done:  make(chan struct{}),
	}
}

// Status returns a snapshot.
func (t *Tunnel) Status() Status {
	t.mu.Lock()
	defer t.mu.Unlock()

	return Status{
		Spec:     t.spec,
		Host:     t.host.Alias,
		State:    t.state,
		Since:    t.since,
		Restarts: t.restarts,
		LastErr:  t.lastErr,
	}
}

// Start supervises the tunnel until ctx is cancelled or Stop is called.
//
// It blocks. Callers that want it in the background should run it in a
// goroutine and watch Status.
func (t *Tunnel) Start(ctx context.Context, log io.Writer) {
	ctx, cancel := context.WithCancel(ctx)

	t.mu.Lock()
	t.cancel = cancel
	t.mu.Unlock()

	defer close(t.done)
	defer cancel()

	backoff := minBackoff

	for {
		if ctx.Err() != nil {
			t.setState(Stopped, nil)
			return
		}

		t.setState(Starting, nil)
		err := t.runOnce(ctx)

		if ctx.Err() != nil {
			// We were asked to stop; ssh dying is the expected consequence, not
			// a failure worth reporting.
			t.setState(Stopped, nil)
			return
		}

		t.mu.Lock()
		t.restarts++
		n := t.restarts
		t.mu.Unlock()

		t.setState(Retrying, err)

		if log != nil {
			reason := "ssh exited"
			if err != nil {
				reason = err.Error()
			}
			fmt.Fprintf(log, "%s: %s — reconnecting in %s (attempt %d)\n",
				t.host.Alias, reason, backoff.Round(time.Millisecond), n)
		}

		select {
		case <-ctx.Done():
			t.setState(Stopped, nil)
			return
		case <-time.After(jitter(backoff)):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// runOnce runs ssh until it exits.
func (t *Tunnel) runOnce(ctx context.Context) error {
	args := []string{}
	if t.opts.ConfigPath != "" {
		args = append(args, "-F", t.opts.ConfigPath)
	}

	args = append(args,
		// -N: do not run a remote command. A forward does not need a shell, and
		// asking for one means a tunnel to a locked-down account fails for no
		// reason.
		"-N",
		// Fail fast instead of waiting on a TCP timeout, so the supervisor gets
		// to retry rather than hanging.
		"-o", "ExitOnForwardFailure=yes",
		"-o", "ServerAliveInterval=15",
		"-o", "ServerAliveCountMax=3",
		t.spec.Flag(), t.spec.Arg(),
		t.host.Alias,
	)

	cmd := exec.CommandContext(ctx, sshBinary(), args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	// ssh is up. It may still fail immediately — a bound port, a refused
	// forward — but there is no signal for "the forward is established", so
	// treat a process that stays alive as an established tunnel.
	t.setState(Up, nil)
	return cmd.Wait()
}

// Stop shuts the tunnel down and waits for it.
func (t *Tunnel) Stop() {
	t.mu.Lock()
	cancel := t.cancel
	t.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	<-t.done
}

func (t *Tunnel) setState(s State, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.state != s {
		t.since = time.Now()
	}
	t.state = s
	if err != nil {
		t.lastErr = err
	}
}

// jitter spreads reconnect attempts out.
//
// Without it, a laptop waking up with eight tunnels reconnects all eight in the
// same millisecond, which is exactly the burst a bastion's rate limiter is
// there to reject.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	delta := time.Duration(rand.Int63n(int64(d / 2)))
	return d/2 + delta
}

func sshBinary() string {
	if s := os.Getenv("HOPPR_SSH"); s != "" {
		return s
	}
	return "ssh"
}
