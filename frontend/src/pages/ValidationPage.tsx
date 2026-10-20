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
  DialogTitle,
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
import { Link as RouterLink } from 'react-router-dom';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import { ValidationFindingsTable } from '../components/validation/ValidationFindingsTable';
import { ValidationRunSummary } from '../components/validation/ValidationRunSummary';
import { ValidationTechnicalEvidence } from '../components/validation/ValidationTechnicalEvidence';
import type { ValidationFinding, ValidationOverride, ValidationSummary, ValidationRun } from '../api/types';
import { canDismissValidationJob } from '../utils/validationDismissal';

type ValidationView = 'exceptions' | 'pending' | 'all';

export function ValidationPage() {
  const queryClient = useQueryClient();
  const [detailsJob, setDetailsJob] = useState<ValidationSummary | null>(null);
  const [validationView, setValidationView] = useState<ValidationView>('exceptions');
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const validationSettings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const jobs = useQuery({
    queryKey: ['validationReview', validationView, page, rowsPerPage],
    queryFn: () => api.validationReview({ view: validationView, limit: rowsPerPage, offset: page * rowsPerPage }),
  });
  const validateJob = useMutation({
    mutationFn: api.validateJob,
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ['validationReview'] }); },
  });
  const dismissJob = useMutation({
    mutationFn: api.dismissQueueJob,
    onSuccess: async () => Promise.all([
      queryClient.invalidateQueries({ queryKey: ['validationReview'] }),
      queryClient.invalidateQueries({ queryKey: ['validationHistory'] }),
      queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
      queryClient.invalidateQueries({ queryKey: ['assets'] }),
      queryClient.invalidateQueries({ queryKey: ['queueBatchPreflight'] }),
      queryClient.invalidateQueries({ queryKey: ['publisherEligibility'] }),
    ]),
  });
  const pagedJobs = jobs.data?.items ?? [];

  function dismissValidationJob(job: ValidationSummary) {
    if (!window.confirm(`Dismiss Job #${job.executionNumber ?? job.id} from active Validation and Queue? This allows a fresh Queue operation. The Queue history, Result, Validation findings, logs, reports, staged output, sidecars, and source media will be preserved.`)) return;
    dismissJob.mutate(job.id);
  }

  return (
    <>
      <PageHeader title="Validation" eyebrow="Post conversion quality gate">
        <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} spacing={1.5} sx={{ mt: 1 }}>
          <Typography color="text.secondary" sx={{ maxWidth: 820 }}>
            Review exceptions from immutable Validation runs. Clean passes advance by policy and remain available in History.
          </Typography>
          <Typography variant="body2" fontWeight={700}>
            Validation Policy: {policySummary(validationSettings.data?.find((setting) => setting.key === 'validation')?.value)}
          </Typography>
          <Button component={RouterLink} to="/settings?section=pipeline#validation" variant="outlined">Validation Settings</Button>
        </Stack>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {jobs.isError ? <Alert severity="warning">Unable to load completed jobs.</Alert> : null}
        {validateJob.isError ? <Alert severity="warning" sx={{ mb: 2 }}>{errorMessage(validateJob.error)}</Alert> : null}
        {dismissJob.isError ? <Alert severity="warning" sx={{ mb: 2 }}>{errorMessage(dismissJob.error)}</Alert> : null}
        {dismissJob.isSuccess ? <Alert severity="success" sx={{ mb: 2 }}>Dismissed from active Validation and Queue. The job remains in History and its files and evidence are preserved.</Alert> : null}
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
                        {canDismissValidationJob(job) ? <Button variant="outlined" color="error" onClick={() => dismissValidationJob(job)} disabled={dismissJob.isPending || validateJob.isPending}>Dismiss</Button> : null}
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

function ValidationRunDetails({ job }: { job: ValidationSummary }) {
  const queryClient = useQueryClient();
  const detail = useQuery({
    queryKey: ['validationRun', job.id, job.validationRunId],
    queryFn: () => api.validationRun({ jobId: job.id, runId: job.validationRunId! }),
    enabled: Boolean(job.validationRunId),
  });
  const [overrideOpen, setOverrideOpen] = useState(false);
  const [overrideScope, setOverrideScope] = useState<'rule' | 'all_overrideable'>('rule');
  const [detailsTab, setDetailsTab] = useState<'diagnostics' | 'technical'>('diagnostics');
  const [reason, setReason] = useState('');
  const [selectedRules, setSelectedRules] = useState<string[]>([]);
  const override = useMutation({
    mutationFn: () => api.createValidationOverride({ jobId: job.id, scope: overrideScope, reason, ruleKeys: selectedRules, requestedHandoff: 'publisher' }),
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
  const skippedByKey = new Map<string, ValidationOverride>();
  if (run) {
    for (const event of detail.data?.overrides ?? []) {
      if (!overrideEventMatchesRun(event, run)) continue;
      for (const key of event.ruleKeys) skippedByKey.set(key, event);
    }
  }
  const unresolved = (finding: ValidationFinding) => finding.state !== 'PASS' && finding.state !== 'NOT_APPLICABLE';
  const remainingOverrideable = (run?.findings ?? []).filter((finding) => finding.overrideable && unresolved(finding) && !skippedByKey.has(finding.key));
  const openRuleSkip = (finding: ValidationFinding) => {
    setOverrideScope('rule');
    setSelectedRules([finding.key]);
    setReason('');
    setOverrideOpen(true);
  };
  const openSkipAll = () => {
    setOverrideScope('all_overrideable');
    setSelectedRules(remainingOverrideable.map((finding) => finding.key).sort());
    setReason('');
    setOverrideOpen(true);
  };
  const matchingOverrides = (detail.data?.overrides ?? []).filter((event) => run && overrideEventMatchesRun(event, run));
  return (
    <Stack spacing={2} sx={{ pt: 1 }}>
      {detail.isError ? <Alert severity="warning">{errorMessage(detail.error)}</Alert> : null}
      {detail.isLoading ? <Typography color="text.secondary">Loading immutable findings…</Typography> : null}
      {!detail.isLoading && !detail.isError && !run ? <Alert severity="info">No Validation run data was returned.</Alert> : null}
      {run ? <>
        <ValidationRunSummary job={job} run={run} effectiveHandoff={detail.data?.effectiveHandoff ?? 'UNAVAILABLE'} />
        <Tabs value={detailsTab} onChange={(_, value: 'diagnostics' | 'technical') => setDetailsTab(value)} aria-label="Validation run detail views">
          <Tab label="Diagnostics" value="diagnostics" />
          <Tab label="Technical Evidence" value="technical" />
        </Tabs>
        {detailsTab === 'diagnostics' ? <Stack spacing={2}>
          <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
            <Alert severity="info" sx={{ flex: 1 }}><b>Policy used by this run:</b> {run.policySnapshot ? `${presetName(run.policyPreset)} · ${run.policyFingerprint}` : 'No frozen policy snapshot is available for this historical run.'}</Alert>
            <Alert severity={detail.data?.policyCurrent ? 'success' : 'warning'} sx={{ flex: 1 }}><b>Current Validation Settings:</b> {policySummary(detail.data?.currentPolicy as unknown as Record<string, unknown>)}{!detail.data?.policyCurrent ? ' · different from this run' : ''}</Alert>
          </Stack>
          <ValidationFindingsTable findings={run.findings} overridesByKey={skippedByKey} onSkip={openRuleSkip} />
          {remainingOverrideable.length > 0 ? <Stack direction="row" justifyContent="flex-end"><Button variant="outlined" onClick={openSkipAll}>Skip all overrideable findings</Button></Stack> : null}
          {matchingOverrides.length > 0 ? <Stack spacing={0.5}>
            <Typography variant="subtitle2">Override audit history</Typography>
            {matchingOverrides.map((event) => <Alert key={event.eventId} severity="info">
              Manual SKIPPED event by {event.actorId} · {new Date(event.createdAt).toLocaleString()} · {event.reason ? `Reason: ${event.reason}` : 'No reason provided'} · {event.ruleKeys.join(', ')}. Canonical findings remain unchanged.
            </Alert>)}
          </Stack> : null}
        </Stack> : <ValidationTechnicalEvidence run={run} />}
        <OverrideDialog scope={overrideScope} open={overrideOpen} onClose={() => setOverrideOpen(false)} findings={run.findings.filter((finding) => selectedRules.includes(finding.key))} reason={reason} setReason={setReason} pending={override.isPending} error={override.isError ? errorMessage(override.error) : ''} onSave={() => override.mutate()} />
      </> : null}
    </Stack>
  );
}

function overrideEventMatchesRun(event: ValidationOverride, run: ValidationRun): boolean {
  return event.validationRunId === run.runId && event.resultArtifactId === run.resultArtifactId &&
    event.resultGenerationToken === run.resultGenerationToken && event.resultGenerationVersion === run.resultGenerationVersion &&
    event.resultOutputRevision === run.resultOutputRevision && event.policySchemaVersion === run.policySchemaVersion && event.policyVersion === run.policyVersion &&
    event.policyFingerprint === run.policyFingerprint && Boolean(event.actorSource && event.actorId && event.createdAt && event.ruleKeys.length > 0);
}

function OverrideDialog({ scope, open, onClose, findings, reason, setReason, pending, error, onSave }: {
  scope: 'rule' | 'all_overrideable'; open: boolean; onClose: () => void; findings: ValidationFinding[];
  reason: string; setReason: (value: string) => void; pending: boolean; error: string; onSave: () => void;
}) {
  return <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
    <DialogTitle>{scope === 'rule' ? 'Skip validation rule' : 'Skip all validation findings'}</DialogTitle>
    <DialogContent><Stack spacing={1} sx={{ pt: 1 }}>
      <Alert severity="info">The canonical Validation result remains unchanged. Publisher handoff is evaluated separately from the skipped finding.</Alert>
      {findings.map((finding) => <Box key={finding.key} sx={{ py: 0.5 }}><Typography fontWeight={700}>{finding.key}</Typography><Typography variant="body2">Canonical result: {finding.state}</Typography><Typography variant="body2" color="text.secondary">{finding.explanation}</Typography></Box>)}
      <TextField label="Reason (optional)" value={reason} onChange={(event) => setReason(event.target.value)} multiline minRows={2} inputProps={{ maxLength: 2000 }} />
      {error ? <Alert severity="warning">{error}</Alert> : null}
    </Stack></DialogContent>
    <DialogActions><Button onClick={onClose}>Cancel</Button><Button onClick={onSave} disabled={pending || findings.length === 0}>{scope === 'rule' ? 'Skip rule' : 'Skip all'}</Button></DialogActions>
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
function errorMessage(error: unknown) { return error instanceof Error ? error.message : 'The request could not be completed.'; }

function presetName(value: string): string {
  return value ? value[0].toUpperCase() + value.slice(1) : 'Unknown';
}

function policySummary(value: Record<string, unknown> | undefined): string {
  if (!value || typeof value.preset !== 'string') return 'Loading';
  if (value.preset === 'custom') {
    const basedOn = typeof value.basedOn === 'string' ? value.basedOn : 'Default';
    const count = typeof value.differenceCount === 'number' ? value.differenceCount : 0;
    return `Custom · Based on: ${basedOn} · ${count} ${count === 1 ? 'change' : 'changes'}`;
  }
  return value.preset === 'default' ? 'Default (Recommended)' : String(value.preset).replace(/^./, (letter) => letter.toUpperCase());
}
