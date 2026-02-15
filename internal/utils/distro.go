package utils

import (
	"bufio"
	"os"
	"strings"
)

type DistroFamily string

const (
	DistroFamilyUnknown       DistroFamily = "unknown"
	DistroFamilyFedoraLike    DistroFamily = "fedora-like"
	DistroFamilyNonFedoraLike DistroFamily = "non-fedora-like"
)

var fedoraLikeTokens = map[string]struct{}{
	"fedora":    {},
	"rhel":      {},
	"centos":    {},
	"rocky":     {},
	"almalinux": {},
}

// DetectDistroFamily reads /etc/os-release and classifies the distro family.
func DetectDistroFamily() DistroFamily {
	contents, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return DistroFamilyUnknown
	}

	return detectDistroFamilyFromOSRelease(string(contents))
}

func detectDistroFamilyFromOSRelease(contents string) DistroFamily {
	values := parseOSRelease(contents)
	id, hasID := values["ID"]
	idLike, hasIDLike := values["ID_LIKE"]
	if !hasID && !hasIDLike {
		return DistroFamilyUnknown
	}

	if hasFedoraLikeToken(id) || hasFedoraLikeToken(idLike) {
		return DistroFamilyFedoraLike
	}

	return DistroFamilyNonFedoraLike
}

func parseOSRelease(contents string) map[string]string {
	values := make(map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(contents))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		eqIdx := strings.IndexByte(line, '=')
		if eqIdx < 1 {
			continue
		}

		key := strings.TrimSpace(line[:eqIdx])
		if key == "" {
			continue
		}

		rawValue := strings.TrimSpace(line[eqIdx+1:])
		value := parseOSReleaseValue(rawValue)
		values[strings.ToUpper(key)] = value
	}

	return values
}

func parseOSReleaseValue(raw string) string {
	if raw == "" {
		return ""
	}

	if raw[0] == '"' || raw[0] == '\'' {
		quote := raw[0]
		var b strings.Builder
		escaped := false
		for i := 1; i < len(raw); i++ {
			ch := raw[i]
			if escaped {
				b.WriteByte(ch)
				escaped = false
				continue
			}

			if ch == '\\' {
				escaped = true
				continue
			}

			if ch == quote {
				return b.String()
			}

			b.WriteByte(ch)
		}

		return b.String()
	}

	if commentIdx := strings.IndexByte(raw, '#'); commentIdx >= 0 {
		raw = raw[:commentIdx]
	}

	return strings.TrimSpace(raw)
}

func hasFedoraLikeToken(value string) bool {
	tokens := strings.FieldsFunc(value, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})

	for _, token := range tokens {
		token = strings.ToLower(token)
		if _, ok := fedoraLikeTokens[token]; ok {
			return true
		}
	}

	return false
}
