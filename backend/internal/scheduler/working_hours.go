package scheduler

import (
	"fmt"
	"time"
)

type WorkingHoursConfig struct {
	Enabled             bool                `json:"enabled"`
	Timezone            string              `json:"timezone"`
	Preset              string              `json:"preset"`
	Windows             []ConversionWindow  `json:"windows"`
	OutsideWindowPolicy OutsideWindowPolicy `json:"outsideWindowPolicy"`
}

type ConversionWindow struct {
	Name  string   `json:"name"`
	Days  []string `json:"days"`
	Start string   `json:"start"`
	End   string   `json:"end"`
}

type OutsideWindowPolicy struct {
	StartNewHeavyJobs   bool `json:"startNewHeavyJobs"`
	ContinueRunningJobs bool `json:"continueRunningJobs"`
	AllowAnalysis       bool `json:"allowAnalysisJobs"`
	AllowValidation     bool `json:"allowValidationJobs"`
	AllowPublisher      bool `json:"allowPublisherJobs"`
	AllowCleanup        bool `json:"allowCleanupJobs"`
	AllowLabPreviews    bool `json:"allowLabPreviews"`
}

type WorkingHoursDecision struct {
	Allowed         bool       `json:"allowed"`
	InWindow        bool       `json:"inWindow"`
	Reason          string     `json:"reason"`
	NextWindowStart *time.Time `json:"nextWindowStart,omitempty"`
}

func EvaluateWorkingHours(
	config WorkingHoursConfig,
	now time.Time,
	requiresWindow bool,
) (WorkingHoursDecision, error) {

	if !config.Enabled {
		return WorkingHoursDecision{
			Allowed:  true,
			InWindow: true,
			Reason:   "Working hours are disabled",
		}, nil
	}

	if !requiresWindow {
		return WorkingHoursDecision{
			Allowed:  true,
			InWindow: false,
			Reason:   "Job does not require a conversion window",
		}, nil
	}

	location, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return WorkingHoursDecision{}, fmt.Errorf(
			"invalid working hours timezone %q: %w",
			config.Timezone,
			err,
		)
	}

	localNow := now.In(location)

	for _, window := range config.Windows {
		inWindow, err := IsWithinConversionWindow(window, localNow)
		if err != nil {
			return WorkingHoursDecision{}, fmt.Errorf(
				"invalid conversion window %q: %w",
				window.Name,
				err,
			)
		}

		if inWindow {
			return WorkingHoursDecision{
				Allowed:  true,
				InWindow: true,
				Reason:   "Inside conversion window: " + window.Name,
			}, nil
		}
	}

	if config.OutsideWindowPolicy.StartNewHeavyJobs {
		return WorkingHoursDecision{
			Allowed:  true,
			InWindow: false,
			Reason:   "Outside conversion window, but heavy jobs are allowed",
		}, nil
	}

	return WorkingHoursDecision{
		Allowed:         false,
		InWindow:        false,
		Reason:          "Waiting for an allowed conversion window",
		NextWindowStart: nextWindowStart(config, localNow),
	}, nil

}

func nextWindowStart(config WorkingHoursConfig, localNow time.Time) *time.Time {
	var earliest *time.Time
	for offset := 0; offset <= 7; offset++ {
		day := localNow.AddDate(0, 0, offset)
		dayKey := weekdayKey(day.Weekday())
		for _, window := range config.Windows {
			startMinutes, err := parseClockMinutes(window.Start)
			if err != nil {
				continue
			}
			matches := false
			for _, configuredDay := range window.Days {
				if configuredDay == dayKey {
					matches = true
					break
				}
			}
			if !matches {
				continue
			}
			candidate := time.Date(day.Year(), day.Month(), day.Day(), startMinutes/60, startMinutes%60, 0, 0, localNow.Location())
			if !candidate.After(localNow) {
				continue
			}
			if earliest == nil || candidate.Before(*earliest) {
				copy := candidate
				earliest = &copy
			}
		}
	}
	return earliest
}

func parseClockMinutes(value string) (int, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return 0, err
	}

	return parsed.Hour()*60 + parsed.Minute(), nil
}

func validWeekday(value string) bool {
	switch value {
	case "mon", "tue", "wed", "thu", "fri", "sat", "sun":
		return true
	default:
		return false
	}
}

func weekdayKey(value time.Weekday) string {
	switch value {
	case time.Monday:
		return "mon"
	case time.Tuesday:
		return "tue"
	case time.Wednesday:
		return "wed"
	case time.Thursday:
		return "thu"
	case time.Friday:
		return "fri"
	case time.Saturday:
		return "sat"
	default:
		return "sun"
	}
}

func DefaultWorkingHoursConfig() WorkingHoursConfig {

	return WorkingHoursConfig{
		Enabled:  false,
		Timezone: "America/Mexico_City",
		Preset:   "disabled",

		// Se inicializa explícitamente para que JSON devuelva []
		// en lugar de null.
		Windows: []ConversionWindow{},

		OutsideWindowPolicy: OutsideWindowPolicy{
			StartNewHeavyJobs:   true,
			ContinueRunningJobs: true,

			// Todas las tareas ligeras permanecen permitidas.
			AllowAnalysis:    true,
			AllowValidation:  true,
			AllowPublisher:   true,
			AllowCleanup:     true,
			AllowLabPreviews: true,
		},
	}
}

func IsWithinConversionWindow(window ConversionWindow, now time.Time) (bool, error) {
	startMinutes, err := parseClockMinutes(window.Start)
	if err != nil {
		return false, fmt.Errorf("invalid start time: %w", err)
	}

	endMinutes, err := parseClockMinutes(window.End)
	if err != nil {
		return false, fmt.Errorf("invalid end time: %w", err)
	}

	configuredDays := make(map[string]bool, len(window.Days))
	for _, day := range window.Days {
		if !validWeekday(day) {
			return false, fmt.Errorf("invalid weekday %q", day)
		}
		configuredDays[day] = true
	}

	currentDay := weekdayKey(now.Weekday())
	previousDay := weekdayKey(now.AddDate(0, 0, -1).Weekday())
	currentMinutes := now.Hour()*60 + now.Minute()

	switch {
	case startMinutes < endMinutes:
		// Ventana normal: 09:00–17:00.
		return configuredDays[currentDay] &&
			currentMinutes >= startMinutes &&
			currentMinutes < endMinutes, nil

	case startMinutes > endMinutes:
		// Cruza medianoche: 23:00–07:00.
		startedToday := configuredDays[currentDay] &&
			currentMinutes >= startMinutes

		startedYesterday := configuredDays[previousDay] &&
			currentMinutes < endMinutes

		return startedToday || startedYesterday, nil

	default:
		// Start == End representa 24 horas desde el día configurado.
		startedToday := configuredDays[currentDay] &&
			currentMinutes >= startMinutes

		startedYesterday := configuredDays[previousDay] &&
			currentMinutes < startMinutes

		return startedToday || startedYesterday, nil
	}
}
