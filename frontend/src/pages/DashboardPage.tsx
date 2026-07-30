import {
  Alert,
  Box,
  Button,
  Card,
  CardActionArea,
  CardContent,
  Chip,
  FormControlLabel,
  Grid,
  LinearProgress,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import FolderIcon from '@mui/icons-material/Folder';
import InventoryIcon from '@mui/icons-material/Inventory';
import MemoryIcon from '@mui/icons-material/Memory';
import QueueIcon from '@mui/icons-material/Queue';
import TaskAltIcon from '@mui/icons-material/TaskAlt';
import WarningIcon from '@mui/icons-material/Warning';
import { useEffect, useMemo, useState } from 'react';
import { Link as RouterLink, useNavigate } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { MetricCard } from '../components/MetricCard';
import { PageHeader } from '../components/PageHeader';
import type { AssetInventory, Library, QueueJob, RuntimeSnapshot } from '../api/types';

export function DashboardPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [autoRefreshRuntime, setAutoRefreshRuntime] = useState(false);
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const jobs = useQuery({
    queryKey: ['queueJobs'],
    queryFn: api.queueJobs,
    refetchInterval: (query) => (query.state.data?.some((job) => job.status === 'queued' || job.status === 'running') ? 2000 : false),
  });
  const assets = useQuery({ queryKey: ['assets'], queryFn: api.assets });
  const runtime = useQuery({ queryKey: ['runtime-snapshot'], queryFn: api.runtimeSnapshot });
  const refreshRuntime = useMutation({ mutationFn: api.refreshRuntimeSnapshot, onSuccess: (snapshot) => queryClient.setQueryData(['runtime-snapshot'], snapshot) });
  useEffect(() => {
    if (!autoRefreshRuntime) return undefined;
    const timer = window.setInterval(() => refreshRuntime.mutate(), 30_000);
    return () => window.clearInterval(timer);
  }, [autoRefreshRuntime]);

  const summary = useMemo(
    () => buildDashboardSummary(libraries.data ?? [], jobs.data ?? [], assets.data),
    [libraries.data, jobs.data, assets.data],
  );

  return (
    <>
      <PageHeader eyebrow="Operations" title="Dashboard">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Live overview of libraries, unprocessed media, queue progress, and worker activity.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {(libraries.isError || profiles.isError || jobs.isError || assets.isError) && (
          <Alert severity="warning" sx={{ mb: 2 }}>
            Some operational data is unavailable. Check the backend container and configured library paths.
          </Alert>
        )}

        <Grid container spacing={1.5} sx={{ mb: 1.5 }}>
          <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
            <MetricCard label="Libraries" value={libraries.data?.length ?? 0} icon={<FolderIcon />} to="/libraries" />
          </Grid>
          <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
            <MetricCard label="Unprocessed Assets" value={summary.unprocessedAssets} icon={<InventoryIcon />} to="/assets" />
          </Grid>
          <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
            <MetricCard label="Active Jobs" value={summary.activeJobs} icon={<QueueIcon />} to="/queue?status=queued" />
          </Grid>
          <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
            <MetricCard label="Workers Active" value={summary.activeWorkers} icon={<MemoryIcon />} to="/workers" />
          </Grid>
        </Grid>

        <RuntimeOverviewPanel snapshot={runtime.data} loading={runtime.isLoading} error={runtime.isError || refreshRuntime.isError} refreshing={refreshRuntime.isPending} autoRefresh={autoRefreshRuntime} onAutoRefresh={setAutoRefreshRuntime} onRefresh={() => refreshRuntime.mutate()} />

        <Grid container spacing={2} sx={{ mt: 2 }}>
          <Grid size={{ xs: 12, lg: 7 }}>
            <Card sx={{ height: '100%' }}>
              <CardContent>
                <Stack spacing={2}>
                  <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                    <Stack>
                      <Typography variant="h2">Pipeline Progress</Typography>
                      <Typography color="text.secondary" variant="body2">
                        Completed jobs compared with all jobs currently known by MVForge.
                      </Typography>
                    </Stack>
                    <Chip label={`${summary.completedPercent}% complete`} color="success" sx={{ alignSelf: { xs: 'flex-start', sm: 'center' } }} />
                  </Stack>
                  <LinearProgress variant="determinate" value={summary.completedPercent} sx={{ height: 10, borderRadius: 1 }} />
                  <Grid container spacing={1.5}>
                    <PipelineChip label="Queued" value={summary.queuedJobs} color="warning" to="/queue?status=queued" />
                    <PipelineChip label="Running" value={summary.runningJobs} color="primary" to="/queue?status=running" />
                    <PipelineChip label="Completed" value={summary.completedJobs} color="success" to="/queue?status=completed" />
                    <PipelineChip label="Failed" value={summary.failedJobs} color="error" to="/queue?status=failed" />
                  </Grid>
                </Stack>
              </CardContent>
            </Card>
          </Grid>

          <Grid size={{ xs: 12, lg: 5 }}>
            <Card sx={{ height: '100%' }}>
              <CardContent>
                <Stack spacing={2}>
                  <Stack>
                    <Typography variant="h2">Asset Coverage</Typography>
                    <Typography color="text.secondary" variant="body2">
                      Current filesystem inventory grouped by processing state.
                    </Typography>
                  </Stack>
                  <Stack spacing={1.2}>
                    <CoverageRow label="Library Assets" value={summary.libraryAssets} total={summary.totalAssets} color="primary" />
                    <CoverageRow label="Converted" value={summary.convertedAssets} total={summary.libraryAssets} color="success" />
                    <CoverageRow label="Unverified in Library" value={summary.unverifiedAssets} total={summary.libraryAssets} color="warning" />
                    <CoverageRow label="Unprocessed" value={summary.unprocessedAssets} total={summary.totalAssets} color="warning" />
                  </Stack>
                </Stack>
              </CardContent>
            </Card>
          </Grid>
        </Grid>

        <Grid container spacing={2} sx={{ mt: 1 }}>
          <Grid size={{ xs: 12, lg: 7 }}>
            <Card>
              <CardContent>
                <Stack spacing={2}>
                  <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                    <Typography variant="h2">Recent Activity</Typography>
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Button component={RouterLink} to="/queue" size="small" variant="outlined">
                        View More
                      </Button>
                      <TaskAltIcon color="primary" />
                    </Stack>
                  </Stack>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell>Media</TableCell>
                        <TableCell>Status</TableCell>
                        <TableCell>Progress</TableCell>
                        <TableCell>Updated</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {summary.recentJobs.map((job) => (
                        <TableRow
                          key={job.id}
                          hover
                          onClick={() => navigate(`/queue?status=${job.status}&job=${job.id}`)}
                          sx={{ cursor: 'pointer' }}
                        >
                          <TableCell sx={{ maxWidth: 280 }}>
                            <Typography noWrap fontWeight={700}>
                              {fileNameFromPath(job.mediaPath)}
                            </Typography>
                            <Typography color="text.secondary" variant="body2" noWrap>
                              {job.outputPath || job.mediaPath}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Chip label={job.status} color={statusColor(job.status)} size="small" />
                          </TableCell>
                          <TableCell sx={{ minWidth: 140 }}>
                            <Stack spacing={0.5}>
                              <Typography color="text.secondary" variant="body2">
                                {job.progress}%
                              </Typography>
                              <LinearProgress variant="determinate" value={job.progress} />
                              <Typography color="text.secondary" variant="body2">
                                {jobTiming(job).elapsed ? `Elapsed ${jobTiming(job).elapsed}` : 'Not started'}
                                {jobTiming(job).eta ? ` · ETA ${jobTiming(job).eta}` : ''}
                              </Typography>
                            </Stack>
                          </TableCell>
                          <TableCell>{formatDate(job.updatedAt)}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                  {!jobs.isLoading && summary.recentJobs.length === 0 ? (
                    <Alert severity="info">No queue activity yet. Queue an asset from the Assets page to begin.</Alert>
                  ) : null}
                </Stack>
              </CardContent>
            </Card>
          </Grid>

          <Grid size={{ xs: 12, lg: 5 }}>
            <Card sx={{ mb: 2 }}>
              <CardContent>
                <Stack spacing={2}>
                  <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                    <Typography variant="h2">Batch Backlog</Typography>
                    <QueueIcon color="primary" />
                  </Stack>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell>Batch</TableCell>
                        <TableCell>Remaining</TableCell>
                        <TableCell>Progress</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {summary.batchSummaries.map((batch) => (
                        <TableRow
                          key={batch.id}
                          hover
                          onClick={() => navigate(`/queue?batch=${encodeURIComponent(batch.id)}`)}
                          sx={{ cursor: 'pointer' }}
                        >
                          <TableCell>
                            <Typography fontWeight={700} noWrap>
                              {batch.name}
                            </Typography>
                          </TableCell>
                          <TableCell>{batch.remaining}</TableCell>
                          <TableCell sx={{ minWidth: 140 }}>
                            <Stack spacing={0.5}>
                              <Typography color="text.secondary" variant="body2">
                                {batch.completed}/{batch.total}
                              </Typography>
                              <LinearProgress variant="determinate" value={batch.percent} />
                            </Stack>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                  {summary.batchSummaries.length === 0 ? (
                    <Typography color="text.secondary">Folder batches will appear here after queueing a folder.</Typography>
                  ) : null}
                </Stack>
              </CardContent>
            </Card>
            <Card>
              <CardContent>
                <Stack spacing={2}>
                  <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                    <Typography variant="h2">Attention</Typography>
                    <WarningIcon color={summary.attentionItems.length ? 'warning' : 'success'} />
                  </Stack>
                  <Stack spacing={1}>
                    {summary.attentionItems.map((item) => (
                      <Alert
                        key={item.message}
                        severity="warning"
                        action={
                          <Button color="inherit" component={RouterLink} to={item.to} size="small">
                            Open
                          </Button>
                        }
                      >
                        {item.message}
                      </Alert>
                    ))}
                    {summary.attentionItems.length === 0 ? (
                      <Alert severity="success">No immediate setup or queue issues detected.</Alert>
                    ) : null}
                  </Stack>
                </Stack>
              </CardContent>
            </Card>
          </Grid>
        </Grid>
      </Box>
    </>
  );
}

function RuntimeOverviewPanel({ snapshot, loading, error, refreshing, autoRefresh, onAutoRefresh, onRefresh }: { snapshot?: RuntimeSnapshot; loading: boolean; error: boolean; refreshing: boolean; autoRefresh: boolean; onAutoRefresh: (enabled: boolean) => void; onRefresh: () => void }) {
  const disks = snapshot ? Object.entries(snapshot.disks ?? {}) : [];
  const encoders = snapshot ? Object.entries(snapshot.encoders ?? {}) : [];
  const ramPercent = snapshot && snapshot.totalMemoryBytes > 0 ? Math.round((snapshot.availableMemoryBytes / snapshot.totalMemoryBytes) * 100) : 0;
  const cpuLoadPercent = snapshot && snapshot.cpuCores > 0
    ? Math.min(100, Math.round((snapshot.cpuLoad1 / snapshot.cpuCores) * 100))
    : 0;
  const gpuUsagePercent = snapshot?.gpuUsageAvailable ? Math.round(snapshot.gpuUsagePercent) : 0;
  return <Card><CardContent><Stack spacing={2}>
    <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" spacing={1.5}>
      <Stack><Typography variant="h2">Runtime & Host</Typography><Typography color="text.secondary" variant="body2">Effective scheduler policy and the host capabilities used for dispatch decisions.</Typography></Stack>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}><FormControlLabel control={<Switch checked={autoRefresh} onChange={(event) => onAutoRefresh(event.target.checked)} />} label="Auto refresh · 30s" /><Button variant="outlined" onClick={onRefresh} disabled={refreshing}>{refreshing ? 'Detecting…' : 'Refresh host'}</Button><Button component={RouterLink} to="/settings?section=runtime" variant="contained">Runtime settings</Button></Stack>
    </Stack>
    {error ? <Alert severity="warning">Runtime diagnostics are temporarily unavailable.</Alert> : loading ? <Typography color="text.secondary">Loading runtime diagnostics…</Typography> : snapshot ? <>
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap><Chip color="primary" label={`Effective: ${snapshot.selectedProfile}`} /><Chip label={`Detected: ${snapshot.recommendedProfile}`} /><Chip label={`Preferred: ${snapshot.preferredProfile || 'auto'}`} />{snapshot.appliedOverrides.length ? <Chip color="warning" label={`${snapshot.appliedOverrides.length} overrides`} /> : null}<Chip label={`${snapshot.os}/${snapshot.architecture}`} /><Chip label={`${snapshot.cpuCores} CPU cores · load ${snapshot.cpuLoad1.toFixed(2)}`} /><Chip color={snapshot.onBattery ? 'warning' : 'default'} label={snapshot.batteryPresent ? `${snapshot.onBattery ? 'Battery' : 'AC'} ${snapshot.batteryPercent}%` : `Power ${snapshot.powerSource}`} /></Stack>
      <Grid container spacing={2}>
        <Grid size={{ xs: 12, md: 4 }}>
          <Stack spacing={2}>
            <Stack spacing={0.75}>
              <Stack direction="row" justifyContent="space-between">
                <Typography fontWeight={700}>Available RAM</Typography>
                <Typography color="text.secondary">{formatBytes(snapshot.availableMemoryBytes)} / {formatBytes(snapshot.totalMemoryBytes)}</Typography>
              </Stack>
              <LinearProgress variant="determinate" value={ramPercent} color={ramPercent < 25 ? 'warning' : 'success'} />
              <Typography color="text.secondary" variant="caption">{ramPercent}% available</Typography>
            </Stack>
            <Stack spacing={0.75}>
              <Stack direction="row" justifyContent="space-between">
                <Typography fontWeight={700}>CPU load</Typography>
                <Typography color="text.secondary">{snapshot.cpuLoad1.toFixed(2)} / {snapshot.cpuCores} cores</Typography>
              </Stack>
              <LinearProgress variant="determinate" value={cpuLoadPercent} color={cpuLoadPercent >= 85 ? 'error' : cpuLoadPercent >= 65 ? 'warning' : 'success'} />
              <Typography color="text.secondary" variant="caption">{cpuLoadPercent}% normalized 1-minute load</Typography>
            </Stack>
            <Stack spacing={0.75}>
              <Stack direction="row" justifyContent="space-between">
                <Typography fontWeight={700}>GPU usage</Typography>
                <Typography color="text.secondary">
                  {snapshot.gpuUsageAvailable ? `${gpuUsagePercent}%` : snapshot.gpuDetected ? 'Unavailable' : 'Not detected'}
                </Typography>
              </Stack>
              <LinearProgress
                variant="determinate"
                value={gpuUsagePercent}
                color={gpuUsagePercent >= 90 ? 'error' : gpuUsagePercent >= 70 ? 'warning' : 'success'}
              />
              <Typography color="text.secondary" variant="caption">
                {snapshot.gpuUsageAvailable
                  ? `Media ${Math.round(snapshot.gpuMediaUsagePercent)}% · Render ${Math.round(snapshot.gpuRenderUsagePercent)}%`
                  : 'GPU engine counters are not exposed by this host/runtime.'}
              </Typography>
            </Stack>
          </Stack>
        </Grid>
        <Grid size={{ xs: 12, md: 4 }}><Stack spacing={0.75}><Typography fontWeight={700}>Storage</Typography>{disks.map(([role, raw]) => { const disk = raw && typeof raw === 'object' ? raw as Record<string, unknown> : {}; return <Stack direction="row" justifyContent="space-between" key={role}><Typography color="text.secondary" variant="body2">{role} · {String(disk.type ?? 'unknown')}</Typography><Typography variant="body2">{formatBytes(Number(disk.availableBytes ?? 0))} free</Typography></Stack>; })}{!disks.length ? <Typography color="text.secondary" variant="body2">No controlled disks detected.</Typography> : null}</Stack></Grid>
        <Grid size={{ xs: 12, md: 4 }}><Stack spacing={0.75}><Typography fontWeight={700}>FFmpeg encoders</Typography><Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>{encoders.map(([name, encoder]) => <Chip key={name} size="small" color={encoder.usable ? 'success' : 'default'} variant={encoder.usable ? 'filled' : 'outlined'} label={`${name} · ${encoder.usable ? 'usable' : encoder.listed ? 'disabled' : 'not found'}`} title={encoder.reason || (encoder.usable ? 'Passed capability check' : 'Unavailable')} />)}{!encoders.length ? <Chip size="small" color="warning" label="No encoder diagnostics available" /> : null}</Stack><Typography color="text.secondary" variant="caption">Hover an encoder to see its capability diagnostic.</Typography></Stack></Grid>
      </Grid>
      <Typography color="text.secondary" variant="caption">Host snapshot #{snapshot.id} · {formatDate(snapshot.detectedAt)}</Typography>
    </> : null}
  </Stack></CardContent></Card>;
}

function PipelineChip({
  label,
  value,
  color,
  to,
}: {
  label: string;
  value: number;
  color: 'warning' | 'primary' | 'success' | 'error';
  to: string;
}) {
  const content = (
    <Stack spacing={0.75}>
      <Chip label={label} color={color} size="small" sx={{ alignSelf: 'flex-start' }} />
      <Typography variant="h2">{value}</Typography>
    </Stack>
  );

  return (
    <Grid size={{ xs: 6, sm: 3 }}>
      <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
        <CardActionArea component={RouterLink} to={to} sx={{ borderRadius: 1 }}>
          {content}
        </CardActionArea>
      </Box>
    </Grid>
  );
}

function CoverageRow({
  label,
  value,
  total,
  color,
}: {
  label: string;
  value: number;
  total: number;
  color: 'primary' | 'success' | 'warning';
}) {
  const percent = total > 0 ? Math.round((value / total) * 100) : 0;
  return (
    <Stack spacing={0.6}>
      <Stack direction="row" justifyContent="space-between" spacing={1}>
        <Stack direction="row" spacing={1} alignItems="center">
          <Chip label={label} color={color} size="small" />
          <Typography color="text.secondary" variant="body2">
            {value} assets
          </Typography>
        </Stack>
        <Typography color="text.secondary" variant="body2">
          {percent}%
        </Typography>
      </Stack>
      <LinearProgress variant="determinate" value={percent} color={color} />
    </Stack>
  );
}

function buildDashboardSummary(libraries: Library[], jobs: QueueJob[], assets?: AssetInventory) {
  const unprocessedAssets = assets?.unprocessed.length ?? 0;
  const libraryAssets = assets?.library?.length ?? assets?.converted.length ?? 0;
  const convertedAssets = assets?.converted.length ?? 0;
  const unverifiedAssets = assets?.unverified?.length ?? Math.max(0, libraryAssets - convertedAssets);
  const totalAssets = unprocessedAssets + libraryAssets;
  const queuedJobs = jobs.filter((job) => job.status === 'queued').length;
  const runningJobs = jobs.filter((job) => job.status === 'running').length;
  const completedJobs = jobs.filter((job) => job.status === 'completed').length;
  const failedJobs = jobs.filter((job) => job.status === 'failed').length;
  const activeJobs = queuedJobs + runningJobs;
  const terminalJobs = completedJobs + failedJobs + jobs.filter((job) => job.status === 'canceled').length;
  const completedPercent = jobs.length > 0 ? Math.round((completedJobs / jobs.length) * 100) : 0;
  const activeWorkers = new Set(jobs.filter((job) => job.status === 'running' && job.workerName).map((job) => job.workerName)).size;
  const recentJobs = [...jobs]
    .sort((a, b) => new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime())
    .slice(0, 5);
  const batchSummaries = buildBatchSummaries(jobs);
  const librariesWithoutProfile = libraries.filter((library) => !library.defaultProfileId).length;

  const attentionItems = [
    libraries.length === 0 ? { message: 'No libraries configured yet.', to: '/libraries' } : null,
    librariesWithoutProfile > 0
      ? { message: `${librariesWithoutProfile} libraries do not have a default profile.`, to: '/libraries' }
      : null,
    unprocessedAssets > 0 && queuedJobs === 0
      ? { message: `${unprocessedAssets} unprocessed assets are not queued yet.`, to: '/assets' }
      : null,
    failedJobs > 0 ? { message: `${failedJobs} jobs have failed and need review.`, to: '/queue?status=failed' } : null,
    unverifiedAssets > 0 ? { message: `${unverifiedAssets} library assets have not been verified by MVForge.`, to: '/assets' } : null,
    activeJobs > 0 && activeWorkers === 0
      ? { message: 'Queued jobs are waiting for a worker.', to: '/workers' }
      : null,
    jobs.length > 0 && terminalJobs === jobs.length && failedJobs === 0 ? null : null,
  ].filter((item): item is { message: string; to: string } => Boolean(item));

  return {
    unprocessedAssets,
    libraryAssets,
    convertedAssets,
    unverifiedAssets,
    totalAssets,
    queuedJobs,
    runningJobs,
    completedJobs,
    failedJobs,
    activeJobs,
    activeWorkers,
    completedPercent,
    recentJobs,
    batchSummaries,
    attentionItems,
  };
}

function buildBatchSummaries(jobs: QueueJob[]) {
  const batches = new Map<string, QueueJob[]>();
  jobs.forEach((job) => {
    if (!job.batchId) {
      return;
    }
    batches.set(job.batchId, [...(batches.get(job.batchId) ?? []), job]);
  });

  return [...batches.entries()]
    .map(([id, batchJobs]) => {
      const completed = batchJobs.filter((job) => job.status === 'completed').length;
      const remaining = batchJobs.filter((job) => job.status === 'queued' || job.status === 'running').length;
      const name = batchJobs.find((job) => job.batchName)?.batchName || `Batch ${id}`;
      return {
        id,
        name,
        total: batchJobs.length,
        completed,
        remaining,
        percent: batchJobs.length > 0 ? Math.round((completed / batchJobs.length) * 100) : 0,
      };
    })
    .filter((batch) => batch.remaining > 0)
    .sort((a, b) => b.remaining - a.remaining || a.name.localeCompare(b.name))
    .slice(0, 6);
}

function statusColor(status: QueueJob['status']) {
  switch (status) {
    case 'completed':
      return 'success';
    case 'failed':
      return 'error';
    case 'running':
      return 'primary';
    default:
      return 'warning';
  }
}

function fileNameFromPath(path: string) {
  return path.split('/').filter(Boolean).pop() ?? path;
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value));
}

function formatBytes(bytes: number) {
  if (!bytes) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let index = 0;
  while (value >= 1024 && index < units.length - 1) { value /= 1024; index += 1; }
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[index]}`;
}

function jobTiming(job: QueueJob) {
  if (!job.startedAt) {
    return { elapsed: '', eta: '' };
  }

  const startMs = new Date(job.startedAt).getTime();
  const endMs = job.finishedAt ? new Date(job.finishedAt).getTime() : Date.now();
  if (!Number.isFinite(startMs) || !Number.isFinite(endMs) || endMs < startMs) {
    return { elapsed: '', eta: '' };
  }

  const elapsedSeconds = Math.max(0, Math.round((endMs - startMs) / 1000));
  const canEstimate = job.status === 'running' && job.progress > 0 && job.progress < 100;
  const etaSeconds = canEstimate ? Math.round((elapsedSeconds * (100 - job.progress)) / job.progress) : 0;

  return {
    elapsed: formatDuration(elapsedSeconds),
    eta: canEstimate ? formatDuration(etaSeconds) : '',
  };
}

function formatDuration(totalSeconds: number) {
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  if (hours > 0) {
    return `${hours}h ${minutes}m ${seconds}s`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}
