package updater

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// Status is returned to the UI for update checks.
type Status struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	UpdateAvailable bool   `json:"update_available"`
	Message         string `json:"message,omitempty"`
}

// Check compares the running version to the latest GitHub release.
func Check(current string) (*Status, error) {
	st := &Status{Current: current}
	if _, err := repoFromEnv(); err != nil {
		st.Message = err.Error()
		return st, nil
	}

	rel, err := FetchLatestRelease()
	if err != nil {
		return nil, err
	}
	latest := strings.TrimSpace(rel.TagName)
	st.Latest = latest
	if latest == "" || current == "" || current == "dev" {
		st.Message = "Could not compare versions (need a release tag and a non-dev build version)"
		return st, nil
	}

	c := normalizeSemver(current)
	l := normalizeSemver(latest)
	if c == "" || l == "" {
		st.Message = "Non-semver tags; compare manually"
		return st, nil
	}
	st.UpdateAvailable = semver.Compare(l, c) > 0
	if !st.UpdateAvailable {
		st.Message = "You are on the latest release"
	} else {
		st.Message = fmt.Sprintf("Update available: %s → %s", current, latest)
	}
	return st, nil
}
