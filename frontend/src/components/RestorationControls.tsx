import { Grid, MenuItem, Stack, TextField, Typography } from '@mui/material';
import type { RestorationConfig } from '../utils/restorationFilters';

const standardOptions = [
  { value: 'off', label: 'Off' },
  { value: 'light', label: 'Light' },
  { value: 'medium', label: 'Medium' },
  { value: 'strong', label: 'Strong' },
  { value: 'custom', label: 'Custom' },
] as const;

const denoiseOptions = [
  { value: 'off', label: 'Off' },
  { value: 'film-grain', label: 'Preserve grain · very light' },
  { value: 'film-restore', label: 'Film restoration · gentle' },
  { value: 'light', label: 'Light' },
  { value: 'medium', label: 'Medium' },
  { value: 'strong', label: 'Strong' },
  { value: 'custom', label: 'Custom HQDN3D' },
] as const;

export function RestorationControls({
  config,
  onChange,
  disabled = false,
  title,
}: {
  config: RestorationConfig;
  onChange: (patch: RestorationConfig) => void;
  disabled?: boolean;
  title?: string;
}) {
  const mode = (key: string) => typeof config[key] === 'string' ? config[key] as string : 'off';
  const number = (key: string, fallback: number) => {
    const value = config[key];
    const parsed = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : NaN;
    return Number.isFinite(parsed) ? parsed : fallback;
  };
  const changeMode = (key: string, value: string) => {
    const patch: RestorationConfig = { [key]: value };
    if (value === 'custom' && key === 'deblockFilter') {
      if (config.deblockCustomFilter === undefined) patch.deblockCustomFilter = 'strong';
      if (config.deblockCustomBlockSize === undefined) patch.deblockCustomBlockSize = 8;
    }
    if (value === 'custom' && key === 'denoise') {
      if (config.hqdn3dLumaSpatial === undefined) patch.hqdn3dLumaSpatial = 4;
      if (config.hqdn3dChromaSpatial === undefined) patch.hqdn3dChromaSpatial = 3;
      if (config.hqdn3dLumaTemporal === undefined) patch.hqdn3dLumaTemporal = 6;
      if (config.hqdn3dChromaTemporal === undefined) patch.hqdn3dChromaTemporal = 4.5;
    }
    if (value === 'custom' && key === 'chromaNR') {
      if (config.chromaNRThreshold === undefined) patch.chromaNRThreshold = 25;
      if (config.chromaNRWindowWidth === undefined) patch.chromaNRWindowWidth = 3;
      if (config.chromaNRWindowHeight === undefined) patch.chromaNRWindowHeight = 3;
    }
    if (value === 'custom' && key === 'deband' && config.debandThreshold === undefined) patch.debandThreshold = 0.024;
    onChange(patch);
  };
  const numericField = (label: string, key: string, fallback: number, min: number, max: number, step: number) => (
    <Grid size={{ xs: 12, sm: 6, md: 3 }}>
      <TextField
        label={label}
        type="number"
        value={number(key, fallback)}
        onChange={(event) => onChange({ [key]: Number(event.target.value) })}
        inputProps={{ min, max, step }}
        disabled={disabled}
        size="small"
        fullWidth
      />
    </Grid>
  );

  return (
    <Stack spacing={1.5}>
      {title ? <Typography fontWeight={700} variant="body2">{title}</Typography> : null}
      <Grid container spacing={1.5}>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <TextField label="Deblock" value={mode('deblockFilter')} onChange={(event) => changeMode('deblockFilter', event.target.value)} disabled={disabled} select fullWidth>
            {standardOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}
          </TextField>
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <TextField label="HQDN3D" value={mode('denoise')} onChange={(event) => changeMode('denoise', event.target.value)} disabled={disabled} select fullWidth>
            {denoiseOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}
          </TextField>
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <TextField label="Chroma NR" value={mode('chromaNR')} onChange={(event) => changeMode('chromaNR', event.target.value)} disabled={disabled} select fullWidth>
            {standardOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}
          </TextField>
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <TextField label="Deband" value={mode('deband')} onChange={(event) => changeMode('deband', event.target.value)} disabled={disabled} select fullWidth>
            {standardOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}
          </TextField>
        </Grid>

        {mode('deblockFilter') === 'custom' ? (
          <>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <TextField label="Deblock filter" value={mode('deblockCustomFilter') === 'weak' ? 'weak' : 'strong'} onChange={(event) => onChange({ deblockCustomFilter: event.target.value })} disabled={disabled} select size="small" fullWidth>
                <MenuItem value="weak">Weak</MenuItem>
                <MenuItem value="strong">Strong</MenuItem>
              </TextField>
            </Grid>
            {numericField('Block size', 'deblockCustomBlockSize', 8, 4, 64, 1)}
          </>
        ) : null}
        {mode('denoise') === 'custom' ? (
          <>
            {numericField('Luma spatial', 'hqdn3dLumaSpatial', 4, 0, 100, 0.1)}
            {numericField('Chroma spatial', 'hqdn3dChromaSpatial', 3, 0, 100, 0.1)}
            {numericField('Luma temporal', 'hqdn3dLumaTemporal', 6, 0, 100, 0.1)}
            {numericField('Chroma temporal', 'hqdn3dChromaTemporal', 4.5, 0, 100, 0.1)}
          </>
        ) : null}
        {mode('chromaNR') === 'custom' ? (
          <>
            {numericField('Chroma threshold', 'chromaNRThreshold', 25, 1, 100, 0.1)}
            {numericField('Window width', 'chromaNRWindowWidth', 3, 1, 99, 2)}
            {numericField('Window height', 'chromaNRWindowHeight', 3, 1, 99, 2)}
          </>
        ) : null}
        {mode('deband') === 'custom' ? numericField('Deband threshold', 'debandThreshold', 0.024, 0.001, 1, 0.001) : null}
      </Grid>
    </Stack>
  );
}
