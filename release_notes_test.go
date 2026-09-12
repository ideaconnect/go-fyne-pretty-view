package prettyview

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestReleaseNotesScript exercises .github/scripts/release-notes.sh, which the release
// workflow runs before any build: the tag's CHANGELOG section must come back verbatim,
// the heading's title must become the release name, and a missing or empty section must
// fail so a tag is never published without an account of what it contains. It also runs
// the script against the real CHANGELOG for the newest section, so a heading typo on a
// release day is caught by `make check` rather than by the tag push.
func TestReleaseNotesScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("bash script; covered on Linux/macOS")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	script, err := filepath.Abs(filepath.Join(".github", "scripts", "release-notes.sh"))
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		out, err := exec.Command("bash", append([]string{script}, args...)...).Output()
		return strings.TrimSpace(string(out)), err
	}

	dir := t.TempDir()
	changelog := filepath.Join(dir, "CHANGELOG.md")
	const fixture = `# Changelog

## [Unreleased]

_Nothing pending._

## [v9.9.9] - 2030-01-02 - a title with: punctuation

### Added
- one thing
- another

## [v9.9.8] — 2029-12-31 — old em-dash style

body of the older release

## [v9.9.7] - 2029-12-30

## [v9.9.6]
`
	if err := os.WriteFile(changelog, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}

	body, err := run("v9.9.9", changelog)
	if err != nil {
		t.Fatalf("v9.9.9 body: %v", err)
	}
	if want := "### Added\n- one thing\n- another"; body != want {
		t.Errorf("v9.9.9 body = %q, want %q", body, want)
	}
	if strings.Contains(body, "v9.9.8") || strings.Contains(body, "old em-dash") {
		t.Error("body leaked into the next section")
	}

	if name, err := run("--title", "v9.9.9", changelog); err != nil || name != "v9.9.9: a title with: punctuation" {
		t.Errorf("--title v9.9.9 = %q, %v; want %q", name, err, "v9.9.9: a title with: punctuation")
	}
	if name, err := run("--title", "v9.9.8", changelog); err != nil || name != "v9.9.8: old em-dash style" {
		t.Errorf("--title v9.9.8 = %q, %v (em-dash heading)", name, err)
	}
	if body, err := run("v9.9.8", changelog); err != nil || body != "body of the older release" {
		t.Errorf("v9.9.8 body = %q, %v", body, err)
	}
	if name, err := run("--title", "v9.9.7", changelog); err != nil || name != "v9.9.7" {
		t.Errorf("--title without a title = %q, %v; want the bare tag", name, err)
	}
	if _, err := run("v9.9.6", changelog); err == nil {
		t.Error("an empty section must fail")
	}
	if _, err := run("v0.0.0", changelog); err == nil {
		t.Error("a missing section must fail")
	}

	// The real CHANGELOG: its newest released section must extract, or the next tag push
	// fails at the notes job.
	real, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	var newest string
	for _, line := range strings.Split(string(real), "\n") {
		if strings.HasPrefix(line, "## [v") {
			newest = line[len("## ["):strings.Index(line, "]")]
			break
		}
	}
	if newest == "" {
		t.Fatal("no released section in CHANGELOG.md")
	}
	if body, err := run(newest, "CHANGELOG.md"); err != nil || body == "" {
		t.Errorf("newest section %s does not extract: %v", newest, err)
	}
}
