import {
  Alert,
  AlertTitle,
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
  ko: 'Korean',
  fr: 'French',
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
  const labels: Partial<Record<SubtitleExtractionOperation['phase'], string>> = {
    ocr: 'OCR',
    ocr_review: 'OCR review',
    translating: 'Translating',
    validating: 'Validating',
    publishing: 'Publishing',
  };
  return labels[phase] ?? phase.charAt(0).toUpperCase() + phase.slice(1);
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
  onReviewOCR,
}: {
  operation: SubtitleExtractionOperation;
  operationError?: unknown;
  cancelPending: boolean;
  retryPending: boolean;
  retryError?: unknown;
  onCancel: () => void;
  onRetry: () => void;
  onReviewOCR?: (operation: SubtitleExtractionOperation) => void;
}) {
  const [reviewedCueIDs, setReviewedCueIDs] = useState<Set<string>>(() => new Set());
  const [editingCueID, setEditingCueID] = useState<string | null>(null);
  const [editedText, setEditedText] = useState('');
  const qualityReview = useQuery({
    queryKey: ['subtitleTranslationQualityReview', operation.id],
    queryFn: () => api.getSubtitleTranslationQualityReview(operation.id),
    enabled: Boolean(operation.translation?.quality?.persisted),
  });
  const qualityDecision = useMutation({
    mutationFn: (input: { cueId: string; action: 'keep' | 'edit'; text?: string }) =>
      api.decideSubtitleTranslationQuality(operation.id, input),
    onSuccess: (response) => {
      setReviewedCueIDs((current) => new Set(current).add(response.cueId));
      setEditingCueID(null);
      void qualityReview.refetch();
    },
  });
  const editableCueRequest = useMutation({
    mutationFn: (input: { cueId: string; suggestion?: string }) =>
      api.getSubtitleTranslationQualityEditableCue(operation.id, input.cueId),
    onSuccess: (cue, input) => {
      setEditingCueID(cue.cueId);
      setEditedText(input.suggestion ?? cue.text);
    },
  });
  const qualityReviewRecoveryRequired = qualityReview.data?.status === 'recovery_required';
  const qualityReviewCanDecide = qualityReview.data?.status === 'open' && !qualityReview.isError;
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
  const ocrReviewRequired = operation.translation?.failureClass === 'ocr_review_required';

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
      {operation.translation?.quality ? (
        <Stack spacing={1}>
          <Alert severity={operation.translation.quality.reviewCount > 0 ? 'warning' : 'success'}>
            <AlertTitle>Translation quality</AlertTitle>
            {operation.translation.quality.cueCount} cues · {operation.translation.quality.cleanCount} clean · {operation.translation.quality.warningCount} warnings · {operation.translation.quality.reviewCount} for review. These signals are advisory and do not assess OCR source accuracy.
          </Alert>
          {operation.translation.quality.reviewer ? (
            <Alert severity={operation.translation.quality.reviewer.state === 'completed' ? 'info' : 'warning'}>
              Independent reviewer: {operation.translation.quality.reviewer.state}
              {operation.translation.quality.reviewer.model ? ` · ${operation.translation.quality.reviewer.model}` : ''}
            </Alert>
          ) : null}
          {qualityReviewRecoveryRequired ? (
            <Alert severity="warning">Publisher could not confirm rollback of a reviewed edit. Further decisions are locked pending recovery.</Alert>
          ) : null}
          {(operation.translation.quality.findings ?? []).map((finding) => (
            <Alert key={finding.cueId} severity="info">
              <AlertTitle>Cue {finding.cueId} · {finding.reasons.join(', ')}</AlertTitle>
              <Typography variant="caption" component="div">Source: {finding.sourceText}</Typography>
              {operation.translation?.quality?.reviewer?.findings?.find((item) => item.cueId === finding.cueId)?.suggestedText ? (
                <>
                  <Typography variant="caption" component="div">Reviewer suggestion (advisory): {operation.translation.quality.reviewer.findings.find((item) => item.cueId === finding.cueId)?.suggestedText}</Typography>
                  <Button size="small" disabled={!qualityReviewCanDecide || editableCueRequest.isPending} onClick={() => { const suggestion = operation.translation?.quality?.reviewer?.findings?.find((item) => item.cueId === finding.cueId)?.suggestedText; if (suggestion) { editableCueRequest.mutate({ cueId: finding.cueId, suggestion }); } }}>Review suggestion</Button>
                </>
              ) : null}
              {editingCueID === finding.cueId ? (
                <TextField fullWidth size="small" label="Edit translation" value={editedText} onChange={(event) => setEditedText(event.target.value)} helperText={editedText.length > 4000 ? 'Reduce this cue to 4,000 characters or fewer before saving.' : undefined} sx={{ mt: 1 }} />
              ) : <Typography variant="caption" component="div">Translation preview: {finding.translatedText}</Typography>}
              {reviewedCueIDs.has(finding.cueId) || qualityReview.data?.decisions.some((decision) => decision.cueId === finding.cueId) ? (
                <Chip size="small" label="Reviewed" sx={{ mt: 1 }} />
              ) : editingCueID === finding.cueId ? (
                <Stack direction="row" spacing={1} sx={{ mt: 1 }}>
                  <Button size="small" variant="contained" disabled={qualityDecision.isPending || !editedText.trim() || [...editedText].length > 4000 || !qualityReviewCanDecide} onClick={() => qualityDecision.mutate({ cueId: finding.cueId, action: 'edit', text: editedText })}>Save edit</Button>
                  <Button size="small" onClick={() => setEditingCueID(null)}>Cancel</Button>
                </Stack>
              ) : (
                <Stack direction="row" spacing={1} sx={{ mt: 1 }}>
                  <Button size="small" disabled={qualityDecision.isPending || !qualityReviewCanDecide} onClick={() => qualityDecision.mutate({ cueId: finding.cueId, action: 'keep' })}>Keep translation</Button>
                  <Button size="small" disabled={!qualityReviewCanDecide || qualityDecision.isPending || editableCueRequest.isPending} onClick={() => editableCueRequest.mutate({ cueId: finding.cueId })}>Edit</Button>
                </Stack>
              )}
            </Alert>
          ))}
          {operation.translation.quality.truncated ? <Typography variant="caption" color="text.secondary">Additional flagged cues were omitted from this bounded review summary.</Typography> : null}
          {operation.translation.quality.persistenceWarning ? <Alert severity="warning">{operation.translation.quality.persistenceWarning}</Alert> : null}
          {editableCueRequest.isError ? <Alert severity="warning">{apiErrorMessage(editableCueRequest.error, 'Could not load the complete translation cue from its verified revision.')}</Alert> : null}
          {qualityReview.isError ? <Alert severity="warning">{apiErrorMessage(qualityReview.error, 'Could not verify the saved translation review.')}</Alert> : null}
          {qualityDecision.isError ? <Alert severity="warning">{apiErrorMessage(qualityDecision.error, 'Could not save the translation review decision.')}</Alert> : null}
        </Stack>
      ) : null}
      {cancelled ? <Alert severity="info">Translation cancelled.</Alert> : null}
      {operation.status === 'error' ? (
        ocrReviewRequired ? (
          <Stack spacing={1}>
            <Alert severity="warning">
              <AlertTitle>Translation paused for source OCR review</AlertTitle>
              {operation.translation?.ocrReviewTarget?.message || diagnostic}
            </Alert>
            <Typography variant="caption" color="text.secondary">Source OCR uncertainty is separate from translation quality. No translation correction or quality approval has been recorded.</Typography>
          </Stack>
        ) : (
          <Alert severity="warning">
            {operation.translation?.failureClass ? `${operation.translation.failureClass}: ` : ''}
            {diagnostic}
          </Alert>
        )
      ) : null}
      {operation.status === 'error' && ['ocr', 'ocr_review', 'translating', 'validating', 'publishing'].includes(operation.phase) ? (
        <Typography variant="caption" color="text.secondary">Stage: {operationPhaseLabel(operation.phase)}</Typography>
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
      {ocrReviewRequired && onReviewOCR ? (
        <Button variant="outlined" onClick={() => onReviewOCR(operation)}>Review OCR</Button>
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
  onReviewOCR,
}: {
  open: boolean;
  path: string;
  source: SubtitleTranslationSourceSelector;
  onClose: () => void;
  onReviewOCR?: (operation: SubtitleExtractionOperation) => void;
}) {
  const queryClient = useQueryClient();
  const [sourceLanguage, setSourceLanguage] = useState('');
  const [targetLocale, setTargetLocale] = useState('');
  const [outputRepresentation, setOutputRepresentation] = useState('');
  const [operation, setOperation] = useState<SubtitleExtractionOperation | null>(null);
  const capability = useQuery({
    queryKey: ['subtitleTranslationCapability', path, source],
    queryFn: () => api.subtitleTranslationCapability({ path, source }),
    enabled: open && !operation,
    retry: false,
  });
  const completedOperationRef = useRef<string | null>(null);
  const operationQuery = useQuery({
    queryKey: ['subtitleOperation', operation?.id ?? ''],
    queryFn: () => api.subtitleOperation(operation!.id),
    enabled: open && Boolean(operation?.id) && operation?.kind !== 'quality_review_resume',
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
  const savedQualityReviews = useQuery({
    queryKey: ['subtitleTranslationQualityReviews', path, value?.source.sourceIdentity ?? ''],
    queryFn: () => api.listSubtitleTranslationQualityReviews(path, value!.source.sourceIdentity),
    enabled: open && !operation && Boolean(value?.source.sourceIdentity),
    retry: false,
  });
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
          {!currentOperation && (savedQualityReviews.data?.reviews.length ?? 0) > 0 ? (
            <Stack spacing={0.5}>
              <Typography variant="subtitle2">Saved translation reviews</Typography>
              {savedQualityReviews.data!.reviews.map((review) => (
                <Stack key={review.operationId} direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                  <Typography variant="caption">{review.quality.reviewCount} cues for review · {new Date(review.updatedAt).toLocaleString()}</Typography>
                  <Button size="small" onClick={() => setOperation({
                    id: review.operationId, assetPath: path, status: 'completed', phase: 'completed', progress: 100,
                    processed: 0, total: 0, streamIndex: 0, format: 'original', kind: 'quality_review_resume',
                    message: 'Resumed saved translation review',
                    createdAt: review.updatedAt, updatedAt: review.updatedAt,
                    translation: { retryable: false, cancellable: false, quality: review.quality },
                  })}>Resume review</Button>
                </Stack>
              ))}
            </Stack>
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
              onReviewOCR={onReviewOCR}
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
