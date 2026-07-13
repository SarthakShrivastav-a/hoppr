//go:build !windows

package keys

import (
	"fmt"
	"os"
)

// On Unix the question is simply whether anyone but the owner can read the key.
// OpenSSH rejects a private key whose group or other bits are set at all.

// safeMode is what a private key should be: read/write for the owner, nothing
// for anyone else.
const safeMode os.FileMode = 0o600

// groupOtherBits are the permission bits OpenSSH objects to on a private key.
const groupOtherBits os.FileMode = 0o077

func audit(path string) PermStatus {
	st, err := os.Stat(path)
	if err != nil {
		return PermStatus{Detail: fmt.Sprintf("cannot stat: %v", err)}
	}

	perm := st.Mode().Perm()
	if bad := perm & groupOtherBits; bad != 0 {
		return PermStatus{
			Detail:  fmt.Sprintf("mode %04o is group/world accessible — ssh will refuse this key", perm),
			Fixable: true,
		}
	}
	return PermStatus{OK: true, Detail: fmt.Sprintf("mode %04o", perm)}
}

func fix(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}

	mode := safeMode
	if st.IsDir() {
		// A directory needs the execute bit to be traversable at all.
		mode = 0o700
	}
	if err := os.Chmod(path, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", path, err)
	}
	return nil
}
