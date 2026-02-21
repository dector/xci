package update

import (
	"bytes"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"
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

func TestSelectBackends(t *testing.T) {
	t.Parallel()

	available := []toolBackend{
		{Name: "mise"},
		{Name: "flatpak"},
		{Name: "dnf5"},
	}

	testCases := []struct {
		name    string
		args    []string
		want    []string
		wantErr string
	}{
		{name: "default all", args: nil, want: []string{"mise", "flatpak", "dnf5"}},
		{name: "all keyword", args: []string{"all"}, want: []string{"mise", "flatpak", "dnf5"}},
		{name: "single subsystem", args: []string{"mise"}, want: []string{"mise"}},
		{name: "multiple subsystems", args: []string{"mise", "flatpak"}, want: []string{"mise", "flatpak"}},
		{name: "exclude from all", args: []string{"no-mise"}, want: []string{"flatpak", "dnf5"}},
		{name: "all with exclusion", args: []string{"all", "no-mise"}, want: []string{"flatpak", "dnf5"}},
		{name: "include and exclude", args: []string{"mise", "flatpak", "no-mise"}, want: []string{"flatpak"}},
		{name: "dnf alias", args: []string{"dnf"}, want: []string{"dnf5"}},
		{name: "dnf5 alias", args: []string{"dnf5"}, want: []string{"dnf5"}},
		{name: "invalid selector", args: []string{"foo"}, wantErr: "unknown update selector"},
		{name: "invalid no-selector", args: []string{"no-foo"}, wantErr: "unknown update selector"},
		{name: "empty selection", args: []string{"mise", "no-mise"}, wantErr: "no update subsystems selected"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectBackends(tc.args, available)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("unexpected error\nwant contains: %q\ngot: %q", tc.wantErr, err.Error())
				}

				return
			}

			if err != nil {
				t.Fatalf("selectBackends returned error: %v", err)
			}

			gotNames := backendNames(got)
			if !reflect.DeepEqual(gotNames, tc.want) {
				t.Fatalf("unexpected selected backends\nwant: %#v\ngot:  %#v", tc.want, gotNames)
			}
		})
	}
}

func TestSelectBackendsErrorsWhenRequestedBackendUnavailable(t *testing.T) {
	t.Parallel()

	available := []toolBackend{{Name: "mise"}, {Name: "flatpak"}}
	_, err := selectBackends([]string{"dnf"}, available)
	if err == nil {
		t.Fatalf("expected unavailable backend error, got nil")
	}

	if !strings.Contains(err.Error(), "dnf update subsystem is not available") {
		t.Fatalf("unexpected unavailable backend error: %q", err.Error())
	}
}

func TestBackendsWithSkipped(t *testing.T) {
	t.Parallel()

	available := []toolBackend{{Name: "mise"}, {Name: "flatpak"}, {Name: "dnf5"}}
	selected := []toolBackend{{Name: "flatpak"}}

	got := backendsWithSkipped(available, selected)
	if len(got) != 3 {
		t.Fatalf("unexpected backend count: %d", len(got))
	}

	if got[0].Name != "mise" || !got[0].Skipped {
		t.Fatalf("expected mise backend to be marked skipped, got: %#v", got[0])
	}
	if got[1].Name != "flatpak" || got[1].Skipped {
		t.Fatalf("expected flatpak backend to be selected, got: %#v", got[1])
	}
	if got[2].Name != "dnf5" || !got[2].Skipped {
		t.Fatalf("expected dnf5 backend to be marked skipped, got: %#v", got[2])
	}
}

func TestCollectUpdatePlansWithProgressPreservesBackendOrder(t *testing.T) {
	t.Parallel()

	backends := []toolBackend{
		{
			Name: "mise",
			ListOutdated: func() ([]updatePackage, string, error) {
				time.Sleep(50 * time.Millisecond)
				return []updatePackage{{Name: "node"}}, "mise output", nil
			},
		},
		{
			Name: "flatpak",
			ListOutdated: func() ([]updatePackage, string, error) {
				time.Sleep(10 * time.Millisecond)
				return []updatePackage{{Name: "org.mozilla.firefox"}}, "flatpak output", nil
			},
		},
		{
			Name: "dnf5",
			ListOutdated: func() ([]updatePackage, string, error) {
				time.Sleep(20 * time.Millisecond)
				return []updatePackage{{Name: "bash.x86_64"}}, "dnf5 output", nil
			},
		},
	}

	plans := collectUpdatePlansWithProgressForBackends(backends, nil)

	gotOrder := make([]string, 0, len(plans))
	for _, plan := range plans {
		gotOrder = append(gotOrder, plan.ToolName)
	}

	wantOrder := []string{"mise", "flatpak", "dnf5"}
	if !reflect.DeepEqual(gotOrder, wantOrder) {
		t.Fatalf("unexpected plan order\nwant: %#v\ngot:  %#v", wantOrder, gotOrder)
	}
}

func TestFormatCheckingLoaderLine(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		frame string
		tool  string
		want  string
	}{
		{name: "mise", frame: "Ooo", tool: "mise", want: "Ooo [mise] checking..."},
		{name: "flatpak", frame: "oOo", tool: "flatpak", want: "oOo [flatpak] checking..."},
		{name: "dnf5", frame: "ooO", tool: "dnf5", want: "ooO [dnf5] checking..."},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatCheckingLoaderLine(tc.frame, tc.tool); got != tc.want {
				t.Fatalf("unexpected checking line\nwant: %q\ngot:  %q", tc.want, got)
			}
		})
	}
}

func TestFormatDoneLoaderLine(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		tool  string
		count int
		want  string
	}{
		{name: "mise", tool: "mise", count: 1, want: "[mise] 1 found"},
		{name: "flatpak", tool: "flatpak", count: 7, want: "[flatpak] 7 found"},
		{name: "dnf5", tool: "dnf5", count: 0, want: "[dnf5] 0 found"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatDoneLoaderLine(tc.tool, tc.count); got != tc.want {
				t.Fatalf("unexpected done line\nwant: %q\ngot:  %q", tc.want, got)
			}
		})
	}
}

func TestUpdateProgressRendererDoneStateFreezesLine(t *testing.T) {
	t.Parallel()

	renderer := newUpdateProgressRenderer([]toolBackend{{Name: "mise"}}, &bytes.Buffer{}, true)

	if got := renderer.lineForRow(0); got != "Ooo [mise] checking..." {
		t.Fatalf("unexpected initial renderer line: %q", got)
	}

	renderer.AdvanceFrame()
	if got := renderer.lineForRow(0); got != "oOo [mise] checking..." {
		t.Fatalf("unexpected animated renderer line: %q", got)
	}

	renderer.MarkDone(0, 2)
	if got := renderer.lineForRow(0); got != "[mise] 2 found" {
		t.Fatalf("unexpected done renderer line: %q", got)
	}

	renderer.AdvanceFrame()
	if got := renderer.lineForRow(0); got != "[mise] 2 found" {
		t.Fatalf("done line should stay frozen, got: %q", got)
	}
}

func TestUpdateProgressRendererDefersThirdFrameUntilDelay(t *testing.T) {
	t.Parallel()

	renderer := newUpdateProgressRenderer([]toolBackend{{Name: "mise"}}, &bytes.Buffer{}, true)
	now := time.Now()
	renderer.nowFunc = func() time.Time { return now }
	renderer.rows[0].startedAt = now.Add(-300 * time.Millisecond)
	renderer.frameIndex = 2

	if got := renderer.lineForRow(0); got != "oOo [mise] checking..." {
		t.Fatalf("expected third frame to be deferred before delay, got: %q", got)
	}

	renderer.rows[0].startedAt = now.Add(-700 * time.Millisecond)
	if got := renderer.lineForRow(0); got != "ooO [mise] checking..." {
		t.Fatalf("expected third frame to appear after delay, got: %q", got)
	}
}

func TestUpdateProgressRendererNonInteractiveFallback(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderer := newUpdateProgressRenderer([]toolBackend{{Name: "mise"}, {Name: "flatpak"}}, &output, false)

	renderer.Start()
	renderer.AdvanceFrame()
	renderer.MarkDone(1, 3)
	renderer.AdvanceFrame()
	renderer.MarkDone(0, 1)

	got := output.String()
	want := strings.Join([]string{
		"Ooo [mise] checking...",
		"Ooo [flatpak] checking...",
		"[flatpak] 3 found",
		"[mise] 1 found",
	}, "\n") + "\n"

	if got != want {
		t.Fatalf("unexpected non-interactive renderer output\nwant: %q\ngot:  %q", want, got)
	}

	if strings.Contains(got, "\x1b[") {
		t.Fatalf("expected fallback mode to avoid ANSI control sequences, got: %q", got)
	}
}

func TestUpdateProgressRendererShowsSkippedRows(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderer := newUpdateProgressRenderer(
		[]toolBackend{{Name: "mise", Skipped: true}, {Name: "flatpak"}},
		&output,
		false,
	)

	renderer.Start()
	renderer.AdvanceFrame()
	renderer.MarkDone(1, 2)

	got := output.String()
	want := strings.Join([]string{
		"[mise] skipped",
		"Ooo [flatpak] checking...",
		"[flatpak] 2 found",
	}, "\n") + "\n"

	if got != want {
		t.Fatalf("unexpected skipped renderer output\nwant: %q\ngot:  %q", want, got)
	}
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

func TestWrappedToolSummaryLinesWrapsNames(t *testing.T) {
	t.Parallel()

	got := wrappedToolSummaryLines("dnf5", []string{"alpha", "beta", "gamma", "delta"}, 20)
	want := []string{
		"dnf5: alpha beta",
		"      gamma delta",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected wrapped lines:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestWrappedToolSummaryLinesWrapsLongName(t *testing.T) {
	t.Parallel()

	got := wrappedToolSummaryLines("dnf5", []string{"abcdefghijkl"}, 12)
	want := []string{
		"dnf5: abcdef",
		"      ghijkl",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected wrapped long-name lines:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestWrappedToolSummaryLinesNone(t *testing.T) {
	t.Parallel()

	got := wrappedToolSummaryLines("flatpak", nil, 20)
	want := []string{"flatpak: (none)"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected none lines:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestSummaryContentWidth(t *testing.T) {
	original := terminalColumnsFunc
	terminalColumnsFunc = func() int { return 120 }
	t.Cleanup(func() {
		terminalColumnsFunc = original
	})

	if got := summaryContentWidth(); got != 116 {
		t.Fatalf("unexpected summary width for wide terminal: %d", got)
	}
}

func TestSummaryContentWidthFallbackWhenInvalid(t *testing.T) {
	original := terminalColumnsFunc
	terminalColumnsFunc = func() int { return 0 }
	t.Cleanup(func() {
		terminalColumnsFunc = original
	})

	if got := summaryContentWidth(); got != 76 {
		t.Fatalf("unexpected fallback summary width: %d", got)
	}
}

func TestOverallSummaryDisplayLinesRespectWidth(t *testing.T) {
	t.Parallel()

	lines := overallSummaryDisplayLines([]toolUpdateSummary{
		{
			ToolName:     "dnf5",
			UpdatedNames: []string{"alpha", "beta", "gamma", "delta", "epsilon"},
		},
	}, 5, 0, 20)

	for _, line := range lines {
		if visibleLen(line) > 20 {
			t.Fatalf("line exceeds configured width (%d): %q", visibleLen(line), line)
		}
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
