import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Chip,
  FormControlLabel,
  Grid,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import SaveIcon from '@mui/icons-material/Save';
import { useEffect, useMemo, useState } from 'react';
import type { ValidationPolicy, ValidationRulePolicy, ValidationPreset } from '../api/types';

const presetLabels: Record<ValidationPreset, string> = {
  disabled: 'Disabled', basic: 'Basic', default: 'Default', strict: 'Strict', custom: 'Custom',
};

const presetDescriptions: Record<ValidationPreset, string> = {
  disabled: 'Quality-oriented Validation is disabled. Result identity, integrity, frozen execution binding, and critical package safety remain active.',
  basic: 'Low-friction checks cover essential media and package behavior. Advanced fidelity findings are advisory.',
  default: 'Default provides the recommended balance between safety, compatibility, fidelity, and false positives.',
  strict: 'Stricter fidelity checks block when the frozen execution contains authoritative evidence.',
  custom: 'A custom policy keeps the selected base preset and records each effective value you changed.',
};

const ruleDescriptions: Record<string, {
  title: string; purpose: string; semantics: string; recommended: string; factory: string; why: string; effect: string;
}> = {
  'artifact.identity': {
    title: 'Result identity', purpose: 'Confirms the exact Result generation and output content revision.',
    semantics: 'PASS means the current output matches the Result record. FAIL means the content identity changed or is incomplete. Missing identity is never skippable.',
    recommended: 'Always enabled, blocking, and not overrideable.', factory: 'Always enabled, blocking, and not overrideable.',
    why: 'Publication must refer to the exact output that Validation inspected.', effect: 'This mandatory safety rule cannot be loosened.',
  },
  'artifact.manifest': {
    title: 'Result package manifest', purpose: 'Checks the Result manifest identity, primary output path, and frozen required artifact IDs.',
    semantics: 'PASS means the critical package identity is complete. FAIL means the manifest is missing, mismatched, or incomplete.',
    recommended: 'Always enabled, blocking, and not overrideable.', factory: 'Always enabled, blocking, and not overrideable.',
    why: 'The manifest binds the produced Result package to the exact Queue plan.', effect: 'This mandatory safety rule cannot be loosened.',
  },
  'artifact.integrity': {
    title: 'Result integrity', purpose: 'Checks that the produced Result exists, is readable, and is non-empty.',
    semantics: 'PASS means readable non-empty output. FAIL means missing, unreadable, empty, or a directory.',
    recommended: 'Always enabled, blocking, and not overrideable.', factory: 'Always enabled, blocking, and not overrideable.',
    why: 'A missing or unreadable Result can never be safely published.', effect: 'This mandatory safety rule cannot be loosened.',
  },
  'execution.binding': {
    title: 'Frozen execution binding', purpose: 'Checks that the Result belongs to the active frozen Queue execution and ExecutionPlan.',
    semantics: 'PASS means the frozen identities match. REVIEW_REQUIRED means the binding cannot be proven.',
    recommended: 'Always enabled, blocking, and not overrideable.', factory: 'Always enabled, blocking, and not overrideable.',
    why: 'Validation must use the exact execution snapshot that produced the Result.', effect: 'This mandatory safety rule cannot be loosened.',
  },
  'duration.timeline': {
    title: 'Duration / timeline', purpose: 'Detects unexpectedly truncated or extended output against frozen source duration evidence.',
    semantics: 'PASS is within tolerance. FAIL is a measured mismatch. UNVERIFIED means source or output duration is unavailable.',
    recommended: 'Default: max(2 seconds, 2% of source duration).', factory: 'maxAbsoluteSeconds 2; maxPercent 2.',
    why: 'Small container and timestamp differences are expected; larger shifts can indicate incomplete output.', effect: 'Lower tolerances catch smaller changes and may increase reviews; higher tolerances are more permissive.',
  },
  'timeline.av_alignment': {
    title: 'A/V alignment', purpose: 'Checks whether conversion introduced audio/video timestamp drift.',
    semantics: 'PASS is within PASS tolerance. WARNING is beyond PASS tolerance but within WARNING tolerance. FAIL exceeds WARNING tolerance. UNVERIFIED means required timing evidence is unavailable.',
    recommended: 'Default: PASS through 1 frame, WARNING through 2 frames, FAIL above 2 frames.', factory: 'passFrames 1; warningFrames 2.',
    why: 'Container timestamp quantization can cause sub-frame differences; larger offsets can become perceptible.', effect: 'Lower frame limits catch smaller drift and may increase review; higher limits allow more offset.',
  },
  'video.properties': {
    title: 'Video properties and fidelity', purpose: 'Compares codec, dimensions, aspect ratios, frame rate/cadence, field structure, pixel semantics, bit depth, chroma location, HEVC level/tier, and authoritative color contract.',
    semantics: 'PASS means all enabled authoritative fields match. FAIL is an authoritative mismatch. UNVERIFIED means required frozen evidence is missing. Optional non-authoritative color evidence stays optional.',
    recommended: 'Default: enabled and blocking when the frozen execution makes a field authoritative.', factory: 'All listed fields enabled; no added tolerance beyond canonical evaluator semantics.',
    why: 'Frozen Queue and ExecutionPlan evidence defines intended output while optional source metadata remains non-authoritative.', effect: 'Disabling fields reduces fidelity coverage; enabling more fields can surface more authoritative mismatches.',
  },
  'audio.tracks_actions': {
    title: 'Audio tracks and actions', purpose: 'Compares output audio stream membership with frozen keep/remove actions.',
    semantics: 'PASS means planned membership matches. FAIL means a known stream-count mismatch. UNVERIFIED means the frozen plan or output evidence is missing.',
    recommended: 'Enabled, blocking, and manually skippable per finding.', factory: 'Enabled, blocking, overrideable.',
    why: 'Unexpected audio additions or losses can change the intended package.', effect: 'Advisory disposition allows a warning; disabling skips this quality check.',
  },
  'subtitles.streams_actions': {
    title: 'Embedded subtitle streams', purpose: 'Compares embedded subtitle membership with frozen keep/remove/extract actions.',
    semantics: 'PASS means planned membership matches. FAIL is a known mismatch. UNVERIFIED means required evidence is unavailable.',
    recommended: 'Enabled and blocking.', factory: 'Enabled, blocking, overrideable.',
    why: 'The frozen track plan is the authority for which subtitle streams should remain embedded.', effect: 'Advisory or disabled settings can permit missing or extra embedded streams.',
  },
  'chapters.actions': {
    title: 'Chapter actions', purpose: 'Checks chapter presence against the frozen chapter action.',
    semantics: 'PASS means chapter presence matches. WARNING or UNVERIFIED reports a mismatch or missing evidence.',
    recommended: 'Advisory because chapter evidence is not always available from every container.', factory: 'Enabled, advisory, overrideable.',
    why: 'Chapter differences are useful to review but are less safety-sensitive than Result identity or required streams.', effect: 'Blocking disposition sends warnings to review; disabled stops evaluating chapter quality.',
  },
  'attachments.stream_actions': {
    title: 'Attachment stream actions', purpose: 'Compares output attachment membership with the frozen attachment action.',
    semantics: 'PASS means planned membership matches. FAIL is a known mismatch. UNVERIFIED means evidence is unavailable.',
    recommended: 'Enabled and blocking.', factory: 'Enabled, blocking, overrideable.',
    why: 'Unexpected attachment loss or addition can affect fonts and downstream playback.', effect: 'Advisory or disabled settings allow a mismatch to continue with less review.',
  },
  'attachments.font_artifacts': {
    title: 'Required font artifacts', purpose: 'Checks required font attachment artifacts from the frozen plan.',
    semantics: 'PASS means required artifacts are present. FAIL or UNVERIFIED means a required artifact failed or cannot be verified.',
    recommended: 'Enabled and blocking.', factory: 'Enabled, blocking, overrideable.',
    why: 'Missing required fonts can change subtitle rendering.', effect: 'Advisory or disabled settings reduce assurance that subtitles render as intended.',
  },
  'sidecars.package': {
    title: 'Required sidecar package', purpose: 'Checks required subtitle sidecars without modifying or regenerating them.',
    semantics: 'PASS means required sidecars are present and readable. FAIL or UNVERIFIED means a required artifact failed or cannot be verified.',
    recommended: 'Enabled and blocking.', factory: 'Enabled, blocking, overrideable.',
    why: 'Required package artifacts should match the frozen Queue intent.', effect: 'Advisory or disabled settings allow incomplete sidecar delivery.',
  },
};

const fidelityFieldLabels: Record<string, string> = {
  codec: 'Codec', width: 'Width', height: 'Height', sampleAspectRatio: 'SAR', displayAspectRatio: 'DAR',
  frameRate: 'Frame rate and cadence', fieldStructure: 'Field structure', pixelFormat: 'Pixel semantics',
  bitDepth: 'Bit depth', chromaLocation: 'Chroma location', hevcLevel: 'HEVC level', hevcTier: 'HEVC tier', colorPolicy: 'Color contract',
};

export function ValidationPolicySettingsCard({ value, saving, autoPublisherEnabled, onSave }: {
  value: Record<string, unknown> | undefined;
  saving: boolean;
  autoPublisherEnabled?: boolean;
  onSave: (value: Record<string, unknown>) => void;
}) {
  const storedPolicy = useMemo(() => validationPolicyFromSetting(value), [value]);
  const [draft, setDraft] = useState(storedPolicy);
  const [showDifferences, setShowDifferences] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [confirmWarningOptInOpen, setConfirmWarningOptInOpen] = useState(false);

  useEffect(() => { setDraft(storedPolicy); }, [storedPolicy]);

  const storedOutcomeSetting = value?.autoAdvanceOutcomes;
  const currentAutoAdvanceOutcomes = normalizeAutoAdvanceOutcomes(storedOutcomeSetting);
  const draftAutoAdvanceOutcomes = normalizeAutoAdvanceOutcomes(draft.autoAdvanceOutcomes);
  const currentOutcomeSettingMissing = !Array.isArray(storedOutcomeSetting) || storedOutcomeSetting.length === 0;
  const allowsWarningAdvancement = draftAutoAdvanceOutcomes.includes('PASS_WITH_WARNINGS');
  const basePreset = draft.preset === 'custom' ? (draft.basedOn ?? 'default') : draft.preset;
  const basePolicy = draft.presetDefaults?.[basePreset];
  const differences = policyDifferences(draft, basePolicy);

  const changePreset = (preset: ValidationPreset) => {
    if (preset === 'custom') {
      setDraft((current) => ({ ...current, preset: 'custom', basedOn: current.preset === 'custom' ? current.basedOn : current.preset }));
      return;
    }
    const factory = draft.presetDefaults?.[preset];
    if (!factory) return;
    setDraft((current) => ({ ...current, ...factory, presetDefaults: current.presetDefaults, autoAdvanceOutcomes: current.autoAdvanceOutcomes }));
  };

  const updateRule = (key: string, update: (rule: ValidationRulePolicy) => ValidationRulePolicy) => {
    setDraft((current) => {
      const base = current.preset === 'custom' ? (current.basedOn ?? 'default') : current.preset;
      const next: ValidationPolicy = {
        ...current,
        preset: 'custom',
        basedOn: base,
        rules: { ...current.rules, [key]: update(current.rules[key]) },
      };
      return next;
    });
  };

  const restoreDefaults = () => {
    const factory = draft.presetDefaults?.[basePreset];
    if (factory) setDraft((current) => ({ ...current, ...factory, presetDefaults: current.presetDefaults, autoAdvanceOutcomes: current.autoAdvanceOutcomes }));
  };

  const save = () => {
    const policy: Record<string, unknown> = { ...(value ?? {}), ...draft, autoAdvanceOutcomes: draftAutoAdvanceOutcomes };
    delete policy.presetDefaults;
    delete policy.differenceCount;
    delete policy.differences;
    onSave(policy);
  };

  const setWarningAdvancement = (enabled: boolean) => {
    setDraft((current) => ({
      ...current,
      autoAdvanceOutcomes: enabled ? ['PASS', 'PASS_WITH_WARNINGS'] : ['PASS'],
    }));
  };

  const handleWarningAdvancementChange = (enabled: boolean) => {
    if (enabled && !allowsWarningAdvancement) {
      setConfirmWarningOptInOpen(true);
      return;
    }
    setWarningAdvancement(enabled);
  };

  return (
    <Card id="validation-settings" sx={{ scrollMarginTop: 90 }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={1}>
            <Stack>
              <Typography variant="h3">Validation</Typography>
              <Typography color="text.secondary" variant="body2">Choose the rule policy for future Validation runs. Existing immutable runs keep their frozen policy.</Typography>
            </Stack>
            {draft.preset === 'default' ? <Chip color="success" label="Recommended" /> : null}
          </Stack>
          <Grid container spacing={1.5} alignItems="center">
            <Grid size={{ xs: 12, md: 5 }}>
              <TextField select fullWidth label="Validation Mode" value={draft.preset} onChange={(event) => changePreset(event.target.value as ValidationPreset)}>
                {(['disabled', 'basic', 'default', 'strict', 'custom'] as const).map((preset) => <MenuItem key={preset} value={preset}>{presetLabels[preset]}{preset === 'default' ? ' · Recommended' : ''}</MenuItem>)}
              </TextField>
            </Grid>
            <Grid size={{ xs: 12, md: 7 }}><Typography color="text.secondary">{presetDescriptions[draft.preset]}</Typography></Grid>
          </Grid>
          {draft.preset === 'disabled' ? <Alert severity="warning">Quality Validation is disabled. MVForge will still verify Result identity, artifact integrity, frozen execution binding, and publication package safety. These checks cannot be disabled or overridden.</Alert> : null}
          <Stack spacing={1}>
            <Typography variant="h6">Automatic Validation advancement</Typography>
            <Typography variant="body2" color="text.secondary">
              PASS is always allowed. Optionally allow PASS_WITH_WARNINGS when its findings are advisory. Actionable findings remain visible for follow-up; blocking findings still prevent handoff. FAIL and REVIEW_REQUIRED are never automatic opt-ins.
            </Typography>
            <Alert severity="info">
              <b>Current effective policy:</b> {automaticOutcomeLabel(currentAutoAdvanceOutcomes)}{currentOutcomeSettingMissing ? ' (the missing or empty setting defaults to PASS only).' : '.'}
            </Alert>
            {autoPublisherEnabled === false ? (
              <Alert severity="info">Auto Publisher is disabled. Saving an outcome policy will not start automatic publication while Auto Publisher remains disabled.</Alert>
            ) : autoPublisherEnabled === true ? (
              <Alert severity="success">Auto Publisher is enabled and will apply this policy at its next authorization check.</Alert>
            ) : (
              <Alert severity="info">Auto Publisher status is unavailable until its Settings value loads.</Alert>
            )}
            <FormControlLabel
              control={<Checkbox checked={allowsWarningAdvancement} disabled={saving} onChange={(event) => handleWarningAdvancementChange(event.target.checked)} />}
              label="Allow PASS_WITH_WARNINGS for automatic advancement"
            />
            {automaticOutcomeLabel(currentAutoAdvanceOutcomes) !== automaticOutcomeLabel(draftAutoAdvanceOutcomes) ? (
              <Typography variant="body2">Unsaved outcome policy: {automaticOutcomeLabel(draftAutoAdvanceOutcomes)}.</Typography>
            ) : null}
            <Alert severity="warning">
              This setting does not dismiss Validation findings or bypass Publisher checks. Publisher independently verifies Result identity, required sidecars, destination readiness, replacement authority, and other delivery requirements.
            </Alert>
          </Stack>
          {draft.preset === 'custom' ? <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} spacing={1}>
            <Typography>Custom · Based on: {presetLabels[basePreset]} · {differences.length} {differences.length === 1 ? 'value differs' : 'values differ'}</Typography>
            <Button size="small" onClick={() => setShowDifferences((current) => !current)}>{showDifferences ? 'Hide differences' : 'Show differences'}</Button>
          </Stack> : null}
          {draft.preset === 'custom' && showDifferences ? <Alert severity="info"><Stack spacing={0.25}>{differences.map((path) => <Typography variant="body2" key={path}>{describeDifference(path)}</Typography>)}{differences.length === 0 ? <Typography variant="body2">No values differ from the base preset.</Typography> : null}</Stack></Alert> : null}
          <Button variant="outlined" onClick={restoreDefaults} disabled={!draft.presetDefaults?.[basePreset]}>Restore preset defaults</Button>
          <Alert severity="info">The numeric Validation score is informational. Canonical rule outcomes and valid exact overrides determine handoff; score never authorizes publication.</Alert>
          <Accordion expanded={advancedOpen} onChange={(_, expanded) => setAdvancedOpen(expanded)}>
            <AccordionSummary expandIcon={<ExpandMoreIcon />}><Typography fontWeight={700}>Advanced rule editor</Typography></AccordionSummary>
            <AccordionDetails><Stack spacing={1.5}>
              {Object.keys(draft.rules).sort().map((key) => {
                const rule = draft.rules[key];
                const description = ruleDescriptions[key] ?? {
                  title: key, purpose: 'Checks canonical Validation evidence for this rule.', semantics: 'PASS is clean; WARNING is advisory; FAIL is a known violation; UNVERIFIED means required evidence is unavailable.',
                  recommended: 'Follow the Default preset.', factory: 'See the Default preset.', why: 'The canonical evaluator uses frozen execution evidence.', effect: 'Stricter settings increase review; looser settings reduce coverage.',
                };
                const mandatory = ['artifact.identity', 'artifact.integrity', 'artifact.manifest', 'execution.binding'].includes(key);
                return <Card key={key} variant="outlined"><CardContent><Stack spacing={1.25}>
                  <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}><Typography variant="h6">{description.title}</Typography><Chip size="small" variant="outlined" label={key} /></Stack>
                  <Typography variant="body2">{description.purpose}</Typography>
                  <Typography variant="body2" color="text.secondary">{description.semantics}</Typography>
                  <Typography variant="body2"><b>Recommended:</b> {description.recommended}</Typography>
                  <Typography variant="body2"><b>Factory default:</b> {description.factory}</Typography>
                  <Typography variant="body2"><b>Why:</b> {description.why}</Typography>
                  <Typography variant="body2"><b>Stricter / looser:</b> {description.effect}</Typography>
                  {mandatory ? <Alert severity="info">Always enabled · Blocking · Manual skip unavailable. This safety rule cannot be configured or overridden.</Alert> : <Grid container spacing={1}>
                    <Grid size={{ xs: 12, sm: 4 }}><FormControlLabel control={<Checkbox checked={rule.enabled} onChange={(event) => updateRule(key, (current) => ({ ...current, enabled: event.target.checked }))} />} label="Enabled" /></Grid>
                <Grid size={{ xs: 12, sm: 4 }}><TextField select fullWidth size="small" label="Disposition" value={rule.disposition} onChange={(event) => updateRule(key, (current) => ({ ...current, disposition: event.target.value as ValidationRulePolicy['disposition'] }))}><MenuItem value="blocking">Blocking</MenuItem><MenuItem value="advisory">Advisory</MenuItem></TextField></Grid>
                    <Grid size={{ xs: 12, sm: 4 }}><FormControlLabel control={<Checkbox checked={rule.overrideable} onChange={(event) => updateRule(key, (current) => ({ ...current, overrideable: event.target.checked }))} />} label="Allow manual skip" /></Grid>
                    {Object.entries(rule.thresholds ?? {}).map(([name, threshold]) => <Grid size={{ xs: 12, sm: 4 }} key={name}><TextField size="small" fullWidth type="number" label={thresholdLabel(name)} value={threshold} inputProps={{ min: 0, step: 0.1 }} onChange={(event) => updateRule(key, (current) => ({ ...current, thresholds: { ...current.thresholds, [name]: Number(event.target.value) } }))} /></Grid>)}
                    <Grid size={{ xs: 12, sm: 4 }}><TextField size="small" fullWidth type="number" label="Warning score penalty" value={rule.warningPenalty} inputProps={{ min: 0, max: 100 }} onChange={(event) => updateRule(key, (current) => ({ ...current, warningPenalty: clampPenalty(event.target.value) }))} /></Grid>
                    <Grid size={{ xs: 12, sm: 4 }}><TextField size="small" fullWidth type="number" label="Failure score penalty" value={rule.failurePenalty} inputProps={{ min: 0, max: 100 }} onChange={(event) => updateRule(key, (current) => ({ ...current, failurePenalty: clampPenalty(event.target.value) }))} /></Grid>
                    {key === 'video.properties' ? <Grid size={{ xs: 12 }}><Typography fontWeight={700} variant="body2">Fidelity fields evaluated by the canonical rule</Typography><Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>{Object.entries(rule.fields ?? {}).map(([field, enabled]) => <FormControlLabel key={field} control={<Checkbox size="small" checked={enabled} onChange={(event) => updateRule(key, (current) => ({ ...current, fields: { ...current.fields, [field]: event.target.checked } }))} />} label={fidelityFieldLabels[field] ?? field} />)}</Box></Grid> : null}
                  </Grid>}
                </Stack></CardContent></Card>;
              })}
            </Stack></AccordionDetails>
          </Accordion>
          <Button startIcon={<SaveIcon />} variant="contained" onClick={save} disabled={saving}>Save Validation settings</Button>
        </Stack>
      </CardContent>
      <Dialog open={confirmWarningOptInOpen} onClose={() => setConfirmWarningOptInOpen(false)} aria-labelledby="validation-warning-opt-in-title">
        <DialogTitle id="validation-warning-opt-in-title">Confirm automatic warning advancement</DialogTitle>
        <DialogContent>
          <Typography>
            After you confirm and save, the next Auto Publisher tick may attempt eligible jobs already at ready_to_publish with PASS_WITH_WARNINGS, as well as future jobs. This applies only while Auto Publisher is enabled. It does not trigger publication by itself, and every attempt still passes the independent Publisher identity and delivery checks.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmWarningOptInOpen(false)}>Cancel</Button>
          <Button variant="contained" onClick={() => { setWarningAdvancement(true); setConfirmWarningOptInOpen(false); }}>
            Confirm warning opt-in
          </Button>
        </DialogActions>
      </Dialog>
    </Card>
  );
}

function validationPolicyFromSetting(value: Record<string, unknown> | undefined): ValidationPolicy {
  const policy = value && typeof value.rules === 'object' && value.rules !== null
    ? value as unknown as ValidationPolicy
    : { schemaVersion: 1, policyVersion: 'validation-policy-v1', preset: 'default' as const, rules: {}, presetDefaults: {} };
  return { ...policy, autoAdvanceOutcomes: normalizeAutoAdvanceOutcomes(value?.autoAdvanceOutcomes) };
}

function normalizeAutoAdvanceOutcomes(value: unknown): string[] {
  const outcomes = Array.isArray(value) ? value.filter((outcome): outcome is string => typeof outcome === 'string') : [];
  return outcomes.includes('PASS_WITH_WARNINGS') ? ['PASS', 'PASS_WITH_WARNINGS'] : ['PASS'];
}

function automaticOutcomeLabel(outcomes: string[]): string {
  return outcomes.includes('PASS_WITH_WARNINGS') ? 'PASS and PASS_WITH_WARNINGS' : 'PASS only';
}

function policyDifferences(policy: ValidationPolicy, base: ValidationPolicy | undefined): string[] {
  if (!base) return policy.differences ?? [];
  const differences: string[] = [];
  for (const key of Object.keys(base.rules).sort()) {
    const actual = policy.rules[key]; const expected = base.rules[key];
    if (!actual || !expected) { differences.push(`rules.${key}`); continue; }
    for (const field of ['enabled', 'disposition', 'overrideable', 'warningPenalty', 'failurePenalty'] as const) {
      if (actual[field] !== expected[field]) differences.push(`rules.${key}.${field}`);
    }
    for (const field of new Set([...Object.keys(actual.thresholds ?? {}), ...Object.keys(expected.thresholds ?? {})])) {
      if (actual.thresholds?.[field] !== expected.thresholds?.[field]) differences.push(`rules.${key}.thresholds.${field}`);
    }
    for (const field of new Set([...Object.keys(actual.fields ?? {}), ...Object.keys(expected.fields ?? {})])) {
      if (actual.fields?.[field] !== expected.fields?.[field]) differences.push(`rules.${key}.fields.${field}`);
    }
  }
  return differences;
}

function describeDifference(path: string): string {
  return path.replace(/^rules\./, '').replaceAll('.', ' · ');
}

function thresholdLabel(name: string): string {
  const labels: Record<string, string> = { maxAbsoluteSeconds: 'Maximum absolute seconds', maxPercent: 'Maximum duration percent', passFrames: 'PASS tolerance · frames', warningFrames: 'WARNING tolerance · frames' };
  return labels[name] ?? name;
}

function clampPenalty(value: string): number {
  return Math.min(100, Math.max(0, Number(value) || 0));
}
