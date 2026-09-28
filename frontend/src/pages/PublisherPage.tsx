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
import { api } from '../api/client';
import type { QueueJob } from '../api/types';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';

import { PageHeader } from '../components/PageHeader';

export function PublisherPage() {
  const queryClient = useQueryClient();
  const jobs = useQuery({ queryKey: ['queueJobs'], queryFn: api.queueJobs, refetchInterval: 5000 });
  const publications = useQuery({
    queryKey: ['publisherPublications'],
    queryFn: api.publisherPublications,
    refetchInterval: 5000,
  });
  const [actionReason, setActionReason] = useState('');
  const [discardTarget, setDiscardTarget] = useState<QueueJob | null>(null);
  const publishJob = useMutation({
    mutationFn: api.publishJob,
    onSuccess: async () => {
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
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
    },
  });

  const openDiscardDialog = (job: QueueJob) => {
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


  const publishableJobs = (jobs.data ?? []).filter(
    (job) => !job.publishedAt && job.status === 'completed' && (job.validationStatus === 'passed' || job.validationStatus === 'warning'),
  );
  const publicationRank: Record<string, number> = { BLOCKED: 0, FAILED: 1, HELD: 2, READY: 3, PUBLISHING: 4, PUBLISHED: 5 };
  const publicationItems = [...(publications.data?.items ?? [])].sort(
    (left, right) => (publicationRank[left.publication.state] ?? 6) - (publicationRank[right.publication.state] ?? 6),
  );

  return (
    <>
      <PageHeader title="Publisher" eyebrow="Destination release">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Review persisted publication attempts and deliver validated Results to the destination frozen with their Queue execution.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {jobs.isError || publications.isError ? <Alert severity="warning">Unable to load persisted Publisher summaries.</Alert> : null}
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

        <Card sx={{ mb: 3 }}>
          <CardContent>
            <Typography variant="h6" sx={{ mb: 1 }}>Publication attempts</Typography>
            <Table sx={{ tableLayout: 'fixed' }}>
              <TableHead>
                <TableRow>
                  <TableCell>State</TableCell>
                  <TableCell>Frozen destination</TableCell>
                  <TableCell>Delivery and archive</TableCell>
                  <TableCell>Attempts</TableCell>
                  <TableCell align="right">Audited actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {publicationItems.map(({ publication, evidence, audit }) => (
                  <TableRow key={publication.publicationId} hover>
                    <TableCell>
                      <Stack spacing={0.5}>
                        <Chip label={publication.state} size="small" color={publication.state === 'PUBLISHED' ? 'success' : publication.state === 'BLOCKED' || publication.state === 'FAILED' ? 'error' : publication.state === 'HELD' ? 'warning' : 'default'} />
                        <Typography variant="caption">Job #{publication.queueJobId} · Run {publication.validationRunId}</Typography>
                        <Typography variant="caption" color="text.secondary">{publication.failureSummary || publication.holdReason || publication.state}</Typography>
                      </Stack>
                    </TableCell>
                    <TableCell sx={{ wordBreak: 'break-all' }}>{publication.destinationPath}</TableCell>
                    <TableCell sx={{ wordBreak: 'break-word' }}>
                      <Typography variant="body2">{publication.failureSummary || publication.state}</Typography>
                      <Typography variant="caption" color="text.secondary">Archive: {publication.originalArchiveStatus}{publication.originalArchivedPath ? ` · ${publication.originalArchivedPath}` : ''}</Typography>
                      {evidence[0] ? <Typography variant="caption" display="block" color="text.secondary">{evidence[0].phase}: {evidence[0].summary || evidence[0].outcome}</Typography> : null}
                      {audit[0] ? <Typography variant="caption" display="block" color="text.secondary">Last action: {audit[0].action} · {audit[0].actorId}</Typography> : null}
                    </TableCell>
                    <TableCell>{publication.attemptCount} · fence {publication.fencingGeneration}</TableCell>
                    <TableCell align="right">
                      {publication.state === 'READY' ? (
                        <Stack direction="row" justifyContent="flex-end" spacing={1}>
                          <Button size="small" variant="contained" startIcon={<PublishIcon />} onClick={() => publishJob.mutate({ jobId: publication.queueJobId, reason: actionReason })} disabled={publishJob.isPending || !actionReason.trim()}>Publish</Button>
                          <Button size="small" onClick={() => publisherAction.mutate({ publicationId: publication.publicationId, action: 'hold', reason: actionReason })} disabled={!actionReason.trim() || publisherAction.isPending}>Hold</Button>
                        </Stack>
                      ) : publication.state === 'HELD' ? (
                        <Button size="small" onClick={() => publisherAction.mutate({ publicationId: publication.publicationId, action: 'release', reason: actionReason })} disabled={!actionReason.trim() || publisherAction.isPending}>Release</Button>
                      ) : publication.state === 'FAILED' || publication.state === 'BLOCKED' ? (
                        <Stack direction="row" justifyContent="flex-end" spacing={1}>
                          <Button size="small" onClick={() => publisherAction.mutate({ publicationId: publication.publicationId, action: 'retry', reason: actionReason })} disabled={!actionReason.trim() || publisherAction.isPending}>Retry</Button>
                          <Button size="small" onClick={() => publisherAction.mutate({ publicationId: publication.publicationId, action: 'hold', reason: actionReason })} disabled={!actionReason.trim() || publisherAction.isPending}>Hold</Button>
                        </Stack>
                      ) : null}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
            {!publications.isLoading && publicationItems.length === 0 ? <Alert severity="info" sx={{ mt: 2 }}>No persisted publication attempts yet.</Alert> : null}
          </CardContent>
        </Card>

        <Card>
          <CardContent sx={{ p: 0, '&:last-child': { pb: 0 } }}>
            <Table sx={{ tableLayout: 'fixed' }}>
              <TableHead>
                <TableRow>
                  <TableCell>Job</TableCell>
                  <TableCell>Staged Output</TableCell>
                  <TableCell>Published Path</TableCell>
                  <TableCell>Validation</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {publishableJobs.map((job) => (
                  <TableRow key={job.id} hover>
                    <TableCell>
                      <Stack spacing={0.5}>
                        <Typography fontWeight={700}>{fileNameFromPath(job.mediaPath)}</Typography>
                        <Typography color="text.secondary" variant="body2">
                          Job #{job.executionNumber ?? job.id}
                        </Typography>
                      </Stack>
                    </TableCell>
                    <TableCell sx={{ wordBreak: 'break-all' }}>{job.outputPath}</TableCell>
                    <TableCell sx={{ wordBreak: 'break-all' }}>{job.publishedPath || 'Not published yet'}</TableCell>
                    <TableCell>
                      <Chip
                        label={`${job.validationStatus} ${job.validationScore || 0}/100`}
                        color={job.validationStatus === 'passed' ? 'success' : 'warning'}
                        size="small"
                      />
                    </TableCell>
                    <TableCell align="right">
                      <Button
                        startIcon={<PublishIcon />}
                        variant="contained"
                        onClick={() => publishJob.mutate({ jobId: job.id, reason: actionReason })}
                        disabled={publishJob.isPending || Boolean(job.publishedAt) || !actionReason.trim()}
                      >
                        {job.publishedAt ? 'Published' : 'Publish'}
                      </Button>
                        <Button
                          variant="outlined"
                          color="error"
                          size="small"
                          startIcon={<DeleteOutlineIcon />}
                          disabled={publishJob.isPending || discardJob.isPending}
                          onClick={() => openDiscardDialog(job)}
                        >
                          Discard
                        </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
        {!jobs.isLoading && publishableJobs.length === 0 ? (
          <Alert severity="info" sx={{ mt: 2 }}>
            Validated jobs will appear here after they pass Validation.
          </Alert>
        ) : null}
      </Box>
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
