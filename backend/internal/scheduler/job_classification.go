package scheduler

import "fmt"

type JobType string
type JobWeight string

const (
	JobTypeVideoConversion  JobType = "video_conversion"
	JobTypeAudioRestoration JobType = "audio_restoration"
	JobTypeLabPreview       JobType = "lab_preview"
	JobTypeAnalysis         JobType = "analysis"
	JobTypeValidation       JobType = "validation"
	JobTypePublishing       JobType = "publishing"
	JobTypeCleanup          JobType = "cleanup"
)

const (
	JobWeightLight  JobWeight = "light"
	JobWeightMedium JobWeight = "medium"
	JobWeightHeavy  JobWeight = "heavy"
)

type JobClassification struct {
	Type                  JobType   `json:"type"`
	Weight                JobWeight `json:"weight"`
	RequiresWorkingWindow bool      `json:"requiresWorkingWindow"`
}

func ClassifyJob(jobType JobType) (JobClassification, error) {
	switch jobType {
	case JobTypeVideoConversion, JobTypeAudioRestoration:
		return JobClassification{Type: jobType, Weight: JobWeightHeavy, RequiresWorkingWindow: true}, nil
	case JobTypeLabPreview, JobTypeAnalysis, JobTypeValidation, JobTypePublishing, JobTypeCleanup:
		return JobClassification{Type: jobType, Weight: JobWeightLight, RequiresWorkingWindow: false}, nil
	default:
		return JobClassification{}, fmt.Errorf("unknown scheduler job type %q", jobType)
	}
}
