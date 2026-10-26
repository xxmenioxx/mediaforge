package nasobserver

import (
	"bytes"
	"encoding/json"
	"io"
	"errors"
	"fmt"
	"strings"
	"time"
)

const maxJellyfinSessionsBytes = 1 << 20
const maxJellyfinSessions = 512

// AttributedSignal is a separately observed fact, not cryptographic
// authentication. The owning adapter must independently establish the
// source's authority; caller-controlled labels cannot establish trust.
type AttributedSignal struct {
	SourceIdentity string
	ObservedAt     time.Time
	Known          bool
	Value          bool
}

// CriticalEvidence describes four independently sourced facts. A healthy
// storage signal is true; the three activity signals are true when busy.
type CriticalEvidence struct {
	JellyfinPlayback AttributedSignal
	FFmpegActivity   AttributedSignal
	WorkerActivity   AttributedSignal
	StorageHealthy   AttributedSignal
}

type CriticalSourceIdentities struct {
	Jellyfin string
	FFmpeg  string
	Worker  string
	Storage string
}

// GuardFromAttributedEvidence enforces identity and freshness before calling
// the existing policy-neutral guard aggregation. This does not attest a
// source identity or authorize provider access, signing or admission.
func GuardFromAttributedEvidence(e CriticalEvidence, expected CriticalSourceIdentities, guardSource string, now time.Time, maximumAge time.Duration) Critical {
	result := Critical{SourceIdentity: guardSource, Status: "unavailable", ObservedAt: now}
	if !validIdentity(guardSource) || now.IsZero() || maximumAge <= 0 || maximumAge > 5*time.Minute {
		return result
	}
	signals := []struct {
		signal   AttributedSignal
		expected string
	}{
		{e.JellyfinPlayback, expected.Jellyfin},
		{e.FFmpegActivity, expected.FFmpeg},
		{e.WorkerActivity, expected.Worker},
		{e.StorageHealthy, expected.Storage},
	}
	for _, entry := range signals {
		if !validIdentity(entry.expected) || !entry.signal.Known ||
			entry.signal.SourceIdentity != entry.expected ||
			entry.signal.ObservedAt.IsZero() ||
			entry.signal.ObservedAt.After(now.Add(5*time.Second)) ||
			now.Sub(entry.signal.ObservedAt) > maximumAge {
			return result
		}
	}
	return GuardFromSignals(ServiceSignals{
		JellyfinPlaybackActive: boolPointer(e.JellyfinPlayback.Value),
		FFmpegActive:           boolPointer(e.FFmpegActivity.Value),
		MVForgeWorkerActive:    boolPointer(e.WorkerActivity.Value),
		StorageHealthy:         boolPointer(e.StorageHealthy.Value),
	}, guardSource, now)
}

func boolPointer(value bool) *bool { return &value }

// ParseJellyfinSessions consumes a separately supplied, complete Jellyfin
// /Sessions JSON response. Presence of any NowPlayingItem is treated as busy,
// including paused items. An empty list is idle ONLY if the upstream adapter
// separately authenticates the endpoint, authorization scope, response
// completeness and freshness. This parser does not contact Jellyfin.
func ParseJellyfinSessions(content []byte) (bool, error) {
	if len(content) == 0 || len(content) > maxJellyfinSessionsBytes {
		return false, errors.New("Jellyfin sessions data missing or oversized")
	}
	if err := validateUniqueJSON(content); err != nil {
		return false, fmt.Errorf("invalid Jellyfin sessions JSON: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	var sessions []json.RawMessage
	if err := decoder.Decode(&sessions); err != nil || sessions == nil ||
		len(sessions) > maxJellyfinSessions {
		return false, errors.New("Jellyfin sessions must be a bounded array")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return false, errors.New("trailing Jellyfin sessions data")
	}
	busy := false
	for _, session := range sessions {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(session, &fields); err != nil || fields == nil {
			return false, errors.New("malformed Jellyfin session")
		}
		for key := range fields {
			if strings.EqualFold(key, "NowPlayingItem") && key != "NowPlayingItem" {
				return false, errors.New("noncanonical NowPlayingItem key")
			}
		}
		if raw, ok := fields["NowPlayingItem"]; ok {
			trimmed := bytes.TrimSpace(raw)
			if !bytes.Equal(trimmed, []byte("null")) {
				if len(trimmed) == 0 || trimmed[0] != '{' {
					return false, errors.New("invalid NowPlayingItem")
				}
				var item map[string]json.RawMessage
				if err := json.Unmarshal(raw, &item); err != nil || item == nil {
					return false, errors.New("invalid NowPlayingItem")
				}
				busy = true
			}
		}
	}
	return busy, nil
}
