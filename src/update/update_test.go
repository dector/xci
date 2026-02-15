package update

import (
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"xci/internal/utils"
	"xci/src/tools/dnf5"
)

func TestRegisteredBackendsIncludesDNF5WhenFound(t *testing.T) {
	setUpdateDeps(t,
		func(name string) (string, error) { return "/usr/bin/" + name, nil },
		func() utils.DistroFamily { return utils.DistroFamilyUnknown },
	)

	names := backendNames(registeredBackends())
	if !containsName(names, "dnf5") {
		t.Fatalf("expected dnf5 backend to be included, got: %#v", names)
	}
}

func TestRegisteredBackendsIncludesDNF5WhenMissingOnFedoraLike(t *testing.T) {
	setUpdateDeps(t,
		func(name string) (string, error) { return "", exec.ErrNotFound },
		func() utils.DistroFamily { return utils.DistroFamilyFedoraLike },
	)

	names := backendNames(registeredBackends())
	if !containsName(names, "dnf5") {
		t.Fatalf("expected dnf5 backend to be included, got: %#v", names)
	}
}

func TestRegisteredBackendsSkipsDNF5WhenMissingOnNonFedoraLike(t *testing.T) {
	setUpdateDeps(t,
		func(name string) (string, error) { return "", exec.ErrNotFound },
		func() utils.DistroFamily { return utils.DistroFamilyNonFedoraLike },
	)

	names := backendNames(registeredBackends())
	if containsName(names, "dnf5") {
		t.Fatalf("expected dnf5 backend to be skipped, got: %#v", names)
	}
}

func TestRegisteredBackendsSkipsDNF5WhenMissingOnUnknown(t *testing.T) {
	setUpdateDeps(t,
		func(name string) (string, error) { return "", exec.ErrNotFound },
		func() utils.DistroFamily { return utils.DistroFamilyUnknown },
	)

	names := backendNames(registeredBackends())
	if containsName(names, "dnf5") {
		t.Fatalf("expected dnf5 backend to be skipped, got: %#v", names)
	}
}

func TestRegisteredBackendsIncludesDNF5OnLookupError(t *testing.T) {
	setUpdateDeps(t,
		func(name string) (string, error) { return "", errors.New("boom") },
		func() utils.DistroFamily { return utils.DistroFamilyNonFedoraLike },
	)

	names := backendNames(registeredBackends())
	if !containsName(names, "dnf5") {
		t.Fatalf("expected dnf5 backend to be included, got: %#v", names)
	}
}

func setUpdateDeps(t *testing.T, lookPath func(string) (string, error), detectFamily func() utils.DistroFamily) {
	t.Helper()

	originalLookPath := lookPathFunc
	originalDetect := detectDistroFamilyFunc

	lookPathFunc = lookPath
	detectDistroFamilyFunc = detectFamily

	t.Cleanup(func() {
		lookPathFunc = originalLookPath
		detectDistroFamilyFunc = originalDetect
	})
}

func backendNames(backends []toolBackend) []string {
	names := make([]string, 0, len(backends))
	for _, backend := range backends {
		names = append(names, backend.Name)
	}

	return names
}

func containsName(names []string, needle string) bool {
	for _, name := range names {
		if name == needle {
			return true
		}
	}

	return false
}

func TestFormatUpdatedNames(t *testing.T) {
	t.Parallel()

	got := formatUpdatedNames([]string{"foo", " baz ", "foo", "", "bar"})
	if got != "bar baz foo" {
		t.Fatalf("unexpected formatted package names: %q", got)
	}
}

func TestOverallSummaryLines(t *testing.T) {
	t.Parallel()

	summaries := []toolUpdateSummary{
		{
			ToolName:     "mise",
			UpdatedNames: []string{"foo", "bar", "foo"},
		},
		{
			ToolName:     "dnf5",
			UpdatedNames: []string{"zeta", "alpha"},
		},
		{
			ToolName: "flatpak",
		},
	}

	got := overallSummaryLines(summaries, 4, 1)
	want := []string{
		"Updated: 4",
		"Failed: 1",
		"",
		"Updated packages by tool:",
		"mise: bar foo",
		"dnf5: alpha zeta",
		"flatpak: (none)",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected summary lines:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestFramedBlock(t *testing.T) {
	t.Parallel()

	block := framedBlock([]string{"Updated: 2", "mise: foo bar"})
	if !strings.Contains(block, "┌───────────────┐") {
		t.Fatalf("expected top border in frame, got: %q", block)
	}
	if !strings.Contains(block, "│ Updated: 2    │") {
		t.Fatalf("expected padded first line in frame, got: %q", block)
	}
	if !strings.Contains(block, "│ mise: foo bar │") {
		t.Fatalf("expected second line in frame, got: %q", block)
	}
	if !strings.Contains(block, "└───────────────┘") {
		t.Fatalf("expected bottom border in frame, got: %q", block)
	}
}

func TestFramedBlockIgnoresANSIInWidth(t *testing.T) {
	t.Parallel()

	block := framedBlock([]string{"Updated: \x1b[32m2\x1b[0m", "tool: \x1b[31mfoo\x1b[0m"})
	if !strings.Contains(block, "┌────────────┐") {
		t.Fatalf("expected compact frame width with ANSI text, got: %q", block)
	}
	if !strings.Contains(block, "│ tool: \x1b[31mfoo\x1b[0m  │") {
		t.Fatalf("expected ANSI line to keep visual padding, got: %q", block)
	}
}

func TestMapDnf5OutdatedPackagesIncludesCurrent(t *testing.T) {
	t.Parallel()

	input := []dnf5.OutdatedPackage{
		{
			Name:       "bash",
			Arch:       "x86_64",
			Repository: "updates",
			Current:    "5.2-7.fc41",
			Latest:     "5.2-8.fc41",
		},
	}

	got := mapDnf5OutdatedPackages(input)
	want := []updatePackage{
		{
			Name:      "bash.x86_64",
			Requested: "updates",
			Current:   "5.2-7.fc41",
			Latest:    "5.2-8.fc41",
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected mapped packages\nwant: %#v\ngot:  %#v", want, got)
	}
}
