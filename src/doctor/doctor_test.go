package doctor

import (
	"errors"
	"io"
	"os/exec"
	"testing"
	"xci/internal/utils"
)

func TestCheckDNF5Installed(t *testing.T) {
	setDoctorDeps(t,
		func(name string) (string, error) { return "/usr/bin/" + name, nil },
		func() utils.DistroFamily { return utils.DistroFamilyFedoraLike },
	)

	got := checkDNF5()
	if got.Status != statusOK {
		t.Fatalf("unexpected status\nwant: %q\ngot:  %q", statusOK, got.Status)
	}
}

func TestCheckDNF5MissingFedoraLikeIsError(t *testing.T) {
	setDoctorDeps(t,
		func(name string) (string, error) { return "", exec.ErrNotFound },
		func() utils.DistroFamily { return utils.DistroFamilyFedoraLike },
	)

	got := checkDNF5()
	if got.Status != statusError {
		t.Fatalf("unexpected status\nwant: %q\ngot:  %q", statusError, got.Status)
	}
}

func TestCheckDNF5MissingNonFedoraLikeIsWarning(t *testing.T) {
	setDoctorDeps(t,
		func(name string) (string, error) { return "", exec.ErrNotFound },
		func() utils.DistroFamily { return utils.DistroFamilyNonFedoraLike },
	)

	got := checkDNF5()
	if got.Status != statusWarning {
		t.Fatalf("unexpected status\nwant: %q\ngot:  %q", statusWarning, got.Status)
	}
}

func TestCheckDNF5LookupErrorFedoraLikeIsError(t *testing.T) {
	setDoctorDeps(t,
		func(name string) (string, error) { return "", errors.New("boom") },
		func() utils.DistroFamily { return utils.DistroFamilyFedoraLike },
	)

	got := checkDNF5()
	if got.Status != statusError {
		t.Fatalf("unexpected status\nwant: %q\ngot:  %q", statusError, got.Status)
	}
}

func TestCheckDNF5LookupErrorUnknownIsWarning(t *testing.T) {
	setDoctorDeps(t,
		func(name string) (string, error) { return "", errors.New("boom") },
		func() utils.DistroFamily { return utils.DistroFamilyUnknown },
	)

	got := checkDNF5()
	if got.Status != statusWarning {
		t.Fatalf("unexpected status\nwant: %q\ngot:  %q", statusWarning, got.Status)
	}
}

func TestCheckDNF5LookupErrorNonFedoraLikeIsWarning(t *testing.T) {
	setDoctorDeps(t,
		func(name string) (string, error) { return "", errors.New("boom") },
		func() utils.DistroFamily { return utils.DistroFamilyNonFedoraLike },
	)

	got := checkDNF5()
	if got.Status != statusWarning {
		t.Fatalf("unexpected status\nwant: %q\ngot:  %q", statusWarning, got.Status)
	}
}

func TestRunChecksWarningsOnlyPass(t *testing.T) {
	originalOutWriter := outWriter
	outWriter = io.Discard
	t.Cleanup(func() {
		outWriter = originalOutWriter
	})

	checks := []func() DoctorResult{
		func() DoctorResult {
			return DoctorResult{Name: "dnf5", Status: statusWarning, Message: "warn"}
		},
		func() DoctorResult {
			return DoctorResult{Name: "tool", Status: statusOK, Message: "ok"}
		},
	}

	if !runChecks(checks) {
		t.Fatalf("expected warnings-only run to pass")
	}
}

func setDoctorDeps(t *testing.T, lookPath func(string) (string, error), detectFamily func() utils.DistroFamily) {
	t.Helper()

	originalLookPath := lookPathFunc
	originalDetect := detectDistroFamilyFunc
	originalOutWriter := outWriter

	lookPathFunc = lookPath
	detectDistroFamilyFunc = detectFamily
	outWriter = io.Discard

	t.Cleanup(func() {
		lookPathFunc = originalLookPath
		detectDistroFamilyFunc = originalDetect
		outWriter = originalOutWriter
	})
}
