package scheduler

import (
	"path/filepath"
	"time"

	"github.com/anuelvs/mvforge/backend/internal/models"
	"gorm.io/gorm"
)

func ActivateTaskReservation(db *gorm.DB, ownerType string, ownerID uint, assetPath string, plan models.ExecutionPlan, workerName string) error {
	now := time.Now()
	values := BuildReservation(plan)
	reservation := models.TaskReservation{
		OwnerType: ownerType, OwnerID: ownerID, AssetKey: filepath.Clean(assetPath),
		State: ReservationStateActive, WorkerName: workerName,
		JobType: jsonString(values, "jobType"), Encoder: plan.SelectedEncoder,
		EncoderClass: jsonString(values, "encoderClass"), MemoryBytes: jsonInt64(values, "memoryBytes"),
		WorkspaceBytes: jsonInt64(values, "workspaceBytes"), LibraryBytes: jsonInt64(values, "libraryBytes"),
		AcquiredAt: &now,
	}
	return db.Where("owner_type = ? AND owner_id = ?", ownerType, ownerID).Assign(reservation).FirstOrCreate(&reservation).Error
}

func ReleaseTaskReservation(db *gorm.DB, ownerType string, ownerID uint) error {
	return db.Where("owner_type = ? AND owner_id = ?", ownerType, ownerID).Delete(&models.TaskReservation{}).Error
}
