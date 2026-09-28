//go:build !linux && !darwin

package handlers

import "os"

func atomicRenameNoReplace(source, destination string) error {
	if err := os.Link(source, destination); err != nil {
		return err
	}
	return os.Remove(source)
}
