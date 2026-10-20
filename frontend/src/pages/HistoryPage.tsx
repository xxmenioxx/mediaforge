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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import { api } from '../api/client';
import type { ValidationHistorySummary } from '../api/types';
import { PageHeader } from '../components/PageHeader';

export function HistoryPage() {
  const queryClient = useQueryClient();
  const [search, setSearch] = useState('');
  const [sortDirection, setSortDirection] = useState<'desc' | 'asc'>('desc');
  const [pageSize, setPageSize] = useState(25);
  const [page, setPage] = useState(1);
  const jobs = useQuery({
    queryKey: ['validationHistory', search, sortDirection, pageSize, page],
    queryFn: () => api.validationHistory({ q: search.trim(), sort: sortDirection, limit: pageSize, offset: (page - 1) * pageSize }),
    refetchInterval: 5000,
  });
  const jellyfinOutcomes = useQuery({ queryKey: ['jellyfinOutcomes'], queryFn: api.jellyfinOutcomes, refetchInterval: 5000 });
  const retryJellyfin = useMutation({
    mutationFn: api.retryJellyfinOutcome,
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ['jellyfinOutcomes'] }),
  });
  const historicalJobs = jobs.data?.items ?? [];
  const totalPages = Math.max(1, Math.ceil((jobs.data?.total ?? 0) / pageSize));
  const pageJobs = historicalJobs;

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
                  <TableCell>Publication</TableCell>
                  <TableCell>Original archive</TableCell>
                  <TableCell>Updated</TableCell>
                  <TableCell align="right">Action</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {pageJobs.map((job) => {
                  const jellyfinOutcome = job.publisher
                    ? jellyfinOutcomes.data?.find((outcome) => outcome.publicationId === job.publisher?.publicationId)
                    : undefined;
                  return (
                  <TableRow key={job.id} hover>
                    <TableCell>#{job.executionNumber ?? job.id}</TableCell>
                    <TableCell>
                      <Typography fontWeight={700} noWrap>{fileNameFromPath(job.mediaPath)}</Typography>
                      <Typography color="text.secondary" variant="body2" noWrap>{job.mediaPath}</Typography>
                    </TableCell>
                    <TableCell>
                      <Stack spacing={0.5} alignItems="flex-start">
                        <Chip label={job.status} color={statusColor(job)} size="small" />
                        {job.dismissedAt ? <Chip label="Dismissed" size="small" variant="outlined" /> : null}
                      </Stack>
                    </TableCell>
                    <TableCell>
                      <Stack spacing={0.25}>
                        <Typography>{job.validationOutcome || job.validationStatus || 'pending'}{job.validationRunId ? ` · ${job.validationScore}/100` : ''}</Typography>
                        {job.validationRunId ? <Typography variant="caption" color="text.secondary">Run {job.validationRunId}{job.validationOverridePresent ? ' · override recorded' : ''}</Typography> : null}
                        {job.dismissedAt ? <Typography variant="caption" color="text.secondary">
                          {job.status === 'completed' && (job.validationOutcome === 'FAIL' || job.validationOutcome === 'REVIEW_REQUIRED')
                            ? `Dismissed after Validation ${job.validationOutcome}`
                            : 'Dismissed from Queue'} · {formatDate(job.dismissedAt)}
                        </Typography> : null}
                      </Stack>
                    </TableCell>
                    <TableCell>
                      <Stack spacing={0.25}>
                        {job.publisher ? <>
                          <Typography>{job.publisher.state}</Typography>
                          <Typography variant="caption" color="text.secondary">Publication {job.publisher.publicationId}</Typography>
                          <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-all' }}>{job.publisher.destinationPath}</Typography>
                          <Typography variant="caption" color="text.secondary">Mode: {job.publisher.publishMode}</Typography>
                          <Typography variant="caption" color="text.secondary">
                            Attempts: {job.publisher.attemptCount} · latest {job.publisher.latestAttempt?.outcome ?? 'no persisted outcome'}
                            {job.publisher.latestAttempt?.summary ? ` · ${job.publisher.latestAttempt.summary}` : ''}
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            Validation Run {job.publisher.validationRunId || '—'}
                            {job.publisher.validationOverridePresent
                              ? ` · override recorded${job.publisher.validationOverrideEventIds?.length ? ` (${job.publisher.validationOverrideEventIds.join(', ')})` : ''}`
                              : ' · no override'}
                          </Typography>
                          {jellyfinOutcome ? (
                            <Stack spacing={0.5}>
                              <Typography variant="caption" color="text.secondary">
                                Jellyfin refresh: {jellyfinOutcome.status === 'success' ? (jellyfinOutcome.effectiveScope === 'server_wide' ? 'server scan requested' : 'requested') : jellyfinOutcome.status.replace(/_/g, ' ')} · {formatDate(jellyfinOutcome.updatedAt)}
                              </Typography>
                              {jellyfinOutcome.retryEligible ? <Button size="small" disabled={retryJellyfin.isPending} onClick={() => retryJellyfin.mutate(jellyfinOutcome.id)}>Retry Jellyfin refresh</Button> : null}
                            </Stack>
                          ) : null}
                        </> : hasLegacyPublication(job) ? <>
                          <Typography>Legacy publication</Typography>
                          {job.publishedPath ? <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-all' }}>{job.publishedPath}</Typography> : null}
                          {job.publishedAt ? <Typography variant="caption" color="text.secondary">Published {formatDate(job.publishedAt)}</Typography> : null}
                        </> : <Typography>—</Typography>}
                      </Stack>
                    </TableCell>
                    <TableCell>
                      <Stack spacing={0.25}>
                        <Typography>{job.originalArchiveStatus || (job.originalArchivedPath ? 'complete' : '—')}</Typography>
                        {job.originalArchivedPath ? <Typography variant="caption" color="text.secondary" sx={{ wordBreak: 'break-all' }}>{job.originalArchivedPath}</Typography> : null}
                      </Stack>
                    </TableCell>
                    <TableCell>{formatDate(job.updatedAt)}</TableCell>
                    <TableCell align="right">
                      <Button component={RouterLink} to={`/queue?job=${job.id}`} variant="contained" size="small">Review</Button>
                    </TableCell>
                  </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
        {!jobs.isLoading && (jobs.data?.total ?? 0) === 0 ? <Alert severity="info" sx={{ mt: 2 }}>No historical jobs yet.</Alert> : null}
      </Box>
    </>
  );
}

function fileNameFromPath(value: string) {
  return value.split('/').filter(Boolean).at(-1) ?? value;
}

function statusColor(job: ValidationHistorySummary) {
  if (job.status === 'completed') {
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

function hasLegacyPublication(job: ValidationHistorySummary) {
  return !job.publisher && Boolean(job.publishedAt || job.publishedPath);
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}
