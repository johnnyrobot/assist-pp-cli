// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/johnnyrobot/assist-pp-cli/internal/client"
	"github.com/johnnyrobot/assist-pp-cli/internal/store"
)

// resolveAssistScopedCatalogRead preserves ASSIST's institution/year partition
// for both local reads and auto-mode write-through caching. The generic resolver
// cannot infer path scope and would otherwise read every year or treat the final
// year path segment as a record ID.
func resolveAssistScopedCatalogRead(ctx context.Context, c *client.Client, flags *rootFlags, resourceType, institutionID, academicYearID, path string, params map[string]string, hintWriter io.Writer) (json.RawMessage, DataProvenance, error) {
	readLocal := func(reason string) (json.RawMessage, DataProvenance, error) {
		db, err := openStoreForRead(ctx, "assist-pp-cli")
		if err != nil {
			return nil, DataProvenance{}, fmt.Errorf("opening local database: %w", err)
		}
		if db == nil {
			return nil, DataProvenance{}, fmt.Errorf("no local data; sync %s for institution %s and academic year %s first", resourceType, institutionID, academicYearID)
		}
		defer db.Close()
		snapshot, err := db.GetAssistCatalogSnapshot(resourceType, institutionID, academicYearID)
		if err != nil {
			return nil, DataProvenance{}, fmt.Errorf("no local %s snapshot for institution %s and academic year %s; sync that scope first", resourceType, institutionID, academicYearID)
		}
		scope := institutionID + ":" + academicYearID
		rows, err := db.DB().QueryContext(ctx, `SELECT data FROM resources WHERE resource_type=? AND json_valid(data)
			AND json_extract(data, '$._assistScope')=? ORDER BY id`, resourceType, scope)
		if err != nil {
			return nil, DataProvenance{}, err
		}
		defer rows.Close()
		items := make([]map[string]any, 0, snapshot.TotalCount)
		for rows.Next() {
			var raw string
			if err := rows.Scan(&raw); err != nil {
				return nil, DataProvenance{}, err
			}
			var record map[string]any
			if err := json.Unmarshal([]byte(raw), &record); err != nil {
				return nil, DataProvenance{}, err
			}
			items = append(items, cleanCatalogRecord(record))
		}
		if err := rows.Err(); err != nil {
			return nil, DataProvenance{}, err
		}
		if len(params) > 0 {
			fmt.Fprintln(hintWriter, "warning: local ASSIST catalog data is scoped by institution/year, but endpoint filter flags are not reapplied")
		}
		data, err := json.Marshal(items)
		if err != nil {
			return nil, DataProvenance{}, err
		}
		syncedAt := snapshot.LastSyncedAt
		return data, attachFreshness(DataProvenance{Source: "local", Reason: reason, ResourceType: resourceType, SyncedAt: &syncedAt}, flags), nil
	}

	if flags.dataSource == "local" {
		return readLocal("user_requested")
	}
	data, err := c.GetWithHeaders(ctx, path, params, nil)
	if err != nil {
		if flags.dataSource == "live" || !isNetworkError(err) {
			return nil, DataProvenance{}, err
		}
		fallback, prov, localErr := readLocal(networkFallbackReason)
		if localErr != nil {
			return nil, DataProvenance{}, fmt.Errorf("API unreachable and scoped local data unavailable: %w", err)
		}
		return fallback, prov, nil
	}
	if c.DryRun || isDryRunResponse(data) {
		return data, attachFreshness(DataProvenance{Source: "live"}, flags), nil
	}
	if err := assertLiveJSONBody(data); err != nil {
		return nil, DataProvenance{}, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, DataProvenance{}, fmt.Errorf("ASSIST %s response is not an array: %w", resourceType, err)
	}
	if flags.dataSource != "live" {
		annotated := annotateAssistSyncItems(resourceType, items, map[string]string{"institutionId": institutionID, "academicYearId": academicYearID})
		if db, openErr := store.OpenWithContext(ctx, defaultDBPath("assist-pp-cli")); openErr == nil {
			_, _, _ = db.UpsertBatch(resourceType, annotated)
			_ = db.SaveAssistCatalogSnapshot(resourceType, institutionID, academicYearID, len(items))
			_ = db.Close()
		}
	}
	return data, attachFreshness(DataProvenance{Source: "live"}, flags), nil
}
