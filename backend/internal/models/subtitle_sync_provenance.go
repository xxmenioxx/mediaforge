package models

import "time"

// SubtitleSyncProvenance stores bounded evidence for one applied subtitle
// synchronization. Subtitle text and raw engine logs are intentionally absent.
type SubtitleSyncProvenance struct {
	OperationID               string    `json:"operationId" gorm:"primaryKey;size:128"`
	AssetPath                 string    `json:"assetPath" gorm:"not null;size:2048;index"`
	SubtitlePath              string    `json:"subtitlePath" gorm:"not null;size:2048;index"`
	SourceIdentity            string    `json:"sourceIdentity" gorm:"not null;size:512"`
	SourceRevision            string    `json:"sourceRevision" gorm:"not null;size:512"`
	ReferenceAudioStreamIndex int       `json:"referenceAudioStreamIndex" gorm:"not null"`
	ReferenceAudioStreamID    string    `json:"referenceAudioStreamId" gorm:"size:128"`
	ReferenceAudioCodec       string    `json:"referenceAudioCodec" gorm:"not null;size:128"`
	ReferenceAudioLanguage    string    `json:"referenceAudioLanguage" gorm:"size:64"`
	ReferenceAudioTitle       string    `json:"referenceAudioTitle" gorm:"size:512"`
	Engine                    string    `json:"engine" gorm:"not null;size:64"`
	EngineVersion             string    `json:"engineVersion" gorm:"not null;size:64"`
	Model                     string    `json:"model" gorm:"not null;size:32"`
	OffsetMilliseconds        int64     `json:"offsetMilliseconds" gorm:"not null"`
	Scale                     float64   `json:"scale" gorm:"not null"`
	SegmentsJSON              string    `json:"segmentsJson" gorm:"not null;type:text"`
	QualityState              string    `json:"qualityState" gorm:"not null;size:32"`
	AnalysisAt                time.Time `json:"analysisAt" gorm:"not null"`
	AppliedAt                 time.Time `json:"appliedAt" gorm:"not null;index"`
}
