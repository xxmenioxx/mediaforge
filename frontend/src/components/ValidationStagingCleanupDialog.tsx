import { useEffect, useState } from 'react';
import { Alert, Box, Button, Checkbox, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, List, ListItem, Stack, Typography } from '@mui/material';
import { api } from '../api/client';
import type { ValidationStagingCleanupPreview, ValidationStagingCleanupResult } from '../api/types';

export function ValidationStagingCleanupDialog({
  open, jobId, executionNumber, mode, onClose, onDismiss,
  onComplete,
}: {
  open: boolean;
  jobId: number;
  executionNumber?: number;
  mode: 'dismiss' | 'retry';
  onClose: () => void;
  onDismiss?: () => Promise<void>;
  onComplete?: () => void;
}) {
  const [preview, setPreview] = useState<ValidationStagingCleanupPreview | null>(null);
  const [loading, setLoading] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [committed, setCommitted] = useState(false);
  const [includeCleanup, setIncludeCleanup] = useState(true);
  const [previewError, setPreviewError] = useState('');
  const [dismissError, setDismissError] = useState('');
  const [result, setResult] = useState<ValidationStagingCleanupResult | null>(null);

  useEffect(() => {
    if (!open) return;
    let current = true;
    setPreview(null); setPreviewError(''); setDismissError(''); setResult(null); setCommitted(false); setIncludeCleanup(true); setLoading(true);
    api.previewValidationStagingCleanup(jobId).then((value) => {
      if (!current) return;
      setPreview(value);
      if (value.state === 'blocked') setIncludeCleanup(false);
    })
      .catch((error: unknown) => { if (current) setPreviewError(message(error)); })
      .finally(() => { if (current) setLoading(false); });
    return () => { current = false; };
  }, [open, jobId]);

  async function confirm() {
    setConfirming(true); setDismissError(''); setResult(null);
    try {
      if (mode === 'dismiss') {
        if (!onDismiss) throw new Error('Dismiss action is unavailable.');
        await onDismiss();
        setCommitted(true);
      }
    } catch (error) {
      setDismissError(message(error)); setConfirming(false); return;
    }
    if (includeCleanup && preview && preview.state !== 'blocked') {
      try { const cleanupResult = await api.cleanupValidationStaging({ jobId, previewId: preview.previewId }); setResult(cleanupResult); onComplete?.(); }
      catch (error) {
        setResult({ jobId, previewId: preview.previewId, outcome: 'failed', removedPaths: [], recoveredBytes: 0, reason: message(error), evidenceId: '' });
        onComplete?.();
      }
    }
    setConfirming(false);
  }

  const done = Boolean(result) || (mode === 'dismiss' && committed && !includeCleanup);
  const cleanupAllowed = Boolean(preview && preview.state !== 'blocked');
  return <Dialog open={open} onClose={confirming ? undefined : onClose} maxWidth="sm" fullWidth>
    <DialogTitle>{mode === 'dismiss' ? 'Dismiss job' : 'Review staged files and retry cleanup'}</DialogTitle>
    <DialogContent>
      <Stack spacing={1.5}>
        <Typography>QueueJob #{jobId}{executionNumber ? ` · Execution #${executionNumber}` : ''}</Typography>
        {loading ? <Typography color="text.secondary">Loading read-only staging preview…</Typography> : null}
        {previewError ? <Alert severity="warning">Preview unavailable: {previewError}</Alert> : null}
        {preview ? <>
          <Typography variant="body2">Workspace: {preview.workspacePath || 'No workspace found'} · Ownership: {preview.ownership || 'unavailable'}</Typography>
          {preview.reason ? <Typography variant="body2" color="text.secondary">{preview.reason}</Typography> : null}
          <Typography fontWeight={700}>{preview.files.length} files · {formatBytes(preview.totalBytes)}</Typography>
          {preview.files.length ? <Box sx={{ maxHeight: 180, overflow: 'auto', border: 1, borderColor: 'divider', borderRadius: 1 }}><List dense>
            {preview.files.map((file) => <ListItem key={file.path} sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
              <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>{file.path}</Typography><Typography variant="caption" whiteSpace="nowrap">{formatBytes(file.sizeBytes)}</Typography>
            </ListItem>)}
          </List></Box> : null}
          {preview.blockers.map((blocker) => <Alert severity="warning" key={blocker}>{blocker}</Alert>)}
          {preview.state === 'blocked' ? <Alert severity="info">Cleanup is blocked. You can still dismiss the job and retain its workspace.</Alert> : null}
        </> : null}
        {mode === 'dismiss' ? <FormControlLabel control={<Checkbox checked={includeCleanup} disabled={loading || done} onChange={(event) => setIncludeCleanup(event.target.checked)} />} label="Also clean staged output" /> : null}
        {dismissError ? <Alert severity="error">Dismiss failed: {dismissError}</Alert> : null}
        {mode === 'dismiss' && committed && !includeCleanup ? <Alert severity="success">Dismissed; staging retained.</Alert> : null}
        {result ? <Alert severity={result.outcome === 'cleaned' || result.outcome === 'noop' ? 'success' : 'warning'}>
          {mode === 'dismiss' ? 'Dismissed; ' : ''}{stagingResult(result)}{result.reason ? `: ${result.reason}` : ''}
        </Alert> : null}
      </Stack>
    </DialogContent>
    <DialogActions>
      <Button onClick={onClose}>{done ? 'Close' : 'Cancel'}</Button>
      {!done ? <Button variant="contained" color={mode === 'dismiss' ? 'error' : 'primary'} onClick={confirm} disabled={loading || confirming || (includeCleanup && (!preview || !cleanupAllowed)) || (mode === 'retry' && (!preview || !cleanupAllowed))}>
        {confirming ? 'Working…' : mode === 'dismiss' ? 'Dismiss' : 'Confirm cleanup'}
      </Button> : null}
    </DialogActions>
  </Dialog>;
}

function stagingResult(result: ValidationStagingCleanupResult) {
  if (result.outcome === 'cleaned') return `staging cleaned (${formatBytes(result.recoveredBytes)} recovered)`;
  if (result.outcome === 'noop') return 'staging retained; no workspace to clean (0 bytes recovered)';
  return `staging ${result.outcome}`;
}
function formatBytes(value: number) { return `${(value || 0).toLocaleString()} bytes`; }
function message(error: unknown) { return error instanceof Error ? error.message : 'Request failed'; }
