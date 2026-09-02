import { Alert, Box, Checkbox, Chip, FormControlLabel, Stack, Typography } from '@mui/material';
import type { RestorationRecommendation, RestorationRecommendationPlan } from '../api/types';
import { isActionableRestorationRecommendation, restorationRecommendationSelectionID } from '../utils/restorationRecommendations';

export function RestorationRecommendationPanel({ plan, selected, onToggle }: {
  plan: RestorationRecommendationPlan;
  selected: string[];
  onToggle: (id: string, checked: boolean) => void;
}) {
  return (
    <Stack spacing={1}>
      <Typography variant="h3">Restoration recommendation plan</Typography>
      {plan.applyLocked ? <Alert severity="warning">{plan.applyLockReason}</Alert> : null}
      <Stack spacing={1}>
        {(plan.recommendations ?? []).map((item) => {
          const selectionID = restorationRecommendationSelectionID(item.id);
          const actionable = isActionableRestorationRecommendation(item);
          const output = item.resolvedOutput?.width && item.resolvedOutput?.height
            ? `${item.resolvedOutput.width}×${item.resolvedOutput.height}${item.resolvedOutput.sar ? ` · SAR ${item.resolvedOutput.sar}` : ''}`
            : '';
          return (
            <Box key={item.id} sx={{ p: 1.25, border: 1, borderColor: 'divider', borderRadius: 1 }}>
              {actionable ? (
                <FormControlLabel
                  disabled={plan.applyLocked}
                  control={<Checkbox checked={selected.includes(selectionID)} onChange={(event) => onToggle(selectionID, event.target.checked)} />}
                  label={<Stack><Typography fontWeight={700}>{item.domain}</Typography><Typography variant="body2">{displayRecommendation(item)}{output ? ` · ${output}` : ''}</Typography></Stack>}
                />
              ) : (
                <Stack><Typography fontWeight={700}>{item.domain}</Typography><Typography variant="body2">{displayRecommendation(item)}</Typography></Stack>
              )}
              <Stack direction="row" spacing={0.5} sx={{ pl: actionable ? 4 : 0, mt: 0.5 }}>
                <Chip size="small" label={humanize(item.state)} color={item.state === 'manual_review' ? 'warning' : 'default'} />
                <Chip size="small" label={`confidence ${humanize(item.confidence)}`} />
              </Stack>
              <Typography variant="caption" color="text.secondary" display="block" sx={{ pl: actionable ? 4 : 0 }}>Current: {item.currentValue ? humanize(item.currentValue) : 'Not configured'}</Typography>
              <Typography variant="caption" color="text.secondary" display="block" sx={{ pl: actionable ? 4 : 0 }}>Recommended: {displayRecommendation(item)}{output ? ` · ${output}` : ''}</Typography>
              {(item.reasons ?? []).map((detail, index) => (
                <Typography key={`${item.id}-reason-${index}`} variant="caption" color="text.secondary" display="block" sx={{ pl: actionable ? 4 : 0 }}>Reason: {detail}</Typography>
              ))}
              {(item.warnings ?? []).map((detail, index) => (
                <Typography key={`${item.id}-warning-${index}`} variant="caption" color="warning.main" display="block" sx={{ pl: actionable ? 4 : 0 }}>Warning: {detail}</Typography>
              ))}
              {(item.supportingEvidence ?? []).map((detail, index) => (
                <Typography key={`${item.id}-evidence-${index}`} variant="caption" color="text.secondary" display="block" sx={{ pl: actionable ? 4 : 0 }}>Evidence: {detail}</Typography>
              ))}
            </Box>
          );
        })}
      </Stack>
    </Stack>
  );
}

function displayRecommendation(item: RestorationRecommendation) {
  if (item.state === 'manual_review') return 'Manual review';
  if (item.state === 'no_recommendation') return 'Analysis unavailable · no recommendation';
  if (item.state === 'not_applicable') return 'Not applicable';
  return item.recommendedValue ? humanize(item.recommendedValue) : 'Recommended';
}

function humanize(value: string) {
  return value.replaceAll('_', ' ').replace(/^./, (character) => character.toUpperCase());
}
