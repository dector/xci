package flatpak

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

type Scope string

const (
	ScopeUser   Scope = "user"
	ScopeSystem Scope = "system"
)

type Options struct {
	Scope Scope
}

type OutdatedPackage struct {
	Ref string
}

type PackageUpdateResult struct {
	Package OutdatedPackage
	Success bool
	Output  string
	Reason  string
}

// ListOutdated returns installed Flatpak refs that have updates available.
func ListOutdated(opts Options) ([]OutdatedPackage, string, error) {
	args := []string{"remote-ls", "--updates", "--columns=ref"}
	args = append(args, scopeArg(opts.Scope))

	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command("flatpak", args...)
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())
	combinedOutput := combineOutput(stdout, stderr)
	if err != nil {
		return nil, combinedOutput, fmt.Errorf("failed to list outdated flatpaks: %w", err)
	}

	packages, err := parseOutdatedRefs(stdout)
	if err != nil {
		return nil, combinedOutput, fmt.Errorf("failed to parse outdated flatpaks: %w", err)
	}

	return packages, combinedOutput, nil
}

// UpdatePackages updates one flatpak ref at a time and returns per-ref results.
func UpdatePackages(opts Options, packages []OutdatedPackage) []PackageUpdateResult {
	results := make([]PackageUpdateResult, 0, len(packages))

	for _, pkg := range packages {
		args := []string{"update", "--assumeyes", "--noninteractive"}
		args = append(args, scopeArg(opts.Scope))
		args = append(args, pkg.Ref)

		var outBuf, errBuf bytes.Buffer
		cmd := exec.Command("flatpak", args...)
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf

		err := cmd.Run()
		stdout := strings.TrimSpace(outBuf.String())
		stderr := strings.TrimSpace(errBuf.String())
		output := combineOutput(stdout, stderr)

		result := PackageUpdateResult{
			Package: pkg,
			Success: err == nil,
			Output:  output,
		}

		if err != nil {
			result.Reason = buildFailureReason(err, output)
		}

		results = append(results, result)
	}

	return results
}

func scopeArg(scope Scope) string {
	switch scope {
	case ScopeSystem:
		return "--system"
	default:
		return "--user"
	}
}

func parseOutdatedRefs(output string) ([]OutdatedPackage, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	seen := map[string]struct{}{}
	packages := make([]OutdatedPackage, 0)

	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.EqualFold(line, "ref") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		ref := strings.TrimSpace(fields[0])
		if ref == "" {
			continue
		}

		if _, ok := seen[ref]; ok {
			continue
		}

		seen[ref] = struct{}{}
		packages = append(packages, OutdatedPackage{Ref: ref})
	}

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Ref < packages[j].Ref
	})

	return packages, nil
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
