import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Grid,
  IconButton,
  LinearProgress,
  MenuItem,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import CancelIcon from '@mui/icons-material/Cancel';
import EditIcon from '@mui/icons-material/Edit';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import KeyboardArrowUpIcon from '@mui/icons-material/KeyboardArrowUp';
import PlaylistAddIcon from '@mui/icons-material/PlaylistAdd';
import ReplayIcon from '@mui/icons-material/Replay';
import RemoveCircleOutlineIcon from '@mui/icons-material/RemoveCircleOutline';
import DragIndicatorIcon from '@mui/icons-material/DragIndicator';
import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { DragEvent, FormEvent, useMemo, useState } from 'react';
import { Link as RouterLink, useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import { canRemoveQueueBatch } from '../utils/queueBatch';
import { hasDeterminateQueueProgress, queueStagePresentation, queueStatusColor, queueStatusLabel } from '../utils/queuePresentation';
import { CompactActionGroup } from '../components/CompactActionGroup';
import { JobDetailsDialog } from '../components/JobDetailsDialog';
import { PageContent, PageHeader } from '../components/PageHeader';
import { WorkspaceFeedback } from '../components/WorkspaceFeedback';
import { getMVForgeTokens } from '../theme/theme';
import type { AppSetting, Asset, AssetInventory, AudioEnhancementProfile, Library, Profile, QueueJob, QueueJobInput } from '../api/types';
import { emptyTrackProfile, getTrackProfiles, trackProfileOverride, type TrackProfile } from '../trackProfiles';

const initialJob: QueueJobInput = {
  mediaPath: '',
  libraryId: 0,
  profileId: 0,
  priority: 5,
  notes: '',
};

export function QueuePage() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const jobs = useQuery({
    queryKey: ['queueJobs'],
    queryFn: api.queueJobs,
    refetchInterval: (query) => (
      query.state.data?.some((job) => job.status === 'queued' || job.status === 'running') ? 2000 : false
    ),
  });
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const assets = useQuery({ queryKey: ['assets'], queryFn: api.assets });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const audioProfiles = getAudioProfiles(settings.data);
  const trackProfiles = getTrackProfiles(settings.data);
  const workerSettings = getWorkerSettings(settings.data);
  const [form, setForm] = useState<QueueJobInput>(initialJob);
  const [isJobDialogOpen, setIsJobDialogOpen] = useState(false);
  const [editingJob, setEditingJob] = useState<QueueJob | null>(null);
  const [selectedTrackProfileKey, setSelectedTrackProfileKey] = useState('');
  const [jobSearch, setJobSearch] = useState('');
  const [jobsPerPage, setJobsPerPage] = useState(25);
  const [jobPage, setJobPage] = useState(1);
  const statusFilter = normalizeStatusFilter(searchParams.get('status'));
  const batchFilter = searchParams.get('batch') ?? '';
  const pathFilter = searchParams.get('path') ?? '';
  const workerFilter = searchParams.get('worker') ?? 'all';
  const selectedJobId = Number(searchParams.get('job'));
  const jobHistory = useMemo(() => jobs.data ?? [], [jobs.data]);
  const allJobs = useMemo(() => queueJobsForDisplay(jobHistory), [jobHistory]);
  const workerOptions = useMemo(
    () => [...new Set(allJobs.map((job) => job.workerName || 'unassigned'))].sort((left, right) => left.localeCompare(right)),
    [allJobs],
  );
  const filteredJobs = useMemo(
    () =>
      allJobs.filter((job) => {
        const matchesStatus = statusFilter === 'all' || job.status === statusFilter;
        const matchesBatch = !batchFilter || job.batchId === batchFilter;
        const matchesPath = !pathFilter || normalizePath(queueGroupPathFromMediaPath(job.mediaPath)) === normalizePath(pathFilter);
        const matchesWorker = workerFilter === 'all' || (job.workerName || 'unassigned') === workerFilter;
        const query = jobSearch.trim().toLowerCase();
        const matchesSearch =
          !query ||
          [
            String(job.id),
            job.mediaPath,
            job.status,
            job.workerName,
            job.notes,
            job.errorMessage,
            job.audioProfileKey,
          ].some((value) => value.toLowerCase().includes(query));
        return matchesStatus && matchesBatch && matchesPath && matchesWorker && matchesSearch;
      }),
    [allJobs, batchFilter, jobSearch, pathFilter, statusFilter, workerFilter],
  );
  const orderedJobs = useMemo(() => [...filteredJobs].sort(sortJobsForQueue), [filteredJobs]);
  const totalJobPages = Math.max(1, Math.ceil(orderedJobs.length / jobsPerPage));
  const safeJobPage = Math.min(jobPage, totalJobPages);
  const pagedJobs = orderedJobs.slice((safeJobPage - 1) * jobsPerPage, safeJobPage * jobsPerPage);
  const statusCounts = useMemo(() => summarizeStatusCounts(allJobs), [allJobs]);
  const selectedJob = selectedJobId ? jobHistory.find((job) => job.id === selectedJobId) ?? null : null;

  async function applyQueueSelections(input: QueueJobInput) {
    const asset = findQueueAsset(assets.data, input.mediaPath);
    const trackProfile = trackProfiles.find((profile) => profile.key === selectedTrackProfileKey);
    await api.updateAssetConversion({
      path: input.mediaPath,
      ...(asset?.conversion ?? {}),
      ...(trackProfile ? trackProfileOverride(trackProfile) : {
        keepVideoStreams: null,
        keepAudioStreams: null,
        keepSubtitleStreams: null,
        videoMetadata: undefined,
        audioMetadata: undefined,
        subtitleMetadata: undefined,
        subtitleTransforms: undefined,
      }),
      trackProfileKey: trackProfile?.key,
    });
  }

  const createJob = useMutation({
    mutationFn: async (input: QueueJobInput) => {
      await applyQueueSelections(input);
      return api.createQueueJob(input);
    },
    onSuccess: async () => {
      setForm(initialJob);
      setEditingJob(null);
      setSelectedTrackProfileKey('');
      setIsJobDialogOpen(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
      ]);
    },
  });
  const editJob = useMutation({
    mutationFn: async (input: Parameters<typeof api.updateQueueJob>[0]) => {
      await applyQueueSelections({
        ...form,
        mediaPath: input.mediaPath ?? form.mediaPath,
        libraryId: input.libraryId ?? form.libraryId,
        profileId: input.profileId ?? form.profileId,
        audioProfileKey: input.audioProfileKey ?? form.audioProfileKey,
        trackProfileKey: input.trackProfileKey ?? form.trackProfileKey,
        processingMode: input.processingMode ?? form.processingMode,
        priority: input.priority ?? form.priority,
      });
      return api.updateQueueJob(input);
    },
    onSuccess: async () => {
      setForm(initialJob);
      setEditingJob(null);
      setSelectedTrackProfileKey('');
      setIsJobDialogOpen(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
      ]);
    },
  });

  function updateField<K extends keyof QueueJobInput>(field: K, value: QueueJobInput[K]) {
    setForm((current) => ({ ...current, [field]: value }));
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const technicalProfileId = form.profileId > 0 ? form.profileId : editingJob?.profileId || profiles.data?.[0]?.id || 0;
    if (!technicalProfileId || (!form.profileId && !form.audioProfileKey && !selectedTrackProfileKey)) return;
    const input: QueueJobInput = {
      ...form,
      profileId: technicalProfileId,
      trackProfileKey: selectedTrackProfileKey,
      processingMode: form.profileId > 0 ? 'full_encode' : 'audio_only',
    };
    if (editingJob) {
      editJob.mutate({ jobId: editingJob.id, ...input });
      return;
    }
    createJob.mutate(input);
  }

  function openCreateJobDialog() {
    setEditingJob(null);
    setForm(initialJob);
    setSelectedTrackProfileKey('');
    setIsJobDialogOpen(true);
  }

  function openEditJobDialog(job: QueueJob) {
    const conversion = findQueueAsset(assets.data, job.mediaPath)?.conversion;
    setEditingJob(job);
    setSelectedTrackProfileKey(job.trackProfileKey || conversion?.trackProfileKey || '');
    setForm({
      mediaPath: job.mediaPath,
      libraryId: job.libraryId,
      profileId: job.processingMode === 'audio_only' ? 0 : job.profileId,
      trackProfileKey: job.trackProfileKey,
      processingMode: job.processingMode || undefined,
      audioProfileKey: job.audioProfileKey,
      priority: job.priority,
      notes: job.notes,
    });
    setIsJobDialogOpen(true);
  }

  function closeJobDialog() {
    setIsJobDialogOpen(false);
    setEditingJob(null);
    setForm(initialJob);
    setSelectedTrackProfileKey('');
  }

  return (
    <>
      <PageHeader
        title="Queue"
        eyebrow="Operations"
        actions={<Button startIcon={<PlaylistAddIcon />} variant="contained" onClick={openCreateJobDialog}>Queue Job</Button>}
      >
        <Typography color="text.secondary" sx={{ mt: 0.5, maxWidth: 760 }}>
          Review queued work and monitor each job through processing.
        </Typography>
      </PageHeader>
      <PageContent>
        {jobs.isError ? (
          <Box sx={{ mb: 1.25 }}>
            <WorkspaceFeedback kind="error" action={<Button size="small" onClick={() => void jobs.refetch()}>Retry</Button>}>
              Unable to load Queue.
            </WorkspaceFeedback>
          </Box>
        ) : null}
        <QueueSummaryStrip counts={jobs.data ? statusCounts : undefined} autoWorkerEnabled={workerSettings.autoWorkerEnabled} />
        <CompactActionGroup role="group" aria-label="Filter Queue by status" sx={{ mb: 1.25 }}>
            {(['all', 'running', 'queued', 'completed', 'failed', 'canceled'] as const).map((status) => (
              <Button
                key={status}
                variant={statusFilter === status ? 'contained' : 'outlined'}
                color={status === 'failed' ? 'error' : 'primary'}
                size="small"
                aria-pressed={statusFilter === status}
                onClick={() => {
                  const nextParams = new URLSearchParams();
                  nextParams.set('status', status);
                  if (batchFilter) {
                    nextParams.set('batch', batchFilter);
                  }
                  if (workerFilter !== 'all') {
                    nextParams.set('worker', workerFilter);
                  }
                  setSearchParams(nextParams);
                }}
              >
                {statusLabel(status)}
              </Button>
            ))}
            {batchFilter || pathFilter || workerFilter !== 'all' ? (
              <Button size="small" variant="outlined" onClick={() => setSearchParams(statusFilter === 'all' ? {} : { status: statusFilter })}>
                Clear focus
              </Button>
            ) : null}
        </CompactActionGroup>
        <Card variant="outlined" sx={{ mb: 1.25 }}>
          <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
            <Grid container spacing={1} alignItems="center">
              <Grid size={{ xs: 12, md: 4 }}>
                <TextField
                  size="small"
                  label="Search jobs"
                  value={jobSearch}
                  onChange={(event) => { setJobSearch(event.target.value); setJobPage(1); }}
                  placeholder="Job number, asset, status, worker, notes..."
                  fullWidth
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <TextField
                  size="small"
                  select
                  label="Worker"
                  value={workerFilter}
                  fullWidth
                  onChange={(event) => {
                    const worker = event.target.value;
                    const nextParams = new URLSearchParams(searchParams);
                    if (worker === 'all') nextParams.delete('worker');
                    else nextParams.set('worker', worker);
                    setSearchParams(nextParams);
                  }}
                >
                  {['all', ...new Set([...workerOptions, ...(workerFilter === 'all' ? [] : [workerFilter])])].map((worker) => (
                    <MenuItem key={worker} value={worker}>
                      {worker === 'all' ? 'All workers' : worker === 'unassigned' ? 'Unassigned' : worker}
                    </MenuItem>
                  ))}
                </TextField>
              </Grid>
              <Grid size={{ xs: 6, md: 2 }}>
                <TextField
                  size="small"
                  label="Jobs per page"
                  value={jobsPerPage}
                  onChange={(event) => { setJobsPerPage(Number(event.target.value)); setJobPage(1); }}
                  select
                  fullWidth
                >
                  {[5, 10, 25, 50].map((count) => (
                    <MenuItem key={count} value={count}>
                      {count}
                    </MenuItem>
                  ))}
                </TextField>
              </Grid>
              <Grid size={{ xs: 12 }}>
                <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={0.75}>
                  <Typography color="text.secondary" variant="caption" aria-live="polite">
                    Page {safeJobPage} of {totalJobPages} · {filteredJobs.length} matching jobs
                  </Typography>
                  <CompactActionGroup>
                    <Button size="small" variant="outlined" disabled={safeJobPage <= 1} onClick={() => setJobPage((current) => Math.max(1, current - 1))}>
                      Previous
                    </Button>
                    <Button size="small" variant="outlined" disabled={safeJobPage >= totalJobPages} onClick={() => setJobPage((current) => Math.min(totalJobPages, current + 1))}>
                      Next
                    </Button>
                  </CompactActionGroup>
                </Stack>
              </Grid>
            </Grid>
          </CardContent>
        </Card>
        <Dialog
          open={isJobDialogOpen}
          onClose={closeJobDialog}
          maxWidth="md"
          fullWidth
          PaperProps={{ sx: { m: { xs: 0, sm: 4 }, width: { xs: '100%', sm: 'calc(100% - 64px)' }, maxHeight: { xs: '100%', sm: 'calc(100% - 64px)' }, borderRadius: { xs: 0, sm: 1 } } }}
        >
          <DialogTitle>{editingJob ? (editingJob.executionNumber ? `Edit Job #${editingJob.executionNumber}` : 'Edit Pending Item') : 'Queue Job'}</DialogTitle>
          <Box component="form" onSubmit={submit}>
            <DialogContent sx={{ px: { xs: 2, sm: 3 } }}>
              <Grid container spacing={2}>
                <Grid size={{ xs: 12, md: 4 }}>
                  <AssetAutocomplete
                    assets={availableQueueAssets(assets.data?.unprocessed ?? [], allJobs, editingJob?.mediaPath)}
                    value={form.mediaPath}
                    onChange={(path) => updateField('mediaPath', path)}
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 2 }}>
                  <LibraryAutocomplete
                    libraries={libraries.data ?? []}
                    value={form.libraryId}
                    onChange={(libraryId) => updateField('libraryId', libraryId)}
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 2 }}>
                  <ProfileAutocomplete
                    profiles={profiles.data ?? []}
                    value={form.profileId}
                    onChange={(profileId) => updateField('profileId', profileId)}
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 2 }}>
                  <AudioProfileAutocomplete
                    profiles={audioProfiles}
                    value={form.audioProfileKey ?? ''}
                    onChange={(audioProfileKey) => updateField('audioProfileKey', audioProfileKey)}
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 2 }}>
                  <TrackProfileAutocomplete
                    profiles={trackProfiles}
                    value={selectedTrackProfileKey}
                    onChange={setSelectedTrackProfileKey}
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 1 }}>
                  <TextField
                    label="Priority"
                    value={form.priority}
                    onChange={(event) => updateField('priority', Number(event.target.value))}
                    type="number"
                    inputProps={{ min: 1, max: 10 }}
                    fullWidth
                  />
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <TextField
                    label="Notes"
                    value={form.notes}
                    onChange={(event) => updateField('notes', event.target.value)}
                    fullWidth
                  />
                </Grid>
              </Grid>
            {createJob.isError || editJob.isError ? (
              <Alert severity="warning" sx={{ mt: 2 }}>
                Job could not be saved.
              </Alert>
            ) : null}
            {(libraries.data?.length ?? 0) === 0 ? (
              <Alert severity="info" sx={{ mt: 2 }}>
                Create a library before queueing jobs.
              </Alert>
            ) : null}
            </DialogContent>
            <DialogActions sx={{ px: { xs: 2, sm: 3 }, pb: { xs: 2, sm: 1 }, '& .MuiButton-root': { flex: { xs: 1, sm: 'initial' } } }}>
              <Button onClick={closeJobDialog}>Cancel</Button>
              <Button type="submit" variant="contained" disabled={createJob.isPending || editJob.isPending || !form.mediaPath || !form.libraryId || (!form.profileId && !form.audioProfileKey && !selectedTrackProfileKey)}>
                {editingJob ? 'Save' : 'Queue'}
              </Button>
            </DialogActions>
          </Box>
        </Dialog>
        <QueueFlatList jobs={pagedJobs} selectedJobId={selectedJobId} onEditJob={openEditJobDialog} />
        {jobs.isLoading ? <WorkspaceFeedback kind="loading">Loading Queue…</WorkspaceFeedback> : null}
        {!jobs.isLoading && jobs.data?.length === 0 ? (
          <WorkspaceFeedback kind="empty">No conversion jobs have been queued yet.</WorkspaceFeedback>
        ) : null}
        {!jobs.isLoading && allJobs.length > 0 && filteredJobs.length === 0 ? (
          <WorkspaceFeedback kind="empty">No jobs match the current filter.</WorkspaceFeedback>
        ) : null}
      </PageContent>
      <JobDetailsDialog
        key={selectedJob?.id ?? 'closed'}
        job={selectedJob}
        onClose={() => {
          const nextParams = new URLSearchParams(searchParams);
          nextParams.delete('job');
          setSearchParams(nextParams);
        }}
      />
    </>
  );
}

function QueueSummaryStrip({ counts, autoWorkerEnabled }: {
  counts?: QueueStatusCounts;
  autoWorkerEnabled: boolean;
}) {
  const items = [
    { label: 'All jobs', value: counts?.all, color: 'text.primary' },
    { label: 'Queued', value: counts?.queued, color: 'text.secondary' },
    { label: 'Running', value: counts?.running, color: 'info.main' },
    { label: 'Completed', value: counts?.completed, color: 'success.main' },
    { label: 'Failed', value: counts?.failed, color: 'error.main' },
    { label: 'Canceled', value: counts?.canceled, color: 'text.secondary' },
  ] as const;

  return (
    <Box component="section" aria-label="Queue summary">
      <Card variant="outlined">
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: { xs: 'repeat(3, minmax(0, 1fr))', sm: 'repeat(6, minmax(0, 1fr)) auto' },
            alignItems: 'center',
          }}
        >
          {items.map(({ label, value, color }, index) => (
            <Box
              key={label}
              data-testid={`queue-summary-${label.toLowerCase().replaceAll(' ', '-')}`}
              sx={{
                minWidth: 0,
                px: { xs: 0.9, sm: 1.1 },
                py: 0.75,
                borderRight: { xs: index % 3 === 2 ? 0 : 1, sm: index === items.length - 1 ? 0 : 1 },
                borderBottom: { xs: index < 3 ? 1 : 0, sm: 0 },
                borderColor: 'divider',
              }}
            >
              <Typography variant="caption" color="text.secondary" noWrap>{label}</Typography>
              <Typography variant="body2" fontWeight={700} sx={{ color }}>
                {value ?? '—'}
              </Typography>
            </Box>
          ))}
          <Stack
            direction="row"
            alignItems="center"
            justifyContent={{ xs: 'flex-start', sm: 'flex-end' }}
            spacing={0.75}
            sx={{ gridColumn: { xs: '1 / -1', sm: 'auto' }, p: 0.75, borderTop: { xs: 1, sm: 0 }, borderColor: 'divider' }}
          >
            <Chip
              label={`Auto worker ${autoWorkerEnabled ? 'enabled' : 'disabled'}`}
              color={autoWorkerEnabled ? 'info' : 'warning'}
              size="small"
              variant="outlined"
            />
            <Button component={RouterLink} to="/settings" size="small">Settings</Button>
          </Stack>
        </Box>
      </Card>
    </Box>
  );
}

function QueueFlatList({ jobs, selectedJobId, onEditJob }: {
  jobs: QueueJob[];
  selectedJobId: number;
  onEditJob: (job: QueueJob) => void;
}) {
  const queryClient = useQueryClient();
  const [, setSearchParams] = useSearchParams();
  const [draggedJobID, setDraggedJobID] = useState<number | null>(null);
  const [dropTarget, setDropTarget] = useState<{ id: number; placement: 'before' | 'after' } | null>(null);
  const groups = useMemo(() => {
    const jobOrder = new Map(jobs.map((job, index) => [job.id, index]));
    const groupOrder = (group: QueueJobGroup) => Math.min(...group.jobs.map((job) => jobOrder.get(job.id) ?? Number.MAX_SAFE_INTEGER));
    return buildJobGroups(jobs, 'asc')
      .map((group) => ({
        ...group,
        jobs: [...group.jobs].sort((left, right) => (jobOrder.get(left.id) ?? 0) - (jobOrder.get(right.id) ?? 0)),
      }))
      .sort((left, right) => groupOrder(left) - groupOrder(right));
  }, [jobs]);
  const updateJob = useMutation({
    mutationFn: api.updateQueueJob,
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
  });
  const dismissJob = useMutation({
    mutationFn: api.dismissQueueJob,
    onSuccess: async () => Promise.all([
      queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
      queryClient.invalidateQueries({ queryKey: ['assets'] }),
    ]),
  });
  const reorderJob = useMutation({
    mutationFn: api.reorderQueueJob,
    onMutate: async (input) => {
      await queryClient.cancelQueries({ queryKey: ['queueJobs'] });
      const previous = queryClient.getQueryData<QueueJob[]>(['queueJobs']);
      queryClient.setQueryData<QueueJob[]>(['queueJobs'], (current) => current ? reorderQueueJobs(current, input.jobId, input.targetJobId, input.placement) : current);
      return { previous };
    },
    onError: (_error, _input, context) => {
      if (context?.previous) queryClient.setQueryData(['queueJobs'], context.previous);
    },
    onSettled: async () => queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
  });
  const isUpdating = updateJob.isPending || dismissJob.isPending || reorderJob.isPending;

  function openDetails(job: QueueJob) {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.set('job', String(job.id));
      return next;
    });
  }

  function removeJob(job: QueueJob) {
    if (job.executionNumber && !window.confirm(`Remove Job #${job.executionNumber} from Queue? Its database record, logs, and reports will be preserved.`)) return;
    dismissJob.mutate(job.id);
  }

  function handleDragOver(event: DragEvent<HTMLDivElement>, target: QueueJob) {
    if (draggedJobID === null || target.status !== 'queued' || target.id === draggedJobID) return;
    event.preventDefault();
    const bounds = event.currentTarget.getBoundingClientRect();
    setDropTarget({ id: target.id, placement: event.clientY < bounds.top + bounds.height / 2 ? 'before' : 'after' });
  }

  function handleDrop(event: DragEvent<HTMLDivElement>, target: QueueJob) {
    event.preventDefault();
    if (draggedJobID === null || !dropTarget || target.id !== dropTarget.id || target.status !== 'queued') return;
    reorderJob.mutate({ jobId: draggedJobID, targetJobId: target.id, placement: dropTarget.placement });
    setDraggedJobID(null);
    setDropTarget(null);
  }

  if (!jobs.length) return null;
  return (
    <Stack spacing={0.75}>
      {reorderJob.isError ? <Alert severity="warning">The queue order could not be saved. The previous order was restored.</Alert> : null}
      {dismissJob.isError ? <Alert severity="warning">Could not remove this job from Queue.</Alert> : null}
      <Typography variant="body2" color="text.secondary">
        Drag queued jobs by the handle to set their exact execution order. Running and historical jobs are locked.
      </Typography>
      {groups.map((group) => (
        <Box
          component="section"
          key={group.id}
          aria-label={`${group.isBatch ? 'Batch' : 'Path group'}: ${group.name}`}
          sx={{ border: 1, borderColor: 'divider', borderRadius: 1, overflow: 'hidden', bgcolor: 'background.default' }}
        >
          <Stack
            direction={{ xs: 'column', sm: 'row' }}
            alignItems={{ xs: 'stretch', sm: 'center' }}
            justifyContent="space-between"
            spacing={0.5}
            sx={{ px: 1, py: 0.75, borderBottom: 1, borderColor: 'divider', bgcolor: 'background.paper' }}
          >
            <Stack direction="row" alignItems="center" spacing={0.75} flexWrap="wrap" useFlexGap sx={{ minWidth: 0 }}>
              <Chip label={group.isBatch ? 'Batch' : group.jobs.length > 1 ? 'Path group' : 'Single job'} size="small" variant="outlined" />
              <Typography variant="body2" fontWeight={700} noWrap title={group.name}>{group.name}</Typography>
              <Typography variant="caption" color="text.secondary">{group.jobs.length} {group.jobs.length === 1 ? 'job' : 'jobs'}</Typography>
            </Stack>
            <CompactActionGroup aria-label={`Status counts for ${group.name}`}>
              {group.running ? <Typography variant="caption" color="info.main">{group.running} running</Typography> : null}
              {group.queued ? <Typography variant="caption" color="text.secondary">{group.queued} queued</Typography> : null}
              {group.completed ? <Typography variant="caption" color="success.main">{group.completed} completed</Typography> : null}
              {group.failed ? <Typography variant="caption" color="error.main">{group.failed} failed</Typography> : null}
              {group.canceled ? <Typography variant="caption" color="text.secondary">{group.canceled} canceled</Typography> : null}
            </CompactActionGroup>
          </Stack>
          <Stack role="list" aria-label={`Jobs in ${group.name}`} spacing={0.5} sx={{ p: 0.75 }}>
            {group.jobs.map((job) => (
              <FlatJobRow
                key={job.id}
                job={job}
                isSelected={selectedJobId === job.id}
                isUpdating={isUpdating}
                isDragging={draggedJobID === job.id}
                dropPlacement={dropTarget?.id === job.id ? dropTarget.placement : undefined}
                onDragStart={(event) => {
                  if (job.status !== 'queued' || isUpdating) return;
                  event.dataTransfer.effectAllowed = 'move';
                  event.dataTransfer.setData('text/plain', String(job.id));
                  setDraggedJobID(job.id);
                }}
                onDragEnd={() => { setDraggedJobID(null); setDropTarget(null); }}
                onDragOver={(event) => handleDragOver(event, job)}
                onDrop={(event) => handleDrop(event, job)}
                onDetails={() => openDetails(job)}
                onEdit={() => onEditJob(job)}
                onCancel={() => updateJob.mutate({ jobId: job.id, status: 'canceled' })}
                onRequeue={() => updateJob.mutate({ jobId: job.id, status: 'queued' })}
                onRemove={() => removeJob(job)}
              />
            ))}
          </Stack>
        </Box>
      ))}
    </Stack>
  );
}

function FlatJobRow({
  job, isSelected, isUpdating, isDragging, dropPlacement, onDragStart, onDragEnd, onDragOver, onDrop,
  onDetails, onEdit, onCancel, onRequeue, onRemove,
}: {
  job: QueueJob;
  isSelected: boolean;
  isUpdating: boolean;
  isDragging: boolean;
  dropPlacement?: 'before' | 'after';
  onDragStart: (event: DragEvent<HTMLButtonElement>) => void;
  onDragEnd: () => void;
  onDragOver: (event: DragEvent<HTMLDivElement>) => void;
  onDrop: (event: DragEvent<HTMLDivElement>) => void;
  onDetails: () => void;
  onEdit: () => void;
  onCancel: () => void;
  onRequeue: () => void;
  onRemove: () => void;
}) {
  const stage = queueStagePresentation(job);
  const timing = jobTiming(job);
  const canEdit = job.status === 'queued' || job.status === 'failed' || job.status === 'canceled';
  const canCancel = job.status === 'queued' || job.status === 'running';
  const canRequeue = job.status === 'failed' || job.status === 'canceled';
  const canRemove = job.status === 'queued' || job.status === 'failed' || job.status === 'canceled';
  const nextAction = nextPipelineAction(job);
  const hasDeterminateProgress = hasDeterminateQueueProgress(job.progress);
  const jobLabel = job.executionNumber ? `Job #${job.executionNumber}` : `Pending job ${job.id}`;
  const mediaName = fileNameFromPath(job.mediaPath);
  return (
    <Box
      component="article"
      role="listitem"
      aria-label={`${jobLabel}: ${mediaName}`}
      aria-current={isSelected ? 'true' : undefined}
      data-testid={`queue-job-${job.id}`}
      data-selected={isSelected ? 'true' : 'false'}
      data-running={job.status === 'running' ? 'true' : 'false'}
      onDragOver={onDragOver}
      onDrop={onDrop}
      sx={{
        position: 'relative',
        border: 1,
        borderLeftWidth: isSelected || job.status === 'running' ? 3 : 1,
        borderColor: (theme) => isSelected
          ? getMVForgeTokens(theme).interaction.selectedBorder
          : job.status === 'running'
            ? getMVForgeTokens(theme).colors.status.info
            : 'divider',
        borderRadius: 1,
        p: { xs: 0.75, sm: 1 },
        bgcolor: isSelected ? undefined : job.status === 'running' ? (theme) => getMVForgeTokens(theme).colors.status.infoSurface : 'background.paper',
        background: isSelected ? (theme) => getMVForgeTokens(theme).interaction.selectedBackground : undefined,
        opacity: isDragging ? 0.55 : 1,
        '&:hover': { borderColor: (theme) => getMVForgeTokens(theme).interaction.hoverBorder },
        '&::before': dropPlacement === 'before' ? dropIndicator('top') : undefined,
        '&::after': dropPlacement === 'after' ? dropIndicator('bottom') : undefined,
      }}
    >
      <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '32px minmax(0, 1fr)', lg: '32px minmax(0, 1.1fr) minmax(0, 1.1fr) minmax(130px, 0.7fr) auto' }, gap: { xs: 0.75, sm: 1 }, alignItems: 'center' }}>
        <Tooltip title={job.status === 'queued' ? 'Drag to reorder' : 'Only queued jobs can be reordered'}>
          <span>
            <IconButton draggable={job.status === 'queued' && !isUpdating} onDragStart={onDragStart} onDragEnd={onDragEnd} disabled={job.status !== 'queued' || isUpdating} size="small" aria-label={`Reorder ${mediaName}`} sx={{ cursor: job.status === 'queued' ? 'grab' : 'default' }}>
              <DragIndicatorIcon fontSize="small" />
            </IconButton>
          </span>
        </Tooltip>
        <Stack spacing={0.4} sx={{ minWidth: 0 }}>
          <Typography fontWeight={700} variant="body2" noWrap title={mediaName}>
            {job.executionNumber ? `#${job.executionNumber} · ` : ''}{mediaName}
          </Typography>
          <Typography variant="caption" color="text.secondary" noWrap title={job.mediaPath}>
            {compactPath(job.mediaPath)}
          </Typography>
        </Stack>
        <Stack spacing={0.35} sx={{ minWidth: 0, gridColumn: { xs: '1 / -1', lg: 'auto' } }}>
          <CompactActionGroup aria-label={`Status for ${jobLabel}`}>
            <Chip label={queueStatusLabel(job.status)} color={queueStatusColor(job.status)} size="small" aria-label={`Queue status: ${queueStatusLabel(job.status)}`} />
            <Chip label={stage.label} color={stage.color} size="small" variant="outlined" aria-label={`Queue stage: ${stage.label}`} />
            {job.validationStatus === 'warning' ? <Chip label="Validation warning" color="warning" size="small" /> : null}
            {job.validationStatus === 'failed' ? <Chip label="Validation failed" color="error" size="small" /> : null}
          </CompactActionGroup>
          <CompactActionGroup aria-label={`Operational details for ${jobLabel}`}>
            {job.status === 'queued' ? <Typography variant="caption" color="text.secondary">Queue {job.queuePosition || 'pending'}</Typography> : null}
            <Typography variant="caption" color="text.secondary">Worker: {job.workerName || 'Unassigned'}</Typography>
            <Typography variant="caption" color="text.secondary">
              Library #{job.libraryId} · {job.processingMode === 'audio_only' ? 'Audio only' : `Video profile #${job.profileId}`}
            </Typography>
            {job.audioProfileKey ? <Typography variant="caption" color="text.secondary">Audio: {job.audioProfileKey}</Typography> : null}
            {job.trackProfileKey ? <Typography variant="caption" color="text.secondary">Tracks: {job.trackProfileKey}</Typography> : null}
          </CompactActionGroup>
        </Stack>
        <Stack spacing={0.35} sx={{ minWidth: 0, gridColumn: { xs: '1 / -1', lg: 'auto' } }}>
          {timing.elapsed ? <Typography variant="caption" color="text.secondary">Elapsed {timing.elapsed}</Typography> : null}
          {timing.eta ? <Typography variant="caption" color="text.secondary">ETA {timing.eta}</Typography> : null}
          {job.status === 'running' ? (
            <Stack spacing={0.2} aria-label={`Progress for ${jobLabel}`}>
              <Typography variant="caption" color="text.secondary">
                {hasDeterminateProgress ? `Progress ${job.progress}%` : stage.label}
              </Typography>
              <LinearProgress
                variant={hasDeterminateProgress ? 'determinate' : 'indeterminate'}
                value={hasDeterminateProgress ? job.progress : undefined}
                sx={{ height: 4, borderRadius: 1 }}
              />
            </Stack>
          ) : null}
        </Stack>
        <CompactActionGroup aria-label={`Actions for ${jobLabel}`} justifyContent={{ xs: 'flex-start', lg: 'flex-end' }} sx={{ gridColumn: { xs: '1 / -1', lg: 'auto' }, borderTop: { xs: 1, lg: 0 }, borderColor: 'divider', pt: { xs: 0.5, lg: 0 } }}>
          {nextAction ? <Tooltip title={nextAction.label}><IconButton component={RouterLink} to={nextAction.to} size="small" color="primary" aria-label={`${nextAction.label} for ${jobLabel}`}><OpenInNewIcon fontSize="small" /></IconButton></Tooltip> : null}
          <Tooltip title="Details"><IconButton size="small" color={job.status === 'failed' ? 'error' : 'primary'} onClick={onDetails} aria-label={`Details for ${jobLabel}`}><ErrorOutlineIcon fontSize="small" /></IconButton></Tooltip>
          <Tooltip title="Edit"><span><IconButton size="small" disabled={isUpdating || !canEdit} onClick={onEdit} aria-label={`Edit ${jobLabel}`}><EditIcon fontSize="small" /></IconButton></span></Tooltip>
          <Tooltip title="Cancel"><span><IconButton size="small" color="warning" disabled={isUpdating || !canCancel} onClick={onCancel} aria-label={`Cancel ${jobLabel}`}><CancelIcon fontSize="small" /></IconButton></span></Tooltip>
          <Tooltip title="Requeue"><span><IconButton size="small" color="primary" disabled={isUpdating || !canRequeue} onClick={onRequeue} aria-label={`Requeue ${jobLabel}`}><ReplayIcon fontSize="small" /></IconButton></span></Tooltip>
          <Tooltip title="Remove"><span><IconButton size="small" color="error" disabled={isUpdating || !canRemove} onClick={onRemove} aria-label={`Remove ${jobLabel}`}><RemoveCircleOutlineIcon fontSize="small" /></IconButton></span></Tooltip>
        </CompactActionGroup>
      </Box>
      {job.errorMessage ? <Alert severity={job.status === 'failed' ? 'error' : 'warning'} sx={{ mt: 0.75 }}>{shortText(job.errorMessage, 180)}</Alert> : null}
    </Box>
  );
}

function dropIndicator(edge: 'top' | 'bottom') {
  return { content: '""', position: 'absolute', left: 4, right: 4, [edge]: -3, height: 3, borderRadius: 3, bgcolor: 'primary.main', zIndex: 1 };
}

function reorderQueueJobs(jobs: QueueJob[], jobID: number, targetJobID: number, placement: 'before' | 'after') {
  const queued = jobs.filter((job) => job.status === 'queued').sort((left, right) => (left.queuePosition || Number.MAX_SAFE_INTEGER) - (right.queuePosition || Number.MAX_SAFE_INTEGER));
  const draggedIndex = queued.findIndex((job) => job.id === jobID);
  if (draggedIndex < 0) return jobs;
  const [dragged] = queued.splice(draggedIndex, 1);
  const targetIndex = queued.findIndex((job) => job.id === targetJobID);
  if (targetIndex < 0) return jobs;
  queued.splice(targetIndex + (placement === 'after' ? 1 : 0), 0, dragged);
  const positions = new Map(queued.map((job, index) => [job.id, index + 1]));
  return jobs.map((job) => positions.has(job.id) ? { ...job, queuePosition: positions.get(job.id) ?? job.queuePosition } : job);
}

type QueueJobGroup = {
  id: string;
  name: string;
  jobs: QueueJob[];
  isBatch: boolean;
  batchId: string;
  completed: number;
  failed: number;
  running: number;
  queued: number;
  canceled: number;
};

function AssetAutocomplete({ assets, value, onChange }: { assets: Asset[]; value: string; onChange: (path: string) => void }) {
  const selected = assets.find((asset) => asset.path === value) ?? null;

  return (
    <Autocomplete
      freeSolo
      options={assets}
      value={selected}
      inputValue={value}
      onInputChange={(_, nextValue) => onChange(nextValue)}
      onChange={(_, asset) => onChange(typeof asset === 'string' ? asset : asset?.path ?? '')}
      getOptionLabel={(asset) => (typeof asset === 'string' ? asset : asset.relativePath || asset.path)}
      filterOptions={(options, state) => filterAssets(options, state.inputValue)}
      renderInput={(params) => <TextField {...params} label="Media path" required placeholder="/media/raw/movie.mkv" />}
      renderOption={(props, asset) => (
        <Box component="li" {...props} key={asset.path}>
          <Stack sx={{ minWidth: 0 }}>
            <Typography fontWeight={700} noWrap>
              {asset.fileName}
            </Typography>
            <Typography color="text.secondary" variant="body2" noWrap>
              {asset.relativePath || asset.path}
            </Typography>
          </Stack>
        </Box>
      )}
      fullWidth
    />
  );
}

function LibraryAutocomplete({ libraries, value, onChange }: { libraries: Library[]; value: number; onChange: (id: number) => void }) {
  return (
    <Autocomplete
      options={libraries}
      value={libraries.find((library) => library.id === value) ?? null}
      onChange={(_, library) => onChange(library?.id ?? 0)}
      getOptionLabel={(library) => library.name}
      isOptionEqualToValue={(option, selected) => option.id === selected.id}
      filterOptions={(options, state) => filterByText(options, state.inputValue, (library) => [library.name, library.type, library.destinationPath])}
      renderInput={(params) => <TextField {...params} label="Library" required />}
      fullWidth
    />
  );
}

function ProfileAutocomplete({ profiles, value, onChange }: { profiles: Profile[]; value: number; onChange: (id: number) => void }) {
  const none = { id: 0, name: 'None', videoCodec: 'copy', description: '', container: '', audioCodec: '', none: true };
  const options = [none, ...profiles];
  return (
    <Autocomplete
      options={options}
      value={options.find((profile) => profile.id === value) ?? none}
      onChange={(_, profile) => onChange(profile?.id ?? 0)}
      getOptionLabel={(profile) => profile.id ? `${profile.name} · ${profile.videoCodec}` : 'None'}
      isOptionEqualToValue={(option, selected) => option.id === selected.id}
      filterOptions={(options, state) =>
        filterByText(options, state.inputValue, (profile) => [
          profile.name,
          profile.description,
          profile.container,
          profile.videoCodec,
          profile.audioCodec,
        ])
      }
      renderInput={(params) => <TextField {...params} label="Video profile" />}
      fullWidth
    />
  );
}

function AudioProfileAutocomplete({
  profiles,
  value,
  onChange,
}: {
  profiles: AudioEnhancementProfile[];
  value: string;
  onChange: (key: string) => void;
}) {
  const none: AudioEnhancementProfile = {
    key: '', name: 'None', description: '', intent: '', filters: '', rnnoiseModelPath: '', channelMode: 'preserve',
    forceStereoMode: 'auto', stereoDelayMs: 0, stereoWidth: 0, eqBands: {}, preserveOriginalTrack: true,
    outputCodec: 'copy', targetLoudness: 0, truePeak: 0, notes: '',
  };
  const options = [none, ...profiles];
  return (
    <Autocomplete
      options={options}
      value={options.find((profile) => profile.key === value) ?? none}
      onChange={(_, profile) => onChange(profile?.key ?? '')}
      getOptionLabel={(profile) => profile.key ? `${profile.name} · ${profile.outputCodec || 'copy'}` : 'None'}
      isOptionEqualToValue={(option, selected) => option.key === selected.key}
      filterOptions={(options, state) =>
        filterByText(options, state.inputValue, (profile) => [
          profile.name,
          profile.description,
          profile.intent,
          profile.outputCodec,
          profile.key,
        ])
      }
      renderInput={(params) => <TextField {...params} label="Audio profile" />}
      fullWidth
    />
  );
}

function TrackProfileAutocomplete({ profiles, value, onChange }: { profiles: TrackProfile[]; value: string; onChange: (key: string) => void }) {
  const none: TrackProfile = { ...emptyTrackProfile, key: '', name: 'None', audioRequired: false, subtitlesRequired: false };
  const options = [none, ...profiles];
  return (
    <Autocomplete
      options={options}
      value={options.find((profile) => profile.key === value) ?? none}
      onChange={(_, profile) => onChange(profile?.key ?? '')}
      getOptionLabel={(profile) => profile.name}
      isOptionEqualToValue={(option, selected) => option.key === selected.key}
      filterOptions={(items, state) => filterByText(items, state.inputValue, (profile) => [profile.name, profile.description, profile.key])}
      renderInput={(params) => <TextField {...params} label="Track profile" />}
      fullWidth
    />
  );
}

function filterAssets(assets: Asset[], inputValue: string) {
  return filterByText(assets, inputValue, (asset) => [asset.fileName, asset.relativePath, asset.path, asset.extension]);
}

function availableQueueAssets(assets: Asset[], jobs: QueueJob[], currentPath = '') {
  return assets.filter((asset) => asset.path === currentPath || !jobIsOpenForPath(jobs, asset.path));
}

function findQueueAsset(inventory: AssetInventory | undefined, mediaPath: string) {
  if (!inventory) return undefined;
  return [inventory.unprocessed, inventory.library, inventory.unverified, inventory.converted]
    .flat()
    .find((asset) => asset.path === mediaPath);
}

function jobIsOpenForPath(jobs: QueueJob[], mediaPath: string) {
  return jobs.some((job) => job.mediaPath === mediaPath && job.status !== 'canceled' && !job.publishedAt);
}

function filterByText<T>(items: T[], inputValue: string, getValues: (item: T) => string[]) {
  const query = inputValue.trim().toLowerCase();
  if (!query) {
    return items.slice(0, 50);
  }

  return items
    .filter((item) => getValues(item).some((value) => value.toLowerCase().includes(query)))
    .slice(0, 50);
}

function getAudioProfiles(settings?: AppSetting[]) {
  const value = settings?.find((setting) => setting.key === 'audioEnhancementProfiles')?.value.profiles;
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter((profile): profile is AudioEnhancementProfile => {
    if (!profile || typeof profile !== 'object') {
      return false;
    }
    const candidate = profile as Partial<AudioEnhancementProfile>;
    return Boolean(candidate.key && candidate.name && !candidate.disabled && !candidate.deletedAt);
  });
}

export function QueueGroupCard({
  group,
  onEditJob,
  initiallyExpanded = false,
}: {
  group: QueueJobGroup;
  onEditJob: (job: QueueJob) => void;
  initiallyExpanded?: boolean;
}) {
  const queryClient = useQueryClient();
  const [expanded, setExpanded] = useState(initiallyExpanded);
  const [visibleJobsCount, setVisibleJobsCount] = useState(25);
  const updateJob = useMutation({
    mutationFn: api.updateQueueJob,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
    },
  });
  const dismissJob = useMutation({
    mutationFn: api.dismissQueueJob,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
      ]);
    },
  });
  const batchAction = useMutation({
    mutationFn: async (action: 'cancel' | 'remove' | 'requeue') => {
      const jobs = group.jobs.filter((job) => job.status !== 'completed');
      if (action === 'requeue') {
        const recoverable = jobs
          .filter((job) => job.status === 'failed' || job.status === 'canceled')
          .sort((left, right) => (left.batchPosition || Number.MAX_SAFE_INTEGER) - (right.batchPosition || Number.MAX_SAFE_INTEGER) || left.id - right.id);
        for (const job of recoverable) {
          await api.updateQueueJob({ jobId: job.id, status: 'queued' });
        }
        return;
      }
      if (action === 'cancel') {
        await Promise.all(
          jobs
            .filter((job) => job.status === 'queued' || job.status === 'running')
            .map((job) => api.updateQueueJob({ jobId: job.id, status: 'canceled' })),
        );
        return;
      }
      if (!group.batchId) {
        throw new Error('This Queue group does not have a batch identifier.');
      }
      await api.dismissQueueBatch(group.batchId);
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
      ]);
    },
  });

  function updatePriority(job: QueueJob, nextPriority: number) {
    updateJob.mutate({ jobId: job.id, priority: nextPriority });
  }

  function cancelJob(job: QueueJob) {
    updateJob.mutate({ jobId: job.id, status: 'canceled' });
  }

  function requeueJob(job: QueueJob) {
    updateJob.mutate({ jobId: job.id, status: 'queued' });
  }

  function removeJob(job: QueueJob) {
    if (job.executionNumber && !window.confirm(`Remove Job #${job.executionNumber} from Queue? Its database record, logs, and reports will be preserved.`)) return;
    dismissJob.mutate(job.id);
  }

  function cancelBatch() {
    const count = group.jobs.filter((job) => job.status === 'queued' || job.status === 'running').length;
    if (!count || !window.confirm(`Cancel ${count} queued/running job${count === 1 ? '' : 's'} in “${group.name}”?`)) return;
    batchAction.mutate('cancel');
  }

  function removeBatch() {
    if (!canRemoveQueueBatch(group.jobs)) return;
    const removable = group.jobs.filter((job) => job.status === 'canceled');
    const count = removable.length;
    if (!count) return;
    if (!window.confirm(`Remove ${count} cancelled job${count === 1 ? '' : 's'} from “${group.name}”? Their database records, logs, and reports will be preserved.`)) return;
    batchAction.mutate('remove');
  }

  function requeueBatch() {
    const count = group.jobs.filter((job) => job.status === 'failed' || job.status === 'canceled').length;
    if (!count || !window.confirm(`Requeue ${count} failed/canceled job${count === 1 ? '' : 's'} in “${group.name}” using their saved batch order and configuration?`)) return;
    batchAction.mutate('requeue');
  }

  const visibleJobs = group.jobs.slice(0, visibleJobsCount);

  return (
    <Card sx={{ overflow: 'hidden' }}>
      <CardContent sx={{ p: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Stack spacing={1}>
          {dismissJob.isError ? <Alert severity="warning">Could not remove this job from Queue.</Alert> : null}
          {batchAction.isError ? <Alert severity="warning">Could not complete the batch action. Refresh Queue to review the remaining jobs.</Alert> : null}
          <Stack direction={{ xs: 'column', md: 'row' }} alignItems={{ xs: 'stretch', md: 'center' }} justifyContent="space-between" spacing={1.5}>
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap sx={{ minWidth: 0 }}>
              <Typography variant="h3" sx={{ wordBreak: 'break-word' }}>
                {group.name}
              </Typography>
              <Chip label={group.isBatch ? 'Batch' : group.jobs.length > 1 ? 'Path group' : 'Single job'} color={group.isBatch ? 'primary' : 'default'} size="small" />
              <Chip label={`${group.completed}/${group.jobs.length} completed`} color="success" size="small" />
              <Chip label={`${group.queued} queued`} color="default" size="small" />
              <Chip label={`${group.running} running`} color="info" size="small" />
              {group.failed ? <Chip label={`${group.failed} failed`} color="error" size="small" /> : null}
              {group.canceled ? <Chip label={`${group.canceled} canceled`} color="default" size="small" /> : null}
            </Stack>
            <Stack
              direction={{ xs: 'column', sm: 'row' }}
              spacing={1}
              alignItems="stretch"
              sx={{ flexShrink: 0, '& .MuiButton-root': { width: { xs: '100%', sm: 'auto' } } }}
            >
              {group.isBatch && (group.queued > 0 || group.running > 0) ? (
                <Button
                  startIcon={<CancelIcon />}
                  color="warning"
                  variant="outlined"
                  size="small"
                  disabled={batchAction.isPending}
                  onClick={cancelBatch}
                >
                  Cancel Batch
                </Button>
              ) : null}
              {group.isBatch && canRemoveQueueBatch(group.jobs) ? (
                <Button
                  startIcon={<RemoveCircleOutlineIcon />}
                  color="error"
                  variant="outlined"
                  size="small"
                  disabled={batchAction.isPending}
                  onClick={removeBatch}
                >
                  Remove Batch
                </Button>
              ) : null}
              {group.jobs.some((job) => job.status === 'failed' || job.status === 'canceled') ? (
                <Button
                  startIcon={<ReplayIcon />}
                  color="primary"
                  variant="outlined"
                  size="small"
                  disabled={batchAction.isPending}
                  onClick={requeueBatch}
                >
                  Requeue Batch
                </Button>
              ) : null}
              <IconButton
                color="primary"
                onClick={() => setExpanded((current) => {
                  if (current) setVisibleJobsCount(25);
                  return !current;
                })}
                aria-label={expanded ? 'Collapse queue group' : 'Expand queue group'}
                sx={{ border: 1, borderColor: 'divider', width: 44, height: 44 }}
              >
                <ExpandMoreIcon
                  sx={{
                    transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
                    transition: (theme) => theme.transitions.create('transform', { duration: theme.transitions.duration.shortest }),
                  }}
                />
              </IconButton>
            </Stack>
          </Stack>
          <Collapse in={expanded} timeout="auto" unmountOnExit>
            <Stack spacing={1.25} sx={{ pt: 1 }}>
              {visibleJobs.map((job) => (
                <JobRow
                  key={job.id}
                  job={job}
                  isUpdating={updateJob.isPending || dismissJob.isPending || batchAction.isPending}
                  onCancel={cancelJob}
                  onEdit={onEditJob}
                  onPriorityChange={updatePriority}
                  onRequeue={requeueJob}
                  onRemove={removeJob}
                />
              ))}
              {visibleJobsCount < group.jobs.length ? (
                <Button variant="outlined" size="small" onClick={() => setVisibleJobsCount((current) => current + 25)}>
                  Show more jobs ({group.jobs.length - visibleJobsCount} remaining)
                </Button>
              ) : null}
            </Stack>
          </Collapse>
        </Stack>
      </CardContent>
    </Card>
  );
}

function JobRow({
  job,
  isUpdating,
  onCancel,
  onEdit,
  onPriorityChange,
  onRequeue,
  onRemove,
}: {
  job: QueueJob;
  isUpdating: boolean;
  onCancel: (job: QueueJob) => void;
  onEdit: (job: QueueJob) => void;
  onPriorityChange: (job: QueueJob, nextPriority: number) => void;
  onRequeue: (job: QueueJob) => void;
  onRemove: (job: QueueJob) => void;
}) {
  const [, setSearchParams] = useSearchParams();
  const canCancel = job.status === 'queued' || job.status === 'running';
  const canRequeue = job.status === 'failed' || job.status === 'canceled';
  const canEdit = job.status === 'queued' || job.status === 'failed' || job.status === 'canceled';
  const canRemove = job.status === 'queued' || job.status === 'failed' || job.status === 'canceled';
  const stage = queueStagePresentation(job);
  const timing = jobTiming(job);

  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 0.9, bgcolor: 'background.paper' }}>
      <Stack spacing={0.9}>
        <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" spacing={1}>
          <Stack spacing={0.4} sx={{ minWidth: 0 }}>
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
              <Typography fontWeight={700}>
                {job.executionNumber ? `Job #${job.executionNumber}` : 'Pending'}
              </Typography>
              <Typography fontWeight={700} noWrap>
                {fileNameFromPath(job.mediaPath)}
              </Typography>
            </Stack>
            <Typography color="text.secondary" variant="body2" noWrap>
              {compactPath(job.mediaPath)}
            </Typography>
          </Stack>
          <Stack
            direction="row"
            spacing={0.75}
            flexWrap="wrap"
            useFlexGap
            sx={{ '& .MuiButton-root': { flex: { xs: '1 1 calc(50% - 6px)', sm: '0 0 auto' } } }}
          >
            {nextPipelineAction(job) ? (
              <Button component={RouterLink} to={nextPipelineAction(job)?.to ?? '#'} variant="contained" size="small">
                {nextPipelineAction(job)?.label}
              </Button>
            ) : null}
              <Button
              startIcon={<ErrorOutlineIcon />}
              color={job.status === 'failed' ? 'error' : 'primary'}
              variant="outlined"
              size="small"
              onClick={() => setSearchParams({ status: job.status, job: String(job.id) })}
            >
              Details
            </Button>
            <Button
              startIcon={<EditIcon />}
              variant="outlined"
              size="small"
              disabled={isUpdating || !canEdit}
              onClick={() => onEdit(job)}
            >
              Edit
            </Button>
            <Button
              startIcon={<KeyboardArrowUpIcon />}
              variant="outlined"
              size="small"
              disabled={isUpdating || job.priority <= 1 || job.status === 'completed'}
              onClick={() => onPriorityChange(job, job.priority - 1)}
            >
              Higher
            </Button>
            <Button
              startIcon={<KeyboardArrowDownIcon />}
              variant="outlined"
              size="small"
              disabled={isUpdating || job.priority >= 10 || job.status === 'completed'}
              onClick={() => onPriorityChange(job, job.priority + 1)}
            >
              Lower
            </Button>
            <Button
              startIcon={<CancelIcon />}
              color="warning"
              variant="outlined"
              size="small"
              disabled={isUpdating || !canCancel}
              onClick={() => onCancel(job)}
            >
              Cancel
            </Button>
            <Button
              startIcon={<ReplayIcon />}
              variant="contained"
              size="small"
              disabled={isUpdating || !canRequeue}
              onClick={() => onRequeue(job)}
            >
              Requeue
            </Button>
            <Button
              startIcon={<RemoveCircleOutlineIcon />}
              color="error"
              variant="outlined"
              size="small"
              disabled={isUpdating || !canRemove}
              onClick={() => onRemove(job)}
            >
              Remove
            </Button>
          </Stack>
        </Stack>
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
          <Chip label={queueStatusLabel(job.status)} color={queueStatusColor(job.status)} size="small" />
          <Chip label={stage.label} color={stage.color} size="small" variant="outlined" />
          <Typography color="text.secondary" variant="body2">
            {job.workerName || 'Unclaimed'}
          </Typography>
          <Typography color="text.secondary" variant="body2">Priority: {job.priority}</Typography>
          <Typography color="text.secondary" variant="body2">Progress: {job.progress}%</Typography>
          <Typography color="text.secondary" variant="body2">
            Library: {job.libraryId}
          </Typography>
          <Typography color="text.secondary" variant="body2">
            Video: {job.processingMode === 'audio_only' ? 'None (copy)' : `Profile ${job.profileId} · v${job.profileVersion || 1}`}
          </Typography>
          {job.audioProfileKey ? (
            <Typography color="text.secondary" variant="body2">
              Audio: {job.audioProfileKey}
            </Typography>
          ) : null}
          {job.trackProfileKey ? (
            <Typography color="text.secondary" variant="body2">
              Tracks: {job.trackProfileKey}
            </Typography>
          ) : null}
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
        <LinearProgress variant="determinate" value={job.progress} />
        {job.errorMessage ? (
          <Alert
            severity={job.status === 'failed' ? 'error' : 'warning'}
            action={
              <Button color="inherit" size="small" onClick={() => setSearchParams({ status: job.status, job: String(job.id) })}>Details</Button>
            }
          >
            {shortText(job.errorMessage, 240)}
          </Alert>
        ) : null}
        {!job.errorMessage && job.notes ? (
          <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-word' }}>
            {shortText(job.notes, 180)}
          </Typography>
        ) : null}
      </Stack>
    </Box>
  );
}

function buildJobGroups(jobs: QueueJob[], sortDirection: 'asc' | 'desc') {
  const groups = new Map<string, QueueJobGroup>();

  [...jobs].sort((left, right) => sortJobIds(left, right, sortDirection)).forEach((job) => {
    const groupId = job.batchId ? `batch:${job.batchId}` : queueGroupPathFromMediaPath(job.mediaPath);
    const existing = groups.get(groupId);
    const group =
      existing ??
      ({
        id: groupId,
        name: job.batchName || queueGroupPathFromMediaPath(job.mediaPath),
        jobs: [],
        isBatch: Boolean(job.batchId),
        batchId: job.batchId || '',
        completed: 0,
        failed: 0,
        running: 0,
        queued: 0,
        canceled: 0,
      } satisfies QueueJobGroup);

    group.jobs.push(job);
    if (job.batchId && !group.batchId) {
      group.batchId = job.batchId;
      group.isBatch = true;
    }
    groups.set(groupId, group);
  });

  return [...groups.values()].map((group) => {
    group.jobs.sort((left, right) => sortJobsByPipelineState(left, right, sortDirection));
    const completed = group.jobs.filter((job) => job.status === 'completed').length;
    const failed = group.jobs.filter((job) => job.status === 'failed').length;
    const running = group.jobs.filter((job) => job.status === 'running').length;
    const queued = group.jobs.filter((job) => job.status === 'queued').length;
    const canceled = group.jobs.filter((job) => job.status === 'canceled').length;
    return {
      ...group,
      completed,
      failed,
      running,
      queued,
      canceled,
    };
  }).sort((left, right) => {
    if (left.running !== right.running) {
      return right.running - left.running;
    }
    const leftID = sortDirection === 'desc' ? Math.max(...left.jobs.map((job) => job.id)) : Math.min(...left.jobs.map((job) => job.id));
    const rightID = sortDirection === 'desc' ? Math.max(...right.jobs.map((job) => job.id)) : Math.min(...right.jobs.map((job) => job.id));
    const order = sortDirection === 'desc' ? rightID - leftID : leftID - rightID;
    return order || left.name.localeCompare(right.name);
  });
}

function latestJobsByAsset(jobs: QueueJob[]) {
  const latest = new Map<string, QueueJob>();
  for (const job of jobs) {
    const key = queueAssetIdentity(job.mediaPath);
    const current = latest.get(key);
    if (!current || job.id > current.id) {
      latest.set(key, job);
    }
  }
  return [...latest.values()];
}

function queueJobsForDisplay(jobs: QueueJob[]) {
  const visible = new Map(latestJobsByAsset(jobs).map((job) => [job.id, job]));
  const actionableBatchIds = new Set(
    jobs
      .filter((job) => job.batchId && job.status !== 'completed')
      .map((job) => job.batchId),
  );

  // A batch is one Queue operation. Keep all of its members together while any
  // member can still be canceled, requeued, or removed, even when a newer job
  // for the same asset would otherwise replace it in the latest-job view.
  for (const job of jobs) {
    if (job.batchId && actionableBatchIds.has(job.batchId)) {
      visible.set(job.id, job);
    }
  }
  return [...visible.values()];
}

function sortJobsForQueue(left: QueueJob, right: QueueJob) {
  const statusDiff = queueStatusRank(left.status) - queueStatusRank(right.status);
  if (statusDiff !== 0) return statusDiff;
  if (left.status === 'queued' && right.status === 'queued') {
    return (left.queuePosition || Number.MAX_SAFE_INTEGER) - (right.queuePosition || Number.MAX_SAFE_INTEGER) || left.id - right.id;
  }
  if (left.status === 'running' && right.status === 'running') {
    return new Date(left.startedAt ?? left.createdAt).getTime() - new Date(right.startedAt ?? right.createdAt).getTime();
  }
  return right.id - left.id;
}

function queueAssetIdentity(mediaPath: string) {
  const normalized = normalizePath(mediaPath).replace(/^\/+/, '');
  const rawMarker = 'media/raw/';
  const rawIndex = normalized.indexOf(rawMarker);
  return rawIndex >= 0 ? normalized.slice(rawIndex + rawMarker.length) : normalized;
}

function sortJobsByPipelineState(left: QueueJob, right: QueueJob, direction: 'asc' | 'desc') {
  const statusDiff = queueStatusRank(left.status) - queueStatusRank(right.status);
  if (statusDiff !== 0) {
    return statusDiff;
  }
  return sortJobIds(left, right, direction);
}

function queueStatusRank(status: QueueJob['status']) {
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

function sortJobIds(left: QueueJob, right: QueueJob, direction: 'asc' | 'desc') {
  return direction === 'desc' ? right.id - left.id : left.id - right.id;
}

function groupPathFromMediaPath(mediaPath: string) {
  const normalized = mediaPath.replace(/\\/g, '/');
  const index = normalized.lastIndexOf('/');
  if (index <= 0) {
    return 'Root';
  }
  return normalized.slice(0, index);
}
function queueGroupPathFromMediaPath(mediaPath: string) {
  return groupPathFromMediaPath(normalizePath(mediaPath));
}

function normalizePath(value: string) {
  return value.replace(/\\/g, '/').replace(/\/+$/, '');
}

function nextPipelineAction(job: QueueJob): { label: string; to: string } | null {
  if (job.publishedAt) {
    return null;
  }
  if (job.validationStatus === 'passed' || job.validationStatus === 'warning') {
    return { label: 'Publish', to: '/publisher' };
  }
  if (job.status === 'completed') {
    if (job.validationStatus) {
      return { label: 'Validate', to: '/validation' };
    }
    const params = new URLSearchParams({
      asset: job.mediaPath,
      job: String(job.id),
      autorun: '1',
    });
    return { label: 'Analyze', to: `/analysis?${params.toString()}` };
  }
  if (job.status === 'queued' || job.status === 'running') {
    return { label: 'Workers', to: '/workers' };
  }
  return null;
}

function fileNameFromPath(path: string) {
  return path.split('/').filter(Boolean).pop() || path;
}

function compactPath(path: string) {
  const parts = path.split('/').filter(Boolean);
  if (parts.length <= 3) {
    return path;
  }
  return `.../${parts.slice(-3).join('/')}`;
}

function shortText(value: string, maxLength: number) {
  const compact = value.replace(/\s+/g, ' ').trim();
  if (compact.length <= maxLength) {
    return compact;
  }
  return `${compact.slice(0, maxLength - 1)}...`;
}

type QueueStatusCounts = Record<QueueJob['status'] | 'all', number>;

function summarizeStatusCounts(jobs: QueueJob[]): QueueStatusCounts {
  return jobs.reduce(
    (summary, job) => ({ ...summary, [job.status]: summary[job.status] + 1, all: summary.all + 1 }),
    { all: 0, queued: 0, running: 0, completed: 0, failed: 0, canceled: 0 },
  );
}

function normalizeStatusFilter(value: string | null): QueueJob['status'] | 'all' {
  return value === 'all' || value === 'queued' || value === 'running' || value === 'completed' || value === 'failed' || value === 'canceled'
    ? value
    : 'all';
}

function statusLabel(status: QueueJob['status'] | 'all') {
  return status === 'all' ? 'All' : queueStatusLabel(status);
}

function getWorkerSettings(settings?: AppSetting[]) {
  const value = settings?.find((setting) => setting.key === 'workers')?.value ?? {};
  return {
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
