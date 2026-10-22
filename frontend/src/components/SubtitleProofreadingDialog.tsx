import { useMemo, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  InputLabel,
  LinearProgress,
  MenuItem,
  Select,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { api } from '../api/client';
import type { ExternalSubtitle, SubtitleExtractionOperation, SubtitleProofreadingFinding } from '../api/types';

type Decision = { action: 'accept' | 'edit' | 'reject' | 'keep_original'; text?: string };

const delay = (milliseconds: number) => new Promise((resolve) => window.setTimeout(resolve, milliseconds));

export function SubtitleProofreadingDialog({
  open,
  path,
  subtitle,
  onClose,
  onApplied,
}: {
  open: boolean;
  path: string;
  subtitle: ExternalSubtitle | null;
  onClose: () => void;
  onApplied?: () => void;
}) {
  const [mode, setMode] = useState<'quick' | 'extended'>('quick');
  const [operation, setOperation] = useState<SubtitleExtractionOperation | null>(null);
  const [operationId, setOperationId] = useState('');
  const [decisions, setDecisions] = useState<Record<string, Decision>>({});
  const [filter, setFilter] = useState('all');
  const [confirming, setConfirming] = useState(false);
  const [currentRevision, setCurrentRevision] = useState('');
  const [restoreRevision, setRestoreRevision] = useState('');
  const [busy, setBusy] = useState(false);
  const [cancelPending, setCancelPending] = useState(false);
  const [error, setError] = useState('');

  const report = operation?.proofreading;
  const findings = report?.findings ?? [];
  const visibleFindings = useMemo(
    () => filter === 'all' ? findings : findings.filter((finding) => finding.category === filter),
    [filter, findings],
  );
  const selectedDecisions = useMemo(() => Object.entries(decisions).flatMap(([cueId, decision]) => {
    const finding = findings.find((candidate) => candidate.cueId === cueId);
    if (!finding) return [];
    return [{ cueId, finding, decision }];
  }), [decisions, findings]);
  const selectedChanges = selectedDecisions.filter(({ finding, decision }) => {
    if (decision.action === 'accept') return Boolean(finding.suggestedText && finding.suggestedText !== finding.originalText);
    if (decision.action === 'edit') return Boolean(decision.text && decision.text !== finding.originalText);
    return false;
  });

  function reset() {
    setMode('quick');
    setOperation(null);
    setOperationId('');
    setDecisions({});
    setFilter('all');
    setConfirming(false);
    setCurrentRevision('');
    setRestoreRevision('');
    setBusy(false);
    setCancelPending(false);
    setError('');
  }

  async function analyze(selectedMode: 'quick' | 'extended' = mode) {
    if (!subtitle) return;
    setBusy(true);
    setError('');
    setOperation(null);
    setOperationId('');
    setDecisions({});
    setConfirming(false);
    try {
      const started = await api.startSubtitleProofreading({ path, subtitlePath: subtitle.path, mode: selectedMode });
      setOperationId(started.operationId);
      const deadline = Date.now() + 5 * 60 * 1000;
      while (Date.now() < deadline) {
        const next = await api.subtitleOperation(started.operationId);
        setOperation(next);
        if (['completed', 'cancelled', 'error'].includes(next.status)) {
          if (next.status === 'error') setError(next.error || next.message || 'Proofreading failed.');
          return;
        }
        await delay(500);
      }
      setError('Proofreading is still running. You can keep this dialog open or cancel the operation.');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not start subtitle proofreading.');
    } finally {
      setBusy(false);
      setCancelPending(false);
    }
  }

  async function cancel() {
    if (!operationId) return;
    setCancelPending(true);
    try {
      const cancelled = await api.cancelSubtitleExtractionOperation(operationId);
      setOperation(cancelled);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not cancel proofreading.');
      setCancelPending(false);
    }
  }

  function selectDecision(finding: SubtitleProofreadingFinding, decision?: Decision) {
    setDecisions((current) => {
      const next = { ...current };
      if (!decision) delete next[finding.cueId];
      else next[finding.cueId] = decision;
      return next;
    });
  }

  async function applyDecisions() {
    if (!subtitle || !report || !operationId || selectedDecisions.length === 0) return;
    setBusy(true);
    setError('');
    try {
      const result = await api.applySubtitleProofreading({
        path,
        subtitlePath: subtitle.path,
        operationId,
        revision: report.revision,
        confirmed: true,
        decisions: selectedDecisions.map(({ cueId, finding, decision }) => ({
          cueId,
          sourceTextHash: finding.sourceTextHash,
          action: decision.action,
          text: decision.text,
        })),
      });
      setCurrentRevision(result.sourceRevision);
      setRestoreRevision(result.revisionId);
      setConfirming(false);
      onApplied?.();
      setOperation((current) => current ? { ...current, message: result.message } : current);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not apply the selected proofreading decisions.');
    } finally {
      setBusy(false);
    }
  }

  async function restore() {
    if (!subtitle || !restoreRevision || !currentRevision) return;
    setBusy(true);
    setError('');
    try {
      const result = await api.restoreSubtitleProofreadingRevision({
        path,
        subtitlePath: subtitle.path,
        revisionId: restoreRevision,
        expectedRevision: currentRevision,
      });
      setCurrentRevision(result.sourceRevision);
      setRestoreRevision(result.revisionId);
      setOperation((current) => current ? { ...current, message: result.message } : current);
      onApplied?.();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Could not restore the saved subtitle revision.');
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={busy ? undefined : () => { reset(); onClose(); }}
      maxWidth="md"
      fullWidth
      aria-labelledby="subtitle-proofreading-title"
    >
      <DialogTitle id="subtitle-proofreading-title">Review sidecar with AI</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={1.5} sx={{ pt: 0.5 }}>
          {subtitle ? <Typography variant="body2" color="text.secondary">{subtitle.fileName} · {subtitle.format.toUpperCase()}</Typography> : null}
          {!report ? (
            <>
              <Typography variant="body2">Quick review scans for likely text defects and sends only suspicious cues with bounded nearby context to the configured Translation model.</Typography>
              <Typography variant="caption" color="text.secondary">Analyze leaves this sidecar untouched. Language, source type, and provider readiness are reported with the findings; the existing Translation model is checked when review starts.</Typography>
              {mode === 'extended' ? <Alert severity="warning">Extended review may take longer and use more local model resources. It sends up to 240 cues in bounded chunks; remaining cues are reported as unassessed.</Alert> : null}
              <FormControl size="small" fullWidth>
                <InputLabel id="proofreading-mode-label">Review mode</InputLabel>
                <Select
                  labelId="proofreading-mode-label"
                  label="Review mode"
                  value={mode}
                  disabled={busy}
                  onChange={(event) => setMode(event.target.value as 'quick' | 'extended')}
                >
                  <MenuItem value="quick">Quick review (recommended)</MenuItem>
                  <MenuItem value="extended">Extended review (more model work)</MenuItem>
                </Select>
              </FormControl>
              {busy ? <Stack spacing={1}><LinearProgress /><Typography variant="caption">{operation?.message || 'Reviewing bounded subtitle cues…'}</Typography><Button size="small" color="warning" variant="outlined" disabled={cancelPending || !operationId} onClick={() => void cancel()}>{cancelPending ? 'Cancelling…' : 'Cancel review'}</Button></Stack> : null}
              {error ? <Alert severity="warning">{error}</Alert> : null}
              <Button variant="contained" disabled={busy || !subtitle} onClick={() => void analyze()}>{error ? 'Retry review' : 'Analyze'}</Button>
            </>
          ) : (
            <>
              <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                <Chip size="small" label={`${report.totalCount} total`} />
                <Chip size="small" color="success" label={`${report.cleanCount} clean`} />
                <Chip size="small" color="warning" label={`${report.suggestedCount} suggested fixes`} />
                <Chip size="small" color="error" label={`${report.reviewCount} needs review`} />
                <Chip size="small" label={`${report.unassessedCount} unassessed`} />
              </Stack>
              <Typography variant="body2" color="text.secondary">
                Language: {report.language || 'unknown'} · Source: {report.sourceType} · Provider: {report.provider}{report.model ? ` · ${report.model}` : ''} · Readiness: {report.providerState}
              </Typography>
              {report.selfReview ? <Alert severity="info">The configured Translation model may be reviewing its own output; this is not independent verification.</Alert> : null}
              {report.providerNotice ? <Alert severity={report.providerState === 'ready' || report.providerState === 'not_needed' ? 'info' : 'warning'}>{report.providerNotice}</Alert> : null}
              {operation?.message ? <Typography variant="caption" color="text.secondary">{operation.message}</Typography> : null}
              {error ? <Alert severity="warning">{error}</Alert> : null}
              {findings.length > 0 ? <FormControl size="small" sx={{ maxWidth: 320 }}>
                <InputLabel id="proofreading-filter-label">Filter findings</InputLabel>
                <Select labelId="proofreading-filter-label" label="Filter findings" value={filter} onChange={(event) => setFilter(event.target.value)}>
                  <MenuItem value="all">All findings</MenuItem>
                  {[...new Set(findings.map((finding) => finding.category))].map((category) => <MenuItem key={category} value={category}>{category}</MenuItem>)}
                </Select>
              </FormControl> : null}
              {visibleFindings.length === 0 ? <Alert severity="success">No suspicious cues need a manual decision for this revision.</Alert> : null}
              {visibleFindings.map((finding) => {
                const decision = decisions[finding.cueId];
                return <Box key={finding.cueId} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.25 }}>
                  <Stack spacing={0.75}>
                    <Stack direction="row" alignItems="center" spacing={1} flexWrap="wrap" useFlexGap>
                      <Checkbox
                        size="small"
                        checked={Boolean(decision)}
                        aria-label={`Select cue ${finding.ordinal}`}
                        onChange={(event) => selectDecision(finding, event.target.checked ? { action: finding.suggestedText ? 'accept' : 'keep_original' } : undefined)}
                      />
                      <Typography fontWeight={700}>Cue {finding.ordinal}</Typography>
                      <Chip size="small" label={finding.category} />
                      <Chip size="small" label={finding.status === 'suggested' ? 'suggestion pending' : 'needs review'} color="warning" />
                    </Stack>
                    <Typography variant="caption" color="text.secondary">{finding.reason}</Typography>
                    <Typography variant="body2"><strong>Original:</strong> {finding.originalText}</Typography>
                    {finding.suggestedText ? <Typography variant="body2"><strong>Suggested:</strong> {finding.suggestedText}</Typography> : null}
                    {finding.neighbors?.length ? <Typography variant="caption" color="text.secondary">Nearby: {finding.neighbors.map((neighbor) => neighbor.text).join(' · ')}</Typography> : null}
                    {decision?.action === 'edit' ? <TextField size="small" label={`Edit cue ${finding.ordinal}`} multiline minRows={1} value={decision.text ?? ''} onChange={(event) => selectDecision(finding, { action: 'edit', text: event.target.value })} /> : null}
                    <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                      {finding.suggestedText ? <Button size="small" variant={decision?.action === 'accept' ? 'contained' : 'outlined'} onClick={() => selectDecision(finding, { action: 'accept' })}>Accept</Button> : null}
                      <Button size="small" variant={decision?.action === 'keep_original' ? 'contained' : 'outlined'} onClick={() => selectDecision(finding, { action: 'keep_original' })}>Keep original</Button>
                      {finding.suggestedText ? <Button size="small" variant={decision?.action === 'reject' ? 'contained' : 'outlined'} onClick={() => selectDecision(finding, { action: 'reject' })}>Reject</Button> : null}
                      <Button size="small" variant={decision?.action === 'edit' ? 'contained' : 'outlined'} onClick={() => selectDecision(finding, { action: 'edit', text: finding.suggestedText ?? finding.originalText })}>Edit</Button>
                    </Stack>
                  </Stack>
                </Box>;
              })}
              {selectedDecisions.length > 0 ? <Alert severity="info">{selectedDecisions.length} cue decisions selected · {selectedChanges.length} text changes</Alert> : null}
              {confirming ? <Box sx={{ border: 1, borderColor: 'warning.main', borderRadius: 1, p: 1.25 }}>
                <Stack spacing={0.75}>
                  <Typography fontWeight={700}>Confirm {selectedDecisions.length} cue decisions</Typography>
                  <Typography variant="body2">{selectedChanges.length} cue texts will change. Review each exact Original → Selected value before publishing.</Typography>
                  {selectedDecisions.map(({ finding, decision }) => {
                    const next = decision.action === 'accept' ? finding.suggestedText : decision.action === 'edit' ? decision.text : finding.originalText;
                    return <Typography key={finding.cueId} variant="caption"><strong>Cue {finding.ordinal}:</strong> {finding.originalText} → {next}</Typography>;
                  })}
                  <Stack direction="row" spacing={1}>
                    <Button size="small" variant="contained" disabled={busy} onClick={() => void applyDecisions()}>Confirm and apply</Button>
                    <Button size="small" disabled={busy} onClick={() => setConfirming(false)}>Back to review</Button>
                  </Stack>
                </Stack>
              </Box> : null}
              {!confirming ? <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                <Button variant="contained" disabled={busy || selectedDecisions.length === 0} onClick={() => setConfirming(true)}>Review selected decisions</Button>
                <Button variant="outlined" disabled={busy} onClick={() => void analyze(mode)}>Analyze again</Button>
                {restoreRevision && currentRevision ? <Button color="warning" variant="outlined" disabled={busy} onClick={() => void restore()}>Restore previous revision</Button> : null}
              </Stack> : null}
            </>
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button disabled={busy} onClick={() => { reset(); onClose(); }}>Close</Button>
      </DialogActions>
    </Dialog>
  );
}
