import { Grid, MenuItem, Stack, TextField, Typography } from '@mui/material';
import type { ScanResult } from '../api/types';

export type FieldStructureMode = '' | 'preserve' | 'auto' | 'deinterlace';
export type CadenceMode = '' | 'preserve' | 'auto' | 'remove_soft_telecine' | 'inverse_telecine';
export type CadenceFieldOrder = '' | 'auto' | 'tff' | 'bff';
export type DeinterlaceFieldOrder = '' | 'auto' | 'tff' | 'bff';

type Props = {
  fieldStructureMode: string;
  cadenceMode: string;
  cadenceFieldOrder?: string;
  deinterlaceFieldOrder?: string;
  onFieldStructureChange: (value: FieldStructureMode) => void;
  onCadenceChange: (value: CadenceMode) => void;
  onCadenceFieldOrderChange?: (value: CadenceFieldOrder) => void;
  onDeinterlaceFieldOrderChange?: (value: DeinterlaceFieldOrder) => void;
  scan?: ScanResult;
  allowProfileDefault?: boolean;
  disabled?: boolean;
};

export function FrameCadenceControls({ fieldStructureMode, cadenceMode, cadenceFieldOrder = 'auto', deinterlaceFieldOrder = 'auto', onFieldStructureChange, onCadenceChange, onCadenceFieldOrderChange, onDeinterlaceFieldOrderChange, scan, allowProfileDefault = false, disabled = false }: Props) {
  const fieldOptions = [
    ...(allowProfileDefault ? [{ value: '', label: 'Profile default' }] : []),
    { value: 'preserve', label: 'Preserve' },
    { value: 'auto', label: 'Auto at conversion (uses Analysis)' },
    { value: 'deinterlace', label: 'Force deinterlace' },
  ];
  const cadenceOptions = [
    ...(allowProfileDefault ? [{ value: '', label: 'Profile default' }] : []),
    { value: 'preserve', label: 'Preserve' },
    { value: 'auto', label: 'Auto at conversion (uses Analysis)' },
    { value: 'remove_soft_telecine', label: 'Remove soft telecine' },
    { value: 'inverse_telecine', label: 'Inverse telecine' },
  ];
  const cadence = scan?.cadenceAnalysis;
  const recommendation = scan?.cadenceRecommendation;
  return <>
    <Grid size={{ xs: 12, md: 6 }}>
      <TextField select fullWidth size="small" label="Frame Structure" value={fieldStructureMode} disabled={disabled} onChange={(event) => onFieldStructureChange(event.target.value as FieldStructureMode)} helperText={scan ? `Detected: ${fieldStructureLabel(scan)}` : 'Controls preservation or deinterlacing; field order is resolved from Analysis.'}>
        {fieldOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}
      </TextField>
    </Grid>
    <Grid size={{ xs: 12, md: 6 }}>
      <Stack spacing={0.75}>
        <TextField select fullWidth size="small" label="Cadence" value={cadenceMode} disabled={disabled} onChange={(event) => onCadenceChange(event.target.value as CadenceMode)} helperText={cadence ? cadenceHelper(cadence.type, cadence.pattern, recommendation?.outputFrameRate) : 'Controls cadence preservation, soft-telecine timing removal, or inverse telecine.'}>
          {cadenceOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}
        </TextField>
        {cadence?.type === 'mixed' && cadenceMode === 'auto' ? <Typography variant="caption" color="warning.main">Mixed cadence detected. Automatic correction is disabled; review representative previews in LAB.</Typography> : null}
      </Stack>
    </Grid>
    {cadenceMode === 'inverse_telecine' && onCadenceFieldOrderChange ? <Grid size={{ xs: 12, md: 6 }}>
      <TextField select fullWidth size="small" label="Inverse telecine field order" value={cadenceFieldOrder || 'auto'} disabled={disabled} onChange={(event) => onCadenceFieldOrderChange(event.target.value as CadenceFieldOrder)} helperText="Advanced override. Auto uses the measured field order when available.">
        <MenuItem value="auto">Auto</MenuItem><MenuItem value="tff">TFF</MenuItem><MenuItem value="bff">BFF</MenuItem>
      </TextField>
    </Grid> : null}
    {onDeinterlaceFieldOrderChange ? <Grid size={{ xs: 12, md: 6 }}>
      <TextField select fullWidth size="small" label="Deinterlace field order" value={deinterlaceFieldOrder || (allowProfileDefault ? '' : 'auto')} disabled={disabled} onChange={(event) => onDeinterlaceFieldOrderChange(event.target.value as DeinterlaceFieldOrder)} helperText="Advanced override. It changes parity only when deinterlacing is already resolved; it never enables deinterlacing.">
        {allowProfileDefault ? <MenuItem value="">Profile default</MenuItem> : null}
        <MenuItem value="auto">Auto</MenuItem><MenuItem value="tff">TFF</MenuItem><MenuItem value="bff">BFF</MenuItem>
      </TextField>
    </Grid> : null}
  </>;
}

function fieldStructureLabel(scan: ScanResult) {
  const status = scan.interlaceAnalysis?.status ?? 'unknown';
  if (status === 'progressive') return 'Progressive';
  if (status === 'interlaced') return `Interlaced${scan.interlaceAnalysis.detectedFieldOrder ? ` · ${scan.interlaceAnalysis.detectedFieldOrder.toUpperCase()}` : ''}`;
  if (status === 'telecine' || status === 'telecine_suspected') return 'Field-based telecine';
  if (status === 'hybrid' || status === 'mixed') return 'Mixed · review';
  return 'Unknown';
}

function cadenceHelper(type?: string, pattern?: string, output?: string) {
  const detected = type === 'soft_telecine' ? `${pattern || '2:3'} soft telecine` : type?.replaceAll('_', ' ') || 'unknown';
  return `Detected: ${detected}${output ? ` · Recommended: ${output}p` : ''}`;
}
