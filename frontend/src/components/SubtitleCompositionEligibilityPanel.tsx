import { Alert, Button, Stack, Typography } from '@mui/material';
import type { SubtitleCompositionEligibility } from '../api/types';

type SubtitleCompositionEligibilityPanelProps = {
  eligibility?: SubtitleCompositionEligibility;
  loading?: boolean;
  onCompose?: () => void;
};

export function SubtitleCompositionEligibilityPanel({ eligibility, loading = false, onCompose }: SubtitleCompositionEligibilityPanelProps) {
  if (loading) return <Typography variant="body2" color="text.secondary">Loading subtitle composition eligibility…</Typography>;
  if (!eligibility) return null;
  return (
    <Stack spacing={0.75} sx={{ mt: 1 }}>
      <Typography fontWeight={700}>Subtitle composition</Typography>
      <Typography variant="body2" color="text.secondary">
        Backend policy {eligibility.policyVersion}. {eligibility.primaryCandidates.length} primary candidate(s), {eligibility.supplementalCandidates.length} supplemental candidate(s).
      </Typography>
      {!eligibility.eligible ? <Alert severity="info">{eligibility.reason || 'No compatible subtitle composition is currently eligible.'}</Alert> : null}
      {eligibility.eligible ? <Button size="small" variant="outlined" disabled={!onCompose} onClick={onCompose}>Compose subtitles</Button> : null}
    </Stack>
  );
}

