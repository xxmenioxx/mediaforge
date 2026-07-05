import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogContent,
  DialogTitle,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import ArticleIcon from '@mui/icons-material/Article';
import RefreshIcon from '@mui/icons-material/Refresh';
import { useQuery } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import type { LogFile } from '../api/types';

export function LogsPage() {
  const logFiles = useQuery({ queryKey: ['logFiles'], queryFn: api.logFiles });
  const [selectedFile, setSelectedFile] = useState<LogFile | null>(null);
  const selectedContent = useQuery({
    queryKey: ['logFile', selectedFile?.name],
    queryFn: () => api.logFile(selectedFile?.name ?? ''),
    enabled: Boolean(selectedFile),
  });

  return (
    <>
      <PageHeader title="Logs" eyebrow="Diagnostics">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Open generated log files for queue jobs, validation results, publishing activity, and worker diagnostics.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {logFiles.isError ? <Alert severity="warning">Unable to read log files.</Alert> : null}
        <Stack direction="row" justifyContent="flex-end" sx={{ mb: 2 }}>
          <Button startIcon={<RefreshIcon />} variant="outlined" onClick={() => logFiles.refetch()}>
            Refresh
          </Button>
        </Stack>

        <Card>
          <CardContent sx={{ p: 0, '&:last-child': { pb: 0 } }}>
            <Table sx={{ tableLayout: 'fixed' }}>
              <TableHead>
                <TableRow>
                  <TableCell>File</TableCell>
                  <TableCell>Modified</TableCell>
                  <TableCell>Size</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {(logFiles.data ?? []).map((file) => (
                  <TableRow key={file.name} hover>
                    <TableCell>
                      <Stack direction="row" alignItems="center" spacing={1}>
                        <ArticleIcon color="primary" />
                        <Typography fontWeight={700}>{file.name}</Typography>
                      </Stack>
                    </TableCell>
                    <TableCell>{formatDate(file.modifiedAt)}</TableCell>
                    <TableCell>
                      <Chip label={formatBytes(file.sizeBytes)} size="small" />
                    </TableCell>
                    <TableCell align="right">
                      <Button variant="contained" onClick={() => setSelectedFile(file)}>
                        Open
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
        {!logFiles.isLoading && logFiles.data?.length === 0 ? (
          <Alert severity="info" sx={{ mt: 2 }}>
            No log files are available yet.
          </Alert>
        ) : null}
      </Box>

      <Dialog open={Boolean(selectedFile)} onClose={() => setSelectedFile(null)} maxWidth="lg" fullWidth>
        <DialogTitle>{selectedFile?.name ?? 'Log file'}</DialogTitle>
        <DialogContent>
          <Stack spacing={2}>
            {selectedContent.data?.truncated ? (
              <Alert severity="warning">This log is large, showing only the last 1 MB.</Alert>
            ) : null}
            {selectedContent.isError ? <Alert severity="warning">Unable to open this log file.</Alert> : null}
            <Box
              component="pre"
              sx={{
                border: 1,
                borderColor: 'divider',
                borderRadius: 1,
                bgcolor: 'rgba(255,255,255,0.03)',
                color: 'text.primary',
                fontFamily: 'monospace',
                fontSize: 13,
                lineHeight: 1.55,
                maxHeight: '68vh',
                overflow: 'auto',
                p: 2,
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
              }}
            >
              {selectedContent.isLoading ? 'Loading log...' : selectedContent.data?.content ?? ''}
            </Box>
          </Stack>
        </DialogContent>
      </Dialog>
    </>
  );
}

function formatBytes(bytes: number) {
  if (!bytes) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[unitIndex]}`;
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value));
}
