import {
  Alert,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { useMutation, useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type {
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
  const capability = useQuery({
    queryKey: ['subtitleTranslationCapability', path, source],
    queryFn: () => api.subtitleTranslationCapability({ path, source }),
    enabled: open,
    retry: false,
  });
  const [sourceLanguage, setSourceLanguage] = useState('');
  const [targetLocale, setTargetLocale] = useState('');
  const [outputRepresentation, setOutputRepresentation] = useState('');
  const [started, setStarted] = useState(false);
  const start = useMutation({
    mutationFn: api.startSubtitleTranslation,
    onSuccess: () => setStarted(true),
  });
  const resetStart = start.reset;

  useEffect(() => {
    if (!open) return;
    const value = capability.data;
    if (!value) return;
    setSourceLanguage((current) =>
      value.sourceLanguages.includes(current)
        ? current
        : value.sourceLanguages.includes('auto')
          ? 'auto'
          : value.sourceLanguages[0] ?? '',
    );
    setTargetLocale((current) =>
      value.targetLocales.includes(current)
        ? current
        : value.targetLocales.includes('es-419')
          ? 'es-419'
          : value.targetLocales[0] ?? '',
    );
    setOutputRepresentation((current) =>
      value.outputRepresentations.includes(current)
        ? current
        : value.outputRepresentations[0] ?? '',
    );
  }, [capability.data, open]);

  useEffect(() => {
    if (!open) resetStart();
  }, [open, resetStart]);

  const value = capability.data;
  const outputRepresentations = value?.outputRepresentations ?? [];
  const canStart = Boolean(
    value &&
      value.eligible &&
      value.available &&
      value.sourceLanguages.length > 0 &&
      value.targetLocales.length > 0 &&
      outputRepresentations.length > 0 &&
      sourceLanguage &&
      targetLocale &&
      outputRepresentation &&
      !start.isPending &&
      !started,
  );

  const submit = () => {
    if (!canStart) return;
    start.mutate({
      path,
      source,
      sourceLanguage,
      targetLocale,
      outputRepresentation,
    });
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Translate subtitle</DialogTitle>
      <DialogContent>
        <Stack spacing={1.5} sx={{ pt: 1 }}>
          {capability.isLoading ? <Alert severity="info">Loading translation capability…</Alert> : null}
          {capability.isError ? (
            <Alert severity="warning">
              {capability.error instanceof Error ? capability.error.message : 'Could not load translation capability.'}
            </Alert>
          ) : null}
          {value && !value.eligible ? <Alert severity="warning">{value.reason || 'This subtitle cannot be translated.'}</Alert> : null}
          {value && value.eligible && !value.available ? (
            <Alert severity="warning">{value.reason || 'Translation is currently unavailable.'}</Alert>
          ) : null}
          {value && value.eligible && value.available && outputRepresentations.length === 0 ? (
            <Alert severity="warning">No valid translation output is available for this subtitle.</Alert>
          ) : null}
          {value ? (
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
                value={sourceLanguage}
                onChange={(event) => setSourceLanguage(event.target.value)}
                disabled={!value.available || value.sourceLanguages.length === 0 || started}
              >
                {value.sourceLanguages.map((language) => (
                  <MenuItem key={language} value={language}>
                    {languageLabels[language] ?? language}
                  </MenuItem>
                ))}
              </TextField>
              <TextField
                select
                fullWidth
                size="small"
                label="Target locale"
                value={targetLocale}
                onChange={(event) => setTargetLocale(event.target.value)}
                disabled={!value.available || value.targetLocales.length === 0 || started}
              >
                {value.targetLocales.map((locale) => (
                  <MenuItem key={locale} value={locale}>
                    {localeLabels[locale] ?? locale}
                  </MenuItem>
                ))}
              </TextField>
              {outputRepresentations.length > 1 ? (
                <TextField
                  select
                  fullWidth
                  size="small"
                  label="Output format"
                  value={outputRepresentation}
                  onChange={(event) => setOutputRepresentation(event.target.value)}
                  disabled={!value.available || started}
                >
                  {outputRepresentations.map((representation) => (
                    <MenuItem key={representation} value={representation}>
                      {representation.toUpperCase()}
                    </MenuItem>
                  ))}
                </TextField>
              ) : null}
              <Typography variant="body2" color="text.secondary">
                Media context: {value.context.status}{value.context.source ? ` · ${value.context.source}` : ''}
              </Typography>
            </>
          ) : null}
          {start.isError ? (
            <Alert severity="warning">
              {start.error instanceof Error ? start.error.message : 'Could not start subtitle translation.'}
            </Alert>
          ) : null}
          {started ? <Alert severity="success">Subtitle translation started.</Alert> : null}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{started ? 'Close' : 'Cancel'}</Button>
        {!started ? (
          <Button variant="contained" onClick={submit} disabled={!canStart}>
            Start translation
          </Button>
        ) : null}
      </DialogActions>
    </Dialog>
  );
}
