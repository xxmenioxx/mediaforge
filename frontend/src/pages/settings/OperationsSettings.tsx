import { useEffect, useState } from 'react';
import type { Dispatch, SetStateAction } from 'react';
import { Alert, Box, Button, Card, CardContent, Checkbox, Chip, FormControlLabel, Grid, MenuItem, Stack, Switch, Table, TableBody, TableCell, TableHead, TableRow, TextField, Typography } from '@mui/material';
import SaveIcon from '@mui/icons-material/Save';
import type { HousekeepingReport, SchedulerRecoveryReport } from '../../api/types';
import { useSettingsDraftValue } from '../../hooks/useSettingsDraftValue';
import { formatBytes } from './formatBytes';
import type { HousekeepingValue, SettingsForm } from '../SettingsPage';

type SettingsCardProps = { form: SettingsForm; setForm: Dispatch<SetStateAction<SettingsForm>> };

function identitySettingsValue<T>(value: unknown): T { return value as T; }

export type OperationsSettingsDomainProps = {
  form: SettingsForm;
  setForm: Dispatch<SetStateAction<SettingsForm>>;
  recovery: { report?: SchedulerRecoveryReport; loading: boolean; error: boolean; running: boolean; runError: Error | null; onRun: () => void };
  housekeeping: { value: HousekeepingValue; savedValue: HousekeepingValue; saving: boolean; preview?: HousekeepingReport; result?: HousekeepingReport; busy: boolean; onDraftChange: (value: HousekeepingValue) => void; onPreview: () => void; onRun: (previewId: string) => void };
};

function SchedulerRecoveryCard({ report, loading, error, running, runError, onRun }: { report?: SchedulerRecoveryReport; loading: boolean; error: boolean; running: boolean; runError: Error | null; onRun: () => void }) {
  return <Card><CardContent><Stack spacing={1.5}>
    <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
      <Stack><Typography variant="h3">Scheduler recovery</Typography><Typography color="text.secondary" variant="body2">Conservative reconciliation for jobs, reservations, workers and workspaces after a restart.</Typography></Stack>
      <Button onClick={onRun} disabled={running}>{running ? 'Reconciling…' : 'Run reconciliation'}</Button>
    </Stack>
    {report ? <><Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap><Chip label={`${report.interruptedJobs} interrupted jobs`} /><Chip label={`${report.reservationsReleased} reservations released`} /><Chip label={`${report.workersMarkedOffline} workers offline`} /><Chip label={`${report.partialOutputsPreserved} partial outputs preserved`} /><Chip label={`${report.orphanWorkspacePaths.length} orphan workspaces`} color={report.orphanWorkspacePaths.length ? 'warning' : 'success'} /><Chip label={`${report.missingCompletedOutputs} missing outputs`} color={report.missingCompletedOutputs ? 'error' : 'default'} /></Stack>{report.orphanWorkspacePaths.map((path) => <Alert severity="warning" key={path}>{path}</Alert>)}<Typography color="text.secondary" variant="caption">Last reconciliation: {new Date(report.ranAt).toLocaleString()}</Typography></> : error ? <Alert severity="warning">No recovery report is available.</Alert> : loading ? <Typography color="text.secondary">Loading recovery report…</Typography> : null}
    {runError ? <Alert severity="error">Recovery failed: {runError.message}</Alert> : null}
  </Stack></CardContent></Card>;
}

const housekeepingKindLabels: Record<HousekeepingReport['candidates'][number]['kind'], string> = {
  job_workspace: 'Job workspace', job_log: 'Job log', stage_artifact: 'Stage artifact',
  empty_directory: 'Empty directory', test_encode: 'Test Encode', runtime_snapshot: 'Runtime snapshot',
};

export function HousekeepingCard({ value, savedValue = value, saving, preview, result, busy, onSave, onDraftChange, draftOnly, onPreview, onRun }: { value: HousekeepingValue; savedValue?: HousekeepingValue; saving: boolean; preview?: HousekeepingReport; result?: HousekeepingReport; busy: boolean; onSave: (value: HousekeepingValue) => void; onDraftChange?: (value: HousekeepingValue) => void; draftOnly?: boolean; onPreview: () => void; onRun: (previewId: string) => void }) {
  const [draft, setDraft] = useSettingsDraftValue(value, identitySettingsValue<HousekeepingValue>, onDraftChange);
  const [now, setNow] = useState(0);
  const [category, setCategory] = useState<'all' | HousekeepingReport['candidates'][number]['kind']>('all');
  const report = result ?? preview;
  const activePreview = preview?.dryRun ? preview : undefined;
  useEffect(() => {
    const refreshTimer = window.setTimeout(() => setNow(Date.now()), 0);
    const expiresAt = activePreview?.previewExpiresAt;
    const expiryTimer = expiresAt
      ? window.setTimeout(() => setNow(Date.now()), Math.max(Date.parse(expiresAt) - Date.now() + 1, 0))
      : undefined;
    return () => {
      window.clearTimeout(refreshTimer);
      if (expiryTimer !== undefined) window.clearTimeout(expiryTimer);
    };
  }, [activePreview?.previewExpiresAt, preview, result]);
  const validPreview = Boolean(activePreview?.previewId && activePreview.previewExpiresAt && now > 0 && Date.parse(activePreview.previewExpiresAt) > now);
  const safeCandidates = activePreview?.candidates.filter((item) => item.safeToDelete) ?? [];
  const policyDirty = JSON.stringify(draft) !== JSON.stringify(savedValue);
  const recoverableBytes = report?.summaries.reduce((total, summary) => total + summary.recoverableBytes, 0) ?? 0;
  const daysField = (key: keyof HousekeepingValue, label: string) => <TextField type="number" label={label} value={draft[key] as number} inputProps={{ min: 0 }} onChange={(event) => setDraft({ ...draft, [key]: Math.max(0, Number(event.target.value)) })} fullWidth />;
  const candidateAge = (modifiedAt: string) => {
    if (!now) return 'Unknown';
    const ageDays = Math.max(0, Math.floor((now - Date.parse(modifiedAt)) / 86_400_000));
    return Number.isFinite(ageDays) ? `${ageDays} days` : 'Unknown';
  };
  const rows = (report?.candidates ?? []).filter((item) => category === 'all' || item.kind === category);
  return <Card><CardContent><Stack spacing={2}>
    <Stack><Typography variant="h3">Workspace Housekeeping</Typography><Typography color="text.secondary" variant="body2">Review cleanup candidates before removal. Only items marked eligible can be removed.</Typography></Stack>
    <FormControlLabel control={<Switch checked={draft.autoEnabled} onChange={(event) => setDraft({ ...draft, autoEnabled: event.target.checked })} />} label="Enable automatic housekeeping" />
    <Grid container spacing={2}>
      <Grid size={{ xs: 12, md: 3 }}><TextField type="number" label="Interval (hours)" value={draft.intervalHours} inputProps={{ min: 1 }} onChange={(event) => setDraft({ ...draft, intervalHours: Math.max(1, Number(event.target.value)) })} fullWidth /></Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('failedRetentionDays', 'Failed workspace retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('canceledRetentionDays', 'Canceled workspace retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('orphanRetentionDays', 'Orphan workspace retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('publishedWorkspaceRetentionDays', 'Published workspace retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('publishedJobLogRetentionDays', 'Published job log retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('failedJobLogRetentionDays', 'Failed job log retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('canceledJobLogRetentionDays', 'Canceled job log retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('orphanJobLogRetentionDays', 'Orphan job log retention (days)')}</Grid>
    </Grid>
    <FormControlLabel control={<Switch checked={draft.emptyRawDirectoriesEnabled} onChange={(event) => setDraft({ ...draft, emptyRawDirectoriesEnabled: event.target.checked })} />} label="Remove eligible empty RAW directories" />
    <FormControlLabel control={<Switch checked={draft.orphanStageArtifactsEnabled} onChange={(event) => setDraft({ ...draft, orphanStageArtifactsEnabled: event.target.checked })} />} label="Remove eligible orphan stage artifacts" />
    <Alert severity="info">Preview authorizes one short-lived cleanup run. Blocked candidates stay visible with their reason.</Alert>
    <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
      {!draftOnly ? <Button startIcon={<SaveIcon />} variant="contained" disabled={saving || busy} onClick={() => onSave(draft)}>Save policy</Button> : null}
      <Button variant="outlined" disabled={busy || saving || policyDirty} onClick={onPreview}>Preview cleanup</Button>
      <Button color="error" variant="outlined" disabled={busy || saving || policyDirty || !validPreview || safeCandidates.length === 0} onClick={() => { if (activePreview?.previewId) onRun(activePreview.previewId); }}>Remove previewed candidates</Button>
    </Stack>
    {policyDirty ? <Alert severity="info">Save policy changes before previewing cleanup.</Alert> : null}
    {preview?.previewId && !validPreview ? <Alert severity="warning">This preview has expired. Run Preview cleanup again before removing candidates.</Alert> : null}
    {report ? <Stack spacing={1}>
      <Typography variant="h4">{report.dryRun ? 'Cleanup preview' : 'Cleanup result'}</Typography>
      {report.previewExpiresAt ? <Typography variant="body2" color="text.secondary">Preview expires {new Date(report.previewExpiresAt).toLocaleString()}</Typography> : null}
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>{report.summaries.map((summary) => <Chip key={summary.kind} label={`${housekeepingKindLabels[summary.kind]}: ${summary.candidateCount} · ${formatBytes(summary.recoverableBytes)}`} />)}{!report.dryRun && report.outcome ? <Chip label={`Outcome: ${report.outcome.replaceAll('_', ' ')}`} color={report.outcome === 'cleaned' || report.outcome === 'already_absent' ? 'success' : report.outcome === 'failed' ? 'error' : report.outcome === 'partial' ? 'warning' : 'default'} /> : null}<Chip label={`${report.dryRun ? 'Recoverable' : 'Recovered'}: ${formatBytes(report.dryRun ? recoverableBytes : report.recoveredBytes)}`} /><Chip label={`${report.removedPaths.length} removed`} color={report.removedPaths.length ? 'success' : 'default'} /></Stack>
      <TextField select label="Category" value={category} onChange={(event) => setCategory(event.target.value as typeof category)} fullWidth>
        <MenuItem value="all">All categories ({report.candidates.length})</MenuItem>
        {report.summaries.map((summary) => <MenuItem key={summary.kind} value={summary.kind}>{housekeepingKindLabels[summary.kind]} ({summary.candidateCount})</MenuItem>)}
      </TextField>
      {rows.length ? <Box sx={{ overflowX: 'auto' }}><Table size="small" aria-label="Housekeeping candidates"><TableHead><TableRow>
        <TableCell>Category</TableCell><TableCell>Path</TableCell><TableCell>Owner / job</TableCell><TableCell>Reason</TableCell><TableCell>Age</TableCell><TableCell>Size</TableCell><TableCell>Eligibility</TableCell>
      </TableRow></TableHead><TableBody>{rows.map((item) => <TableRow key={item.candidateId}>
        <TableCell>{housekeepingKindLabels[item.kind]}</TableCell><TableCell sx={{ overflowWrap: 'anywhere' }}>{item.path}</TableCell>
        <TableCell>{[item.ownership, item.jobId !== undefined ? `Job ${item.jobId}` : undefined, item.operationId, item.testEncodeId !== undefined ? `Test Encode ${item.testEncodeId}` : undefined].filter(Boolean).join(' · ') || '—'}</TableCell>
        <TableCell sx={{ overflowWrap: 'anywhere' }}><Stack spacing={0.5}>
          <Typography variant="body2">{item.reason}</Typography>
          {item.maintenance ? <Stack spacing={0.25}>
            <Typography variant="caption">Asset: {item.maintenance.assetPath}</Typography>
            <Typography variant="caption">{item.maintenance.assetRecordId ? `AssetRecord #${item.maintenance.assetRecordId} · ${item.maintenance.assetRecordStatus || 'status unknown'} · ${item.maintenance.assetRecordMissing ? 'marked missing' : 'not marked missing'}` : 'AssetRecord identity unavailable'}</Typography>
            <Typography variant="caption">Operation: {item.maintenance.status} / {item.maintenance.phase}{item.maintenance.finishedAt ? ` · finished ${new Date(item.maintenance.finishedAt).toLocaleString()}` : ' · finished time unknown'}</Typography>
            <Typography variant="caption">Temporary file: {item.maintenance.temporaryPresence}{item.maintenance.temporarySizeKnown ? ` · ${formatBytes(item.sizeBytes)}` : ' · size unknown'}</Typography>
            <Typography variant="caption">Backup: {item.maintenance.backupPath || 'path not recorded'} · {item.maintenance.backupPresence} · {item.maintenance.recoveryState}</Typography>
            {item.maintenance.resultFingerprint ? <Typography variant="caption" sx={{ overflowWrap: 'anywhere' }}>Result fingerprint: {item.maintenance.resultFingerprint}</Typography> : null}
            <Typography variant="caption" sx={{ overflowWrap: 'anywhere' }}>Current asset fingerprint: {item.maintenance.assetFingerprintState}{item.maintenance.currentAssetFingerprint ? ` · ${item.maintenance.currentAssetFingerprint}` : ''}</Typography>
            {item.maintenance.warning ? <Typography variant="caption" color="warning.main">Operation warning: {item.maintenance.warning}</Typography> : null}
          </Stack> : null}
          {item.protectionReferences?.map((reference) => <Typography key={`${reference.source}-${reference.recordId}-${reference.field}-${reference.path}`} variant="caption">
            {reference.source} #{reference.recordId} · {reference.field}: {reference.path} · {reference.state} — {reference.reason}
          </Typography>)}
          {item.protectionReferenceCount && item.protectionReferences && item.protectionReferenceCount > item.protectionReferences.length ? <Typography variant="caption">{item.protectionReferenceCount - item.protectionReferences.length} additional references</Typography> : null}
        </Stack></TableCell>
        <TableCell>{item.maintenance?.finishedAt ? candidateAge(item.maintenance.finishedAt) : candidateAge(item.modifiedAt)}</TableCell>
        <TableCell>{item.maintenance?.temporaryPresence === 'unknown' ? 'Unknown' : item.kind === 'empty_directory' ? '0 bytes (directory entry only)' : formatBytes(item.sizeBytes)}</TableCell>
        <TableCell>{item.outcome === 'already_absent' ? 'Already absent · 0 recoverable bytes' : item.safeToDelete ? (item.manualOnly ? 'Eligible · manual cleanup' : 'Eligible') : `Blocked: ${item.blockedReason || 'Not eligible'}`}</TableCell>
      </TableRow>)}</TableBody></Table></Box> : <Typography color="text.secondary">No candidates in this category.</Typography>}
      {report.errors.map((error) => <Alert severity="error" key={error}>{error}</Alert>)}
    </Stack> : null}
  </Stack></CardContent></Card>;
}

function CancellationPolicyCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Cancel Job</Typography>
            <Typography color="text.secondary" variant="body2">
              Cleanup defaults for canceled jobs.
            </Typography>
          </Stack>
          <FormControlLabel
            control={
              <Checkbox
                checked={form.cancellationPolicy.keepLogsAndDiagnostics}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    cancellationPolicy: {
                      ...current.cancellationPolicy,
                      keepLogsAndDiagnostics: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Keep logs and diagnostics"
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={form.cancellationPolicy.deleteGeneratedFiles}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    cancellationPolicy: {
                      ...current.cancellationPolicy,
                      deleteGeneratedFiles: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Delete generated files and temporary artifacts"
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={form.cancellationPolicy.deletePartialOutputFromStaging}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    cancellationPolicy: {
                      ...current.cancellationPolicy,
                      deletePartialOutputFromStaging: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Delete partial output from staging"
          />
          <Alert severity="info">
            Cleanup can only remove artifacts inside MVForge-controlled paths.
          </Alert>
          <Stack spacing={0.75}>
            <Typography color="text.secondary" variant="body2">
              Controlled paths
            </Typography>
            <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
              {form.cancellationPolicy.controlledRoots.map((root) => (
                <Chip key={root} label={root} size="small" />
              ))}
            </Stack>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}

export function OperationsSettingsDomain({ form, setForm, recovery, housekeeping }: OperationsSettingsDomainProps) {
  return <Grid container spacing={2}>
    <Grid size={{ xs: 12 }}><SchedulerRecoveryCard {...recovery} /></Grid>
    <Grid size={{ xs: 12, md: 6 }}><CancellationPolicyCard form={form} setForm={setForm} /></Grid>
    <Grid size={{ xs: 12 }}><HousekeepingCard {...housekeeping} draftOnly onSave={() => undefined} /></Grid>
  </Grid>;
}
