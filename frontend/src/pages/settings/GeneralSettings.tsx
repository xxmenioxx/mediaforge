import {
  Alert,
  Autocomplete,
  Button,
  Card,
  CardContent,
  Grid,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import type { MVForgePreferences } from '../../api/types';
import { defaultMVForgePreferences } from '../../mvforgePreferences';
import { normalizeTrackLanguageList, subtitleLanguageOptions, trackLanguageLabel } from '../../utils/subtitleLanguages';

export function GeneralSettingsDomain({ value, availableEncoders, onChange }: {
  value: MVForgePreferences;
  availableEncoders: string[];
  onChange: (value: MVForgePreferences) => void;
}) {
  return (
    <Grid container spacing={2}>
      <Grid size={{ xs: 12 }}>
        <MVForgePreferencesCard
          value={value}
          availableEncoders={availableEncoders}
          onChange={onChange}
        />
      </Grid>
    </Grid>
  );
}

function MVForgePreferencesCard({ value, availableEncoders, onChange }: {
  value: MVForgePreferences;
  availableEncoders: string[];
  onChange: (value: MVForgePreferences) => void;
}) {
  const draft = value;
  const candidates = draft.executionPreference === 'hardware'
    ? [
        { value: 'auto', label: 'Auto · first available hardware encoder' },
        { value: 'hevc_qsv', label: 'HEVC Quick Sync' },
        { value: 'hevc_videotoolbox', label: 'HEVC VideoToolbox' },
        { value: 'hevc_nvenc', label: 'HEVC NVENC' },
      ]
    : [
        { value: 'auto', label: 'Auto · software encoder for codec' },
        { value: 'libx265', label: 'libx265' },
        { value: 'libx264', label: 'libx264' },
        { value: 'libsvtav1', label: 'SVT-AV1' },
      ];
  const encoderOptions = candidates.filter((option) => option.value === 'auto' || availableEncoders.includes(option.value));
  const selectedEncoder = encoderOptions.some((option) => option.value === draft.preferredVideoEncoder)
    ? draft.preferredVideoEncoder
    : 'auto';

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">MVForge draft preferences</Typography>
            <Typography color="text.secondary" variant="body2">
              These values initialize new LAB, Profile, and Asset Override drafts. Queue and Worker never read this setting.
            </Typography>
          </Stack>
          <Grid container spacing={1.5}>
            <Grid size={{ xs: 12, md: 4 }}>
              <TextField select fullWidth label="Quality / storage preference" value={draft.qualityGoal} onChange={(event) => onChange({ ...draft, qualityGoal: event.target.value as MVForgePreferences['qualityGoal'], preferredVideoEncoder: selectedEncoder })}>
                <MenuItem value="maximum_savings">Maximum space saving</MenuItem>
                <MenuItem value="balanced">Balanced</MenuItem>
                <MenuItem value="conservative">Conservative quality</MenuItem>
                <MenuItem value="maximum_quality">Maximum quality</MenuItem>
                <MenuItem value="archive">Archive</MenuItem>
              </TextField>
            </Grid>
            <Grid size={{ xs: 12, md: 4 }}>
              <TextField select fullWidth label="Preferred execution" value={draft.executionPreference} onChange={(event) => onChange({ ...draft, executionPreference: event.target.value as MVForgePreferences['executionPreference'], preferredVideoEncoder: 'auto' })}>
                <MenuItem value="software">Software preferred</MenuItem>
                <MenuItem value="hardware">Hardware preferred</MenuItem>
              </TextField>
            </Grid>
            <Grid size={{ xs: 12, md: 4 }}>
              <TextField select fullWidth label="Preferred video encoder" value={selectedEncoder} onChange={(event) => onChange({ ...draft, preferredVideoEncoder: event.target.value })}>
                {encoderOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}
              </TextField>
            </Grid>
            {encoderOptions.length === 1 ? (
              <Grid size={{ xs: 12 }}>
                <Alert severity="info">No worker currently reports a compatible {draft.executionPreference} encoder. Auto remains available as a draft preference and Advisor will flag unavailable hardware instead of substituting it silently.</Alert>
              </Grid>
            ) : null}
            <Grid size={{ xs: 12 }}>
              <Autocomplete
                multiple
                freeSolo
                options={subtitleLanguageOptions.map((option) => option.value)}
                value={draft.preferredLanguages}
                onChange={(_, languages) => onChange({ ...draft, preferredLanguages: normalizeTrackLanguageList(languages), preferredVideoEncoder: selectedEncoder })}
                getOptionLabel={trackLanguageLabel}
                renderOption={(props, language) => <li {...props}>{trackLanguageLabel(language)}</li>}
                renderInput={(params) => <TextField {...params} label="Preferred languages" helperText="Ordered favorites used to prefill language fields; aliases normalize to the same language and do not select or remove tracks automatically." />}
              />
            </Grid>
          </Grid>
          <Stack direction="row" spacing={1}>
            <Button onClick={() => onChange(defaultMVForgePreferences)}>Restore defaults</Button>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}
