package tools

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"xci/src/version"
)

// SelfInstall installs the current binary to ~/.local/bin/xci
func SelfInstall() error {
	fmt.Println("Starting xci self-installation...")

	// Get current binary path
	currentBinary, err := getCurrentBinary()
	if err != nil {
		return err
	}
	fmt.Printf("Current binary: %s\n", currentBinary)

	// Get target installation path
	targetPath, err := getTargetPath()
	if err != nil {
		return err
	}
	fmt.Printf("Target location: %s\n", targetPath)

	// Check if target already exists
	if _, err := os.Stat(targetPath); err == nil {
		// File exists - check if it's ours
		hasMarker, err := hasXCIMagicString(targetPath)
		if err != nil {
			return fmt.Errorf("failed to check existing binary: %w", err)
		}

		if !hasMarker {
			return fmt.Errorf(
				"ERROR: %s exists but was not installed by xci.\n"+
					"This could be another tool. Refusing to overwrite.\n"+
					"Please remove it manually if you want to proceed.",
				targetPath,
			)
		}

		fmt.Println("Existing xci installation detected. Will update.")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check target path: %w", err)
	}

	// Ensure directory exists
	targetDir := filepath.Dir(targetPath)
	if err := ensureDirectoryExists(targetDir); err != nil {
		return err
	}

	// Perform the copy
	fmt.Println("Installing...")
	if err := copyAndSetExecutable(currentBinary, targetPath); err != nil {
		return err
	}

	// Verify installation
	fmt.Println("Verifying installation...")
	if err := exec.Command(targetPath, "version").Run(); err != nil {
		return fmt.Errorf("installation verification failed: %w", err)
	}

	fmt.Printf("\n✓ Successfully installed xci to %s\n", targetPath)

	// Check if ~/.local/bin is in PATH
	pathEnv := os.Getenv("PATH")
	homeDir, _ := os.UserHomeDir()
	localBin := filepath.Join(homeDir, ".local", "bin")
	if !strings.Contains(pathEnv, localBin) {
		fmt.Printf("\nNOTE: %s is not in your PATH.\n", localBin)
		fmt.Println("Add this to your shell profile (~/.bashrc or ~/.zshrc):")
		fmt.Printf("  export PATH=\"$HOME/.local/bin:$PATH\"\n")
	}

	return nil
}

// getCurrentBinary returns the path to the currently running binary
func getCurrentBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve any symlinks
	realPath, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	return realPath, nil
}

// getTargetPath returns the installation target path
func getTargetPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".local", "bin", "xci"), nil
}

// hasXCIMagicString checks if a file contains the xci magic string
func hasXCIMagicString(filepath string) (bool, error) {
	// Read the entire file (Go binaries are typically 2-10MB)
	data, err := os.ReadFile(filepath)
	if err != nil {
		return false, fmt.Errorf("failed to read file: %w", err)
	}

	// Search for magic string
	return bytes.Contains(data, []byte(version.MagicString)), nil
}

// ensureDirectoryExists creates directory if it doesn't exist
func ensureDirectoryExists(dirPath string) error {
	info, err := os.Stat(dirPath)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s exists but is not a directory", dirPath)
		}
		return nil
	}

	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat directory: %w", err)
	}

	// Directory doesn't exist, ask user
	fmt.Printf("Directory %s does not exist.\n", dirPath)
	confirmed, err := askUserConfirmation("Create it now?")
	if err != nil {
		return err
	}

	if !confirmed {
		return fmt.Errorf("installation cancelled by user")
	}

	// Create with proper permissions
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	fmt.Printf("Created directory: %s\n", dirPath)
	return nil
}

// copyAndSetExecutable copies file and sets executable permissions
func copyAndSetExecutable(src, dst string) error {
	// Copy to temporary file first for atomic operation
	tempPath := dst + ".tmp"

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer srcFile.Close()

	// Create destination file with proper permissions
	dstFile, err := os.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	// Copy contents
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(tempPath) // Clean up on failure
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Close before rename
	dstFile.Close()

	// Atomic rename
	if err := os.Rename(tempPath, dst); err != nil {
		os.Remove(tempPath) // Clean up
		return fmt.Errorf("failed to install: %w", err)
	}

	// Ensure permissions are set (in case umask interfered)
	if err := os.Chmod(dst, 0755); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	return nil
}

// askUserConfirmation prompts user for yes/no
func askUserConfirmation(prompt string) (bool, error) {
	fmt.Printf("%s (y/n): ", prompt)

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read input: %w", err)
	}

	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y" || response == "yes", nil
}
