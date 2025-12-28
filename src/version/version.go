package version

const (
	// MagicString is embedded in every xci binary for identification
	// Using a simpler format to avoid compiler optimizations that might split it
	MagicString = "<<XCI-SELFINSTALL-MAGIC-v1>>"
	Version     = "0.1.0"
	Name        = "xci"
)

// Embedded ensures the magic string is included in the binary
// Using a byte slice to force it into the data section
var Embedded = []byte(MagicString)
