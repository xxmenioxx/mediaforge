import { useMutation, useQuery } from '@tanstack/react-query';
import { Alert, Button, Card, CardContent, FormControlLabel, MenuItem, Stack, Switch, TextField, Typography } from '@mui/material';
import { useEffect, useState } from 'react';
import { ApiRequestError, api } from '../../api/client';
import type { OCRAISettingsValue, OllamaModelReadinessResponse } from '../../api/types';
import { ollamaModelCatalogQueryKey } from './OllamaSettings';

export function ocrAISettingsValue(value?: Record<string, unknown>): OCRAISettingsValue {
  return {
    ...value,
    enabled: typeof value?.enabled === 'boolean' ? value.enabled : false,
    model: typeof value?.model === 'string' ? value.model : '',
  };
}

export function OCRAISettingsCard({
  value,
  providerConfigured,
  providerConfigurationRevision,
  onDraftChange,
}: {
  value: OCRAISettingsValue;
  providerConfigured: boolean;
  providerConfigurationRevision: string;
  onDraftChange: (value: Record<string, unknown>) => void;
}) {
  const draft = value;
  const [readiness, setReadiness] = useState<{
    response: OllamaModelReadinessResponse;
    providerConfigurationRevision: string;
  } | null>(null);
  const models = useQuery({
    queryKey: ollamaModelCatalogQueryKey(providerConfigurationRevision),
    queryFn: api.listOllamaModels,
    enabled: false,
    retry: false,
  });
  const checkReadiness = useMutation({
    mutationFn: ({ modelId }: { modelId: string; providerConfigurationRevision: string }) => api.testOllamaModelReadiness(modelId),
    onMutate: () => setReadiness(null),
    onSuccess: (response, variables) => setReadiness({ response, providerConfigurationRevision: variables.providerConfigurationRevision }),
  });

  const currentReadiness = readiness?.providerConfigurationRevision === providerConfigurationRevision
    && readiness.response.modelId === draft.model
    ? readiness.response
    : null;
  const savedModelIsMissing = models.data && draft.model &&
    !models.data.installedModels.some((model) => model.id === draft.model || model.name === draft.model);
  const readinessCheckMatchesCurrentSelection = checkReadiness.variables?.providerConfigurationRevision === providerConfigurationRevision
    && checkReadiness.variables.modelId === draft.model;
  const readinessErrorState = readinessCheckMatchesCurrentSelection && checkReadiness.error instanceof ApiRequestError
    ? checkReadiness.error.state
    : undefined;

  useEffect(() => {
    setReadiness(null);
    checkReadiness.reset();
  }, [providerConfigurationRevision, checkReadiness.reset]);

  function updateModel(model: string) {
    onDraftChange({ ...draft, model });
    setReadiness(null);
    checkReadiness.reset();
  }

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">OCR AI assistance</Typography>
            <Typography color="text.secondary" variant="body2">
              Choose a model independently from Translation. OCR AI is off by default; saving this setting never runs inference or changes subtitle cues.
            </Typography>
          </Stack>

          <FormControlLabel
            control={(
              <Switch
                checked={draft.enabled}
                onChange={(event) => onDraftChange({ ...draft, enabled: event.target.checked })}
                disabled={!draft.enabled && (!providerConfigured || !draft.model || currentReadiness?.state !== 'available')}
              />
            )}
            label="Enable OCR AI"
          />
          {!draft.enabled && draft.model && currentReadiness?.state !== 'available' ? (
            <Typography color="text.secondary" variant="body2">
              Check the selected model before enabling OCR AI.
            </Typography>
          ) : null}

          <TextField
            select
            label="OCR AI model"
            value={draft.model}
            disabled={!providerConfigured}
            SelectProps={{
              displayEmpty: true,
              renderValue: (selected) => typeof selected === 'string' && selected ? selected : 'No model selected',
            }}
            helperText="Uses the shared Ollama connection. The same model may be selected for Translation, but no selection is copied automatically."
            onChange={(event) => updateModel(event.target.value)}
          >
            <MenuItem value="">No model selected</MenuItem>
            {draft.model && !models.data?.installedModels.some((model) => model.id === draft.model || model.name === draft.model) ? (
              <MenuItem value={draft.model}>
                {draft.model} {models.isError ? '(catalog unavailable)' : models.data ? '(not in latest catalog)' : '(saved selection)'}
              </MenuItem>
            ) : null}
            {models.data?.installedModels.map((model) => (
              <MenuItem key={model.id} value={model.id}>{model.name}</MenuItem>
            ))}
          </TextField>

          {!providerConfigured ? (
            <Alert severity="info">Configure the shared Ollama provider before loading or checking models.</Alert>
          ) : null}
          {savedModelIsMissing ? (
            <Alert severity="warning">The saved model is absent from the latest successful catalog. Its selection is preserved; choose another model explicitly or check its readiness.</Alert>
          ) : null}
          {models.isError ? (
            <Alert severity="warning">The installed-model catalog is unavailable. The saved model selection is unchanged.</Alert>
          ) : null}
          {models.data && models.data.installedModels.length === 0 ? (
            <Alert severity="info">No Ollama models are installed. Model discovery does not download models.</Alert>
          ) : null}
          {currentReadiness?.state === 'available' ? (
            <Alert severity="success">Model {currentReadiness.modelId} is available at the shared provider.</Alert>
          ) : null}
          {currentReadiness?.state === 'model_not_found' || readinessErrorState === 'model_not_found' ? (
            <Alert severity="warning">The selected model was not found. Its saved selection is unchanged.</Alert>
          ) : null}
          {currentReadiness && currentReadiness.state !== 'available' && currentReadiness.state !== 'model_not_found' ? (
            <Alert severity="warning">Model readiness is {currentReadiness.state}. The saved selection is unchanged.</Alert>
          ) : null}
          {readinessCheckMatchesCurrentSelection && checkReadiness.isError && readinessErrorState !== 'model_not_found' ? (
            <Alert severity="warning">The selected model could not be checked because the provider is unavailable or returned an invalid response. Its saved selection is unchanged.</Alert>
          ) : null}

          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} flexWrap="wrap" useFlexGap>
            <Button
              variant="outlined"
              onClick={() => { void models.refetch(); }}
              disabled={!providerConfigured || models.isFetching}
            >
              {models.isFetching ? 'Loading models…' : 'Load installed models'}
            </Button>
            <Button
              variant="outlined"
              onClick={() => checkReadiness.mutate({ modelId: draft.model, providerConfigurationRevision })}
              disabled={!providerConfigured || !draft.model || checkReadiness.isPending}
            >
              {checkReadiness.isPending ? 'Checking model…' : 'Check selected model'}
            </Button>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}
