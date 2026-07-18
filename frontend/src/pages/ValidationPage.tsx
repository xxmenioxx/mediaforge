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
  Tab,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TablePagination,
  TableRow,
  Tabs,
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
  const [validationView, setValidationView] = useState<'pending' | 'validated'>('pending');
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const validateJob = useMutation({
    mutationFn: api.validateJob,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
    },
  });
  const completedJobs = (jobs.data ?? [])
    .filter((job) => job.status === 'completed' && !job.publishedAt)
    .sort((left, right) => validationEntryTime(right).localeCompare(validationEntryTime(left)));
  const pendingJobs = completedJobs.filter((job) => !job.validationStatus || job.validationStatus === 'pending');
  const validatedJobs = completedJobs.filter((job) => Boolean(job.validationStatus && job.validationStatus !== 'pending'));
  const visibleJobs = validationView === 'pending' ? pendingJobs : validatedJobs;
  const pagedJobs = visibleJobs.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);

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
            <Tabs
              value={validationView}
              onChange={(_, value) => {
                setValidationView(value);
                setPage(0);
              }}
              sx={{ px: 2, borderBottom: 1, borderColor: 'divider' }}
            >
              <Tab label={`Pending validation (${pendingJobs.length})`} value="pending" />
              <Tab label={`Validated (${validatedJobs.length})`} value="validated" />
            </Tabs>
            <Box sx={{ overflowX: 'auto' }}>
              <Table sx={{ minWidth: 1120, tableLayout: 'fixed' }}>
                <TableHead>
                  <TableRow>
                    <TableCell sx={{ width: 190 }}>Asset</TableCell>
                    <TableCell sx={{ width: 250 }}>Output</TableCell>
                    <TableCell sx={{ width: 155 }}>Entered validation</TableCell>
                    <TableCell sx={{ width: 155 }}>Validated at</TableCell>
                    <TableCell sx={{ width: 120 }}>Validation</TableCell>
                    <TableCell sx={{ width: 120 }}>Score</TableCell>
                    <TableCell align="right" sx={{ width: 230 }}>Actions</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {pagedJobs.map((job) => (
                    <TableRow key={job.id} hover>
                      <TableCell>
                        <Stack spacing={0.5}>
                          <Typography fontWeight={700} sx={{ wordBreak: 'break-word' }}>{fileNameFromPath(job.mediaPath)}</Typography>
                          <Typography color="text.secondary" variant="body2">
                            Job #{job.id}
                          </Typography>
                        </Stack>
                      </TableCell>
                      <TableCell sx={{ wordBreak: 'break-all' }}>{job.outputPath || 'No output path recorded'}</TableCell>
                      <TableCell>{formatValidationDate(validationEntryTime(job))}</TableCell>
                      <TableCell>{formatValidationDate(validationDate(job))}</TableCell>
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
                          <Button startIcon={<InfoOutlinedIcon />} variant="outlined" onClick={() => setDetailsJob(job)}>
                            Score
                          </Button>
                          <Button
                            startIcon={<FactCheckIcon />}
                            variant="contained"
                            onClick={() => validateJob.mutate(job.id)}
                            disabled={validateJob.isPending || Boolean(job.publishedAt)}
                          >
                            {job.publishedAt ? 'Published' : job.validationStatus && job.validationStatus !== 'pending' ? 'Run Again' : 'Run Checks'}
                          </Button>
                        </Stack>
                      </TableCell>
                    </TableRow>
                  ))}
                  {pagedJobs.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={7}>
                        <Alert severity="info">
                          {validationView === 'pending' ? 'No assets are waiting for validation.' : 'No assets have been validated yet.'}
                        </Alert>
                      </TableCell>
                    </TableRow>
                  ) : null}
                </TableBody>
              </Table>
            </Box>
            <TablePagination
              component="div"
              count={visibleJobs.length}
              page={Math.min(page, Math.max(0, Math.ceil(visibleJobs.length / rowsPerPage) - 1))}
              rowsPerPage={rowsPerPage}
              rowsPerPageOptions={[10, 25, 50, 100]}
              onPageChange={(_, nextPage) => setPage(nextPage)}
              onRowsPerPageChange={(event) => {
                setRowsPerPage(Number(event.target.value));
                setPage(0);
              }}
            />
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

function validationEntryTime(job: QueueJob) {
  return job.finishedAt || job.createdAt;
}

function validationDate(job: QueueJob) {
  const value = job.validationReport?.validatedAt;
  return typeof value === 'string' ? value : '';
}

function formatValidationDate(value: string) {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) {
    return 'Not yet';
  }
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function ValidationScoreDetails({ job }: { job: QueueJob }) {
  const checks = reportChecks(job.validationReport);
  const directPlay = reportObject(job.validationReport, 'directPlay');
  const directPlayClients = reportArray(directPlay, 'clients');
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
      {directPlay ? <>
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
          <Typography variant="h4">Final DirectPlay report</Typography>
          <Chip label={`Risk: ${reportString(directPlay, 'risk') || 'unknown'}`} color={reportString(directPlay, 'risk') === 'low' ? 'success' : reportString(directPlay, 'risk') === 'high' ? 'error' : 'warning'} size="small" />
          <Chip label={`Lowest score: ${reportNumber(directPlay, 'lowestScore')}/100`} size="small" />
          {directPlay.blocked === true ? <Chip label="Publishing blocked" color="error" size="small" /> : null}
        </Stack>
        <Table size="small"><TableHead><TableRow><TableCell>Client</TableCell><TableCell>Score</TableCell><TableCell>Risk</TableCell><TableCell>Findings</TableCell></TableRow></TableHead><TableBody>
          {directPlayClients.map((client, index) => <TableRow key={`${reportString(client, 'client')}-${index}`}><TableCell>{reportString(client, 'client').replaceAll('_', ' ')}</TableCell><TableCell>{reportNumber(client, 'score')}/100</TableCell><TableCell>{reportString(client, 'risk')}</TableCell><TableCell>{reportStringArray(client, 'warnings').join(' · ') || 'No compatibility warnings'}</TableCell></TableRow>)}
        </TableBody></Table>
      </> : null}
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
        Safety checks validate the output file. DirectPlay scores inspect its real container and streams with ffprobe; actual device behavior can still vary by player version and hardware.
      </Alert>
    </Stack>
  );
}

function reportObject(value: Record<string, unknown> | null | undefined, key: string) {
  const nested = value?.[key];
  return nested && typeof nested === 'object' && !Array.isArray(nested) ? nested as Record<string, unknown> : undefined;
}

function reportArray(value: Record<string, unknown> | undefined, key: string) {
  const list = value?.[key];
  return Array.isArray(list) ? list.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === 'object' && !Array.isArray(item)) : [];
}

function reportString(value: Record<string, unknown>, key: string) { return typeof value[key] === 'string' ? value[key] as string : ''; }
function reportNumber(value: Record<string, unknown>, key: string) { return typeof value[key] === 'number' ? value[key] as number : 0; }
function reportStringArray(value: Record<string, unknown>, key: string) { return Array.isArray(value[key]) ? (value[key] as unknown[]).filter((item): item is string => typeof item === 'string') : []; }

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
