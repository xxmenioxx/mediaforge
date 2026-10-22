import { useEffect, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import { Alert, Button, Card, CardContent, Checkbox, Chip, Divider, FormControl, FormControlLabel, FormLabel, Grid, InputAdornment, MenuItem, Radio, RadioGroup, Stack, Switch, Table, TableBody, TableCell, TableHead, TableRow, TextField, Tooltip, Typography } from '@mui/material';
import EditIcon from '@mui/icons-material/Edit';
import SaveIcon from '@mui/icons-material/Save';
import { api } from '../../api/client';
import type { RemoteExecutorProbe, RuntimeProfileOverride, RuntimeProfilesResponse, RuntimeProfileValues, RuntimeState } from '../../api/types';
import { useSettingsDraftValue } from '../../hooks/useSettingsDraftValue';
import { liveRuntimeStateLabel, runtimeRefreshSuccessMessage } from '../../utils/runtimeStatePresentation';
import { formatBytes } from './formatBytes';
import { arrayValue, booleanValue, numberValue, stringValue, workerProgressPercentError } from '../../utils/settingsPageHelpers';
import type { SettingsForm, WorkerJobStartPacingMode } from '../SettingsPage';

type SettingsCardProps = { form: SettingsForm; setForm: Dispatch<SetStateAction<SettingsForm>> };
function identitySettingsValue<T>(value: unknown): T { return value as T; }

type DirectPlayValue = { enabled: boolean; strategy: string; targetClients: string[]; minimumScore: number; enforcement: 'warn' | 'block' };

type RuntimePolicyValue = { schemaVersion: number; mode: 'automatic' | 'manual'; preferredProfile: string; fallbackProfile: string; overrides: Record<string, RuntimeProfileOverride> };

type RemoteStorageMapping = {
  localRoot: string;
  remoteRoot: string;
};

type RemoteExecutorConfig = {
  name: string;
  enabled: boolean;
  host: string;
  user: string;
  sshKeyPath: string;
  knownHostsPath: string;
  ffmpegPath: string;
  ffprobePath: string;
  storageMappings: RemoteStorageMapping[];
};

type RemoteExecutorsValue = {
  executors: RemoteExecutorConfig[];
};

export function runtimePolicyValue(value: Record<string, unknown> | undefined): RuntimePolicyValue {
  const overrides = value?.overrides && typeof value.overrides === 'object' ? value.overrides as Record<string, RuntimeProfileOverride> : {};
  const legacySelected = stringValue(value?.selectedProfile, 'desktop_balanced');
  return { schemaVersion: 2, mode: value?.mode === 'manual' ? 'manual' : 'automatic', preferredProfile: stringValue(value?.preferredProfile, value?.mode === 'manual' ? legacySelected : 'auto'), fallbackProfile: stringValue(value?.fallbackProfile, 'desktop_safe'), overrides };
}

const runtimeNumericFields: Array<{ key: keyof RuntimeProfileValues; label: string; suffix?: string }> = [
  { key: 'maxRunningJobs', label: 'Max running jobs' }, { key: 'maxVideoJobs', label: 'Max video jobs' },
  { key: 'maxSoftwareX265Jobs', label: 'Max software x265 jobs' }, { key: 'maxHardwareEncodeJobs', label: 'Max hardware encode jobs' },
  { key: 'maxAudioJobs', label: 'Max audio jobs' }, { key: 'maxLabJobs', label: 'Max Lab jobs' },
  { key: 'minFreeRamGb', label: 'Minimum free RAM', suffix: 'GB' }, { key: 'minFreeWorkGb', label: 'Work disk reserve', suffix: 'GB' },
  { key: 'minFreeLibraryGb', label: 'Library reserve', suffix: 'GB' }, { key: 'maxWorkspaceGb', label: 'Maximum workspace', suffix: 'GB' },
];

export function RuntimeProfilesCard({ catalog, value, detectedProfile, saving, onSave, onDraftChange, draftOnly }: { catalog?: RuntimeProfilesResponse; value: RuntimePolicyValue; detectedProfile: string; saving: boolean; onSave: (value: RuntimePolicyValue) => void; onDraftChange?: (value: RuntimePolicyValue) => void; draftOnly?: boolean }) {
  const [draft, setDraft] = useSettingsDraftValue(value, identitySettingsValue<RuntimePolicyValue>, onDraftChange);
  if (!catalog) return <Card><CardContent><Typography color="text.secondary">Loading runtime profiles…</Typography></CardContent></Card>;
  const selectedKey = draft.preferredProfile === 'auto' ? detectedProfile : draft.preferredProfile;
  const selected = catalog.profiles.find((profile) => profile.key === selectedKey) ?? catalog.profiles[0];
  if (!selected) return <Alert severity="error">No default runtime profiles are available.</Alert>;
  const override = draft.overrides[selected.key] ?? {};
  const effective = { ...selected.values, ...override };
  const setOverride = (key: keyof RuntimeProfileValues, next: number | boolean) => setDraft((current) => ({ ...current, overrides: { ...current.overrides, [selected.key]: { ...(current.overrides[selected.key] ?? {}), [key]: next } } }));
  const overridden = (key: keyof RuntimeProfileValues) => Object.prototype.hasOwnProperty.call(override, key);
  const numericField = ({ key, label, suffix }: { key: keyof RuntimeProfileValues; label: string; suffix?: string }) => { const changed = overridden(key); return <Stack spacing={1} sx={{ border: 1, borderColor: changed ? 'warning.main' : 'divider', bgcolor: changed ? 'rgba(255, 167, 38, 0.05)' : 'transparent', borderRadius: 1, p: 1.25, height: '100%' }}><Stack direction="row" spacing={0.5} alignItems="center"><Typography variant="body2" fontWeight={700}>{label}</Typography>{changed ? <Tooltip title="Changed from the profile default"><EditIcon color="warning" sx={{ fontSize: 16 }} /></Tooltip> : null}</Stack><TextField type="number" size="small" value={Number(effective[key])} inputProps={{ min: 1 }} InputProps={suffix ? { endAdornment: <InputAdornment position="end">{suffix}</InputAdornment> } : undefined} onChange={(event) => setOverride(key, Math.max(1, Number(event.target.value)))} fullWidth /></Stack>; };
  const booleanField = (key: 'allowDirectMode' | 'pauseWhenOnBattery' | 'preventSleepDuringJobs', label: string) => { const changed = overridden(key); return <Stack spacing={0.75} sx={{ border: 1, borderColor: changed ? 'warning.main' : 'divider', bgcolor: changed ? 'rgba(255, 167, 38, 0.05)' : 'transparent', borderRadius: 1, px: 1.25, py: 1, height: '100%' }}><Stack direction="row" spacing={0.5} alignItems="center"><Typography variant="body2" fontWeight={700}>{label}</Typography>{changed ? <Tooltip title="Changed from the profile default"><EditIcon color="warning" sx={{ fontSize: 16 }} /></Tooltip> : null}</Stack><Switch size="small" checked={Boolean(effective[key])} onChange={(event) => setOverride(key, event.target.checked)} sx={{ alignSelf: 'flex-start' }} /></Stack>; };
  return <Card><CardContent><Stack spacing={2.5}>
    <Stack><Typography variant="h3">Effective Runtime Profiles</Typography><Typography color="text.secondary" variant="body2">Profile defaults remain unchanged. Editing a field creates a custom value only for the selected base profile.</Typography></Stack>
    <Grid container spacing={2}>
      <Grid size={{ xs: 12, md: 4 }}><TextField select label="Detection mode" value={draft.mode} onChange={(event) => { const mode = event.target.value as RuntimePolicyValue['mode']; setDraft({ ...draft, mode, preferredProfile: mode === 'manual' && draft.preferredProfile === 'auto' ? detectedProfile : draft.preferredProfile }); }} fullWidth><MenuItem value="automatic">Automatic</MenuItem><MenuItem value="manual">Manual</MenuItem></TextField></Grid>
      <Grid size={{ xs: 12, md: 4 }}><TextField select label="Preferred base profile" value={draft.preferredProfile} onChange={(event) => setDraft({ ...draft, preferredProfile: event.target.value })} fullWidth><MenuItem value="auto">Auto recommended ({detectedProfile.replaceAll('_', ' ')})</MenuItem>{catalog.profiles.map((profile) => <MenuItem value={profile.key} key={profile.key}>{profile.name}</MenuItem>)}</TextField></Grid>
      <Grid size={{ xs: 12, md: 4 }}><TextField select label="Safe fallback" value={draft.fallbackProfile} onChange={(event) => setDraft({ ...draft, fallbackProfile: event.target.value })} fullWidth>{catalog.profiles.map((profile) => <MenuItem value={profile.key} key={profile.key}>{profile.name}</MenuItem>)}</TextField></Grid>
    </Grid>
    <Alert severity="info">Detected: {detectedProfile.replaceAll('_', ' ')} · Base: {selected.name} · Effective overrides: {Object.keys(override).length}</Alert>
    <Stack><Typography variant="h4">{selected.name}</Typography><Typography color="text.secondary" variant="body2">{selected.description}</Typography></Stack>
    <Grid container spacing={2} alignItems="flex-start">
      <Grid size={{ xs: 12, lg: 6 }}><Stack spacing={1}><Typography variant="h4">Concurrency</Typography><Grid container spacing={1}>{runtimeNumericFields.slice(0, 6).map((field) => <Grid size={{ xs: 12, sm: 6 }} key={field.key}>{numericField(field)}</Grid>)}</Grid></Stack></Grid>
      <Grid size={{ xs: 12, lg: 6 }}><Stack spacing={1}><Typography variant="h4">Resource reserves</Typography><Grid container spacing={1}>{runtimeNumericFields.slice(6).map((field) => <Grid size={{ xs: 12, sm: 6 }} key={field.key}>{numericField(field)}</Grid>)}</Grid></Stack></Grid>
    </Grid>
    <Stack spacing={1}><Typography variant="h4">Runtime behavior</Typography><Grid container spacing={1}><Grid size={{ xs: 12, sm: 6 }}>{booleanField('allowDirectMode', 'Allow direct workspace mode')}</Grid><Grid size={{ xs: 12, sm: 6 }}>{booleanField('pauseWhenOnBattery', 'Pause new jobs while on battery')}</Grid><Grid size={{ xs: 12, sm: 6 }}>{booleanField('preventSleepDuringJobs', 'Prevent sleep during active jobs (macOS)')}</Grid></Grid></Stack>
    <Stack direction="row" spacing={1}>{!draftOnly ? <Button startIcon={<SaveIcon />} variant="contained" disabled={saving} onClick={() => onSave(draft)}>Save effective runtime policy</Button> : null}<Button disabled={!Object.keys(override).length} onClick={() => setDraft((current) => { const overrides = { ...current.overrides }; delete overrides[selected.key]; return { ...current, overrides }; })}>Restore profile defaults</Button></Stack>
  </Stack></CardContent></Card>;
}

const directPlayClients = ['jellyfin_web', 'jellyfin_android_tv', 'jellyfin_roku', 'jellyfin_webos', 'apple_tv'];

export function directPlayValue(value: Record<string, unknown> | undefined): DirectPlayValue {
  return {
    enabled: booleanValue(value?.enabled, true), strategy: stringValue(value?.strategy, 'balanced'),
    targetClients: arrayValue(value?.targetClients, directPlayClients).filter((client): client is string => typeof client === 'string'),
    minimumScore: numberValue(value?.minimumScore, 70), enforcement: value?.enforcement === 'block' ? 'block' : 'warn',
  };
}

export function DirectPlayCard({ value, saving, onSave, onDraftChange, draftOnly }: { value: DirectPlayValue; saving: boolean; onSave: (value: DirectPlayValue) => void; onDraftChange?: (value: DirectPlayValue) => void; draftOnly?: boolean }) {
  const [draft, setDraft] = useSettingsDraftValue(value, identitySettingsValue<DirectPlayValue>, onDraftChange);
  const toggleClient = (client: string) => setDraft((current) => ({ ...current, targetClients: current.targetClients.includes(client) ? current.targetClients.filter((item) => item !== client) : [...current.targetClients, client] }));
  return <Card><CardContent><Stack spacing={2}>
    <Stack><Typography variant="h3">DirectPlay Policy</Typography><Typography color="text.secondary" variant="body2">Estimate playback compatibility from the planned output profile for each target client.</Typography></Stack>
    <FormControlLabel control={<Switch checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} />} label="Enable DirectPlay preflight" />
    <Grid container spacing={2}>
      <Grid size={{ xs: 12, md: 4 }}><TextField select label="Strategy" value={draft.strategy} disabled={!draft.enabled} onChange={(event) => setDraft({ ...draft, strategy: event.target.value })} fullWidth><MenuItem value="balanced">Balanced</MenuItem><MenuItem value="maximum_compatibility">Maximum compatibility</MenuItem><MenuItem value="modern_clients">Modern clients</MenuItem></TextField></Grid>
      <Grid size={{ xs: 12, md: 4 }}><TextField type="number" label="Minimum client score" value={draft.minimumScore} disabled={!draft.enabled} inputProps={{ min: 1, max: 100 }} onChange={(event) => setDraft({ ...draft, minimumScore: Math.min(100, Math.max(1, Number(event.target.value))) })} fullWidth /></Grid>
      <Grid size={{ xs: 12, md: 4 }}><TextField select label="Below threshold" value={draft.enforcement} disabled={!draft.enabled} onChange={(event) => setDraft({ ...draft, enforcement: event.target.value as DirectPlayValue['enforcement'] })} fullWidth><MenuItem value="warn">Warn only</MenuItem><MenuItem value="block">Require manual review</MenuItem></TextField></Grid>
    </Grid>
    <Stack><Typography variant="h4">Target clients</Typography><Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>{directPlayClients.map((client) => <FormControlLabel key={client} control={<Checkbox checked={draft.targetClients.includes(client)} disabled={!draft.enabled} onChange={() => toggleClient(client)} />} label={client.replaceAll('_', ' ')} />)}</Stack></Stack>
    <Alert severity="info">This is a preflight estimate. MVForge records it in the Execution Plan; definitive compatibility requires analyzing the final file.</Alert>
    {!draftOnly ? <Button startIcon={<SaveIcon />} variant="contained" disabled={saving || (draft.enabled && draft.targetClients.length === 0)} onClick={() => onSave(draft)}>Save DirectPlay policy</Button> : null}
  </Stack></CardContent></Card>;
}

export function WorkersCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Workers</Typography>
            <Typography color="text.secondary" variant="body2">
              Defaults used by local and future distributed workers.
            </Typography>
          </Stack>
          <TextField
            label="Default worker name"
            value={form.workers.defaultWorkerName}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                workers: { ...current.workers, defaultWorkerName: event.target.value },
              }))
            }
            fullWidth
          />
          <FormControlLabel
            control={
              <Switch
                checked={form.workers.autoWorkerEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    workers: { ...current.workers, autoWorkerEnabled: event.target.checked },
                  }))
                }
              />
            }
            label={form.workers.autoWorkerEnabled ? 'Auto worker enabled' : 'Auto worker disabled'}
          />
          <TextField
            label="Max concurrent jobs"
            type="number"
            value={form.workers.maxConcurrentJobs}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                workers: { ...current.workers, maxConcurrentJobs: Number(event.target.value) },
              }))
            }
            inputProps={{ min: 1, max: 8 }}
            fullWidth
          />
          <TextField
            label="Jobs per worker batch"
            type="number"
            value={form.workers.maxJobsPerBatch}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                workers: { ...current.workers, maxJobsPerBatch: Number(event.target.value) },
              }))
            }
            helperText="General limit of jobs the worker may start before resting. This is not tied to a folder batch."
            inputProps={{ min: 0, max: 1000 }}
            fullWidth
          />
          <TextField
            label="Batch rest time"
            type="number"
            value={form.workers.batchCooldownSeconds}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                workers: { ...current.workers, batchCooldownSeconds: Number(event.target.value) },
              }))
            }
            helperText="Seconds to wait after starting the configured number of jobs. 600 = 10 minutes."
            inputProps={{ min: 0, max: 86400 }}
            fullWidth
          />
          <FormControl>
            <FormLabel id="worker-start-pacing-label">Start next job by</FormLabel>
            <RadioGroup
              row
              aria-labelledby="worker-start-pacing-label"
              value={form.workers.jobStartPacingMode}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  workers: {
                    ...current.workers,
                    jobStartPacingMode: event.target.value as WorkerJobStartPacingMode,
                  },
                }))
              }
            >
              <FormControlLabel value="time" control={<Radio />} label="Time delay" />
              <FormControlLabel value="progress" control={<Radio />} label="Job progress" />
            </RadioGroup>
          </FormControl>
          {form.workers.jobStartPacingMode === 'progress' ? (
            <TextField
              label="Progress trigger"
              type="number"
              value={form.workers.startNextJobAtProgressPercent}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  workers: {
                    ...current.workers,
                    startNextJobAtProgressPercent: Number(event.target.value),
                  },
                }))
              }
              error={Boolean(workerProgressPercentError(form.workers.startNextJobAtProgressPercent))}
              helperText={workerProgressPercentError(form.workers.startNextJobAtProgressPercent) ?? 'Start the next job when the latest worker job reaches this percentage.'}
              inputProps={{ min: 1, max: 100, step: 1 }}
              InputProps={{ endAdornment: <InputAdornment position="end">%</InputAdornment> }}
              fullWidth
            />
          ) : (
            <TextField
              label="Delay between job starts"
              type="number"
              value={form.workers.delaySecondsBetweenJobs}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  workers: { ...current.workers, delaySecondsBetweenJobs: Number(event.target.value) },
                }))
              }
              helperText="Seconds to wait between claiming jobs."
              inputProps={{ min: 0, max: 86400 }}
              fullWidth
            />
          )}
          <FormControlLabel
            control={
              <Checkbox
                checked={form.workers.dryRunOnly}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    workers: { ...current.workers, dryRunOnly: event.target.checked },
                  }))
                }
              />
            }
            label="Dry-run only"
          />
        </Stack>
      </CardContent>
    </Card>
  );
}

export function remoteExecutorsValue(value: unknown): RemoteExecutorsValue {
  const record =
    value && typeof value === 'object'
      ? (value as Record<string, unknown>)
      : {};

  const executors = Array.isArray(record.executors)
    ? record.executors
    : [];

  return {
    executors: executors
      .filter(
        (item): item is Record<string, unknown> =>
          Boolean(item) && typeof item === 'object',
      )
      .map((item) => ({
        name: typeof item.name === 'string' ? item.name : '',
        enabled: item.enabled !== false,
        host: typeof item.host === 'string' ? item.host : '',
        user: typeof item.user === 'string' ? item.user : '',
        sshKeyPath:
          typeof item.sshKeyPath === 'string'
            ? item.sshKeyPath
            : '/etc/mvforge/ssh/id_ed25519',
        knownHostsPath:
          typeof item.knownHostsPath === 'string'
            ? item.knownHostsPath
            : '/etc/mvforge/ssh/known_hosts',
        ffmpegPath:
          typeof item.ffmpegPath === 'string'
            ? item.ffmpegPath
            : '/usr/local/bin/ffmpeg',
        ffprobePath:
          typeof item.ffprobePath === 'string'
            ? item.ffprobePath
            : '/usr/local/bin/ffprobe',
        storageMappings:
          Array.isArray(item.storageMappings)
            ? item.storageMappings
                .filter(
                  (
                    mapping,
                  ): mapping is {
                    localRoot: string;
                    remoteRoot: string;
                  } =>
                    typeof mapping === 'object' &&
                    mapping !== null &&
                    typeof mapping.localRoot === 'string' &&
                    typeof mapping.remoteRoot === 'string',
                )
                .map((mapping) => ({
                  localRoot: mapping.localRoot,
                  remoteRoot: mapping.remoteRoot,
                }))
            : typeof item.storageRoot === 'string'
              ? [
                  {
                    localRoot: '/media/raw',
                    remoteRoot: `${item.storageRoot}/raw`,
                  },
                  {
                    localRoot: '/media/staging',
                    remoteRoot: `${item.storageRoot}/staging`,
                  },
                ]
              : [],
      })),
  };
}

export function RemoteExecutorsCard({
  value,
  saving,
  onSave,
}: {
  value: RemoteExecutorsValue;
  saving: boolean;
  onSave: (value: RemoteExecutorsValue) => void;
}) {
  const [draft, setDraft] = useState<RemoteExecutorsValue>(value);
  const [probeResult, setProbeResult] = useState<RemoteExecutorProbe | null>(
    null,
  );
  const [probing, setProbing] = useState(false);

  useEffect(() => {
    setDraft(value);
  }, [value]);

  const executor = draft.executors[0] ?? {
    name: 'macbook',
    enabled: true,
    host: '',
    user: 'anuelvs',
    sshKeyPath: '/etc/mvforge/ssh/id_ed25519',
    knownHostsPath: '/etc/mvforge/ssh/known_hosts',
    ffmpegPath: '/usr/local/bin/ffmpeg',
    ffprobePath: '/usr/local/bin/ffprobe',
    storageMappings: [
      {
        localRoot: '/media/raw',
        remoteRoot: '/Volumes/docker/nas-media-stack/work/mediaforge/raw',
      },
      {
        localRoot: '/media/staging',
        remoteRoot: '/Volumes/docker/nas-media-stack/work/mediaforge/staging',
      },
    ],
  };

  function updateExecutor(
    key: keyof RemoteExecutorConfig,
    nextValue: string | boolean,
  ) {
    setDraft({
      executors: [
        {
          ...executor,
          [key]: nextValue,
        },
      ],
    });
  }

  function updateStorageMapping(
    index: number,
    key: keyof RemoteStorageMapping,
    nextValue: string,
  ) {
    setDraft({
      executors: [
        {
          ...executor,
          storageMappings: executor.storageMappings.map(
            (mapping, mappingIndex) =>
              mappingIndex === index
                ? {
                    ...mapping,
                    [key]: nextValue,
                  }
                : mapping,
          ),
        },
      ],
    });
  }
  async function probe() {
    setProbing(true);
    setProbeResult(null);

    try {
      const result = await api.probeRemoteExecutor(executor);
      setProbeResult(result);
    } catch (error) {
      setProbeResult({
        name: executor.name,
        reachable: false,
        ffmpegAvailable: false,
        ffprobeAvailable: false,
        videoToolbox: false,
        storageAccessible: false,
        error:
          error instanceof Error
            ? error.message
            : 'Remote executor probe failed.',
      });
    } finally {
      setProbing(false);
    }
  }

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Remote Executors</Typography>
            <Typography color="text.secondary" variant="body2">
              Optional machines that can provide additional media processing
              capacity.
            </Typography>
          </Stack>

          <FormControlLabel
            control={
              <Switch
                checked={executor.enabled}
                onChange={(event) =>
                  updateExecutor('enabled', event.target.checked)
                }
              />
            }
            label="Enabled"
          />

          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 4 }}>
              <TextField
                label="Name"
                value={executor.name}
                onChange={(event) =>
                  updateExecutor('name', event.target.value)
                }
                fullWidth
              />
            </Grid>

            <Grid size={{ xs: 12, md: 4 }}>
              <TextField
                label="Host"
                value={executor.host}
                onChange={(event) =>
                  updateExecutor('host', event.target.value)
                }
                fullWidth
              />
            </Grid>

            <Grid size={{ xs: 12, md: 4 }}>
              <TextField
                label="User"
                value={executor.user}
                onChange={(event) =>
                  updateExecutor('user', event.target.value)
                }
                fullWidth
              />
            </Grid>

            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="FFmpeg path"
                value={executor.ffmpegPath}
                onChange={(event) =>
                  updateExecutor('ffmpegPath', event.target.value)
                }
                fullWidth
              />
            </Grid>

            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="FFprobe path"
                value={executor.ffprobePath}
                onChange={(event) =>
                  updateExecutor('ffprobePath', event.target.value)
                }
                fullWidth
              />
            </Grid>

           <Grid size={{ xs: 12 }}>
            <Typography variant="h4">Storage mappings</Typography>
          </Grid>

          <Grid size={{ xs: 12, md: 6 }}>
            <TextField
              label="NAS Raw path"
              value={executor.storageMappings[0]?.localRoot ?? ''}
              onChange={(event) =>
                updateStorageMapping(0, 'localRoot', event.target.value)
              }
              helperText="Path visible to the MVForge backend."
              fullWidth
            />
          </Grid>

          <Grid size={{ xs: 12, md: 6 }}>
            <TextField
              label="Mac Raw path"
              value={executor.storageMappings[0]?.remoteRoot ?? ''}
              onChange={(event) =>
                updateStorageMapping(0, 'remoteRoot', event.target.value)
              }
              helperText="Equivalent path visible to the remote executor."
              fullWidth
            />
          </Grid>

          <Grid size={{ xs: 12, md: 6 }}>
            <TextField
              label="NAS Staging path"
              value={executor.storageMappings[1]?.localRoot ?? ''}
              onChange={(event) =>
                updateStorageMapping(1, 'localRoot', event.target.value)
              }
              helperText="Staging path visible to MVForge."
              fullWidth
            />
          </Grid>

          <Grid size={{ xs: 12, md: 6 }}>
            <TextField
              label="Mac Staging path"
              value={executor.storageMappings[1]?.remoteRoot ?? ''}
              onChange={(event) =>
                updateStorageMapping(1, 'remoteRoot', event.target.value)
              }
              helperText="Equivalent staging path visible to the remote executor."
              fullWidth
            />
          </Grid>

            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="SSH key path"
                value={executor.sshKeyPath}
                onChange={(event) =>
                  updateExecutor('sshKeyPath', event.target.value)
                }
                fullWidth
              />
            </Grid>

            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="Known hosts path"
                value={executor.knownHostsPath}
                onChange={(event) =>
                  updateExecutor('knownHostsPath', event.target.value)
                }
                fullWidth
              />
            </Grid>
          </Grid>

          <Stack direction="row" spacing={1}>
            <Button
              variant="contained"
              startIcon={<SaveIcon />}
              disabled={saving}
              onClick={() => onSave(draft)}
            >
              Save Executor
            </Button>

            <Button
              variant="outlined"
              disabled={probing || !executor.host}
              onClick={() => void probe()}
            >
              {probing ? 'Probing…' : 'Probe'}
            </Button>
          </Stack>

          {probeResult ? (
            <Alert
              severity={
                probeResult.reachable &&
                probeResult.ffmpegAvailable &&
                probeResult.ffprobeAvailable &&
                probeResult.storageAccessible
                  ? 'success'
                  : 'warning'
              }
            >
              {probeResult.reachable
                ? `${probeResult.hostname ?? executor.name} · FFmpeg ${
                    probeResult.ffmpegAvailable ? 'OK' : 'missing'
                  } · FFprobe ${
                    probeResult.ffprobeAvailable ? 'OK' : 'missing'
                  } · VideoToolbox ${
                    probeResult.videoToolbox ? 'available' : 'unavailable'
                  } · Storage ${
                    probeResult.storageAccessible ? 'OK' : 'unavailable'
                  }`
                : probeResult.error ?? 'Executor is unreachable.'}
            </Alert>
          ) : null}
        </Stack>
      </CardContent>
    </Card>
  );
}

function safeRuntimeList(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function safeRuntimeRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? value as Record<string, unknown> : {};
}

function runtimeEncoder(value: unknown) {
  const encoder = safeRuntimeRecord(value);
  return {
    listed: encoder.listed === true,
    usable: encoder.usable === true,
    reason: typeof encoder.reason === 'string' ? encoder.reason : '',
  };
}

function summarizeModeReason(reason: string) {
  const lower = reason.toLowerCase();

  if (lower.includes('invalid video parameters')) {
    return 'Unsupported parameter combination';
  }

  if (lower.includes('requested qsv rate control la_icq but encoder used icq')) {
    return 'LA-ICQ is not supported by this runtime';
  }

  if (lower.includes('encoder is not listed')) {
    return 'Encoder is not available';
  }

  if (lower.includes('unsupported')) {
    return 'Unsupported by this encoder/runtime';
  }

  if (lower.includes('skipped because')) {
    return reason;
  }

  return 'Capability probe failed';
}

function runtimeEncoderCapabilities(name: string, value: unknown) {
  const encoder = safeRuntimeRecord(value);
  const tested = safeRuntimeRecord(encoder.testedModes);
  const reasons = safeRuntimeRecord(encoder.modeReasons);
  const labels: Array<[string, string]> = [
    ['qsvIcqMain8', 'QSV ICQ Main'],
    ['qsvIcqMain10', 'QSV ICQ Main10'],

    ['qsvCqpMain8', 'QSV CQP Main'],
    ['qsvCqpMain10', 'QSV CQP Main10'],

    ['qsvVbrMain8', 'QSV VBR Main'],
	['qsvVbrExtBrcMain8', 'QSV VBR + ExtBRC Main'],
	['qsvVbrLookAheadMain8', 'QSV VBR + LookAhead Main'],
	['qsvVbrAdvancedMain8', 'QSV VBR Advanced Main'],
    ['qsvVbrMain10', 'QSV VBR Main10'],
    ['qsvVbrExtBrcMain10', 'QSV VBR + ExtBRC Main10'],
    ['qsvVbrLookAheadMain10', 'QSV VBR + LookAhead Main10'],
    ['qsvVbrAdvancedMain10', 'QSV VBR Advanced Main10'],

    ['qsvCbrMain8', 'QSV CBR Main'],
	['qsvCbrExtBrcMain8', 'QSV CBR + ExtBRC Main'],
	['qsvCbrLookAheadMain8', 'QSV CBR + LookAhead Main'],
    ['qsvCbrMain10', 'QSV CBR Main10'],
    ['qsvCbrExtBrcMain10', 'QSV CBR + ExtBRC Main10'],
    ['qsvCbrLookAheadMain10', 'QSV CBR + LookAhead Main10'],

    ['qsvAdaptiveIMain10', 'QSV Adaptive I Main10'],
    ['qsvAdaptiveBMain10', 'QSV Adaptive B Main10'],
    ['qsvAdaptiveIMain8', 'QSV Adaptive I Main8'],
    ['qsvAdaptiveBMain8', 'QSV Adaptive B Main8'],

    ['qsvLowPowerMain8', 'QSV Low Power Main'],
    ['qsvLowPowerMain10', 'QSV Low Power Main10'],

    ['qsvLaIcqMain10', 'QSV LA-ICQ Main10'],
	['qsvLaIcqMain8', 'QSV LA-ICQ Main'],

    ['videoToolboxMain', 'VT Main'],
    ['videoToolboxMain10', 'VT Main10'],
    ['videoToolboxBFramesMain', 'VT B-frames Main'],
    ['videoToolboxBFramesMain10', 'VT B-frames Main10'],
    ['videoToolboxPowerEfficientMain', 'VT power efficient Main'],
    ['videoToolboxPowerEfficientMain10', 'VT power efficient Main10'],
  ];
  const isQSV = name.endsWith('_qsv');
  const isVideoToolbox = name.endsWith('_videotoolbox');

  const relevantLabels = labels.filter(([key]) => {
    if (key.startsWith('qsv')) {
      return isQSV;
    }

    if (key.startsWith('videoToolbox')) {
      return isVideoToolbox;
    }

    return true;
  });
  return relevantLabels
  .filter(([key]) =>
    typeof tested[key] === 'boolean' ||
    typeof encoder[key] === 'boolean'
  )
  .map(([key, label]) => ({
    key,
    label,
    passed: tested[key] === true || encoder[key] === true,
    reason: typeof reasons[key] === 'string' ? reasons[key] as string : '',
  }));
}

function runtimeDisk(value: unknown): { path: string; type: string; totalBytes: number; availableBytes: number } {
  const disk = value && typeof value === 'object' ? value as Record<string, unknown> : {};
  return {
    path: typeof disk.path === 'string' ? disk.path : 'Unknown',
    type: typeof disk.type === 'string' ? disk.type : 'unknown',
    totalBytes: typeof disk.totalBytes === 'number' ? disk.totalBytes : 0,
    availableBytes: typeof disk.availableBytes === 'number' ? disk.availableBytes : 0,
  };
}

export type RuntimeSnapshotCardProps = {
  snapshot?: RuntimeState;
  loading: boolean;
  error: boolean;
  errorMessage: string;
  refreshPending: boolean;
  refreshSucceeded: boolean;
  refreshErrorMessage?: string;
  onRefresh: () => void;
};
export function RuntimeSnapshotCard({ snapshot, loading, error, errorMessage, refreshPending, refreshSucceeded, refreshErrorMessage, onRefresh }: RuntimeSnapshotCardProps) {
  return <Card>
                    <CardContent>
                      <Stack spacing={1.5}>
                        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                          <Stack>
                            <Typography variant="h3">Scheduler runtime</Typography>
                            <Typography color="text.secondary" variant="body2">
                              Live host state used to select the execution policy.
                            </Typography>
                          </Stack>
                          <Button onClick={() => onRefresh()} disabled={refreshPending}>
                            {refreshPending ? 'Detecting…' : 'Refresh detection'}
                          </Button>
                        </Stack>
                        {error ? (
                          <Alert severity="warning">
                            Unable to load runtime detection: {errorMessage}
                          </Alert>
                        ) : loading ? (
                          <Typography color="text.secondary">Detecting runtime capabilities…</Typography>
                        ) : snapshot ? (
                          <Stack spacing={2}>
                            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                              <Chip label={`Effective: ${snapshot.selectedProfile}`} color="primary" />
                              <Chip label={`Detected: ${snapshot.recommendedProfile}`} />
                              <Chip label={`Preferred: ${snapshot.preferredProfile || 'auto'}`} />
                              {snapshot.appliedOverrides.length ? <Chip label={`${snapshot.appliedOverrides.length} overrides`} color="warning" /> : null}
                              <Chip label={`${snapshot.os}/${snapshot.architecture}`} />
                              <Chip label={`${snapshot.cpuCores} CPU cores`} />
                              <Chip label={`Load ${snapshot.cpuLoad1.toFixed(2)}`} />
                              <Chip label={`${formatBytes(snapshot.totalMemoryBytes)} RAM total`} />
                              <Chip label={`${formatBytes(snapshot.availableMemoryBytes)} RAM available`} />
                              <Chip label={snapshot.container ? 'Container' : 'Native host'} />
                              <Chip label={snapshot.batteryPresent ? `${snapshot.onBattery ? 'Battery' : 'AC'} · ${snapshot.batteryPercent}%` : `Power: ${snapshot.powerSource}`} color={snapshot.onBattery ? 'warning' : 'default'} />
                            </Stack>

                            <Divider />
                            <Typography variant="h4">Storage</Typography>
                            <Table size="small">
                              <TableHead><TableRow><TableCell>Role</TableCell><TableCell>Path</TableCell><TableCell>Type</TableCell><TableCell>Total</TableCell><TableCell>Available</TableCell></TableRow></TableHead>
                              <TableBody>
                                {Object.entries(safeRuntimeRecord(snapshot.disks)).map(([role, rawDisk]) => {
                                  const disk = runtimeDisk(rawDisk);
                                  return <TableRow key={role}><TableCell>{role}</TableCell><TableCell sx={{ wordBreak: 'break-all' }}>{disk.path}</TableCell><TableCell>{disk.type}</TableCell><TableCell>{formatBytes(disk.totalBytes)}</TableCell><TableCell>{formatBytes(disk.availableBytes)}</TableCell></TableRow>;
                                })}
                              </TableBody>
                            </Table>

                            <Typography variant="h4">FFmpeg encoders</Typography>
                            <Table size="small">
                              <TableHead><TableRow><TableCell>Encoder</TableCell><TableCell>Listed</TableCell><TableCell>Usable</TableCell><TableCell>Verified combinations</TableCell><TableCell>Diagnostic</TableCell></TableRow></TableHead>
                              <TableBody>
                                {Object.entries(safeRuntimeRecord(snapshot.encoders)).map(([name, rawEncoder]) => {
                                  const encoder = runtimeEncoder(rawEncoder);
                                  const modes = runtimeEncoderCapabilities(name, rawEncoder);
                                  const failedModes = modes.filter((mode) => !mode.passed && mode.reason);
                                  return <TableRow key={name}><TableCell>{name}</TableCell><TableCell>{encoder.listed ? 'Yes' : 'No'}</TableCell><TableCell><Chip size="small" color={encoder.usable ? 'success' : 'default'} label={encoder.usable ? 'Usable' : 'Unavailable'} /></TableCell><TableCell><Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>{modes.map((mode) => <Chip key={mode.key} size="small" variant="outlined" color={mode.passed ? 'success' : 'warning'} label={`${mode.label} · ${mode.passed ? 'passed' : 'failed'}`} title={mode.reason || undefined} />)}{modes.length === 0 ? '—' : null}</Stack></TableCell>
                                  <TableCell>
                                    <Stack spacing={0.5}>
                                      {encoder.reason ? (
                                        <Typography variant="body2">{encoder.reason}</Typography>
                                      ) : failedModes.length ? (
                                          failedModes.map((mode) => (
                                          <Tooltip key={mode.key} title={mode.reason || ''} arrow>
                                            <Typography
                                              variant="caption"
                                              color="text.secondary"
                                              sx={{ display: 'block', cursor: mode.reason ? 'help' : 'default' }}
                                            >
                                              <strong>{mode.label}:</strong>{' '}
                                              {summarizeModeReason(mode.reason)}
                                            </Typography>
                                          </Tooltip>
                                        ))
                                      ) : (
                                        <Typography variant="body2">Passed capability check</Typography>
                                      )}
                                    </Stack>
                                  </TableCell>
                                  </TableRow>;
                                })}
                              </TableBody>
                            </Table>

                            {safeRuntimeList(snapshot.selectionReasons).length ? <Alert severity="info">{safeRuntimeList(snapshot.selectionReasons).map(String).join(' · ')}</Alert> : null}
                            {safeRuntimeList(snapshot.warnings).map((warning, index) => <Alert key={index} severity="warning">{String(warning)}</Alert>)}
                            <Typography color="text.secondary" variant="caption">{liveRuntimeStateLabel(snapshot.detectedAt, (value) => new Date(value).toLocaleString())}</Typography>
                          </Stack>
                        ) : (
                          <Typography color="text.secondary">No current runtime state has been detected yet.</Typography>
                        )}
                        {refreshSucceeded ? <Alert severity="success">{runtimeRefreshSuccessMessage}</Alert> : null}
                        {refreshErrorMessage ? <Alert severity="error">Refresh failed: {refreshErrorMessage}</Alert> : null}
                      </Stack>
                    </CardContent>
                  </Card>
}
