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
  TextField,
  MenuItem,
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
import { useMemo, useState } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import type { LogFile } from '../api/types';

export function LogsPage() {
  const logFiles = useQuery({ queryKey: ['logFiles'], queryFn: api.logFiles });
  const [selectedFile, setSelectedFile] = useState<LogFile | null>(null);
  const [category, setCategory] = useState('all');
  const [query, setQuery] = useState('');
  const selectedContent = useQuery({
    queryKey: ['logFile', selectedFile?.name],
    queryFn: () => api.logFile(selectedFile?.name ?? ''),
    enabled: Boolean(selectedFile),
  });
  const filteredFiles = useMemo(() => (logFiles.data ?? []).filter((file) => (category === 'all' || file.category === category) && `${file.name} ${file.description}`.toLowerCase().includes(query.trim().toLowerCase())), [logFiles.data, category, query]);

  return (
    <>
      <PageHeader title="Logs" eyebrow="Diagnostics">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Unified diagnostics for system events, scheduler decisions, workers, pipeline lifecycle and individual jobs.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {logFiles.isError ? <Alert severity="warning">Unable to read log files.</Alert> : null}
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} justifyContent="flex-end" sx={{ mb: 2 }}>
          <TextField size="small" label="Search logs" value={query} onChange={(event) => setQuery(event.target.value)} sx={{ minWidth: 260 }} />
          <TextField select size="small" label="Category" value={category} onChange={(event) => setCategory(event.target.value)} sx={{ minWidth: 170 }}><MenuItem value="all">All logs</MenuItem><MenuItem value="system">System</MenuItem><MenuItem value="scheduler">Scheduler</MenuItem><MenuItem value="workers">Workers</MenuItem><MenuItem value="pipeline">Pipeline</MenuItem><MenuItem value="jobs">Jobs</MenuItem></TextField>
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
                  <TableCell>Category</TableCell>
                  <TableCell>Modified</TableCell>
                  <TableCell>Size</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {filteredFiles.map((file) => (
                  <TableRow key={file.name} hover>
                    <TableCell>
                      <Stack direction="row" alignItems="center" spacing={1}>
                        <ArticleIcon color="primary" />
                        <Stack><Typography fontWeight={700}>{file.name}</Typography><Typography color="text.secondary" variant="body2">{file.description}</Typography></Stack>
                      </Stack>
                    </TableCell>
                    <TableCell><Chip label={file.category} size="small" color={file.category === 'system' ? 'warning' : file.category === 'scheduler' ? 'primary' : 'default'} /></TableCell>
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
        {!logFiles.isLoading && filteredFiles.length === 0 ? (
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
