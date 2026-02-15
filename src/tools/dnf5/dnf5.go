package dnf5

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type OutdatedPackage struct {
	Name       string
	Arch       string
	Latest     string
	Repository string
}

type PackageUpdateResult struct {
	Package OutdatedPackage
	Success bool
	Output  string
	Reason  string
}

// ListOutdated returns packages with available upgrades from dnf5.
func ListOutdated() ([]OutdatedPackage, string, error) {
	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command("dnf5", "check-upgrade", "--json")
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())
	combinedOutput := combineOutput(stdout, stderr)

	if !isAllowedCheckUpgradeExit(err) {
		if strings.TrimSpace(combinedOutput) == "" {
			return nil, combinedOutput, fmt.Errorf("failed to list outdated dnf5 packages: %w", err)
		}

		return nil, combinedOutput, fmt.Errorf("failed to list outdated dnf5 packages: %w: %s", err, combinedOutput)
	}

	packages, err := parseCheckUpgradeJSON(stdout)
	if err != nil {
		return nil, combinedOutput, fmt.Errorf("failed to parse outdated dnf5 packages: %w", err)
	}

	return packages, combinedOutput, nil
}

// UpdatePackages upgrades all packages in a single dnf5 transaction.
func UpdatePackages(packages []OutdatedPackage) []PackageUpdateResult {
	if len(packages) == 0 {
		return nil
	}

	args := []string{"dnf5", "upgrade", "-y"}
	for _, pkg := range packages {
		args = append(args, packageSpec(pkg.Name, pkg.Arch))
	}

	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command("sudo", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, &outBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &errBuf)

	err := cmd.Run()
	output := combineOutput(outBuf.String(), errBuf.String())

	results := make([]PackageUpdateResult, 0, len(packages))
	if err != nil {
		reason := buildFailureReason(err, output)
		for _, pkg := range packages {
			results = append(results, PackageUpdateResult{
				Package: pkg,
				Success: false,
				Output:  output,
				Reason:  reason,
			})
		}

		return results
	}

	for _, pkg := range packages {
		results = append(results, PackageUpdateResult{
			Package: pkg,
			Success: true,
			Output:  output,
		})
	}

	return results
}

func parseCheckUpgradeJSON(output string) ([]OutdatedPackage, error) {
	output = strings.TrimSpace(output)
	if output == "" || output == "{}" || output == "null" {
		return nil, nil
	}

	raw := map[string][]struct {
		Name       string `json:"name"`
		Arch       string `json:"arch"`
		EVR        string `json:"evr"`
		Repository string `json:"repository"`
	}{}

	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, err
	}

	flattened := make([]OutdatedPackage, 0)
	for _, section := range raw {
		for _, item := range section {
			flattened = append(flattened, OutdatedPackage{
				Name:       item.Name,
				Arch:       item.Arch,
				Latest:     item.EVR,
				Repository: item.Repository,
			})
		}
	}

	sort.Slice(flattened, func(i, j int) bool {
		specI := packageSpec(flattened[i].Name, flattened[i].Arch)
		specJ := packageSpec(flattened[j].Name, flattened[j].Arch)
		if specI != specJ {
			return specI < specJ
		}
		if flattened[i].Latest != flattened[j].Latest {
			return flattened[i].Latest < flattened[j].Latest
		}

		return flattened[i].Repository < flattened[j].Repository
	})

	seen := make(map[string]struct{}, len(flattened))
	packages := make([]OutdatedPackage, 0, len(flattened))
	for _, pkg := range flattened {
		spec := packageSpec(pkg.Name, pkg.Arch)
		if _, ok := seen[spec]; ok {
			continue
		}

		seen[spec] = struct{}{}
		packages = append(packages, pkg)
	}

	return packages, nil
}

func checkUpgradeExitCode(err error) (int, bool) {
	if err == nil {
		return 0, true
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0, false
	}

	return exitErr.ExitCode(), true
}

func isAllowedCheckUpgradeExit(err error) bool {
	code, ok := checkUpgradeExitCode(err)
	if !ok {
		return false
	}

	return code == 0 || code == 100
}

func packageSpec(name, arch string) string {
	name = strings.TrimSpace(name)
	arch = strings.TrimSpace(arch)
	if arch == "" {
		return name
	}

	return fmt.Sprintf("%s.%s", name, arch)
}

func combineOutput(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		nonEmpty = append(nonEmpty, trimmed)
	}

	return strings.Join(nonEmpty, "\n")
}

func buildFailureReason(err error, output string) string {
	line := lastNonEmptyLine(output)
	if line == "" {
		return err.Error()
	}

	return fmt.Sprintf("%s (%v)", line, err)
}

func lastNonEmptyLine(text string) string {
	lines := strings.Split(text, "\n")
	for idx := len(lines) - 1; idx >= 0; idx-- {
		line := strings.TrimSpace(lines[idx])
		if line != "" {
			return line
		}
	}

	return ""
}
