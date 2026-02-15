package main

import (
	"fmt"
	"os"
	"path"
	"xci/internal/utils"
	"xci/src/tools"
	"xci/src/version"
)

func main() {
	cmd := command()

	// Force inclusion of magic string in binary - must be used to prevent optimization
	if len(version.Embedded) == 0 {
		// This will never execute but prevents the compiler from optimizing away Embedded
		panic("impossible")
	}

	switch cmd {
	case "doctor":
		if !runDoctor() {
			os.Exit(1)
		}

	case "i", "install":
		args := os.Args[2:]
		if len(args) == 0 {
			fmt.Println("Nothing to install")
			return
		}

		if err := tools.Install(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error installing: %v\n", err)
			os.Exit(1)
		}

	case "update":
		if err := tools.Update(); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating: %v\n", err)
			os.Exit(1)
		}

	case "self":
		if len(os.Args) < 3 || os.Args[2] != "install" {
			fmt.Fprintf(os.Stderr, "Usage: xci self install\n")
			os.Exit(1)
		}
		if err := tools.SelfInstall(); err != nil {
			fmt.Fprintf(os.Stderr, "Error installing: %v\n", err)
			os.Exit(1)
		}

	case "version", "--version", "-v":
		fmt.Printf("%s version %s\n", version.Name, version.Version)
		// Reference to ensure magic string is embedded in binary
		_ = version.Embedded

	}

}

func runDoctor() bool {
	checks := []func() tools.DoctorResult{
		tools.Doctor,
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

func command() string {
	cmd := os.Args[0]
	_, cmd = path.Split(cmd)

	if len(os.Args) <= 1 {
		return cmd
	}

	if !utils.IsCLICommandSource(cmd) {
		return cmd
	}

	if os.Args[1] == "--" {
		if len(os.Args) > 2 {
			return os.Args[2]
		}

		return cmd
	}

	return os.Args[1]
}
