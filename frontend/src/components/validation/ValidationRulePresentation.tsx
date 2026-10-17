import { Alert, Chip, Divider, Stack, Typography } from '@mui/material';
import type { ValidationFinding, ValidationOverride } from '../../api/types';
import { formatEvidenceFields, validationFindingDiagnosis } from './validationRuleFormatters';

export function ValidationFindingStatus({ state }: { state: ValidationFinding['state'] }) {
  const color = state === 'FAIL' ? 'error' : state === 'PASS' ? 'success' : state === 'NOT_APPLICABLE' ? 'default' : 'warning';
  const label = state === 'REVIEW_REQUIRED' ? 'REVIEW REQUIRED' : state === 'NOT_APPLICABLE' ? 'NOT APPLICABLE' : state;
  return <Chip size="small" label={label} color={color} />;
}

function EvidenceList({ value, label }: { value: unknown; label: string }) {
  const fields = formatEvidenceFields(value, label);
  if (!fields.length) return <Typography color="text.secondary">No structured evidence recorded.</Typography>;
  return <Stack spacing={0.5}>
    {fields.map((field, index) => <Stack key={`${field.label}-${index}`} direction={{ xs: 'column', sm: 'row' }} spacing={{ xs: 0, sm: 1 }}>
      <Typography variant="body2" color="text.secondary" sx={{ minWidth: { sm: 155 }, overflowWrap: 'anywhere' }}>{field.label}</Typography>
      <Typography variant="body2" sx={{ whiteSpace: 'pre-wrap', overflowWrap: 'anywhere', flex: 1 }}>{field.value}</Typography>
    </Stack>)}
  </Stack>;
}

export function ValidationFindingDetails({ finding, override }: { finding: ValidationFinding; override?: ValidationOverride }) {
  const expectedFields = formatEvidenceFields(finding.expected, 'Expected');
  const actualFields = formatEvidenceFields(finding.actual, 'Observed');
  const primaryEvidenceMissing = expectedFields.some((field) => field.value === 'Not recorded' || field.value === 'No fields recorded')
    || actualFields.some((field) => field.value === 'Not recorded' || field.value === 'No fields recorded');

  return <Stack spacing={1.5} sx={{ py: 1.5, px: { xs: 1, sm: 2 } }}>
    {finding.state === 'UNVERIFIED' ? <Alert severity="warning"><b>UNVERIFIED.</b> This finding is not a pass because the available evidence could not verify it.</Alert> : null}
    {primaryEvidenceMissing ? <Alert severity="info"><b>Evidence availability: UNVERIFIED.</b> One or more expected or observed values are unavailable in this run. The canonical finding state remains {finding.state}.</Alert> : null}
    <Typography>{validationFindingDiagnosis(finding)}</Typography>
    <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}>
      <Stack spacing={0.75} sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="subtitle2">Expected</Typography>
        <EvidenceList value={finding.expected} label="Expected" />
      </Stack>
      <Stack spacing={0.75} sx={{ flex: 1, minWidth: 0 }}>
        <Typography variant="subtitle2">Observed</Typography>
        <EvidenceList value={finding.actual} label="Observed" />
      </Stack>
    </Stack>
    <Stack spacing={0.75}>
      <Typography variant="subtitle2">{finding.key === 'timeline.av_alignment' ? 'Per-track timing evidence' : 'Additional structured evidence'}</Typography>
      {finding.details === undefined || finding.details === null
        ? <Typography variant="body2" color="text.secondary">{finding.key === 'timeline.av_alignment' ? 'Per-track comparison details are unavailable in this run.' : 'No additional structured evidence was recorded for this finding.'}</Typography>
        : <EvidenceList value={finding.details} label={finding.key === 'timeline.av_alignment' ? 'Per-track timing' : 'Evidence'} />}
    </Stack>
    <Stack spacing={0.5}>
      <Typography variant="subtitle2">Evidence origin</Typography>
      {finding.evidenceReferences.length
        ? <Stack direction="row" spacing={0.75} useFlexGap flexWrap="wrap">{finding.evidenceReferences.map((reference) => <Chip key={reference} size="small" label={reference} variant="outlined" sx={{ maxWidth: '100%', '& .MuiChip-label': { whiteSpace: 'normal', overflowWrap: 'anywhere' } }} />)}</Stack>
        : <Typography variant="body2" color="text.secondary">Evidence origin is unavailable in this run.</Typography>}
    </Stack>
    <Divider />
    <Stack spacing={0.5}>
      <Typography variant="subtitle2">Next step</Typography>
      <Typography variant="body2">Review the recorded evidence and the available run actions before continuing. This view does not prescribe an automated repair.</Typography>
    </Stack>
    {override ? <Alert severity="info"><b>Manual SKIPPED override recorded.</b> Canonical status remains {finding.state}. By {override.actorId} · {new Date(override.createdAt).toLocaleString()} · {override.reason ? `Reason: ${override.reason}` : 'No reason provided'}.</Alert> : null}
  </Stack>;
}
