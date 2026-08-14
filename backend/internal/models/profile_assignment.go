package models

import "time"

// ProfileAssignment binds one media-profile dimension to an asset or to the
// canonical asset group shown by Assets. Selection=disabled is an explicit
// inheritance stop and is therefore different from a missing assignment.
type ProfileAssignment struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	TargetType     string    `json:"targetType" gorm:"not null;uniqueIndex:idx_profile_assignment_target"`
	TargetPath     string    `json:"targetPath" gorm:"not null;uniqueIndex:idx_profile_assignment_target"`
	MediaType      string    `json:"mediaType" gorm:"not null;uniqueIndex:idx_profile_assignment_target"`
	Selection      string    `json:"selection" gorm:"not null;default:profile"`
	VideoProfileID uint      `json:"videoProfileId,omitempty" gorm:"index"`
	ProfileKey     string    `json:"profileKey,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
