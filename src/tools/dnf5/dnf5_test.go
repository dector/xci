package dnf5

import (
	"os/exec"
	"reflect"
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

func runExitCodeCommand(t *testing.T, code int) error {
	t.Helper()

	command := "exit 1"
	if code == 100 {
		command = "exit 100"
	}

	cmd := exec.Command("sh", "-c", command)

	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected non-zero exit code")
	}

	return err
}
