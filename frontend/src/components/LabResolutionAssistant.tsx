import { Alert, Button, Chip, Stack, Typography } from '@mui/material';
import type { LabResolutionRequired } from '../api/types';

function displayEvidenceValue(value: unknown) {
  if (value == null || value === '') return '';
  if (typeof value === 'string' || typeof value === 'number' || typeof value === 'boolean') return String(value);
  try { return JSON.stringify(value); } catch { return '[unavailable]'; }
}

type Props = {
  resolution: LabResolutionRequired;
  onApply: (action: LabResolutionRequired['actions'][number], fieldOrder?: 'tff' | 'bff') => void;
  onTest: (action: LabResolutionRequired['actions'][number], fieldOrder?: 'tff' | 'bff') => void;
  onReanalyze: () => void;
  reanalyzing?: boolean;
};

export function LabResolutionAssistant({ resolution, onApply, onTest, onReanalyze, reanalyzing = false }: Props) {
  const recommended = resolution.actions.find((action) => action.eligible && action.recommended);
  const fidelityReview = resolution.gate === 'canonical_fidelity';
  return <Alert severity="warning" data-testid="lab-resolution-assistant">
    <Stack spacing={1}>
      <Typography variant="subtitle2">{fidelityReview ? 'Why Canonical Frame Fidelity needs review' : 'Why Auto needs review'}</Typography>
      {resolution.blockingFields.map((item) => <Stack key={`${item.field}:${item.source ?? ''}`} spacing={0.25}>
        <Typography variant="body2"><strong>{item.field}</strong> · {item.authorityState}{item.authority ? ` · ${item.authority}` : ''}</Typography>
        <Typography variant="caption" color="text.secondary">{item.reason}{displayEvidenceValue(item.expected) ? ` Expected: ${displayEvidenceValue(item.expected)}.` : ''}{displayEvidenceValue(item.value) ? ` Observed: ${displayEvidenceValue(item.value)}.` : ''}</Typography>
      </Stack>)}
      {resolution.actions.filter((action) => action.eligible).map((action) => <Stack key={action.id} spacing={0.5}>
        <Stack direction="row" spacing={0.75} alignItems="center">
          <Typography variant="body2">{action.label}</Typography>
          {action === recommended ? <Chip size="small" color="primary" label="Recommended alternative" /> : null}
        </Stack>
        <Typography variant="caption" color="text.secondary">{action.reason}{action.tradeoffs ? ` Trade-offs: ${action.tradeoffs}.` : ''}</Typography>
        {action.id === 'reanalyze_cadence'
          ? <Button size="small" variant="outlined" disabled={reanalyzing} onClick={onReanalyze}>{reanalyzing ? 'Analyzing…' : 'Re-analyze cadence'}</Button>
          : action.id === 'compare_field_order'
            ? <Stack direction="row" spacing={1}>
              {(['tff', 'bff'] as const).map((order) => <Button key={order} size="small" variant="outlined" onClick={() => onTest(action, order)}>Test {order.toUpperCase()} alternative</Button>)}
            </Stack>
            : <Stack direction="row" spacing={1}>
              <Button size="small" variant="outlined" onClick={() => onApply(action)}>Apply to this asset</Button>
              <Button size="small" variant="contained" onClick={() => onTest(action)}>Test alternative</Button>
            </Stack>}
      </Stack>)}
      <Typography variant="caption" color="text.secondary">{fidelityReview
        ? `Plan status: ${resolution.planningStatus}. Canonical Frame Fidelity: ${resolution.fidelityStatus ?? 'UNVERIFIED'}. Safe remains unavailable until canonical evidence passes.`
        : 'Plan status: NEEDS_REVIEW. Fidelity remains governed by the existing sample and canonical evidence.'}</Typography>
    </Stack>
  </Alert>;
}
