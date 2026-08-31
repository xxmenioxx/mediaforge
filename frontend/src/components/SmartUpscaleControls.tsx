import { Grid, MenuItem, Stack, TextField, Typography } from '@mui/material';
import type { UpscaleMode, UpscaleSharpen } from '../api/types';
import { customUpscaleHeightError, customUpscaleSharpenError, upscaleModeOptions, upscaleSharpenOptions } from '../upscale';

export type SmartUpscaleControlValues = {
  mode?: UpscaleMode;
  sharpen?: UpscaleSharpen;
  customHeight?: number;
  customSharpenStrength?: number;
};

export function SmartUpscaleControls({
  value,
  onChange,
  allowInherit = false,
  disabled = false,
  size = 'small',
}: {
  value: SmartUpscaleControlValues;
  onChange: (patch: Partial<SmartUpscaleControlValues>) => void;
  allowInherit?: boolean;
  disabled?: boolean;
  size?: 'small' | 'medium';
}) {
  const mode = value.mode ?? (allowInherit ? undefined : 'disabled');
  const sharpen = value.sharpen ?? (allowInherit ? undefined : 'off');
  const heightError = customUpscaleHeightError(mode, value.customHeight);
  const sharpenError = customUpscaleSharpenError(sharpen, value.customSharpenStrength);
  return (
    <Stack spacing={1.5}>
      <Stack spacing={0.35}>
        <Typography fontWeight={700}>Smart Upscale</Typography>
        <Typography variant="body2" color="text.secondary">
          Upscaling improves presentation and resampling on modern displays, but it cannot restore detail that is not present in the source.
        </Typography>
      </Stack>
      <Grid container spacing={1.5}>
        <Grid size={{ xs: 12, md: 4 }}>
          <TextField select fullWidth size={size} label="Upscale" value={mode ?? ''} disabled={disabled} onChange={(event) => onChange({ mode: (event.target.value || undefined) as UpscaleMode | undefined })} helperText="Auto uses the backend-resolved frame structure, crop, aspect ratio, and available evidence.">
            {allowInherit ? <MenuItem value="">Inherit</MenuItem> : null}
            {upscaleModeOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}
          </TextField>
        </Grid>
        {mode === 'custom' ? <Grid size={{ xs: 12, md: 4 }}>
          <TextField required fullWidth size={size} type="number" label="Target height" value={value.customHeight ?? ''} disabled={disabled} error={Boolean(heightError)} helperText={heightError || 'Width is derived from the effective display aspect ratio.'} inputProps={{ min: 2, step: 2 }} onChange={(event) => onChange({ customHeight: event.target.value === '' ? undefined : Number(event.target.value) })} />
        </Grid> : null}
        <Grid size={{ xs: 12, md: 4 }}>
          <TextField select fullWidth size={size} label="Sharpen after upscale" value={sharpen ?? ''} disabled={disabled} onChange={(event) => {
            const next = (event.target.value || undefined) as UpscaleSharpen | undefined;
            onChange({ sharpen: next, ...(next === 'custom' ? { customSharpenStrength: value.customSharpenStrength ?? 0.16 } : { customSharpenStrength: undefined }) });
          }} helperText="Light is recommended for most sources.">
            {allowInherit ? <MenuItem value="">Inherit</MenuItem> : null}
            {upscaleSharpenOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}
          </TextField>
        </Grid>
        {sharpen === 'custom' ? <Grid size={{ xs: 12, md: 4 }}>
          <TextField required fullWidth size={size} type="number" label="CAS strength" value={value.customSharpenStrength ?? 0.16} disabled={disabled} error={Boolean(sharpenError)} helperText={sharpenError || 'Custom range 0.00–0.40.'} inputProps={{ min: 0, max: 0.4, step: 0.01 }} onChange={(event) => onChange({ customSharpenStrength: event.target.value === '' ? undefined : Number(event.target.value) })} />
        </Grid> : null}
      </Grid>
    </Stack>
  );
}
