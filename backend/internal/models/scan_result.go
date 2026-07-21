package models

import "time"

type ScanResult struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	Path              string    `json:"path" gorm:"not null;index"`
	FileName          string    `json:"fileName" gorm:"not null"`
	Container         string    `json:"container"`
	SizeBytes         int64     `json:"sizeBytes"`
	Duration          float64   `json:"duration"`
	Bitrate           int64     `json:"bitrate"`
	VideoCodec        string    `json:"videoCodec"`
	Width             int       `json:"width"`
	Height            int       `json:"height"`
	HDR               bool      `json:"hdr"`
	AudioTracks       int       `json:"audioTracks"`
	SubtitleTracks    int       `json:"subtitleTracks"`
	Chapters          int       `json:"chapters"`
	VideoStreams      JSONList  `json:"videoStreams" gorm:"type:json"`
	AudioStreams      JSONList  `json:"audioStreams" gorm:"type:json"`
	SubtitleStreams   JSONList  `json:"subtitleStreams" gorm:"type:json"`
	InterlaceAnalysis JSONMap   `json:"interlaceAnalysis" gorm:"type:json"`
	RawProbe          JSONMap   `json:"rawProbe" gorm:"type:json"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}
