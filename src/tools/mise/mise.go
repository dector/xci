package mise

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type OutdatedPackage struct {
	Name      string
	Requested string
	Current   string
	Latest    string
}

type PackageUpdateResult struct {
	Package OutdatedPackage
	Success bool
	Output  string
	Reason  string
}

// Install runs 'mise use -g' with the provided arguments
func Install(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no packages specified")
	}

	fmt.Println("Installing:", args)

	// Build the mise command: mise use -g <args...>
	miseArgs := append([]string{"use", "-g"}, args...)
	miseCmd := exec.Command("mise", miseArgs...)

	// Connect stdio to show mise output
	miseCmd.Stdout = os.Stdout
	miseCmd.Stderr = os.Stderr
	miseCmd.Stdin = os.Stdin

	// Execute the command
	return miseCmd.Run()
}

// ListOutdated returns packages that have updates available.
func ListOutdated() ([]OutdatedPackage, string, error) {
	var outBuf, errBuf bytes.Buffer
	cmd := exec.Command("mise", "outdated", "--json", "--quiet")
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout := strings.TrimSpace(outBuf.String())
	stderr := strings.TrimSpace(errBuf.String())
	combinedOutput := combineOutput(stdout, stderr)
	if err != nil {
		return nil, combinedOutput, fmt.Errorf("failed to list outdated tools: %w", err)
	}

	packages, err := parseOutdatedJSON(stdout)
	if err != nil {
		return nil, combinedOutput, fmt.Errorf("failed to parse outdated tools: %w", err)
	}

	return packages, combinedOutput, nil
}

// UpdatePackages updates one package at a time and returns per-package results.
func UpdatePackages(packages []OutdatedPackage) []PackageUpdateResult {
	results := make([]PackageUpdateResult, 0, len(packages))

	for _, pkg := range packages {
		var outBuf, errBuf bytes.Buffer
		cmd := exec.Command("mise", "up", pkg.Name)
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

func parseOutdatedJSON(output string) ([]OutdatedPackage, error) {
	output = strings.TrimSpace(output)
	if output == "" || output == "null" {
		return nil, nil
	}
	if strings.Contains(output, "All tools are up to date") || strings.Contains(output, "All tools are up-to-date") {
		return nil, nil
	}

	raw := map[string]struct {
		Requested string `json:"requested"`
		Current   string `json:"current"`
		Latest    string `json:"latest"`
	}{}

	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, err
	}

	packages := make([]OutdatedPackage, 0, len(raw))
	for name, item := range raw {
		packages = append(packages, OutdatedPackage{
			Name:      name,
			Requested: item.Requested,
			Current:   item.Current,
			Latest:    item.Latest,
		})
	}

	sort.Slice(packages, func(i, j int) bool {
		return packages[i].Name < packages[j].Name
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
