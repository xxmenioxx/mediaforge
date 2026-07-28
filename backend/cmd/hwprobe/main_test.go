package main

import "testing"

func TestClassifyHardwareFailures(t *testing.T) {
	tests := map[string]string{
		"Permission denied opening /dev/dri/renderD128":     "permission_denied",
		"Unknown encoder 'hevc_qsv'":                        "not_listed",
		"No such file or directory":                         "device_missing",
		"some encoding parameters are not supported by QSV": "feature_unsupported",
		"Error creating a MFX session":                      "runtime_incompatible",
		"unrelated failure":                                 "command_failed",
	}
	for message, expected := range tests {
		if actual := classify(message); actual != expected {
			t.Fatalf("classify(%q) = %q, expected %q", message, actual, expected)
		}
	}
}
