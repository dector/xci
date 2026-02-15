package dnf5

import (
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func TestParseCheckUpgradeJSON(t *testing.T) {
	t.Parallel()

	input := `{
		"upgrades": [
			{"name": "zlib", "arch": "x86_64", "evr": "1.3-1.fc41", "repository": "updates"},
			{"name": "bash", "arch": "x86_64", "evr": "5.2-8.fc41", "repository": "updates"}
		],
		"security": [
			{"name": "bash", "arch": "x86_64", "evr": "5.2-8.fc41", "repository": "updates"}
		]
	}`

	got, err := parseCheckUpgradeJSON(input)
	if err != nil {
		t.Fatalf("parseCheckUpgradeJSON returned error: %v", err)
	}

	want := []OutdatedPackage{
		{Name: "bash", Arch: "x86_64", Latest: "5.2-8.fc41", Repository: "updates"},
		{Name: "zlib", Arch: "x86_64", Latest: "1.3-1.fc41", Repository: "updates"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected packages\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestParseCheckUpgradeJSONEmpty(t *testing.T) {
	t.Parallel()

	got, err := parseCheckUpgradeJSON("{}")
	if err != nil {
		t.Fatalf("parseCheckUpgradeJSON returned error: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected no packages, got %d", len(got))
	}
}

func TestCheckUpgradeExitCode(t *testing.T) {
	t.Parallel()

	code, ok := checkUpgradeExitCode(nil)
	if !ok || code != 0 {
		t.Fatalf("unexpected result for nil error: ok=%v code=%d", ok, code)
	}

	exitErr := runExitCodeCommand(t, 100)
	code, ok = checkUpgradeExitCode(exitErr)
	if !ok || code != 100 {
		t.Fatalf("unexpected exit code: ok=%v code=%d", ok, code)
	}

	if !isAllowedCheckUpgradeExit(exitErr) {
		t.Fatalf("expected exit code 100 to be allowed")
	}

	exitErr = runExitCodeCommand(t, 1)
	if isAllowedCheckUpgradeExit(exitErr) {
		t.Fatalf("expected exit code 1 to be disallowed")
	}
}

func TestParseCheckUpgradeText(t *testing.T) {
	t.Parallel()

	input := "Last metadata expiration check: 0:00:10 ago on Thu 01 Jan 1970.\n" +
		"bash.x86_64 5.2-8.fc41 updates\n" +
		"zlib.i686 1.3-1.fc41 updates\n" +
		"bash.x86_64 5.2-8.fc41 updates\n"

	got, err := parseCheckUpgradeText(input)
	if err != nil {
		t.Fatalf("parseCheckUpgradeText returned error: %v", err)
	}

	want := []OutdatedPackage{
		{Name: "bash", Arch: "x86_64", Latest: "5.2-8.fc41", Repository: "updates"},
		{Name: "zlib", Arch: "i686", Latest: "1.3-1.fc41", Repository: "updates"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected packages\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestUnsupportedCheckUpgradeJSONError(t *testing.T) {
	t.Parallel()

	err := runExitCodeCommand(t, 2)
	output := `Unknown argument "--json" for command "check-upgrade"`
	if !isUnsupportedCheckUpgradeJSONError(err, output) {
		t.Fatalf("expected unsupported json error to be detected")
	}
}

func TestParseInstalledVersionOutput(t *testing.T) {
	t.Parallel()

	input := "bash.x86_64|0|5.2-8.fc41\n" +
		"kernel.x86_64|1|6.8.12-100.fc41\n" +
		"zlib.i686|(none)|1.3-1.fc41\n" +
		"invalid-line"

	got := parseInstalledVersionOutput(input)
	want := map[string]string{
		"bash.x86_64":   "5.2-8.fc41",
		"kernel.x86_64": "1:6.8.12-100.fc41",
		"zlib.i686":     "1.3-1.fc41",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected versions\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestApplyInstalledVersions(t *testing.T) {
	t.Parallel()

	packages := []OutdatedPackage{
		{Name: "bash", Arch: "x86_64", Latest: "5.2-8.fc41"},
		{Name: "zlib", Arch: "i686", Latest: "1.3-1.fc41"},
		{Name: "vim", Arch: "x86_64", Latest: "9.1-2.fc41"},
	}

	applyInstalledVersions(packages, map[string]string{
		"bash.x86_64": "5.2-7.fc41",
		"zlib.i686":   "1.2.13-4.fc41",
	})

	if packages[0].Current != "5.2-7.fc41" {
		t.Fatalf("unexpected current version for bash: %q", packages[0].Current)
	}
	if packages[1].Current != "1.2.13-4.fc41" {
		t.Fatalf("unexpected current version for zlib: %q", packages[1].Current)
	}
	if packages[2].Current != "" {
		t.Fatalf("expected empty current version for vim, got: %q", packages[2].Current)
	}
}

func TestBuildVerifiedUpdateResultsMarksStillOutdatedAsFailure(t *testing.T) {
	t.Parallel()

	requested := []OutdatedPackage{
		{Name: "bash", Arch: "x86_64", Latest: "5.2-8.fc41"},
		{Name: "zlib", Arch: "x86_64", Latest: "1.3-1.fc41"},
	}

	results := buildVerifiedUpdateResults(
		requested,
		"upgrade output",
		func() ([]OutdatedPackage, string, error) {
			return []OutdatedPackage{{Name: "bash", Arch: "x86_64", Latest: "5.2-8.fc41"}}, "check output", nil
		},
	)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Success {
		t.Fatalf("expected bash to be marked failed")
	}
	if !strings.Contains(results[0].Reason, "still reported as outdated") {
		t.Fatalf("expected stale-package reason, got: %q", results[0].Reason)
	}
	if !strings.Contains(results[0].Output, "post-upgrade check") {
		t.Fatalf("expected post-upgrade check output to be included, got: %q", results[0].Output)
	}

	if !results[1].Success {
		t.Fatalf("expected zlib to remain successful")
	}
}

func TestBuildVerifiedUpdateResultsKeepsSuccessWhenCheckFails(t *testing.T) {
	t.Parallel()

	requested := []OutdatedPackage{{Name: "bash", Arch: "x86_64", Latest: "5.2-8.fc41"}}
	results := buildVerifiedUpdateResults(
		requested,
		"upgrade output",
		func() ([]OutdatedPackage, string, error) {
			return nil, "dnf check output", errors.New("boom")
		},
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("expected success when post-upgrade check fails")
	}
	if !strings.Contains(results[0].Output, "post-upgrade check failed") {
		t.Fatalf("expected check failure note in output, got: %q", results[0].Output)
	}
}

func TestBuildVerifiedUpdateResultsTreatsMatchingCurrentAsSuccess(t *testing.T) {
	t.Parallel()

	requested := []OutdatedPackage{{Name: "cosmic-app-library", Arch: "x86_64", Latest: "1.0.6-1.fc43"}}
	results := buildVerifiedUpdateResults(
		requested,
		"upgrade output",
		func() ([]OutdatedPackage, string, error) {
			return []OutdatedPackage{{
				Name:    "cosmic-app-library",
				Arch:    "x86_64",
				Current: "1.0.6-1.fc43",
				Latest:  "1.0.6-1.fc43",
			}}, "check output", nil
		},
	)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("expected matching current/latest to be treated as success, got failure: %q", results[0].Reason)
	}
}

func TestVersionsMatch(t *testing.T) {
	t.Parallel()

	if !versionsMatch("1.0.6-1.fc43", "1.0.6-1.fc43") {
		t.Fatalf("expected exact versions to match")
	}
	if !versionsMatch("0:1.0.6-1.fc43", "1.0.6-1.fc43") {
		t.Fatalf("expected zero-epoch prefix to be ignored")
	}
	if versionsMatch("1.0.5-1.fc43", "1.0.6-1.fc43") {
		t.Fatalf("expected different versions not to match")
	}
}

func TestFormatPostUpgradeCheckOutput(t *testing.T) {
	t.Parallel()

	got := formatPostUpgradeCheckOutput("some output", nil)
	if got != "post-upgrade check:\nsome output" {
		t.Fatalf("unexpected formatted post-upgrade output: %q", got)
	}

	got = formatPostUpgradeCheckOutput("", errors.New("boom"))
	if got != "post-upgrade check failed: boom" {
		t.Fatalf("unexpected formatted check failure output: %q", got)
	}
}

func TestUpdateCommandArgs(t *testing.T) {
	t.Parallel()

	got := updateCommandArgs([]OutdatedPackage{
		{Name: "bash", Arch: "x86_64"},
		{Name: "", Arch: "x86_64"},
		{Name: "zlib", Arch: "i686"},
		{Name: "bash", Arch: "x86_64"},
	})

	want := []string{"dnf5", "upgrade", "--refresh", "--best", "-y", "bash.x86_64", "zlib.i686"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected update command args\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestCheckUpgradeCommandArgs(t *testing.T) {
	t.Parallel()

	got := checkUpgradeCommandArgs("--json")
	want := []string{"check-upgrade", "--refresh", "--json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected check-upgrade command args\nwant: %#v\ngot:  %#v", want, got)
	}
}

func runExitCodeCommand(t *testing.T, code int) error {
	t.Helper()

	command := fmt.Sprintf("exit %d", code)
	cmd := exec.Command("sh", "-c", command)

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit code")
	}

	return err
}
