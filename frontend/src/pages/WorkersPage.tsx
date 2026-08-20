import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Grid,
  LinearProgress,
  Stack,
  TablePagination,
  TextField,
  Typography,
} from '@mui/material';
import CancelIcon from '@mui/icons-material/Cancel';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import RemoveCircleOutlineIcon from '@mui/icons-material/RemoveCircleOutline';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { Link as RouterLink, useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import type { AppSetting, QueueJob } from '../api/types';
import { JobDetailsDialog } from '../components/JobDetailsDialog';
import { PageHeader } from '../components/PageHeader';

export function WorkersPage() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const jobs = useQuery({
    queryKey: ['queueJobs'],
    queryFn: api.queueJobs,
    refetchInterval: (query) => (query.state.data?.some((job) => job.status === 'running') ? 2000 : false),
  });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const workerNodes = useQuery({ queryKey: ['workerNodes'], queryFn: api.workerNodes, refetchInterval: 5000 });
  const workerSettings = getWorkerSettings(settings.data);
  const [workerNameOverride, setWorkerNameOverride] = useState('');
  const workerName = workerNameOverride || workerSettings.defaultWorkerName || 'local-worker';
  const [activePage, setActivePage] = useState(0);
  const [activeRowsPerPage, setActiveRowsPerPage] = useState(5);
  const [failedPage, setFailedPage] = useState(0);
  const [failedRowsPerPage, setFailedRowsPerPage] = useState(6);
  const [detailsJob, setDetailsJob] = useState<QueueJob | null>(null);
  const statusFilter = normalizeStatusFilter(searchParams.get('status'));
  const workerFilter = searchParams.get('worker') ?? 'all';

  const claimJob = useMutation({
    mutationFn: async (claim: { workerName: string }) => {
      const job = await api.claimQueueJob(claim);
      return api.executeQueueJob({ jobId: job.id, overwrite: true });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
    },
  });

  const updateJob = useMutation({
    mutationFn: api.updateQueueJobStatus,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
    },
  });

  const removeBatch = useMutation({
    mutationFn: api.dismissQueueBatch,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
    },
  });

  const stats = useMemo(() => summarizeJobs(jobs.data ?? []), [jobs.data]);
  const activeJobs = (jobs.data ?? [])
    .filter((job) => (statusFilter === 'all' || job.status === statusFilter) && (workerFilter === 'all' || (job.workerName || 'unassigned') === workerFilter))
    .sort(compareWorkerJobs);
  const workerOptions = [...new Set((jobs.data ?? []).map((job) => job.workerName || 'unassigned'))].sort();
  const activeBatches = groupWorkerJobsByBatch(activeJobs);
  const failedJobs = (jobs.data ?? []).filter((job) => job.status === 'failed');
  const pagedActiveBatches = activeBatches.slice(activePage * activeRowsPerPage, activePage * activeRowsPerPage + activeRowsPerPage);
  const pagedFailedJobs = failedJobs.slice(failedPage * failedRowsPerPage, failedPage * failedRowsPerPage + failedRowsPerPage);
  const recentJobs = (jobs.data ?? []).filter((job) => job.status !== 'queued' && job.status !== 'running').slice(0, 6);

  function claimAndRunNextJob() {
    claimJob.mutate({ workerName });
  }

  function cancelJob(job: QueueJob) {
    updateJob.mutate({
      jobId: job.id,
      status: 'canceled',
      progress: job.progress,
    });
  }

  function cancelBatch(batch: WorkerJobBatch) {
    batch.jobs.filter((job) => job.status === 'queued' || job.status === 'running').forEach(cancelJob);
  }

  return (
    <>
      <PageHeader title="Workers" eyebrow="Execution control">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 760 }}>
          Workers claim queued jobs and move them through a controlled lifecycle before real conversion execution is enabled.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {jobs.isError ? <Alert severity="warning">Unable to load worker queue.</Alert> : null}
        <Alert severity={workerSettings.autoWorkerEnabled ? 'success' : 'warning'} sx={{ mb: 2 }}>
          Auto worker is {workerSettings.autoWorkerEnabled ? 'enabled' : 'disabled'} in Settings.{' '}
          <Button component={RouterLink} to="/settings" size="small" color="inherit">
            Open Settings
          </Button>
        </Alert>

        <Card sx={{ mb: 2 }}><CardContent><Stack spacing={1.5}>
          <Typography variant="h3">Worker availability</Typography>
          {workerNodes.isError ? <Alert severity="warning">Unable to load registered workers.</Alert> : null}
          <Grid container spacing={1}>
            {(workerNodes.data ?? []).map((worker) => (
              <Grid key={worker.id} size={{ xs: 12, sm: 6, lg: 4 }}>
                <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.25, height: '100%' }}>
                  <Stack spacing={0.75}>
                    <Stack direction="row" justifyContent="space-between" spacing={1} alignItems="center">
                      <Typography fontWeight={700} sx={{ minWidth: 0, overflowWrap: 'anywhere' }}>{worker.name}</Typography>
                      <Chip label={worker.status} color={worker.status === 'online' ? 'success' : 'default'} size="small" />
                    </Stack>
                    <Typography color="text.secondary" variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                      {worker.runtimeProfile || 'Unknown runtime'} · {worker.maxConcurrentJobs} slots
                    </Typography>
                    <Typography color="text.secondary" variant="caption" sx={{ overflowWrap: 'anywhere' }}>
                      {worker.encoders.filter((item): item is string => typeof item === 'string').join(', ') || 'No encoders reported'}
                    </Typography>
                  </Stack>
                </Box>
              </Grid>
            ))}
            {!workerNodes.isLoading && !(workerNodes.data ?? []).length ? <Grid size={{ xs: 12 }}><Typography color="text.secondary">No workers have registered a heartbeat.</Typography></Grid> : null}
          </Grid>
          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
            {['all', ...workerOptions].map((worker) => (
              <Chip
                key={worker}
                clickable
                color={workerFilter === worker ? 'primary' : 'default'}
                variant={workerFilter === worker ? 'filled' : 'outlined'}
                label={worker === 'all' ? 'All workers' : worker === 'unassigned' ? 'Unassigned' : worker}
                onClick={() => {
                  const next = new URLSearchParams(searchParams);
                  if (worker === 'all') next.delete('worker'); else next.set('worker', worker);
                  setSearchParams(next);
                  setActivePage(0);
                }}
              />
            ))}
          </Stack>
        </Stack></CardContent></Card>

        <Grid container spacing={2} sx={{ mb: 2 }}>
          <Grid size={{ xs: 12, md: 5 }}>
            <Card sx={{ height: '100%' }}>
              <CardContent sx={{ p: { xs: 1.5, sm: 2 }, '&:last-child': { pb: { xs: 1.5, sm: 2 } } }}>
                <Stack spacing={2}>
                  <Stack>
                    <Typography variant="h3">Local Worker</Typography>
                    <Typography color="text.secondary" variant="body2">
                      Claim and immediately execute the next queued job by priority and creation time.
                    </Typography>
                  </Stack>
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                    <TextField
                      label="Worker name"
                      value={workerName}
                      onChange={(event) => setWorkerNameOverride(event.target.value)}
                      fullWidth
                    />
                    <Button
                      startIcon={<PlayArrowIcon />}
                      variant="contained"
                      onClick={claimAndRunNextJob}
                      disabled={claimJob.isPending || !workerName}
                      sx={{ minWidth: { sm: 148 }, width: { xs: '100%', sm: 'auto' } }}
                    >
                      Claim &amp; run
                    </Button>
                  </Stack>
                  {claimJob.isError ? <Alert severity="info">{claimJob.error instanceof Error ? claimJob.error.message : 'No runnable queued jobs are available right now.'}</Alert> : null}
                </Stack>
              </CardContent>
            </Card>
          </Grid>
          <Grid size={{ xs: 12, md: 7 }}>
            <Grid container spacing={2}>
              <WorkerMetric label="Queued" value={stats.queued} color="warning" active={statusFilter === 'queued'} onClick={() => setSearchParams({ status: 'queued' })} />
              <WorkerMetric label="Running" value={stats.running} color="primary" active={statusFilter === 'running'} onClick={() => setSearchParams({ status: 'running' })} />
              <WorkerMetric label="Completed" value={stats.completed} color="success" active={statusFilter === 'completed'} onClick={() => setSearchParams({ status: 'completed' })} />
              <WorkerMetric label="Failed" value={stats.failed} color="error" active={statusFilter === 'failed'} onClick={() => setSearchParams({ status: 'failed' })} />
            </Grid>
          </Grid>
        </Grid>

        <Grid container spacing={2}>
          <Grid size={{ xs: 12, lg: 8 }}>
            <Card>
              <CardContent>
                  <Typography variant="h3" sx={{ mb: 2 }}>
                  Jobs
                </Typography>
                <Stack spacing={1.5}>
                  {pagedActiveBatches.map((batch) => (
                    <WorkerBatchCard
                      key={batch.id}
                      batch={batch}
                      onCancelJob={cancelJob}
                      onCancelBatch={cancelBatch}
                      onRemoveBatch={(item) => {
                        const removable = item.jobs.filter((job) => job.status !== 'completed').length;
                        if (!removable || !window.confirm(`Remove ${removable} non-completed job${removable === 1 ? '' : 's'} from “${item.name}”? Logs and completed jobs will be preserved.`)) return;
                        removeBatch.mutate(item.batchId);
                      }}
                      onDetails={setDetailsJob}
                      isUpdating={updateJob.isPending || removeBatch.isPending}
                    />
                  ))}
                  {!jobs.isLoading && activeJobs.length === 0 ? (
                    <Alert severity="info">No jobs match this status filter.</Alert>
                  ) : null}
                </Stack>
                {activeJobs.length > 0 ? (
                  <TablePagination
                    component="div"
                    count={activeBatches.length}
                    page={activePage}
                    rowsPerPage={activeRowsPerPage}
                    onPageChange={(_, page) => setActivePage(page)}
                    onRowsPerPageChange={(event) => {
                      setActiveRowsPerPage(Number(event.target.value));
                      setActivePage(0);
                    }}
                    rowsPerPageOptions={[5, 10, 25]}
                    sx={mobilePaginationSx}
                  />
                ) : null}
              </CardContent>
            </Card>
          </Grid>
          <Grid size={{ xs: 12, lg: 4 }}>
            <Card sx={{ mb: 2 }}>
              <CardContent>
                <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1} sx={{ mb: 2 }}>
                  <Typography variant="h3">Failed Jobs</Typography>
                  <Chip label={failedJobs.length} color={failedJobs.length ? 'error' : 'default'} size="small" />
                </Stack>
                <Stack spacing={1.5}>
                  {pagedFailedJobs.map((job) => (
                    <Stack key={job.id} spacing={0.75} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.25 }}>
                      <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} justifyContent="space-between" spacing={1}>
                        <Stack sx={{ minWidth: 0 }}>
                          <Typography fontWeight={700} sx={{ overflowWrap: 'anywhere' }}>
                            {job.executionNumber ? `Job #${job.executionNumber}` : 'Pending'} · {fileNameFromPath(job.mediaPath)}
                          </Typography>
                          <Typography color="text.secondary" variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                            {job.workerName || 'Unclaimed'} · P{job.priority}
                          </Typography>
                        </Stack>
                        <JobStatusChip status={job.status} />
                      </Stack>
                      <Stack direction="row" spacing={1} justifyContent="flex-end">
                        <Button size="small" variant="outlined" onClick={() => setDetailsJob(job)}>
                          Details
                        </Button>
                      </Stack>
                    </Stack>
                  ))}
                  {!jobs.isLoading && failedJobs.length === 0 ? (
                    <Typography color="text.secondary">No failed jobs right now.</Typography>
                  ) : null}
                </Stack>
                {failedJobs.length > 0 ? (
                  <TablePagination
                    component="div"
                    count={failedJobs.length}
                    page={failedPage}
                    rowsPerPage={failedRowsPerPage}
                    onPageChange={(_, page) => setFailedPage(page)}
                    onRowsPerPageChange={(event) => {
                      setFailedRowsPerPage(Number(event.target.value));
                      setFailedPage(0);
                    }}
                    rowsPerPageOptions={[6, 12, 24]}
                    sx={mobilePaginationSx}
                  />
                ) : null}
              </CardContent>
            </Card>
            <Card>
              <CardContent>
                <Typography variant="h3" sx={{ mb: 2 }}>
                  Recent Results
                </Typography>
                <Stack spacing={1.5}>
                  {recentJobs.map((job) => (
                    <Stack key={job.id} spacing={0.5}>
                      <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} justifyContent="space-between" spacing={1}>
                        <Typography fontWeight={700} sx={{ overflowWrap: 'anywhere' }}>
                          {fileNameFromPath(job.mediaPath)}
                        </Typography>
                        <JobStatusChip status={job.status} />
                      </Stack>
                      <Typography color="text.secondary" variant="body2">
                        {job.finishedAt ? formatDate(job.finishedAt) : 'Finished time unavailable'}
                      </Typography>
                    </Stack>
                  ))}
                  {!jobs.isLoading && recentJobs.length === 0 ? (
                    <Typography color="text.secondary">Completed and failed jobs will appear here.</Typography>
                  ) : null}
                </Stack>
              </CardContent>
            </Card>
          </Grid>
        </Grid>
      </Box>
      <JobDetailsDialog job={detailsJob} onClose={() => setDetailsJob(null)} />
    </>
  );
}

function WorkerMetric({
  label,
  value,
  color,
  active,
  onClick,
}: {
  label: string;
  value: number;
  color: 'warning' | 'primary' | 'success' | 'error';
  active: boolean;
  onClick: () => void;
}) {
  return (
    <Grid size={{ xs: 6, sm: 3 }}>
      <Card
        onClick={onClick}
        sx={{
          height: '100%',
          cursor: 'pointer',
          border: active ? 1 : undefined,
          borderColor: active ? `${color}.main` : undefined,
        }}
      >
        <CardContent>
          <Stack spacing={1}>
            <Chip label={label} color={color} size="small" sx={{ alignSelf: 'flex-start' }} />
            <Typography variant="h2">{value}</Typography>
          </Stack>
        </CardContent>
      </Card>
    </Grid>
  );
}

function WorkerBatchCard({
  batch,
  onCancelJob,
  onCancelBatch,
  onRemoveBatch,
  onDetails,
  isUpdating,
}: {
  batch: WorkerJobBatch;
  onCancelJob: (job: QueueJob) => void;
  onCancelBatch: (batch: WorkerJobBatch) => void;
  onRemoveBatch: (batch: WorkerJobBatch) => void;
  onDetails: (job: QueueJob) => void;
  isUpdating: boolean;
}) {
  const pending = batch.jobs.filter((job) => job.status === 'queued' || job.status === 'running');
  const completed = batch.jobs.filter((job) => job.status === 'completed').length;
  const progress = Math.round(batch.jobs.reduce((sum, job) => sum + job.progress, 0) / Math.max(1, batch.jobs.length));

  return (
    <Card variant="outlined" sx={{ bgcolor: 'transparent' }}>
      <CardContent sx={{ p: { xs: 1.5, sm: 2 }, '&:last-child': { pb: { xs: 1.5, sm: 2 } } }}>
        <Stack spacing={1.5}>
          <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" spacing={1}>
            <Stack spacing={0.4} sx={{ minWidth: 0 }}>
              <Typography variant="h3">
                {batch.name}
              </Typography>
              <Typography color="text.secondary" variant="body2">
                {completed}/{batch.jobs.length} completed · {pending.length} active
              </Typography>
            </Stack>
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
              {[...new Set(batch.jobs.map((job) => job.workerName || 'Unassigned'))].map((worker) => <Chip key={worker} label={worker} size="small" />)}
              {batch.batchId ? <Chip label="Batch" color="primary" size="small" /> : <Chip label="Single job" size="small" />}
            </Stack>
          </Stack>
          <LinearProgress variant="determinate" value={progress} />
          <Stack spacing={1}>
            {batch.jobs.map((job) => (
              <Stack key={job.id} direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" spacing={1} sx={{ borderTop: 1, borderColor: 'divider', pt: 1 }}>
                <Stack sx={{ minWidth: 0 }}>
                  <Typography fontWeight={700} noWrap>{fileNameFromPath(job.mediaPath)}</Typography>
                  <Typography color="text.secondary" variant="body2">{job.progress}% · P{job.priority} · {job.workerName || 'Unclaimed'}</Typography>
                  {job.errorMessage ? <Typography color="error" variant="body2">{job.errorMessage}</Typography> : null}
                </Stack>
                <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap sx={{ '& .MuiButton-root': { flex: { xs: '1 1 calc(50% - 8px)', sm: '0 0 auto' } } }}>
                  <JobStatusChip status={job.status} />
                  <Button size="small" variant="outlined" onClick={() => onDetails(job)}>Details</Button>
                  <Button size="small" color="warning" onClick={() => onCancelJob(job)} disabled={!['queued', 'running'].includes(job.status) || isUpdating}>Cancel</Button>
                </Stack>
              </Stack>
            ))}
          </Stack>
          {batch.batchId ? (
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} justifyContent="flex-end" sx={{ '& .MuiButton-root': { width: { xs: '100%', sm: 'auto' } } }}>
              <Button startIcon={<CancelIcon />} color="warning" variant="outlined" onClick={() => onCancelBatch(batch)} disabled={!pending.length || isUpdating}>Cancel Batch</Button>
              <Button startIcon={<RemoveCircleOutlineIcon />} color="error" variant="outlined" onClick={() => onRemoveBatch(batch)} disabled={isUpdating || batch.jobs.every((job) => job.status === 'completed')}>Remove Batch</Button>
            </Stack>
          ) : null}
        </Stack>
      </CardContent>
    </Card>
  );
}

type WorkerJobBatch = { id: string; batchId: string; name: string; jobs: QueueJob[] };

function groupWorkerJobsByBatch(jobs: QueueJob[]): WorkerJobBatch[] {
  const groups = new Map<string, WorkerJobBatch>();
  for (const job of jobs) {
    const id = job.batchId ? `batch:${job.batchId}` : `job:${job.id}`;
    const group = groups.get(id) ?? { id, batchId: job.batchId || '', name: job.batchName || fileNameFromPath(job.mediaPath), jobs: [] };
    group.jobs.push(job);
    groups.set(id, group);
  }
  return [...groups.values()];
}

const mobilePaginationSx = {
  '& .MuiTablePagination-toolbar': {
    flexWrap: { xs: 'wrap', sm: 'nowrap' },
    justifyContent: { xs: 'center', sm: 'flex-end' },
    px: { xs: 0, sm: 2 },
  },
  '& .MuiTablePagination-spacer': { display: { xs: 'none', sm: 'block' } },
  '& .MuiTablePagination-selectLabel': { display: { xs: 'none', sm: 'block' } },
} as const;

function JobStatusChip({ status }: { status: QueueJob['status'] }) {
  const color =
    status === 'completed' ? 'success' : status === 'failed' ? 'error' : status === 'running' ? 'primary' : 'warning';
  return <Chip label={status} color={color} size="small" />;
}

function summarizeJobs(jobs: QueueJob[]) {
  return jobs.reduce(
    (summary, job) => ({ ...summary, [job.status]: summary[job.status] + 1 }),
    { queued: 0, running: 0, completed: 0, failed: 0, canceled: 0 },
  );
}

function compareWorkerJobs(left: QueueJob, right: QueueJob) {
  const statusDiff = workerStatusRank(left.status) - workerStatusRank(right.status);
  if (statusDiff !== 0) {
    return statusDiff;
  }
  return right.id - left.id;
}

function workerStatusRank(status: QueueJob['status']) {
  switch (status) {
    case 'running':
      return 0;
    case 'queued':
      return 1;
    case 'completed':
      return 2;
    case 'failed':
      return 3;
    case 'canceled':
      return 4;
    default:
      return 5;
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

function normalizeStatusFilter(value: string | null): QueueJob['status'] | 'all' {
  return value === 'queued' || value === 'running' || value === 'completed' || value === 'failed' || value === 'canceled'
    ? value
    : 'all';
}

function getWorkerSettings(settings?: AppSetting[]) {
  const value = settings?.find((setting) => setting.key === 'workers')?.value ?? {};
  return {
    defaultWorkerName: typeof value.defaultWorkerName === 'string' ? value.defaultWorkerName : 'local-worker',
    autoWorkerEnabled: typeof value.autoWorkerEnabled === 'boolean' ? value.autoWorkerEnabled : true,
  };
}
