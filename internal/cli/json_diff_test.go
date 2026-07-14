// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"reflect"
	"testing"
)

func TestDiffJSONTrees(t *testing.T) {
	tests := []struct {
		name   string
		before any
		after  any
		want   jsonTreeDiff
	}{
		{
			name: "nested objects arrays and escaped pointer tokens",
			before: map[string]any{
				"a/b":  map[string]any{"~gone": true, "same": "yes"},
				"list": []any{map[string]any{"value": "old"}, "remove"},
			},
			after: map[string]any{
				"a/b":  map[string]any{"new": 3, "same": "yes"},
				"list": []any{map[string]any{"value": "new"}, "remove", "add"},
			},
			want: jsonTreeDiff{
				Added: []jsonValueDelta{
					{Path: "/a~1b/new", Value: 3},
					{Path: "/list/2", Value: "add"},
				},
				Removed: []jsonValueDelta{{Path: "/a~1b/~0gone", Value: true}},
				Changed: []jsonValueChange{{Path: "/list/0/value", Before: "old", After: "new"}},
			},
		},
		{
			name:   "identical values yield non-nil empty slices",
			before: map[string]any{"same": []any{1, 2}},
			after:  map[string]any{"same": []any{1, 2}},
			want: jsonTreeDiff{
				Added:   []jsonValueDelta{},
				Removed: []jsonValueDelta{},
				Changed: []jsonValueChange{},
			},
		},
		{
			name:   "type mismatch is one honest subtree change",
			before: map[string]any{"value": map[string]any{"nested": true}},
			after:  map[string]any{"value": []any{true}},
			want: jsonTreeDiff{
				Added:   []jsonValueDelta{},
				Removed: []jsonValueDelta{},
				Changed: []jsonValueChange{{Path: "/value", Before: map[string]any{"nested": true}, After: []any{true}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diffJSONTrees(tt.before, tt.after)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("diffJSONTrees() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
