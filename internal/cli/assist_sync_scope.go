// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/url"
	"strings"
)

var assistScopedSyncResources = map[string]bool{
	"courses":     true,
	"departments": true,
	"prefixes":    true,
}

// resolveAssistSyncPath lets the framework's existing --resource-param flags
// fill ASSIST's institution/year path parameters. If either value is absent,
// the path is left untouched so the framework emits its standard actionable
// unfilled-path warning instead of silently choosing a scope.
func resolveAssistSyncPath(resource, path string, params *syncUserParams) (string, map[string]string) {
	if !assistScopedSyncResources[resource] || params == nil {
		return path, nil
	}
	values := map[string]string{}
	params.applyTo(resource, values, false)
	institutionID := strings.TrimSpace(values["institutionId"])
	academicYearID := strings.TrimSpace(values["academicYearId"])
	if institutionID == "" || academicYearID == "" {
		return path, nil
	}
	path = strings.ReplaceAll(path, "{institutionId}", url.PathEscape(institutionID))
	path = strings.ReplaceAll(path, "{academicYearId}", url.PathEscape(academicYearID))
	return path, map[string]string{
		"institutionId":  institutionID,
		"academicYearId": academicYearID,
	}
}

// annotateAssistSyncItems preserves the partition that came from the request
// path. These underscore-prefixed fields are local provenance, not claims
// about the upstream payload. The store uses _assistScope in its composite
// key so identical API IDs from two catalog years do not overwrite each other.
func annotateAssistSyncItems(resource string, items []json.RawMessage, scope map[string]string) []json.RawMessage {
	if !assistScopedSyncResources[resource] || len(scope) == 0 {
		return items
	}
	result := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		var object map[string]any
		if err := json.Unmarshal(item, &object); err != nil || object == nil {
			result = append(result, item)
			continue
		}
		institutionID := scope["institutionId"]
		academicYearID := scope["academicYearId"]
		object["_assistInstitutionId"] = institutionID
		object["_assistAcademicYearId"] = academicYearID
		object["_assistScope"] = institutionID + ":" + academicYearID
		encoded, err := json.Marshal(object)
		if err != nil {
			result = append(result, item)
			continue
		}
		result = append(result, encoded)
	}
	return result
}
