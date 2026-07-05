import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogContent,
  DialogTitle,
  LinearProgress,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import FactCheckIcon from '@mui/icons-material/FactCheck';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import type { QueueJob } from '../api/types';

export function ValidationPage() {
  const queryClient = useQueryClient();
  const jobs = useQuery({ queryKey: ['queueJobs'], queryFn: api.queueJobs });
  const [detailsJob, setDetailsJob] = useState<QueueJob | null>(null);
  const validateJob = useMutation({
    mutationFn: api.validateJob,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
    },
  });
  const completedJobs = (jobs.data ?? []).filter((job) => job.status === 'completed');

  return (
    <>
      <PageHeader title="Validation" eyebrow="Safety checks">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Validate completed job outputs before publishing them to destination libraries.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {jobs.isError ? <Alert severity="warning">Unable to load completed jobs.</Alert> : null}
        {validateJob.isError ? <Alert severity="warning" sx={{ mb: 2 }}>Validation could not be completed.</Alert> : null}

        <Card>
          <CardContent sx={{ p: 0, '&:last-child': { pb: 0 } }}>
            <Table sx={{ tableLayout: 'fixed' }}>
              <TableHead>
                <TableRow>
                  <TableCell>Job</TableCell>
                  <TableCell>Output</TableCell>
                  <TableCell>Validation</TableCell>
                  <TableCell>Score</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {completedJobs.map((job) => (
                  <TableRow key={job.id} hover>
                    <TableCell>
                      <Stack spacing={0.5}>
                        <Typography fontWeight={700}>{fileNameFromPath(job.mediaPath)}</Typography>
                        <Typography color="text.secondary" variant="body2">
                          Job #{job.id}
                        </Typography>
                      </Stack>
                    </TableCell>
                    <TableCell sx={{ wordBreak: 'break-all' }}>{job.outputPath || 'No output path recorded'}</TableCell>
                    <TableCell>
                      <ValidationChip job={job} />
                    </TableCell>
                    <TableCell>
                      <Stack spacing={0.6}>
                        <Typography>{job.validationScore || 0}/100</Typography>
                        <LinearProgress variant="determinate" value={job.validationScore || 0} />
                      </Stack>
                    </TableCell>
                  <TableCell align="right">
                      <Stack direction="row" spacing={1} justifyContent="flex-end">
                        <Button
                          startIcon={<InfoOutlinedIcon />}
                          variant="outlined"
                          onClick={() => setDetailsJob(job)}
                        >
                          Score
                        </Button>
                        <Button
                          startIcon={<FactCheckIcon />}
                          variant="contained"
                          onClick={() => validateJob.mutate(job.id)}
                          disabled={validateJob.isPending}
                        >
                          Run Checks
                        </Button>
                      </Stack>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
        {!jobs.isLoading && completedJobs.length === 0 ? (
          <Alert severity="info" sx={{ mt: 2 }}>
            Completed jobs will appear here before publishing.
          </Alert>
        ) : null}
      </Box>
      <Dialog open={Boolean(detailsJob)} onClose={() => setDetailsJob(null)} maxWidth="md" fullWidth>
        <DialogTitle>Validation Score</DialogTitle>
        <DialogContent>
          {detailsJob ? <ValidationScoreDetails job={detailsJob} /> : null}
        </DialogContent>
      </Dialog>
    </>
  );
}

function ValidationChip({ job }: { job: QueueJob }) {
  const status = job.validationStatus || 'pending';
  const color = status === 'passed' ? 'success' : status === 'failed' ? 'error' : status === 'warning' ? 'warning' : 'default';
  return <Chip label={status} color={color} size="small" />;
}

function fileNameFromPath(path: string) {
  return path.split('/').filter(Boolean).pop() ?? path;
}

function ValidationScoreDetails({ job }: { job: QueueJob }) {
  const checks = reportChecks(job.validationReport);
  return (
    <Stack spacing={2}>
      <Alert severity="info">
        Run Checks validates the completed job output before Publisher is allowed to release it. The score starts at 100
        and loses points when required safety checks fail.
      </Alert>
      <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
        <ValidationChip job={job} />
        <Chip label={`${job.validationScore || 0}/100`} size="small" />
      </Stack>
      <Table size="small">
        <TableHead>
          <TableRow>
            <TableCell>Check</TableCell>
            <TableCell>Status</TableCell>
            <TableCell>What it means</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {checks.map((check) => (
            <TableRow key={check.key}>
              <TableCell>{check.label}</TableCell>
              <TableCell>
                <Chip label={check.status} color={check.status === 'passed' ? 'success' : 'error'} size="small" />
              </TableCell>
              <TableCell>{check.message}</TableCell>
            </TableRow>
          ))}
          {checks.length === 0 ? (
            <TableRow>
              <TableCell colSpan={3}>
                <Typography color="text.secondary">
                  No validation report yet. Use Run Checks to generate the first score.
                </Typography>
              </TableCell>
            </TableRow>
          ) : null}
        </TableBody>
      </Table>
      <Alert severity="warning">
        Current basic checks verify completion, output path, file existence, and non-empty output. Deeper media checks will
        be added when real conversion writes staging files.
      </Alert>
    </Stack>
  );
}

function reportChecks(report: Record<string, unknown> | null | undefined) {
  const checks = report?.checks;
  if (!Array.isArray(checks)) {
    return [];
  }

  return checks
    .map((check) => {
      if (!check || typeof check !== 'object') {
        return null;
      }
      const value = check as Record<string, unknown>;
      return {
        key: typeof value.key === 'string' ? value.key : '',
        label: typeof value.label === 'string' ? value.label : 'Check',
        status: value.status === 'passed' ? 'passed' : 'failed',
        message: typeof value.message === 'string' ? value.message : '',
      };
    })
    .filter((check): check is { key: string; label: string; status: 'passed' | 'failed'; message: string } =>
      Boolean(check?.key),
    );
}
