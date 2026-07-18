package codexruntime

import (
	"strconv"
	"strings"
)

type semanticVersion struct {
	core       [3]int
	prerelease []string
	valid      bool
}

func compareCodexVersions(left, right string) int {
	a := parseCodexVersion(left)
	b := parseCodexVersion(right)
	if !a.valid && !b.valid {
		return 0
	}
	if a.valid && !b.valid {
		return 1
	}
	if !a.valid {
		return -1
	}
	for index := range a.core {
		if a.core[index] > b.core[index] {
			return 1
		}
		if a.core[index] < b.core[index] {
			return -1
		}
	}
	if len(a.prerelease) == 0 && len(b.prerelease) > 0 {
		return 1
	}
	if len(a.prerelease) > 0 && len(b.prerelease) == 0 {
		return -1
	}
	for index := 0; index < len(a.prerelease) && index < len(b.prerelease); index++ {
		if comparison := comparePrereleaseIdentifier(a.prerelease[index], b.prerelease[index]); comparison != 0 {
			return comparison
		}
	}
	if len(a.prerelease) > len(b.prerelease) {
		return 1
	}
	if len(a.prerelease) < len(b.prerelease) {
		return -1
	}
	return 0
}

func parseCodexVersion(value string) semanticVersion {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return semanticVersion{}
	}
	token := strings.TrimPrefix(fields[len(fields)-1], "v")
	token, _, _ = strings.Cut(token, "+")
	coreValue, prereleaseValue, hasPrerelease := strings.Cut(token, "-")
	parts := strings.Split(coreValue, ".")
	if len(parts) != 3 {
		return semanticVersion{}
	}
	parsed := semanticVersion{valid: true}
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return semanticVersion{}
		}
		parsed.core[index] = number
	}
	if hasPrerelease {
		if prereleaseValue == "" {
			return semanticVersion{}
		}
		parsed.prerelease = strings.Split(prereleaseValue, ".")
	}
	return parsed
}

func comparePrereleaseIdentifier(left, right string) int {
	leftNumber, leftNumeric := numericIdentifier(left)
	rightNumber, rightNumeric := numericIdentifier(right)
	if leftNumeric && rightNumeric {
		if leftNumber > rightNumber {
			return 1
		}
		if leftNumber < rightNumber {
			return -1
		}
		return 0
	}
	if leftNumeric {
		return -1
	}
	if rightNumeric {
		return 1
	}
	return strings.Compare(left, right)
}

func numericIdentifier(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	number, err := strconv.Atoi(value)
	return number, err == nil && number >= 0
}
