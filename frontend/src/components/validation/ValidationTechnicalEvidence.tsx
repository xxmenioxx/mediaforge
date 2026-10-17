import { useState } from 'react';
import { Alert, Button, Paper, Stack, Typography } from '@mui/material';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import DownloadIcon from '@mui/icons-material/Download';
import type { ValidationRun } from '../../api/types';

function exportFileName(run: ValidationRun): string {
  const identity = run.runId.replace(/[^a-zA-Z0-9._-]+/g, '-').replace(/^-+|-+$/g, '') || String(run.id);
  return `validation-${identity}.json`;
}

export function ValidationTechnicalEvidence({ run }: { run: ValidationRun }) {
  const [message, setMessage] = useState('');
  const json = JSON.stringify(run, null, 2);

  const copy = async () => {
    try {
      if (!navigator.clipboard?.writeText) throw new Error('Clipboard API unavailable');
      await navigator.clipboard.writeText(json);
      setMessage('Validation run JSON copied.');
    } catch {
      setMessage('Copy is unavailable. Select the JSON below and copy it manually.');
    }
  };

  const download = () => {
    let objectUrl: string | undefined;
    try {
      if (typeof Blob === 'undefined' || typeof URL.createObjectURL !== 'function' || typeof document === 'undefined') {
        throw new Error('Download API unavailable');
      }
      objectUrl = URL.createObjectURL(new Blob([json], { type: 'application/json' }));
      const anchor = document.createElement('a');
      anchor.href = objectUrl;
      anchor.download = exportFileName(run);
      anchor.style.display = 'none';
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      setMessage(`JSON download started: ${anchor.download}`);
    } catch {
      setMessage('Download is unavailable in this browser. The complete JSON remains available below.');
    } finally {
      if (objectUrl && typeof URL.revokeObjectURL === 'function') URL.revokeObjectURL(objectUrl);
    }
  };

  return <Stack spacing={1.5}>
    <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ sm: 'center' }} spacing={1}>
      <Stack spacing={0.25}>
        <Typography variant="subtitle1" fontWeight={700}>Technical Evidence</Typography>
        <Typography variant="body2" color="text.secondary">Immutable Validation run {run.runId}. This view represents the run returned by the API without changing it.</Typography>
      </Stack>
      <Stack direction="row" spacing={1}>
        <Button startIcon={<ContentCopyIcon />} variant="outlined" onClick={copy}>Copy JSON</Button>
        <Button startIcon={<DownloadIcon />} variant="outlined" onClick={download}>Download JSON</Button>
      </Stack>
    </Stack>
    {message ? <Alert severity={message.includes('unavailable') ? 'info' : 'success'} role="status">{message}</Alert> : null}
    <Paper variant="outlined" sx={{ maxHeight: '55vh', overflow: 'auto', p: 1.5 }}>
      <Typography component="pre" aria-label="Validation run JSON" tabIndex={0} sx={{ m: 0, fontFamily: 'monospace', fontSize: '0.8rem', whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{json}</Typography>
    </Paper>
  </Stack>;
}
