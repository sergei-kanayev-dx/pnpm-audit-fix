package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

var stdin = bufio.NewReader(os.Stdin)

func main() {
	if err := runApp(); err != nil {
		fmt.Fprintf(os.Stderr, "\n%v\n", err)
		os.Exit(1)
	}
}

func runApp() error {
	autoYes := flag.Bool("y", false, "confirm every package automatically")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [-y] <report.json>\n\nRun it in the root of the pnpm repository to fix the audit report.\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		return fmt.Errorf("a path to the JSON report is required")
	}

	report, err := ReadReport(flag.Arg(0))
	if err != nil {
		return err
	}

	changed, err := gitStatusPaths(false)
	if err != nil {
		return err
	}
	if len(changed) > 0 {
		return fmt.Errorf("the repository has uncommitted changes, commit or stash them and try again:\n%s", formatPaths(changed))
	}

	if err := dedupeAndCommit(); err != nil {
		return err
	}

	groups := GroupFixes(report.Fixes)
	fmt.Printf("\n%d vulnerable package(s) in %s.\n", len(groups), flag.Arg(0))
	for _, g := range groups {
		if err := processGroup(g, *autoYes); err != nil {
			return err
		}
	}
	return nil
}

// dedupeAndCommit dedupes the store and commits the lockfile if it changed.
func dedupeAndCommit() error {
	if err := pnpmDedupe(); err != nil {
		return err
	}
	changed, err := gitStatusPaths(false)
	if err != nil {
		return err
	}
	if len(changed) == 0 {
		fmt.Println("\nDedupe changed nothing.")
		return nil
	}
	if !onlyLockChanged(changed) {
		return unexpectedChanges("pnpm dedupe", changed)
	}
	fmt.Printf("\nDedupe changed %s, committing it.\n", lockFile)
	return gitCommit("Dedupe", lockFile)
}

// processGroup fixes a single vulnerable package version.
func processGroup(g Group, autoYes bool) error {
	fmt.Printf("\n=== %s (%d advisory(ies)) ===\n", g.Name(), len(g.Fixes))
	if !g.Fixable() {
		fmt.Printf("Package %s can't be fixed automatically. Override it.\n", g.Name())
		return nil
	}

	patched := g.PatchedVersion()
	updates := g.UpdateCommands()
	fmt.Println("The following commands will be executed:")
	for _, u := range updates {
		fmt.Printf("  pnpm add -w %s@%s\n", u.Package, u.To)
	}
	if !autoYes && !confirm(fmt.Sprintf("Fix %s -> %s? [y/N] ", g.Name(), patched)) {
		fmt.Printf("Skipped %s.\n", g.Name())
		return nil
	}

	for _, u := range updates {
		if err := pnpmAdd(u.Package, u.To); err != nil {
			return err
		}
	}
	if err := pnpmDedupe(); err != nil {
		return err
	}
	if err := gitCheckout("package.json"); err != nil {
		return err
	}
	if err := pnpmInstallLockfileOnly(); err != nil {
		return err
	}

	changed, err := gitStatusPaths(false)
	if err != nil {
		return err
	}
	if len(changed) == 0 {
		fmt.Printf("\n%s is not changed, nothing to commit for %s.\n", lockFile, g.Name())
		return nil
	}
	if !onlyLockChanged(changed) {
		return unexpectedChanges(g.Name(), changed)
	}
	return gitCommit(fmt.Sprintf("Fix audit - update the %s package to the %s version", g.Name(), patched), lockFile)
}

func onlyLockChanged(paths []string) bool {
	for _, p := range paths {
		if p != lockFile {
			return false
		}
	}
	return len(paths) > 0
}

func unexpectedChanges(what string, paths []string) error {
	return fmt.Errorf("%s changed more than %s:\n%s\nthe working tree is left as is, check it manually", what, lockFile, formatPaths(paths))
}

func formatPaths(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&b, "  %s\n", p)
	}
	return strings.TrimRight(b.String(), "\n")
}

func confirm(question string) bool {
	fmt.Print(question)
	answer, err := stdin.ReadString('\n')
	if err != nil && answer == "" {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}
