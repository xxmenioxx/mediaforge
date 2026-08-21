import { Alert, Box, Button, Card, CardContent, Checkbox, Chip, FormControlLabel, Stack, Typography } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import type { AdvisorFinding, Profile, ProfileSuggestion } from '../api/types';

export function ProfileSuggestionCard({ suggestion, onSelect, onApplyRecommendations, onReviewInLab }: { suggestion: ProfileSuggestion; onSelect?: (profile: Profile) => void; onApplyRecommendations?: (findings: AdvisorFinding[]) => Promise<string>; onReviewInLab?: () => void }) {
  const queryClient = useQueryClient();
  const create = useMutation({
    mutationFn: api.createProfile,
    onSuccess: async (profile) => {
      await queryClient.invalidateQueries({ queryKey: ['profiles'] });
      onSelect?.(profile);
    },
  });
  const selectionKey = `${suggestion.scan.id}:${suggestion.scan.updatedAt}:${suggestion.matchType}`;
  const defaultFindingIds = (suggestion.findings ?? []).filter((finding) => finding.actionable && finding.defaultSelected).map((finding) => finding.id);
  const [findingSelection, setFindingSelection] = useState<{ key: string; ids: string[] }>(() => ({ key: selectionKey, ids: defaultFindingIds }));
  const selectedFindingIds = findingSelection.key === selectionKey ? findingSelection.ids : defaultFindingIds;
  const setSelectedFindingIds = (update: (current: string[]) => string[]) => setFindingSelection({ key: selectionKey, ids: update(selectedFindingIds) });
  const applyFindings = useMutation({
    mutationFn: async () => onApplyRecommendations?.((suggestion.findings ?? []).filter((finding) => finding.actionable && selectedFindingIds.includes(finding.id))) ?? '',
  });
  const existing = suggestion.suggestedProfile;
  const proposed = suggestion.proposedProfile;
  const motion = motionDiagnosis(suggestion);
  const staleMotionAnalysis = (suggestion.scan.interlaceAnalysis?.version ?? 0) < 3;
  const motionDetail = staleMotionAnalysis
    ? 'This motion analysis was created by an older detector and must be analyzed again before its correction can be applied.'
    : motion.detail;

  return (
    <Card variant="outlined">
      <CardContent>
        <Stack spacing={1.5}>
          <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
            <Typography variant="h3">MVForge Suggestions</Typography>
            <Chip label={matchTypeLabel(suggestion.matchType)} color={suggestion.matchType === 'create' ? 'primary' : 'success'} size="small" />
          </Stack>
          <Typography color="text.secondary">{suggestion.summary}</Typography>
          <Alert severity={staleMotionAnalysis ? 'warning' : motion.severity}>
            <Typography fontWeight={700}>{motion.title}</Typography>
            <Typography variant="body2">{motionDetail}</Typography>
            <Stack spacing={0.5} sx={{ mt: 1 }}>
              <Typography fontWeight={700} variant="body2">Findings and recommendations</Typography>
              {(suggestion.findings ?? []).map((finding) => (
                <Box key={finding.id} sx={{ py: 0.35 }}>
                  {finding.actionable ? (
                    <FormControlLabel
                      control={<Checkbox size="small" checked={selectedFindingIds.includes(finding.id)} onChange={(_, checked) => setSelectedFindingIds((current) => checked ? [...new Set([...current, finding.id])] : current.filter((id) => id !== finding.id))} />}
                      label={<Stack><Typography variant="body2" fontWeight={700}>{finding.title}</Typography><Typography variant="caption" color="text.secondary">{finding.detail}</Typography></Stack>}
                    />
                  ) : <Stack sx={{ pl: 0.5 }}><Typography variant="body2" fontWeight={700}>{finding.title}</Typography><Typography variant="caption" color="text.secondary">{finding.detail}</Typography></Stack>}
                  <Stack direction="row" spacing={0.5} sx={{ pl: finding.actionable ? 4 : 0.5, mt: 0.25 }}><Chip size="small" label={finding.category.replace('_', ' ')} /><Chip size="small" label={finding.severity} color={finding.severity === 'review' || finding.severity === 'unsafe' ? 'warning' : 'default'} /><Chip size="small" label={`confidence ${finding.confidence}`} /></Stack>
                </Box>
              ))}
              {!(suggestion.findings ?? []).length ? allRecommendations(suggestion, motionDetail).map((recommendation) => <Typography key={recommendation} variant="body2">• {recommendation}</Typography>) : null}
            </Stack>
          </Alert>
          <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
            {onApplyRecommendations ? (
              <Button
                variant="contained"
                size="small"
                disabled={applyFindings.isPending || selectedFindingIds.length === 0}
                onClick={() => applyFindings.mutate()}
              >
                {applyFindings.isPending ? 'Applying…' : `Apply selected (${selectedFindingIds.length})`}
              </Button>
            ) : null}
            {onReviewInLab ? <Button variant="outlined" size="small" onClick={onReviewInLab}>Review in Lab</Button> : null}
            {suggestion.insights ? (
              <>
                <Chip label={`Recommended CRF ${suggestion.insights.recommendedCrf}`} color="primary" size="small" />
                <Chip label={`Estimated size ${formatBytes(suggestion.insights.estimatedMinBytes)}–${formatBytes(suggestion.insights.estimatedMaxBytes)}`} size="small" />
                <Chip label={savingsLabel(suggestion.insights.estimatedSavingsLow, suggestion.insights.estimatedSavingsHigh)} color={suggestion.insights.estimatedSavingsHigh > 0 ? 'success' : 'warning'} size="small" />
              </>
            ) : null}
          </Stack>
          {applyFindings.isSuccess ? <Alert severity="success">{applyFindings.data || 'Selected recommendations applied to this asset.'}</Alert> : null}
          {applyFindings.isError ? <Alert severity="warning">Could not apply recommendation: {applyFindings.error instanceof Error ? applyFindings.error.message : 'unknown error'}</Alert> : null}
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
              {suggestion.candidates.slice(0, 2).map((candidate) => candidate.profile.id === existing.id ? null : <Box key={candidate.profile.id} sx={{ p: 1, border: 1, borderColor: 'divider', borderRadius: 1 }}><Typography fontWeight={700}>{candidate.profile.name}</Typography><Typography variant="caption" color="text.secondary">Alternative · {candidate.score}% fit</Typography>{onSelect ? <Button size="small" onClick={() => onSelect(candidate.profile)}>Select alternative</Button> : null}</Box>)}
            </>
          ) : (
            <>
              <Typography fontWeight={700}>{proposed.name}</Typography>
              <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                <Chip label={proposed.videoCodec} size="small" />
                <Chip label={`CRF ${proposed.qualityValue}`} size="small" />
                <Chip label={proposed.container.toUpperCase()} size="small" />
                {proposed.optimizationIntent ? <Chip label={proposed.optimizationIntent.replaceAll('_', ' ')} size="small" variant="outlined" /> : null}
              </Stack>
              <Alert severity="info">This is a draft derived from technical metadata. It is never saved or used automatically.</Alert>
              <Button startIcon={<AddIcon />} variant="contained" disabled={create.isPending} onClick={() => create.mutate(proposed)} sx={{ alignSelf: 'flex-start' }}>Create proposed profile</Button>
              {suggestion.candidates.slice(0, 2).map((candidate) => <Box key={candidate.profile.id} sx={{ p: 1, border: 1, borderColor: 'divider', borderRadius: 1 }}><Typography fontWeight={700}>{candidate.profile.name}</Typography><Typography variant="caption" color="text.secondary">Existing alternative · {candidate.score}% fit</Typography>{onSelect ? <Button size="small" onClick={() => onSelect(candidate.profile)}>Select alternative</Button> : null}</Box>)}
              {create.isSuccess ? <Alert severity="success">Profile created{onSelect ? ' and selected' : ''}.</Alert> : null}
              {create.isError ? <Alert severity="warning">Could not create the proposed profile. Its name may already exist.</Alert> : null}
            </>
          )}
        </Stack>
      </CardContent>
    </Card>
  );
}

function matchTypeLabel(value: ProfileSuggestion['matchType']) {
  if (value === 'assigned_asset') return 'Assigned asset profile';
  if (value === 'assigned_path') return 'Inherited path profile';
  if (value === 'existing') return 'Existing match';
  return 'New profile proposed';
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
      return {
        title: `Progressive${confidence}`,
        detail: analysis.fieldOrderMismatch
          ? `Sampled frames are progressive, but the container declares ${(analysis.containerFieldOrder || 'interlaced').toUpperCase()}. Correct field metadata to progressive without deinterlacing${window}.`
          : `No deinterlacing is recommended${window}.`,
        severity: analysis.fieldOrderMismatch ? 'warning' : 'success',
        action: analysis.fieldOrderMismatch ? 'Apply progressive metadata' : 'Apply: no deinterlace',
      };
    case 'interlaced':
      return { title: `Interlaced${confidence}`, detail: `${analysis.recommendedFilter ? `Recommended filter: ${analysis.recommendedFilter}.` : 'Deinterlacing is recommended.'}${window}`, severity: 'warning', action: 'Apply bwdif' };
    case 'mixed':
    case 'hybrid':
      return { title: `Hybrid progressive/interlaced${confidence}`, detail: `${analysis.decisionReason || 'Distributed regions contain different frame evidence.'} Auto will not apply a destructive motion filter${window}.`, severity: 'warning', action: 'Mark for review' };
    case 'telecine':
      return {
        title: `Telecine validated${confidence}`,
        detail: `Distributed cadence validation recommends ${analysis.recommendedFilter || 'inverse telecine'}${window}.`,
        severity: 'warning',
        action: analysis.recommendedMode ? `Apply ${analysis.recommendedMode.toUpperCase()}` : 'Review in LAB',
      };
    case 'telecine_suspected':
      return {
        title: `Telecine suspected${confidence}`,
        detail: analysis.recommendedFilter
          ? `${analysis.fieldOrderMismatch ? `Container field order ${(analysis.containerFieldOrder || 'unknown').toUpperCase()} conflicts with detected ${(analysis.detectedFieldOrder || 'unknown').toUpperCase()}. ` : ''}Recommended filter: ${analysis.recommendedFilter}${window}.`
          : `Validate cadence in LAB before choosing deinterlacing or IVTC${window}.`,
        severity: 'warning',
        action: analysis.recommendedMode && analysis.recommendedFilter ? `Apply ${analysis.recommendedMode.toUpperCase()}` : 'Mark for review',
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
