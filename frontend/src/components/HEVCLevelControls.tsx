import { Alert, Box, Grid, MenuItem, Stack, TextField, Typography } from '@mui/material';
import type { HEVCLevelRecommendation } from '../api/types';

const hevcLevels = ['1.0', '2.0', '2.1', '3.0', '3.1', '4.0', '4.1', '5.0', '5.1', '5.2', '6.0', '6.1', '6.2'] as const;

type HEVCLevelMode = 'auto' | 'recommended' | 'custom';

type Props = {
  config: Record<string, unknown>;
  recommendation?: HEVCLevelRecommendation;
  onChange: (key: string, value: unknown) => void;
  onChangeMany?: (patch: Record<string, unknown>) => void;
  encoder: string;
  disabled?: boolean;
  compact?: boolean;
};

export function HEVCLevelControls({ config, recommendation, onChange, onChangeMany, encoder, disabled = false, compact = false }: Props) {
  const configuredMode = String(config.hevcLevelMode ?? 'auto');
  const mode: HEVCLevelMode = configuredMode === 'recommended' || configuredMode === 'custom' ? configuredMode : 'auto';
  const recommendedLevel = recommendation?.recommendedLevel;
  const configuredLevel = normalizeHEVCLevel(config.hevcLevel) ?? recommendedLevel ?? '4.0';
  const selectedLevel = mode === 'recommended' && recommendedLevel ? recommendedLevel : configuredLevel;
  const sourceLevel = recommendation?.sourceLevel;
  const explicitLevelSupported = encoder === 'libx265' || encoder === 'hevc_qsv';
  const commit = (patch: Record<string, unknown>) => {
    if (onChangeMany) onChangeMany(patch);
    else Object.entries(patch).forEach(([key, value]) => onChange(key, value));
  };

  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: compact ? 1.5 : 2, bgcolor: 'rgba(255,255,255,0.018)' }}>
      <Stack spacing={1.5}>
        <Stack spacing={0.25}>
          <Typography fontWeight={700}>HEVC Level</Typography>
          <Typography variant="body2" color="text.secondary">
            Controls the decoder capability signaled by the output. MVForge calculates the minimum Main Tier level from resolution, frame rate, and available bitrate evidence.
          </Typography>
        </Stack>
        <Grid container spacing={1.5}>
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <TextField
              select
              label="Level mode"
              value={mode}
              disabled={disabled || !explicitLevelSupported}
              onChange={(event) => {
                const next = event.target.value as HEVCLevelMode;
                commit({ hevcLevelMode: next, ...(next === 'recommended' && recommendedLevel ? { hevcLevel: recommendedLevel } : {}) });
              }}
              helperText={mode === 'auto' ? 'Encoder selects and signals the Level.' : mode === 'recommended' ? 'Minimum appropriate Level calculated from this asset and verified on output.' : 'Explicit stream constraint.'}
              size={compact ? 'small' : 'medium'}
              fullWidth
            >
              <MenuItem value="auto">Auto · encoder decides</MenuItem>
              <MenuItem value="recommended" disabled={!recommendedLevel}>Recommended · MVForge</MenuItem>
              <MenuItem value="custom">Custom</MenuItem>
            </TextField>
          </Grid>
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <TextField
              select
              label={mode === 'recommended' ? 'Recommended Level' : 'HEVC Level'}
              value={selectedLevel}
              disabled={disabled || !explicitLevelSupported || mode !== 'custom'}
              onChange={(event) => onChange('hevcLevel', event.target.value)}
              helperText={mode === 'auto' ? 'No Level is sent to the encoder.' : `Main Tier · Level ${selectedLevel}`}
              size={compact ? 'small' : 'medium'}
              fullWidth
            >
              {hevcLevels.map((level) => <MenuItem key={level} value={level}>{level}</MenuItem>)}
            </TextField>
          </Grid>
          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
            <TextField
              label="Snapshot evidence"
              value={recommendation ? `${recommendation.width}×${recommendation.height} · ${recommendation.fps.toFixed(3)} fps` : 'Snapshot unavailable'}
              helperText={recommendation ? `Source ${sourceLevel ? `Level ${sourceLevel}` : 'Level unknown'} · limiting factor: ${friendlyFactor(recommendation.limitingFactor)}` : 'Run Snapshot to calculate Recommended.'}
              disabled
              size={compact ? 'small' : 'medium'}
              fullWidth
            />
          </Grid>
        </Grid>
        {mode === 'recommended' && recommendation?.warnings?.map((warning) => <Alert key={warning} severity="warning">{warning}</Alert>)}
        {!explicitLevelSupported ? <Alert severity="info">The active encoder does not have a validated explicit-Level mapping in MVForge. Snapshot still reports the recommended and observed Levels, but Auto remains effective.</Alert> : null}
        {mode === 'custom' ? <Alert severity="info">Custom Level is an explicit constraint. Process Video must confirm that the encoder honored it and that the output still fits the selected Level.</Alert> : null}
      </Stack>
    </Box>
  );
}

function normalizeHEVCLevel(value: unknown) {
  const text = String(value ?? '').trim();
  const normalized = text.includes('.') ? text : `${text}.0`;
  return hevcLevels.includes(normalized as typeof hevcLevels[number]) ? normalized : undefined;
}

function friendlyFactor(value?: string) {
  if (value === 'sample_rate') return 'samples per second';
  if (value === 'picture_size') return 'picture size';
  if (value === 'bitrate') return 'bitrate';
  return 'available stream constraints';
}
