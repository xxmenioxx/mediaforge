import {
  Alert,
  Button,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  LinearProgress,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import type {
  SubtitleExtractionOperation,
  SubtitleTranslationCapability,
  SubtitleTranslationSourceSelector,
} from '../api/types';

const languageLabels: Record<string, string> = {
  auto: 'Auto',
  en: 'English',
  ja: 'Japanese',
};

const localeLabels: Record<string, string> = {
  'es-419': 'Spanish (Latin America)',
  'es-ES': 'Spanish (Spain)',
};

function providerLabel(provider?: string) {
  if (!provider) return 'Not configured';
  return provider.toLowerCase() === 'ollama' ? 'Ollama' : provider;
}

function operationPhaseLabel(phase: SubtitleExtractionOperation['phase']) {
  return phase.charAt(0).toUpperCase() + phase.slice(1);
}

function apiErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function TranslationOperationStatus({
  operation,
  operationError,
  cancelPending,
  retryPending,
  retryError,
  onCancel,
  onRetry,
}: {
  operation: SubtitleExtractionOperation;
  operationError?: unknown;
  cancelPending: boolean;
  retryPending: boolean;
  retryError?: unknown;
  onCancel: () => void;
  onRetry: () => void;
}) {
  const running = operation.status === 'running';
  const cancelled = operation.status === 'cancelled';
  const completed = operation.status === 'completed';
  const diagnostic =
    operation.translation?.diagnostic ||
    operation.error ||
    operation.message ||
    'Subtitle translation failed.';
  const translatedArtifacts = (operation.result?.artifacts ?? []).filter(
    (artifact) => artifact.mode === 'translation',
  );

  return (
    <Stack spacing={1.5}>
      {running ? (
        <>
          <Stack direction="row" justifyContent="space-between" spacing={1}>
            <Typography variant="body2">
              {operationPhaseLabel(operation.phase)}
              {operation.total > 0 ? ` · ${operation.processed} / ${operation.total}` : ''}
            </Typography>
            <Typography variant="body2" color="text.secondary">
              {operation.progress}%
            </Typography>
          </Stack>
          <LinearProgress
            variant="determinate"
            value={operation.progress}
            aria-label="Subtitle translation progress"
          />
          {operation.message ? <Typography variant="body2" color="text.secondary">{operation.message}</Typography> : null}
        </>
      ) : null}
      {operationError ? <Alert severity="warning">{apiErrorMessage(operationError, 'Could not poll subtitle translation.')}</Alert> : null}
      {completed ? <Alert severity="success">Translation completed.</Alert> : null}
      {cancelled ? <Alert severity="info">Translation cancelled.</Alert> : null}
      {operation.status === 'error' ? (
        <Alert severity="warning">
          {operation.translation?.failureClass ? `${operation.translation.failureClass}: ` : ''}
          {diagnostic}
        </Alert>
      ) : null}
      {translatedArtifacts.length > 0 ? (
        <Stack spacing={0.75}>
          <Typography variant="subtitle2">Translated artifacts</Typography>
          {translatedArtifacts.map((artifact) => (
            <Stack key={artifact.artifactId} direction="row" spacing={0.75} alignItems="center" flexWrap="wrap" useFlexGap>
              <Chip label={(artifact.translation?.targetLocale || 'translated').toUpperCase()} size="small" />
              <Chip label={artifact.format.toUpperCase()} size="small" />
              {artifact.publishedPath ? <Typography variant="caption" sx={{ overflowWrap: 'anywhere' }}>{artifact.publishedPath}</Typography> : null}
            </Stack>
          ))}
        </Stack>
      ) : null}
      {!running && operation.translation?.retryable ? (
        <Button variant="outlined" onClick={onRetry} disabled={retryPending}>
          {retryPending ? 'Retrying…' : 'Retry translation'}
        </Button>
      ) : null}
      {running && operation.translation?.cancellable ? (
        <Button color="warning" variant="outlined" onClick={onCancel} disabled={cancelPending}>
          {cancelPending ? 'Cancelling…' : 'Cancel translation'}
        </Button>
      ) : null}
      {retryError ? <Alert severity="warning">{apiErrorMessage(retryError, 'Could not retry subtitle translation.')}</Alert> : null}
    </Stack>
  );
}

export function SubtitleTranslationDialog({
  open,
  path,
  source,
  onClose,
}: {
  open: boolean;
  path: string;
  source: SubtitleTranslationSourceSelector;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const capability = useQuery({
    queryKey: ['subtitleTranslationCapability', path, source],
    queryFn: () => api.subtitleTranslationCapability({ path, source }),
    enabled: open,
    retry: false,
  });
  const [sourceLanguage, setSourceLanguage] = useState('');
  const [targetLocale, setTargetLocale] = useState('');
  const [outputRepresentation, setOutputRepresentation] = useState('');
  const [operation, setOperation] = useState<SubtitleExtractionOperation | null>(null);
  const completedOperationRef = useRef<string | null>(null);
  const operationQuery = useQuery({
    queryKey: ['subtitleOperation', operation?.id ?? ''],
    queryFn: () => api.subtitleOperation(operation!.id),
    enabled: open && Boolean(operation?.id),
    retry: false,
    refetchInterval: (query) => query.state.data?.status === 'running' ? 500 : false,
  });
  const start = useMutation({
    mutationFn: api.startSubtitleTranslation,
    onSuccess: (response) => setOperation(response.operation),
  });
  const cancel = useMutation({
    mutationFn: (operationId: string) => api.cancelSubtitleOperation(operationId),
    onSuccess: (response, operationId) => {
      setOperation(response);
      queryClient.setQueryData(['subtitleOperation', operationId], response);
    },
  });
  const retry = useMutation({
    mutationFn: (operationId: string) => api.retrySubtitleTranslation(operationId),
    onSuccess: (response) => {
      cancel.reset();
      setOperation(response.operation);
    },
  });

  useEffect(() => {
    const observed = operationQuery.data;
    if (!observed || observed.status !== 'completed' || completedOperationRef.current === observed.id) return;
    completedOperationRef.current = observed.id;
    void queryClient.invalidateQueries({ queryKey: ['externalSubtitles', path] });
  }, [operationQuery.data, path, queryClient]);

  const value: SubtitleTranslationCapability | undefined = capability.data;
  const outputRepresentations = value?.outputRepresentations ?? [];
  const selectedSourceLanguage = value
    ? value.sourceLanguages.includes(sourceLanguage)
      ? sourceLanguage
      : value.sourceLanguages.includes('auto')
        ? 'auto'
        : value.sourceLanguages[0] ?? ''
    : '';
  const selectedTargetLocale = value
    ? value.targetLocales.includes(targetLocale)
      ? targetLocale
      : value.targetLocales.includes('es-419')
        ? 'es-419'
        : value.targetLocales[0] ?? ''
    : '';
  const selectedOutputRepresentation = value
    ? outputRepresentations.includes(outputRepresentation)
      ? outputRepresentation
      : outputRepresentations[0] ?? ''
    : '';
  const canStart = Boolean(
    value &&
      value.eligible &&
      value.available &&
      value.sourceLanguages.length > 0 &&
      value.targetLocales.length > 0 &&
      outputRepresentations.length > 0 &&
      selectedSourceLanguage &&
      selectedTargetLocale &&
      selectedOutputRepresentation &&
      !start.isPending &&
      !operation,
  );
  const currentOperation = operationQuery.data ?? operation;

  const submit = () => {
    if (!canStart) return;
    start.mutate({
      path,
      source,
      sourceLanguage: selectedSourceLanguage,
      targetLocale: selectedTargetLocale,
      outputRepresentation: selectedOutputRepresentation,
    });
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Translate subtitle</DialogTitle>
      <DialogContent>
        <Stack spacing={1.5} sx={{ pt: 1 }}>
          {!currentOperation && capability.isLoading ? <Alert severity="info">Loading translation capability…</Alert> : null}
          {!currentOperation && capability.isError ? (
            <Alert severity="warning">{apiErrorMessage(capability.error, 'Could not load translation capability.')}</Alert>
          ) : null}
          {!currentOperation && value && !value.eligible ? <Alert severity="warning">{value.reason || 'This subtitle cannot be translated.'}</Alert> : null}
          {!currentOperation && value && value.eligible && !value.available ? (
            <Alert severity="warning">{value.reason || 'Translation is currently unavailable.'}</Alert>
          ) : null}
          {!currentOperation && value && value.eligible && value.available && outputRepresentations.length === 0 ? (
            <Alert severity="warning">No valid translation output is available for this subtitle.</Alert>
          ) : null}
          {!currentOperation && value ? (
            <>
              <Typography variant="body2">
                Provider: {providerLabel(value.provider.name)}
                {value.provider.modelSelectable && value.provider.model ? ` · Model: ${value.provider.model}` : ''}
              </Typography>
              <TextField
                select
                fullWidth
                size="small"
                label="Source language"
                value={selectedSourceLanguage}
                onChange={(event) => setSourceLanguage(event.target.value)}
                disabled={!value.available || value.sourceLanguages.length === 0}
              >
                {value.sourceLanguages.map((language) => (
                  <MenuItem key={language} value={language}>{languageLabels[language] ?? language}</MenuItem>
                ))}
              </TextField>
              <TextField
                select
                fullWidth
                size="small"
                label="Target locale"
                value={selectedTargetLocale}
                onChange={(event) => setTargetLocale(event.target.value)}
                disabled={!value.available || value.targetLocales.length === 0}
              >
                {value.targetLocales.map((locale) => (
                  <MenuItem key={locale} value={locale}>{localeLabels[locale] ?? locale}</MenuItem>
                ))}
              </TextField>
              {outputRepresentations.length > 1 ? (
                <TextField
                  select
                  fullWidth
                  size="small"
                  label="Output format"
                  value={selectedOutputRepresentation}
                  onChange={(event) => setOutputRepresentation(event.target.value)}
                  disabled={!value.available}
                >
                  {outputRepresentations.map((representation) => (
                    <MenuItem key={representation} value={representation}>{representation.toUpperCase()}</MenuItem>
                  ))}
                </TextField>
              ) : null}
              <Typography variant="body2" color="text.secondary">
                Media context: {value.context.status}{value.context.source ? ` · ${value.context.source}` : ''}
              </Typography>
            </>
          ) : null}
          {!currentOperation && start.isError ? (
            <Alert severity="warning">{apiErrorMessage(start.error, 'Could not start subtitle translation.')}</Alert>
          ) : null}
          {currentOperation ? (
            <TranslationOperationStatus
              operation={currentOperation}
              operationError={operationQuery.error}
              cancelPending={cancel.isPending}
              retryPending={retry.isPending}
              retryError={retry.error}
              onCancel={() => cancel.mutate(currentOperation.id)}
              onRetry={() => retry.mutate(currentOperation.id)}
            />
          ) : null}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{currentOperation ? 'Close' : 'Cancel'}</Button>
        {!currentOperation ? <Button variant="contained" onClick={submit} disabled={!canStart}>Start translation</Button> : null}
      </DialogActions>
    </Dialog>
  );
}
