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
  DialogTitle,
  FormControlLabel,
  Stack,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TablePagination,
  TableRow,
  Tabs,
  TextField,
  Typography,
} from '@mui/material';
import FactCheckIcon from '@mui/icons-material/FactCheck';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import type { ValidationFinding, ValidationRunDetail, ValidationSummary } from '../api/types';

type ValidationView = 'exceptions' | 'pending' | 'all';

export function ValidationPage() {
  const queryClient = useQueryClient();
  const [detailsJob, setDetailsJob] = useState<ValidationSummary | null>(null);
  const [validationView, setValidationView] = useState<ValidationView>('exceptions');
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const jobs = useQuery({
    queryKey: ['validationReview', validationView, page, rowsPerPage],
    queryFn: () => api.validationReview({ view: validationView, limit: rowsPerPage, offset: page * rowsPerPage }),
  });
  const validateJob = useMutation({
    mutationFn: api.validateJob,
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ['validationReview'] }); },
  });
  const pagedJobs = jobs.data?.items ?? [];

  return (
    <>
      <PageHeader title="Validation" eyebrow="Post conversion quality gate">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Review exceptions from immutable Validation runs. Clean passes advance by policy and remain available in History.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {jobs.isError ? <Alert severity="warning">Unable to load completed jobs.</Alert> : null}
        {validateJob.isError ? <Alert severity="warning" sx={{ mb: 2 }}>{errorMessage(validateJob.error)}</Alert> : null}
        <Card>
          <CardContent sx={{ p: 0, '&:last-child': { pb: 0 } }}>
            <Tabs value={validationView} onChange={(_, value: ValidationView) => { setValidationView(value); setPage(0); }} sx={{ px: 2, borderBottom: 1, borderColor: 'divider' }}>
              <Tab label="Exceptions" value="exceptions" />
              <Tab label="Pending" value="pending" />
              <Tab label="All completed" value="all" />
            </Tabs>
            <Box sx={{ overflowX: 'auto' }}>
              <Table sx={{ minWidth: 1050, tableLayout: 'fixed' }}>
                <TableHead><TableRow>
                  <TableCell sx={{ width: 210 }}>Asset</TableCell>
                  <TableCell sx={{ width: 270 }}>Output</TableCell>
                  <TableCell sx={{ width: 160 }}>Validation</TableCell>
                  <TableCell sx={{ width: 115 }}>Score</TableCell>
                  <TableCell align="right" sx={{ width: 250 }}>Actions</TableCell>
                </TableRow></TableHead>
                <TableBody>
                  {pagedJobs.map((job) => (
                    <TableRow key={job.id} hover>
                      <TableCell><Stack spacing={0.5}><Typography fontWeight={700} sx={{ wordBreak: 'break-word' }}>{fileNameFromPath(job.mediaPath)}</Typography><Typography color="text.secondary" variant="body2">Job #{job.executionNumber ?? job.id}</Typography></Stack></TableCell>
                      <TableCell sx={{ wordBreak: 'break-all' }}>{job.outputPath || 'No output path recorded'}</TableCell>
                      <TableCell><ValidationChip job={job} /></TableCell>
                      <TableCell>{job.validationRunId ? `${job.validationScore || 0}/100` : '—'}</TableCell>
                      <TableCell align="right"><Stack direction="row" spacing={1} justifyContent="flex-end">
                        <Button startIcon={<InfoOutlinedIcon />} variant="outlined" onClick={() => setDetailsJob(job)} disabled={!job.validationRunId}>Details</Button>
                        <Button startIcon={<FactCheckIcon />} variant="contained" onClick={() => validateJob.mutate(job.id)} disabled={validateJob.isPending || Boolean(job.publishedAt)}>
                          {job.validationRunId ? 'Retry validation' : 'Run validation'}
                        </Button>
                      </Stack></TableCell>
                    </TableRow>
                  ))}
                  {pagedJobs.length === 0 ? <TableRow><TableCell colSpan={5}><Alert severity="info">{emptyMessage(validationView)}</Alert></TableCell></TableRow> : null}
                </TableBody>
              </Table>
            </Box>
            <TablePagination component="div" count={jobs.data?.total ?? 0} page={page} rowsPerPage={rowsPerPage} rowsPerPageOptions={[10, 25, 50, 100]} onPageChange={(_, nextPage) => setPage(nextPage)} onRowsPerPageChange={(event) => { setRowsPerPage(Number(event.target.value)); setPage(0); }} />
          </CardContent>
        </Card>
        {jobs.isError ? <Alert severity="warning" sx={{ mt: 2 }}>{errorMessage(jobs.error)}</Alert> : null}
      </Box>
      <Dialog open={Boolean(detailsJob)} onClose={() => setDetailsJob(null)} maxWidth="lg" fullWidth>
        <DialogTitle>Validation run details</DialogTitle>
        <DialogContent>{detailsJob ? <ValidationRunDetails job={detailsJob} /> : null}</DialogContent>
      </Dialog>
    </>
  );
}

function ValidationRunDetails({ job }: { job: Pick<ValidationSummary, 'id' | 'validationRunId'> }) {
  const queryClient = useQueryClient();
  const detail = useQuery({
    queryKey: ['validationRun', job.id, job.validationRunId],
    queryFn: () => api.validationRun({ jobId: job.id, runId: job.validationRunId! }),
    enabled: Boolean(job.validationRunId),
  });
  const [overrideOpen, setOverrideOpen] = useState(false);
  const [reason, setReason] = useState('');
  const [selectedRules, setSelectedRules] = useState<string[]>([]);
  const override = useMutation({
    mutationFn: () => api.createValidationOverride({ jobId: job.id, reason, ruleKeys: selectedRules, requestedHandoff: 'publisher' }),
    onSuccess: async () => {
      setOverrideOpen(false);
      setReason('');
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['validationReview'] }),
        queryClient.invalidateQueries({ queryKey: ['validationRun', job.id, job.validationRunId] }),
      ]);
    },
  });
  const run = detail.data?.run;
  const overrideable = (run?.findings ?? []).filter((finding) => finding.overrideable && finding.state !== 'PASS' && finding.state !== 'NOT_APPLICABLE');
  const openOverride = () => {
    setSelectedRules(overrideable.map((finding) => finding.key));
    setOverrideOpen(true);
  };
  return (
    <Stack spacing={2} sx={{ pt: 1 }}>
      {detail.isError ? <Alert severity="warning">{errorMessage(detail.error)}</Alert> : null}
      {detail.isLoading ? <Typography color="text.secondary">Loading immutable findings…</Typography> : null}
      {run ? <>
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
          <Chip label={run.outcome} color={outcomeColor(run.outcome)} />
          <Chip label={`Display score ${run.score}/100`} variant="outlined" />
          <Chip label={`Run ${run.runId}`} variant="outlined" />
          <Chip label={`${run.actorId} · ${new Date(run.createdAt).toLocaleString()}`} variant="outlined" />
        </Stack>
        {detail.data?.overrides.map((event) => <Alert key={event.eventId} severity="warning">Override by {event.actorId}: {event.reason} · {event.ruleKeys.join(', ')}</Alert>)}
        <Table size="small"><TableHead><TableRow><TableCell>Rule</TableCell><TableCell>Outcome</TableCell><TableCell>Expected</TableCell><TableCell>Actual</TableCell><TableCell>Finding</TableCell></TableRow></TableHead>
          <TableBody>{run.findings.map((finding) => <TableRow key={finding.key}>
            <TableCell>{finding.key}</TableCell><TableCell><Chip size="small" label={finding.state} color={finding.state === 'FAIL' ? 'error' : finding.state === 'PASS' ? 'success' : 'warning'} /></TableCell>
            <TableCell sx={{ maxWidth: 210, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{displayValue(finding.expected)}</TableCell>
            <TableCell sx={{ maxWidth: 210, whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>{displayValue(finding.actual)}</TableCell>
            <TableCell>{finding.explanation}<Typography variant="caption" display="block" color="text.secondary">{finding.evidenceReferences.join(' · ')}</Typography></TableCell>
          </TableRow>)}</TableBody>
        </Table>
        <Stack direction="row" justifyContent="flex-end"><Button variant="outlined" disabled={overrideable.length === 0} onClick={openOverride}>Record override</Button></Stack>
        <OverrideDialog open={overrideOpen} onClose={() => setOverrideOpen(false)} findings={overrideable} selected={selectedRules} setSelected={setSelectedRules} reason={reason} setReason={setReason} pending={override.isPending} error={override.isError ? errorMessage(override.error) : ''} onSave={() => override.mutate()} />
      </> : null}
    </Stack>
  );
}

function OverrideDialog({ open, onClose, findings, selected, setSelected, reason, setReason, pending, error, onSave }: {
  open: boolean; onClose: () => void; findings: ValidationFinding[]; selected: string[]; setSelected: (value: string[]) => void;
  reason: string; setReason: (value: string) => void; pending: boolean; error: string; onSave: () => void;
}) {
  return <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
    <DialogTitle>Record operator override</DialogTitle>
    <DialogContent><Stack spacing={1} sx={{ pt: 1 }}>
      <Alert severity="info">The run outcome remains unchanged. The server records the interactive actor as local_operator.</Alert>
      {findings.map((finding) => <FormControlLabel key={finding.key} control={<Checkbox checked={selected.includes(finding.key)} onChange={(event) => setSelected(event.target.checked ? [...selected, finding.key] : selected.filter((key) => key !== finding.key))} />} label={`${finding.key}: ${finding.explanation}`} />)}
      <TextField label="Reason" value={reason} onChange={(event) => setReason(event.target.value)} multiline minRows={3} inputProps={{ maxLength: 2000 }} required />
      {error ? <Alert severity="warning">{error}</Alert> : null}
    </Stack></DialogContent>
    <DialogActions><Button onClick={onClose}>Cancel</Button><Button onClick={onSave} disabled={pending || !reason.trim() || selected.length === 0}>Record override</Button></DialogActions>
  </Dialog>;
}

function ValidationChip({ job }: { job: Pick<ValidationSummary, 'validationOutcome' | 'validationRunId' | 'validationStatus'> }) {
  const outcome = job.validationOutcome || (job.validationRunId ? 'REVIEW_REQUIRED' : job.validationStatus || 'pending');
  return <Chip label={outcome} color={outcomeColor(outcome)} size="small" />;
}

function outcomeColor(outcome: string): 'success' | 'warning' | 'error' | 'default' {
  if (outcome === 'PASS' || outcome === 'passed') return 'success';
  if (outcome === 'FAIL' || outcome === 'failed') return 'error';
  if (outcome === 'PASS_WITH_WARNINGS' || outcome === 'REVIEW_REQUIRED' || outcome === 'warning') return 'warning';
  return 'default';
}

function emptyMessage(view: ValidationView) {
  if (view === 'exceptions') return 'No validation exceptions need review.';
  if (view === 'pending') return 'No completed jobs are waiting for their first Validation run.';
  return 'No unpublished completed jobs.';
}

function fileNameFromPath(path: string) { return path.split('/').filter(Boolean).pop() ?? path; }
function displayValue(value: unknown) { return value === undefined ? '—' : typeof value === 'string' ? value : JSON.stringify(value, null, 2); }
function errorMessage(error: unknown) { return error instanceof Error ? error.message : 'The request could not be completed.'; }
