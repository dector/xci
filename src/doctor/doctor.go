package doctor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"xci/internal/utils"
)

type DoctorResult struct {
	Name    string
	Status  doctorStatus
	Message string
}

type doctorStatus string

const (
	statusOK      doctorStatus = "ok"
	statusWarning doctorStatus = "warning"
	statusError   doctorStatus = "error"

	ansiReset  = "\033[0m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiRed    = "\033[31m"
)

var (
	lookPathFunc                     = exec.LookPath
	detectDistroFamilyFunc           = utils.DetectDistroFamily
	outWriter              io.Writer = os.Stdout
	colorOutputEnabled               = supportsColorOutput()
)

func Run() bool {
	checks := []func() DoctorResult{
		checkMise,
		checkFlatpak,
		checkDNF5,
	}

	return runChecks(checks)
}

func runChecks(checks []func() DoctorResult) bool {
	hasErrors := false
	for _, check := range checks {
		result := check()
		switch result.Status {
		case statusOK:
			fmt.Fprintln(outWriter, colorize("✓ "+result.Message, ansiGreen))
		case statusWarning:
			fmt.Fprintln(outWriter, colorize("! "+result.Message, ansiYellow))
		case statusError:
			hasErrors = true
			fmt.Fprintln(outWriter, colorize("✗ "+result.Message, ansiRed))
		default:
			hasErrors = true
			fmt.Fprintln(outWriter, colorize("✗ "+result.Message, ansiRed))
		}
	}

	return !hasErrors
}

func colorize(text, color string) string {
	if !colorOutputEnabled || color == "" {
		return text
	}

	return color + text + ansiReset
}

func supportsColorOutput() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}

	term := os.Getenv("TERM")
	if term == "" || term == "dumb" {
		return false
	}

	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0
}

func checkFlatpak() DoctorResult {
	return checkRequiredTool("flatpak")
}

func checkMise() DoctorResult {
	return checkRequiredTool("mise")
}

func checkDNF5() DoctorResult {
	distroFamily := detectDistroFamilyFunc()
	toolPath, err := lookPathFunc("dnf5")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			status := statusWarning
			if distroFamily == utils.DistroFamilyFedoraLike {
				status = statusError
			}

			return DoctorResult{
				Name:    "dnf5",
				Status:  status,
				Message: "dnf5 is not installed",
			}
		}

		status := statusWarning
		if distroFamily == utils.DistroFamilyFedoraLike {
			status = statusError
		}

		return DoctorResult{
			Name:    "dnf5",
			Status:  status,
			Message: fmt.Sprintf("failed to find dnf5: %v", err),
		}
	}

	return DoctorResult{
		Name:    "dnf5",
		Status:  statusOK,
		Message: fmt.Sprintf("dnf5 is installed (%s)", toolPath),
	}
}

func checkRequiredTool(name string) DoctorResult {
	toolPath, err := lookPathFunc(name)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return DoctorResult{
				Name:    name,
				Status:  statusError,
				Message: fmt.Sprintf("%s is not installed", name),
			}
		}

		return DoctorResult{
			Name:    name,
			Status:  statusError,
			Message: fmt.Sprintf("failed to find %s: %v", name, err),
		}
	}

	return DoctorResult{
		Name:    name,
		Status:  statusOK,
		Message: fmt.Sprintf("%s is installed (%s)", name, toolPath),
	}
}
