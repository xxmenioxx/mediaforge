import { Alert, Box, Chip, Grid, Stack, Typography } from '@mui/material';
import type { ResolvedUpscaleDecision, UpscaleMode, UpscaleSharpen } from '../api/types';
import { upscaleEvidenceLabel, upscaleModeLabel, upscaleSharpenLabel } from '../upscale';

export function SmartUpscaleDecision({ decision, requestedMode, requestedSharpen, storageWidth, storageHeight }: {
  decision?: ResolvedUpscaleDecision;
  requestedMode?: UpscaleMode;
  requestedSharpen?: UpscaleSharpen;
  storageWidth?: number;
  storageHeight?: number;
}) {
  if (!decision) return <Alert severity="info">Process Video to resolve Smart Upscale for the current asset and profile.</Alert>;
  const storageGeometry = geometry(storageWidth, storageHeight);
  const effectiveGeometry = geometry(decision.sourceWidth, decision.sourceHeight);
  const outputGeometry = decision.upscaleApplied ? geometry(decision.targetWidth, decision.targetHeight) : 'Keep Source';
  const showEffective = Boolean(effectiveGeometry && effectiveGeometry !== storageGeometry);
  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
      <Stack spacing={1.25}>
        <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
          <Typography fontWeight={700}>Smart Upscale</Typography>
          <Chip size="small" label={`Confidence · ${confidenceLabel(decision.confidence)}`} />
        </Stack>
        <Grid container spacing={1.25}>
          <DecisionValue label="Requested" value={upscaleModeLabel(requestedMode ?? decision.requestedMode)} />
          <DecisionValue label="Requested sharpen" value={upscaleSharpenLabel(requestedSharpen ?? decision.sharpenMode)} />
          <DecisionValue label="Resolved" value={upscaleModeLabel(decision.resolvedMode)} />
          {storageGeometry ? <DecisionValue label="Source storage geometry" value={storageGeometry} /> : null}
          {showEffective ? <DecisionValue label="Effective geometry after crop" value={effectiveGeometry} /> : null}
          <DecisionValue label="Resolved output" value={outputGeometry} />
          <DecisionValue label="Pixel aspect" value={decision.targetSAR || (decision.upscaleApplied ? '1:1' : decision.sourceSAR) || 'Unknown'} />
          <DecisionValue label="Sharpen" value={decision.sharpenMode === 'custom' ? `Custom · ${decision.sharpenStrength?.toFixed(2) ?? 'Unknown'}` : upscaleSharpenLabel(decision.sharpenMode)} />
        </Grid>
        {decision.reasons?.length ? <Stack spacing={0.35}><Typography variant="caption" color="text.secondary">Why</Typography>{decision.reasons.map((reason) => <Typography key={reason} variant="body2">✓ {upscaleEvidenceLabel(reason)}</Typography>)}</Stack> : null}
        {decision.warnings?.length ? <Alert severity="warning"><Stack spacing={0.35}>{decision.warnings.map((warning) => <Typography key={warning} variant="body2">• {upscaleEvidenceLabel(warning)}</Typography>)}</Stack></Alert> : null}
      </Stack>
    </Box>
  );
}

function DecisionValue({ label, value }: { label: string; value: string }) {
  return <Grid size={{ xs: 12, sm: 6, md: 4 }}><Typography variant="caption" color="text.secondary">{label}</Typography><Typography>{value}</Typography></Grid>;
}

function geometry(width?: number, height?: number) {
  return width && height ? `${width}×${height}` : '';
}

function confidenceLabel(value: ResolvedUpscaleDecision['confidence']) {
  return value === 'unavailable' ? 'Unknown' : value.charAt(0).toUpperCase() + value.slice(1);
}
