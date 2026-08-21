package models

import "time"

// TestEncode is a short, real conversion produced for playback testing. It is
// intentionally independent from QueueJob and never participates in publish or
// original-archive lifecycle transitions.
type TestEncode struct {
	ID                     uint       `json:"id" gorm:"primaryKey"`
	SourceAssetID          uint       `json:"sourceAssetId" gorm:"index"`
	SourcePath             string     `json:"sourcePath" gorm:"not null;index"`
	SourceFingerprint      string     `json:"sourceFingerprint"`
	SourceSizeBytes        int64      `json:"sourceSizeBytes"`
	SourceModifiedAt       *time.Time `json:"sourceModifiedAt"`
	LibraryID              uint       `json:"libraryId" gorm:"not null;index"`
	ConfigurationSource    string     `json:"configurationSource" gorm:"not null;index"`
	RequestedConfiguration JSONMap    `json:"requestedConfiguration" gorm:"type:json"`
	EffectiveConfiguration JSONMap    `json:"effectiveConfiguration" gorm:"type:json"`
	ConfigurationHash      string     `json:"configurationHash" gorm:"not null;index"`
	ProfileID              uint       `json:"profileId"`
	ProfileVersion         int        `json:"profileVersion"`
	RuntimeSnapshotID      *uint      `json:"runtimeSnapshotId" gorm:"index"`
	WorkerName             string     `json:"workerName"`
	EffectiveEncoder       string     `json:"effectiveEncoder"`
	StartSeconds           float64    `json:"startSeconds"`
	DurationSeconds        int        `json:"durationSeconds"`
	Status                 string     `json:"status" gorm:"not null;index"`
	Phase                  string     `json:"phase" gorm:"not null;index"`
	Progress               int        `json:"progress"`
	FFmpegCommand          string     `json:"ffmpegCommand" gorm:"column:ffmpeg_command"`
	OutputPath             string     `json:"outputPath" gorm:"index"`
	TemporaryPath          string     `json:"temporaryPath"`
	OutputSizeBytes        int64      `json:"outputSizeBytes"`
	SubtitleArtifacts      JSONList   `json:"subtitleArtifacts" gorm:"type:json"`
	ValidationReport       JSONMap    `json:"validationReport" gorm:"type:json"`
	Keep                   bool       `json:"keep" gorm:"not null;default:false"`
	ExpiresAt              *time.Time `json:"expiresAt" gorm:"index"`
	ErrorMessage           string     `json:"errorMessage"`
	Stale                  bool       `json:"stale" gorm:"-"`
	StaleReason            string     `json:"staleReason,omitempty" gorm:"-"`
	StartedAt              *time.Time `json:"startedAt"`
	CompletedAt            *time.Time `json:"completedAt"`
	CanceledAt             *time.Time `json:"canceledAt"`
	DeletedAt              *time.Time `json:"deletedAt" gorm:"index"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

// TaskReservation lets non-Queue execution types participate in the same
// resource accounting without borrowing QueueJob lifecycle semantics.
type TaskReservation struct {
	ID             uint       `json:"id" gorm:"primaryKey"`
	OwnerType      string     `json:"ownerType" gorm:"not null;uniqueIndex:idx_task_reservation_owner"`
	OwnerID        uint       `json:"ownerId" gorm:"not null;uniqueIndex:idx_task_reservation_owner"`
	AssetKey       string     `json:"assetKey" gorm:"not null;index"`
	State          string     `json:"state" gorm:"not null;index"`
	WorkerName     string     `json:"workerName"`
	JobType        string     `json:"jobType"`
	Encoder        string     `json:"encoder"`
	EncoderClass   string     `json:"encoderClass"`
	MemoryBytes    int64      `json:"memoryBytes"`
	WorkspaceBytes int64      `json:"workspaceBytes"`
	LibraryBytes   int64      `json:"libraryBytes"`
	AcquiredAt     *time.Time `json:"acquiredAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}
