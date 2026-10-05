package main

import "testing"

func loadGroups(t *testing.T) []Group {
	t.Helper()
	report, err := ReadReport("testdata/fix.json")
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	return GroupFixes(report.Fixes)
}

func findGroup(t *testing.T, groups []Group, name string) Group {
	t.Helper()
	for _, g := range groups {
		if g.Name() == name {
			return g
		}
	}
	t.Fatalf("group %s is not found", name)
	return Group{}
}

func TestGroupFixesKeepsReportOrder(t *testing.T) {
	groups := loadGroups(t)
	if len(groups) != 15 {
		t.Fatalf("got %d groups, want 15", len(groups))
	}
	want := []string{
		"@angular/platform-server@20.3.27",
		"browserslist@4.28.4",
		"fast-uri@3.1.5",
		"js-yaml@3.15.1",
		"js-yaml@4.3.1",
		"smol-toml@1.6.1",
		"svgo@2.8.3",
	}
	for i, name := range want {
		if groups[i].Name() != name {
			t.Errorf("group %d is %s, want %s", i, groups[i].Name(), name)
		}
	}
	// The same package in two versions has to stay in two groups.
	if n := len(findGroup(t, groups, "js-yaml@3.15.1").Fixes); n != 1 {
		t.Errorf("js-yaml@3.15.1 has %d fixes, want 1", n)
	}
	// Advisories of the same package are collected, including different severities.
	if n := len(findGroup(t, groups, "svgo@2.8.3").Fixes); n != 2 {
		t.Errorf("svgo@2.8.3 has %d fixes, want 2", n)
	}
}

func TestFixableOnlyOverrideBlocks(t *testing.T) {
	groups := loadGroups(t)
	var blocked []string
	for _, g := range groups {
		if !g.Fixable() {
			blocked = append(blocked, g.Name())
		}
	}
	want := map[string]bool{"smol-toml@1.6.1": true, "body-parser@2.2.2": true}
	if len(blocked) != len(want) {
		t.Fatalf("blocked groups %v, want %v", blocked, want)
	}
	for _, name := range blocked {
		if !want[name] {
			t.Errorf("group %s is blocked unexpectedly", name)
		}
	}
	// A group mixing "bump" and "update-parent" is still fixable.
	if !findGroup(t, groups, "@angular/common@20.3.27").Fixable() {
		t.Error("@angular/common@20.3.27 has to be fixable")
	}
}

func TestUpdateCommands(t *testing.T) {
	groups := loadGroups(t)

	// Four advisories of the same bump collapse into a single command.
	fastURI := findGroup(t, groups, "fast-uri@3.1.5")
	if len(fastURI.Fixes) != 4 {
		t.Fatalf("fast-uri@3.1.5 has %d fixes, want 4", len(fastURI.Fixes))
	}
	updates := fastURI.UpdateCommands()
	if len(updates) != 1 || updates[0].Package != "fast-uri" || updates[0].To != "3.1.6" {
		t.Errorf("fast-uri commands are %v, want one fast-uri@3.1.6", updates)
	}

	// The bump of the package itself plus the four parents.
	common := findGroup(t, groups, "@angular/common@20.3.27")
	wantPackages := []string{"@angular/common", "@angular/forms", "@angular/platform-browser", "@angular/router", "@angular/platform-server"}
	updates = common.UpdateCommands()
	if len(updates) != len(wantPackages) {
		t.Fatalf("@angular/common commands are %v, want %d of them", updates, len(wantPackages))
	}
	for i, pkg := range wantPackages {
		if updates[i].Package != pkg {
			t.Errorf("command %d is for %s, want %s", i, updates[i].Package, pkg)
		}
		if updates[i].To != "20.3.28" {
			t.Errorf("command %d bumps to %s, want 20.3.28", i, updates[i].To)
		}
	}

	// Both the package and its parent are bumped.
	qs := findGroup(t, groups, "qs@6.15.3")
	updates = qs.UpdateCommands()
	if len(updates) != 2 {
		t.Fatalf("qs commands are %v, want 2 of them", updates)
	}
	if updates[0].Package != "body-parser" || updates[0].To != "1.20.8" {
		t.Errorf("first qs command is %v, want body-parser@1.20.8", updates[0])
	}
	if updates[1].Package != "qs" || updates[1].To != "6.16.0" {
		t.Errorf("second qs command is %v, want qs@6.16.0", updates[1])
	}
}

func TestUpdateCommandsKeepsHighestVersion(t *testing.T) {
	g := Group{Module: "a", Current: "1.0.0", Fixes: []Fix{
		{Resolution: "bump", Update: &Update{Package: "a", To: "1.2.0"}},
		{Resolution: "bump", Update: &Update{Package: "a", To: "1.10.0"}},
		{Resolution: "bump", Update: &Update{Package: "a", To: "1.3.0"}},
	}}
	updates := g.UpdateCommands()
	if len(updates) != 1 || updates[0].To != "1.10.0" {
		t.Errorf("commands are %v, want one a@1.10.0", updates)
	}
}

func TestPatchedVersion(t *testing.T) {
	groups := loadGroups(t)
	cases := map[string]string{
		"browserslist@4.28.4":              "4.28.7",
		"@angular/common@20.3.27":          "20.3.28",
		"svgo@2.8.3":                       "2.8.4",
		"@angular/platform-server@20.3.27": "20.3.30",
	}
	for name, want := range cases {
		if got := findGroup(t, groups, name).PatchedVersion(); got != want {
			t.Errorf("%s patched version is %q, want %q", name, got, want)
		}
	}

	// The highest of several patched versions wins.
	g := Group{Fixes: []Fix{{Patched: ">=1.2.3"}, {Patched: ">=1.10.0"}, {Patched: "^1.4.0"}}}
	if got := g.PatchedVersion(); got != "1.10.0" {
		t.Errorf("patched version is %q, want 1.10.0", got)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.2.3", "1.2.3", 0},
		{"1.2.3", "1.2.4", -1},
		{"1.10.0", "1.9.9", 1},
		{"2.0", "2.0.0", 0},
		{"2.0.1", "2.0", 1},
		{">=4.28.7", "4.28.7", 0},
		{"1.2.3-beta.1", "1.2.3", 0},
		{"20.3.30", "20.3.28", 1},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
