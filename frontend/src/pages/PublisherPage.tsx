import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  FormControl,
  InputLabel,
  Link,
  MenuItem,
  Select,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
  TextField,
} from '@mui/material';
import PublishIcon from '@mui/icons-material/Publish';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { api } from '../api/client';
import type { PublisherEligibilityCandidate, PublisherPublicationSummary } from '../api/types';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';

import { PageHeader } from '../components/PageHeader';

type ManualPublishTarget = {
  jobId: number;
  executionNumber?: number;
  destinationPath?: string;
  status?: PublisherEligibilityCandidate['status'];
};

export function PublisherPage() {
  const queryClient = useQueryClient();
  const eligibility = useQuery({ queryKey: ['publisherEligibility'], queryFn: () => api.publisherEligibility(), refetchInterval: 5000 });
  const publications = useQuery({
    queryKey: ['publisherPublications'],
    queryFn: api.publisherPublications,
    refetchInterval: 5000,
  });
  const [actionReason, setActionReason] = useState('');
  const [stateFilter, setStateFilter] = useState('All active');
  const [search, setSearch] = useState('');
  const [publishTarget, setPublishTarget] = useState<ManualPublishTarget | null>(null);
  const [discardTarget, setDiscardTarget] = useState<PublisherEligibilityCandidate['job'] | null>(null);
  const publishJob = useMutation({
    mutationFn: api.publishJob,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['publisherEligibility'] });
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
      await queryClient.invalidateQueries({ queryKey: ['publisherPublications'] });
    },
  });

  const publisherAction = useMutation({
    mutationFn: api.publisherAction,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['publisherPublications'] });
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
      await queryClient.invalidateQueries({ queryKey: ['validationHistory'] });
      setActionReason('');
    },
  });

  const discardJob = useMutation({
    mutationFn: api.discardPublisherJob,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['publisherEligibility'] });
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
    },
  });

  const openPublishDialog = (candidate: PublisherEligibilityCandidate) => {
    publishJob.reset();
    setPublishTarget({
      jobId: candidate.job.id,
      executionNumber: candidate.job.executionNumber,
      destinationPath: candidate.job.destinationPath,
      status: candidate.status,
    });
  };

  const openExistingPublicationDialog = (publication: PublisherPublicationSummary['publication']) => {
    publishJob.reset();
    setPublishTarget({
      jobId: publication.queueJobId,
      executionNumber: publication.executionNumber,
      destinationPath: publication.destinationPath,
      status: publication.validationOverrideEventIds?.length ? 'ALLOWED_BY_OVERRIDE' : undefined,
    });
  };

  const confirmPublish = async () => {
    if (!publishTarget || !actionReason.trim()) return;
    try {
      await publishJob.mutateAsync({ jobId: publishTarget.jobId, reason: actionReason.trim() });
      setPublishTarget(null);
      setActionReason('');
    } catch {
      // Error state is already handled by the mutation alert.
    }
  };

  const closePublishDialog = () => {
    if (publishJob.isPending) return;
    setPublishTarget(null);
    setActionReason('');
  };

  const openDiscardDialog = (job: PublisherEligibilityCandidate['job']) => {
    discardJob.reset();
    setDiscardTarget(job);
  };

  const closeDiscardDialog = () => {
    if (discardJob.isPending) {
      return;
    }

    setDiscardTarget(null);
  };

  const confirmDiscard = async () => {
    if (!discardTarget) {
      return;
    }

    try {
      await discardJob.mutateAsync(discardTarget.id);
      setDiscardTarget(null);
    } catch {
      // Error state is already handled by the mutation alert.
    }
  };


  const eligibilityItems = eligibility.data?.items ?? [];
  const publicationsByExecution = new Map<string, number>();
  for (const item of publications.data?.items ?? []) {
    const key = `${item.publication.queueJobId}:${item.publication.executionNumber}`;
    publicationsByExecution.set(key, (publicationsByExecution.get(key) ?? 0) + 1);
  }
  const candidateRows = eligibilityItems.filter((candidate) =>
    !publicationsByExecution.has(`${candidate.job.id}:${candidate.job.executionNumber}`),
  );
  const searchMatches = (value: string) => value.toLowerCase().includes(search.trim().toLowerCase());
  const visibleCandidates = candidateRows.filter((candidate) => {
    const state = candidate.status === 'BLOCKED' ? 'Failed/Blocked' : 'Ready';
    return (stateFilter === 'All active' || stateFilter === state) &&
      searchMatches(`${fileNameFromPath(candidate.job.mediaPath)} ${candidate.job.id} ${candidate.job.destinationPath ?? ''}`);
  });
  const visiblePublications = [...(publications.data?.items ?? [])].filter(({ publication }) => {
    const matches = searchMatches(`${publication.queueJobId} ${publication.executionNumber} ${publication.destinationPath} ${publication.publicationId}`);
    const state = publication.state === 'PUBLISHED' ? 'Published' : publication.state === 'PUBLISHING' ? 'Publishing' : publication.state === 'HELD' ? 'Held' : publication.state === 'FAILED' || publication.state === 'BLOCKED' ? 'Failed/Blocked' : 'Ready';
    return matches && (stateFilter === 'All active' ? publication.state !== 'PUBLISHED' : stateFilter === state || (stateFilter === 'Failed/Blocked' && state === 'Failed/Blocked'));
  }).sort((left, right) => {
    const order: Record<string, number> = { BLOCKED: 0, FAILED: 1, HELD: 2, READY: 3, PUBLISHING: 4, PUBLISHED: 5 };
    return order[left.publication.state] - order[right.publication.state];
  });
  const countFor = (state: string) => candidateRows.filter((item) =>
    state === 'Failed/Blocked' ? item.status === 'BLOCKED' : state === 'Ready' && item.status !== 'BLOCKED',
  ).length + (publications.data?.items ?? []).filter((item) => {
    const value = item.publication.state;
    return state === 'Failed/Blocked' ? value === 'FAILED' || value === 'BLOCKED' : value === state.toUpperCase();
  }).length;

  return (
    <>
      <PageHeader title="Publisher" eyebrow="Destination release">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Current publication work first. Read models and evidence below are bounded windows, not global counts; History contains the complete audit view.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {eligibility.isError || publications.isError ? <Alert severity="warning">Unable to load Publisher eligibility or persisted publication summaries.</Alert> : null}
        {publishJob.isError ? <Alert severity="warning" sx={{ mb: 2 }}>Publish failed. The file may already exist or be missing.</Alert> : null}
        {publishJob.isSuccess ? <Alert severity="success" sx={{ mb: 2 }}>Job published.</Alert> : null}
        {publisherAction.isError ? <Alert severity="error" sx={{ mb: 2 }}>{publisherAction.error instanceof Error ? publisherAction.error.message : 'Publisher action failed.'}</Alert> : null}
        {discardJob.isError ? (
          <Alert severity="error" sx={{ mb: 2 }}>
            {discardJob.error instanceof Error
              ? discardJob.error.message
              : 'Unable to discard the job.'}
          </Alert>
        ) : null}

        {discardJob.isSuccess ? (
          <Alert severity="success" sx={{ mb: 2 }}>
            {discardJob.data?.originalRecovery === 'restored'
              ? 'Job discarded. The staged output was removed and the original was restored to Raw.'
              : 'Job discarded. The staged output was removed and the original remains in Raw.'}
          </Alert>
        ) : null}
        <TextField
          label="Reason for Publisher action"
          value={actionReason}
          onChange={(event) => setActionReason(event.target.value)}
          size="small"
          fullWidth
          sx={{ mb: 2 }}
        />

        <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} sx={{ mb: 2 }} alignItems={{ md: 'center' }}>
          {['Ready', 'Publishing', 'Held', 'Failed/Blocked', 'Published'].map((state) => (
            <Chip key={state} label={`${state}: ${countFor(state)}`} size="small" />
          ))}
          <TextField label="Search filename, job, or destination" value={search} onChange={(event) => setSearch(event.target.value)} size="small" />
          <FormControl size="small" sx={{ minWidth: 160 }}>
            <InputLabel id="publisher-state-filter-label">State</InputLabel>
            <Select labelId="publisher-state-filter-label" label="State" value={stateFilter} onChange={(event) => setStateFilter(event.target.value)}>
              {['All active', 'Ready', 'Publishing', 'Held', 'Failed/Blocked', 'Published'].map((state) => <MenuItem key={state} value={state}>{state}</MenuItem>)}
            </Select>
          </FormControl>
          <Link component={RouterLink} to="/history">Full History</Link>
        </Stack>

        <Card sx={{ mb: 3 }}>
          <CardContent>
            <Typography variant="h6" sx={{ mb: 1 }}>Current Publisher status</Typography>
            <Table sx={{ tableLayout: 'fixed' }}>
              <TableHead><TableRow><TableCell>Asset / Job</TableCell><TableCell>Frozen destination</TableCell><TableCell>Publication / Validation</TableCell><TableCell>Evidence</TableCell><TableCell align="right">Actions</TableCell></TableRow></TableHead>
              <TableBody>
                {visibleCandidates.map((candidate) => <TableRow key={`job-${candidate.job.id}-${candidate.job.executionNumber ?? 'unknown'}`} hover>
                  <TableCell><Typography fontWeight={700}>{fileNameFromPath(candidate.job.mediaPath)}</Typography><Typography variant="caption">Job #{candidate.job.id} · Run {candidate.job.executionNumber ?? 'unknown'}</Typography></TableCell>
                  <TableCell sx={{ wordBreak: 'break-all' }}>{candidate.job.destinationPath || 'Frozen destination unavailable'}</TableCell>
                  <TableCell><Chip size="small" label={candidate.status === 'BLOCKED' ? 'BLOCKED' : 'READY'} color={candidate.status === 'BLOCKED' ? 'error' : candidate.status === 'ALLOWED_BY_OVERRIDE' ? 'warning' : 'success'} /><Typography display="block" variant="caption">Eligibility: {candidate.status}</Typography><Typography display="block" variant="caption">{candidate.job.validationOutcome || candidate.job.validationStatus || 'Validation unavailable'}</Typography>{candidate.reason ? <Typography variant="caption" color="error.main">{candidate.reason}</Typography> : null}</TableCell>
                  <TableCell>Eligibility window</TableCell>
                  <TableCell align="right"><Button startIcon={<PublishIcon />} variant="contained" onClick={() => openPublishDialog(candidate)} disabled={publishJob.isPending || candidate.status === 'BLOCKED'}>Publish</Button><Button variant="outlined" color="error" size="small" startIcon={<DeleteOutlineIcon />} disabled={publishJob.isPending || discardJob.isPending} onClick={() => openDiscardDialog(candidate.job)}>Discard</Button></TableCell>
                </TableRow>)}
                {visiblePublications.map(({ publication, evidence, audit }) => (
                  <TableRow key={publication.publicationId} hover>
                    <TableCell><Typography fontWeight={700}>{fileNameFromPath(publication.destinationPath)}</Typography><Typography variant="caption">Job #{publication.queueJobId} · Run {publication.executionNumber}</Typography><Typography variant="caption" display="block">Publication {publication.publicationId}</Typography></TableCell>
                    <TableCell sx={{ wordBreak: 'break-all' }}>{publication.destinationPath}</TableCell>
                    <TableCell><Stack spacing={0.5}><Chip label={publication.state} size="small" color={publication.state === 'PUBLISHED' ? 'success' : publication.state === 'BLOCKED' || publication.state === 'FAILED' ? 'error' : publication.state === 'HELD' ? 'warning' : 'default'} />{publication.validationOverrideEventIds?.length ? <Chip label="ALLOWED_BY_OVERRIDE" color="warning" size="small" /> : null}<Typography variant="caption">Validation {publication.validationRunId}</Typography><Typography variant="caption">{publication.failureSummary || publication.holdReason || publication.state}</Typography></Stack></TableCell>
                    <TableCell><details><summary>Evidence · {publication.attemptCount} attempts</summary><Typography variant="caption" display="block">Archive: {publication.originalArchiveStatus}{publication.originalArchivedPath ? ` · ${publication.originalArchivedPath}` : ''}</Typography>{evidence.map((entry) => <Typography key={`${entry.attemptId}-${entry.phase}`} variant="caption" display="block">{entry.createdAt} · {entry.phase}: {entry.summary || entry.outcome}</Typography>)}{audit.map((entry) => <Typography key={`${entry.version}-${entry.action}`} variant="caption" display="block">{entry.createdAt} · {entry.action} · {entry.actorId}{entry.reason ? ` · ${entry.reason}` : ''}</Typography>)}</details></TableCell>
                    <TableCell align="right">
                      {publication.state === 'READY' ? <Stack direction="row" justifyContent="flex-end" spacing={1}><Button size="small" variant="contained" startIcon={<PublishIcon />} onClick={() => openExistingPublicationDialog(publication)} disabled={publishJob.isPending}>Publish</Button><Button size="small" onClick={() => publisherAction.mutate({ publicationId: publication.publicationId, action: 'hold', reason: actionReason })} disabled={!actionReason.trim() || publisherAction.isPending}>Hold</Button></Stack> : publication.state === 'HELD' ? <Button size="small" onClick={() => publisherAction.mutate({ publicationId: publication.publicationId, action: 'release', reason: actionReason })} disabled={!actionReason.trim() || publisherAction.isPending}>Release</Button> : publication.state === 'FAILED' || publication.state === 'BLOCKED' ? <Stack direction="row" justifyContent="flex-end" spacing={1}><Button size="small" onClick={() => publisherAction.mutate({ publicationId: publication.publicationId, action: 'retry', reason: actionReason })} disabled={!actionReason.trim() || publisherAction.isPending}>Retry</Button><Button size="small" onClick={() => publisherAction.mutate({ publicationId: publication.publicationId, action: 'hold', reason: actionReason })} disabled={!actionReason.trim() || publisherAction.isPending}>Hold</Button></Stack> : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {!eligibility.isLoading && !publications.isLoading && visibleCandidates.length + visiblePublications.length === 0 ? <Alert severity="info" sx={{ mt: 2 }}>No Publisher records match this filter in the bounded read-model windows.</Alert> : null}
            <Typography variant="caption" color="text.secondary" display="block" sx={{ mt: 1 }}>Eligibility is limited to 100 jobs; publication summaries and evidence are also bounded. Use <Link component={RouterLink} to="/history">History</Link> for complete audit history.</Typography>
          </CardContent>
        </Card>
      </Box>
      <Dialog open={publishTarget !== null} onClose={closePublishDialog} maxWidth="sm" fullWidth>
        <DialogTitle>Confirm manual publication</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Publisher will recheck the current Validation, Result identity, required artifacts, and frozen delivery destination before delivery.
          </DialogContentText>
          {publishTarget?.status === 'ALLOWED_BY_OVERRIDE' ? (
            <Alert severity="warning" sx={{ mt: 2 }}>
              This Result is eligible through Publisher-scoped Validation overrides. Confirm that you reviewed the findings; publication will preserve the ValidationRun and override history.
            </Alert>
          ) : null}
          {publishTarget ? (
            <DialogContentText sx={{ mt: 2 }}>
              Job #{publishTarget.executionNumber ?? publishTarget.jobId} · {publishTarget.destinationPath || 'Frozen destination unavailable'}
            </DialogContentText>
          ) : null}
          <TextField
            label="Operator reason"
            value={actionReason}
            onChange={(event) => setActionReason(event.target.value)}
            required
            fullWidth
            multiline
            minRows={2}
            sx={{ mt: 2 }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={closePublishDialog} disabled={publishJob.isPending}>Cancel</Button>
          <Button
            variant="contained"
            startIcon={<PublishIcon />}
            onClick={confirmPublish}
            disabled={publishJob.isPending || !actionReason.trim()}
          >
            {publishJob.isPending ? 'Publishing…' : 'Confirm publish'}
          </Button>
        </DialogActions>
      </Dialog>
      <Dialog
        open={discardTarget !== null}
        onClose={closeDiscardDialog}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Discard job?</DialogTitle>

        <DialogContent>
          <DialogContentText>
            The converted file in staging will be permanently removed.
            The original file in Raw will be preserved. If the original is
            missing from Raw and an archived copy is available, MVForge will
            restore it before removing staging.
          </DialogContentText>

          {discardTarget ? (
            <DialogContentText sx={{ mt: 2 }}>
              Job #{discardTarget.id}
            </DialogContentText>
          ) : null}
        </DialogContent>

        <DialogActions>
          <Button
            onClick={closeDiscardDialog}
            disabled={discardJob.isPending}
          >
            Cancel
          </Button>

          <Button
            color="error"
            variant="contained"
            onClick={confirmDiscard}
            disabled={discardJob.isPending}
          >
            {discardJob.isPending ? 'Discarding…' : 'Discard'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}

function fileNameFromPath(path: string) {
  return path.split('/').filter(Boolean).pop() ?? path;
}
