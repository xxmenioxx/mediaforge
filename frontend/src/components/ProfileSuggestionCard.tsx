import { Alert, Button, Card, CardContent, Chip, Stack, Typography } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { Profile, ProfileSuggestion } from '../api/types';

type MotionStatus = NonNullable<ProfileSuggestion['scan']['interlaceAnalysis']>['status'];

export function ProfileSuggestionCard({ suggestion, onSelect, onApplyMotionRecommendation }: { suggestion: ProfileSuggestion; onSelect?: (profile: Profile) => void; onApplyMotionRecommendation?: (status: MotionStatus) => Promise<string> }) {
  const queryClient = useQueryClient();
  const create = useMutation({
    mutationFn: api.createProfile,
    onSuccess: async (profile) => {
      await queryClient.invalidateQueries({ queryKey: ['profiles'] });
      onSelect?.(profile);
    },
  });
  const applyMotion = useMutation({
    mutationFn: async () => onApplyMotionRecommendation?.(suggestion.scan.interlaceAnalysis?.status) ?? '',
  });
  const existing = suggestion.suggestedProfile;
  const proposed = suggestion.proposedProfile;
  const motion = motionDiagnosis(suggestion);

  return (
    <Card variant="outlined">
      <CardContent>
        <Stack spacing={1.5}>
          <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
            <Typography variant="h3">Suggested profile</Typography>
            <Chip label={suggestion.matchType === 'existing' ? 'Existing match' : 'New profile proposed'} color={suggestion.matchType === 'existing' ? 'success' : 'primary'} size="small" />
          </Stack>
          <Typography color="text.secondary">{suggestion.summary}</Typography>
          <Alert severity={motion.severity}>
            <Typography fontWeight={700}>{motion.title}</Typography>
            <Typography variant="body2">{motion.detail}</Typography>
            <Stack spacing={0.5} sx={{ mt: 1 }}>
              <Typography fontWeight={700} variant="body2">All recommendations</Typography>
              {allRecommendations(suggestion, motion.detail).map((recommendation) => (
                <Typography key={recommendation} variant="body2">• {recommendation}</Typography>
              ))}
            </Stack>
          </Alert>
          <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
            {onApplyMotionRecommendation ? (
              <Button
                variant="contained"
                size="small"
                disabled={applyMotion.isPending}
                onClick={() => applyMotion.mutate()}
              >
                {applyMotion.isPending ? 'Applying…' : 'Apply recommendations'}
              </Button>
            ) : null}
            {suggestion.insights ? (
              <>
                <Chip label={`Recommended CRF ${suggestion.insights.recommendedCrf}`} color="primary" size="small" />
                <Chip label={`Estimated size ${formatBytes(suggestion.insights.estimatedMinBytes)}–${formatBytes(suggestion.insights.estimatedMaxBytes)}`} size="small" />
                <Chip label={savingsLabel(suggestion.insights.estimatedSavingsLow, suggestion.insights.estimatedSavingsHigh)} color={suggestion.insights.estimatedSavingsHigh > 0 ? 'success' : 'warning'} size="small" />
              </>
            ) : null}
          </Stack>
          {applyMotion.isSuccess ? <Alert severity="success">{applyMotion.data || 'Recommendation applied to this asset.'}</Alert> : null}
          {applyMotion.isError ? <Alert severity="warning">Could not apply recommendation: {applyMotion.error instanceof Error ? applyMotion.error.message : 'unknown error'}</Alert> : null}
          {existing ? (
            <>
              <Typography fontWeight={700}>{existing.name}</Typography>
              <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                <Chip label={existing.videoCodec} size="small" />
                <Chip label={`CRF ${existing.qualityValue}`} size="small" />
                <Chip label={existing.container.toUpperCase()} size="small" />
                <Chip label={`${suggestion.candidates[0]?.score ?? 0}% fit`} size="small" />
              </Stack>
              {onSelect ? <Button startIcon={<CheckCircleOutlineIcon />} variant="contained" onClick={() => onSelect(existing)} sx={{ alignSelf: 'flex-start' }}>Select suggested profile</Button> : null}
            </>
          ) : (
            <>
              <Typography fontWeight={700}>{proposed.name}</Typography>
              <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                <Chip label={proposed.videoCodec} size="small" />
                <Chip label={`CRF ${proposed.qualityValue}`} size="small" />
                <Chip label={proposed.container.toUpperCase()} size="small" />
              </Stack>
              <Alert severity="info">This is a draft derived from technical metadata. It is never saved or used automatically.</Alert>
              <Button startIcon={<AddIcon />} variant="contained" disabled={create.isPending} onClick={() => create.mutate(proposed)} sx={{ alignSelf: 'flex-start' }}>Create proposed profile</Button>
              {create.isSuccess ? <Alert severity="success">Profile created{onSelect ? ' and selected' : ''}.</Alert> : null}
              {create.isError ? <Alert severity="warning">Could not create the proposed profile. Its name may already exist.</Alert> : null}
            </>
          )}
        </Stack>
      </CardContent>
    </Card>
  );
}

function allRecommendations(suggestion: ProfileSuggestion, motion: string) {
  const values = [
    motion,
    ...(suggestion.insights?.recommendations ?? []),
    ...(suggestion.candidates?.[0]?.reasons ?? []),
  ].map((value) => value.trim()).filter(Boolean);
  return Array.from(new Set(values));
}

function motionDiagnosis(suggestion: ProfileSuggestion): { title: string; detail: string; severity: 'success' | 'info' | 'warning'; action: string } {
  const analysis = suggestion.scan.interlaceAnalysis;
  const confidence = typeof analysis?.confidence === 'number' ? ` · confidence ${Math.round(analysis.confidence * 100)}%` : '';
  const window = analysis?.windowSeconds ? ` · ${analysis.windowSeconds}s sample` : '';
  switch (analysis?.status) {
    case 'progressive':
      return { title: `Progressive${confidence}`, detail: `No deinterlacing is recommended${window}.`, severity: 'success', action: 'Apply: no deinterlace' };
    case 'interlaced':
      return { title: `Interlaced${confidence}`, detail: `${analysis.recommendedFilter ? `Recommended filter: ${analysis.recommendedFilter}.` : 'Deinterlacing is recommended.'}${window}`, severity: 'warning', action: 'Apply bwdif' };
    case 'mixed':
      return { title: `Mixed progressive/interlaced${confidence}`, detail: `Review a motion-heavy preview before conversion; automatic correction should not be assumed safe${window}.`, severity: 'warning', action: 'Mark for review' };
    case 'telecine_suspected':
      return {
        title: `Telecine suspected${confidence}`,
        detail: analysis.recommendedFilter
          ? `${analysis.fieldOrderMismatch ? `Container field order ${(analysis.containerFieldOrder || 'unknown').toUpperCase()} conflicts with detected ${(analysis.detectedFieldOrder || 'unknown').toUpperCase()}. ` : ''}Recommended filter: ${analysis.recommendedFilter}${window}.`
          : `Validate cadence in LAB before choosing deinterlacing or IVTC${window}.`,
        severity: 'warning',
        action: analysis.recommendedMode ? `Apply ${analysis.recommendedMode.toUpperCase()}` : 'Apply fieldmatch + decimate',
      };
    default:
      return { title: 'Scan type unknown', detail: `MVForge could not classify motion structure reliably; inspect a preview before conversion${window}.`, severity: 'info', action: 'Mark for review' };
  }
}

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return 'N/A';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** index).toFixed(index >= 3 ? 2 : 1)} ${units[index]}`;
}

function savingsLabel(low: number, high: number) {
  if (high <= 0) return 'No size saving expected';
  return `Estimated saving ${Math.max(0, low)}–${Math.max(0, high)}%`;
}
