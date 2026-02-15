package dnf5

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
)

type OutdatedPackage struct {
	Name       string
	Arch       string
	Current    string
	Latest     string
	Repository string
}

type PackageUpdateResult struct {
	Package OutdatedPackage
	Success bool
	Output  string
	Reason  string
}

var ansiColorPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// ListOutdated returns packages with available upgrades from dnf5.
func ListOutdated() ([]OutdatedPackage, string, error) {
	stdout, stderr, err := runCheckUpgrade("--json")
	combinedOutput := combineOutput(stdout, stderr)
	if isAllowedCheckUpgradeExit(err) {
		packages, parseErr := parseCheckUpgradeJSON(stdout)
		if parseErr != nil {
			return nil, combinedOutput, fmt.Errorf("failed to parse outdated dnf5 packages: %w", parseErr)
		}

		applyInstalledVersions(packages, installedVersionsByPackageSpec(packages))

		return packages, combinedOutput, nil
	}

	if !isUnsupportedCheckUpgradeJSONError(err, combinedOutput) {
		return nil, combinedOutput, buildListOutdatedError(err, combinedOutput)
	}

	stdout, stderr, err = runCheckUpgrade()
	combinedOutput = combineOutput(stdout, stderr)
	if !isAllowedCheckUpgradeExit(err) {
		return nil, combinedOutput, buildListOutdatedError(err, combinedOutput)
	}

	packages, parseErr := parseCheckUpgradeText(combinedOutput)
	if parseErr != nil {
		return nil, combinedOutput, fmt.Errorf("failed to parse outdated dnf5 packages: %w", parseErr)
	}

	applyInstalledVersions(packages, installedVersionsByPackageSpec(packages))

	return packages, combinedOutput, nil
}

// UpdatePackages upgrades the selected outdated package specs.
func UpdatePackages(packages []OutdatedPackage) []PackageUpdateResult {
	if len(packages) == 0 {
		return nil
	}

	args := updateCommandArgs(packages)
	if len(args) == 5 {
		return nil
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

	return buildVerifiedUpdateResults(packages, output, ListOutdated)
}

func updateCommandArgs(packages []OutdatedPackage) []string {
	args := []string{"dnf5", "upgrade", "--refresh", "--best", "-y"}
	seen := make(map[string]struct{}, len(packages))

	for _, pkg := range packages {
		spec := packageSpec(pkg.Name, pkg.Arch)
		if spec == "" {
			continue
		}
		if _, ok := seen[spec]; ok {
			continue
		}

		seen[spec] = struct{}{}
		args = append(args, spec)
	}

	// fmt.Printf("%+s\n", args)

	return args
}

func buildVerifiedUpdateResults(
	packages []OutdatedPackage,
	upgradeOutput string,
	listOutdated func() ([]OutdatedPackage, string, error),
) []PackageUpdateResult {
	if listOutdated == nil {
		return buildSuccessfulUpdateResults(packages, upgradeOutput)
	}

	outdated, checkOutput, err := listOutdated()
	combinedOutput := combineOutput(upgradeOutput, formatPostUpgradeCheckOutput(checkOutput, err))
	if err != nil {
		return buildSuccessfulUpdateResults(packages, combinedOutput)
	}

	outdatedBySpec := make(map[string]OutdatedPackage, len(outdated))
	for _, pkg := range outdated {
		spec := packageSpec(pkg.Name, pkg.Arch)
		if spec == "" {
			continue
		}

		outdatedBySpec[spec] = pkg
	}

	results := make([]PackageUpdateResult, 0, len(packages))
	for _, pkg := range packages {
		spec := packageSpec(pkg.Name, pkg.Arch)
		outdatedPkg, stillOutdated := outdatedBySpec[spec]
		if !stillOutdated {
			results = append(results, PackageUpdateResult{
				Package: pkg,
				Success: true,
				Output:  combinedOutput,
			})
			continue
		}

		if versionsMatch(outdatedPkg.Current, outdatedPkg.Latest) {
			results = append(results, PackageUpdateResult{
				Package: pkg,
				Success: true,
				Output:  combinedOutput,
			})
			continue
		}

		reason := "still reported as outdated after running dnf5 upgrade"
		if latest := strings.TrimSpace(outdatedPkg.Latest); latest != "" {
			reason = fmt.Sprintf("still reported as outdated after running dnf5 upgrade (latest: %s)", latest)
		}

		results = append(results, PackageUpdateResult{
			Package: pkg,
			Success: false,
			Output:  combinedOutput,
			Reason:  reason,
		})
	}

	return results
}

func versionsMatch(current, latest string) bool {
	current = normalizeVersion(current)
	latest = normalizeVersion(latest)
	if current == "" || latest == "" {
		return false
	}

	return current == latest
}

func normalizeVersion(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimPrefix(value, "0:")
}

func buildSuccessfulUpdateResults(packages []OutdatedPackage, output string) []PackageUpdateResult {
	results := make([]PackageUpdateResult, 0, len(packages))
	for _, pkg := range packages {
		results = append(results, PackageUpdateResult{
			Package: pkg,
			Success: true,
			Output:  output,
		})
	}

	return results
}

func formatPostUpgradeCheckOutput(output string, err error) string {
	output = strings.TrimSpace(output)
	if err != nil {
		if output == "" {
			return fmt.Sprintf("post-upgrade check failed: %v", err)
		}

		return fmt.Sprintf("post-upgrade check failed: %v\n%s", err, output)
	}

	if output == "" {
		return ""
	}

	return fmt.Sprintf("post-upgrade check:\n%s", output)
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

func parseCheckUpgradeText(output string) ([]OutdatedPackage, error) {
	output = stripANSIColors(output)
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	flattened := make([]OutdatedPackage, 0)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		name, arch, ok := splitPackageSpec(fields[0])
		if !ok {
			continue
		}

		flattened = append(flattened, OutdatedPackage{
			Name:       name,
			Arch:       arch,
			Latest:     fields[1],
			Repository: fields[2],
		})
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

func installedVersionsByPackageSpec(packages []OutdatedPackage) map[string]string {
	specs := uniquePackageSpecs(packages)
	if len(specs) == 0 {
		return nil
	}

	stdout, _, err := queryInstalledPackageVersions(specs)
	versions := parseInstalledVersionOutput(stdout)
	if err != nil && len(versions) == 0 {
		return nil
	}

	return versions
}

func uniquePackageSpecs(packages []OutdatedPackage) []string {
	seen := make(map[string]struct{}, len(packages))
	specs := make([]string, 0, len(packages))

	for _, pkg := range packages {
		spec := packageSpec(pkg.Name, pkg.Arch)
		if spec == "" {
			continue
		}

		if _, ok := seen[spec]; ok {
			continue
		}

		seen[spec] = struct{}{}
		specs = append(specs, spec)
	}

	sort.Strings(specs)
	return specs
}

func queryInstalledPackageVersions(specs []string) (string, string, error) {
	args := []string{"-q", "--queryformat", `%{NAME}.%{ARCH}|%{EPOCHNUM}|%{VERSION}-%{RELEASE}\n`}
	args = append(args, specs...)

	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command("rpm", args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())

	return stdout, stderr, err
}

func parseInstalledVersionOutput(output string) map[string]string {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil
	}

	versions := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}

		spec := strings.TrimSpace(parts[0])
		epoch := strings.TrimSpace(parts[1])
		version := strings.TrimSpace(parts[2])
		if spec == "" || version == "" {
			continue
		}

		if epoch != "" && epoch != "0" && !strings.EqualFold(epoch, "(none)") {
			version = fmt.Sprintf("%s:%s", epoch, version)
		}

		versions[spec] = version
	}

	if len(versions) == 0 {
		return nil
	}

	return versions
}

func applyInstalledVersions(packages []OutdatedPackage, versions map[string]string) {
	if len(packages) == 0 || len(versions) == 0 {
		return
	}

	for idx := range packages {
		spec := packageSpec(packages[idx].Name, packages[idx].Arch)
		if current, ok := versions[spec]; ok {
			packages[idx].Current = current
		}
	}
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

func isUnsupportedCheckUpgradeJSONError(err error, output string) bool {
	code, ok := checkUpgradeExitCode(err)
	if !ok || code != 2 {
		return false
	}

	normalized := strings.ToLower(strings.TrimSpace(output))
	return strings.Contains(normalized, `unknown argument "--json"`) && strings.Contains(normalized, "check-upgrade")
}

func buildListOutdatedError(err error, output string) error {
	if strings.TrimSpace(output) == "" {
		return fmt.Errorf("failed to list outdated dnf5 packages: %w", err)
	}

	return fmt.Errorf("failed to list outdated dnf5 packages: %w: %s", err, output)
}

func runCheckUpgrade(args ...string) (string, string, error) {
	cmdArgs := checkUpgradeCommandArgs(args...)

	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command("dnf5", cmdArgs...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())

	return stdout, stderr, err
}

func checkUpgradeCommandArgs(args ...string) []string {
	cmdArgs := []string{"check-upgrade", "--refresh"}
	cmdArgs = append(cmdArgs, args...)
	return cmdArgs
}

func stripANSIColors(text string) string {
	return ansiColorPattern.ReplaceAllString(text, "")
}

func splitPackageSpec(spec string) (string, string, bool) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", "", false
	}

	idx := strings.LastIndex(spec, ".")
	if idx <= 0 || idx == len(spec)-1 {
		return "", "", false
	}

	return spec[:idx], spec[idx+1:], true
}

func packageSpec(name, arch string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

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
