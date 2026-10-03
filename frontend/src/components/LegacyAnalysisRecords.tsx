import { Alert, Box, Button, Card, CardContent, Collapse, Stack, Typography } from '@mui/material';
import { useState } from 'react';
import type { AppSetting } from '../api/types';

export function LegacyAnalysisRecords({ settings }: { settings: AppSetting[] }) {
  const [open, setOpen] = useState(false);
  const value = settings.find((setting) => setting.key === 'analysisRecords')?.value.records;
  const records = Array.isArray(value) ? value.filter((record): record is Record<string, unknown> => Boolean(record && typeof record === 'object')) : [];
  if (!records.length) return null;
  return <Box data-testid="legacy-analysis-settings" sx={{ mb: 1.5 }}>
    <Button size="small" onClick={() => setOpen((current) => !current)} aria-expanded={open} aria-controls="legacy-analysis-records">
      Legacy Analysis Decisions ({records.length})
    </Button>
    <Collapse in={open} id="legacy-analysis-records">
      <Stack spacing={1} sx={{ mt: 1 }}>
        <Alert severity="info">Saved decisions and notes are preserved as legacy records. Asset Info History shows an annotation only when the full source-evidence identity matches; path equality alone is never sufficient.</Alert>
        {records.map((record, index) => <Card variant="outlined" key={String(record.id ?? index)}><CardContent sx={{ py: 1 }}>
          <Typography fontWeight={700}>{String(record.assetName ?? 'Legacy asset')} · {String(record.decision ?? 'No decision')}</Typography>
          <Typography variant="body2" color="text.secondary">Legacy record · {String(record.assetPath ?? 'source path unavailable')} · {String(record.createdAt ?? '')}</Typography>
          {record.notes ? <Typography variant="body2" sx={{ mt: 0.5 }}>{String(record.notes)}</Typography> : null}
        </CardContent></Card>)}
      </Stack>
    </Collapse>
  </Box>;
}
