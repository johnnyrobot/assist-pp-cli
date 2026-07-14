// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// TestNovelAgreementsDiffHelpWires smoke-tests that the agreements diff command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelAgreementsDiffHelpWires(t *testing.T) {
	cmd := RootCmd()
	cmd.SetArgs([]string{"agreements", "diff", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("agreements diff --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "diff"} {
		if !strings.Contains(help, want) {
			t.Fatalf("agreements diff --help missing %q in output:\n%s", want, help)
		}
	}
}

func TestAgreementsDiffFetchesBothOfficialPayloadsAndDiffsNestedJSON(t *testing.T) {
	responses := map[string]string{
		"from/key": `{"result":{"name":"old","nested":{"keep":1,"gone":true},"arr":[{"v":"a"},2]}}`,
		"to/key":   `{"result":{"name":"new","nested":{"keep":1,"add":"yes"},"arr":[{"v":"b"},2,3]}}`,
	}
	var gotKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/articulation/Agreements" {
			t.Errorf("path = %q, want /api/articulation/Agreements", r.URL.Path)
		}
		if got := r.Header.Get("API-Key"); got != "" {
			t.Errorf("public request unexpectedly sent API-Key = %q", got)
		}
		key := r.URL.Query().Get("Key")
		gotKeys = append(gotKeys, key)
		body, ok := responses[key]
		if !ok {
			http.Error(w, "unexpected key", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()
	t.Setenv("ASSIST_BASE_URL", server.URL)
	t.Setenv("ASSIST_API_KEY", "diff-test-key")

	stdout, stderr, err := runRootArgs(t, "agreements", "diff", "from/key", "to/key", "--json", "--no-cache")
	if err != nil {
		t.Fatalf("agreements diff error = %v (stderr=%q)", err, stderr)
	}
	if !reflect.DeepEqual(gotKeys, []string{"from/key", "to/key"}) {
		t.Fatalf("request Key values = %#v, want from/key then to/key", gotKeys)
	}
	var got agreementDiffResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode output %q: %v", stdout, err)
	}
	if got.Keys != (agreementDiffKeys{From: "from/key", To: "to/key"}) || got.Source != "live" {
		t.Fatalf("keys/source = %#v/%q", got.Keys, got.Source)
	}
	if got.Counts != (agreementDiffCounts{Added: 2, Removed: 1, Changed: 2}) {
		t.Fatalf("counts = %#v", got.Counts)
	}
	assertDeltaPaths(t, got.Added, []string{"/arr/2", "/nested/add"})
	assertDeltaPaths(t, got.Removed, []string{"/nested/gone"})
	assertChangePaths(t, got.Changed, []string{"/arr/0/v", "/name"})
	if got.Added[0].Value != float64(3) || got.Added[1].Value != "yes" || got.Removed[0].Value != true {
		t.Fatalf("unexpected exact delta values: added=%#v removed=%#v", got.Added, got.Removed)
	}
	if got.Changed[0].Before != "a" || got.Changed[0].After != "b" || got.Changed[1].Before != "old" || got.Changed[1].After != "new" {
		t.Fatalf("unexpected exact changed values: %#v", got.Changed)
	}
}

func TestAgreementsDiffIdenticalPayloadsHaveEmptyArrays(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":{"id":1,"nested":{"same":true}}}`))
	}))
	defer server.Close()
	t.Setenv("ASSIST_BASE_URL", server.URL)
	t.Setenv("ASSIST_API_KEY", "diff-test-key")

	stdout, stderr, err := runRootArgs(t, "agreements", "diff", "one", "two", "--json", "--no-cache")
	if err != nil {
		t.Fatalf("agreements diff error = %v (stderr=%q)", err, stderr)
	}
	var got agreementDiffResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if got.Counts != (agreementDiffCounts{}) || len(got.Added) != 0 || len(got.Removed) != 0 || len(got.Changed) != 0 {
		t.Fatalf("identical diff = %#v", got)
	}
	if got.Added == nil || got.Removed == nil || got.Changed == nil {
		t.Fatalf("empty arrays must marshal as [] rather than null: %s", stdout)
	}
}

func TestAgreementsDiffExpandsOfficialJSONStringFields(t *testing.T) {
	responses := []string{
		`{"result":{"articulations":"[{\"course\":{\"id\":10,\"title\":\"Old\"}}]","receivingInstitution":"{\"id\":7}"}}`,
		`{"result":{"articulations":"[{\"course\":{\"id\":10,\"title\":\"New\"}}]","receivingInstitution":"{\"id\":7}"}}`,
	}
	index := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(responses[index]))
		index++
	}))
	defer server.Close()
	t.Setenv("ASSIST_BASE_URL", server.URL)
	t.Setenv("ASSIST_API_KEY", "diff-test-key")

	stdout, stderr, err := runRootArgs(t, "agreements", "diff", "old", "new", "--agent", "--no-cache")
	if err != nil {
		t.Fatalf("agreements diff error = %v (stderr=%q)", err, stderr)
	}
	var envelope struct {
		Results agreementDiffResult `json:"results"`
		Meta    map[string]any      `json:"meta"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Meta["source"] != "live" {
		t.Fatalf("agent meta source = %#v, want live", envelope.Meta["source"])
	}
	assertChangePaths(t, envelope.Results.Changed, []string{"/articulations/0/course/title"})
}

func TestAgreementsDiffRejectsPartialInputAndWrongDataSource(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "one key", args: []string{"agreements", "diff", "only-one"}, want: "exactly two"},
		{name: "one key dry run", args: []string{"agreements", "diff", "only-one", "--dry-run"}, want: "exactly two"},
		{name: "local source", args: []string{"agreements", "diff", "one", "two", "--data-source", "local"}, want: "no local data source"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := runRootArgs(t, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func assertDeltaPaths(t *testing.T, got []jsonValueDelta, want []string) {
	t.Helper()
	paths := make([]string, len(got))
	for i := range got {
		paths[i] = got[i].Path
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("delta paths = %#v, want %#v", paths, want)
	}
}

func assertChangePaths(t *testing.T, got []jsonValueChange, want []string) {
	t.Helper()
	paths := make([]string, len(got))
	for i := range got {
		paths[i] = got[i].Path
	}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("change paths = %#v, want %#v", paths, want)
	}
}
