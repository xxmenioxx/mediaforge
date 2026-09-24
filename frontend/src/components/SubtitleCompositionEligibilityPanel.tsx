import {
  Alert,
  Box,
  Button,
  Checkbox,
  FormControl,
  FormControlLabel,
  FormGroup,
  InputLabel,
  LinearProgress,
  MenuItem,
  Select,
  Stack,
  Typography,
} from '@mui/material';
import { useEffect, useMemo, useState } from 'react';
import type { SubtitleCompositionCandidate, SubtitleCompositionEligibility, SubtitleCompositionInput, SubtitleExtractionOperation } from '../api/types';

type SubtitleCompositionEligibilityPanelProps = {
  assetPath?: string;
  eligibility?: SubtitleCompositionEligibility;
  loading?: boolean;
  disabled?: boolean;
  operation?: SubtitleExtractionOperation | null;
  error?: string;
  onCompose?: (input: SubtitleCompositionInput) => void | Promise<void>;
  onCancel?: () => void;
  onRetry?: () => void;
};

function candidateLabel(candidate: SubtitleCompositionCandidate) {
  const source = candidate.sourcePath || candidate.sourceIdentity;
  return `${source} · ${candidate.language || 'unknown'} · ${candidate.role || 'unknown'} · ${candidate.compositionClass || 'unknown'}`;
}

function sourceRequest(candidate: SubtitleCompositionCandidate) {
  return {
    identity: candidate.sourceIdentity,
    revision: candidate.sourceRevision,
    representation: candidate.representation as 'srt' | 'ass' | 'ssa',
    language: candidate.language,
    role: candidate.role,
    cueClass: candidate.compositionClass,
    ...(candidate.roleProvenance ? { roleProvenance: candidate.roleProvenance } : {}),
    ...(candidate.roleConfidence ? { roleConfidence: candidate.roleConfidence } : {}),
    ...(candidate.roleClassifierVersion ? { roleClassifierVersion: candidate.roleClassifierVersion } : {}),
    ...(candidate.roleOverrideState ? { roleOverrideState: candidate.roleOverrideState } : {}),
    sourcePath: candidate.sourcePath || '',
    ...(candidate.canonicalOcrArtifactIdentity ? { canonicalOcrArtifactIdentity: candidate.canonicalOcrArtifactIdentity } : {}),
    ...(candidate.canonicalOcrArtifactRevision ? { canonicalOcrArtifactRevision: candidate.canonicalOcrArtifactRevision } : {}),
    ...(candidate.canonicalOcrArtifactPath ? { canonicalOcrArtifactPath: candidate.canonicalOcrArtifactPath } : {}),
    ...(candidate.canonicalOcrLogicalTransform ? { canonicalOcrLogicalTransform: candidate.canonicalOcrLogicalTransform } : {}),
  };
}

export function SubtitleCompositionEligibilityPanel({ assetPath = '', eligibility, loading = false, disabled = false, operation = null, error, onCompose, onCancel, onRetry }: SubtitleCompositionEligibilityPanelProps) {
  const primaryCandidates = eligibility?.primaryCandidates ?? [];
  const supplementalCandidates = eligibility?.supplementalCandidates ?? [];
  const firstEligiblePrimary = useMemo(() => primaryCandidates.find((candidate) => candidate.eligible)?.sourceIdentity ?? '', [primaryCandidates]);
  const [primaryIdentity, setPrimaryIdentity] = useState(firstEligiblePrimary);
  const [supplementalIdentities, setSupplementalIdentities] = useState<string[]>([]);

  useEffect(() => {
    setPrimaryIdentity(firstEligiblePrimary);
    setSupplementalIdentities([]);
  }, [firstEligiblePrimary, eligibility?.policyVersion]);

  if (loading) return <Typography variant="body2" color="text.secondary">Loading subtitle composition eligibility…</Typography>;
  if (!eligibility) return null;

  const primary = primaryCandidates.find((candidate) => candidate.sourceIdentity === primaryIdentity && candidate.eligible);
  const selectedSupplementals = supplementalCandidates.filter((candidate) => supplementalIdentities.includes(candidate.sourceIdentity) && candidate.eligible && candidate.sourceIdentity !== primaryIdentity);
  const canSubmit = Boolean(onCompose && eligibility.eligible && primary && selectedSupplementals.length > 0 && selectedSupplementals.length <= eligibility.maxSupplementals && !disabled && operation?.status !== 'running');
  const submit = () => {
    if (!canSubmit || !primary) return;
    const sources = [primary, ...selectedSupplementals];
    const observedRevisions = Object.fromEntries(sources.map((candidate) => [candidate.sourceIdentity, candidate.sourceRevision]));
    onCompose?.({
      path: assetPath,
      primary: sourceRequest(primary),
      supplementals: selectedSupplementals.map(sourceRequest),
      observedRevisions,
    });
  };

  return (
    <Stack spacing={0.75} sx={{ mt: 1 }}>
      <Typography fontWeight={700}>Subtitle composition</Typography>
      <Typography variant="body2" color="text.secondary">
        Backend policy {eligibility.policyVersion}. {eligibility.primaryCandidates.length} primary candidate(s), {eligibility.supplementalCandidates.length} supplemental candidate(s).
      </Typography>
      {!eligibility.eligible ? <Alert severity="info">{eligibility.reason || 'No compatible subtitle composition is currently eligible.'}</Alert> : null}
      {error ? <Alert severity="error">{error}</Alert> : null}
      {eligibility.eligible ? <>
        <FormControl size="small" fullWidth disabled={disabled || operation?.status === 'running'}>
          <InputLabel id="subtitle-composition-primary-label">Primary source</InputLabel>
          <Select labelId="subtitle-composition-primary-label" label="Primary source" value={primaryIdentity} onChange={(event) => {
            const next = String(event.target.value);
            setPrimaryIdentity(next);
            setSupplementalIdentities((current) => current.filter((identity) => identity !== next));
          }}>
            {primaryCandidates.map((candidate) => <MenuItem key={candidate.sourceIdentity} value={candidate.sourceIdentity} disabled={!candidate.eligible}>{candidateLabel(candidate)}{!candidate.eligible && candidate.reason ? ` — ${candidate.reason}` : ''}</MenuItem>)}
          </Select>
        </FormControl>
        <Typography variant="caption" color="text.secondary">Choose one backend-authorized primary and at least one supplemental (maximum {eligibility.maxSupplementals}).</Typography>
        <FormGroup>
          {supplementalCandidates.map((candidate) => {
            const checked = supplementalIdentities.includes(candidate.sourceIdentity) && candidate.sourceIdentity !== primaryIdentity;
            const atLimit = selectedSupplementals.length >= eligibility.maxSupplementals && !checked;
            return <FormControlLabel key={candidate.sourceIdentity} control={<Checkbox size="small" checked={checked} disabled={disabled || operation?.status === 'running' || !candidate.eligible || candidate.sourceIdentity === primaryIdentity || atLimit} onChange={(event) => {
              setSupplementalIdentities((current) => event.target.checked ? [...current.filter((identity) => identity !== candidate.sourceIdentity), candidate.sourceIdentity] : current.filter((identity) => identity !== candidate.sourceIdentity));
            }} />} label={<Typography variant="body2">{candidateLabel(candidate)}{!candidate.eligible && candidate.reason ? ` — ${candidate.reason}` : ''}</Typography>} />;
          })}
        </FormGroup>
        <Button size="small" variant="outlined" disabled={!canSubmit} onClick={submit}>Compose subtitles</Button>
      </> : null}
      {operation ? <Box sx={{ borderTop: 1, borderColor: 'divider', pt: 0.75 }}>
        {operation.status === 'running' ? <Stack spacing={0.5}>
          <Stack direction="row" justifyContent="space-between"><Typography variant="caption">{operation.message || operation.phase || 'Subtitle composition running'}</Typography><Typography variant="caption">{Math.round(operation.progress)}%</Typography></Stack>
          <LinearProgress variant="determinate" value={operation.progress} aria-label="Subtitle composition progress" />
          {onCancel ? <Button size="small" color="warning" variant="outlined" disabled={disabled} onClick={onCancel}>Cancel</Button> : null}
        </Stack> : <Stack direction="row" spacing={1} alignItems="center"><Alert severity={operation.status === 'completed' ? 'success' : operation.status === 'cancelled' ? 'info' : 'error'} sx={{ py: 0, flex: 1 }}>{operation.error || operation.message || (operation.status === 'completed' ? 'Subtitle composition completed.' : 'Subtitle composition failed.')}</Alert>{operation.retryable && onRetry ? <Button size="small" variant="outlined" onClick={onRetry}>Retry</Button> : null}</Stack>}
      </Box> : null}
    </Stack>
  );
}
