package handlers

import "testing"

func TestTerminalStageForStatus(t *testing.T) {
	cases := map[string]string{JobStatusQueued: JobStageQueued, JobStatusRunning: JobStageConverting, JobStatusCompleted: JobStageReadyToPublish, JobStatusFailed: JobStageFailed, JobStatusCanceled: JobStageCanceled}
	for status, expected := range cases {
		if actual := terminalStageForStatus(status); actual != expected {
			t.Fatalf("status %s: expected %s, got %s", status, expected, actual)
		}
	}
}
