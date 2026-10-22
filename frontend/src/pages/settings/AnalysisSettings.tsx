import { Alert, Button, Card, CardContent, Divider, FormControlLabel, Grid, MenuItem, Stack, Switch, TextField, Typography } from '@mui/material';
import SaveIcon from '@mui/icons-material/Save';
import { useState } from 'react';
import { booleanValue, numberValue } from '../../utils/settingsPageHelpers';

export type AnalysisMode = 'fast' | 'balanced' | 'thorough' | 'custom';
export type AnalysisPolicyValue = { mode: AnalysisMode; adaptiveAnalysis: boolean; earlyConfidenceEnabled: boolean; earlyConfidenceThreshold: number; initialWindows: number; maximumWindows: number; windowSeconds: number; positions: number[]; interlaceValidation: 'automatic' | 'always'; cropDepth: 'reduced' | 'normal' | 'full'; reuseSnapshots: boolean; incrementalRefresh: boolean; concurrentAssets: number };
export type ProfileSampleEstimatePolicyValue = { hardwareWindows: number; softwareWindows: number; operationTimeoutMinutes: number };

export function AnalysisSettingsDomain({ analysisPolicy, profileSamplePolicy, saving, onSaveAnalysisPolicy, onSaveProfileSamplePolicy }: {
  analysisPolicy: AnalysisPolicyValue;
  profileSamplePolicy: ProfileSampleEstimatePolicyValue;
  saving: boolean;
  onSaveAnalysisPolicy: (value: AnalysisPolicyValue) => void;
  onSaveProfileSamplePolicy: (value: ProfileSampleEstimatePolicyValue) => void;
}) {
  return (
    <Stack spacing={2}>
      <AnalysisPolicyCard key={JSON.stringify(analysisPolicy)} value={analysisPolicy} saving={saving} onSave={onSaveAnalysisPolicy} />
      <ProfileSampleEstimatePolicyCard key={JSON.stringify(profileSamplePolicy)} value={profileSamplePolicy} saving={saving} onSave={onSaveProfileSamplePolicy} />
    </Stack>
  );
}

export function analysisPolicyValue(value: Record<string, unknown> | undefined): AnalysisPolicyValue {
  const mode: AnalysisMode = ['fast', 'balanced', 'thorough', 'custom'].includes(String(value?.mode)) ? value?.mode as AnalysisMode : 'balanced';
  const positions = Array.isArray(value?.positions) ? value.positions.map(Number).filter((item) => Number.isFinite(item) && item > 0 && item < 1) : [];
  return {
    mode,
    adaptiveAnalysis: booleanValue(value?.adaptiveAnalysis, true),
    earlyConfidenceEnabled: booleanValue(value?.earlyConfidenceEnabled, true),
    earlyConfidenceThreshold: Math.max(0.9, Math.min(1, numberValue(value?.earlyConfidenceThreshold, 0.98))),
    initialWindows: Math.max(3, Math.min(5, numberValue(value?.initialWindows, 3))),
    maximumWindows: Math.max(3, Math.min(5, numberValue(value?.maximumWindows, 5))),
    windowSeconds: Math.max(5, Math.min(60, numberValue(value?.windowSeconds, 20))),
    positions: positions.length ? positions : [0.08, 0.27, 0.5, 0.73, 0.92],
    interlaceValidation: value?.interlaceValidation === 'always' ? 'always' : 'automatic',
    cropDepth: value?.cropDepth === 'reduced' || value?.cropDepth === 'full' ? value.cropDepth : 'normal',
    reuseSnapshots: booleanValue(value?.reuseSnapshots, true),
    incrementalRefresh: booleanValue(value?.incrementalRefresh, true),
    concurrentAssets: Math.max(1, Math.min(4, numberValue(value?.concurrentAssets, 1))),
  };
}

function AnalysisPolicyCard({ value, saving, onSave }: {
  value: AnalysisPolicyValue;
  saving: boolean;
  onSave: (value: AnalysisPolicyValue) => void;
}) {
  const [draft, setDraft] = useState(value);
  const custom = draft.mode === 'custom';
  const summaries: Record<Exclude<AnalysisMode, 'custom'>, string> = {
    fast: '3 windows, reduced crop and automatic interlace validation.',
    balanced: '3 → 5 windows, 98% early confidence, normal crop and automatic interlace validation.',
    thorough: '5 windows, no early stop, full crop and always validate interlace.',
  };

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Snapshot analysis policy</Typography>
            <Typography color="text.secondary" variant="body2">Choose intent first. Low-level sampling controls are available only in Custom mode.</Typography>
          </Stack>
          <TextField select label="Analysis mode" value={draft.mode} onChange={(event) => setDraft({ ...draft, mode: event.target.value as AnalysisMode })} fullWidth>
            <MenuItem value="fast">Fast</MenuItem>
            <MenuItem value="balanced">Balanced (default)</MenuItem>
            <MenuItem value="thorough">Thorough</MenuItem>
            <MenuItem value="custom">Custom</MenuItem>
          </TextField>
          {!custom ? <Alert severity="info">{summaries[draft.mode as Exclude<AnalysisMode, 'custom'>]}</Alert> : null}
          {custom ? (
            <>
              <Grid container spacing={1.5}>
                <Grid size={{ xs: 12, sm: 4 }}><TextField type="number" label="Initial windows" value={draft.initialWindows} inputProps={{ min: 3, max: 5 }} onChange={(event) => setDraft({ ...draft, initialWindows: Math.max(3, Math.min(5, Number(event.target.value))) })} fullWidth /></Grid>
                <Grid size={{ xs: 12, sm: 4 }}><TextField type="number" label="Maximum windows" value={draft.maximumWindows} inputProps={{ min: draft.initialWindows, max: 5 }} onChange={(event) => setDraft({ ...draft, maximumWindows: Math.max(draft.initialWindows, Math.min(5, Number(event.target.value))) })} fullWidth /></Grid>
                <Grid size={{ xs: 12, sm: 4 }}><TextField type="number" label="Window length (seconds)" value={draft.windowSeconds} inputProps={{ min: 5, max: 60 }} onChange={(event) => setDraft({ ...draft, windowSeconds: Math.max(5, Math.min(60, Number(event.target.value))) })} fullWidth /></Grid>
      <Grid size={{ xs: 12 }}><TextField label="Window centers (%)" value={draft.positions.map((position: number) => Math.round(position * 100)).join(', ')} onChange={(event) => setDraft({ ...draft, positions: event.target.value.split(',').map((item) => Number(item.trim()) / 100).filter((item) => Number.isFinite(item) && item > 0 && item < 1) })} helperText="At least one position per maximum window; for example 8, 27, 50, 73, 92." fullWidth /></Grid>
                <Grid size={{ xs: 12, sm: 6 }}><TextField select label="Interlace validation" value={draft.interlaceValidation} onChange={(event) => setDraft({ ...draft, interlaceValidation: event.target.value as AnalysisPolicyValue['interlaceValidation'] })} fullWidth><MenuItem value="automatic">Automatic</MenuItem><MenuItem value="always">Always deep</MenuItem></TextField></Grid>
                <Grid size={{ xs: 12, sm: 6 }}><TextField select label="Crop sampling" value={draft.cropDepth} onChange={(event) => setDraft({ ...draft, cropDepth: event.target.value as AnalysisPolicyValue['cropDepth'] })} fullWidth><MenuItem value="reduced">Reduced</MenuItem><MenuItem value="normal">Normal</MenuItem><MenuItem value="full">Full</MenuItem></TextField></Grid>
              </Grid>
              <Stack>
                <FormControlLabel control={<Switch checked={draft.adaptiveAnalysis} onChange={(event) => setDraft({ ...draft, adaptiveAnalysis: event.target.checked })} />} label="Adaptive Analysis" />
                <Typography color="text.secondary" variant="body2">Start with the initial sampling windows and collect additional windows only when evidence is uncertain. Sampling duration may also adapt to asset length.</Typography>
              </Stack>
              <FormControlLabel control={<Switch checked={draft.earlyConfidenceEnabled} onChange={(event) => setDraft({ ...draft, earlyConfidenceEnabled: event.target.checked })} />} label="Stop early when evidence is stable" />
              {draft.earlyConfidenceEnabled ? <TextField type="number" label="Early confidence threshold" value={draft.earlyConfidenceThreshold} inputProps={{ min: 0.9, max: 1, step: 0.01 }} onChange={(event) => setDraft({ ...draft, earlyConfidenceThreshold: Math.max(0.9, Math.min(1, Number(event.target.value))) })} /> : null}
            </>
          ) : null}
          <Divider />
          <Typography variant="h4">Cache and workload</Typography>
          <FormControlLabel control={<Switch checked={draft.reuseSnapshots} onChange={(event) => setDraft({ ...draft, reuseSnapshots: event.target.checked })} />} label="Reuse valid snapshots" />
          <FormControlLabel control={<Switch checked={draft.incrementalRefresh} disabled={!draft.reuseSnapshots} onChange={(event) => setDraft({ ...draft, incrementalRefresh: event.target.checked })} />} label="Refresh only stale analysis components" />
          <TextField type="number" label="Concurrent assets" value={draft.concurrentAssets} inputProps={{ min: 1, max: 4 }} onChange={(event) => setDraft({ ...draft, concurrentAssets: Math.max(1, Math.min(4, Number(event.target.value))) })} helperText="Applies to snapshot operations started from the API. Default: 1." />
          <Button startIcon={<SaveIcon />} variant="contained" disabled={saving || (custom && (draft.maximumWindows < draft.initialWindows || draft.positions.length < draft.maximumWindows))} onClick={() => onSave(draft)}>Save analysis policy</Button>
        </Stack>
      </CardContent>
    </Card>
  );
}

export function profileSampleEstimatePolicyValue(value: Record<string, unknown> | undefined): ProfileSampleEstimatePolicyValue {
  return {
    hardwareWindows: Math.max(1, Math.min(10, numberValue(value?.hardwareWindows, 5))),
    softwareWindows: Math.max(1, Math.min(10, numberValue(value?.softwareWindows, 3))),
    operationTimeoutMinutes: Math.max(5, Math.min(120, numberValue(value?.operationTimeoutMinutes, 30))),
  };
}

function ProfileSampleEstimatePolicyCard({ value, saving, onSave }: {
  value: ProfileSampleEstimatePolicyValue;
  saving: boolean;
  onSave: (value: ProfileSampleEstimatePolicyValue) => void;
}) {
  const [draft, setDraft] = useState(value);

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Profile LAB sample estimate</Typography>
            <Typography color="text.secondary" variant="body2">Control distributed sample coverage and the safety limit for background Profile LAB estimates.</Typography>
          </Stack>
          <Grid container spacing={1.5}>
            <Grid size={{ xs: 12, md: 4 }}><TextField type="number" label="Hardware sample windows" value={draft.hardwareWindows} inputProps={{ min: 1, max: 10, step: 1 }} onChange={(event) => setDraft({ ...draft, hardwareWindows: Math.max(1, Math.min(10, Number(event.target.value))) })} helperText="Number of distributed samples used with hardware encoders such as QSV." fullWidth /></Grid>
            <Grid size={{ xs: 12, md: 4 }}><TextField type="number" label="Software sample windows" value={draft.softwareWindows} inputProps={{ min: 1, max: 10, step: 1 }} onChange={(event) => setDraft({ ...draft, softwareWindows: Math.max(1, Math.min(10, Number(event.target.value))) })} helperText="Number of distributed samples used with software encoders such as libx265. Fewer samples reduce long CPU-bound LAB estimates." fullWidth /></Grid>
            <Grid size={{ xs: 12, md: 4 }}><TextField type="number" label="Maximum estimate runtime" value={draft.operationTimeoutMinutes} inputProps={{ min: 5, max: 120, step: 1 }} onChange={(event) => setDraft({ ...draft, operationTimeoutMinutes: Math.max(5, Math.min(120, Number(event.target.value))) })} helperText="Safety watchdog for the complete background sample estimate operation (minutes)." fullWidth /></Grid>
          </Grid>
          <Button startIcon={<SaveIcon />} variant="contained" disabled={saving} onClick={() => onSave(draft)}>Save Profile LAB estimate policy</Button>
        </Stack>
      </CardContent>
    </Card>
  );
}
