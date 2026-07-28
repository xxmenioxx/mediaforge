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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import { Link as RouterLink, useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import { JobDetailsDialog } from '../components/JobDetailsDialog';
import { PageHeader } from '../components/PageHeader';
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
    refetchInterval: (query) => (query.state.data?.some((job) => job.status === 'running') ? 2000 : false),
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
  const [jobSortDirection, setJobSortDirection] = useState<'asc' | 'desc'>('desc');
  const [jobsPerPage, setJobsPerPage] = useState(10);
  const [jobPage, setJobPage] = useState(1);
  const statusFilter = normalizeStatusFilter(searchParams.get('status'));
  const batchFilter = searchParams.get('batch') ?? '';
  const pathFilter = searchParams.get('path') ?? '';
  const selectedJobId = Number(searchParams.get('job'));
  const jobHistory = useMemo(() => jobs.data ?? [], [jobs.data]);
  const allJobs = useMemo(() => latestJobsByAsset(jobHistory), [jobHistory]);
  const filteredJobs = useMemo(
    () =>
      allJobs.filter((job) => {
        const matchesStatus = statusFilter === 'all' || job.status === statusFilter;
        const matchesBatch = !batchFilter || job.batchId === batchFilter;
        const matchesPath = !pathFilter || normalizePath(queueGroupPathFromMediaPath(job.mediaPath)) === normalizePath(pathFilter);
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
        return matchesStatus && matchesBatch && matchesPath && matchesSearch;
      }),
    [allJobs, batchFilter, jobSearch, pathFilter, statusFilter],
  );
  const jobGroups = useMemo(() => buildJobGroups(filteredJobs, jobSortDirection), [filteredJobs, jobSortDirection]);
  const totalJobPages = Math.max(1, Math.ceil(jobGroups.length / jobsPerPage));
  const pagedJobGroups = jobGroups.slice((jobPage - 1) * jobsPerPage, jobPage * jobsPerPage);
  const statusCounts = useMemo(() => summarizeStatusCounts(allJobs), [allJobs]);
  const selectedJob = selectedJobId ? jobHistory.find((job) => job.id === selectedJobId) ?? null : null;

  useEffect(() => {
    setJobPage(1);
  }, [batchFilter, jobSearch, jobsPerPage, jobSortDirection, pathFilter, statusFilter]);

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
      processingMode: input.processingMode,
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
      profileId: (job.processingMode || conversion?.processingMode) === 'audio_only' ? 0 : job.profileId,
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
      <PageHeader title="Queue" eyebrow="Manual approval">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 760 }}>
          Jobs appear here only after the user chooses media, a destination library, and a conversion profile.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {jobs.isError ? <Alert severity="warning">Unable to load queue.</Alert> : null}
        <Alert severity={workerSettings.autoWorkerEnabled ? 'success' : 'warning'} sx={{ mb: 2 }}>
          Auto worker is {workerSettings.autoWorkerEnabled ? 'enabled' : 'disabled'} in Settings.{' '}
          <Button component={RouterLink} to="/settings" size="small" color="inherit">
            Open Settings
          </Button>
        </Alert>
        <Stack direction="row" spacing={1} flexWrap="wrap" justifyContent="space-between" useFlexGap sx={{ mb: 2 }}>
          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
            {(['queued', 'running', 'completed', 'failed', 'canceled', 'all'] as const).map((status) => (
              <Button
                key={status}
                variant={statusFilter === status ? 'contained' : 'outlined'}
                color={status === 'failed' ? 'error' : 'primary'}
                size="small"
                onClick={() => {
                  const nextParams = new URLSearchParams();
                  nextParams.set('status', status);
                  if (batchFilter) {
                    nextParams.set('batch', batchFilter);
                  }
                  setSearchParams(nextParams);
                }}
              >
                {statusLabel(status)} {statusCounts[status]}
              </Button>
            ))}
            {batchFilter || pathFilter ? (
              <Button color="warning" size="small" variant="outlined" onClick={() => setSearchParams(statusFilter === 'all' ? {} : { status: statusFilter })}>
                Clear focus
              </Button>
            ) : null}
          </Stack>
          <Button startIcon={<PlaylistAddIcon />} variant="contained" onClick={openCreateJobDialog}>
            Queue Job
          </Button>
        </Stack>
        <Card sx={{ mb: 2 }}>
          <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
            <Grid container spacing={1.5} alignItems="center">
              <Grid size={{ xs: 12, md: 6 }}>
                <TextField
                  label="Search jobs"
                  value={jobSearch}
                  onChange={(event) => setJobSearch(event.target.value)}
                  placeholder="Job number, asset, status, worker, notes..."
                  fullWidth
                />
              </Grid>
              <Grid size={{ xs: 6, md: 2 }}>
                <TextField
                  label="Sort"
                  value={jobSortDirection}
                  onChange={(event) => setJobSortDirection(event.target.value as 'asc' | 'desc')}
                  select
                  fullWidth
                >
                  <MenuItem value="desc">Newest first</MenuItem>
                  <MenuItem value="asc">Oldest first</MenuItem>
                </TextField>
              </Grid>
              <Grid size={{ xs: 6, md: 2 }}>
                <TextField
                  label="Groups per page"
                  value={jobsPerPage}
                  onChange={(event) => setJobsPerPage(Number(event.target.value))}
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
              <Grid size={{ xs: 12, md: 2 }}>
                <Stack direction="row" spacing={1} justifyContent={{ xs: 'flex-start', md: 'flex-end' }}>
                  <Button variant="outlined" disabled={jobPage <= 1} onClick={() => setJobPage((current) => Math.max(1, current - 1))}>
                    Prev
                  </Button>
                  <Button variant="outlined" disabled={jobPage >= totalJobPages} onClick={() => setJobPage((current) => Math.min(totalJobPages, current + 1))}>
                    Next
                  </Button>
                </Stack>
                <Typography color="text.secondary" variant="body2" align="right" sx={{ mt: 0.5 }}>
                  Page {jobPage} / {totalJobPages}
                </Typography>
              </Grid>
            </Grid>
          </CardContent>
        </Card>
        <Dialog open={isJobDialogOpen} onClose={closeJobDialog} maxWidth="md" fullWidth>
          <DialogTitle>{editingJob ? (editingJob.executionNumber ? `Edit Job #${editingJob.executionNumber}` : 'Edit Pending Item') : 'Queue Job'}</DialogTitle>
          <Box component="form" onSubmit={submit}>
            <DialogContent>
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
            <DialogActions>
              <Button onClick={closeJobDialog}>Cancel</Button>
              <Button type="submit" variant="contained" disabled={createJob.isPending || editJob.isPending || !form.mediaPath || !form.libraryId || (!form.profileId && !form.audioProfileKey && !selectedTrackProfileKey)}>
                {editingJob ? 'Save' : 'Queue'}
              </Button>
            </DialogActions>
          </Box>
        </Dialog>
        <Stack spacing={2}>
          {pagedJobGroups.map((group) => (
            <QueueGroupCard
              key={group.id}
              group={group}
              onEditJob={openEditJobDialog}
              initiallyExpanded={Boolean(group.running || (pathFilter && normalizePath(group.id) === normalizePath(pathFilter)))}
            />
          ))}
        </Stack>
        {!jobs.isLoading && jobs.data?.length === 0 ? (
          <Alert severity="info">No conversion jobs have been queued yet.</Alert>
        ) : null}
        {!jobs.isLoading && allJobs.length > 0 && filteredJobs.length === 0 ? (
          <Alert severity="info" sx={{ mt: 2 }}>
            No jobs match the current filter.
          </Alert>
        ) : null}
      </Box>
      <JobDetailsDialog
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

type QueueJobGroup = {
  id: string;
  name: string;
  jobs: QueueJob[];
  isBatch: boolean;
  batchId: string;
  progress: number;
  completed: number;
  failed: number;
  running: number;
  queued: number;
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
  return [inventory.unprocessed, inventory.library, inventory.unverified, inventory.converted, inventory.archive]
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

function QueueGroupCard({
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
    mutationFn: async (action: 'cancel' | 'remove') => {
      const jobs = group.jobs.filter((job) => job.status !== 'completed');
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

  function requeueFailedJobs() {
    group.jobs
      .filter((job) => job.status === 'failed')
      .forEach((job) => updateJob.mutate({ jobId: job.id, status: 'queued' }));
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
    const removable = group.jobs.filter((job) => job.status !== 'completed');
    const count = removable.length;
    const onlyPlaceholders = removable.every((job) => !job.executionNumber && !job.startedAt);
    if (!count) return;
    if (!onlyPlaceholders && !window.confirm(`Remove ${count} non-completed job${count === 1 ? '' : 's'} from “${group.name}”? Running jobs will be canceled first. Logs and reports will be preserved.`)) return;
    batchAction.mutate('remove');
  }

  useEffect(() => {
    if (initiallyExpanded) {
      setExpanded(true);
    }
  }, [initiallyExpanded]);

  useEffect(() => {
    if (!expanded) {
      setVisibleJobsCount(25);
    }
  }, [expanded]);

  const visibleJobs = group.jobs.slice(0, visibleJobsCount);

  return (
    <Card sx={{ overflow: 'hidden' }}>
      <CardContent sx={{ p: 1.5, '&:last-child': { pb: 1.5 } }}>
        <Stack spacing={1}>
          {dismissJob.isError ? <Alert severity="warning">Could not remove this job from Queue.</Alert> : null}
          {batchAction.isError ? <Alert severity="warning">Could not complete the batch action. Refresh Queue to review the remaining jobs.</Alert> : null}
          <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1.5}>
            <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap sx={{ minWidth: 0 }}>
              <Typography variant="h3" sx={{ wordBreak: 'break-word' }}>
                {group.name}
              </Typography>
              <Chip label={group.isBatch ? 'Folder batch' : group.jobs.length > 1 ? 'Path group' : 'Single job'} color={group.isBatch ? 'primary' : 'default'} size="small" />
              <Chip label={`${group.completed}/${group.jobs.length} completed`} color="success" size="small" />
              <Chip label={`${group.queued} queued`} color="warning" size="small" />
              <Chip label={`${group.running} running`} color="primary" size="small" />
              <Chip label={`${group.progress}%`} color="primary" size="small" variant="outlined" />
              {group.failed ? <Chip label={`${group.failed} failed`} color="error" size="small" /> : null}
            </Stack>
            <Stack direction="row" spacing={1} alignItems="center" sx={{ flexShrink: 0 }}>
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
              {group.isBatch && group.jobs.some((job) => job.status !== 'completed') ? (
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
              {group.failed ? (
                <Button
                  startIcon={<ReplayIcon />}
                  color="error"
                  variant="outlined"
                  size="small"
                  disabled={updateJob.isPending}
                  onClick={requeueFailedJobs}
                >
                  Retry Failed
                </Button>
              ) : null}
              <IconButton
                color="primary"
                onClick={() => setExpanded((current) => !current)}
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
          <LinearProgress variant="determinate" value={group.progress} sx={{ height: 5, borderRadius: 1 }} />
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
  const stage = pipelineStage(job);
  const timing = jobTiming(job);

  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.1, bgcolor: 'rgba(255,255,255,0.018)' }}>
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
          <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
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
          <Chip label={job.status} color={statusColor(job.status)} size="small" />
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
    const groupId = queueGroupPathFromMediaPath(job.mediaPath);
    const existing = groups.get(groupId);
    const group =
      existing ??
      ({
        id: groupId,
        name: job.batchName || groupId,
        jobs: [],
        isBatch: Boolean(job.batchId) || false,
        batchId: job.batchId || '',
        progress: 0,
        completed: 0,
        failed: 0,
        running: 0,
        queued: 0,
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
    const progress =
      group.jobs.length > 0
        ? Math.round(group.jobs.reduce((total, job) => total + job.progress, 0) / group.jobs.length)
        : 0;

    return {
      ...group,
      completed,
      failed,
      running,
      queued,
      progress,
    };
  }).sort((left, right) => {
    if (left.running !== right.running) {
      return right.running - left.running;
    }
    const leftID = Math.max(...left.jobs.map((job) => job.id));
    const rightID = Math.max(...right.jobs.map((job) => job.id));
    return sortDirection === 'desc' ? rightID - leftID : leftID - rightID;
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
  const normalized = normalizePath(mediaPath);
  const rawMarker = '/media/raw/';
  const rawIndex = normalized.indexOf(rawMarker);
  if (rawIndex >= 0) {
    const root = normalized.slice(0, rawIndex + rawMarker.length - 1);
    const relativeParts = normalized
      .slice(rawIndex + rawMarker.length)
      .split('/')
      .filter(Boolean);
    if (relativeParts.length <= 1) {
      return root;
    }
    if (relativeParts.length === 2) {
      return `${root}/${relativeParts[0]}`;
    }
    return `${root}/${relativeParts[0]}/${relativeParts[1]}`;
  }
  return groupPathFromMediaPath(normalized);
}

function normalizePath(value: string) {
  return value.replace(/\\/g, '/').replace(/\/+$/, '');
}

function pipelineStage(job: QueueJob): { label: string; color: 'default' | 'primary' | 'success' | 'warning' | 'error' } {
  if (job.stage) {
    const color = job.stage === 'completed' ? 'success' : job.stage === 'failed' ? 'error' : job.stage === 'canceled' ? 'default' : job.stage === 'queued' ? 'warning' : 'primary';
    return { label: job.stage.replaceAll('_', ' '), color };
  }
  if (job.publishedAt) {
    return { label: 'Publisher: published', color: 'success' };
  }
  if (job.validationStatus === 'passed' || job.validationStatus === 'warning') {
    return { label: 'Publisher: ready', color: 'primary' };
  }
  if (job.status === 'completed') {
    return { label: 'Analysis/Validation: waiting', color: 'primary' };
  }
  if (job.status === 'running') {
    return { label: 'Workers: running', color: 'primary' };
  }
  if (job.status === 'queued') {
    return { label: 'Queue: waiting', color: 'warning' };
  }
  if (job.status === 'failed') {
    return { label: 'Review: failed', color: 'error' };
  }
  if (job.status === 'canceled') {
    return { label: 'Queue: canceled', color: 'default' };
  }
  return { label: 'Assets: queued', color: 'default' };
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

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value));
}

function statusColor(status: string) {
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
    : 'queued';
}

function statusLabel(status: QueueJob['status'] | 'all') {
  return status === 'all' ? 'All' : status[0].toUpperCase() + status.slice(1);
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
