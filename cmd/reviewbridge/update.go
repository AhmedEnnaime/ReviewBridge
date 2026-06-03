package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const releasesURL = "https://api.github.com/repos/ahmedennaime/reviewbridge/releases/latest"

type updateChecker struct {
	version     string
	http        *http.Client
	releasesURL string
}

func newUpdateChecker(currentVersion string) *updateChecker {
	return &updateChecker{
		version:     strings.TrimPrefix(currentVersion, "v"),
		http:        &http.Client{Timeout: 10 * time.Second},
		releasesURL: releasesURL,
	}
}

func (u *updateChecker) check(out io.Writer) error {
	req, err := http.NewRequest("GET", u.releasesURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := u.http.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("could not parse release info: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")
	current := strings.TrimPrefix(u.version, "v")

	if latest == "" || latest == current {
		fmt.Fprintf(out, "reviewbridge v%s is up to date\n", current)
		return nil
	}

	fmt.Fprintf(out, "New version available: v%s (current: v%s)\n", latest, current)
	fmt.Fprintf(out, "Download: %s\n", release.HTMLURL)
	fmt.Fprintf(out, "Or run:   brew upgrade reviewbridge\n")
	return nil
}
