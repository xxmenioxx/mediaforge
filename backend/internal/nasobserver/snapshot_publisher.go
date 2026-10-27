package nasobserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const maxPublishedSnapshot = 1 << 20

// PublishSignedSnapshotAtomically publishes bytes already signed by an
// independently reviewed signer. No private key, service probe, or network
// access is provided here. The destination must be an existing regular file
// or absent, not a link; the parent must exist and not be a symlink.
func PublishSignedSnapshotAtomically(ctx context.Context, destination string, signed []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(destination) || len(signed) == 0 || len(signed) > maxPublishedSnapshot {
		return errors.New("absolute destination and bounded signed snapshot required")
	}
	parent := filepath.Dir(destination)
	info, err := os.Lstat(parent)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("snapshot parent must be an existing directory")
	}
	if existing, err := os.Lstat(destination); err == nil {
		if !existing.Mode().IsRegular() {
			return errors.New("snapshot destination is not a regular file")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect snapshot destination: %w", err)
	}
	// Never publish plaintext or structurally invalid envelopes. This checks only syntax;
	// signature verification and source authority remain responsibilities of
	// the canonical trusted reader and separately reviewed signer.
	if err := validateSignedEnvelopeShape(signed); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(parent, ".mvforge-observation-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if _, err = tmp.Write(signed); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Reject a symlink swap immediately before renaming as a defense-in-depth
	// check. The deployment must still secure its directory against untrusted
	// writers; directory ownership is outside this helper's authority.
	if existing, err := os.Lstat(destination); err == nil {
		if !existing.Mode().IsRegular() {
			return errors.New("snapshot destination changed type")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(name, destination); err != nil {
		return err
	}
	dir, err := os.Open(parent)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return err
	}
	return nil
}

func validateSignedEnvelopeShape(content []byte) error {
	// JSON validation is intentionally structural and DOES NOT assert signature
	// authenticity. Authenticated admission uses #431's canonical verifier.
	var envelope Envelope
	if err := validateUniqueJSON(content); err != nil {
		return err
	}
	if err := json.Unmarshal(content, &envelope); err != nil {
		return err
	}
	if len(envelope.Payload) == 0 || len(envelope.Signature) != 128 {
		return errors.New("incomplete signed envelope")
	}
	return nil
}
