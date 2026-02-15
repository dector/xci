package utils

import "strings"

// IsCLICommandSource reports whether the binary name expects subcommands in argv.
func IsCLICommandSource(cmd string) bool {
	if cmd == "xci" {
		return true
	}

	if strings.HasPrefix(cmd, "xci.") {
		return true
	}

	if cmd == "main" {
		return true
	}

	return false
}
