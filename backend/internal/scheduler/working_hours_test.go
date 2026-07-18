package scheduler

import (
	"testing"
	"time"
)

func TestDefaultWorkingHoursConfig(t *testing.T) {

	config := DefaultWorkingHoursConfig()

	if config.Enabled {
		t.Fatal("working hours should be disabled by default")
	}

	if config.Timezone != "America/Mexico_City" {
		t.Fatalf("unexpected timezone: %s", config.Timezone)
	}

	if !config.OutsideWindowPolicy.StartNewHeavyJobs {
		t.Fatal("heavy jobs should be allowed when working hours are disabled")
	}

	if len(config.Windows) != 0 {
		t.Fatalf("expected no default windows, got %d", len(config.Windows))
	}

	policy := config.OutsideWindowPolicy

	if !policy.AllowAnalysis ||
		!policy.AllowValidation ||
		!policy.AllowPublisher ||
		!policy.AllowCleanup ||
		!policy.AllowLabPreviews {
		t.Fatal("all light jobs should be allowed by default")
	}

	if !policy.ContinueRunningJobs {
		t.Fatal("running jobs should be allowed to continue by default")
	}

}

func TestIsWithinConversionWindow(t *testing.T) {
	monday := time.Date(2026, time.July, 13, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		window  ConversionWindow
		now     time.Time
		want    bool
		wantErr bool
	}{
		{
			name: "inside normal window",
			window: ConversionWindow{
				Days:  []string{"mon"},
				Start: "09:00",
				End:   "17:00",
			},
			now:  monday.Add(12 * time.Hour),
			want: true,
		},
		{
			name: "outside normal window",
			window: ConversionWindow{
				Days:  []string{"mon"},
				Start: "09:00",
				End:   "17:00",
			},
			now:  monday.Add(18 * time.Hour),
			want: false,
		},
		{
			name: "evening part of overnight window",
			window: ConversionWindow{
				Days:  []string{"mon"},
				Start: "23:00",
				End:   "07:00",
			},
			now:  monday.Add(23*time.Hour + 30*time.Minute),
			want: true,
		},
		{
			name: "morning part of previous window",
			window: ConversionWindow{
				Days:  []string{"mon"},
				Start: "23:00",
				End:   "07:00",
			},
			now:  monday.Add(26 * time.Hour),
			want: true,
		},
		{
			name: "after overnight window",
			window: ConversionWindow{
				Days:  []string{"mon"},
				Start: "23:00",
				End:   "07:00",
			},
			now:  monday.Add(32 * time.Hour),
			want: false,
		},
		{
			name: "invalid time",
			window: ConversionWindow{
				Days:  []string{"mon"},
				Start: "25:00",
				End:   "07:00",
			},
			now:     monday,
			wantErr: true,
		},
		{
			name: "invalid weekday",
			window: ConversionWindow{
				Days:  []string{"monday"},
				Start: "09:00",
				End:   "17:00",
			},
			now:     monday,
			wantErr: true,
		},
		{
			name: "inside 24 hour window on configured day",
			window: ConversionWindow{
				Days:  []string{"mon"},
				Start: "09:00",
				End:   "09:00",
			},
			now:  monday.Add(15 * time.Hour),
			want: true,
		},
		{
			name: "inside 24 hour window on following morning",
			window: ConversionWindow{
				Days:  []string{"mon"},
				Start: "09:00",
				End:   "09:00",
			},
			now:  monday.Add(26 * time.Hour),
			want: true,
		},
		{
			name: "after 24 hour window ended",
			window: ConversionWindow{
				Days:  []string{"mon"},
				Start: "09:00",
				End:   "09:00",
			},
			now:  monday.Add(34 * time.Hour),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := IsWithinConversionWindow(test.window, test.now)

			if test.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if got != test.want {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
}

func TestEvaluateWorkingHoursDisabled(t *testing.T) {
	config := DefaultWorkingHoursConfig()
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)

	decision, err := EvaluateWorkingHours(config, now, true)
	if err != nil {
		t.Fatal(err)
	}

	if !decision.Allowed {
		t.Fatal("disabled working hours should allow heavy jobs")
	}

	if !decision.InWindow {
		t.Fatal("disabled working hours should behave as inside a window")
	}

	if decision.Reason != "Working hours are disabled" {
		t.Fatalf("unexpected reason: %s", decision.Reason)
	}
}

func mexicoTime(t *testing.T, hour int) time.Time {
	t.Helper()

	location, err := time.LoadLocation("America/Mexico_City")
	if err != nil {
		t.Fatal(err)
	}

	return time.Date(2026, time.July, 13, hour, 0, 0, 0, location)
}

func enabledWorkingHoursConfig() WorkingHoursConfig {
	return WorkingHoursConfig{
		Enabled:  true,
		Timezone: "America/Mexico_City",
		Windows: []ConversionWindow{
			{
				Name:  "Weekday daytime",
				Days:  []string{"mon"},
				Start: "09:00",
				End:   "17:00",
			},
		},
		OutsideWindowPolicy: OutsideWindowPolicy{
			StartNewHeavyJobs: false,
		},
	}
}

func TestEvaluateWorkingHoursAllowsLightJob(t *testing.T) {
	config := enabledWorkingHoursConfig()

	decision, err := EvaluateWorkingHours(
		config,
		mexicoTime(t, 20),
		false,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !decision.Allowed {
		t.Fatal("light job should be allowed outside the window")
	}

	if decision.InWindow {
		t.Fatal("20:00 should be outside the configured window")
	}

	if decision.Reason != "Job does not require a conversion window" {
		t.Fatalf("unexpected reason: %s", decision.Reason)
	}
}
func TestEvaluateWorkingHoursAllowsHeavyJobInsideWindow(t *testing.T) {
	config := enabledWorkingHoursConfig()

	decision, err := EvaluateWorkingHours(
		config,
		mexicoTime(t, 12),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !decision.Allowed || !decision.InWindow {
		t.Fatalf("expected allowed in-window decision: %#v", decision)
	}

	if decision.Reason != "Inside conversion window: Weekday daytime" {
		t.Fatalf("unexpected reason: %s", decision.Reason)
	}
}
func TestEvaluateWorkingHoursBlocksHeavyJobOutsideWindow(t *testing.T) {
	config := enabledWorkingHoursConfig()

	decision, err := EvaluateWorkingHours(
		config,
		mexicoTime(t, 20),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	if decision.Allowed {
		t.Fatal("heavy job should be blocked outside the window")
	}

	if decision.InWindow {
		t.Fatal("20:00 should be outside the configured window")
	}

	if decision.Reason != "Waiting for an allowed conversion window" {
		t.Fatalf("unexpected reason: %s", decision.Reason)
	}
}
func TestEvaluateWorkingHoursAllowsHeavyJobOutsideWhenConfigured(t *testing.T) {
	config := enabledWorkingHoursConfig()
	config.OutsideWindowPolicy.StartNewHeavyJobs = true

	decision, err := EvaluateWorkingHours(
		config,
		mexicoTime(t, 20),
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !decision.Allowed {
		t.Fatal("policy should allow heavy jobs outside the window")
	}

	if decision.InWindow {
		t.Fatal("job should remain marked as outside the window")
	}

	if decision.Reason != "Outside conversion window, but heavy jobs are allowed" {
		t.Fatalf("unexpected reason: %s", decision.Reason)
	}
}
func TestEvaluateWorkingHoursRejectsInvalidTimezone(t *testing.T) {
	config := enabledWorkingHoursConfig()
	config.Timezone = "Invalid/Timezone"

	_, err := EvaluateWorkingHours(
		config,
		time.Now(),
		true,
	)
	if err == nil {
		t.Fatal("expected invalid timezone error")
	}
}

func TestEvaluateWorkingHoursPropagatesInvalidWindow(t *testing.T) {
	config := enabledWorkingHoursConfig()
	config.Windows[0].Start = "25:00"

	_, err := EvaluateWorkingHours(
		config,
		mexicoTime(t, 12),
		true,
	)
	if err == nil {
		t.Fatal("expected invalid window error")
	}
}

func TestEvaluateWorkingHoursReportsNextWindow(t *testing.T) {
	config := enabledWorkingHoursConfig()
	decision, err := EvaluateWorkingHours(config, mexicoTime(t, 20), true)
	if err != nil {
		t.Fatal(err)
	}
	if decision.NextWindowStart == nil {
		t.Fatal("expected next window start")
	}
	if got := decision.NextWindowStart.Format("2006-01-02 15:04"); got != "2026-07-20 09:00" {
		t.Fatalf("unexpected next window: %s", got)
	}
}
