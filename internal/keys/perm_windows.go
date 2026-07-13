//go:build windows

package keys

import (
	"fmt"
	"os/exec"
	"os/user"
	"strings"
)

// On Windows, a private key's Unix mode bits are meaningless. OpenSSH checks
// the NTFS ACL, and refuses the key with
//
//	@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@
//	WARNING: UNPROTECTED PRIVATE KEY FILE!
//	Permissions for 'prod.pem' are too open.
//
// if anyone other than the owner, SYSTEM, or Administrators can read it.
//
// The trap is that users hit this without doing anything wrong: a new file in
// ~/.ssh inherits that directory's ACL, and on a normal Windows install that
// ACL grants read access to Users or Authenticated Users. Downloading a .pem
// from a cloud console and double-clicking into it is enough.
//
// `chmod 600` appears to succeed and changes nothing. The fix is to strip
// inheritance and grant the file to exactly one account, which is what icacls
// does below.

// allowedPrincipals may hold a key without OpenSSH objecting.
var allowedPrincipals = []string{
	`NT AUTHORITY\SYSTEM`,
	`BUILTIN\Administrators`,
}

func audit(path string) PermStatus {
	me, err := user.Current()
	if err != nil {
		return PermStatus{Detail: "cannot determine the current user", Fixable: false}
	}

	out, err := exec.Command("icacls", path).CombinedOutput()
	if err != nil {
		return PermStatus{
			Detail:  fmt.Sprintf("cannot read the ACL: %v", err),
			Fixable: true,
		}
	}

	var offenders []string
	for _, p := range parseICACLS(string(out), path) {
		if principalAllowed(p, me.Username) {
			continue
		}
		offenders = append(offenders, p)
	}

	if len(offenders) > 0 {
		return PermStatus{
			Detail:  fmt.Sprintf("readable by %s — ssh will refuse this key", strings.Join(offenders, ", ")),
			Fixable: true,
		}
	}
	return PermStatus{OK: true, Detail: "locked to " + me.Username}
}

func principalAllowed(principal, me string) bool {
	if strings.EqualFold(principal, me) {
		return true
	}
	for _, a := range allowedPrincipals {
		if strings.EqualFold(principal, a) {
			return true
		}
	}
	return false
}

// parseICACLS pulls the principal names out of icacls output, which looks like:
//
//	C:\Users\me\.ssh\prod.pem NT AUTHORITY\SYSTEM:(F)
//	                          BUILTIN\Administrators:(F)
//	                          DESKTOP-1\me:(F)
//	                          BUILTIN\Users:(RX)
//
// The first line carries the path; later lines are indented continuations. The
// principal is everything before the last ':' on the line, since a principal
// itself contains a backslash but never a colon.
func parseICACLS(out, path string) []string {
	var principals []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" {
			continue
		}
		// The summary line icacls prints at the end.
		if strings.HasPrefix(line, "Successfully processed") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, path))

		i := strings.LastIndex(line, ":")
		if i <= 0 {
			continue
		}
		p := strings.TrimSpace(line[:i])
		// Guard against a drive letter being mistaken for a separator on the
		// path-bearing first line.
		if p == "" || len(p) == 1 {
			continue
		}
		principals = append(principals, p)
	}
	return principals
}

func fix(path string) error {
	me, err := user.Current()
	if err != nil {
		return fmt.Errorf("determine current user: %w", err)
	}

	// /inheritance:r drops the ACL inherited from ~/.ssh, which is where the
	// over-broad grant came from. /grant:r then replaces any existing grant for
	// this user rather than adding a second one.
	steps := [][]string{
		{path, "/inheritance:r"},
		{path, "/grant:r", me.Username + ":(F)"},
	}
	for _, args := range steps {
		if out, err := exec.Command("icacls", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("icacls %s: %w: %s", strings.Join(args[1:], " "), err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}
