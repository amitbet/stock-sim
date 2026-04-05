package updater

import (
	"strconv"
	"strings"
)

type parsedSemver struct {
	major int
	minor int
	patch int
	pre   []string
}

func parseSemver(v string) (parsedSemver, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	if v == "" {
		return parsedSemver{}, false
	}

	core := v
	build := ""
	if idx := strings.IndexByte(core, '+'); idx >= 0 {
		build = core[idx+1:]
		core = core[:idx]
	}
	if build != "" && strings.Contains(build, " ") {
		return parsedSemver{}, false
	}

	pre := ""
	if idx := strings.IndexByte(core, '-'); idx >= 0 {
		pre = core[idx+1:]
		core = core[:idx]
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return parsedSemver{}, false
	}

	nums := [3]int{}
	for i, part := range parts {
		n, ok := parseSemverNumber(part)
		if !ok {
			return parsedSemver{}, false
		}
		nums[i] = n
	}

	var preParts []string
	if pre != "" {
		preParts = strings.Split(pre, ".")
		for _, part := range preParts {
			if !isValidPrereleaseIdentifier(part) {
				return parsedSemver{}, false
			}
		}
	}

	return parsedSemver{
		major: nums[0],
		minor: nums[1],
		patch: nums[2],
		pre:   preParts,
	}, true
}

func parseSemverNumber(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

func isValidPrereleaseIdentifier(s string) bool {
	if s == "" {
		return false
	}
	numeric := true
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && r != '-' {
			return false
		}
		if r < '0' || r > '9' {
			numeric = false
		}
	}
	if numeric && len(s) > 1 && s[0] == '0' {
		return false
	}
	return true
}

func compareSemver(a, b string) int {
	av, aok := parseSemver(a)
	bv, bok := parseSemver(b)
	if !aok || !bok {
		return 0
	}

	if av.major != bv.major {
		return compareInt(av.major, bv.major)
	}
	if av.minor != bv.minor {
		return compareInt(av.minor, bv.minor)
	}
	if av.patch != bv.patch {
		return compareInt(av.patch, bv.patch)
	}
	return comparePrerelease(av.pre, bv.pre)
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func comparePrerelease(a, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	if len(a) == 0 {
		return 1
	}
	if len(b) == 0 {
		return -1
	}

	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] == b[i] {
			continue
		}
		ai, aNum := parseNumericIdentifier(a[i])
		bi, bNum := parseNumericIdentifier(b[i])
		switch {
		case aNum && bNum:
			return compareInt(ai, bi)
		case aNum:
			return -1
		case bNum:
			return 1
		case a[i] < b[i]:
			return -1
		default:
			return 1
		}
	}

	return compareInt(len(a), len(b))
}

func parseNumericIdentifier(s string) (int, bool) {
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}
