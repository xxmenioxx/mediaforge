import { Alert, Box, Grid, MenuItem, Stack, TextField, Typography } from '@mui/material';

export type GOPMode = 'auto' | 'recommended' | 'custom';
export type BFrameMode = 'auto' | 'recommended' | 'custom' | 'off';

type Props = {
  config: Record<string, unknown>;
  recommendedGop?: number;
  recommendedBFrames?: number;
  onChange: (key: string, value: unknown) => void;
  onChangeMany?: (patch: Record<string, unknown>) => void;
  encoder?: string;
  disabled?: boolean;
  compact?: boolean;
};

export function FrameStructureControls({ config, recommendedGop, recommendedBFrames, onChange, onChangeMany, encoder = '', disabled = false, compact = false }: Props) {
  const gopMode = mode<GOPMode>(config.frameStructureGopMode, ['auto', 'recommended', 'custom'], 'auto');
  const bFrameMode = mode<BFrameMode>(config.frameStructureBFrameMode, ['auto', 'recommended', 'custom', 'off'], 'auto');
  const gop = positiveInt(config.frameStructureGopFrames, recommendedGop || 120);
  const bFrames = boundedInt(config.frameStructureMaxBFrames, recommendedBFrames || 3, 1, 16);
  const commit = (patch: Record<string, unknown>) => {
    if (onChangeMany) onChangeMany(patch);
    else Object.entries(patch).forEach(([key, value]) => onChange(key, value));
  };
  const x265Params = parseX265Params(String(config.x265Params ?? ''));
  const updateX265Param = (key: string, value: string) => {
    const next = { ...x265Params };
    if (value.trim()) next[key] = value.trim();
    else delete next[key];
    commit({ x265Params: Object.entries(next).map(([name, setting]) => `${name}=${setting}`).join(':') });
  };

  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: compact ? 1.5 : 2, bgcolor: 'rgba(255,255,255,0.018)' }}>
      <Stack spacing={1.5}>
        <Stack spacing={0.25}>
          <Typography fontWeight={700}>Frame Structure</Typography>
          <Typography variant="body2" color="text.secondary">Simple common policy. Encoder-specific tuning remains under Advanced settings.</Typography>
        </Stack>
        <Grid container spacing={1.5}>
          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
            <TextField select label="GOP" value={gopMode} disabled={disabled} onChange={(event) => {
              const next = event.target.value as GOPMode;
              const params = { ...x265Params };
              if (next !== 'custom') delete params.scenecut;
              commit({ frameStructureGopMode: next, ...(next === 'recommended' && recommendedGop ? { frameStructureGopFrames: recommendedGop } : {}), ...(encoder === 'libx265' ? { x265Params: serializeX265Params(params) } : {}) });
            }} size={compact ? 'small' : 'medium'} fullWidth>
              <MenuItem value="auto">Auto</MenuItem>
              <MenuItem value="recommended" disabled={!recommendedGop}>Recommended</MenuItem>
              <MenuItem value="custom">Custom</MenuItem>
            </TextField>
          </Grid>
          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
            <TextField label={gopMode === 'recommended' ? 'Recommended GOP' : 'Custom GOP'} type="number" value={gop} disabled={disabled || gopMode === 'auto' || gopMode === 'recommended'} onChange={(event) => commit({ frameStructureGopFrames: Math.max(1, Math.min(1000, Number(event.target.value))) })} helperText={gopMode === 'auto' ? 'Encoder decides' : gopMode === 'recommended' ? `MVForge analysis: ${recommendedGop}` : 'Frames between keyframes'} inputProps={{ min: 1, max: 1000 }} size={compact ? 'small' : 'medium'} fullWidth />
          </Grid>
          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
            <TextField select label="B-frames" value={bFrameMode} disabled={disabled} onChange={(event) => {
              const next = event.target.value as BFrameMode;
              const params = { ...x265Params };
              delete params.bframes;
              if (next !== 'custom') {
                delete params['b-adapt'];
                delete params['b-pyramid'];
              }
              commit({
                frameStructureBFrameMode: next,
                ...(next === 'recommended' && recommendedBFrames ? { frameStructureMaxBFrames: recommendedBFrames } : {}),
                ...(next === 'off' ? { frameStructureMaxBFrames: 0, qsvAdaptiveB: false } : {}),
                ...(encoder === 'libx265' ? { x265Params: serializeX265Params(params) } : {}),
              });
            }} size={compact ? 'small' : 'medium'} fullWidth>
              <MenuItem value="auto">Auto</MenuItem>
              <MenuItem value="recommended" disabled={!recommendedBFrames}>Recommended</MenuItem>
              <MenuItem value="custom">Custom</MenuItem>
              <MenuItem value="off">Off</MenuItem>
            </TextField>
          </Grid>
          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
            <TextField label={bFrameMode === 'recommended' ? 'Recommended maximum' : 'Maximum B-frames'} type="number" value={bFrames} disabled={disabled || bFrameMode === 'auto' || bFrameMode === 'recommended' || bFrameMode === 'off'} onChange={(event) => commit({ frameStructureMaxBFrames: Math.max(1, Math.min(16, Number(event.target.value))) })} helperText={bFrameMode === 'auto' ? 'Encoder decides' : bFrameMode === 'off' ? 'Effective request: -bf 0' : bFrameMode === 'recommended' ? `MVForge analysis: ${recommendedBFrames}` : 'User-selected maximum'} inputProps={{ min: 1, max: 16 }} size={compact ? 'small' : 'medium'} fullWidth />
          </Grid>
        </Grid>
        {encoder === 'libx265' && (gopMode === 'custom' || bFrameMode === 'custom') ? (
          <Box sx={{ borderTop: 1, borderColor: 'divider', pt: 1.5 }}>
            <Typography variant="body2" fontWeight={700} sx={{ mb: 1 }}>x265 controls for the selected Custom modes</Typography>
            <Grid container spacing={1.5}>
              {gopMode === 'custom' ? <Grid size={{ xs: 12, sm: 6, md: 4 }}><TextField label="Scenecut threshold" type="number" value={x265Params.scenecut ?? ''} onChange={(event) => updateX265Param('scenecut', event.target.value)} placeholder="Encoder default" helperText="0 disables scenecut; x265 commonly defaults to 40." inputProps={{ min: 0, max: 100 }} size={compact ? 'small' : 'medium'} fullWidth /></Grid> : null}
              {bFrameMode === 'custom' ? <Grid size={{ xs: 12, sm: 6, md: 4 }}><TextField select label="B-frame adaptation" value={x265Params['b-adapt'] ?? ''} onChange={(event) => updateX265Param('b-adapt', event.target.value)} helperText="How x265 selects B-frame placement." size={compact ? 'small' : 'medium'} fullWidth><MenuItem value="">Encoder default</MenuItem><MenuItem value="0">Off</MenuItem><MenuItem value="1">Fast</MenuItem><MenuItem value="2">Full</MenuItem></TextField></Grid> : null}
              {bFrameMode === 'custom' ? <Grid size={{ xs: 12, sm: 6, md: 4 }}><TextField select label="B-pyramid" value={x265Params['b-pyramid'] ?? ''} onChange={(event) => updateX265Param('b-pyramid', event.target.value)} helperText="Allows B-frames to reference other B-frames." size={compact ? 'small' : 'medium'} fullWidth><MenuItem value="">Encoder default</MenuItem><MenuItem value="1">Enabled</MenuItem><MenuItem value="0">Disabled</MenuItem></TextField></Grid> : null}
            </Grid>
          </Box>
        ) : null}
        {bFrameMode === 'off' ? <Alert severity="warning"><strong>B-frames disabled.</strong> The encoder will use I/P frame structures only. This may improve compatibility or help diagnose unusual frame structures, but may reduce compression efficiency and increase bitrate or file size at equivalent quality.</Alert> : null}
      </Stack>
    </Box>
  );
}

function parseX265Params(value: string) {
  return value.split(':').map((part) => part.trim()).filter(Boolean).reduce<Record<string, string>>((result, part) => {
    const separator = part.indexOf('=');
    result[separator < 0 ? part : part.slice(0, separator)] = separator < 0 ? '1' : part.slice(separator + 1);
    return result;
  }, {});
}

function serializeX265Params(params: Record<string, string>) {
  return Object.entries(params).filter(([, value]) => value.trim() !== '').map(([key, value]) => `${key}=${value}`).join(':');
}

function mode<T extends string>(value: unknown, allowed: T[], fallback: T): T {
  return typeof value === 'string' && allowed.includes(value as T) ? value as T : fallback;
}

function positiveInt(value: unknown, fallback: number) {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.round(parsed) : fallback;
}

function boundedInt(value: unknown, fallback: number, minimum: number, maximum: number) {
  return Math.max(minimum, Math.min(maximum, positiveInt(value, fallback)));
}
