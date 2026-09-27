//go:build !linux && !darwin

package handlers

import "errors"

func publisherAvailableBytes(string) (uint64, error) {
	return 0, errors.New("destination free-space information is unavailable on this platform")
}
