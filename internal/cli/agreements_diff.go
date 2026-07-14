// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// pp:data-source live

type agreementDiffCounts struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Changed int `json:"changed"`
}

type agreementDiffKeys struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type agreementDiffResult struct {
	Keys    agreementDiffKeys   `json:"keys"`
	Source  string              `json:"source"`
	Counts  agreementDiffCounts `json:"counts"`
	Added   []jsonValueDelta    `json:"added"`
	Removed []jsonValueDelta    `json:"removed"`
	Changed []jsonValueChange   `json:"changed"`
}

func newNovelAgreementsDiffCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <from-key> <to-key>",
		Short: "Compare two official agreement payloads as deterministic added, removed, and changed JSON paths.",
		Long: `Compare two complete published ASSIST agreement payloads by their keys.

The command performs two live GET /api/articulation/Agreements requests and
reports additions, removals, and changes as RFC 6901 JSON Pointer paths. Object
keys are canonicalized lexicographically. Arrays use explicit positional
semantics: matching indices are compared and trailing indices are added or
removed. Use catalog diff for cross-year membership changes in a local mirror.`,
		Example:     `  assist-pp-cli agreements diff "KEY-2024" "KEY-2025" --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) != 2 {
				return usageErr(fmt.Errorf("agreements diff requires exactly two agreement keys\nUsage: %s <from-key> <to-key>", cmd.CommandPath()))
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return nil
			}

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			before, err := fetchAgreementResult(cmd, c, args[0])
			if err != nil {
				return classifyAPIError(err, flags)
			}
			after, err := fetchAgreementResult(cmd, c, args[1])
			if err != nil {
				return classifyAPIError(err, flags)
			}

			diff := diffJSONTrees(before, after)
			result := agreementDiffResult{
				Keys:   agreementDiffKeys{From: args[0], To: args[1]},
				Source: "live",
				Counts: agreementDiffCounts{
					Added: len(diff.Added), Removed: len(diff.Removed), Changed: len(diff.Changed),
				},
				Added: diff.Added, Removed: diff.Removed, Changed: diff.Changed,
			}
			return printJSONFilteredWithMeta(cmd.OutOrStdout(), result, flags, map[string]any{"source": "live"})
		},
	}
	return cmd
}

type agreementGetter interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
}

func fetchAgreementResult(cmd *cobra.Command, c agreementGetter, key string) (any, error) {
	raw, err := c.Get(cmd.Context(), "/api/articulation/Agreements", map[string]string{"Key": key})
	if err != nil {
		return nil, err
	}
	var envelope map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		return nil, fmt.Errorf("decoding agreement response: %w", err)
	}
	resultRaw, ok := envelope["result"]
	if !ok {
		return nil, fmt.Errorf("agreement response is missing the result envelope")
	}
	var result any
	decoder = json.NewDecoder(bytes.NewReader(resultRaw))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding agreement result: %w", err)
	}
	return expandAgreementJSONStrings(result), nil
}

// ASSIST serializes several nested agreement fields as JSON strings. Expand
// object/array-shaped strings recursively so diffs identify the actual changed
// articulation path instead of reporting one opaque string replacement.
func expandAgreementJSONStrings(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			typed[key] = expandAgreementJSONStrings(child)
		}
		return typed
	case []any:
		for i, child := range typed {
			typed[i] = expandAgreementJSONStrings(child)
		}
		return typed
	case string:
		trimmed := bytes.TrimSpace([]byte(typed))
		if len(trimmed) < 2 || (trimmed[0] != '{' && trimmed[0] != '[') {
			return typed
		}
		var nested any
		decoder := json.NewDecoder(bytes.NewReader(trimmed))
		decoder.UseNumber()
		if err := decoder.Decode(&nested); err != nil {
			return typed
		}
		return expandAgreementJSONStrings(nested)
	default:
		return value
	}
}
