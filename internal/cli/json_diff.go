// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
)

// jsonValueDelta describes a value that exists on only one side of a JSON
// comparison. Path is an RFC 6901 JSON Pointer; the empty string identifies
// the document root.
type jsonValueDelta struct {
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// jsonValueChange describes a value that exists on both sides but differs.
// Container values are recursively compared, so Before and After are always
// the smallest differing scalar or type-mismatched subtree.
type jsonValueChange struct {
	Path   string `json:"path"`
	Before any    `json:"before"`
	After  any    `json:"after"`
}

type jsonTreeDiff struct {
	Added   []jsonValueDelta  `json:"added"`
	Removed []jsonValueDelta  `json:"removed"`
	Changed []jsonValueChange `json:"changed"`
}

// diffJSONTrees returns a deterministic structural diff. Object keys are
// visited lexicographically. Arrays are intentionally positional: common
// indices are compared recursively and a trailing length difference is
// represented as added or removed indices. This does not claim set semantics
// for arrays whose elements happen to look alike.
func diffJSONTrees(before, after any) jsonTreeDiff {
	d := jsonTreeDiff{
		Added:   []jsonValueDelta{},
		Removed: []jsonValueDelta{},
		Changed: []jsonValueChange{},
	}
	diffJSONValue(&d, "", before, after)
	return d
}

func diffJSONValue(d *jsonTreeDiff, path string, before, after any) {
	beforeObject, beforeIsObject := before.(map[string]any)
	afterObject, afterIsObject := after.(map[string]any)
	if beforeIsObject && afterIsObject {
		keys := make([]string, 0, len(beforeObject)+len(afterObject))
		seen := make(map[string]struct{}, len(beforeObject)+len(afterObject))
		for key := range beforeObject {
			seen[key] = struct{}{}
			keys = append(keys, key)
		}
		for key := range afterObject {
			if _, ok := seen[key]; !ok {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			beforeValue, beforeOK := beforeObject[key]
			afterValue, afterOK := afterObject[key]
			childPath := joinJSONPointer(path, key)
			switch {
			case !beforeOK:
				d.Added = append(d.Added, jsonValueDelta{Path: childPath, Value: afterValue})
			case !afterOK:
				d.Removed = append(d.Removed, jsonValueDelta{Path: childPath, Value: beforeValue})
			default:
				diffJSONValue(d, childPath, beforeValue, afterValue)
			}
		}
		return
	}

	beforeArray, beforeIsArray := before.([]any)
	afterArray, afterIsArray := after.([]any)
	if beforeIsArray && afterIsArray {
		common := len(beforeArray)
		if len(afterArray) < common {
			common = len(afterArray)
		}
		for i := 0; i < common; i++ {
			diffJSONValue(d, joinJSONPointer(path, strconv.Itoa(i)), beforeArray[i], afterArray[i])
		}
		for i := common; i < len(beforeArray); i++ {
			d.Removed = append(d.Removed, jsonValueDelta{Path: joinJSONPointer(path, strconv.Itoa(i)), Value: beforeArray[i]})
		}
		for i := common; i < len(afterArray); i++ {
			d.Added = append(d.Added, jsonValueDelta{Path: joinJSONPointer(path, strconv.Itoa(i)), Value: afterArray[i]})
		}
		return
	}

	if !reflect.DeepEqual(before, after) {
		d.Changed = append(d.Changed, jsonValueChange{Path: path, Before: before, After: after})
	}
}

func joinJSONPointer(parent, token string) string {
	token = strings.ReplaceAll(token, "~", "~0")
	token = strings.ReplaceAll(token, "/", "~1")
	return parent + "/" + token
}
