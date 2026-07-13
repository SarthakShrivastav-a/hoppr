// Package keys inventories SSH private keys, checks that their permissions are
// actually safe, and adopts stray ones into a single managed directory.
//
// This is the part of SSH that no terminal manager handles today. Keys arrive
// from a cloud console as a .pem in ~/Downloads and stay there. Six months on
// there are four of them in three directories, the connection command that uses
// them is memorized or pasted from a note, and on Windows at least one of them
// is readable by every account on the machine.
package keys

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// maxKeySize bounds how much of a candidate file we are willing to read.
//
// Scanning a directory means opening files we have no guarantee about. A
// private key is a couple of kilobytes; anything larger is not one, and we
// should not pull a 4GB ISO into memory to find that out.
const maxKeySize = 64 << 10

// Key is one private key on disk.
type Key struct {
	Path string

	// PublicPath is the sibling .pub file, if there is one.
	PublicPath string

	// Type is the OpenSSH key type, e.g. "ssh-ed25519" or "ssh-rsa".
	Type string

	// Fingerprint is the SHA256 fingerprint, in the form ssh-keygen prints.
	// Empty when the key is encrypted and has no .pub sibling to read it from.
	Fingerprint string

	// Comment is the trailing comment on the public key, usually user@machine.
	Comment string

	// Encrypted reports whether the key is passphrase-protected.
	Encrypted bool

	// Perm describes whether the file's permissions are safe.
	Perm PermStatus
}

// Name is the key's file name, which is how it is labelled in the UI.
func (k *Key) Name() string { return filepath.Base(k.Path) }

// ShortFingerprint trims the "SHA256:" prefix and truncates, for a list column
// where the full 43 characters would not fit.
func (k *Key) ShortFingerprint() string {
	fp := strings.TrimPrefix(k.Fingerprint, "SHA256:")
	if fp == "" {
		return "—"
	}
	if len(fp) > 16 {
		return fp[:16] + "…"
	}
	return fp
}

// Describe renders the key's type and strength for display.
func (k *Key) Describe() string {
	t := strings.TrimPrefix(k.Type, "ssh-")
	if t == "" {
		t = "unknown"
	}
	if k.Encrypted {
		return t + " (encrypted)"
	}
	return t
}

// ErrNotAKey is returned for a file that is not an SSH private key. It is an
// expected outcome while scanning a directory, not a failure.
var ErrNotAKey = errors.New("not an ssh private key")

// privateKeyMarker is what every supported private key format opens with:
// "-----BEGIN OPENSSH PRIVATE KEY-----", "-----BEGIN RSA PRIVATE KEY-----",
// and so on.
//
// Sniffing the content rather than trusting the extension is deliberate. A key
// downloaded from AWS is called prod.pem, one from ssh-keygen is called id_ed25519
// with no extension at all, and a file called backup.pem might be a certificate.
// The PEM header is the only thing that reliably says "this is a private key".
const privateKeyMarker = "PRIVATE KEY-----"

// Load reads and inspects the private key at path.
func Load(path string) (*Key, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() || st.Size() > maxKeySize {
		return nil, ErrNotAKey
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !looksLikePrivateKey(raw) {
		return nil, ErrNotAKey
	}

	k := &Key{Path: path, Perm: Audit(path)}

	// A passphrase-protected key cannot be fully parsed without the
	// passphrase, but OpenSSH's own format stores the public half in the
	// clear, so we usually still get a type and a fingerprint out of it.
	signer, err := ssh.ParsePrivateKey(raw)
	switch {
	case err == nil:
		k.setFromPublic(signer.PublicKey())
	case isPassphraseMissing(err):
		k.Encrypted = true
		var pm *ssh.PassphraseMissingError
		if errors.As(err, &pm) && pm.PublicKey != nil {
			k.setFromPublic(pm.PublicKey)
		}
	default:
		// It has a private key header but will not parse. Report it rather
		// than skipping it: a corrupt key is exactly the kind of thing the
		// user wants to be told about.
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}

	// Fall back to the .pub sibling for anything still missing — an encrypted
	// PEM-format key gives us nothing at all otherwise.
	if pub := path + ".pub"; fileExists(pub) {
		k.PublicPath = pub
		if k.Fingerprint == "" || k.Comment == "" {
			k.readPublicFile(pub)
		}
	}
	return k, nil
}

func (k *Key) setFromPublic(pub ssh.PublicKey) {
	k.Type = pub.Type()
	k.Fingerprint = ssh.FingerprintSHA256(pub)
}

func (k *Key) readPublicFile(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pub, comment, _, _, err := ssh.ParseAuthorizedKey(raw)
	if err != nil {
		return
	}
	if k.Fingerprint == "" {
		k.setFromPublic(pub)
	}
	if k.Comment == "" {
		k.Comment = comment
	}
}

func looksLikePrivateKey(raw []byte) bool {
	// The header is on the first line, but tolerate a leading blank line or a
	// UTF-8 BOM, both of which Windows editors are happy to add.
	head := raw
	if len(head) > 256 {
		head = head[:256]
	}
	return strings.Contains(string(head), privateKeyMarker)
}

func isPassphraseMissing(err error) bool {
	var pm *ssh.PassphraseMissingError
	return errors.As(err, &pm)
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
