import { useEffect, useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Alert, Button, Card, CardContent, FormControlLabel, Link, MenuItem, Stack, Switch, TextField, Typography } from '@mui/material';
import { Link as RouterLink } from 'react-router-dom';
import { ApiRequestError, api } from '../../api/client';
import { ollamaRuntimeStatus, testOllamaConnection } from '../../api/ollama-settings';
import type { OllamaRuntimeStatus } from '../../api/ollama-settings';
import type { OllamaModelReadinessResponse, SubtitleTranslationSettingsValue } from '../../api/types';
import { booleanValue, numberValue, stringValue, subtitleTranslationSettingsDraftValue } from '../../utils/settingsPageHelpers';

export const ollamaModelCatalogQueryKey = (revision: string) => ['ollama-models', revision] as const;
const ollamaRuntimeQueryKey = (revision: string) => ['ollama-runtime-status', revision] as const;

function useOllamaModelCatalog(revision: string) {
  return useQuery({
    queryKey: ollamaModelCatalogQueryKey(revision),
    queryFn: api.listOllamaModels,
    enabled: false,
    retry: false,
  });
}

function useModelReadiness(model: string, revision: string) {
  const [checked, setChecked] = useState<{ model: string; revision: string; result: OllamaModelReadinessResponse } | null>(null);
  const check = useMutation({
    mutationFn: ({ modelId }: { modelId: string; revision: string }) => api.testOllamaModelReadiness(modelId),
    onMutate: () => setChecked(null),
    onSuccess: (result, variables) => setChecked({ model: variables.modelId, revision: variables.revision, result }),
  });

  useEffect(() => {
    setChecked(null);
    check.reset();
  }, [model, revision, check.reset]);

  const matches = check.variables?.modelId === model && check.variables.revision === revision;
  const result = checked?.model === model && checked.revision === revision ? checked.result : null;
  const errorState = matches && check.error instanceof ApiRequestError ? check.error.state : undefined;
  return { result, errorState, error: matches ? check.error : null, isPending: check.isPending, check: () => check.mutate({ modelId: model, revision }) };
}

export function OllamaModelSelector({
  label,
  helperText,
  model,
  providerConfigured,
  providerConfigurationRevision,
  onChange,
}: {
  label: string;
  helperText: string;
  model: string;
  providerConfigured: boolean;
  providerConfigurationRevision: string;
  onChange: (model: string) => void;
}) {
  const models = useOllamaModelCatalog(providerConfigurationRevision);
  const modelIsInstalled = (candidate: string) => models.data?.installedModels.some((item) => item.id === candidate || item.name === candidate) ?? false;

  return (
    <Stack spacing={1.5}>
      <TextField
        select
        label={label}
        value={model}
        disabled={!providerConfigured}
        SelectProps={{
          displayEmpty: true,
          renderValue: (selected) => typeof selected === 'string' && selected ? selected : 'No model selected',
        }}
        helperText={helperText}
        onChange={(event) => onChange(event.target.value)}
      >
        <MenuItem value="">No model selected</MenuItem>
        {model && !modelIsInstalled(model) ? (
          <MenuItem value={model}>{model} {models.isError ? '(catalog unavailable)' : models.data ? '(not in latest catalog)' : '(saved selection)'}</MenuItem>
        ) : null}
        {models.data?.installedModels.map((item) => <MenuItem key={item.id} value={item.id}>{item.name}</MenuItem>)}
      </TextField>
      {!providerConfigured ? <Alert severity="info">Configure the shared Ollama provider before loading or checking models.</Alert> : null}
      {model && models.data && !modelIsInstalled(model) ? (
        <Alert severity="warning">The saved model is absent from the latest successful catalog. Its selection is preserved until you choose another model.</Alert>
      ) : null}
      {models.isError ? <Alert severity="warning">The installed-model catalog is unavailable. The saved selection is unchanged.</Alert> : null}
      {models.data && models.data.installedModels.length === 0 ? <Alert severity="info">No Ollama models are installed. Model discovery does not download models.</Alert> : null}
      <Button
        variant="outlined"
        onClick={() => { void models.refetch(); }}
        disabled={!providerConfigured || models.isFetching}
        sx={{ alignSelf: 'flex-start' }}
      >
        {models.isFetching ? 'Loading models…' : models.data ? 'Refresh installed models' : 'Load installed models'}
      </Button>
    </Stack>
  );
}

function connectionStateLabel(state: string) {
  if (state === 'reachable' || state === 'available') return 'Connected';
  if (state === 'unconfigured') return 'Not configured';
  if (state === 'migration_required') return 'Migration required';
  if (state === 'migration_conflict') return 'Migration conflict';
  if (state === 'invalid_configuration') return 'Invalid configuration';
  return state.replaceAll('_', ' ');
}

export function OllamaProviderSettingsCard({
  value,
  savedValue,
  configured,
  providerConfigurationRevision,
  onDraftChange,
}: {
  value: Record<string, unknown>;
  savedValue: Record<string, unknown> | undefined;
  configured: boolean;
  providerConfigurationRevision: string;
  onDraftChange: (value: Record<string, unknown>) => void;
}) {
  const models = useOllamaModelCatalog(providerConfigurationRevision);
  const runtime = useQuery({
    queryKey: ollamaRuntimeQueryKey(providerConfigurationRevision),
    queryFn: ollamaRuntimeStatus,
    enabled: configured,
    retry: false,
  });
  const testConnection = useMutation({ mutationFn: testOllamaConnection });
  const endpoint = stringValue(value.endpoint, '');
  const savedEndpoint = stringValue(savedValue?.endpoint, '');
  const endpointDirty = endpoint !== savedEndpoint;
  const providerState = stringValue(savedValue?.state, 'unconfigured');
  const residentCount = runtime.data?.models.filter((model) => model.resident).length ?? 0;

  useEffect(() => {
    testConnection.reset();
  }, [endpoint, providerConfigurationRevision, testConnection.reset]);

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Ollama</Typography>
            <Typography color="text.secondary" variant="body2">
              Configure one shared connection for Translation, OCR AI assistance, and AI Copilot.
            </Typography>
          </Stack>
          <TextField
            label="Ollama endpoint"
            placeholder="http://192.168.1.100:11434"
            value={endpoint}
            onChange={(event) => onDraftChange({ ...value, endpoint: event.target.value })}
            helperText="Use an http or https URL without embedded credentials."
            fullWidth
          />
          <Typography variant="body2">Saved provider state: {connectionStateLabel(providerState)}</Typography>
          {endpointDirty ? <Alert severity="info">The endpoint is an unsaved draft. Save it from the Settings page before testing the connection or refreshing models.</Alert> : null}
          {providerState === 'migration_conflict' ? <Alert severity="error">The saved provider endpoint conflicts with legacy Translation settings. Resolve the migration before testing or discovering models.</Alert> : null}
          {testConnection.data ? (
            <Alert severity={testConnection.data.state === 'reachable' ? 'success' : 'warning'}>
              Connection test: {connectionStateLabel(testConnection.data.state)}{testConnection.data.serverVersion ? ` · Ollama ${testConnection.data.serverVersion}` : ''}{testConnection.data.diagnostic ? ` · ${testConnection.data.diagnostic}` : ''}
            </Alert>
          ) : null}
          {testConnection.isError ? <Alert severity="warning">{testConnection.error instanceof Error ? testConnection.error.message : 'Ollama connection test failed.'}</Alert> : null}
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
            <Button
              variant="outlined"
              onClick={() => testConnection.mutate()}
              disabled={!configured || endpointDirty || testConnection.isPending}
            >
              {testConnection.isPending ? 'Testing connection…' : 'Test connection'}
            </Button>
            <Button
              variant="outlined"
              onClick={() => { void models.refetch(); }}
              disabled={!configured || endpointDirty || models.isFetching}
            >
              {models.isFetching ? 'Refreshing models…' : 'Refresh models'}
            </Button>
          </Stack>
          {models.isError ? <Alert severity="warning">Installed models could not be loaded. {models.error instanceof Error ? models.error.message : 'The provider may be unavailable.'}</Alert> : null}
          {models.data?.diagnostic ? <Alert severity={models.data.state === 'available' ? 'info' : 'warning'}>{models.data.diagnostic}</Alert> : null}
          {models.data && models.data.installedModels.length === 0 ? <Alert severity="info">No Ollama models are installed. Refreshing the list does not download models.</Alert> : null}
          {models.data && models.data.installedModels.length > 0 ? (
            <Stack spacing={0.5}>
              <Typography variant="subtitle2">Installed models ({models.data.installedModels.length})</Typography>
              {models.data.installedModels.map((model) => <Typography key={model.id} variant="body2">{model.name}</Typography>)}
            </Stack>
          ) : null}
          {runtime.isError ? <Alert severity="warning">Resident model status is unavailable. {runtime.error instanceof Error ? runtime.error.message : ''}</Alert> : null}
          {runtime.data ? <Typography variant="body2">Resident models: {residentCount} · Runtime status: {connectionStateLabel(runtime.data.state)}</Typography> : null}
          <Link component={RouterLink} to="/settings?section=runtime" underline="hover">
            Runtime protection and model warmup status
          </Link>
        </Stack>
      </CardContent>
    </Card>
  );
}

export type AICopilotSettingsValue = Record<string, unknown> & {
  enabled: boolean;
  model: string;
  preference: 'quality' | 'balanced' | 'compression';
};

export function aiCopilotSettingsValue(value: Record<string, unknown> | undefined): AICopilotSettingsValue {
  const preference = value?.preference === 'quality' || value?.preference === 'compression' ? value.preference : 'balanced';
  return {
    ...value,
    enabled: booleanValue(value?.enabled, false),
    model: stringValue(value?.model, ''),
    preference,
  };
}

export function AICopilotSettingsCard({
  value,
  providerConfigured,
  providerConfigurationRevision,
  onDraftChange,
}: {
  value: AICopilotSettingsValue;
  providerConfigured: boolean;
  providerConfigurationRevision: string;
  onDraftChange: (value: Record<string, unknown>) => void;
}) {
  const readiness = useModelReadiness(value.model, providerConfigurationRevision);
  const canEnable = providerConfigured && Boolean(value.model) && readiness.result?.state === 'available';
  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">AI Copilot</Typography>
            <Typography color="text.secondary" variant="body2">Choose a Settings-level model and preference. This model choice is not an Asset or Lab operation setting.</Typography>
          </Stack>
          <FormControlLabel
            control={<Switch checked={value.enabled} onChange={(event) => onDraftChange({ ...value, enabled: event.target.checked })} disabled={!value.enabled && !canEnable} />}
            label={value.enabled ? 'AI Copilot enabled' : 'AI Copilot disabled'}
          />
          {!value.enabled && !canEnable ? <Typography color="text.secondary" variant="body2">Check an installed model before enabling AI Copilot.</Typography> : null}
          <OllamaModelSelector
            label="AI Copilot model"
            helperText="Selected independently from Translation and OCR AI assistance. The same installed model can be selected for all three features."
            model={value.model}
            providerConfigured={providerConfigured}
            providerConfigurationRevision={providerConfigurationRevision}
            onChange={(model) => onDraftChange({ ...value, model })}
          />
          <TextField select label="Quality preference" value={value.preference} onChange={(event) => onDraftChange({ ...value, preference: event.target.value })}>
            <MenuItem value="quality">Quality</MenuItem>
            <MenuItem value="balanced">Balanced</MenuItem>
            <MenuItem value="compression">Compression</MenuItem>
          </TextField>
          {readiness.result?.state === 'available' ? <Alert severity="success">Model {readiness.result.modelId} is available at the shared provider.</Alert> : null}
          {readiness.result && readiness.result.state !== 'available' ? <Alert severity="warning">Model readiness is {readiness.result.state}. The selection is preserved.</Alert> : null}
          {readiness.errorState === 'model_not_found' ? <Alert severity="warning">The selected model was not found. Its selection is unchanged.</Alert> : null}
          {readiness.error && readiness.errorState !== 'model_not_found' ? <Alert severity="warning">The selected model is unavailable or returned an invalid readiness response. Its selection is unchanged.</Alert> : null}
          <Button variant="outlined" onClick={readiness.check} disabled={!providerConfigured || !value.model || readiness.isPending} sx={{ alignSelf: 'flex-start' }}>
            {readiness.isPending ? 'Checking model…' : 'Check selected model'}
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}

export function AITranslationSettingsCard({
  value,
  configDirty,
  providerConfigured,
  providerConfigurationRevision,
  disableSaving,
  disableError,
  onDraftChange,
  onDisable,
}: {
  value: SubtitleTranslationSettingsValue;
  configDirty: boolean;
  providerConfigured: boolean;
  providerConfigurationRevision: string;
  disableSaving: boolean;
  disableError?: string;
  onDraftChange: (value: Record<string, unknown>) => void;
  onDisable: () => void;
}) {
  const queryClient = useQueryClient();
  const readiness = useModelReadiness(value.ollamaModel, providerConfigurationRevision);
  const enable = useMutation({
    mutationFn: api.enableSubtitleTranslation,
    onSuccess: async () => { await queryClient.invalidateQueries({ queryKey: ['settings'] }); },
    onError: async () => { await queryClient.invalidateQueries({ queryKey: ['settings'] }); },
  });
  const canEnable = providerConfigured && Boolean(value.ollamaModel) && !configDirty && readiness.result?.state === 'available';
  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">AI Translation</Typography>
            <Typography color="text.secondary" variant="body2">Choose a model from the shared Ollama provider. The feature keeps its own selection and enablement.</Typography>
          </Stack>
          <FormControlLabel
            control={<Switch
              checked={value.enabled}
              onChange={(event) => event.target.checked ? enable.mutate() : onDisable()}
              disabled={enable.isPending || disableSaving || (configDirty && !value.enabled) || (!value.enabled && !canEnable)}
            />}
            label={value.enabled ? 'AI Translation enabled' : 'AI Translation disabled'}
          />
          {!value.enabled && !canEnable ? <Typography color="text.secondary" variant="body2">Save the selected model and check readiness before enabling AI Translation.</Typography> : null}
          <OllamaModelSelector
            label="AI Translation model"
            helperText="Selected independently from OCR AI assistance and AI Copilot. The same installed model can be selected in each feature."
            model={value.ollamaModel}
            providerConfigured={providerConfigured}
            providerConfigurationRevision={providerConfigurationRevision}
            onChange={(ollamaModel) => onDraftChange({ ...subtitleTranslationSettingsDraftValue(value), ollamaModel })}
          />
          <TextField
            label="Independent quality reviewer model (optional)"
            value={value.qualityReviewerModel}
            onChange={(event) => onDraftChange({ ...subtitleTranslationSettingsDraftValue(value), qualityReviewerModel: event.target.value })}
            helperText="Optional second opinion for subtitle review. No model is downloaded automatically."
            fullWidth
          />
          <Typography variant="body2">Saved Translation readiness: {value.readiness.replaceAll('_', ' ')}</Typography>
          {enable.isError ? <Alert severity="warning">{enable.error instanceof Error ? enable.error.message : 'Translation could not be enabled.'}</Alert> : null}
          {disableError ? <Alert severity="warning">{disableError}</Alert> : null}
          {readiness.result?.state === 'available' ? <Alert severity="success">Model {readiness.result.modelId} is available at the shared provider.</Alert> : null}
          {readiness.result && readiness.result.state !== 'available' ? <Alert severity="warning">Model readiness is {readiness.result.state}. The selection is preserved.</Alert> : null}
          {readiness.errorState === 'model_not_found' ? <Alert severity="warning">The selected model was not found. Its selection is unchanged.</Alert> : null}
          {readiness.error && readiness.errorState !== 'model_not_found' ? <Alert severity="warning">The selected model is unavailable or returned an invalid readiness response. Its selection is unchanged.</Alert> : null}
          <Button variant="outlined" onClick={readiness.check} disabled={!providerConfigured || !value.ollamaModel || configDirty || readiness.isPending} sx={{ alignSelf: 'flex-start' }}>
            {readiness.isPending ? 'Checking model…' : 'Check selected model'}
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}

export function AIModelRuntimeSettingsCard({
  value,
  configured,
  providerConfigurationRevision,
  onDraftChange,
}: {
  value: Record<string, unknown>;
  configured: boolean;
  providerConfigurationRevision: string;
  onDraftChange: (value: Record<string, unknown>) => void;
}) {
  const runtime = useQuery({
    queryKey: ollamaRuntimeQueryKey(providerConfigurationRevision),
    queryFn: ollamaRuntimeStatus,
    enabled: configured,
    retry: false,
    refetchInterval: configured ? 15_000 : false,
  });
  const enabled = booleanValue(value.enabled, true);
  const updateNumber = (key: string, raw: string) => onDraftChange({ ...value, [key]: Number(raw) });
  const status = runtime.data as OllamaRuntimeStatus | undefined;
  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">AI model runtime</Typography>
            <Typography color="text.secondary" variant="body2">Control resource protection and inspect model warmup and residency reported by the shared runtime lifecycle.</Typography>
          </Stack>
          <FormControlLabel control={<Switch checked={enabled} onChange={(event) => onDraftChange({ ...value, enabled: event.target.checked })} />} label="AI model resource protection" />
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
            <TextField label="Maximum resident models" type="number" inputProps={{ min: 1, max: 64 }} value={numberValue(value.maxResidentModels, 1)} onChange={(event) => updateNumber('maxResidentModels', event.target.value)} fullWidth />
            <TextField label="Concurrent inferences" type="number" inputProps={{ min: 1, max: 256 }} value={numberValue(value.maxConcurrentInferences, 1)} onChange={(event) => updateNumber('maxConcurrentInferences', event.target.value)} fullWidth />
            <TextField label="Concurrent model loads" type="number" inputProps={{ min: 1, max: 64 }} value={numberValue(value.maxConcurrentModelLoads, 1)} onChange={(event) => updateNumber('maxConcurrentModelLoads', event.target.value)} fullWidth />
          </Stack>
          <Typography color="text.secondary" variant="caption">Model warmup is lifecycle-managed before inference. Save protection changes from the Runtime section save bar.</Typography>
          {!configured ? <Alert severity="info">Configure the shared Ollama provider to inspect runtime model status.</Alert> : null}
          {runtime.isLoading ? <Typography color="text.secondary" variant="body2">Loading runtime status…</Typography> : null}
          {runtime.isError ? <Alert severity="warning">Runtime model status is unavailable. {runtime.error instanceof Error ? runtime.error.message : ''}</Alert> : null}
          {status?.diagnostic ? <Alert severity={status.state === 'available' ? 'info' : 'warning'}>{status.diagnostic}</Alert> : null}
          {status?.models.map((model) => (
            <Typography key={model.modelId} variant="body2">
              {model.modelId}: {model.resident ? 'resident' : model.state}{model.activeInferenceLeases > 0 ? ` · ${model.activeInferenceLeases} active inference lease(s)` : ''}{model.expiresAt ? ` · expires ${model.expiresAt}` : ''}
            </Typography>
          ))}
          {status && status.models.length === 0 ? <Typography color="text.secondary" variant="body2">No model residency has been reported.</Typography> : null}
        </Stack>
      </CardContent>
    </Card>
  );
}
