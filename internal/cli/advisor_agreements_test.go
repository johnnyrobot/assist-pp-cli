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
	"net/url"
	"strings"
	"testing"
)

// TestNovelAdvisorAgreementsHelpWires smoke-tests that the advisor agreements command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelAdvisorAgreementsHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"advisor", "agreements", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("advisor agreements --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "agreements"} {
		if !strings.Contains(help, want) {
			t.Fatalf("advisor agreements --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestNovelAdvisorAgreementsHappyPathCallsEachRequestedType(t *testing.T) {
	requests := &assistRequestLog{counts: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.add(r)
		if got := r.Header.Get("API-Key"); got != "" {
			t.Errorf("%s unexpectedly sent API-Key = %q", r.URL.Path, got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/institutions":
			fmt.Fprint(w, `[
				{"id":110,"code":"DVC","names":[{"name":"Diablo Valley College","fromYear":1949}]},
				{"id":7,"code":"UCD","names":[{"name":"University of California, Davis","fromYear":1959}]},
				{"id":999,"code":"IRRELEVANT","names":[{"name":"Davis Valley College","fromYear":2000}]}
			]`)
		case "/api/AcademicYears":
			fmt.Fprint(w, `[{"id":75,"fallYear":2024,"code":"2024-2025","beginDate":"2024-10-01T00:00:00"}]`)
		case "/api/institutions/110/agreements":
			fmt.Fprint(w, `[{"institutionParentId":7,"receivingYearIds":[75],"note":"official partner"}]`)
		case "/api/agreements":
			switch r.URL.Query().Get("categoryCode") {
			case "major":
				fmt.Fprint(w, `{"result":{"reports":[{"key":"major-key","type":"Major","label":"Chemistry"}]},"isSuccessful":true}`)
			case "dept":
				fmt.Fprint(w, `{"result":{"reports":[{"key":"department-key","type":"Department","label":"Physical Sciences"}]},"isSuccessful":true}`)
			default:
				http.Error(w, `{"error":"unexpected type"}`, http.StatusBadRequest)
			}
		default:
			http.Error(w, `{"error":"unexpected path"}`, http.StatusNotFound)
		}
	}))
	defer server.Close()
	t.Setenv("ASSIST_BASE_URL", server.URL)
	t.Setenv("ASSIST_API_KEY", "test-assist-key")

	stdout, stderr, err := executeAssistTestCommand(t,
		"advisor", "agreements",
		"--sending", "Diablo Valley College",
		"--receiving", "University of California Davis",
		"--year", "2024-2025",
		"--types", "Major, Department",
		"--json", "--no-cache", "--data-source", "live",
	)
	if err != nil {
		t.Fatalf("advisor agreements error = %v\nstderr: %s", err, stderr)
	}

	var out advisorAgreementsEnvelope
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	if out.Source != "live" || out.Resolved.Sending.ID != 110 || out.Resolved.Receiving.ID != 7 || out.Resolved.AcademicYear.ID != 75 {
		t.Errorf("resolved envelope incorrect: %#v", out)
	}
	if got := strings.Join(out.RequestedTypes, ","); got != "Major,Department" {
		t.Errorf("requested_types = %q, want Major,Department", got)
	}
	if !out.PartnerVerification.Verified || !strings.Contains(string(out.PartnerVerification.Record), "official partner") {
		t.Errorf("partner verification does not preserve official record: %#v", out.PartnerVerification)
	}
	if len(out.Agreements) != 2 {
		t.Fatalf("agreements length = %d, want 2; output=%s", len(out.Agreements), stdout)
	}
	if out.Agreements[0].Type != "Major" || !strings.Contains(string(out.Agreements[0].Response), "major-key") {
		t.Errorf("Major official response missing: %#v", out.Agreements[0])
	}
	if out.Agreements[1].Type != "Department" || !strings.Contains(string(out.Agreements[1].Response), "department-key") {
		t.Errorf("Department official response missing: %#v", out.Agreements[1])
	}

	wantURIs := []string{
		"/api/institutions",
		"/api/AcademicYears",
		"/api/institutions/110/agreements?asSendingOnly=true",
		"/api/agreements?receivingInstitutionId=7&sendingInstitutionId=110&academicYearId=75&categoryCode=major",
		"/api/agreements?receivingInstitutionId=7&sendingInstitutionId=110&academicYearId=75&categoryCode=dept",
	}
	for _, uri := range wantURIs {
		if got := requests.count(uri); got != 1 {
			t.Errorf("request count for %s = %d, want 1", uri, got)
		}
	}
}

func TestNovelAdvisorAgreementsRejectsInvalidTypeBeforeNetwork(t *testing.T) {
	requests := &assistRequestLog{counts: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.add(r)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()
	t.Setenv("ASSIST_BASE_URL", server.URL)
	t.Setenv("ASSIST_API_KEY", "test-assist-key")

	_, _, err := executeAssistTestCommand(t,
		"advisor", "agreements",
		"--sending", "DVC", "--receiving", "UCD", "--year", "2024-2025", "--types", "Major,Unknown",
	)
	if err == nil || !strings.Contains(err.Error(), "invalid agreement type") {
		t.Fatalf("error = %v, want invalid agreement type", err)
	}
	requests.mu.Lock()
	defer requests.mu.Unlock()
	if len(requests.counts) != 0 {
		t.Fatalf("invalid type made network calls: %#v", requests.counts)
	}
}

func TestNovelAdvisorAgreementsMissingPublishedPartnerIsExplicitAndStops(t *testing.T) {
	requests := &assistRequestLog{counts: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.add(r)
		if got := r.Header.Get("API-Key"); got != "" {
			t.Errorf("%s unexpectedly sent API-Key = %q", r.URL.Path, got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/institutions":
			fmt.Fprint(w, `[{"id":110,"code":"DVC","names":["Diablo Valley College"]},{"id":7,"code":"UCD","names":["UC Davis"]}]`)
		case "/api/AcademicYears":
			fmt.Fprint(w, `[{"id":75,"code":"2024-2025"}]`)
		case "/api/institutions/110/agreements":
			// The target record exists, but only in the reverse direction. It
			// must not verify a sending-110 -> receiving-7 workflow.
			fmt.Fprint(w, `[{"institutionParentId":7,"receivingYearIds":null,"sendingYearIds":[75]}]`)
		default:
			http.Error(w, `{"error":"agreement list must not be called"}`, http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	t.Setenv("ASSIST_BASE_URL", server.URL)
	t.Setenv("ASSIST_API_KEY", "test-assist-key")

	_, _, err := executeAssistTestCommand(t,
		"advisor", "agreements",
		"--sending", "DVC", "--receiving", "UCD", "--year", "2024-2025", "--types", "Major",
		"--json", "--no-cache", "--data-source", "live",
	)
	if err == nil || !strings.Contains(err.Error(), "not a published receiving partner") {
		t.Fatalf("error = %v, want explicit missing receiving-partner result", err)
	}
	if got := ExitCode(err); got != 3 {
		t.Errorf("ExitCode(error) = %d, want 3", got)
	}
	if requests.count("/api/institutions/110/agreements?asSendingOnly=true") != 1 {
		t.Errorf("published partner endpoint call count = %d, want 1", requests.count("/api/institutions/110/agreements?asSendingOnly=true"))
	}
	requests.mu.Lock()
	defer requests.mu.Unlock()
	for uri := range requests.counts {
		parsed, parseErr := url.ParseRequestURI(uri)
		if parseErr == nil && parsed.Path == "/api/agreements" {
			t.Errorf("agreement list endpoint was called after partner failure: %s", uri)
		}
	}
}

func TestNovelAdvisorAgreementsRejectsLocalDataSource(t *testing.T) {
	t.Setenv("ASSIST_API_KEY", "test-assist-key")
	_, _, err := executeAssistTestCommand(t,
		"advisor", "agreements",
		"--sending", "DVC", "--receiving", "UCD", "--year", "2024-2025", "--types", "Major",
		"--data-source", "local",
	)
	if err == nil || !strings.Contains(err.Error(), "no local data source") {
		t.Fatalf("error = %v, want live-only data-source rejection", err)
	}
}

func TestNovelAdvisorAgreementsMissingInputsRemainUsageErrorsOnDryRun(t *testing.T) {
	_, _, err := executeAssistTestCommand(t,
		"advisor", "agreements", "--sending", "DVC", "--receiving", "UCD", "--year", "2024-2025", "--dry-run",
	)
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "--types") {
		t.Fatalf("error = %v (code %d), want missing --types usage error", err, ExitCode(err))
	}
}
