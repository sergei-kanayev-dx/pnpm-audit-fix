package main

import (
	"strconv"
	"strings"
)

const lockFile = "pnpm-lock.yaml"

// gitStatusPaths returns the paths reported by "git status --porcelain".
func gitStatusPaths(includeUntracked bool) ([]string, error) {
	args := []string{"status", "--porcelain"}
	if !includeUntracked {
		args = append(args, "--untracked-files=no")
	}
	out, err := output("git", args...)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 4 {
			continue
		}
		// Every entry is "XY <path>"; renames and copies are "XY <old> -> <new>".
		p := strings.TrimSpace(line[3:])
		if i := strings.Index(p, " -> "); i >= 0 {
			p = p[i+len(" -> "):]
		}
		paths = append(paths, unquotePath(p))
	}
	return paths, nil
}

// unquotePath undoes the C style quoting git applies to unusual paths.
func unquotePath(p string) string {
	if !strings.HasPrefix(p, `"`) {
		return p
	}
	if s, err := strconv.Unquote(p); err == nil {
		return s
	}
	return p
}

func gitCheckout(paths ...string) error {
	return run("git", append([]string{"checkout", "--"}, paths...)...)
}

func gitCommit(message string, paths ...string) error {
	args := append([]string{"commit", "-m", message, "--"}, paths...)
	return run("git", args...)
}
