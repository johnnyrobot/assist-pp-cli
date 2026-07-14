// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"encoding/json"
	"io"
)

// printJSONFilteredWithMeta is the provenance-aware output path for hand-built
// commands. It lives outside generated helpers.go so force regeneration cannot
// erase the novel-command source contract.
func printJSONFilteredWithMeta(w io.Writer, v any, flags *rootFlags, meta map[string]any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return printOutputWithFlagsMeta(w, json.RawMessage(raw), flags, meta)
}
