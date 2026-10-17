import { Box, Chip, Divider, Stack, Typography } from '@mui/material';
import type { ValidationRun, ValidationSummary } from '../../api/types';

function outcomeColor(outcome: string): 'success' | 'warning' | 'error' | 'default' {
  if (outcome === 'PASS') return 'success';
  if (outcome === 'PASS_WITH_WARNINGS') return 'warning';
  if (outcome === 'FAIL') return 'error';
  return 'default';
}

function handoffLabel(handoff: string): { label: string; color: 'success' | 'warning' | 'error' | 'info' | 'default' } {
  if (handoff === 'ALLOWED') return { label: 'ALLOWED', color: 'success' };
  if (handoff === 'ALLOWED_BY_OVERRIDE') return { label: 'ALLOWED BY OVERRIDE', color: 'warning' };
  if (handoff === 'BLOCKED') return { label: 'BLOCKED', color: 'error' };
  if (handoff === 'HISTORICAL_ONLY') return { label: 'HISTORICAL ONLY', color: 'info' };
  return { label: handoff || 'UNAVAILABLE', color: 'default' };
}

export function ValidationRunSummary({ job, run, effectiveHandoff }: {
  job: ValidationSummary;
  run: ValidationRun;
  effectiveHandoff: string;
}) {
  const handoff = handoffLabel(effectiveHandoff);
  const count = (state: string) => run.findings.filter((finding) => finding.state === state).length;
  const counts = [
    ['Fail', count('FAIL')],
    ['Warning', count('WARNING')],
    ['Unverified', count('UNVERIFIED')],
    ['Review required', count('REVIEW_REQUIRED')],
    ['Passed', count('PASS')],
    ['Not applicable', count('NOT_APPLICABLE')],
  ] as const;

  return <Box component="section" aria-label="Validation run summary" sx={{ border: 1, borderColor: 'divider', borderRadius: 1.5, p: { xs: 1.5, sm: 2 } }}>
    <Stack spacing={1.5}>
      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1.5}>
        <Stack spacing={0.5} sx={{ minWidth: 0 }}>
          <Typography variant="overline" color="text.secondary">Asset</Typography>
          <Typography fontWeight={700} sx={{ overflowWrap: 'anywhere' }}>{job.mediaPath}</Typography>
          <Typography variant="body2" color="text.secondary">Job #{run.jobId} · Execution #{run.executionNumber} · Run {run.runId}</Typography>
          <Typography variant="caption" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>Run time {new Date(run.createdAt).toLocaleString()} · Actor {run.actorId}</Typography>
        </Stack>
        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap alignItems="flex-start" sx={{ maxWidth: { sm: 420 } }}>
          <Chip label={`Canonical Validation: ${run.outcome}`} color={outcomeColor(run.outcome)} />
          <Chip label={`Score ${run.score}/100`} variant="outlined" />
        </Stack>
      </Stack>
      <Divider />
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} alignItems={{ md: 'center' }}>
        <Stack spacing={0.5} sx={{ flex: 1, minWidth: 0 }}>
          <Typography variant="subtitle2">Findings</Typography>
          <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">
            {counts.map(([label, value]) => <Chip key={label} size="small" label={`${label}: ${value}`} variant="outlined" />)}
          </Stack>
        </Stack>
        <Stack spacing={0.5} sx={{ minWidth: { md: 255 } }}>
          <Typography variant="subtitle2">Effective Publisher handoff</Typography>
          <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
            <Chip label={handoff.label} color={handoff.color} />
            <Typography variant="caption" color="text.secondary">Backend result: {effectiveHandoff || 'unavailable'}</Typography>
          </Stack>
        </Stack>
      </Stack>
    </Stack>
  </Box>;
}
