package update

import (
	"errors"
	"os/exec"
	"testing"
	"xci/internal/utils"
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
