package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func main() {
	addressSpace := flag.Uint64("address-space-bytes", 0, "maximum child address space")
	cpuSeconds := flag.Uint64("cpu-seconds", 0, "maximum child CPU time")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 || *addressSpace < 64<<20 || *cpuSeconds == 0 || *cpuSeconds > 120 {
		fmt.Fprintln(os.Stderr, "invalid bounded-exec limits or command")
		os.Exit(2)
	}
	if err := setLimit(syscall.RLIMIT_AS, *addressSpace); err != nil {
		fmt.Fprintln(os.Stderr, "could not apply child address-space limit")
		os.Exit(126)
	}
	if err := setLimit(syscall.RLIMIT_CPU, *cpuSeconds); err != nil {
		fmt.Fprintln(os.Stderr, "could not apply child CPU-time limit")
		os.Exit(126)
	}
	if err := setLimit(syscall.RLIMIT_CORE, 0); err != nil {
		fmt.Fprintln(os.Stderr, "could not disable child core dumps")
		os.Exit(126)
	}

	executable, err := exec.LookPath(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "bounded child executable is unavailable")
		os.Exit(127)
	}
	if err := syscall.Exec(executable, args, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "could not execute bounded child")
		os.Exit(126)
	}
}

func setLimit(resource int, value uint64) error {
	limit := syscall.Rlimit{Cur: value, Max: value}
	return syscall.Setrlimit(resource, &limit)
}
