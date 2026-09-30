import {
  Alert,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Grid,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { api } from '../api/client';
import type { TestEncode, TestEncodePolicy } from '../api/types';

const defaultPolicy: TestEncodePolicy = {
  defaultDurationSeconds: 20,
  defaultStartMode: 'representative',
  defaultRetention: 'temporary',
  temporaryRetentionHours: 24,
  failedRetentionHours: 6,
  maxReconcileBatchSize: 100,
  maxCleanupBatchSize: 25,
};

type Props = {
  value?: Record<string, unknown>;
  saving: boolean;
  onSave: (value: Record<string, unknown>) => void;
};

function policyValue(value?: Record<string, unknown>): TestEncodePolicy {
  return {
    defaultDurationSeconds: numberField(value?.defaultDurationSeconds, defaultPolicy.defaultDurationSeconds),
    defaultStartMode: enumField(value?.defaultStartMode, ['beginning', 'middle', 'representative'], defaultPolicy.defaultStartMode),
    defaultRetention: enumField(value?.defaultRetention, ['temporary', 'keep'], defaultPolicy.defaultRetention),
    temporaryRetentionHours: numberField(value?.temporaryRetentionHours ?? value?.retentionHours, defaultPolicy.temporaryRetentionHours),
    failedRetentionHours: numberField(value?.failedRetentionHours, defaultPolicy.failedRetentionHours),
    maxReconcileBatchSize: numberField(value?.maxReconcileBatchSize, defaultPolicy.maxReconcileBatchSize),
    maxCleanupBatchSize: numberField(value?.maxCleanupBatchSize, defaultPolicy.maxCleanupBatchSize),
  };
}

function numberField(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function enumField<T extends string>(value: unknown, values: readonly T[], fallback: T): T {
  return typeof value === 'string' && values.includes(value as T) ? value as T : fallback;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function formatDate(value?: string): string {
  return value ? new Date(value).toLocaleString() : '—';
}

export function TestEncodeSettingsDomain({ value, saving, onSave }: Props) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<TestEncodePolicy>(() => policyValue(value));
  const [executionStatus, setExecutionStatus] = useState('');
  const [notice, setNotice] = useState('');
  const [retention, setRetention] = useState('');
  const [reconciliation, setReconciliation] = useState('');
  const [ageDays, setAgeDays] = useState('');
  const [libraryId, setLibraryId] = useState('');
  const [sourceAssetId, setSourceAssetId] = useState('');
  const [selected, setSelected] = useState<number[]>([]);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [confirmKeep, setConfirmKeep] = useState(false);
  const [viewing, setViewing] = useState<TestEncode | null>(null);

  const filters = useMemo(() => ({
    limit: 100,
    status: executionStatus || undefined,
    retention: retention || undefined,
    reconciliation: reconciliation || undefined,
    ageDays: ageDays || undefined,
    libraryId: libraryId || undefined,
    sourceAssetId: sourceAssetId || undefined,
  }), [ageDays, executionStatus, libraryId, reconciliation, retention, sourceAssetId]);

  const inventory = useQuery({ queryKey: ['test-encode-inventory', filters], queryFn: () => api.testEncodeInventory(filters) });
  const summary = useQuery({ queryKey: ['test-encode-inventory-summary'], queryFn: api.testEncodeInventorySummary });
  const orphanArtifacts = useQuery({ queryKey: ['test-encode-orphans'], queryFn: api.orphanedTestEncodeArtifacts });
  const invalidateInventory = async () => {
    await queryClient.invalidateQueries({ queryKey: ['test-encode-inventory'] });
    await queryClient.invalidateQueries({ queryKey: ['test-encode-inventory-summary'] });
    await queryClient.invalidateQueries({ queryKey: ['test-encode-orphans'] });
    await queryClient.invalidateQueries({ queryKey: ['test-encodes'] });
  };

  const keepMutation = useMutation({
    mutationFn: ({ id, keep }: { id: number; keep: boolean }) => api.keepTestEncode(id, keep),
    onSuccess: invalidateInventory,
  });
  const reconcileMutation = useMutation({
    mutationFn: (ids: number[]) => api.reconcileTestEncodes(ids),
    onSuccess: async (result) => {
      await invalidateInventory();
      setNotice(`Reconciled ${result.processed} Test Encodes. Active operations were skipped.`);
    },
  });
  const previewDeleteMutation = useMutation({
    mutationFn: (ids: number[]) => api.previewTestEncodeDelete(ids),
    onSuccess: () => {
      setConfirmKeep(false);
      setDeleteOpen(true);
    },
  });
  const deleteMutation = useMutation({
    mutationFn: () => api.deleteTestEncodes(selected, confirmKeep),
    onSuccess: async (result) => {
      const failed = result.results.filter((item) => !item.deleted);
      setDeleteOpen(false);
      setSelected([]);
      await invalidateInventory();
      const completed = result.results.length - failed.length;
      setNotice(failed.length
        ? `${completed} deleted; could not delete ${failed.map((item) => `Test Encode ${item.id}: ${item.error}`).join('; ')}`
        : `Deleted ${completed} Test Encodes and their owned artifacts.`);
    },
  });
  const deleteOrphanMutation = useMutation({
    mutationFn: api.deleteOrphanedTestEncodeArtifact,
    onSuccess: invalidateInventory,
  });

  const runs = inventory.data ?? [];
  const allSelected = runs.length > 0 && runs.every((test) => selected.includes(test.id));
  const toggleAll = (checked: boolean) => setSelected(checked ? runs.map((test) => test.id) : []);
  const toggleOne = (id: number, checked: boolean) => setSelected((current) => checked ? [...new Set([...current, id])] : current.filter((item) => item !== id));
  const beginDelete = (ids: number[]) => {
    setSelected(ids);
    previewDeleteMutation.mutate(ids);
  };

  return <Stack spacing={2}>
    <Card><CardContent><Stack spacing={2}>
      <Stack><Typography variant="h3">Defaults</Typography><Typography color="text.secondary" variant="body2">Sampling and retention defaults used by Test Encode. Encoder, codec, quality, filters and tracks continue to come from the existing resolvers.</Typography></Stack>
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField type="number" label="Default duration (seconds)" value={draft.defaultDurationSeconds} inputProps={{ min: 5, max: 120 }} onChange={(event) => setDraft({ ...draft, defaultDurationSeconds: Math.min(120, Math.max(5, Number(event.target.value))) })} fullWidth /></Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField select label="Default start position" value={draft.defaultStartMode} onChange={(event) => setDraft({ ...draft, defaultStartMode: event.target.value as TestEncodePolicy['defaultStartMode'] })} fullWidth><MenuItem value="beginning">Beginning</MenuItem><MenuItem value="middle">Middle</MenuItem><MenuItem value="representative">Representative scene</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField select label="Retention after comparison" value={draft.defaultRetention} onChange={(event) => setDraft({ ...draft, defaultRetention: event.target.value as TestEncodePolicy['defaultRetention'] })} fullWidth><MenuItem value="temporary">Temporary</MenuItem><MenuItem value="keep">Keep</MenuItem></TextField></Grid>
      </Grid>
      <Stack direction="row"><Button variant="contained" disabled={saving} onClick={() => onSave(draft as unknown as Record<string, unknown>)}>Save Test Encode settings</Button></Stack>
    </Stack></CardContent></Card>

    <Card><CardContent><Stack spacing={2}>
      <Stack><Typography variant="h3">Retention / Cleanup</Typography><Typography color="text.secondary" variant="body2">Keep runs are excluded from automatic cleanup. Cleanup uses bounded batches and removes only recorded Test Encode artifacts.</Typography></Stack>
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField type="number" label="Temporary retention (hours)" value={draft.temporaryRetentionHours} inputProps={{ min: 1, max: 8760 }} onChange={(event) => setDraft({ ...draft, temporaryRetentionHours: Math.min(8760, Math.max(1, Number(event.target.value))) })} fullWidth /></Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField type="number" label="Failed / cancelled retention (hours)" value={draft.failedRetentionHours} inputProps={{ min: 1, max: 8760 }} onChange={(event) => setDraft({ ...draft, failedRetentionHours: Math.min(8760, Math.max(1, Number(event.target.value))) })} fullWidth /></Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField type="number" label="Reconcile batch size" value={draft.maxReconcileBatchSize} inputProps={{ min: 1, max: 250 }} onChange={(event) => setDraft({ ...draft, maxReconcileBatchSize: Math.min(250, Math.max(1, Number(event.target.value))) })} fullWidth /></Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField type="number" label="Automatic cleanup batch size" value={draft.maxCleanupBatchSize} inputProps={{ min: 1, max: 100 }} onChange={(event) => setDraft({ ...draft, maxCleanupBatchSize: Math.min(100, Math.max(1, Number(event.target.value))) })} fullWidth /></Grid>
      </Grid>
      <Alert severity="info">Missing outputs are reconciled as database state. MVForge never recreates them or searches media roots for replacements.</Alert>
    </Stack></CardContent></Card>

    <Card><CardContent><Stack spacing={2}>
      <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" spacing={1}>
        <Stack><Typography variant="h3">Test Runs</Typography><Typography color="text.secondary" variant="body2">Persisted Test Encodes with their execution and reconciliation state.</Typography></Stack>
        <Stack direction="row" flexWrap="wrap" useFlexGap spacing={1}>
          <Chip label={`${summary.data?.totalRuns ?? 0} runs`} />
          <Chip label={`Known storage ${formatBytes(summary.data?.totalKnownBytes ?? 0)}`} />
          <Chip label={`Keep ${formatBytes(summary.data?.keepKnownBytes ?? 0)}`} />
          <Chip label={`${summary.data?.cleanupEligibleRuns ?? 0} eligible · ${formatBytes(summary.data?.cleanupEligibleBytes ?? 0)}`} />
        </Stack>
      </Stack>
      <Grid container spacing={1.5}>
        <Grid size={{ xs: 12, sm: 6, md: 2 }}><TextField select label="Execution" value={executionStatus} onChange={(event) => setExecutionStatus(event.target.value)} fullWidth><MenuItem value="">All</MenuItem>{['waiting', 'generating', 'ready', 'failed', 'canceled', 'expired'].map((item) => <MenuItem value={item} key={item}>{item}</MenuItem>)}</TextField></Grid>
        <Grid size={{ xs: 12, sm: 6, md: 2 }}><TextField select label="Retention" value={retention} onChange={(event) => setRetention(event.target.value)} fullWidth><MenuItem value="">All</MenuItem><MenuItem value="keep">Keep</MenuItem><MenuItem value="temporary">Temporary</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12, sm: 6, md: 2 }}><TextField select label="Reconciliation" value={reconciliation} onChange={(event) => setReconciliation(event.target.value)} fullWidth><MenuItem value="">All</MenuItem>{['unknown', 'healthy', 'missing_output', 'partial', 'orphaned', 'stale', 'cleanup_failed'].map((item) => <MenuItem value={item} key={item}>{item.replaceAll('_', ' ')}</MenuItem>)}</TextField></Grid>
        <Grid size={{ xs: 12, sm: 6, md: 2 }}><TextField select label="Older than" value={ageDays} onChange={(event) => setAgeDays(event.target.value)} fullWidth><MenuItem value="">Any age</MenuItem><MenuItem value="7">7 days</MenuItem><MenuItem value="30">30 days</MenuItem><MenuItem value="90">90 days</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12, sm: 6, md: 2 }}><TextField label="Library ID" value={libraryId} onChange={(event) => setLibraryId(event.target.value)} fullWidth /></Grid>
        <Grid size={{ xs: 12, sm: 6, md: 2 }}><TextField label="Asset ID" value={sourceAssetId} onChange={(event) => setSourceAssetId(event.target.value)} fullWidth /></Grid>
      </Grid>
      <Stack direction="row" flexWrap="wrap" useFlexGap spacing={1}>
        <Button variant="outlined" disabled={reconcileMutation.isPending} onClick={() => reconcileMutation.mutate(selected)}>{selected.length ? `Reconcile ${selected.length} selected` : 'Reconcile next batch'}</Button>
        <Button color="error" variant="outlined" disabled={!selected.length || previewDeleteMutation.isPending} onClick={() => beginDelete(selected)}>Delete selected ({selected.length})</Button>
      </Stack>
      {inventory.isError ? <Alert severity="error">Could not load Test Encode runs.</Alert> : null}
      {previewDeleteMutation.isError ? <Alert severity="error">Could not prepare the deletion summary: {previewDeleteMutation.error instanceof Error ? previewDeleteMutation.error.message : 'unknown error'}</Alert> : null}
      {deleteMutation.isError ? <Alert severity="error">Delete failed: {deleteMutation.error instanceof Error ? deleteMutation.error.message : 'unknown error'}</Alert> : null}
      {reconcileMutation.isError ? <Alert severity="error">Reconciliation failed: {reconcileMutation.error instanceof Error ? reconcileMutation.error.message : 'unknown error'}</Alert> : null}
      {notice ? <Alert severity="info">{notice}</Alert> : null}
      <Table size="small">
        <TableHead><TableRow><TableCell padding="checkbox"><Checkbox checked={allSelected} indeterminate={selected.length > 0 && !allSelected} onChange={(event) => toggleAll(event.target.checked)} /></TableCell><TableCell>Test / Asset</TableCell><TableCell>Created</TableCell><TableCell>Execution</TableCell><TableCell>Retention</TableCell><TableCell>Reconciliation</TableCell><TableCell>Output</TableCell><TableCell>Artifacts / Size</TableCell><TableCell>Actions</TableCell></TableRow></TableHead>
        <TableBody>
          {runs.map((test) => <TableRow key={test.id} hover>
            <TableCell padding="checkbox"><Checkbox checked={selected.includes(test.id)} onChange={(event) => toggleOne(test.id, event.target.checked)} /></TableCell>
            <TableCell><Stack><Typography variant="body2">#{test.id} · Asset {test.sourceAssetId || '—'}</Typography><Typography variant="caption" color="text.secondary" sx={{ maxWidth: 340, overflowWrap: 'anywhere' }}>{test.sourcePath}</Typography></Stack></TableCell>
            <TableCell>{formatDate(test.createdAt)}</TableCell>
            <TableCell><Chip size="small" label={test.status} /></TableCell>
            <TableCell><Chip size="small" color={test.keep ? 'success' : 'default'} label={test.keep ? 'Keep' : 'Temporary'} /></TableCell>
            <TableCell><Chip size="small" label={test.reconciliationStatus || 'unknown'} /></TableCell>
            <TableCell>{test.outputExists ? 'Present' : test.status === 'ready' ? 'Missing' : '—'}</TableCell>
            <TableCell>{test.artifactCount} · {formatBytes(test.knownDiskUsageBytes || test.outputSizeBytes)}</TableCell>
            <TableCell><Stack direction="row" flexWrap="wrap" useFlexGap>
              <Button size="small" onClick={() => setViewing(test)}>View</Button>
              <Button size="small" disabled={keepMutation.isPending || test.status === 'waiting' || test.status === 'generating'} onClick={() => keepMutation.mutate({ id: test.id, keep: !test.keep })}>{test.keep ? 'Release Keep' : 'Keep'}</Button>
              <Button size="small" color="error" onClick={() => beginDelete([test.id])}>Delete</Button>
            </Stack></TableCell>
          </TableRow>)}
          {runs.length === 0 ? <TableRow><TableCell colSpan={9}><Typography color="text.secondary">No Test Encode runs match these filters.</Typography></TableCell></TableRow> : null}
        </TableBody>
      </Table>
      {runs.length >= 100 ? <Typography variant="caption" color="text.secondary">Showing the latest 100 matching runs. Add filters to narrow the list.</Typography> : null}
    </Stack></CardContent></Card>

    <Card><CardContent><Stack spacing={2}>
      <Stack direction="row" justifyContent="space-between" spacing={1}>
        <Stack><Typography variant="h3">Proven orphan artifacts</Typography><Typography color="text.secondary" variant="body2">Only files with a persisted Test Encode ownership row and no live Test Encode record are listed. No filesystem discovery is performed.</Typography></Stack>
        <Chip label={`${orphanArtifacts.data?.artifacts.length ?? 0} found`} />
      </Stack>
      {orphanArtifacts.isError ? <Alert severity="error">Could not load orphaned Test Encode artifacts.</Alert> : null}
      {deleteOrphanMutation.isError ? <Alert severity="error">Orphan cleanup was rejected: {deleteOrphanMutation.error instanceof Error ? deleteOrphanMutation.error.message : 'unknown error'}</Alert> : null}
      {orphanArtifacts.data?.artifacts.map((artifact) => <Stack key={artifact.id} direction={{ xs: 'column', md: 'row' }} alignItems={{ md: 'center' }} justifyContent="space-between" spacing={1} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
        <Stack><Typography variant="body2">Artifact {artifact.id} · Test {artifact.testEncodeId} · {artifact.type} · {artifact.exists ? formatBytes(artifact.sizeBytes) : 'missing'}</Typography><Typography variant="caption" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>{artifact.path}</Typography>{artifact.message ? <Typography variant="caption" color="warning.main">{artifact.message}</Typography> : null}</Stack>
        <Button size="small" color="error" disabled={!artifact.safe || deleteOrphanMutation.isPending} onClick={() => { if (window.confirm(`Delete this proven orphan artifact?\n\n${artifact.path}\n\n${formatBytes(artifact.sizeBytes)}`)) deleteOrphanMutation.mutate(artifact.id); }}>Delete orphan</Button>
      </Stack>)}
      {!orphanArtifacts.data?.artifacts.length ? <Typography color="text.secondary">No persisted orphan artifact records were found.</Typography> : null}
    </Stack></CardContent></Card>

    <Dialog open={deleteOpen} onClose={() => setDeleteOpen(false)} maxWidth="sm" fullWidth>
      <DialogTitle>Delete selected Test Encodes?</DialogTitle>
      <DialogContent><Stack spacing={1.5} sx={{ pt: 1 }}>
        <Typography>This permanently removes only the recorded Test Encode artifact files and marks the selected records deleted.</Typography>
        {previewDeleteMutation.data ? <Stack direction="row" flexWrap="wrap" useFlexGap spacing={1}>
          <Chip label={`${previewDeleteMutation.data.testEncodeCount} tests`} />
          <Chip label={`${previewDeleteMutation.data.mediaOutputCount} media outputs`} />
          <Chip label={`${previewDeleteMutation.data.otherArtifactCount} sidecars / other artifacts`} />
          <Chip label={`Estimated ${formatBytes(previewDeleteMutation.data.estimatedBytes)}`} />
          <Chip color={previewDeleteMutation.data.keepCount ? 'warning' : 'default'} label={`${previewDeleteMutation.data.keepCount} Keep`} />
        </Stack> : null}
        {previewDeleteMutation.data?.keepCount ? <FormControlLabel control={<Checkbox checked={confirmKeep} onChange={(event) => setConfirmKeep(event.target.checked)} />} label="I explicitly confirm deleting the selected Keep Test Encodes" /> : null}
        {deleteMutation.data?.results.filter((item) => !item.deleted).map((item) => <Alert severity="error" key={item.id}>Test Encode {item.id}: {item.error}</Alert>)}
      </Stack></DialogContent>
      <DialogActions><Button onClick={() => setDeleteOpen(false)}>Cancel</Button><Button color="error" variant="contained" disabled={deleteMutation.isPending || previewDeleteMutation.isPending || (Boolean(previewDeleteMutation.data?.keepCount) && !confirmKeep)} onClick={() => deleteMutation.mutate()}>Delete artifacts</Button></DialogActions>
    </Dialog>

    <Dialog open={Boolean(viewing)} onClose={() => setViewing(null)} maxWidth="md" fullWidth>
      <DialogTitle>Test Encode #{viewing?.id}</DialogTitle>
      <DialogContent><Stack spacing={1} sx={{ pt: 1 }}>
        {viewing ? <>
          <Typography><b>Asset:</b> {viewing.sourcePath}</Typography>
          <Typography><b>Created:</b> {formatDate(viewing.createdAt)}</Typography>
          <Typography><b>Sample:</b> {viewing.startSeconds.toFixed(1)}s start · {viewing.durationSeconds}s duration</Typography>
          <Typography><b>Execution:</b> {viewing.status} · {viewing.effectiveEncoder || 'encoder unavailable'} · profile {viewing.profileId || '—'} v{viewing.profileVersion || '—'}</Typography>
          <Typography><b>Retention:</b> {viewing.keep ? 'Keep' : 'Temporary'} · <b>Reconciliation:</b> {viewing.reconciliationStatus || 'unknown'}</Typography>
          {viewing.reconciliationMessage ? <Typography color="text.secondary">{viewing.reconciliationMessage}</Typography> : null}
          <Typography sx={{ overflowWrap: 'anywhere' }}><b>Output:</b> {viewing.outputPath || '—'}</Typography>
          <Typography><b>Owned artifacts:</b> {viewing.artifactCount} · {viewing.sidecarArtifactCount} sidecars · {formatBytes(viewing.knownDiskUsageBytes)}</Typography>
          <Typography><b>Last reconciled:</b> {formatDate(viewing.lastReconciledAt)}</Typography>
        </> : null}
      </Stack></DialogContent>
      <DialogActions><Button onClick={() => setViewing(null)}>Close</Button></DialogActions>
    </Dialog>
  </Stack>;
}
