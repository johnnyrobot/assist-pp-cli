// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestNovelResolveInstitutionHelpWires smoke-tests that the resolve institution command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelResolveInstitutionHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"resolve", "institution", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("resolve institution --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "institution"} {
		if !strings.Contains(help, want) {
			t.Fatalf("resolve institution --help missing %q in output:\n%s", want, help)
		}
	}
}

type assistRequestLog struct {
	mu     sync.Mutex
	counts map[string]int
}

func (l *assistRequestLog) add(r *http.Request) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counts[r.URL.RequestURI()]++
}

func (l *assistRequestLog) count(uri string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.counts[uri]
}

func executeAssistTestCommand(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func newAssistReferenceServer(t *testing.T, institutions, years string) (*httptest.Server, *assistRequestLog) {
	t.Helper()
	log := &assistRequestLog{counts: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.add(r)
		if got := r.Header.Get("API-Key"); got != "" {
			t.Errorf("%s unexpectedly sent API-Key = %q", r.URL.Path, got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/institutions":
			fmt.Fprint(w, institutions)
		case "/api/AcademicYears":
			fmt.Fprint(w, years)
		default:
			http.Error(w, `{"error":"unexpected path"}`, http.StatusNotFound)
		}
	}))
	return server, log
}

func TestNovelResolveInstitutionExactAndNormalizedMatches(t *testing.T) {
	institutions := `[
		{"id":14,"code":"RSC","names":[{"name":"Rancho Santiago College"},{"name":"Santa Ana College","fromYear":1997}],"category":2},
		{"id":7,"code":"UCD","names":{"currentName":"University of California, Davis","formerNames":["Davis Campus"]}}
	]`
	years := `[
		{"id":74,"code":"2023-2024","beginDate":"2023-10-01T00:00:00","endDate":"2024-10-01T00:00:00"},
		{"id":75,"code":"2024-2025","description":"Catalog 2024 / 2025","beginDate":"2024-10-01T00:00:00","endDate":"2025-10-01T00:00:00"}
	]`
	tests := []struct {
		name            string
		institution     string
		year            string
		wantInstitution float64
		wantYear        float64
	}{
		{name: "exact code and numeric year id", institution: "RSC", year: "75", wantInstitution: 14, wantYear: 75},
		{name: "exact current name and year code", institution: "Santa Ana College", year: "2024-2025", wantInstitution: 14, wantYear: 75},
		{name: "normalized punctuation whitespace and date label", institution: "  santa---ANA,   college!! ", year: "2025/10/01", wantInstitution: 14, wantYear: 75},
		{name: "object current-name shape and description label", institution: "University of California Davis", year: "Catalog 2024-2025", wantInstitution: 7, wantYear: 75},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server, requests := newAssistReferenceServer(t, institutions, years)
			defer server.Close()
			t.Setenv("ASSIST_BASE_URL", server.URL)
			t.Setenv("ASSIST_API_KEY", "test-assist-key")

			stdout, stderr, err := executeAssistTestCommand(t,
				"resolve", "institution", tc.institution, "--year", tc.year,
				"--json", "--no-cache", "--data-source", "live",
			)
			if err != nil {
				t.Fatalf("resolve institution error = %v\nstderr: %s", err, stderr)
			}
			var out map[string]any
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("decode output %q: %v", stdout, err)
			}
			if got := out["institution_id"]; got != tc.wantInstitution {
				t.Errorf("institution_id = %#v, want %#v; output=%s", got, tc.wantInstitution, stdout)
			}
			if got := out["academic_year_id"]; got != tc.wantYear {
				t.Errorf("academic_year_id = %#v, want %#v; output=%s", got, tc.wantYear, stdout)
			}
			if out["source"] != "live" {
				t.Errorf("source = %#v, want live", out["source"])
			}
			institution, ok := out["institution"].(map[string]any)
			if !ok || institution["code"] != map[float64]string{14: "RSC", 7: "UCD"}[tc.wantInstitution] {
				t.Errorf("official institution record not preserved: %#v", out["institution"])
			}
			if requests.count("/api/institutions") != 1 || requests.count("/api/AcademicYears") != 1 {
				t.Errorf("reference call counts = institutions %d, years %d; want 1 each", requests.count("/api/institutions"), requests.count("/api/AcademicYears"))
			}
		})
	}
}

func TestNovelResolveInstitutionRejectsAmbiguousMissingAndIrrelevantMatches(t *testing.T) {
	years := `[{"id":75,"code":"2024-2025"}]`
	tests := []struct {
		name         string
		institutions string
		query        string
		wantError    string
		wantCode     int
	}{
		{
			name:         "ambiguous normalized name",
			institutions: `[{"id":11,"code":"N1","names":[{"name":"North College","fromYear":2020}]},{"id":12,"code":"N2","names":[{"name":"North College","fromYear":2021}]}]`,
			query:        "north college",
			wantError:    "ambiguous",
			wantCode:     2,
		},
		{
			name:         "not found",
			institutions: `[{"id":7,"code":"UCD","names":[{"name":"UC Davis","fromYear":2020}]}]`,
			query:        "Berkeley City College",
			wantError:    "was not found",
			wantCode:     3,
		},
		{
			name:         "partial name never selects irrelevant record",
			institutions: `[{"id":7,"code":"UCD","names":[{"name":"UC Davis","fromYear":2020}]},{"id":8,"code":"DTC","names":[{"name":"Davis Technical College","fromYear":2020}]}]`,
			query:        "Davis",
			wantError:    "was not found",
			wantCode:     3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server, _ := newAssistReferenceServer(t, tc.institutions, years)
			defer server.Close()
			t.Setenv("ASSIST_BASE_URL", server.URL)
			t.Setenv("ASSIST_API_KEY", "test-assist-key")

			_, _, err := executeAssistTestCommand(t,
				"resolve", "institution", tc.query, "--year", "2024-2025",
				"--json", "--no-cache", "--data-source", "live",
			)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want substring %q", err, tc.wantError)
			}
			if got := ExitCode(err); got != tc.wantCode {
				t.Errorf("ExitCode(error) = %d, want %d; error=%v", got, tc.wantCode, err)
			}
		})
	}
}

func TestNovelResolveInstitutionRejectsLocalDataSourceBeforeNetwork(t *testing.T) {
	server, requests := newAssistReferenceServer(t, `[]`, `[]`)
	defer server.Close()
	t.Setenv("ASSIST_BASE_URL", server.URL)
	t.Setenv("ASSIST_API_KEY", "test-assist-key")

	_, _, err := executeAssistTestCommand(t,
		"resolve", "institution", "RSC", "--year", "2024-2025", "--data-source", "local",
	)
	if err == nil || !strings.Contains(err.Error(), "no local data source") {
		t.Fatalf("error = %v, want live-only data-source rejection", err)
	}
	if requests.count("/api/institutions") != 0 || requests.count("/api/AcademicYears") != 0 {
		t.Fatalf("local rejection made network calls: %#v", requests.counts)
	}
}

func TestNovelResolveInstitutionMissingInputsRemainUsageErrorsOnDryRun(t *testing.T) {
	tests := [][]string{
		{"resolve", "institution", "RSC", "--dry-run"},
		{"resolve", "institution", "--year", "2024-2025", "--dry-run"},
		{"resolve", "institution", "RSC", "extra", "--year", "2024-2025", "--dry-run"},
	}
	for _, args := range tests {
		_, _, err := executeAssistTestCommand(t, args...)
		if err == nil || ExitCode(err) != 2 {
			t.Errorf("%v error = %v (code %d), want usage error code 2", args, err, ExitCode(err))
		}
	}
}
