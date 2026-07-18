package scheduler

import "testing"

func TestClassifyJob(t *testing.T) {
	tests := []struct {
		jobType  JobType
		weight   JobWeight
		requires bool
	}{
		{JobTypeVideoConversion, JobWeightHeavy, true}, {JobTypeAudioRestoration, JobWeightHeavy, true},
		{JobTypeLabPreview, JobWeightLight, false}, {JobTypeAnalysis, JobWeightLight, false},
		{JobTypeValidation, JobWeightLight, false}, {JobTypePublishing, JobWeightLight, false}, {JobTypeCleanup, JobWeightLight, false},
	}
	for _, test := range tests {
		t.Run(string(test.jobType), func(t *testing.T) {
			got, err := ClassifyJob(test.jobType)
			if err != nil {
				t.Fatal(err)
			}
			if got.Type != test.jobType || got.Weight != test.weight || got.RequiresWorkingWindow != test.requires {
				t.Fatalf("unexpected classification: %#v", got)
			}
		})
	}
}

func TestClassifyJobRejectsUnknownType(t *testing.T) {
	if _, err := ClassifyJob("unknown"); err == nil {
		t.Fatal("expected unknown type error")
	}
}
