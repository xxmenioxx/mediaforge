import { Box, Chip, Stack, Typography } from '@mui/material';
import type { RestorationAnalysis, RestorationSignalEvidence } from '../api/types';

const signalRows: Array<{ key: keyof Pick<RestorationAnalysis, 'blocking' | 'noise' | 'grain' | 'chromaNoise' | 'banding' | 'ringing' | 'edgeDetailConfidence'>; label: string }> = [
  { key: 'blocking', label: 'Blocking' },
  { key: 'noise', label: 'Noise' },
  { key: 'grain', label: 'Grain' },
  { key: 'chromaNoise', label: 'Chroma noise' },
  { key: 'banding', label: 'Banding' },
  { key: 'ringing', label: 'Ringing' },
  { key: 'edgeDetailConfidence', label: 'Edge/detail confidence' },
];

export function RestorationEvidencePanel({ analysis }: { analysis?: RestorationAnalysis }) {
  if (!analysis) return null;
  return (
    <Stack spacing={1.25}>
      <Stack spacing={0.35}>
        <Typography fontWeight={700}>Restoration evidence</Typography>
        <Typography variant="body2" color="text.secondary">
          Sampled source diagnostics only. These signals do not enable restoration filters or represent Preview PSNR/SSIM.
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {analysis.windows} sampled region(s) · {analysis.sampledFrames} measured frame(s) · {analysis.source}
        </Typography>
      </Stack>
      {signalRows.map(({ key, label }) => <EvidenceRow key={key} label={label} evidence={analysis[key]} />)}
    </Stack>
  );
}

function EvidenceRow({ label, evidence }: { label: string; evidence: RestorationSignalEvidence }) {
  const value = evidence.value === undefined ? 'No canonical value' : evidence.value.toFixed(6);
  const color = evidence.availability === 'available' ? 'success' : evidence.availability === 'ambiguous' ? 'warning' : 'default';
  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.25 }}>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={0.75} alignItems={{ xs: 'flex-start', sm: 'center' }}>
        <Typography fontWeight={700} sx={{ minWidth: 150 }}>{label}</Typography>
        <Chip size="small" color={color} variant="outlined" label={humanize(evidence.availability)} />
        <Chip size="small" variant="outlined" label={`Confidence: ${humanize(evidence.confidence)}`} />
        <Typography variant="body2">{value} · Severity: {humanize(evidence.severity)}</Typography>
      </Stack>
      {(evidence.supportingEvidence ?? []).map((item) => <Typography key={item} variant="caption" color="text.secondary" display="block">{item}</Typography>)}
    </Box>
  );
}

function humanize(value: string) {
  return value.replaceAll('_', ' ').replace(/^./, (character) => character.toUpperCase());
}
