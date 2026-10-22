import { Alert, Card, CardContent, Divider, FormControlLabel, Grid, MenuItem, Stack, Switch, TextField, Typography } from '@mui/material';

export type AnalysisMode = 'fast' | 'balanced' | 'thorough' | 'custom';
export type AnalysisPolicyValue = { mode: AnalysisMode; adaptiveAnalysis: boolean; earlyConfidenceEnabled: boolean; earlyConfidenceThreshold: number; initialWindows: number; maximumWindows: number; windowSeconds: number; positions: number[]; interlaceValidation: 'automatic' | 'always'; cropDepth: 'reduced' | 'normal' | 'full'; reuseSnapshots: boolean; incrementalRefresh: boolean; concurrentAssets: number };
export type ProfileSampleEstimatePolicyValue = { hardwareWindows: number; softwareWindows: number; operationTimeoutMinutes: number };

export function AnalysisSettingsDomain({ analysisPolicy, profileSamplePolicy, onChangeAnalysisPolicy, onChangeProfileSamplePolicy }: {
  analysisPolicy: AnalysisPolicyValue;
  profileSamplePolicy: ProfileSampleEstimatePolicyValue;
  onChangeAnalysisPolicy: (value: AnalysisPolicyValue) => void;
  onChangeProfileSamplePolicy: (value: ProfileSampleEstimatePolicyValue) => void;
}) {
  return (
    <Stack spacing={2}>
      <AnalysisPolicyCard value={analysisPolicy} onChange={onChangeAnalysisPolicy} />
      <ProfileSampleEstimatePolicyCard value={profileSamplePolicy} onChange={onChangeProfileSamplePolicy} />
    </Stack>
  );
}

function AnalysisPolicyCard({ value, onChange }: {
  value: AnalysisPolicyValue;
  onChange: (value: AnalysisPolicyValue) => void;
}) {
  const draft = value;
  const custom = draft.mode === 'custom';
  const invalidCustomPolicy = custom && (draft.maximumWindows < draft.initialWindows || draft.positions.length < draft.maximumWindows);
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
          <TextField select label="Analysis mode" value={draft.mode} onChange={(event) => onChange({ ...draft, mode: event.target.value as AnalysisMode })} fullWidth>
            <MenuItem value="fast">Fast</MenuItem>
            <MenuItem value="balanced">Balanced (default)</MenuItem>
            <MenuItem value="thorough">Thorough</MenuItem>
            <MenuItem value="custom">Custom</MenuItem>
          </TextField>
          {!custom ? <Alert severity="info">{summaries[draft.mode as Exclude<AnalysisMode, 'custom'>]}</Alert> : null}
          {custom ? (
            <>
              <Grid container spacing={1.5}>
                <Grid size={{ xs: 12, sm: 4 }}><TextField type="number" label="Initial windows" value={draft.initialWindows} inputProps={{ min: 3, max: 5 }} onChange={(event) => onChange({ ...draft, initialWindows: Math.max(3, Math.min(5, Number(event.target.value))) })} fullWidth /></Grid>
                <Grid size={{ xs: 12, sm: 4 }}><TextField type="number" label="Maximum windows" value={draft.maximumWindows} inputProps={{ min: draft.initialWindows, max: 5 }} onChange={(event) => onChange({ ...draft, maximumWindows: Math.max(draft.initialWindows, Math.min(5, Number(event.target.value))) })} fullWidth /></Grid>
                <Grid size={{ xs: 12, sm: 4 }}><TextField type="number" label="Window length (seconds)" value={draft.windowSeconds} inputProps={{ min: 5, max: 60 }} onChange={(event) => onChange({ ...draft, windowSeconds: Math.max(5, Math.min(60, Number(event.target.value))) })} fullWidth /></Grid>
      <Grid size={{ xs: 12 }}><TextField label="Window centers (%)" value={draft.positions.map((position) => Math.round(position * 100)).join(', ')} onChange={(event) => onChange({ ...draft, positions: event.target.value.split(',').map((item) => Number(item.trim()) / 100).filter((item) => Number.isFinite(item) && item > 0 && item < 1) })} helperText="At least one position per maximum window; for example 8, 27, 50, 73, 92." fullWidth /></Grid>
                <Grid size={{ xs: 12, sm: 6 }}><TextField select label="Interlace validation" value={draft.interlaceValidation} onChange={(event) => onChange({ ...draft, interlaceValidation: event.target.value as AnalysisPolicyValue['interlaceValidation'] })} fullWidth><MenuItem value="automatic">Automatic</MenuItem><MenuItem value="always">Always deep</MenuItem></TextField></Grid>
                <Grid size={{ xs: 12, sm: 6 }}><TextField select label="Crop sampling" value={draft.cropDepth} onChange={(event) => onChange({ ...draft, cropDepth: event.target.value as AnalysisPolicyValue['cropDepth'] })} fullWidth><MenuItem value="reduced">Reduced</MenuItem><MenuItem value="normal">Normal</MenuItem><MenuItem value="full">Full</MenuItem></TextField></Grid>
              </Grid>
              <Stack>
                <FormControlLabel control={<Switch checked={draft.adaptiveAnalysis} onChange={(event) => onChange({ ...draft, adaptiveAnalysis: event.target.checked })} />} label="Adaptive Analysis" />
                <Typography color="text.secondary" variant="body2">Start with the initial sampling windows and collect additional windows only when evidence is uncertain. Sampling duration may also adapt to asset length.</Typography>
              </Stack>
              <FormControlLabel control={<Switch checked={draft.earlyConfidenceEnabled} onChange={(event) => onChange({ ...draft, earlyConfidenceEnabled: event.target.checked })} />} label="Stop early when evidence is stable" />
              {draft.earlyConfidenceEnabled ? <TextField type="number" label="Early confidence threshold" value={draft.earlyConfidenceThreshold} inputProps={{ min: 0.9, max: 1, step: 0.01 }} onChange={(event) => onChange({ ...draft, earlyConfidenceThreshold: Math.max(0.9, Math.min(1, Number(event.target.value))) })} /> : null}
            </>
          ) : null}
          <Divider />
          <Typography variant="h4">Cache and workload</Typography>
          <FormControlLabel control={<Switch checked={draft.reuseSnapshots} onChange={(event) => onChange({ ...draft, reuseSnapshots: event.target.checked })} />} label="Reuse valid snapshots" />
          <FormControlLabel control={<Switch checked={draft.incrementalRefresh} disabled={!draft.reuseSnapshots} onChange={(event) => onChange({ ...draft, incrementalRefresh: event.target.checked })} />} label="Refresh only stale analysis components" />
          <TextField type="number" label="Concurrent assets" value={draft.concurrentAssets} inputProps={{ min: 1, max: 4 }} onChange={(event) => onChange({ ...draft, concurrentAssets: Math.max(1, Math.min(4, Number(event.target.value))) })} helperText="Applies to snapshot operations started from the API. Default: 1." />
          {invalidCustomPolicy ? <Alert severity="warning">Use at least one sampling position for each maximum window, with maximum windows no lower than initial windows.</Alert> : null}
        </Stack>
      </CardContent>
    </Card>
  );
}

function ProfileSampleEstimatePolicyCard({ value, onChange }: {
  value: ProfileSampleEstimatePolicyValue;
  onChange: (value: ProfileSampleEstimatePolicyValue) => void;
}) {
  const draft = value;

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Profile LAB sample estimate</Typography>
            <Typography color="text.secondary" variant="body2">Control distributed sample coverage and the safety limit for background Profile LAB estimates.</Typography>
          </Stack>
          <Grid container spacing={1.5}>
            <Grid size={{ xs: 12, md: 4 }}><TextField type="number" label="Hardware sample windows" value={draft.hardwareWindows} inputProps={{ min: 1, max: 10, step: 1 }} onChange={(event) => onChange({ ...draft, hardwareWindows: Math.max(1, Math.min(10, Number(event.target.value))) })} helperText="Number of distributed samples used with hardware encoders such as QSV." fullWidth /></Grid>
            <Grid size={{ xs: 12, md: 4 }}><TextField type="number" label="Software sample windows" value={draft.softwareWindows} inputProps={{ min: 1, max: 10, step: 1 }} onChange={(event) => onChange({ ...draft, softwareWindows: Math.max(1, Math.min(10, Number(event.target.value))) })} helperText="Number of distributed samples used with software encoders such as libx265. Fewer samples reduce long CPU-bound LAB estimates." fullWidth /></Grid>
            <Grid size={{ xs: 12, md: 4 }}><TextField type="number" label="Maximum estimate runtime" value={draft.operationTimeoutMinutes} inputProps={{ min: 5, max: 120, step: 1 }} onChange={(event) => onChange({ ...draft, operationTimeoutMinutes: Math.max(5, Math.min(120, Number(event.target.value))) })} helperText="Safety watchdog for the complete background sample estimate operation (minutes)." fullWidth /></Grid>
          </Grid>
        </Stack>
      </CardContent>
    </Card>
  );
}
