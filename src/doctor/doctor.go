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
)

var (
	lookPathFunc                     = exec.LookPath
	detectDistroFamilyFunc           = utils.DetectDistroFamily
	outWriter              io.Writer = os.Stdout
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

	const (
		green  = "\033[32m"
		yellow = "\033[33m"
		red    = "\033[31m"
		reset  = "\033[0m"
	)

	hasErrors := false
	for _, check := range checks {
		result := check()
		switch result.Status {
		case statusOK:
			fmt.Fprintf(outWriter, "%s✓ %s%s\n", green, result.Message, reset)
		case statusWarning:
			fmt.Fprintf(outWriter, "%s! %s%s\n", yellow, result.Message, reset)
		case statusError:
			hasErrors = true
			fmt.Fprintf(outWriter, "%s✗ %s%s\n", red, result.Message, reset)
		default:
			hasErrors = true
			fmt.Fprintf(outWriter, "%s✗ %s%s\n", red, result.Message, reset)
		}
	}

	return !hasErrors
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
