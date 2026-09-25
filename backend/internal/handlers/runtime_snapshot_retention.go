package handlers

import (
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

const (
	runtimeSnapshotRetentionAge       = 7 * 24 * time.Hour
	runtimeSnapshotRetentionBatchSize = 100
	runtimeSnapshotReferenceBatchSize = 500
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

// RuntimeSnapshotReferenceProvider is a canonical source of immutable
// RuntimeSnapshot provenance references. New provenance owners can be added
// to the registry without changing pruning or deletion logic.
type RuntimeSnapshotReferenceProvider func(*gorm.DB) ([]uint, error)

// RuntimeSnapshotReferenceRegistry centralizes all reference discovery used by
// retention. The default registry covers every plan and Test Encode row,
// regardless of lifecycle status.
type RuntimeSnapshotReferenceRegistry struct {
	providers []RuntimeSnapshotReferenceProvider
}

func NewRuntimeSnapshotReferenceRegistry(extra ...RuntimeSnapshotReferenceProvider) RuntimeSnapshotReferenceRegistry {
	providers := []RuntimeSnapshotReferenceProvider{
		executionPlanRuntimeSnapshotReferences,
		testEncodeRuntimeSnapshotReferences,
	}
	providers = append(providers, extra...)
	return RuntimeSnapshotReferenceRegistry{providers: providers}
}

// ReferencedRuntimeSnapshotIDs returns the distinct immutable snapshot IDs
// protected from pruning. It is the only reference-discovery boundary used by
// runtime snapshot retention.
func ReferencedRuntimeSnapshotIDs(db *gorm.DB) (map[uint]struct{}, error) {
	return NewRuntimeSnapshotReferenceRegistry().IDs(db)
}

func (r RuntimeSnapshotReferenceRegistry) IDs(db *gorm.DB) (map[uint]struct{}, error) {
	ids := make(map[uint]struct{})
	for _, provider := range r.providers {
		if provider == nil {
			continue
		}
		provided, err := provider(db)
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

func executionPlanRuntimeSnapshotReferences(db *gorm.DB) ([]uint, error) {
	if !db.Migrator().HasTable(&models.ExecutionPlan{}) {
		return nil, nil
	}
	var plans []struct {
		RuntimeSnapshotID uint `gorm:"column:runtime_snapshot_id"`
	}
	err := db.Table("execution_plans").
		Where("runtime_snapshot_id IS NOT NULL").
		Find(&plans).Error
	ids := make([]uint, 0, len(plans))
	for _, plan := range plans {
		ids = append(ids, plan.RuntimeSnapshotID)
	}
	return ids, err
}

func testEncodeRuntimeSnapshotReferences(db *gorm.DB) ([]uint, error) {
	if !db.Migrator().HasTable(&models.TestEncode{}) {
		return nil, nil
	}
	var encodes []struct {
		RuntimeSnapshotID uint `gorm:"column:runtime_snapshot_id"`
	}
	err := db.Table("test_encodes").
		Where("runtime_snapshot_id IS NOT NULL").
		Find(&encodes).Error
	ids := make([]uint, 0, len(encodes))
	for _, encode := range encodes {
		ids = append(ids, encode.RuntimeSnapshotID)
	}
	return ids, err
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

	references, err := registry.IDs(db)
	if err != nil {
		return plan, err
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

	plan.ReferencedKept, err = countReferencedRuntimeSnapshots(db, references, "")
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
	recentReferenced, err := countReferencedRuntimeSnapshots(db, references, "detected_at > ?", cutoff)
	if err != nil {
		return plan, err
	}
	plan.RecentUnreferencedKept = maxInt64(0, recentUnreferenced-recentReferenced)

	plan.candidates, err = findRuntimeSnapshotCandidates(db, references, latest.ID, cutoff)
	if err != nil {
		return plan, err
	}
	plan.PruneCandidates = int64(len(plan.candidates))
	return plan, nil
}

func countReferencedRuntimeSnapshots(db *gorm.DB, references map[uint]struct{}, condition string, args ...any) (int64, error) {
	if len(references) == 0 {
		return 0, nil
	}
	ids := make([]uint, 0, len(references))
	for id := range references {
		ids = append(ids, id)
	}
	var count int64
	for start := 0; start < len(ids); start += runtimeSnapshotReferenceBatchSize {
		end := minInt(start+runtimeSnapshotReferenceBatchSize, len(ids))
		query := db.Model(&models.RuntimeSnapshot{}).Where("id IN ?", ids[start:end])
		if condition != "" {
			query = query.Where(condition, args...)
		}
		var batch int64
		if err := query.Count(&batch).Error; err != nil {
			return 0, err
		}
		count += batch
	}
	return count, nil
}

func findRuntimeSnapshotCandidates(db *gorm.DB, references map[uint]struct{}, latestID uint, cutoff time.Time) ([]runtimeSnapshotRetentionCandidate, error) {
	candidates := make([]runtimeSnapshotRetentionCandidate, 0, runtimeSnapshotRetentionBatchSize)
	var lastDetectedAt time.Time
	var lastID uint
	for len(candidates) < runtimeSnapshotRetentionBatchSize {
		var rows []runtimeSnapshotRetentionCandidate
		query := db.Model(&models.RuntimeSnapshot{}).
			Select("id, detected_at").
			Where("detected_at <= ?", cutoff)
		if !lastDetectedAt.IsZero() {
			query = query.Where("(detected_at > ? OR (detected_at = ? AND id > ?))", lastDetectedAt, lastDetectedAt, lastID)
		}
		if err := query.Order("detected_at ASC").Order("id ASC").Limit(runtimeSnapshotReferenceBatchSize).Find(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			lastDetectedAt = row.DetectedAt
			lastID = row.ID
			if row.ID == latestID {
				continue
			}
			if _, referenced := references[row.ID]; referenced {
				continue
			}
			candidates = append(candidates, row)
			if len(candidates) == runtimeSnapshotRetentionBatchSize {
				break
			}
		}
		if len(rows) < runtimeSnapshotReferenceBatchSize {
			break
		}
	}
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
		references, err := registry.IDs(tx)
		if err != nil {
			return err
		}
		latestID, err := newestRuntimeSnapshotID(tx)
		if err != nil {
			return err
		}
		cutoff := now.Add(-runtimeSnapshotRetentionAge)
		for _, candidate := range plan.candidates {
			if candidate.ID == latestID {
				continue
			}
			if _, referenced := references[candidate.ID]; referenced {
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
