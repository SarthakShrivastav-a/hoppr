// Package connect builds and launches ssh commands, and parses ad-hoc ones back
// into config entries.
package connect

import (
	"os"
	"os/exec"
	"strconv"

	"github.com/SarthakShrivastav-a/hoppr/internal/host"
)

// Options tune a single connection without touching the stored config.
type Options struct {
	// ConfigPath is passed to ssh as -F. Without it, a user pointed at a
	// non-default config via HOPPR_CONFIG would see hoppr list one set of hosts
	// and ssh connect to another.
	ConfigPath string

	// Forward is an optional extra port forward for this session only, in ssh's
	// command-line syntax (e.g. "-L", "8080:localhost:80").
	ForwardFlag string
	ForwardSpec string

	// Command runs on the remote host instead of an interactive shell.
	Command string

	// Verbose adds -v, which is the first thing anyone wants when a connection
	// is failing for reasons the config does not explain.
	Verbose bool
}

// Args builds the argument list for connecting to h.
//
// hoppr shells out to the system ssh rather than speaking the protocol itself.
// That is a deliberate constraint, not a shortcut: the user's config, their
// agent, their ProxyJump chain, their hardware keys, their Match blocks, and
// their known_hosts all work exactly as they already do. A manager that
// reimplements the client gets to reimplement all of that, badly, forever.
func Args(h *host.Host, opt Options) []string {
	var args []string

	if opt.ConfigPath != "" {
		args = append(args, "-F", opt.ConfigPath)
	}
	if opt.Verbose {
		args = append(args, "-v")
	}
	if opt.ForwardFlag != "" && opt.ForwardSpec != "" {
		args = append(args, opt.ForwardFlag, opt.ForwardSpec)
	}

	// Everything else — user, port, key, jump host — comes from the config
	// stanza. Passing them again on the command line would just be a second
	// source of truth that can disagree with the first.
	args = append(args, h.Alias)

	if opt.Command != "" {
		args = append(args, opt.Command)
	}
	return args
}

// Command builds the exec.Cmd that connects to h, wired to the real terminal.
//
// stdin, stdout, and stderr are inherited rather than captured. ssh's own error
// output is the single most useful thing on the screen when a connection fails,
// and swallowing it — or flashing it for a tenth of a second before redrawing a
// TUI over it — is a complaint filed against nearly every manager in this space.
func Command(h *host.Host, opt Options) *exec.Cmd {
	cmd := exec.Command(sshBinary(), Args(h, opt)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// Preview renders the command hoppr would run, for display in the UI. It is the
// answer to "what is this thing actually going to do", which a tool that hides
// the ssh invocation can never give.
func Preview(h *host.Host, opt Options) string {
	out := sshBinary()
	for _, a := range Args(h, opt) {
		out += " " + a
	}
	return out
}

// sshBinary returns the ssh executable to run.
//
// HOPPR_SSH overrides it, which covers the user who wants a specific build —
// a Windows box may have OpenSSH in System32, another in Git's bin directory,
// and a third from Scoop, and they do not all behave identically.
func sshBinary() string {
	if s := os.Getenv("HOPPR_SSH"); s != "" {
		return s
	}
	return "ssh"
}

// ForwardArgs renders a port forward as ssh command-line flags.
func ForwardArgs(kind string, localPort int, remoteHost string, remotePort int) (string, string) {
	switch kind {
	case "dynamic":
		return "-D", strconv.Itoa(localPort)
	case "remote":
		return "-R", strconv.Itoa(localPort) + ":" + remoteHost + ":" + strconv.Itoa(remotePort)
	default: // local
		return "-L", strconv.Itoa(localPort) + ":" + remoteHost + ":" + strconv.Itoa(remotePort)
	}
}
