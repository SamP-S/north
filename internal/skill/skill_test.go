package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentHasVersionAndFrontmatter(t *testing.T) {
	c := Content()
	if !strings.HasPrefix(c, "---\n") {
		t.Error("skill should start with frontmatter")
	}
	if !strings.Contains(c, "north-skill-version: "+Version) {
		t.Error("version comment not injected")
	}
	if !strings.Contains(c, "name: north") {
		t.Error("missing skill name")
	}
}

func TestInstalledVersionRoundTrip(t *testing.T) {
	if got := InstalledVersion(Content()); got != Version {
		t.Errorf("InstalledVersion = %q, want %q", got, Version)
	}
	if got := InstalledVersion("no stamp here"); got != "" {
		t.Errorf("expected empty for unstamped content, got %q", got)
	}
}

func TestInstallProject(t *testing.T) {
	root := t.TempDir()
	targets, err := Install(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) == 0 {
		t.Fatal("no targets written")
	}
	for _, want := range []string{
		filepath.Join(root, ".claude", "skills", "north", "SKILL.md"),
		filepath.Join(root, ".opencode", "skills", "north", "SKILL.md"),
	} {
		data, err := os.ReadFile(want)
		if err != nil {
			t.Errorf("missing %s: %v", want, err)
			continue
		}
		if !strings.Contains(string(data), "north-skill-version:") {
			t.Errorf("%s missing version comment", want)
		}
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	root := t.TempDir()
	if _, err := Install(root, false); err != nil {
		t.Fatal(err)
	}
	// Re-installing overwrites cleanly without error.
	targets, err := Install(root, false)
	if err != nil {
		t.Fatalf("re-install: %v", err)
	}
	data, err := os.ReadFile(targets[0].Path)
	if err != nil || !strings.Contains(string(data), "name: north") {
		t.Errorf("re-installed file wrong: %v", err)
	}
}

func TestAgentsRegistry(t *testing.T) {
	names := map[string]bool{}
	for _, a := range agents {
		names[a.Name] = true
	}
	if !names["claude"] || !names["opencode"] {
		t.Errorf("expected claude and opencode agents, got %v", names)
	}
}

func TestInstallGlobalTargetsHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	targets, err := Targets("", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range targets {
		if !strings.HasPrefix(tg.Dir, home) {
			t.Errorf("global target not under home: %s", tg.Dir)
		}
	}
}
