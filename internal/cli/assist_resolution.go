// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/johnnyrobot/assist-pp-cli/internal/client"
)

// assistResolutionData is a single live snapshot of ASSIST's two reference
// collections. Novel commands share it so resolving two institutions and a
// year does not repeat the same discovery requests.
type assistResolutionData struct {
	institutions  []json.RawMessage
	academicYears []json.RawMessage
}

type assistResolvedRecord struct {
	ID     int64
	Record json.RawMessage
}

type assistResolvedContext struct {
	Institution  assistResolvedRecord
	AcademicYear assistResolvedRecord
}

func fetchAssistResolutionData(ctx context.Context, c *client.Client) (assistResolutionData, error) {
	institutionsRaw, err := c.Get(ctx, "/api/institutions", nil)
	if err != nil {
		return assistResolutionData{}, err
	}
	institutions, err := assistRecordList(institutionsRaw)
	if err != nil {
		return assistResolutionData{}, fmt.Errorf("decode ASSIST institutions response: %w", err)
	}

	yearsRaw, err := c.Get(ctx, "/api/AcademicYears", nil)
	if err != nil {
		return assistResolutionData{}, err
	}
	years, err := assistRecordList(yearsRaw)
	if err != nil {
		return assistResolutionData{}, fmt.Errorf("decode ASSIST academic years response: %w", err)
	}

	return assistResolutionData{institutions: institutions, academicYears: years}, nil
}

func (d assistResolutionData) resolve(institution, year string) (assistResolvedContext, error) {
	resolvedInstitution, err := resolveAssistInstitution(d.institutions, institution)
	if err != nil {
		return assistResolvedContext{}, err
	}
	resolvedYear, err := resolveAssistAcademicYear(d.academicYears, year)
	if err != nil {
		return assistResolvedContext{}, err
	}
	return assistResolvedContext{Institution: resolvedInstitution, AcademicYear: resolvedYear}, nil
}

type assistMatchTerm struct {
	value    string
	priority int
}

type assistMatch struct {
	id       int64
	record   json.RawMessage
	priority int
}

func resolveAssistInstitution(records []json.RawMessage, query string) (assistResolvedRecord, error) {
	queryKey := normalizeAssistLabel(query)
	if queryKey == "" {
		return assistResolvedRecord{}, usageErr(fmt.Errorf("institution must contain at least one letter or number"))
	}

	matches := make([]assistMatch, 0)
	for _, record := range records {
		obj, err := assistObject(record)
		if err != nil {
			return assistResolvedRecord{}, fmt.Errorf("decode institution record: %w", err)
		}
		id, ok := assistObjectInt(obj, "id", "institutionId", "institutionParentId")
		if !ok {
			continue
		}
		best := 0
		for _, term := range assistInstitutionTerms(obj) {
			if normalizeAssistLabel(term.value) == queryKey && term.priority > best {
				best = term.priority
			}
		}
		if best > 0 {
			matches = append(matches, assistMatch{id: id, record: cloneRaw(record), priority: best})
		}
	}

	return uniqueAssistMatch("institution", query, matches)
}

func resolveAssistAcademicYear(records []json.RawMessage, query string) (assistResolvedRecord, error) {
	queryKey := normalizeAssistLabel(query)
	if queryKey == "" {
		return assistResolvedRecord{}, usageErr(fmt.Errorf("academic year must contain at least one letter or number"))
	}

	matches := make([]assistMatch, 0)
	for _, record := range records {
		obj, err := assistObject(record)
		if err != nil {
			return assistResolvedRecord{}, fmt.Errorf("decode academic year record: %w", err)
		}
		id, ok := assistObjectInt(obj, "id", "academicYearId")
		if !ok {
			continue
		}
		best := 0
		for _, term := range assistAcademicYearTerms(obj, id) {
			if normalizeAssistLabel(term.value) == queryKey && term.priority > best {
				best = term.priority
			}
		}
		if best > 0 {
			matches = append(matches, assistMatch{id: id, record: cloneRaw(record), priority: best})
		}
	}

	return uniqueAssistMatch("academic year", query, matches)
}

func uniqueAssistMatch(kind, query string, matches []assistMatch) (assistResolvedRecord, error) {
	if len(matches) == 0 {
		return assistResolvedRecord{}, notFoundErr(fmt.Errorf("%s %q was not found in the live ASSIST reference data", kind, query))
	}
	best := 0
	for _, match := range matches {
		if match.priority > best {
			best = match.priority
		}
	}
	bestMatches := matches[:0]
	for _, match := range matches {
		if match.priority == best {
			bestMatches = append(bestMatches, match)
		}
	}
	if len(bestMatches) != 1 {
		ids := make([]int64, 0, len(bestMatches))
		for _, match := range bestMatches {
			ids = append(ids, match.id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		return assistResolvedRecord{}, usageErr(fmt.Errorf("%s %q is ambiguous; matching ASSIST ids: %s", kind, query, joinAssistIDs(ids)))
	}
	return assistResolvedRecord{ID: bestMatches[0].id, Record: bestMatches[0].record}, nil
}

func assistInstitutionTerms(obj map[string]json.RawMessage) []assistMatchTerm {
	terms := make([]assistMatchTerm, 0, 8)
	if value, ok := assistObjectString(obj, "code"); ok {
		terms = append(terms, assistMatchTerm{value: value, priority: 4})
	}
	for _, key := range []string{"currentName", "displayName", "name", "description", "label"} {
		if value, ok := assistObjectString(obj, key); ok {
			terms = append(terms, assistMatchTerm{value: value, priority: 3})
		}
	}

	if raw, ok := assistLookup(obj, "names"); ok {
		terms = append(terms, assistNameTerms(raw)...)
	}
	return terms
}

func assistNameTerms(raw json.RawMessage) []assistMatchTerm {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '"' {
		var value string
		if json.Unmarshal(raw, &value) == nil {
			return []assistMatchTerm{{value: value, priority: 3}}
		}
		return nil
	}
	if raw[0] == '[' {
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return nil
		}
		type namedItem struct {
			name              string
			fromYear          int64
			hasYear           bool
			explicitlyCurrent bool
		}
		parsed := make([]namedItem, 0, len(items))
		var maxFromYear int64
		for _, item := range items {
			if len(item) > 0 && item[0] == '"' {
				var value string
				if json.Unmarshal(item, &value) == nil {
					parsed = append(parsed, namedItem{name: value, explicitlyCurrent: true})
				}
				continue
			}
			obj, err := assistObject(item)
			if err != nil {
				continue
			}
			name, ok := assistObjectString(obj, "name", "currentName", "displayName", "description", "label")
			if !ok {
				continue
			}
			fromYear, hasYear := assistObjectInt(obj, "fromYear", "startYear", "beginYear")
			if hasYear && fromYear > maxFromYear {
				maxFromYear = fromYear
			}
			current, _ := assistObjectBool(obj, "isCurrent", "current", "active", "isActive")
			parsed = append(parsed, namedItem{name: name, fromYear: fromYear, hasYear: hasYear, explicitlyCurrent: current})
		}
		terms := make([]assistMatchTerm, 0, len(parsed))
		for _, item := range parsed {
			priority := 2
			if item.explicitlyCurrent || item.hasYear && item.fromYear == maxFromYear {
				priority = 3
			}
			terms = append(terms, assistMatchTerm{value: item.name, priority: priority})
		}
		return terms
	}
	if raw[0] == '{' {
		obj, err := assistObject(raw)
		if err != nil {
			return nil
		}
		terms := make([]assistMatchTerm, 0)
		for key, value := range obj {
			lowerKey := strings.ToLower(key)
			priority := 2
			if strings.Contains(lowerKey, "current") || strings.Contains(lowerKey, "display") || lowerKey == "name" {
				priority = 3
			}
			if scalar, ok := assistRawString(value); ok {
				terms = append(terms, assistMatchTerm{value: scalar, priority: priority})
				continue
			}
			for _, nested := range assistNameTerms(value) {
				if nested.priority > priority {
					nested.priority = priority
				}
				terms = append(terms, nested)
			}
		}
		return terms
	}
	return nil
}

func assistAcademicYearTerms(obj map[string]json.RawMessage, id int64) []assistMatchTerm {
	terms := []assistMatchTerm{{value: strconv.FormatInt(id, 10), priority: 4}}
	if fallYear, ok := assistObjectInt(obj, "fallYear"); ok {
		terms = append(terms,
			assistMatchTerm{value: strconv.FormatInt(fallYear, 10), priority: 3},
			assistMatchTerm{value: fmt.Sprintf("%d-%d", fallYear, fallYear+1), priority: 4},
		)
	}
	for _, key := range []string{"code", "description", "name", "displayName", "label"} {
		if value, ok := assistObjectString(obj, key); ok {
			terms = append(terms, assistMatchTerm{value: value, priority: 4})
		}
	}
	for _, key := range []string{"beginDate", "endDate", "startDate", "fromDate", "toDate"} {
		if value, ok := assistObjectString(obj, key); ok {
			terms = append(terms, assistMatchTerm{value: value, priority: 2})
			if len(value) >= 10 {
				terms = append(terms, assistMatchTerm{value: value[:10], priority: 2})
			}
		}
	}
	return terms
}

func assistRecordList(raw json.RawMessage) ([]json.RawMessage, error) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty JSON response")
	}
	if raw[0] == '[' {
		var records []json.RawMessage
		if err := json.Unmarshal(raw, &records); err != nil {
			return nil, err
		}
		return records, nil
	}
	if raw[0] != '{' {
		return nil, fmt.Errorf("expected a JSON array or object")
	}
	obj, err := assistObject(raw)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"result", "results", "data", "items", "value"} {
		if nested, ok := assistLookup(obj, key); ok && string(nested) != "null" {
			records, nestedErr := assistRecordList(nested)
			if nestedErr == nil {
				return records, nil
			}
		}
	}
	if _, ok := assistLookup(obj, "id"); ok {
		return []json.RawMessage{cloneRaw(raw)}, nil
	}
	if _, ok := assistLookup(obj, "institutionId"); ok {
		return []json.RawMessage{cloneRaw(raw)}, nil
	}
	return nil, fmt.Errorf("response object does not contain a recognized record collection")
}

func assistObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return obj, nil
}

func assistLookup(obj map[string]json.RawMessage, names ...string) (json.RawMessage, bool) {
	for _, name := range names {
		for key, value := range obj {
			if strings.EqualFold(key, name) {
				return value, true
			}
		}
	}
	return nil, false
}

func assistObjectString(obj map[string]json.RawMessage, names ...string) (string, bool) {
	raw, ok := assistLookup(obj, names...)
	if !ok {
		return "", false
	}
	return assistRawString(raw)
}

func assistRawString(raw json.RawMessage) (string, bool) {
	var value string
	if json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value) != "" {
		return value, true
	}
	return "", false
}

func assistObjectInt(obj map[string]json.RawMessage, names ...string) (int64, bool) {
	raw, ok := assistLookup(obj, names...)
	if !ok {
		return 0, false
	}
	return assistRawInt(raw)
}

func assistRawInt(raw json.RawMessage) (int64, bool) {
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		if value, err := strconv.ParseInt(number.String(), 10, 64); err == nil {
			return value, true
		}
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
		return value, err == nil
	}
	return 0, false
}

func assistObjectBool(obj map[string]json.RawMessage, names ...string) (bool, bool) {
	raw, ok := assistLookup(obj, names...)
	if !ok {
		return false, false
	}
	var value bool
	if json.Unmarshal(raw, &value) == nil {
		return value, true
	}
	return false, false
}

func normalizeAssistLabel(value string) string {
	var out strings.Builder
	spacePending := false
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if spacePending && out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteRune(r)
			spacePending = false
		} else {
			spacePending = out.Len() > 0
		}
	}
	return out.String()
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

func joinAssistIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return strings.Join(parts, ", ")
}

func assistNumericValues(raw json.RawMessage) []int64 {
	if value, ok := assistRawInt(raw); ok {
		return []int64{value}
	}
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '[' {
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return nil
		}
		values := make([]int64, 0, len(items))
		for _, item := range items {
			values = append(values, assistNumericValues(item)...)
		}
		return values
	}
	if raw[0] == '{' {
		obj, err := assistObject(raw)
		if err != nil {
			return nil
		}
		for _, key := range []string{"id", "academicYearId", "yearId", "value"} {
			if nested, ok := assistLookup(obj, key); ok {
				if values := assistNumericValues(nested); len(values) > 0 {
					return values
				}
			}
		}
	}
	return nil
}

func assistContainsID(values []int64, want int64) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
