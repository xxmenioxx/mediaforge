import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Grid,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { api } from '../api/client';
import type { QueueJob } from '../api/types';
import { PageHeader } from '../components/PageHeader';

export function HistoryPage() {
  const jobs = useQuery({ queryKey: ['queueJobs'], queryFn: api.queueJobs, refetchInterval: 5000 });
  const [search, setSearch] = useState('');
  const [sortDirection, setSortDirection] = useState<'desc' | 'asc'>('desc');
  const [pageSize, setPageSize] = useState(25);
  const [page, setPage] = useState(1);
  const historicalJobs = useMemo(() => {
    const query = search.trim().toLowerCase();
    return (jobs.data ?? [])
      .filter((job) => job.status === 'completed' || job.status === 'failed' || job.status === 'canceled' || Boolean(job.publishedAt))
      .filter((job) => {
        if (!query) {
          return true;
        }
        return [String(job.id), job.mediaPath, job.status, job.publishedPath, job.validationStatus, job.notes]
          .some((value) => value.toLowerCase().includes(query));
      })
      .sort((left, right) => (sortDirection === 'desc' ? right.id - left.id : left.id - right.id));
  }, [jobs.data, search, sortDirection]);
  const totalPages = Math.max(1, Math.ceil(historicalJobs.length / pageSize));
  const pageJobs = historicalJobs.slice((page - 1) * pageSize, page * pageSize);

  return (
    <>
      <PageHeader title="History" eyebrow="Audit trail">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Review completed, failed, canceled, and published jobs without crowding the active queue.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {jobs.isError ? <Alert severity="warning">Unable to load history.</Alert> : null}
        <Card sx={{ mb: 2 }}>
          <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
            <Grid container spacing={1.5} alignItems="center">
              <Grid size={{ xs: 12, md: 6 }}>
                <TextField label="Search history" value={search} onChange={(event) => { setSearch(event.target.value); setPage(1); }} fullWidth />
              </Grid>
              <Grid size={{ xs: 6, md: 2 }}>
                <TextField label="Sort" value={sortDirection} onChange={(event) => setSortDirection(event.target.value as 'desc' | 'asc')} select fullWidth>
                  <MenuItem value="desc">Newest first</MenuItem>
                  <MenuItem value="asc">Oldest first</MenuItem>
                </TextField>
              </Grid>
              <Grid size={{ xs: 6, md: 2 }}>
                <TextField label="Rows" value={pageSize} onChange={(event) => { setPageSize(Number(event.target.value)); setPage(1); }} select fullWidth>
                  {[10, 25, 50, 100].map((count) => (
                    <MenuItem key={count} value={count}>{count}</MenuItem>
                  ))}
                </TextField>
              </Grid>
              <Grid size={{ xs: 12, md: 2 }}>
                <Stack direction="row" spacing={1} justifyContent={{ xs: 'flex-start', md: 'flex-end' }}>
                  <Button variant="outlined" disabled={page <= 1} onClick={() => setPage((current) => Math.max(1, current - 1))}>Prev</Button>
                  <Button variant="outlined" disabled={page >= totalPages} onClick={() => setPage((current) => Math.min(totalPages, current + 1))}>Next</Button>
                </Stack>
                <Typography color="text.secondary" variant="body2" align="right" sx={{ mt: 0.5 }}>
                  Page {page} / {totalPages}
                </Typography>
              </Grid>
            </Grid>
          </CardContent>
        </Card>
        <Card>
          <CardContent sx={{ p: 0, '&:last-child': { pb: 0 } }}>
            <Table sx={{ tableLayout: 'fixed' }}>
              <TableHead>
                <TableRow>
                  <TableCell>Job</TableCell>
                  <TableCell>Asset</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>Validation</TableCell>
                  <TableCell>Updated</TableCell>
                  <TableCell align="right">Action</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {pageJobs.map((job) => (
                  <TableRow key={job.id} hover>
                    <TableCell>#{job.executionNumber ?? job.id}</TableCell>
                    <TableCell>
                      <Typography fontWeight={700} noWrap>{fileNameFromPath(job.mediaPath)}</Typography>
                      <Typography color="text.secondary" variant="body2" noWrap>{job.mediaPath}</Typography>
                    </TableCell>
                    <TableCell><Chip label={job.publishedAt ? 'published' : job.status} color={statusColor(job)} size="small" /></TableCell>
                    <TableCell>{job.validationStatus || 'pending'} · {job.validationScore}/100</TableCell>
                    <TableCell>{formatDate(job.updatedAt)}</TableCell>
                    <TableCell align="right">
                      <Button component={RouterLink} to={`/queue?job=${job.id}`} variant="contained" size="small">Review</Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
        {!jobs.isLoading && historicalJobs.length === 0 ? <Alert severity="info" sx={{ mt: 2 }}>No historical jobs yet.</Alert> : null}
      </Box>
    </>
  );
}

function fileNameFromPath(value: string) {
  return value.split('/').filter(Boolean).at(-1) ?? value;
}

function statusColor(job: QueueJob) {
  if (job.publishedAt || job.status === 'completed') {
    return 'success';
  }
  if (job.status === 'failed') {
    return 'error';
  }
  if (job.status === 'canceled') {
    return 'warning';
  }
  return 'default';
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
