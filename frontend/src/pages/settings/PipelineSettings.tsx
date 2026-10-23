import type { Dispatch, SetStateAction } from 'react';
import { Alert, Button, Card, CardContent, Divider, FormControlLabel, Grid, IconButton, MenuItem, Stack, Switch, TextField, Typography } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import SaveIcon from '@mui/icons-material/Save';
import { useSettingsDraftValue } from '../../hooks/useSettingsDraftValue';
import { arrayValue, booleanValue, stringValue } from '../../utils/settingsPageHelpers';
import type { SettingsForm, WorkingHoursValue } from '../SettingsPage';

type SettingsCardProps = { form: SettingsForm; setForm: Dispatch<SetStateAction<SettingsForm>> };
function identitySettingsValue<T>(value: unknown): T { return value as T; }

type WorkingWindow = { name: string; days: string[]; start: string; end: string };

function PipelineAutomationCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Pipeline Automation</Typography>
            <Typography color="text.secondary" variant="body2">
              Automatic handoff between worker, validation, publishing, and originals archive.
            </Typography>
          </Stack>
          <Alert severity="info">
            Disabled stages stop the pipeline and wait for human action in that section.
          </Alert>
          <FormControlLabel
            control={
              <Switch
                checked={form.pipelineAutomation.autoAnalysisEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      autoAnalysisEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Automatic analysis"
          />
          <FormControlLabel
            control={
              <Switch
                checked={
                  form.pipelineAutomation.forceFreshSnapshotBeforeExecution
                }
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      forceFreshSnapshotBeforeExecution:
                        event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Force fresh snapshot before execution"
          />

          <Typography color="text.secondary" variant="caption">
            Re-analyzes the physical asset immediately before a real conversion.
            Existing snapshots are ignored for that analysis. This adds processing
            time but ensures AUTO decisions use current media metadata and frame
            structure.
          </Typography>
          <TextField
            label="Review Plan approval"
            value={form.pipelineAutomation.reviewMode}
            onChange={(event) => setForm((current) => ({ ...current, pipelineAutomation: { ...current.pipelineAutomation, reviewMode: event.target.value as SettingsForm['pipelineAutomation']['reviewMode'] } }))}
            helperText="Manual waits for approval; conditional approves safe estimates; automatic approves every evaluated plan."
            select
            fullWidth
          >
            <MenuItem value="manual">Manual</MenuItem>
            <MenuItem value="conditional">Conditional</MenuItem>
            <MenuItem value="automatic">Automatic</MenuItem>
          </TextField>
          <FormControlLabel
            control={<Switch checked={form.pipelineAutomation.autoExecutionEnabled} onChange={(event) => setForm((current) => ({ ...current, pipelineAutomation: { ...current.pipelineAutomation, autoExecutionEnabled: event.target.checked } }))} />}
            label="Automatic execution of approved plans"
          />
          <FormControlLabel
            control={
              <Switch
                checked={form.pipelineAutomation.autoValidationEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      autoValidationEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Automatic validation"
          />
          <FormControlLabel
            control={
              <Switch
                checked={form.pipelineAutomation.autoPublisherEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      autoPublisherEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Automatic publisher and originals archive"
          />
          <FormControlLabel
            control={
              <Switch
                checked={form.pipelineAutomation.publisherOverwriteEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      publisherOverwriteEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Allow publisher overwrite"
          />

          <Typography color="text.secondary" variant="caption">
            Allows Publisher to replace existing media files and subtitle sidecars at
            their destination. Disabled by default.
          </Typography>
          <FormControlLabel
            control={
              <Switch
                checked={form.pipelineAutomation.publishedJobReconciliationEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      publishedJobReconciliationEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Published job reconciliation"
          />
          <Typography color="text.secondary" variant="caption">
            Periodically cleans staging for published jobs. It does not move recovered originals back to Archive.
          </Typography>
        </Stack>
      </CardContent>
    </Card>
  );
}

function workingHoursValue(value: Record<string, unknown> | undefined): WorkingHoursValue {
  const policy = value?.outsideWindowPolicy && typeof value.outsideWindowPolicy === 'object' ? value.outsideWindowPolicy as Record<string, unknown> : {};
  const windows = Array.isArray(value?.windows) ? value.windows.flatMap((entry) => {
    if (!entry || typeof entry !== 'object') return [];
    const item = entry as Record<string, unknown>;
    return [{ name: stringValue(item.name, ''), days: arrayValue(item.days, []).filter((day): day is string => typeof day === 'string'), start: stringValue(item.start, '23:00'), end: stringValue(item.end, '07:00') }];
  }) : [];
  return {
    enabled: booleanValue(value?.enabled, false), timezone: stringValue(value?.timezone, 'America/Mexico_City'), preset: stringValue(value?.preset, 'custom'), windows,
    outsideWindowPolicy: {
      startNewHeavyJobs: booleanValue(policy.startNewHeavyJobs, true), continueRunningJobs: booleanValue(policy.continueRunningJobs, true),
      allowAnalysisJobs: booleanValue(policy.allowAnalysisJobs, true), allowValidationJobs: booleanValue(policy.allowValidationJobs, true),
      allowPublisherJobs: booleanValue(policy.allowPublisherJobs, true), allowCleanupJobs: booleanValue(policy.allowCleanupJobs, true), allowLabPreviews: booleanValue(policy.allowLabPreviews, true),
    },
  };
}

function WorkingHoursCard({ value, saving, onSave, onDraftChange, draftOnly }: { value: WorkingHoursValue; saving: boolean; onSave: (value: WorkingHoursValue) => void; onDraftChange?: (value: WorkingHoursValue) => void; draftOnly?: boolean }) {
  const [draft, setDraft] = useSettingsDraftValue(value, identitySettingsValue<WorkingHoursValue>, onDraftChange);
  const updateWindow = (index: number, patch: Partial<WorkingWindow>) => setDraft((current) => ({ ...current, windows: current.windows.map((window, position) => position === index ? { ...window, ...patch } : window) }));
  return <Card><CardContent><Stack spacing={2}>
    <Stack><Typography variant="h3">Working Hours</Typography><Typography color="text.secondary" variant="body2">Heavy conversions only start inside allowed windows; running jobs continue.</Typography></Stack>
    <FormControlLabel control={<Switch checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked, preset: event.target.checked ? 'custom' : 'disabled' })} />} label={draft.enabled ? 'Schedule enabled' : 'Schedule disabled'} />
    <TextField label="Timezone" value={draft.timezone} onChange={(event) => setDraft({ ...draft, timezone: event.target.value })} helperText="IANA timezone, for example America/Mexico_City" />
    <FormControlLabel control={<Switch checked={draft.outsideWindowPolicy.startNewHeavyJobs} onChange={(event) => setDraft({ ...draft, outsideWindowPolicy: { ...draft.outsideWindowPolicy, startNewHeavyJobs: event.target.checked } })} />} label="Allow heavy jobs outside configured windows" />
    <Divider />
    {draft.windows.map((window, index) => <Grid container spacing={1.5} key={index} alignItems="center">
      <Grid size={{ xs: 12, md: 3 }}><TextField label="Window name" value={window.name} onChange={(event) => updateWindow(index, { name: event.target.value })} fullWidth /></Grid>
      <Grid size={{ xs: 12, md: 4 }}><TextField label="Days" value={window.days.join(', ')} onChange={(event) => updateWindow(index, { days: event.target.value.split(',').map((day) => day.trim().toLowerCase()).filter(Boolean) })} helperText="mon, tue, wed, thu, fri, sat, sun" fullWidth /></Grid>
      <Grid size={{ xs: 6, md: 2 }}><TextField label="Start" type="time" value={window.start} onChange={(event) => updateWindow(index, { start: event.target.value })} fullWidth /></Grid>
      <Grid size={{ xs: 6, md: 2 }}><TextField label="End" type="time" value={window.end} onChange={(event) => updateWindow(index, { end: event.target.value })} fullWidth /></Grid>
      <Grid size={{ xs: 12, md: 1 }}><IconButton color="error" onClick={() => setDraft((current) => ({ ...current, windows: current.windows.filter((_, position) => position !== index) }))}><DeleteIcon /></IconButton></Grid>
    </Grid>)}
    {!draft.windows.length ? <Alert severity="info">No conversion windows configured. With heavy jobs disabled outside windows, conversions will wait.</Alert> : null}
    <Stack direction="row" spacing={1}><Button startIcon={<AddIcon />} onClick={() => setDraft((current) => ({ ...current, windows: [...current.windows, { name: 'New window', days: ['mon', 'tue', 'wed', 'thu', 'fri'], start: '23:00', end: '07:00' }] }))}>Add window</Button>{!draftOnly ? <Button startIcon={<SaveIcon />} variant="contained" disabled={saving} onClick={() => onSave(draft)}>Save working hours</Button> : null}</Stack>
  </Stack></CardContent></Card>;
}

export type PipelineSettingsDomainProps = {
  form: SettingsForm;
  setForm: Dispatch<SetStateAction<SettingsForm>>;
  saving: boolean;
  onWorkingHoursDraftChange: (value: WorkingHoursValue) => void;
};

export function PipelineSettingsDomain({ form, setForm, saving, onWorkingHoursDraftChange }: PipelineSettingsDomainProps) {
  return <Stack spacing={2}>
    <PipelineAutomationCard form={form} setForm={setForm} />
    <WorkingHoursCard value={workingHoursValue(form.ordinarySettings.workingHours)} saving={saving} draftOnly onDraftChange={onWorkingHoursDraftChange} onSave={() => undefined} />
  </Stack>;
}
