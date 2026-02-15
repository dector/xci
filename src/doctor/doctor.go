package doctor

import (
	"errors"
	"fmt"
	"os/exec"
)

type DoctorResult struct {
	Name    string
	OK      bool
	Message string
}

func Run() bool {
	checks := []func() DoctorResult{
		checkMise,
		checkFlatpak,
	}

	const (
		green = "\033[32m"
		red   = "\033[31m"
		reset = "\033[0m"
	)

	allOK := true
	for _, check := range checks {
		result := check()
		if result.OK {
			fmt.Printf("%s✓ %s%s\n", green, result.Message, reset)
			continue
		}

		allOK = false
		fmt.Printf("%s✗ %s%s\n", red, result.Message, reset)
	}

	return allOK
}

func checkFlatpak() DoctorResult {
	flatpakPath, err := exec.LookPath("flatpak")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return DoctorResult{
				Name:    "flatpak",
				OK:      false,
				Message: "flatpak is not installed",
			}
		}

		return DoctorResult{
			Name:    "flatpak",
			OK:      false,
			Message: fmt.Sprintf("failed to find flatpak: %v", err),
		}
	}

	return DoctorResult{
		Name:    "flatpak",
		OK:      true,
		Message: fmt.Sprintf("flatpak is installed (%s)", flatpakPath),
	}
}

func checkMise() DoctorResult {
	misePath, err := exec.LookPath("mise")
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return DoctorResult{
				Name:    "mise",
				OK:      false,
				Message: "mise is not installed",
			}
		}

		return DoctorResult{
			Name:    "mise",
			OK:      false,
			Message: fmt.Sprintf("failed to find mise: %v", err),
		}
	}

	return DoctorResult{
		Name:    "mise",
		OK:      true,
		Message: fmt.Sprintf("mise is installed (%s)", misePath),
	}
}
