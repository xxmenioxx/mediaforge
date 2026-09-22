package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

const maxFailureLines = 60

type testEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

type testKey struct {
	pkg  string
	test string
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: filter-go-test-json <go-test-json-log>")
		os.Exit(2)
	}

	failedTests, failedPackages, err := collectFailures(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	testOutput, packageOutput, err := collectFailureOutput(os.Args[1], failedTests, failedPackages)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	keys := make([]testKey, 0, len(failedTests))
	packageHasFailedTest := map[string]bool{}
	for key := range failedTests {
		keys = append(keys, key)
		packageHasFailedTest[key.pkg] = true
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].pkg == keys[j].pkg {
			return keys[i].test < keys[j].test
		}
		return keys[i].pkg < keys[j].pkg
	})

	printed := false
	for _, key := range keys {
		printed = true
		fmt.Printf("Failed test: %s (%s)\n", key.test, key.pkg)
		printLines(testOutput[key])
		fmt.Println()
	}

	packages := make([]string, 0, len(failedPackages))
	for pkg := range failedPackages {
		if !packageHasFailedTest[pkg] {
			packages = append(packages, pkg)
		}
	}
	sort.Strings(packages)
	for _, pkg := range packages {
		printed = true
		fmt.Printf("Failed package: %s\n", pkg)
		printLines(packageOutput[pkg])
		fmt.Println()
	}

	if !printed {
		os.Exit(1)
	}
}

func collectFailures(path string) (map[testKey]bool, map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	failedTests := map[testKey]bool{}
	failedPackages := map[string]bool{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var event testEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Action != "fail" {
			continue
		}
		if event.Test != "" {
			failedTests[testKey{pkg: event.Package, test: event.Test}] = true
		} else if event.Package != "" {
			failedPackages[event.Package] = true
		}
	}
	return failedTests, failedPackages, scanner.Err()
}

func collectFailureOutput(path string, failedTests map[testKey]bool, failedPackages map[string]bool) (map[testKey][]string, map[string][]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()

	testOutput := map[testKey][]string{}
	packageOutput := map[string][]string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var event testEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Action != "output" || event.Output == "" {
			continue
		}
		if event.Test != "" {
			key := testKey{pkg: event.Package, test: event.Test}
			if failedTests[key] {
				testOutput[key] = appendBounded(testOutput[key], event.Output)
			}
			continue
		}
		if failedPackages[event.Package] {
			packageOutput[event.Package] = appendBounded(packageOutput[event.Package], event.Output)
		}
	}
	return testOutput, packageOutput, scanner.Err()
}

func appendBounded(lines []string, output string) []string {
	parts := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	lines = append(lines, parts...)
	if len(lines) > maxFailureLines {
		lines = append([]string(nil), lines[len(lines)-maxFailureLines:]...)
	}
	return lines
}

func printLines(lines []string) {
	if len(lines) == 0 {
		fmt.Println("(no test-scoped output captured)")
		return
	}
	for _, line := range lines {
		fmt.Println(line)
	}
}
