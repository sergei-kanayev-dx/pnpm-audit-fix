package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Report is the pnpm audit fix report (see fix.json).
type Report struct {
	Fixes []Fix `json:"fixes"`
}

// Fix is a single advisory together with the update that resolves it.
type Fix struct {
	Module     string  `json:"module"`
	Current    string  `json:"current"`
	Patched    string  `json:"patched"`
	Severity   string  `json:"severity"`
	AdvisoryID int     `json:"advisoryId"`
	Resolution string  `json:"resolution"`
	Update     *Update `json:"update"`
}

// Update tells which package has to be bumped and to which version.
type Update struct {
	Package string `json:"package"`
	From    string `json:"from"`
	To      string `json:"to"`
}

// Group collects all the advisories of a single vulnerable package version.
type Group struct {
	Module  string
	Current string
	Fixes   []Fix
}

// Name returns the vulnerable package as "module@current".
func (g Group) Name() string { return g.Module + "@" + g.Current }

// ReadReport loads and parses the JSON report at path.
func ReadReport(path string) (*Report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &r, nil
}

// GroupFixes groups the fixes by module@current, keeping the report order.
func GroupFixes(fixes []Fix) []Group {
	index := make(map[string]int)
	var groups []Group
	for _, f := range fixes {
		key := f.Module + "@" + f.Current
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, Group{Module: f.Module, Current: f.Current})
		}
		groups[i].Fixes = append(groups[i].Fixes, f)
	}
	return groups
}

// fixable reports whether the advisory can be resolved by bumping a package.
func (f Fix) fixable() bool {
	switch f.Resolution {
	case "bump", "update-parent":
	default:
		return false
	}
	return f.Update != nil && f.Update.Package != "" && f.Update.To != ""
}

// Fixable reports whether every advisory of the group can be fixed automatically.
func (g Group) Fixable() bool {
	if len(g.Fixes) == 0 {
		return false
	}
	for _, f := range g.Fixes {
		if !f.fixable() {
			return false
		}
	}
	return true
}

// UpdateCommands returns the packages to bump, one entry per package, keeping
// the highest requested version. The report order is preserved.
func (g Group) UpdateCommands() []Update {
	index := make(map[string]int)
	var updates []Update
	for _, f := range g.Fixes {
		if f.Update == nil || f.Update.Package == "" || f.Update.To == "" {
			continue
		}
		u := *f.Update
		if i, ok := index[u.Package]; ok {
			if compareVersions(u.To, updates[i].To) > 0 {
				updates[i].To = u.To
			}
			continue
		}
		index[u.Package] = len(updates)
		updates = append(updates, u)
	}
	return updates
}

// PatchedVersion returns the highest patched version of the group without the
// range operator, e.g. ">=4.28.7" becomes "4.28.7".
func (g Group) PatchedVersion() string {
	best := ""
	for _, f := range g.Fixes {
		v := cleanVersion(f.Patched)
		if v == "" {
			continue
		}
		if best == "" || compareVersions(v, best) > 0 {
			best = v
		}
	}
	return best
}

// cleanVersion strips a leading range operator from a version.
func cleanVersion(s string) string {
	return strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s), "><=^~v "))
}

// compareVersions compares two versions part by part, ignoring any prerelease
// or build suffix. It returns -1, 0 or 1.
func compareVersions(a, b string) int {
	ap, bp := versionParts(a), versionParts(b)
	n := len(ap)
	if len(bp) > n {
		n = len(bp)
	}
	for i := 0; i < n; i++ {
		av, bv := 0, 0
		if i < len(ap) {
			av = ap[i]
		}
		if i < len(bp) {
			bv = bp[i]
		}
		if av != bv {
			if av < bv {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = cleanVersion(v)
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	if v == "" {
		return nil
	}
	var parts []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		parts = append(parts, n)
	}
	return parts
}
