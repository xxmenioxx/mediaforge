//go:build darwin

package handlers

import "golang.org/x/sys/unix"

func atomicRenameNoReplace(source, destination string) error {
	return unix.RenamexNp(source, destination, unix.RENAME_EXCL)
}
