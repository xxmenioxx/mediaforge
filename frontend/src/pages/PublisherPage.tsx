import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  FormControlLabel,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
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
  const [overwrite, setOverwrite] = useState(false);
  const [discardTarget, setDiscardTarget] = useState<QueueJob | null>(null);
  const publishJob = useMutation({
    mutationFn: api.publishJob,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
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

  return (
    <>
      <PageHeader title="Publisher" eyebrow="Destination release">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Publish validated outputs into destination libraries. Overwrites are disabled unless explicitly enabled.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {jobs.isError ? <Alert severity="warning">Unable to load validated jobs.</Alert> : null}
        {publishJob.isError ? <Alert severity="warning" sx={{ mb: 2 }}>Publish failed. The file may already exist or be missing.</Alert> : null}
        {publishJob.isSuccess ? <Alert severity="success" sx={{ mb: 2 }}>Job published.</Alert> : null}
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
        <Stack direction="row" justifyContent="flex-end" sx={{ mb: 2 }}>
          <FormControlLabel
            control={<Checkbox checked={overwrite} onChange={(event) => setOverwrite(event.target.checked)} />}
            label="Allow overwrite"
          />
        </Stack>

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
                        onClick={() => publishJob.mutate({ jobId: job.id, overwrite })}
                        disabled={publishJob.isPending || Boolean(job.publishedAt)}
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

