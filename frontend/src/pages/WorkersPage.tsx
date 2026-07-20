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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
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
  const [workerName, setWorkerName] = useState('local-worker');
  const [activePage, setActivePage] = useState(0);
  const [activeRowsPerPage, setActiveRowsPerPage] = useState(5);
  const [failedPage, setFailedPage] = useState(0);
  const [failedRowsPerPage, setFailedRowsPerPage] = useState(6);
  const [detailsJob, setDetailsJob] = useState<QueueJob | null>(null);
  const statusFilter = normalizeStatusFilter(searchParams.get('status'));

  const claimJob = useMutation({
    mutationFn: api.claimQueueJob,
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

  const stats = useMemo(() => summarizeJobs(jobs.data ?? []), [jobs.data]);
  const activeJobs = (jobs.data ?? [])
    .filter((job) => statusFilter === 'all' || job.status === statusFilter)
    .sort(compareWorkerJobs);
  const failedJobs = (jobs.data ?? []).filter((job) => job.status === 'failed');
  const pagedActiveJobs = activeJobs.slice(activePage * activeRowsPerPage, activePage * activeRowsPerPage + activeRowsPerPage);
  const pagedFailedJobs = failedJobs.slice(failedPage * failedRowsPerPage, failedPage * failedRowsPerPage + failedRowsPerPage);
  const recentJobs = (jobs.data ?? []).filter((job) => job.status !== 'queued' && job.status !== 'running').slice(0, 6);

  useEffect(() => {
    if (workerSettings.defaultWorkerName) {
      setWorkerName(workerSettings.defaultWorkerName);
    }
  }, [workerSettings.defaultWorkerName]);

  function claimNextJob() {
    claimJob.mutate({ workerName });
  }

  function cancelJob(job: QueueJob) {
    updateJob.mutate({
      jobId: job.id,
      status: 'canceled',
      progress: job.progress,
    });
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
          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
            {(workerNodes.data ?? []).map((worker) => <Chip key={worker.id} color={worker.status === 'online' ? 'success' : 'default'} label={`${worker.name} · ${worker.status} · ${worker.runtimeProfile || 'unknown runtime'} · ${worker.maxConcurrentJobs} slots · ${worker.encoders.filter((item): item is string => typeof item === 'string').join(', ') || 'no encoders'}`} />)}
            {!workerNodes.isLoading && !(workerNodes.data ?? []).length ? <Typography color="text.secondary">No workers have registered a heartbeat.</Typography> : null}
          </Stack>
        </Stack></CardContent></Card>

        <Grid container spacing={2} sx={{ mb: 2 }}>
          <Grid size={{ xs: 12, md: 5 }}>
            <Card sx={{ height: '100%' }}>
              <CardContent>
                <Stack spacing={2}>
                  <Stack>
                    <Typography variant="h3">Local Worker</Typography>
                    <Typography color="text.secondary" variant="body2">
                      Claim the next queued job by priority and creation time.
                    </Typography>
                  </Stack>
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                    <TextField
                      label="Worker name"
                      value={workerName}
                      onChange={(event) => setWorkerName(event.target.value)}
                      fullWidth
                    />
                    <Button
                      startIcon={<PlayArrowIcon />}
                      variant="contained"
                      onClick={claimNextJob}
                      disabled={claimJob.isPending || !workerName}
                      sx={{ minWidth: 148 }}
                    >
                      Claim
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
                  {pagedActiveJobs.map((job) => (
                    <WorkerJobCard
                      key={job.id}
                      job={job}
                      onCancel={cancelJob}
                      onDetails={setDetailsJob}
                      isUpdating={updateJob.isPending}
                    />
                  ))}
                  {!jobs.isLoading && activeJobs.length === 0 ? (
                    <Alert severity="info">No jobs match this status filter.</Alert>
                  ) : null}
                </Stack>
                {activeJobs.length > 0 ? (
                  <TablePagination
                    component="div"
                    count={activeJobs.length}
                    page={activePage}
                    rowsPerPage={activeRowsPerPage}
                    onPageChange={(_, page) => setActivePage(page)}
                    onRowsPerPageChange={(event) => {
                      setActiveRowsPerPage(Number(event.target.value));
                      setActivePage(0);
                    }}
                    rowsPerPageOptions={[5, 10, 25]}
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
                      <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                        <Stack sx={{ minWidth: 0 }}>
                          <Typography fontWeight={700} noWrap>
                            Job #{job.id} · {fileNameFromPath(job.mediaPath)}
                          </Typography>
                          <Typography color="text.secondary" variant="body2" noWrap>
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
                      <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                        <Typography fontWeight={700} noWrap>
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

function WorkerJobCard({
  job,
  onCancel,
  onDetails,
  isUpdating,
}: {
  job: QueueJob;
  onCancel: (job: QueueJob) => void;
  onDetails: (job: QueueJob) => void;
  isUpdating: boolean;
}) {
  const canCancel = job.status === 'queued' || job.status === 'running';
  const timing = jobTiming(job);

  return (
    <Card variant="outlined" sx={{ bgcolor: 'transparent' }}>
      <CardContent>
        <Stack spacing={1.5}>
          <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" spacing={1}>
            <Stack spacing={0.4} sx={{ minWidth: 0 }}>
              <Typography variant="h3" noWrap>
                {fileNameFromPath(job.mediaPath)}
              </Typography>
              <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                {job.mediaPath}
              </Typography>
            </Stack>
            <Stack direction="row" spacing={1} alignItems="center">
              <JobStatusChip status={job.status} />
              <Chip label={`P${job.priority}`} size="small" />
            </Stack>
          </Stack>

          <Stack spacing={0.6}>
            <Stack direction="row" justifyContent="space-between">
              <Typography color="text.secondary" variant="body2">
                {job.workerName || 'Unclaimed'}
              </Typography>
              <Typography color="text.secondary" variant="body2">
                {job.progress}%
              </Typography>
            </Stack>
            <LinearProgress variant="determinate" value={job.progress} />
            <Stack direction="row" spacing={1.5} flexWrap="wrap" useFlexGap>
              {timing.elapsed ? (
                <Typography color="text.secondary" variant="body2">
                  Elapsed: {timing.elapsed}
                </Typography>
              ) : null}
              {timing.eta ? (
                <Typography color="text.secondary" variant="body2">
                  ETA: {timing.eta}
                </Typography>
              ) : null}
            </Stack>
          </Stack>

          {job.errorMessage ? <Alert severity="warning">{job.errorMessage}</Alert> : null}
          {job.outputPath ? (
            <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
              Output: {job.outputPath}
            </Typography>
          ) : null}
          {job.notes.includes('Dry-run command:') ? (
            <Box
              component="pre"
              sx={{
                border: 1,
                borderColor: 'divider',
                borderRadius: 1,
                bgcolor: 'rgba(255,255,255,0.03)',
                m: 0,
                p: 1.5,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
              }}
            >
              {job.notes}
            </Box>
          ) : null}

          <Stack direction="row" spacing={1} justifyContent="flex-end" flexWrap="wrap" useFlexGap>
            <Button variant="outlined" onClick={() => onDetails(job)}>
              Details
            </Button>
            <Button
              startIcon={<CancelIcon />}
              color="warning"
              variant="outlined"
              onClick={() => onCancel(job)}
              disabled={!canCancel || isUpdating}
            >
              Cancel
            </Button>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}

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
