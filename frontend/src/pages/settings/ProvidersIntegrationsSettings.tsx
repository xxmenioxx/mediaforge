import { Grid } from '@mui/material';
import { JellyfinSettingsCard } from '../../components/JellyfinSettingsCard';
import type { Library } from '../../api/types';
import { OllamaProviderSettingsCard } from './OllamaSettings';

export function ProvidersIntegrationsSettingsDomain({
  value,
  ollamaValue,
  ollamaSavedValue,
  ollamaConfigured,
  providerConfigurationRevision,
  libraries,
  saving,
  saveError,
  onOllamaDraftChange,
  onSave,
}: {
  value: Record<string, unknown> | undefined;
  ollamaValue: Record<string, unknown>;
  ollamaSavedValue: Record<string, unknown> | undefined;
  ollamaConfigured: boolean;
  providerConfigurationRevision: string;
  libraries: Library[];
  saving: boolean;
  saveError?: string;
  onOllamaDraftChange: (value: Record<string, unknown>) => void;
  onSave: (value: Record<string, unknown>) => void | Promise<void>;
}) {
  return (
    <Grid container spacing={2}>
      <Grid size={{ xs: 12 }}>
        <OllamaProviderSettingsCard
          value={ollamaValue}
          savedValue={ollamaSavedValue}
          configured={ollamaConfigured}
          providerConfigurationRevision={providerConfigurationRevision}
          onDraftChange={onOllamaDraftChange}
        />
      </Grid>
      <Grid size={{ xs: 12 }}>
        <JellyfinSettingsCard value={value} libraries={libraries} saving={saving} saveError={saveError} onSave={onSave} />
      </Grid>
    </Grid>
  );
}
