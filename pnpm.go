package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// run executes a command in the current directory and streams its output.
func run(name string, args ...string) error {
	fmt.Printf("\n$ %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

// output executes a command and returns its standard output.
func output(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func pnpmDedupe() error {
	return run("pnpm", "dedupe")
}

func pnpmAdd(pkg, version string) error {
	return run("pnpm", "add", "-w", pkg+"@"+version)
}

func pnpmInstallLockfileOnly() error {
	return run("pnpm", "install", "--lockfile-only")
}
