import { Alert, Stack, Typography } from '@mui/material';
import type { BPPPFAssessment, SourceBitrateEvidence } from '../api/types';

function bitrateLabel(value?: number) {
  return value && value > 0 ? `${(value / 1_000_000).toFixed(2)} Mbps` : 'unavailable';
}

function severityForAssessment(status: string): 'info' | 'success' | 'warning' {
  if (status === 'normal') return 'success';
  if (status === 'warning' || status === 'high-risk') return 'warning';
  return 'info';
}

export function SourceBitrateQualityEvidence({
  source,
  assessment,
}: {
  source?: SourceBitrateEvidence;
  assessment?: BPPPFAssessment;
}) {
  if (!source && !assessment) return null;
  const method = source?.method.replaceAll('_', ' ') ?? '';

  return (
    <Stack spacing={1}>
      {source ? (
        <Alert severity="info" variant="outlined">
          <Stack spacing={0.25}>
            <Typography variant="body2" fontWeight={700}>
              Source video bitrate: {bitrateLabel(source.bitrate)}
            </Typography>
            <Typography variant="body2">
              Evidence: {method || 'unavailable'} · {source.confidence} confidence
              {source.streamIndex != null ? ` · stream ${source.streamIndex}` : ''}
            </Typography>
            {source.minimumBitrate != null || source.maximumBitrate != null ? (
              <Typography variant="body2">
                Sample range: {bitrateLabel(source.minimumBitrate)}–{bitrateLabel(source.maximumBitrate)}
              </Typography>
            ) : null}
            {source.requestedWindows > 0 ? (
              <Typography variant="body2">
                Packet windows succeeded: {source.successfulWindows}/{source.requestedWindows}
              </Typography>
            ) : source.detail ? <Typography variant="body2">{source.detail}</Typography> : null}
          </Stack>
        </Alert>
      ) : null}
      {assessment ? (
        <Alert severity={severityForAssessment(assessment.status)} variant="outlined">
          <Stack spacing={0.25}>
            <Typography variant="body2" fontWeight={700}>
              BPPPF planning rail: {assessment.status}
              {assessment.bpppf != null ? ` · ${assessment.bpppf.toFixed(4)} bits/pixel/frame` : ''}
            </Typography>
            {assessment.referenceBpppf != null && assessment.referenceBpppf > 0 ? (
              <Typography variant="body2">
                Reference {assessment.referenceBpppf.toFixed(4)} BPPPF · target {bitrateLabel(assessment.targetBitrate)}
              </Typography>
            ) : null}
            <Typography variant="body2">{assessment.explanation}</Typography>
            <Typography variant="caption" color="text.secondary">
              Planning heuristic only. Measured LAB evidence remains authoritative.
            </Typography>
          </Stack>
        </Alert>
      ) : null}
    </Stack>
  );
}
