// Copyright 2026 johnnyrobot and contributors. Licensed under Apache-2.0. See LICENSE.
package store

import (
	"database/sql"
	"fmt"
	"time"
)

type AssistCatalogSnapshot struct {
	LastSyncedAt time.Time
	TotalCount   int
}

func (s *Store) SaveAssistCatalogSnapshot(resourceType, institutionID, academicYearID string, totalCount int) error {
	if resourceType == "" || institutionID == "" || academicYearID == "" {
		return fmt.Errorf("ASSIST catalog snapshot requires resource, institution, and academic year")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.db.Exec(`INSERT INTO assist_catalog_snapshots
		(resource_type, institution_id, academic_year_id, last_synced_at, total_count)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, ?)
		ON CONFLICT(resource_type, institution_id, academic_year_id) DO UPDATE SET
		last_synced_at=CURRENT_TIMESTAMP, total_count=excluded.total_count`,
		resourceType, institutionID, academicYearID, totalCount)
	return err
}

func (s *Store) GetAssistCatalogSnapshot(resourceType, institutionID, academicYearID string) (AssistCatalogSnapshot, error) {
	var snapshot AssistCatalogSnapshot
	var raw string
	err := s.db.QueryRow(`SELECT last_synced_at, total_count FROM assist_catalog_snapshots
		WHERE resource_type=? AND institution_id=? AND academic_year_id=?`,
		resourceType, institutionID, academicYearID).Scan(&raw, &snapshot.TotalCount)
	if err != nil {
		return snapshot, err
	}
	var parsed time.Time
	var parseErr error
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
		parsed, parseErr = time.Parse(layout, raw)
		if parseErr == nil {
			break
		}
	}
	if parseErr != nil {
		return snapshot, fmt.Errorf("parsing ASSIST snapshot timestamp %q: %w", raw, parseErr)
	}
	snapshot.LastSyncedAt = parsed
	return snapshot, nil
}

func IsMissingAssistCatalogSnapshot(err error) bool { return err == sql.ErrNoRows }
