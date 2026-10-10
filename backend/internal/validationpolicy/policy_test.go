package validationpolicy

import (
	"encoding/json"
	"testing"
)

func TestPresetNormalizationIsDeterministic(t *testing.T) {
	for _, preset := range []string{Disabled, Basic, Default, Strict, Custom} {
		t.Run(preset, func(t *testing.T) {
			factory := Factory(preset)
			encoded, err := json.Marshal(factory)
			if err != nil {
				t.Fatal(err)
			}
			var raw map[string]any
			if err := json.Unmarshal(encoded, &raw); err != nil {
				t.Fatal(err)
			}
			first := Normalize(raw)
			second := Normalize(raw)
			if Fingerprint(first) != Fingerprint(second) {
				t.Fatal("normalization fingerprint changed")
			}
			if first.SchemaVersion != SchemaVersion || first.PolicyVersion != PolicyVersion {
				t.Fatalf("unexpected policy versions: %#v", first)
			}
		})
	}
}

func TestDefaultIsFactoryPreset(t *testing.T) {
	policy := DefaultPolicy()
	if policy.Preset != Default {
		t.Fatalf("factory preset=%q, want default", policy.Preset)
	}
	if rule := policy.Rules["timeline.av_alignment"]; !rule.Enabled || rule.Thresholds["passFrames"] != 1 || rule.Thresholds["warningFrames"] != 2 {
		t.Fatalf("default A/V policy lost #314 semantics: %#v", rule)
	}
}

func TestDefaultAndStrictEnableCanonicalPixelFormatPolicyField(t *testing.T) {
	for _, preset := range []string{Default, Strict} {
		policy := Normalize(mustPolicyMap(t, Factory(preset)))
		fields := policy.Rules["video.properties"].Fields
		if !fields["pixelFormat"] {
			t.Errorf("%s policy does not enable canonical pixelFormat field: %#v", preset, fields)
		}
		if _, exists := fields["pixelSemantic"]; exists {
			t.Errorf("%s policy retains obsolete pixelSemantic alias: %#v", preset, fields)
		}
	}
}

func TestPresetDefinitionsKeepDistinctExplicitSemantics(t *testing.T) {
	for _, preset := range []string{Disabled, Basic, Default, Strict} {
		t.Run(preset, func(t *testing.T) {
			policy := Normalize(mustPolicyMap(t, Factory(preset)))
			if policy.Preset != preset {
				t.Fatalf("normalized preset=%q, want %q", policy.Preset, preset)
			}
			for key, rule := range policy.Rules {
				if IsMandatory(key) {
					if !rule.Enabled || rule.Disposition != "blocking" || rule.Overrideable {
						t.Errorf("%s mandatory rule is unsafe: %#v", key, rule)
					}
					continue
				}
				switch preset {
				case Disabled:
					if rule.Enabled {
						t.Errorf("Disabled enables quality rule %s", key)
					}
				case Basic:
					if !rule.Enabled {
						t.Errorf("Basic disables essential rule %s", key)
					}
					if key == "video.properties" || key == "timeline.av_alignment" {
						if rule.Disposition != "advisory" {
							t.Errorf("Basic advanced fidelity rule %s disposition=%s", key, rule.Disposition)
						}
					} else if key != "chapters.actions" && rule.Disposition != "blocking" {
						t.Errorf("Basic essential rule %s disposition=%s", key, rule.Disposition)
					}
				case Default:
					if !rule.Enabled {
						t.Errorf("Default disables rule %s", key)
					}
					if key != "chapters.actions" && rule.Disposition != "blocking" {
						t.Errorf("Default rule %s disposition=%s", key, rule.Disposition)
					}
				case Strict:
					if !rule.Enabled || rule.Disposition != "blocking" {
						t.Errorf("Strict rule %s is not enabled and blocking: %#v", key, rule)
					}
				}
			}
		})
	}
	custom := Normalize(map[string]any{"preset": Custom})
	if custom.Preset != Custom || custom.BasedOn != Default {
		t.Fatalf("empty Custom policy should inherit Default safely: %#v", custom)
	}
}

func TestEditingNamedPresetTransitionsToCustomAndCanRestore(t *testing.T) {
	policy := Factory(Default)
	rule := policy.Rules["duration.timeline"]
	rule.Thresholds["maxPercent"] = 3
	policy.Rules["duration.timeline"] = rule
	encoded, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	custom := Normalize(raw)
	if custom.Preset != Custom || custom.BasedOn != Default {
		t.Fatalf("edited policy identity=%s basedOn=%s", custom.Preset, custom.BasedOn)
	}
	if len(Differences(custom, Default)) != 1 {
		t.Fatalf("expected one changed value, got %v", Differences(custom, Default))
	}
	if got := Factory(Default); got.Preset != Default || got.Rules["duration.timeline"].Thresholds["maxPercent"] != 2 {
		t.Fatalf("restored factory defaults are incorrect: %#v", got)
	}
}

func TestFingerprintTracksNormalizedEffectivePolicy(t *testing.T) {
	base := DefaultPolicy()
	if Fingerprint(base) != Fingerprint(Normalize(map[string]any{"preset": Default})) {
		t.Fatal("identical normalized policy has unstable fingerprint")
	}
	changed := base
	changed.Rules = cloneRules(base.Rules)
	rule := changed.Rules["duration.timeline"]
	rule.Thresholds["maxPercent"] = 3
	changed.Rules["duration.timeline"] = rule
	changed = Normalize(mustPolicyMap(t, changed))
	if Fingerprint(base) == Fingerprint(changed) {
		t.Fatal("changed effective policy retained fingerprint")
	}
}

func TestMandatoryRulesCannotBeDisabledOrOverridden(t *testing.T) {
	raw := map[string]any{"preset": Default, "rules": map[string]any{}}
	rules := raw["rules"].(map[string]any)
	for _, key := range []string{"artifact.identity", "artifact.integrity", "artifact.manifest", "execution.binding"} {
		rules[key] = map[string]any{"enabled": false, "disposition": "advisory", "overrideable": true}
	}
	policy := Normalize(raw)
	for key := range mandatoryRules {
		rule := policy.Rules[key]
		if !rule.Enabled || rule.Disposition != "blocking" || rule.Overrideable {
			t.Errorf("mandatory %s was weakened: %#v", key, rule)
		}
	}
}

func TestLegacyDurationMigrationAndScoreRemoval(t *testing.T) {
	policy := MigrateLegacy(map[string]any{"minimumScore": 100, "requireDurationMatch": false})
	if policy.Preset != Custom || policy.BasedOn != Default {
		t.Fatalf("legacy policy identity=%s basedOn=%s", policy.Preset, policy.BasedOn)
	}
	if policy.Rules["duration.timeline"].Enabled {
		t.Fatal("legacy false duration setting was not mapped to the duration rule")
	}
	if _, exists := mustPolicyMap(t, policy)["minimumScore"]; exists {
		t.Fatal("obsolete minimumScore survived migration")
	}
	if enabled := MigrateLegacy(map[string]any{"minimumScore": 0, "requireDurationMatch": true}).Rules["duration.timeline"].Enabled; !enabled {
		t.Fatal("legacy true duration setting did not retain the real duration rule")
	}
}

func mustPolicyMap(t *testing.T, policy Policy) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func cloneRules(in map[string]Rule) map[string]Rule {
	out := make(map[string]Rule, len(in))
	for key, value := range in {
		copy := value
		copy.Thresholds = map[string]float64{}
		for name, threshold := range value.Thresholds {
			copy.Thresholds[name] = threshold
		}
		copy.Fields = map[string]bool{}
		for name, enabled := range value.Fields {
			copy.Fields[name] = enabled
		}
		out[key] = copy
	}
	return out
}
