package encodingpolicy

import (
	"math"
	"strconv"
	"strings"
)

// VideoToolboxBitrate returns the effective offline bitrate controls for a
// named quality preset. Both the FFmpeg command builder and scheduler use this
// function so estimates cannot drift from execution.
func VideoToolboxBitrate(preset string, sourceBitrate int64, sourceHeight int, filters string) (targetKbps, maxrateKbps, bufferKbps int, ok bool) {
	preset = strings.ToLower(strings.TrimSpace(preset))
	multiplier, known := map[string]float64{"compact": .40, "medium": .52, "recommended": .65, "best_quality": .80, "high_quality": .95}[preset]
	if !known || sourceBitrate <= 0 {
		return 0, 0, 0, false
	}
	height := outputHeight(filters, sourceHeight)
	adjustment := 1.0
	if height > 0 && height <= 576 {
		adjustment = 1.075
	}
	target := int(math.Ceil(float64(sourceBitrate) * multiplier * adjustment / 1000))
	if floor := bitrateFloor(preset, height); target < floor {
		target = floor
	}
	if ceiling := bitrateCeiling(preset, height); ceiling > 0 && target > ceiling {
		target = ceiling
	}
	return target, int(math.Ceil(float64(target) * 1.5)), int(math.Ceil(float64(target) * 2.5)), true
}

func outputHeight(filters string, sourceHeight int) int {
	for _, part := range strings.Split(filters, ",") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(part, "crop=") {
			continue
		}
		values := strings.Split(strings.TrimPrefix(part, "crop="), ":")
		if len(values) >= 2 {
			if height, err := strconv.Atoi(values[1]); err == nil && height > 0 {
				return height
			}
		}
	}
	return sourceHeight
}

func bitrateFloor(preset string, height int) int {
	values := map[string][]int{
		"compact": {1500, 2200, 3000, 6000}, "medium": {2000, 3000, 4000, 8000},
		"recommended": {2500, 4000, 5000, 10000}, "best_quality": {3200, 5000, 6000, 12000}, "high_quality": {4000, 6500, 7000, 14000},
	}
	index := 2
	if height > 0 && height <= 576 {
		index = 0
	} else if height > 0 && height <= 720 {
		index = 1
	} else if height > 1080 {
		index = 3
	}
	return values[preset][index]
}

func bitrateCeiling(preset string, height int) int {
	if height <= 0 || height > 576 {
		return 0
	}
	return map[string]int{"compact": 2500, "medium": 3200, "recommended": 4000, "best_quality": 5000, "high_quality": 6000}[preset]
}
