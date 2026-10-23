import { Grid } from '@mui/material';
import { JellyfinSettingsCard } from '../../components/JellyfinSettingsCard';
import type { Library } from '../../api/types';

export function ProvidersIntegrationsSettingsDomain({
  value,
  libraries,
  saving,
  saveError,
  onSave,
}: {
  value: Record<string, unknown> | undefined;
  libraries: Library[];
  saving: boolean;
  saveError?: string;
  onSave: (value: Record<string, unknown>) => void | Promise<void>;
}) {
  return (
    <Grid container spacing={2}>
      <Grid size={{ xs: 12 }}>
        <JellyfinSettingsCard value={value} libraries={libraries} saving={saving} saveError={saveError} onSave={onSave} />
      </Grid>
    </Grid>
  );
}
