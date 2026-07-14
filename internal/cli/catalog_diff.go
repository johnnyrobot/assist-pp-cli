// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"assist-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// pp:data-source local

type catalogDiffScope struct {
	InstitutionID int64  `json:"institutionId"`
	FromYearID    int64  `json:"fromYearId"`
	ToYearID      int64  `json:"toYearId"`
	Type          string `json:"type"`
}

type catalogDiffCounts struct {
	Added   int `json:"added"`
	Removed int `json:"removed"`
	Changed int `json:"changed"`
}

type catalogRecord struct {
	ID     string         `json:"id"`
	Record map[string]any `json:"record"`
}

type catalogRecordChange struct {
	ID      string         `json:"id"`
	Before  map[string]any `json:"before"`
	After   map[string]any `json:"after"`
	Changes jsonTreeDiff   `json:"changes"`
}

type catalogDiffResult struct {
	Source  string                `json:"source"`
	Scope   catalogDiffScope      `json:"scope"`
	Counts  catalogDiffCounts     `json:"counts"`
	Added   []catalogRecord       `json:"added"`
	Removed []catalogRecord       `json:"removed"`
	Changed []catalogRecordChange `json:"changed"`
}

func newNovelCatalogDiffCmd(flags *rootFlags) *cobra.Command {
	var flagInstitutionID int64
	var flagFromYearID int64
	var flagToYearID int64
	var flagType string
	var dbPath string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Report additions, removals, and changed catalog records across two synced academic years.",
		Long: `Compare two institution-scoped catalog snapshots from the local SQLite mirror.

The result is keyed by official stable domain identifiers and contains the
official records that were added or removed plus before/after records and
field-level JSON Pointer changes for modified entries. Internal mirror scope
metadata is never exposed. This command never falls back to the live API. Use
agreements diff to compare two complete published agreement payloads.`,
		Example:     `  assist-pp-cli catalog diff --institution-id 113 --from-year-id 75 --to-year-id 76 --type courses --agent`,
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if len(args) != 0 {
				return usageErr(fmt.Errorf("catalog diff accepts flags only; unexpected argument %q", args[0]))
			}
			if flagInstitutionID <= 0 {
				return usageErr(fmt.Errorf("--institution-id must be a positive integer"))
			}
			if flagFromYearID <= 0 {
				return usageErr(fmt.Errorf("--from-year-id must be a positive integer"))
			}
			if flagToYearID <= 0 {
				return usageErr(fmt.Errorf("--to-year-id must be a positive integer"))
			}
			flagType = strings.ToLower(strings.TrimSpace(flagType))
			switch flagType {
			case "departments", "prefixes", "courses":
			default:
				return usageErr(fmt.Errorf("--type must be one of departments, prefixes, or courses"))
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return usageErr(err)
			}
			if dryRunOK(flags) {
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			if dbPath == "" {
				dbPath = defaultDBPath("assist-pp-cli")
			}
			if _, err := os.Stat(dbPath); err != nil {
				if os.IsNotExist(err) {
					fmt.Fprintf(cmd.ErrOrStderr(), "hint: local mirror not found at %s. Run 'assist-pp-cli sync' for both catalog years first.\n", dbPath)
					return notFoundErr(fmt.Errorf("local catalog mirror not found"))
				}
				return fmt.Errorf("checking local database: %w", err)
			}

			db, err := store.OpenReadOnlyContext(ctx, dbPath)
			if err != nil {
				return fmt.Errorf("opening local database read-only: %w", err)
			}
			defer db.Close()
			for _, year := range []int64{flagFromYearID, flagToYearID} {
				snapshot, snapshotErr := db.GetAssistCatalogSnapshot(flagType, strconv.FormatInt(flagInstitutionID, 10), strconv.FormatInt(year, 10))
				if snapshotErr != nil {
					if store.IsMissingAssistCatalogSnapshot(snapshotErr) {
						fmt.Fprintf(cmd.ErrOrStderr(), "hint: sync %s for institution %d and academic year %d before comparing snapshots.\n", flagType, flagInstitutionID, year)
						return notFoundErr(fmt.Errorf("%s snapshot for institution %d and academic year %d was not synced", flagType, flagInstitutionID, year))
					}
					return fmt.Errorf("checking scoped snapshot state: %w", snapshotErr)
				}
				if flags.maxAge > 0 && time.Since(snapshot.LastSyncedAt) > flags.maxAge {
					fmt.Fprintf(cmd.ErrOrStderr(), "hint: %s snapshot for institution %d and academic year %d is stale (last synced %s).\n", flagType, flagInstitutionID, year, snapshot.LastSyncedAt.UTC().Format(time.RFC3339))
				}
			}

			before, err := loadCatalogSnapshot(ctx, db.DB(), flagType, flagInstitutionID, flagFromYearID)
			if err != nil {
				return fmt.Errorf("loading %s snapshot for academic year %d: %w", flagType, flagFromYearID, err)
			}
			after, err := loadCatalogSnapshot(ctx, db.DB(), flagType, flagInstitutionID, flagToYearID)
			if err != nil {
				return fmt.Errorf("loading %s snapshot for academic year %d: %w", flagType, flagToYearID, err)
			}

			result := compareCatalogSnapshots(flagType, flagInstitutionID, flagFromYearID, flagToYearID, before, after)
			return printJSONFilteredWithMeta(cmd.OutOrStdout(), result, flags, map[string]any{"source": "local"})
		},
	}
	cmd.Flags().Int64Var(&flagInstitutionID, "institution-id", 0, "ASSIST institution ID shared by both snapshots")
	cmd.Flags().Int64Var(&flagFromYearID, "from-year-id", 0, "Earlier ASSIST academic year ID")
	cmd.Flags().Int64Var(&flagToYearID, "to-year-id", 0, "Later ASSIST academic year ID")
	cmd.Flags().StringVar(&flagType, "type", "", "Catalog resource: departments, prefixes, or courses")
	cmd.Flags().StringVar(&dbPath, "db", "", "SQLite database file path (default: resolved data directory data.db)")
	for _, name := range []string{"institution-id", "from-year-id", "to-year-id", "type"} {
		flag := cmd.Flags().Lookup(name)
		if flag.Annotations == nil {
			flag.Annotations = map[string][]string{}
		}
		flag.Annotations["mcp:required"] = []string{"true"}
	}
	return cmd
}

func loadCatalogSnapshot(ctx context.Context, db *sql.DB, resourceType string, institutionID, academicYearID int64) (map[string]map[string]any, error) {
	scope := strconv.FormatInt(institutionID, 10) + ":" + strconv.FormatInt(academicYearID, 10)
	rows, err := db.QueryContext(ctx, `SELECT id, data FROM resources
		WHERE resource_type = ? AND json_valid(data) AND json_extract(data, '$._assistScope') = ? ORDER BY id`, resourceType, scope)
	if err != nil {
		return nil, err
	}

	result := make(map[string]map[string]any)
	for rows.Next() {
		var storageID, raw string
		if err := rows.Scan(&storageID, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		_ = storageID // Scope and identity come from JSON, never storage-key heuristics.
		record, err := decodeCatalogObject(json.RawMessage(raw))
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("decoding mirrored record: %w", err)
		}
		if !catalogRecordMatchesScope(record, institutionID, academicYearID) {
			continue
		}
		clean := cleanCatalogRecord(record)
		id := stableCatalogRecordID(resourceType, clean)
		if id == "" {
			rows.Close()
			return nil, fmt.Errorf("scoped record has no stable domain identifier")
		}
		if _, exists := result[id]; exists {
			rows.Close()
			return nil, fmt.Errorf("duplicate stable domain identifier %q within one snapshot", id)
		}
		result[id] = clean
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return result, nil
}

func decodeCatalogObject(raw json.RawMessage) (map[string]any, error) {
	var record map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&record); err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("expected a JSON object")
	}
	return record, nil
}

func catalogRecordMatchesScope(record map[string]any, institutionID, academicYearID int64) bool {
	institution := firstCatalogScalar(record,
		"_assistInstitutionId", "institutionId", "institutionID", "institutionParentId")
	year := firstCatalogScalar(record,
		"_assistAcademicYearId", "academicYearId", "academicYearID")
	if scope, ok := record["_assistScope"].(map[string]any); ok {
		if institution == "" {
			institution = firstCatalogScalar(scope, "institutionId", "institutionID")
		}
		if year == "" {
			year = firstCatalogScalar(scope, "academicYearId", "academicYearID")
		}
	}
	return institution == strconv.FormatInt(institutionID, 10) && year == strconv.FormatInt(academicYearID, 10)
}

func firstCatalogScalar(record map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			if scalar := catalogScalarString(value); scalar != "" {
				return scalar
			}
		}
	}
	return ""
}

func catalogScalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return ""
	}
}

func cleanCatalogRecord(record map[string]any) map[string]any {
	clean := make(map[string]any, len(record))
	for key, value := range record {
		if strings.HasPrefix(key, "_assist") {
			continue
		}
		clean[key] = value
	}
	return clean
}

func stableCatalogRecordID(resourceType string, record map[string]any) string {
	var keys []string
	switch resourceType {
	case "courses":
		keys = []string{"courseIdentifierParentId", "id", "parentId"}
	case "departments":
		keys = []string{"parentId", "id"}
	case "prefixes":
		keys = []string{"prefixParentId", "parentId", "id"}
	}
	return firstCatalogScalar(record, keys...)
}

func compareCatalogSnapshots(resourceType string, institutionID, fromYearID, toYearID int64, before, after map[string]map[string]any) catalogDiffResult {
	result := catalogDiffResult{
		Source: "local",
		Scope: catalogDiffScope{
			InstitutionID: institutionID, FromYearID: fromYearID, ToYearID: toYearID, Type: resourceType,
		},
		Added: []catalogRecord{}, Removed: []catalogRecord{}, Changed: []catalogRecordChange{},
	}

	keys := make([]string, 0, len(before)+len(after))
	seen := make(map[string]struct{}, len(before)+len(after))
	for key := range before {
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	for key := range after {
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		beforeRecord, beforeOK := before[key]
		afterRecord, afterOK := after[key]
		switch {
		case !beforeOK:
			result.Added = append(result.Added, catalogRecord{ID: key, Record: afterRecord})
		case !afterOK:
			result.Removed = append(result.Removed, catalogRecord{ID: key, Record: beforeRecord})
		default:
			changes := diffJSONTrees(beforeRecord, afterRecord)
			if len(changes.Added)+len(changes.Removed)+len(changes.Changed) > 0 {
				result.Changed = append(result.Changed, catalogRecordChange{
					ID: key, Before: beforeRecord, After: afterRecord, Changes: changes,
				})
			}
		}
	}
	result.Counts = catalogDiffCounts{
		Added: len(result.Added), Removed: len(result.Removed), Changed: len(result.Changed),
	}
	return result
}
