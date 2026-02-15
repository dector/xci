package mise

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

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

// Update runs 'mise update --dry-run', asks for confirmation, then runs 'mise update'
func Update() error {
	// Run dry-run first and capture output
	var outBuf, errBuf bytes.Buffer
	dryRunCmd := exec.Command("mise", "up", "--dry-run")
	dryRunCmd.Stdout = &outBuf
	dryRunCmd.Stderr = &errBuf

	if err := dryRunCmd.Run(); err != nil {
		return fmt.Errorf("dry-run failed: %w", err)
	}

	// Display the captured output
	output := outBuf.String()
	errOutput := errBuf.String()
	fmt.Print(output)
	fmt.Print(errOutput)

	// Check if all tools are already up to date
	if strings.Contains(output, "All tools are up to date") ||
		strings.Contains(errOutput, "All tools are up to date") {
		return nil
	}

	// Ask user for confirmation
	fmt.Print("\nDo you want to continue with the update? (y/n): ")
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))
	if response != "y" {
		fmt.Println("Update cancelled")
		return nil
	}

	// Run actual update
	fmt.Println("\nUpdating...")
	updateCmd := exec.Command("mise", "up")
	updateCmd.Stdout = os.Stdout
	updateCmd.Stderr = os.Stderr
	updateCmd.Stdin = os.Stdin

	return updateCmd.Run()
}
