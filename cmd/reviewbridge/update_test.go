package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUpdateShowsUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"tag_name": "v0.1.0",
			"html_url": "https://github.com/ahmedennaime/reviewbridge/releases/tag/v0.1.0",
		})
	}))
	defer srv.Close()

	checker := &updateChecker{version: "0.1.0", http: srv.Client(), releasesURL: srv.URL}
	var out strings.Builder
	if err := checker.check(&out); err != nil {
		t.Fatalf("check: %v", err)
	}

	if !strings.Contains(out.String(), "up to date") {
		t.Errorf("expected 'up to date', got: %q", out.String())
	}
}

func TestUpdateShowsNewVersion(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"tag_name": "v0.2.0",
			"html_url": "https://github.com/ahmedennaime/reviewbridge/releases/tag/v0.2.0",
		})
	}))
	defer srv.Close()

	checker := &updateChecker{version: "0.1.0", http: srv.Client(), releasesURL: srv.URL}
	var out strings.Builder
	if err := checker.check(&out); err != nil {
		t.Fatalf("check: %v", err)
	}

	result := out.String()
	if !strings.Contains(result, "0.2.0") {
		t.Errorf("expected new version in output, got: %q", result)
	}
	if !strings.Contains(result, "brew upgrade") {
		t.Errorf("expected upgrade instructions in output, got: %q", result)
	}
}

func TestUpdateHandlesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	checker := &updateChecker{version: "0.1.0", http: srv.Client(), releasesURL: srv.URL}
	err := checker.check(&strings.Builder{})
	if err == nil {
		t.Error("expected error for API failure")
	}
}

func TestUpdateStripsVPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
			"tag_name": "v0.1.0",
			"html_url": "https://github.com/ahmedennaime/reviewbridge/releases/tag/v0.1.0",
		})
	}))
	defer srv.Close()

	checker := &updateChecker{version: "v0.1.0", http: srv.Client(), releasesURL: srv.URL}
	var out strings.Builder
	if err := checker.check(&out); err != nil {
		t.Fatalf("check: %v", err)
	}

	if !strings.Contains(out.String(), "up to date") {
		t.Errorf("v-prefixed current version should compare correctly, got: %q", out.String())
	}
}
