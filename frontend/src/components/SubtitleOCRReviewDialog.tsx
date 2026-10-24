import { useEffect, useState } from 'react';
import {
  Alert,
  Button,
  Chip,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  LinearProgress,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { api } from '../api/client';
import type { CanonicalOCRReviewTarget, SubtitleOCRQualityFinding, SubtitleOCRQualityReport } from '../api/types';

type ReviewDecision = 'correct' | 'approve';
type ReviewStatus = 'idle' | 'loading' | 'completed' | 'cancelled' | 'error' | 'stale';

function delay(ms: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, ms));
}

function isStaleMessage(message: string) {
  return /stale|revision changed|no longer matches/i.test(message);
}

function cueLayoutSummary(finding: SubtitleOCRQualityFinding) {
  const timing = `Timing ${finding.startSeconds.toFixed(3)}–${finding.endSeconds.toFixed(3)} s`;
  const provenance = finding.timingProvenance ? ` · ${finding.timingProvenance}` : '';
  const geometry = finding.geometry
    ? ` · Position ${finding.geometry.bitmapX}, ${finding.geometry.bitmapY}; ${finding.geometry.bitmapWidth}×${finding.geometry.bitmapHeight} on ${finding.geometry.canvasWidth}×${finding.geometry.canvasHeight}`
    : '';
  const role = finding.role ? ` · ${finding.role}` : '';
  return `${timing}${provenance}${geometry}${role}`;
}

export function SubtitleOCRReviewDialog({
  open,
  mediaPath,
  target,
  onClose,
  onSaved,
}: {
  open: boolean;
  mediaPath: string;
  target: CanonicalOCRReviewTarget | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [status, setStatus] = useState<ReviewStatus>('idle');
  const [report, setReport] = useState<SubtitleOCRQualityReport | null>(null);
  const [analysisOperationId, setAnalysisOperationId] = useState('');
  const [cancelPending, setCancelPending] = useState(false);
  const [analysisError, setAnalysisError] = useState('');
  const [drafts, setDrafts] = useState<Record<number, string>>({});
  const [decisions, setDecisions] = useState<Record<number, ReviewDecision>>({});
  const [dismissedAISuggestions, setDismissedAISuggestions] = useState<Record<number, boolean>>({});
  const [previewOpen, setPreviewOpen] = useState(false);
  const [selectionMessage, setSelectionMessage] = useState('');
  const [saving, setSaving] = useState(false);
  const [saveOperationId, setSaveOperationId] = useState('');
  const [saveError, setSaveError] = useState('');
  const [saveCancelPending, setSaveCancelPending] = useState(false);
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    if (!open) return undefined;
    let active = true;
    setReport(null);
    setAnalysisError('');
    setSaveError('');
    setDecisions({});
    setDrafts({});
    setDismissedAISuggestions({});
    setPreviewOpen(false);
    setSelectionMessage('');
    setAnalysisOperationId('');
    setCancelPending(false);
    if (!target || target.state !== 'exact' || !target.identity || !target.sidecarPaths?.length) {
      setStatus(target?.state === 'stale' ? 'stale' : 'error');
      setAnalysisError(target?.message || 'The blocked Translation operation did not return an exact canonical OCR target. Return to Translation and retry to refresh its source identity.');
      return () => { active = false; };
    }
    setStatus('loading');
    void (async () => {
      try {
        const started = await api.analyzeSubtitleOCR({
          mediaPath,
          sidecarPath: target.sidecarPaths?.[0],
          sidecarPaths: target.sidecarPaths,
          format: target.format === 'both' ? 'both' : target.format === 'ass' ? 'ass' : 'srt',
          logicalTransformKey: target.logicalTransformKey,
          reviewTarget: target,
        });
        if (!active) return;
        setAnalysisOperationId(started.operationId);
        let current = await api.subtitleExtractionOperation(started.operationId);
        while (active && current.status === 'running') {
          await delay(350);
          current = await api.subtitleExtractionOperation(started.operationId);
        }
        if (!active) return;
        if (current.status === 'completed' && current.qualityReport) {
          setReport(current.qualityReport);
          setDrafts(Object.fromEntries(current.qualityReport.findings.map((finding) => [finding.cueId, finding.suggestion ?? finding.originalText])));
          setStatus('completed');
          return;
        }
        if (current.status === 'cancelled') {
          setStatus('cancelled');
          setAnalysisError('OCR analysis was cancelled. You can retry analysis or return to Translation.');
          return;
        }
        const message = current.error || current.message || 'OCR analysis failed.';
        setAnalysisError(message);
        setStatus(isStaleMessage(message) ? 'stale' : 'error');
      } catch (error) {
        if (!active) return;
        const message = error instanceof Error ? error.message : 'OCR analysis failed.';
        setAnalysisError(message);
        setStatus(isStaleMessage(message) ? 'stale' : 'error');
      }
    })();
    return () => { active = false; };
  }, [attempt, mediaPath, open, target]);

  const findings = (report?.findings ?? []).filter((finding) => finding.status !== 'clean');
  const selectedDecisionEntries = Object.entries(decisions).slice(0, 128);
  const selectedCount = selectedDecisionEntries.length;

  function chooseDecision(cueId: number, decision: ReviewDecision) {
    setSelectionMessage('');
    setDecisions((current) => {
      if (!(cueId in current) && Object.keys(current).length >= 128) {
        setSelectionMessage('A decision batch can include at most 128 cues. Save this batch before selecting more.');
        return current;
      }
      return { ...current, [cueId]: decision };
    });
  }

  function leavePending(cueId: number) {
    setDecisions((current) => {
      const next = { ...current };
      delete next[cueId];
      return next;
    });
    setSelectionMessage(`Cue ${cueId} remains pending and will not be approved.`);
  }

  function acceptAISuggestion(finding: SubtitleOCRQualityFinding) {
    const suggestion = finding.aiSuggestion;
    if (!suggestion || suggestion.cueId !== finding.cueId || suggestion.baselineRevision !== report?.baselineRevision) {
      setSelectionMessage('This AI suggestion no longer matches the reviewed cue revision.');
      return;
    }
    setDrafts((current) => ({ ...current, [finding.cueId]: suggestion.proposedText }));
    chooseDecision(finding.cueId, 'correct');
    setSelectionMessage(`AI suggestion for cue ${finding.cueId} is staged as a correction. Review it and confirm before saving.`);
  }

  function rejectAISuggestion(finding: SubtitleOCRQualityFinding) {
    const suggestion = finding.aiSuggestion;
    if (!suggestion) return;
    setDismissedAISuggestions((current) => ({ ...current, [finding.cueId]: true }));
    const currentDraft = drafts[finding.cueId] ?? finding.suggestion ?? finding.originalText;
    if (currentDraft === suggestion.proposedText) {
      setDrafts((current) => ({ ...current, [finding.cueId]: finding.suggestion ?? finding.originalText }));
      setDecisions((current) => {
        const next = { ...current };
        delete next[finding.cueId];
        return next;
      });
    }
    setSelectionMessage(`AI suggestion for cue ${finding.cueId} was rejected and will not be saved.`);
  }

  async function cancelAnalysis() {
    if (!analysisOperationId || cancelPending) return;
    setCancelPending(true);
    try {
      await api.cancelSubtitleOperation(analysisOperationId);
    } catch (error) {
      setAnalysisError(error instanceof Error ? error.message : 'Could not cancel OCR analysis.');
      setStatus('error');
      setCancelPending(false);
    }
  }

  async function saveDecisions() {
    if (!report || !target || selectedCount === 0 || saving) return;
    const corrections = selectedDecisionEntries.map(([cueIdText, decision]) => {
      const cueId = Number(cueIdText);
      if (decision === 'approve') return { cueId, action: 'approve' as const };
      return { cueId, action: 'edit' as const, text: (drafts[cueId] ?? '').trim() };
    });
    if (corrections.some((correction) => correction.action === 'edit' && !correction.text)) {
      setSaveError('Corrected cue text must not be empty.');
      return;
    }
    setSaving(true);
    setSaveError('');
    setSaveCancelPending(false);
    try {
      const started = await api.correctSubtitleOCR({
        mediaPath,
        sidecarPath: target.sidecarPaths?.[0],
        sidecarPaths: target.sidecarPaths,
        format: report.format === 'both' ? 'both' : report.format === 'ass' ? 'ass' : 'srt',
        logicalTransformKey: report.logicalTransformKey,
        baselineRevision: report.baselineRevision,
        dictionaryLanguage: report.dictionaryLanguage,
        dictionaryRevision: report.dictionaryRevision,
        revisions: report.revisionSet,
        reviewTarget: report.reviewTarget ?? target,
        corrections,
      });
      setSaveOperationId(started.operationId);
      let current = await api.subtitleExtractionOperation(started.operationId);
      while (current.status === 'running') {
        await delay(350);
        current = await api.subtitleExtractionOperation(started.operationId);
      }
      if (current.status !== 'completed') {
        throw new Error(current.error || current.message || (current.status === 'cancelled' ? 'OCR decision save was cancelled.' : 'OCR decision save failed.'));
      }
      setPreviewOpen(false);
      onSaved();
    } catch (error) {
      const message = error instanceof Error ? error.message : 'OCR decision save failed.';
      setSaveError(message);
      if (isStaleMessage(message)) setStatus('stale');
    } finally {
      setSaving(false);
      setSaveOperationId('');
      setSaveCancelPending(false);
    }
  }

  async function cancelSave() {
    if (!saveOperationId || saveCancelPending) return;
    setSaveCancelPending(true);
    try {
      await api.cancelSubtitleOperation(saveOperationId);
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not cancel OCR decision save.');
      setSaveCancelPending(false);
    }
  }

  return (
    <Dialog open={open} onClose={saving ? undefined : onClose} maxWidth="md" fullWidth>
      <DialogTitle>OCR review for Translation</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={1.5} sx={{ pt: 0.5 }}>
          {target?.identity ? (
            <Typography variant="body2" color="text.secondary">
              Source stream {target.identity.streamIndex} · {target.identity.requestedLanguage.toUpperCase()} OCR · {target.format?.toUpperCase()} · exact published revision
            </Typography>
          ) : null}
          {status === 'loading' ? (
            <Stack spacing={1}>
              <LinearProgress />
              <Typography variant="body2">Analyzing the exact canonical OCR target…</Typography>
              <Button size="small" onClick={() => void cancelAnalysis()} disabled={cancelPending || !analysisOperationId}>
                {cancelPending ? 'Cancelling…' : 'Cancel analysis'}
              </Button>
            </Stack>
          ) : null}
          {status === 'error' || status === 'stale' || status === 'cancelled' ? (
            <Alert severity={status === 'cancelled' ? 'info' : 'warning'}>
              {analysisError || target?.message || 'OCR review is unavailable for this source revision.'}
            </Alert>
          ) : null}
          {report && status === 'completed' ? (
            <>
              <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                <Chip size="small" label={`${report.cleanCount} clean`} color="success" />
                <Chip size="small" label={`${report.suspiciousCount} suspicious`} color="warning" />
                <Chip size="small" label={`${report.reviewCount} review`} color={report.reviewCount ? 'error' : 'default'} />
              </Stack>
              {report.ocrAI ? (
                <Alert severity={['configuration_changed', 'invalid_settings', 'model_unavailable', 'provider_failed', 'provider_unavailable', 'resource_unavailable', 'settings_unavailable'].includes(report.ocrAI.state) ? 'warning' : 'info'}>
                  <Typography fontWeight={700}>
                    OCR AI: {report.ocrAI.state}{report.ocrAI.model ? ` · ${report.ocrAI.model}` : ''}
                  </Typography>
                  {report.ocrAI.notice ? <Typography variant="body2">{report.ocrAI.notice}</Typography> : null}
                  {report.ocrAI.unassessedCount > 0 ? <Typography variant="body2">{report.ocrAI.unassessedCount} suspicious cue(s) were not assessed by AI.</Typography> : null}
                </Alert>
              ) : null}
              {findings.length === 0 ? <Alert severity="success">No OCR findings require a decision for this revision.</Alert> : null}
              {findings.map((finding) => (
                <Stack key={finding.cueId} spacing={1} sx={{ p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1 }}>
                  <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                    <Typography fontWeight={700}>Cue {finding.cueId}</Typography>
                    <Chip size="small" label={finding.status} color={finding.status === 'review' ? 'error' : 'warning'} />
                  </Stack>
                  <Typography variant="body2" color="text.secondary">{(finding.reasons ?? []).join(' · ') || 'OCR quality finding'}</Typography>
                  <Typography variant="caption" color="text.secondary">{cueLayoutSummary(finding)}</Typography>
                  <Typography variant="body2"><strong>Original:</strong> {finding.originalText}</Typography>
                  {finding.suggestion ? (
                    <Typography variant="body2">
                      <strong>{finding.candidateSource === 'contextual-ocr-dictionary' ? 'Dictionary suggestion:' : 'Deterministic suggestion:'}</strong> {finding.suggestion}
                    </Typography>
                  ) : null}
                  {finding.aiSuggestion && !dismissedAISuggestions[finding.cueId] &&
                    finding.aiSuggestion.cueId === finding.cueId && finding.aiSuggestion.baselineRevision === report.baselineRevision ? (
                    <Alert severity="info">
                      <Stack spacing={0.75}>
                        <Typography variant="body2"><strong>AI Suggestion · {finding.aiSuggestion.model}:</strong> {finding.aiSuggestion.proposedText}</Typography>
                        <Typography variant="caption">{finding.aiSuggestion.reason}</Typography>
                        <Typography variant="caption">Accept stages this as a correction. It is saved only after you review and confirm the decision preview.</Typography>
                        <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                          <Button size="small" variant="outlined" disabled={saving} onClick={() => acceptAISuggestion(finding)}>Accept AI suggestion</Button>
                          <Button size="small" color="warning" disabled={saving} onClick={() => rejectAISuggestion(finding)}>Reject AI suggestion</Button>
                        </Stack>
                      </Stack>
                    </Alert>
                  ) : null}
                  <TextField
                    size="small"
                    multiline
                    minRows={2}
                    label={`Corrected text for cue ${finding.cueId}`}
                    value={drafts[finding.cueId] ?? finding.suggestion ?? finding.originalText}
                    onChange={(event) => setDrafts((current) => ({ ...current, [finding.cueId]: event.target.value }))}
                    disabled={saving}
                  />
                  <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                    <Button size="small" variant={decisions[finding.cueId] === 'correct' ? 'contained' : 'outlined'} disabled={saving} onClick={() => chooseDecision(finding.cueId, 'correct')}>Correct cue {finding.cueId}</Button>
                    <Button size="small" variant={decisions[finding.cueId] === 'approve' ? 'contained' : 'outlined'} disabled={saving} onClick={() => {
                      setDrafts((current) => ({ ...current, [finding.cueId]: finding.originalText }));
                      chooseDecision(finding.cueId, 'approve');
                    }}>Accept cue {finding.cueId} as correct (unchanged)</Button>
                    <Button size="small" disabled={saving} onClick={() => leavePending(finding.cueId)}>Leave cue {finding.cueId} pending</Button>
                  </Stack>
                </Stack>
              ))}
            </>
          ) : null}
          {selectionMessage ? <Alert severity="info">{selectionMessage}</Alert> : null}
          {saveError ? <Alert severity="warning">{saveError}</Alert> : null}
          <Collapse in={previewOpen}>
            <Alert severity="info" action={<Button color="inherit" size="small" onClick={() => void saveDecisions()} disabled={saving}>{saving ? 'Saving…' : 'Confirm and save'}</Button>}>
              <Typography fontWeight={700}>Decision preview · {selectedCount} of 128 cues</Typography>
              <Typography variant="body2">
                {selectedDecisionEntries.map(([cueId, decision]) => `${cueId}: ${decision === 'approve' ? 'Accept as correct' : `Correct to “${(drafts[Number(cueId)] ?? '').trim()}”`}`).join(' · ')}
              </Typography>
            </Alert>
          </Collapse>
        </Stack>
      </DialogContent>
      <DialogActions>
        {saving ? <Button color="warning" onClick={() => void cancelSave()} disabled={saveCancelPending || !saveOperationId}>{saveCancelPending ? 'Cancelling…' : 'Cancel save'}</Button> : null}
        {findings.length > 0 && status === 'completed' ? <Button onClick={() => setPreviewOpen(true)} disabled={selectedCount === 0 || saving}>Preview selected decisions ({selectedCount})</Button> : null}
        {(status === 'error' || status === 'cancelled') && target?.state === 'exact' ? <Button onClick={() => setAttempt((value) => value + 1)} disabled={saving}>Retry analysis</Button> : null}
        <Button onClick={onClose} disabled={saving}>Return to Translation</Button>
      </DialogActions>
    </Dialog>
  );
}
