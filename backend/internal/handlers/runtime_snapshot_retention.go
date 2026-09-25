package handlers

import (
	"fmt"
	"strings"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const (
	runtimeSnapshotRetentionAge       = 7 * 24 * time.Hour
	runtimeSnapshotRetentionBatchSize = 100
)

// RuntimeSnapshotRetentionReport describes the database-only part of a
// housekeeping run. It deliberately has no filesystem-size field: deleting
// SQLite rows does not imply that SQLite returned those bytes to the host.
type RuntimeSnapshotRetentionReport struct {
	RowsConsidered         int64    `json:"rowsConsidered"`
	ReferencedKept         int64    `json:"referencedKept"`
	LatestKept             int64    `json:"latestKept"`
	RecentUnreferencedKept int64    `json:"recentUnreferencedKept"`
	PruneCandidates        int64    `json:"pruneCandidates"`
	Deleted                int64    `json:"deleted"`
	Errors                 []string `json:"errors"`
}

// RuntimeSnapshotReferenceProvider describes one canonical provenance owner.
// The registry turns these indexed foreign-key-like columns into EXISTS
// predicates, so housekeeping never materializes the owner's full history in
// Go memory. New provenance owners belong here, not in pruning code.
type RuntimeSnapshotReferenceProvider struct {
	Table  string
	Column string
}

// RuntimeSnapshotReferenceRegistry centralizes all reference discovery used by
// retention. The default registry covers every plan and Test Encode row,
// regardless of lifecycle status.
type RuntimeSnapshotReferenceRegistry struct {
	providers []RuntimeSnapshotReferenceProvider
}

func NewRuntimeSnapshotReferenceRegistry(extra ...RuntimeSnapshotReferenceProvider) RuntimeSnapshotReferenceRegistry {
	providers := []RuntimeSnapshotReferenceProvider{
		{Table: "execution_plans", Column: "runtime_snapshot_id"},
		{Table: "test_encodes", Column: "runtime_snapshot_id"},
	}
	providers = append(providers, extra...)
	return RuntimeSnapshotReferenceRegistry{providers: providers}
}

// ReferencedRuntimeSnapshotIDs is a compatibility/debugging helper for
// callers that explicitly need the complete set. Housekeeping does not call
// it; retention uses indexed candidate-scoped EXISTS queries instead.
func ReferencedRuntimeSnapshotIDs(db *gorm.DB) (map[uint]struct{}, error) {
	return NewRuntimeSnapshotReferenceRegistry().IDs(db)
}

func (r RuntimeSnapshotReferenceRegistry) IDs(db *gorm.DB) (map[uint]struct{}, error) {
	ids := make(map[uint]struct{})
	for _, provider := range r.providers {
		if provider.Table == "" || provider.Column == "" || !db.Migrator().HasTable(provider.Table) {
			continue
		}
		var provided []uint
		err := db.Table(provider.Table).
			Where(provider.Column+" IS NOT NULL").
			Pluck(provider.Column, &provided).Error
		if err != nil {
			return nil, err
		}
		for _, id := range provided {
			if id != 0 {
				ids[id] = struct{}{}
			}
		}
	}
	return ids, nil
}

func (r RuntimeSnapshotReferenceRegistry) referencedPredicate(db *gorm.DB, snapshotAlias string) string {
	clauses := make([]string, 0, len(r.providers))
	for index, provider := range r.providers {
		if provider.Table == "" || provider.Column == "" || !db.Migrator().HasTable(provider.Table) {
			continue
		}
		alias := fmt.Sprintf("runtime_ref_%d", index)
		clauses = append(clauses, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM %s AS %s WHERE %s.%s = %s.id)",
			provider.Table, alias, alias, provider.Column, snapshotAlias,
		))
	}
	if len(clauses) == 0 {
		return "0 = 1"
	}
	return "(" + strings.Join(clauses, " OR ") + ")"
}

func (r RuntimeSnapshotReferenceRegistry) unreferencedPredicate(db *gorm.DB, snapshotAlias string) string {
	return "NOT " + r.referencedPredicate(db, snapshotAlias)
}

func (r RuntimeSnapshotReferenceRegistry) isReferenced(db *gorm.DB, snapshotID uint) (bool, error) {
	var exists int
	query := db.Model(&models.RuntimeSnapshot{}).
		Select("1").
		Where("id = ?", snapshotID).
		Where(r.referencedPredicate(db, "runtime_snapshots"))
	if err := query.Limit(1).Scan(&exists).Error; err != nil {
		return false, err
	}
	return exists == 1, nil
}

type runtimeSnapshotRetentionCandidate struct {
	ID         uint
	DetectedAt time.Time
}

type runtimeSnapshotRetentionPlan struct {
	RuntimeSnapshotRetentionReport
	candidates []runtimeSnapshotRetentionCandidate
}

func resolveRuntimeSnapshotRetention(db *gorm.DB, now time.Time, registry RuntimeSnapshotReferenceRegistry) (runtimeSnapshotRetentionPlan, error) {
	plan := runtimeSnapshotRetentionPlan{RuntimeSnapshotRetentionReport: RuntimeSnapshotRetentionReport{Errors: []string{}}}
	if !db.Migrator().HasTable(&models.RuntimeSnapshot{}) {
		return plan, nil
	}

	if err := db.Model(&models.RuntimeSnapshot{}).Count(&plan.RowsConsidered).Error; err != nil {
		return plan, err
	}
	if plan.RowsConsidered == 0 {
		return plan, nil
	}

	var latest struct {
		ID uint
	}
	latestResult := db.Model(&models.RuntimeSnapshot{}).
		Select("id").
		Order("detected_at DESC").
		Order("id DESC").
		Limit(1).
		Find(&latest)
	if latestResult.Error != nil {
		return plan, latestResult.Error
	}
	if latestResult.RowsAffected > 0 {
		plan.LatestKept = 1
	}

	var err error
	plan.ReferencedKept, err = countReferencedRuntimeSnapshots(db, registry, "")
	if err != nil {
		return plan, err
	}

	cutoff := now.Add(-runtimeSnapshotRetentionAge)
	var recentUnreferenced int64
	if err := db.Model(&models.RuntimeSnapshot{}).
		Where("detected_at > ?", cutoff).
		Count(&recentUnreferenced).Error; err != nil {
		return plan, err
	}
	recentReferenced, err := countReferencedRuntimeSnapshots(db, registry, "detected_at > ?", cutoff)
	if err != nil {
		return plan, err
	}
	plan.RecentUnreferencedKept = maxInt64(0, recentUnreferenced-recentReferenced)

	plan.candidates, err = findRuntimeSnapshotCandidates(db, registry, latest.ID, cutoff)
	if err != nil {
		return plan, err
	}
	plan.PruneCandidates = int64(len(plan.candidates))
	return plan, nil
}

func countReferencedRuntimeSnapshots(db *gorm.DB, registry RuntimeSnapshotReferenceRegistry, condition string, args ...any) (int64, error) {
	var count int64
	query := db.Model(&models.RuntimeSnapshot{}).Where(registry.referencedPredicate(db, "runtime_snapshots"))
	if condition != "" {
		query = query.Where(condition, args...)
	}
	return count, query.Count(&count).Error
}

func findRuntimeSnapshotCandidates(db *gorm.DB, registry RuntimeSnapshotReferenceRegistry, latestID uint, cutoff time.Time) ([]runtimeSnapshotRetentionCandidate, error) {
	candidates := make([]runtimeSnapshotRetentionCandidate, 0, runtimeSnapshotRetentionBatchSize)
	var rows []runtimeSnapshotRetentionCandidate
	query := db.Model(&models.RuntimeSnapshot{}).
		Select("id, detected_at").
		Where("detected_at <= ?", cutoff).
		Where("id <> ?", latestID).
		Where(registry.unreferencedPredicate(db, "runtime_snapshots"))
	if err := query.Order("detected_at ASC").Order("id ASC").Limit(runtimeSnapshotRetentionBatchSize).Find(&rows).Error; err != nil {
		return nil, err
	}
	candidates = append(candidates, rows...)
	return candidates, nil
}

// pruneRuntimeSnapshots resolves candidates from current committed state. A
// normal run repeats that resolution inside its write transaction, then
// rechecks references and the newest-row guard immediately before deletion.
func pruneRuntimeSnapshots(db *gorm.DB, now time.Time, dryRun bool) (RuntimeSnapshotRetentionReport, error) {
	registry := NewRuntimeSnapshotReferenceRegistry()
	if dryRun {
		plan, err := resolveRuntimeSnapshotRetention(db, now, registry)
		return plan.RuntimeSnapshotRetentionReport, err
	}

	var report RuntimeSnapshotRetentionReport
	err := db.Transaction(func(tx *gorm.DB) error {
		plan, err := resolveRuntimeSnapshotRetention(tx, now, registry)
		if err != nil {
			return err
		}
		cutoff := now.Add(-runtimeSnapshotRetentionAge)
		for _, candidate := range plan.candidates {
			latestID, err := newestRuntimeSnapshotID(tx)
			if err != nil {
				return err
			}
			if candidate.ID == latestID {
				continue
			}
			referenced, err := registry.isReferenced(tx, candidate.ID)
			if err != nil {
				return err
			}
			if referenced {
				continue
			}
			result := tx.Where("id = ? AND detected_at <= ?", candidate.ID, cutoff).Delete(&models.RuntimeSnapshot{})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				plan.Deleted++
			}
		}
		report = plan.RuntimeSnapshotRetentionReport
		return nil
	})
	return report, err
}

func newestRuntimeSnapshotID(db *gorm.DB) (uint, error) {
	if !db.Migrator().HasTable(&models.RuntimeSnapshot{}) {
		return 0, nil
	}
	var latest struct {
		ID uint
	}
	result := db.Model(&models.RuntimeSnapshot{}).
		Select("id").
		Order("detected_at DESC").
		Order("id DESC").
		Limit(1).
		Find(&latest)
	return latest.ID, result.Error
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
