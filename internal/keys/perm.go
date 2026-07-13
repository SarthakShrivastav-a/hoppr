package keys

// PermStatus is the verdict on one key file's permissions.
type PermStatus struct {
	// OK reports whether OpenSSH will accept the key without complaint.
	OK bool

	// Detail is a one-line, human explanation. It is shown verbatim in the UI
	// and by `hoppr doctor`, so it says what is wrong, not just that something
	// is.
	Detail string

	// Fixable reports whether Fix can repair this without the user doing
	// anything by hand.
	Fixable bool
}

// Audit reports whether the key file at path has safe permissions.
//
// The implementation is per-platform because the question is genuinely
// different on each. On Unix it is a mode bit check. On Windows the mode bits
// are a fiction and only the NTFS ACL matters — which is why `chmod 600` is
// cargo-culted advice that silently does nothing there.
func Audit(path string) PermStatus {
	return audit(path)
}

// Fix repairs the permissions on the key file at path.
func Fix(path string) error {
	return fix(path)
}

// Harden applies safe permissions to a newly created key or directory. It is
// the same operation as Fix, named for the case where nothing was wrong yet.
func Harden(path string) error {
	return fix(path)
}
