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

function severityForConfidence(confidence: string): 'info' | 'success' | 'warning' {
  if (confidence === 'high') return 'success';
  if (confidence === 'low' || confidence === 'unavailable') return 'warning';
  return 'info';
}

function readableLabel(value: string) {
  const label = value.replaceAll('_', ' ').replaceAll('-', ' ');
  return label.charAt(0).toUpperCase() + label.slice(1);
}

function sourceInterpretation(confidence: string, method: string) {
  if (confidence === 'low') return 'This source bitrate estimate is uncertain; treat it as a guide.';
  if (confidence === 'unavailable') return 'A reliable source bitrate could not be established.';
  if (method === 'packet_sampling' && (confidence === 'high' || confidence === 'medium')) {
    return 'The source bitrate estimate is based on successful sampled windows.';
  }
  if (confidence === 'high') return 'MVForge has high confidence in this source bitrate estimate.';
  if (confidence === 'medium') return 'MVForge has moderate confidence in this source bitrate estimate.';
  return `The source bitrate estimate has ${readableLabel(confidence)} confidence.`;
}

function assessmentStatusLabel(status: string) {
  if (status === 'normal') return 'Normal';
  if (status === 'high-risk') return 'High risk';
  if (status === 'warning') return 'Review recommended';
  if (status === 'unavailable' || !status) return 'Unavailable';
  return readableLabel(status);
}

function assessmentSummary(assessment: BPPPFAssessment) {
  const intent = assessment.optimizationIntent
    ? ` for the selected ${readableLabel(assessment.optimizationIntent)} quality goal`
    : '';

  if (!assessment.targetBitrate || assessment.targetBitrate <= 0) {
    return `MVForge could not provide an estimated bitrate${intent}.`;
  }

  const dimensions = assessment.outputWidth && assessment.outputHeight
    ? `${assessment.outputWidth}×${assessment.outputHeight}`
    : undefined;
  const videoContext = dimensions
    ? `${dimensions} video${assessment.effectiveFps ? ` at ${assessment.effectiveFps} fps` : ''}`
    : assessment.effectiveFps ? `video at ${assessment.effectiveFps} fps` : 'video';
  const context = videoContext ? `For this ${videoContext}, ` : '';

  return `${context}MVForge estimates around ${bitrateLabel(assessment.targetBitrate)} as a starting point${intent}.`;
}

function technicalExplanation(explanation: string) {
  return explanation
    .replace(' This is heuristic planning context; measured LAB evidence remains authoritative.', '')
    .trim();
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
        <Alert severity={severityForConfidence(source.confidence)} variant="outlined">
          <Stack spacing={0.25}>
            <Typography variant="body2" fontWeight={700}>
              Source video bitrate: {bitrateLabel(source.bitrate)}
            </Typography>
            <Typography variant="body2">{sourceInterpretation(source.confidence, source.method)}</Typography>
            {source.minimumBitrate != null || source.maximumBitrate != null ? (
              <Typography variant="body2">
                Measured range: {bitrateLabel(source.minimumBitrate)}–{bitrateLabel(source.maximumBitrate)}
              </Typography>
            ) : null}
            {source.requestedWindows > 0 ? (
              <Typography variant="body2">
                Samples analyzed: {source.successfulWindows} of {source.requestedWindows}
              </Typography>
            ) : source.detail ? <Typography variant="body2">{source.detail}</Typography> : null}
            <Typography variant="body2">Confidence: {readableLabel(source.confidence)}</Typography>
            {source.streamIndex != null ? (
              <Typography variant="body2">Video stream: {source.streamIndex}</Typography>
            ) : null}
            <details>
              <Typography component="summary" variant="caption" color="primary" sx={{ cursor: 'pointer', fontWeight: 600 }}>
                Sampling details
              </Typography>
              <Stack spacing={0.25} sx={{ mt: 0.5 }}>
                <Typography variant="caption">
                  Evidence method: {method ? readableLabel(method) : 'Unavailable'}
                </Typography>
                <Typography variant="caption">Evidence schema version: {source.schemaVersion}</Typography>
                <Typography variant="caption">Sampling policy version: {source.samplingPolicyVersion}</Typography>
              </Stack>
            </details>
          </Stack>
        </Alert>
      ) : null}
      {assessment ? (
        <Alert severity={severityForAssessment(assessment.status)} variant="outlined">
          <Stack spacing={0.25}>
            <Typography variant="body2" fontWeight={700}>
              Encoding estimate: {assessmentStatusLabel(assessment.status)}
            </Typography>
            <Typography variant="body2">{assessmentSummary(assessment)}</Typography>
            <Typography variant="caption" color="text.secondary">
              Final quality should be confirmed with Test Encode / LAB results.
            </Typography>
            <details>
              <Typography component="summary" variant="caption" color="primary" sx={{ cursor: 'pointer', fontWeight: 600 }}>
                Technical reference
              </Typography>
              <Stack spacing={0.25} sx={{ mt: 0.5 }}>
                {assessment.bpppf != null ? (
                  <Typography variant="caption">Estimated density: {assessment.bpppf.toFixed(4)} BPPPF</Typography>
                ) : null}
                {assessment.referenceBpppf != null && assessment.referenceBpppf > 0 ? (
                  <Typography variant="caption">Expected reference: {assessment.referenceBpppf.toFixed(4)} BPPPF</Typography>
                ) : null}
                {assessment.referenceBitrate != null ? (
                  <Typography variant="caption">Reference bitrate: {bitrateLabel(assessment.referenceBitrate)}</Typography>
                ) : null}
                <Typography variant="caption">
                  BPPPF policy version: {assessment.policyVersion} · encoder model version: {assessment.encoderModelVersion}
                </Typography>
                <Typography variant="caption">
                  Reference settings: {[assessment.codecFamily, assessment.encoderClass, assessment.dynamicRange].filter(Boolean).join(' / ') || 'Unavailable'}
                </Typography>
                {assessment.encoder ? <Typography variant="caption">Encoder: {assessment.encoder}</Typography> : null}
                {assessment.bitrateBasis ? <Typography variant="caption">Bitrate basis: {readableLabel(assessment.bitrateBasis)}</Typography> : null}
                {assessment.explanation ? (
                  <Typography variant="caption">Technical explanation: {technicalExplanation(assessment.explanation)}</Typography>
                ) : null}
              </Stack>
            </details>
          </Stack>
        </Alert>
      ) : null}
    </Stack>
  );
}
