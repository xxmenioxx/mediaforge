import {
  Alert,
  Box,
  Button,
  Chip,
  FormControlLabel,
  Grid,
  LinearProgress,
  Link as MuiLink,
  Paper,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableContainer,
  TableRow,
  Typography,
} from '@mui/material';
import { type ReactNode, useEffect, useMemo, useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { PageContent, PageHeader } from '../components/PageHeader';
import { RuntimeStorageSummary } from '../components/RuntimeStorageSummary';
import { WorkspaceFeedback } from '../components/WorkspaceFeedback';
import type { AssetInventory, Library, Profile, QueueJob, RuntimeState } from '../api/types';
import { liveRuntimeStateLabel } from '../utils/runtimeStatePresentation';
import { buildDashboardSummary } from '../utils/dashboardSummary';

export function DashboardPage() {
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
  const runtime = useQuery({ queryKey: ['runtime-state'], queryFn: api.runtimeState });
  const refreshRuntime = useMutation({
    mutationFn: api.refreshRuntime,
    onSuccess: (state) => queryClient.setQueryData(['runtime-state'], state),
  });
  const refreshRuntimeNow = refreshRuntime.mutate;

  useEffect(() => {
    if (!autoRefreshRuntime) return undefined;
    const timer = window.setInterval(() => refreshRuntimeNow(), 30_000);
    return () => window.clearInterval(timer);
  }, [autoRefreshRuntime, refreshRuntimeNow]);

  const summary = useMemo(
    () => buildDashboardSummary(libraries.data ?? [], jobs.data ?? [], assets.data),
    [libraries.data, jobs.data, assets.data],
  );
  const hasUnavailableData = libraries.isError || profiles.isError || jobs.isError || assets.isError;

  return (
    <>
      <PageHeader eyebrow="Operations" title="Dashboard">
        Live view of media processing, inventory, and host capacity.
      </PageHeader>
      <PageContent>
        {hasUnavailableData ? (
          <Alert severity="warning" variant="outlined" sx={{ mb: 1.5 }}>
            Some Dashboard data is temporarily unavailable. Available summaries are still shown.
          </Alert>
        ) : null}

        <Grid container spacing={1.5} alignItems="flex-start" data-testid="dashboard-overview-grid">
          <Grid size={{ xs: 12, lg: 7 }}>
            <RuntimeOverviewPanel
              snapshot={runtime.data}
              loading={runtime.isLoading}
              error={runtime.isError || refreshRuntime.isError}
              refreshing={refreshRuntime.isPending}
              autoRefresh={autoRefreshRuntime}
              onAutoRefresh={setAutoRefreshRuntime}
              onRefresh={() => refreshRuntime.mutate()}
              workersValue={jobs.isLoading ? 'Loading' : jobs.isError ? 'Unavailable' : String(summary.activeWorkers)}
            />
          </Grid>

          <Grid size={{ xs: 12, lg: 5 }}>
            <QueueSummaryPanel
              loading={jobs.isLoading}
              error={jobs.isError}
              jobs={jobs.data}
              summary={summary}
            />
          </Grid>

          <Grid size={{ xs: 12, lg: 7 }}>
            <AssetLibraryPanel libraries={libraries} profiles={profiles} assets={assets} summary={summary} />
          </Grid>

          <Grid size={{ xs: 12, lg: 5 }}>
            <RuntimeStorageSummary snapshot={runtime.data} loading={runtime.isLoading} error={runtime.isError} compact />
          </Grid>

          <Grid size={{ xs: 12, lg: 7 }}>
            <RecentActivityPanel loading={jobs.isLoading} error={jobs.isError} jobs={jobs.data} recentJobs={summary.recentJobs} />
          </Grid>

          <Grid size={{ xs: 12, lg: 5 }}>
            <Stack spacing={1.5}>
              <BatchBacklogPanel loading={jobs.isLoading} error={jobs.isError} hasData={jobs.data !== undefined} batches={summary.batchSummaries} />
              <AttentionPanel
                loading={libraries.isLoading || jobs.isLoading || assets.isLoading}
                error={(libraries.isError && libraries.data === undefined) || (jobs.isError && jobs.data === undefined) || (assets.isError && assets.data === undefined)}
                items={summary.attentionItems}
              />
              <AdvisorPanel />
            </Stack>
          </Grid>
        </Grid>
      </PageContent>
    </>
  );
}

type DashboardSummary = ReturnType<typeof buildDashboardSummary>;
type DashboardQueryState<T> = {
  data: T | undefined;
  isLoading: boolean;
  isError: boolean;
};

function DashboardPanel({ title, action, children }: { title: string; action?: ReactNode; children: ReactNode }) {
  const headingId = 'dashboard-' + title.toLowerCase().replace(/[^a-z0-9]+/g, '-') + '-heading';
  return (
    <Paper component="section" aria-labelledby={headingId} variant="outlined" sx={{ minWidth: 0, p: { xs: 1.25, sm: 1.5 }, bgcolor: 'background.paper' }}>
      <Stack spacing={1.25} sx={{ minWidth: 0 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} justifyContent="space-between" spacing={0.75}>
          <Typography id={headingId} component="h2" sx={{ fontSize: (theme) => theme.mvforge.typography.sectionHeading, fontWeight: 700, lineHeight: 1.3 }}>
            {title}
          </Typography>
          {action}
        </Stack>
        {children}
      </Stack>
    </Paper>
  );
}

function SummaryLink({ label, value, to }: { label: string; value: string; to: string }) {
  return (
    <Button
      component={RouterLink}
      to={to}
      variant="text"
      size="small"
      color="inherit"
      aria-label={label + ': ' + value}
      sx={{ minWidth: 0, width: '100%', justifyContent: 'space-between', gap: 1, textTransform: 'none', px: 1 }}
    >
      <Typography variant="body2" color="text.secondary" noWrap>{label}</Typography>
      <Typography component="span" variant="body2" fontWeight={700} sx={{ flexShrink: 0 }}>{value}</Typography>
    </Button>
  );
}

function StatusCount({ label, value, color }: { label: string; value: number; color: 'default' | 'primary' | 'success' | 'warning' | 'error' }) {
  return <Chip size="small" variant="outlined" color={color} label={label + ' · ' + value} />;
}

function countValue<T>(query: DashboardQueryState<T>, count: (data: T) => number) {
  if (query.data === undefined && query.isLoading) return 'Loading';
  if (query.data === undefined) return 'Unavailable';
  return String(count(query.data));
}

function QueueSummaryPanel({ loading, error, jobs, summary }: {
  loading: boolean;
  error: boolean;
  jobs: QueueJob[] | undefined;
  summary: DashboardSummary;
}) {
  return (
    <DashboardPanel title="Queue" action={<Button component={RouterLink} to="/queue" size="small">Open Queue</Button>}>
      {jobs ? (
        <>
          {jobs.length === 0 ? <WorkspaceFeedback kind="empty">No queue activity yet.</WorkspaceFeedback> : null}
          <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', sm: 'repeat(3, minmax(0, 1fr))' }, gap: 0.75 }}>
            {[
              { label: 'Queued', value: summary.queuedJobs, status: 'queued', color: 'warning' as const },
              { label: 'Running', value: summary.runningJobs, status: 'running', color: 'primary' as const },
              { label: 'Completed', value: summary.completedJobs, status: 'completed', color: 'success' as const },
              { label: 'Failed', value: summary.failedJobs, status: 'failed', color: 'error' as const },
              { label: 'Canceled', value: summary.canceledJobs, status: 'canceled', color: 'inherit' as const },
            ].map((item) => (
              <Button
                key={item.status}
                component={RouterLink}
                to={'/queue?status=' + item.status}
                variant="outlined"
                size="small"
                color={item.color}
                aria-label={item.label + ': ' + item.value + ' jobs'}
                sx={{ minWidth: 0, justifyContent: 'space-between', textTransform: 'none', px: 1 }}
              >
                <span>{item.label}</span>
                <strong>{item.value}</strong>
              </Button>
            ))}
          </Box>
          <Stack spacing={0.5}>
            <LinearProgress aria-label="Queue completion" variant="determinate" value={summary.completedPercent} sx={{ height: 4, borderRadius: 1 }} />
            <Typography variant="caption" color="text.secondary">
              {summary.completedPercent}% of {jobs.length} known jobs completed
            </Typography>
          </Stack>
        </>
      ) : loading ? <WorkspaceFeedback kind="loading">Loading queue summary…</WorkspaceFeedback> : error ? <WorkspaceFeedback kind="error">Queue data is unavailable.</WorkspaceFeedback> : <WorkspaceFeedback kind="empty">Queue summary is not available yet.</WorkspaceFeedback>}
    </DashboardPanel>
  );
}

function AssetLibraryPanel({ libraries, profiles, assets, summary }: {
  libraries: DashboardQueryState<Library[]>;
  profiles: DashboardQueryState<Profile[]>;
  assets: DashboardQueryState<AssetInventory>;
  summary: DashboardSummary;
}) {
  const assetQueryCount = (count: number) => assets.data ? String(count) : assets.isLoading ? 'Loading' : 'Unavailable';
  return (
    <DashboardPanel title="Assets & libraries" action={<Button component={RouterLink} to="/assets" size="small">Open Assets</Button>}>
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', sm: 'repeat(4, minmax(0, 1fr))' }, gap: 0.5 }}>
        <SummaryLink label="Libraries" value={countValue(libraries, (data) => data.length)} to="/libraries" />
        <SummaryLink label="Profiles" value={countValue(profiles, (data) => data.length)} to="/profiles" />
        <SummaryLink label="Unprocessed" value={assetQueryCount(summary.unprocessedAssets)} to="/assets" />
        <SummaryLink label="Library assets" value={assetQueryCount(summary.libraryAssets)} to="/assets" />
      </Box>

      {assets.data ? (
        <>
          <Typography variant="caption" color="text.secondary">{summary.totalAssets} assets in the current inventory</Typography>
          <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">
            <StatusCount label="Converted" value={summary.convertedAssets} color="success" />
            <StatusCount label="Unverified" value={summary.unverifiedAssets} color="warning" />
            <StatusCount label="Processing" value={summary.processingAssets} color="primary" />
            <StatusCount label="Needs review" value={summary.needsReviewAssets} color="warning" />
            <StatusCount label="Validation review" value={summary.validationReviewAssets} color="warning" />
            <StatusCount label="Publication blocked" value={summary.publicationBlockedAssets} color="error" />
            <StatusCount label="Originals archived" value={summary.originalsArchived} color="success" />
          </Stack>
        </>
      ) : assets.isLoading ? <WorkspaceFeedback kind="loading">Loading asset inventory…</WorkspaceFeedback> : assets.isError ? <WorkspaceFeedback kind="error">Asset inventory is unavailable.</WorkspaceFeedback> : <WorkspaceFeedback kind="empty">Asset inventory is not available yet.</WorkspaceFeedback>}
    </DashboardPanel>
  );
}


function RuntimeOverviewPanel({ snapshot, loading, error, refreshing, autoRefresh, onAutoRefresh, onRefresh, workersValue }: {
  snapshot?: RuntimeState;
  loading: boolean;
  error: boolean;
  refreshing: boolean;
  autoRefresh: boolean;
  onAutoRefresh: (enabled: boolean) => void;
  onRefresh: () => void;
  workersValue: string;
}) {
  const encoders = snapshot ? Object.entries(snapshot.encoders ?? {}) : [];
  const ramPercent = snapshot && snapshot.totalMemoryBytes > 0 ? Math.round((snapshot.availableMemoryBytes / snapshot.totalMemoryBytes) * 100) : undefined;
  const cpuLoadPercent = snapshot && snapshot.cpuCores > 0 ? Math.min(100, Math.round((snapshot.cpuLoad1 / snapshot.cpuCores) * 100)) : undefined;
  const gpuUsagePercent = snapshot?.gpuUsageAvailable ? Math.round(snapshot.gpuUsagePercent) : undefined;
  const cpuColor = cpuLoadPercent === undefined ? 'primary' : cpuLoadPercent >= 85 ? 'error' : cpuLoadPercent >= 65 ? 'warning' : 'success';
  const ramColor = ramPercent !== undefined && ramPercent < 25 ? 'warning' : 'success';
  const gpuColor = gpuUsagePercent === undefined ? 'primary' : gpuUsagePercent >= 90 ? 'error' : gpuUsagePercent >= 70 ? 'warning' : 'success';

  return (
    <DashboardPanel
      title="System & runtime"
      action={
        <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} spacing={0.5} useFlexGap flexWrap="wrap">
          <FormControlLabel control={<Switch size="small" checked={autoRefresh} onChange={(event) => onAutoRefresh(event.target.checked)} />} label="Auto refresh every 30s" />
          <Button variant="outlined" size="small" onClick={onRefresh} disabled={refreshing}>{refreshing ? 'Detecting…' : 'Refresh host'}</Button>
          <Button component={RouterLink} to="/settings?section=runtime" size="small">Runtime settings</Button>
        </Stack>
      }
    >
      {error ? <WorkspaceFeedback kind="error">Runtime diagnostics are temporarily unavailable.</WorkspaceFeedback> : loading ? <WorkspaceFeedback kind="loading">Loading runtime diagnostics…</WorkspaceFeedback> : snapshot ? (
        <Stack spacing={1}>
          <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">
            <Chip size="small" color="primary" label={'Effective: ' + snapshot.selectedProfile} />
            <Chip size="small" variant="outlined" label={'Detected: ' + snapshot.recommendedProfile} />
            <Chip size="small" variant="outlined" label={'Preferred: ' + (snapshot.preferredProfile || 'auto')} />
            {snapshot.appliedOverrides.length ? <Chip size="small" color="warning" label={snapshot.appliedOverrides.length + ' overrides'} /> : null}
            <Chip size="small" variant="outlined" label={snapshot.os + '/' + snapshot.architecture} />
            <Chip size="small" variant="outlined" label={snapshot.batteryPresent ? (snapshot.onBattery ? 'Battery ' : 'AC ') + snapshot.batteryPercent + '%' : 'Power ' + snapshot.powerSource} color={snapshot.onBattery ? 'warning' : 'default'} />
            <Button component={RouterLink} to="/workers" size="small" variant="text" color="inherit" sx={{ textTransform: 'none' }}>
              Workers active on jobs: {workersValue}
            </Button>
          </Stack>

          <Grid container spacing={1}>
            <Grid size={{ xs: 12, sm: 4 }}>
              <RuntimeMeter label="Available RAM" value={formatBytes(snapshot.availableMemoryBytes) + ' / ' + formatBytes(snapshot.totalMemoryBytes)} detail={ramPercent === undefined ? 'Capacity unavailable' : ramPercent + '% available'} percent={ramPercent} color={ramColor} />
            </Grid>
            <Grid size={{ xs: 12, sm: 4 }}>
              <RuntimeMeter label="CPU load" value={snapshot.cpuLoad1.toFixed(2) + ' / ' + snapshot.cpuCores + ' cores'} detail={cpuLoadPercent === undefined ? 'Load unavailable' : cpuLoadPercent + '% normalized 1-minute load'} percent={cpuLoadPercent} color={cpuColor} />
            </Grid>
            <Grid size={{ xs: 12, sm: 4 }}>
              <RuntimeMeter label="GPU usage" value={gpuUsagePercent === undefined ? snapshot.gpuDetected ? 'Unavailable' : 'Not detected' : gpuUsagePercent + '%'} detail={gpuUsagePercent === undefined ? 'GPU engine usage is not exposed' : 'Media ' + Math.round(snapshot.gpuMediaUsagePercent) + '% · Render ' + Math.round(snapshot.gpuRenderUsagePercent) + '%'} percent={gpuUsagePercent} color={gpuColor} />
            </Grid>
          </Grid>

          <Stack spacing={0.5}>
            <Typography variant="caption" fontWeight={700}>FFmpeg encoders</Typography>
            <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">
              {encoders.map(([name, encoder]) => (
                <Chip
                  key={name}
                  size="small"
                  color={encoder.usable ? 'success' : 'default'}
                  variant={encoder.usable ? 'filled' : 'outlined'}
                  label={name + ' · ' + (encoder.usable ? 'usable' : encoder.listed ? 'disabled' : 'not found')}
                  title={encoder.reason || (encoder.usable ? 'Passed capability check' : 'Unavailable')}
                />
              ))}
              {!encoders.length ? <Chip size="small" color="warning" label="No encoder diagnostics available" /> : null}
            </Stack>
          </Stack>
          <Typography variant="caption" color="text.secondary">{liveRuntimeStateLabel(snapshot.detectedAt, formatDate)}</Typography>
        </Stack>
      ) : <WorkspaceFeedback kind="empty">No runtime snapshot is available yet.</WorkspaceFeedback>}
    </DashboardPanel>
  );
}

function RuntimeMeter({ label, value, detail, percent, color }: {
  label: string;
  value: string;
  detail: string;
  percent?: number;
  color: 'success' | 'warning' | 'error' | 'primary';
}) {
  return (
    <Stack spacing={0.35} sx={{ minWidth: 0 }}>
      <Stack direction="row" alignItems="baseline" justifyContent="space-between" spacing={0.5}>
        <Typography variant="caption" color="text.secondary">{label}</Typography>
        <Typography variant="caption" fontWeight={700} noWrap>{value}</Typography>
      </Stack>
      {percent !== undefined ? <LinearProgress aria-label={label + ' meter'} variant="determinate" value={percent} color={color} sx={{ height: 3, borderRadius: 1 }} /> : null}
      <Typography variant="caption" color="text.secondary">{detail}</Typography>
    </Stack>
  );
}

function RecentActivityPanel({ loading, error, jobs, recentJobs }: {
  loading: boolean;
  error: boolean;
  jobs: QueueJob[] | undefined;
  recentJobs: QueueJob[];
}) {
  return (
    <DashboardPanel title="Recent activity" action={<Button component={RouterLink} to="/queue" size="small">View more</Button>}>
      {jobs?.length ? (
        <TableContainer sx={{ maxWidth: '100%', overflowX: 'auto' }}>
          <Table size="small" sx={{ minWidth: 560 }}>
            <TableHead>
              <TableRow>
                <TableCell>Media</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Progress</TableCell>
                <TableCell>Updated</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {recentJobs.map((job) => {
                const timing = jobTiming(job);
                return (
                  <TableRow key={job.id}>
                    <TableCell sx={{ maxWidth: 260 }}>
                      <MuiLink component={RouterLink} to={'/queue?status=' + job.status + '&job=' + job.id} underline="hover" fontWeight={700} noWrap>
                        {fileNameFromPath(job.mediaPath)}
                      </MuiLink>
                      <Typography color="text.secondary" variant="caption" display="block" noWrap title={job.outputPath || job.mediaPath}>
                        {job.outputPath || job.mediaPath}
                      </Typography>
                    </TableCell>
                    <TableCell><Chip label={job.status} color={statusColor(job.status)} size="small" /></TableCell>
                    <TableCell sx={{ minWidth: 130 }}>
                      <Stack spacing={0.35}>
                        <Typography variant="caption" color="text.secondary">{job.progress}%</Typography>
                        <LinearProgress variant="determinate" value={job.progress} />
                        <Typography variant="caption" color="text.secondary">
                          {timing.elapsed ? 'Elapsed ' + timing.elapsed : 'Not started'}{timing.eta ? ' · ETA ' + timing.eta : ''}
                        </Typography>
                      </Stack>
                    </TableCell>
                    <TableCell><Typography variant="caption" noWrap>{formatDate(job.updatedAt)}</Typography></TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </TableContainer>
      ) : !jobs && loading ? <WorkspaceFeedback kind="loading">Loading recent queue activity…</WorkspaceFeedback> : !jobs && error ? <WorkspaceFeedback kind="error">Recent queue activity is unavailable.</WorkspaceFeedback> : <WorkspaceFeedback kind="empty" action={<Button component={RouterLink} to="/assets" size="small">Open Assets</Button>}>No queue activity yet. Queue an asset from Assets to begin.</WorkspaceFeedback>}
    </DashboardPanel>
  );
}

function BatchBacklogPanel({ loading, error, hasData, batches }: {
  loading: boolean;
  error: boolean;
  hasData: boolean;
  batches: DashboardSummary['batchSummaries'];
}) {
  return (
    <DashboardPanel title="Batch backlog">
      {hasData && batches.length ? (
        <TableContainer sx={{ maxWidth: '100%', overflowX: 'auto' }}>
          <Table size="small" sx={{ minWidth: 360 }}>
            <TableHead>
              <TableRow><TableCell>Batch</TableCell><TableCell>Remaining</TableCell><TableCell>Progress</TableCell></TableRow>
            </TableHead>
            <TableBody>
              {batches.map((batch) => (
                <TableRow key={batch.id}>
                  <TableCell sx={{ maxWidth: 140 }}>
                    <MuiLink component={RouterLink} to={'/queue?batch=' + encodeURIComponent(batch.id)} underline="hover" fontWeight={700} noWrap>
                      {batch.name}
                    </MuiLink>
                  </TableCell>
                  <TableCell>{batch.remaining}</TableCell>
                  <TableCell sx={{ minWidth: 100 }}>
                    <Stack spacing={0.35}>
                      <Typography variant="caption" color="text.secondary">{batch.completed}/{batch.total}</Typography>
                      <LinearProgress variant="determinate" value={batch.percent} />
                    </Stack>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      ) : hasData ? <WorkspaceFeedback kind="empty">Folder batches will appear here after queueing a folder.</WorkspaceFeedback> : loading ? <WorkspaceFeedback kind="loading">Loading batch backlog…</WorkspaceFeedback> : error ? <WorkspaceFeedback kind="error">Batch backlog is unavailable.</WorkspaceFeedback> : <WorkspaceFeedback kind="empty">Folder batches will appear here after queueing a folder.</WorkspaceFeedback>}
    </DashboardPanel>
  );
}

function AttentionPanel({ loading, error, items }: { loading: boolean; error: boolean; items: DashboardSummary['attentionItems'] }) {
  return (
    <DashboardPanel title="Attention">
      {loading ? <WorkspaceFeedback kind="loading">Loading operational checks…</WorkspaceFeedback> : error ? <WorkspaceFeedback kind="error">Attention summary is incomplete while source data is unavailable.</WorkspaceFeedback> : items.length ? (
        <Stack spacing={0.75}>
          {items.map((item) => (
            <Stack key={item.message} direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} justifyContent="space-between" spacing={0.5}>
              <Stack direction="row" alignItems="center" spacing={0.75} sx={{ minWidth: 0 }}>
                <Chip label="Review" color="warning" size="small" />
                <Typography variant="body2">{item.message}</Typography>
              </Stack>
              <Button component={RouterLink} to={item.to} size="small">Open</Button>
            </Stack>
          ))}
        </Stack>
      ) : <WorkspaceFeedback kind="empty">No immediate setup or queue issues detected.</WorkspaceFeedback>}
    </DashboardPanel>
  );
}

function AdvisorPanel() {
  return (
    <DashboardPanel title="Advisor">
      <Typography variant="body2" color="text.secondary">
        Advisor results are evaluated in an asset context. A dashboard-wide aggregate is not available.
      </Typography>
      <Box>
        <Button component={RouterLink} to="/assets" size="small">Open Assets</Button>
      </Box>
    </DashboardPanel>
  );
}
function statusColor(status: QueueJob['status']) {
  switch (status) {
    case 'completed':
      return 'success';
    case 'failed':
      return 'error';
    case 'canceled':
      return 'default';
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
