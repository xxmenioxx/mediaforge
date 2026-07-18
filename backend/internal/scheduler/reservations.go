package scheduler

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/anuelvs/mediaforge/backend/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrAssetAlreadyReserved = errors.New("asset already has an active scheduler reservation")

const (
	ReservationStateLocked = "locked"
	ReservationStateActive = "active"
)

func LockQueuedAsset(db *gorm.DB, job models.QueueJob) error {
	reservation := models.SchedulerReservation{JobID: job.ID, AssetKey: filepath.Clean(job.MediaPath), State: ReservationStateLocked}
	if err := db.Create(&reservation).Error; err != nil {
		return ErrAssetAlreadyReserved
	}
	return nil
}

func UpdateAssetLock(db *gorm.DB, job models.QueueJob) error {
	result := db.Model(&models.SchedulerReservation{}).Where("job_id = ?", job.ID).Update("asset_key", filepath.Clean(job.MediaPath))
	if result.Error != nil {
		return ErrAssetAlreadyReserved
	}
	if result.RowsAffected == 0 {
		return LockQueuedAsset(db, job)
	}
	return nil
}

func ActivateReservation(db *gorm.DB, job models.QueueJob, plan models.ExecutionPlan, workerName string) error {
	var reservation models.SchedulerReservation
	result := db.Where("job_id = ?", job.ID).Limit(1).Find(&reservation)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		if err := LockQueuedAsset(db, job); err != nil {
			return err
		}
		if err := db.Where("job_id = ?", job.ID).First(&reservation).Error; err != nil {
			return err
		}
	}
	now := time.Now()
	values := map[string]any{
		"state": ReservationStateActive, "worker_name": workerName, "encoder": plan.SelectedEncoder, "encoder_class": jsonString(plan.Reservation, "encoderClass"),
		"memory_bytes": jsonInt64(plan.Reservation, "memoryBytes"), "workspace_bytes": jsonInt64(plan.Reservation, "workspaceBytes"),
		"library_bytes": jsonInt64(plan.Reservation, "libraryBytes"), "acquired_at": &now,
	}
	return db.Model(&reservation).Updates(values).Error
}

func DeactivateReservationResources(db *gorm.DB, jobID uint) error {
	return db.Model(&models.SchedulerReservation{}).Where("job_id = ?", jobID).Updates(map[string]any{
		"state": ReservationStateLocked, "worker_name": "", "encoder": "", "encoder_class": "", "memory_bytes": 0, "workspace_bytes": 0, "library_bytes": 0, "acquired_at": nil,
	}).Error
}

func ReleaseReservation(db *gorm.DB, jobID uint) error {
	return db.Where("job_id = ?", jobID).Delete(&models.SchedulerReservation{}).Error
}

func ReconcileReservations(db *gorm.DB) error {
	if err := db.Where("job_id NOT IN (?)", db.Model(&models.QueueJob{}).Select("id")).Delete(&models.SchedulerReservation{}).Error; err != nil {
		return err
	}
	var jobs []models.QueueJob
	if err := db.Where("status <> ? AND published_at IS NULL", "canceled").Find(&jobs).Error; err != nil {
		return err
	}
	for _, job := range jobs {
		reservation := models.SchedulerReservation{JobID: job.ID, AssetKey: filepath.Clean(job.MediaPath), State: ReservationStateLocked}
		if job.Status == "running" {
			reservation.State = ReservationStateActive
			reservation.WorkerName = job.WorkerName
		}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&reservation).Error; err != nil {
			return err
		}
	}
	return db.Where("job_id IN (?)", db.Model(&models.QueueJob{}).Select("id").Where("status = ? OR published_at IS NOT NULL", "canceled")).Delete(&models.SchedulerReservation{}).Error
}

func jsonString(value models.JSONMap, key string) string {
	typed, _ := value[key].(string)
	return typed
}
func jsonInt64(value models.JSONMap, key string) int64 {
	switch typed := value[key].(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case int:
		return int64(typed)
	}
	return 0
}
