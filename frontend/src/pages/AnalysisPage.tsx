import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogContent,
  DialogTitle,
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
import SaveIcon from '@mui/icons-material/Save';
import SearchIcon from '@mui/icons-material/Search';
import VisibilityIcon from '@mui/icons-material/Visibility';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import type { AppSetting, Asset, ScanResult } from '../api/types';
import { MediaSnapshotDetails } from '../components/MediaSnapshotDetails';
import { PageHeader } from '../components/PageHeader';

type AnalysisRecord = {
  id: string;
  assetPath: string;
  assetName: string;
  decision: string;
  notes: string;
  scan: ScanResult;
  createdAt: string;
};

export function AnalysisPage() {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const assets = useQuery({ queryKey: ['assets'], queryFn: api.assets });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const records = useMemo(() => getAnalysisRecords(settings.data), [settings.data]);
  const rawAssets = assets.data?.unprocessed ?? [];
  const queuedAssetPath = searchParams.get('asset') ?? '';
  const didBackfillReports = useRef(false);
  const [assetPath, setAssetPath] = useState(queuedAssetPath);
  const [decision, setDecision] = useState('needs-profile-review');
  const [notes, setNotes] = useState('');
  const [currentScan, setCurrentScan] = useState<ScanResult | null>(null);
  const [selectedRecord, setSelectedRecord] = useState<AnalysisRecord | null>(null);
  const [recordSearch, setRecordSearch] = useState('');
  const [recordsPerPage, setRecordsPerPage] = useState(10);
  const [recordPage, setRecordPage] = useState(1);
  const selectedAsset = rawAssets.find((asset) => asset.path === assetPath) ?? null;
  const assetOptions = useMemo<Asset[]>(
    () => ensureAssetOption(rawAssets, queuedAssetPath),
    [queuedAssetPath, rawAssets],
  );
  const filteredRecords = useMemo(() => filterAnalysisRecords(records, recordSearch), [records, recordSearch]);
  const totalRecordPages = Math.max(1, Math.ceil(filteredRecords.length / recordsPerPage));
  const pagedRecords = filteredRecords.slice((recordPage - 1) * recordsPerPage, recordPage * recordsPerPage);

  const scanAsset = useMutation({
    mutationFn: api.scan,
    onSuccess: (scan) => setCurrentScan(scan),
  });

  const updateSetting = useMutation({
    mutationFn: api.updateSetting,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });

  const backfillAsIsReports = useMutation({
    mutationFn: api.backfillAnalysisAsIsReports,
    onSuccess: async (result) => {
      if (result.imported > 0) {
        await queryClient.invalidateQueries({ queryKey: ['settings'] });
      }
    },
  });

  useEffect(() => {
    if (settings.isLoading || didBackfillReports.current) {
      return;
    }
    didBackfillReports.current = true;
    backfillAsIsReports.mutate();
  }, [backfillAsIsReports, settings.isLoading]);

  useEffect(() => {
    if (!queuedAssetPath || assetPath === queuedAssetPath) {
      return;
    }
    setAssetPath(queuedAssetPath);
  }, [assetPath, queuedAssetPath]);

  useEffect(() => {
    if (searchParams.get('autorun') !== '1' || !assetPath || scanAsset.isPending || currentScan?.path === assetPath) {
      return;
    }
    scanAsset.mutate({ path: assetPath, force: false });
    const nextParams = new URLSearchParams(searchParams);
    nextParams.delete('autorun');
    setSearchParams(nextParams, { replace: true });
  }, [assetPath, currentScan?.path, scanAsset, searchParams, setSearchParams]);

  useEffect(() => {
    setRecordPage(1);
  }, [recordSearch, recordsPerPage]);

  useEffect(() => {
    setRecordPage((current) => Math.min(current, totalRecordPages));
  }, [totalRecordPages]);

  function saveRecord() {
    if (!currentScan) {
      return;
    }
    const nextRecord: AnalysisRecord = {
      id: `${Date.now()}-${currentScan.path}`,
      assetPath: currentScan.path,
      assetName: currentScan.fileName,
      decision,
      notes,
      scan: currentScan,
      createdAt: new Date().toISOString(),
    };
    updateSetting.mutate({
      key: 'analysisRecords',
      value: { records: [nextRecord, ...records].slice(0, 200) },
    });
  }

  function loadRecord(record: AnalysisRecord) {
    setAssetPath(record.assetPath);
    setDecision(record.decision);
    setNotes(record.notes);
    setCurrentScan(record.scan);
    setSelectedRecord(null);
  }

  function rescanRecord(record: AnalysisRecord) {
    setAssetPath(record.assetPath);
    setDecision(record.decision);
    setNotes(record.notes);
    setSelectedRecord(null);
    scanAsset.mutate({ path: record.assetPath, force: true });
  }

  return (
    <>
      <PageHeader title="Analysis" eyebrow="Manual Evidence">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 860 }}>
          Capture AS IS snapshots, human notes, and review decisions so future agents can learn from real outcomes.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        <Card sx={{ mb: 2 }}>
          <CardContent>
            <Stack spacing={2}>
              <Grid container spacing={2} alignItems="stretch">
                <Grid size={{ xs: 12, md: 6 }}>
                  <AssetAutocomplete assets={assetOptions} value={selectedAsset ?? assetOptionFromPath(assetPath)} onChange={(asset) => setAssetPath(asset?.path ?? '')} />
                </Grid>
                <Grid size={{ xs: 12, md: 3 }}>
                  <TextField label="Decision" value={decision} onChange={(event) => setDecision(event.target.value)} select fullWidth>
                    <MenuItem value="needs-profile-review">Needs profile review</MenuItem>
                    <MenuItem value="good-source">Good source</MenuItem>
                    <MenuItem value="audio-needs-work">Audio needs work</MenuItem>
                    <MenuItem value="video-needs-work">Video needs work</MenuItem>
                    <MenuItem value="subtitle-needs-work">Subtitle needs work</MenuItem>
                    <MenuItem value="reject-source">Reject source</MenuItem>
                  </TextField>
                </Grid>
                <Grid size={{ xs: 12, md: 3 }}>
                  <Button
                    startIcon={<SearchIcon />}
                    variant="contained"
                    disabled={!assetPath || scanAsset.isPending}
                    onClick={() => scanAsset.mutate({ path: assetPath, force: false })}
                    fullWidth
                    sx={{ height: '100%' }}
                  >
                    Analyze AS IS
                  </Button>
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <TextField
                    label="Human notes"
                    value={notes}
                    onChange={(event) => setNotes(event.target.value)}
                    multiline
                    minRows={3}
                    placeholder="Example: old anime source, dialogue is low, hiss present, keep Spanish subtitles."
                    fullWidth
                  />
                </Grid>
              </Grid>
              {scanAsset.isError ? <Alert severity="warning">Could not analyze this asset.</Alert> : null}
              {backfillAsIsReports.data?.imported ? (
                <Alert severity="success">
                  Imported {backfillAsIsReports.data.imported} historical AS-IS report
                  {backfillAsIsReports.data.imported === 1 ? '' : 's'} into Analysis.
                </Alert>
              ) : null}
              {currentScan ? (
                <Stack spacing={2}>
                  <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                    <Chip label="AS IS snapshot" color="primary" size="small" />
                    <Chip label={currentScan.fileName} size="small" />
                    <Chip label={decision} color="warning" size="small" />
                  </Stack>
                  <MediaSnapshotDetails scan={currentScan} />
                  <Button startIcon={<SaveIcon />} variant="contained" disabled={updateSetting.isPending} onClick={saveRecord}>
                    Save Analysis Record
                  </Button>
                </Stack>
              ) : null}
              {updateSetting.isSuccess ? <Alert severity="success">Analysis record saved.</Alert> : null}
            </Stack>
          </CardContent>
        </Card>

        <Card>
          <CardContent>
            <Stack spacing={2}>
              <Typography variant="h3">Saved Evidence</Typography>
              <Grid container spacing={2} alignItems="center">
                <Grid size={{ xs: 12, md: 7 }}>
                  <TextField
                    label="Search evidence"
                    value={recordSearch}
                    onChange={(event) => setRecordSearch(event.target.value)}
                    placeholder="Asset, decision, notes, codec..."
                    fullWidth
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 2 }}>
                  <TextField
                    label="Rows"
                    value={recordsPerPage}
                    onChange={(event) => setRecordsPerPage(Number(event.target.value))}
                    select
                    fullWidth
                  >
                    {[10, 25, 50, 100].map((count) => (
                      <MenuItem key={count} value={count}>
                        {count}
                      </MenuItem>
                    ))}
                  </TextField>
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                  <Stack direction="row" spacing={1} justifyContent={{ xs: 'flex-start', md: 'flex-end' }}>
                    <Button variant="outlined" disabled={recordPage <= 1} onClick={() => setRecordPage((current) => Math.max(1, current - 1))}>
                      Prev
                    </Button>
                    <Button variant="outlined" disabled={recordPage >= totalRecordPages} onClick={() => setRecordPage((current) => Math.min(totalRecordPages, current + 1))}>
                      Next
                    </Button>
                  </Stack>
                  <Typography color="text.secondary" variant="body2" align="right" sx={{ mt: 0.5 }}>
                    Page {recordPage} / {totalRecordPages}
                  </Typography>
                </Grid>
              </Grid>
              <Box sx={{ overflowX: 'auto' }}>
                <Table size="small" sx={{ minWidth: 860 }}>
                  <TableHead>
                    <TableRow>
                      <TableCell>Asset</TableCell>
                      <TableCell>Decision</TableCell>
                      <TableCell>Snapshot</TableCell>
                      <TableCell>Notes</TableCell>
                      <TableCell>Saved</TableCell>
                      <TableCell align="right">Actions</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {pagedRecords.map((record) => (
                      <TableRow key={record.id} hover>
                        <TableCell>
                          <Stack spacing={0.3}>
                            <Typography fontWeight={700}>{record.assetName}</Typography>
                            <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                              {record.assetPath}
                            </Typography>
                          </Stack>
                        </TableCell>
                        <TableCell>
                          <Chip label={record.decision} size="small" />
                        </TableCell>
                        <TableCell>
                          <Typography variant="body2">
                            {record.scan.videoCodec || 'unknown'} · {record.scan.audioTracks} audio · {record.scan.subtitleTracks} subs
                          </Typography>
                        </TableCell>
                        <TableCell sx={{ maxWidth: 320 }}>
                          <Typography color="text.secondary" variant="body2" sx={{ whiteSpace: 'pre-wrap' }}>
                            {record.notes || 'No notes'}
                          </Typography>
                        </TableCell>
                        <TableCell>{formatDate(record.createdAt)}</TableCell>
                        <TableCell align="right">
                          <Button
                            startIcon={<VisibilityIcon />}
                            variant="outlined"
                            size="small"
                            onClick={() => setSelectedRecord(record)}
                          >
                            Review
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </Box>
              {!settings.isLoading && records.length === 0 ? <Alert severity="info">No analysis records saved yet.</Alert> : null}
              {!settings.isLoading && records.length > 0 && filteredRecords.length === 0 ? <Alert severity="info">No analysis records match the current search.</Alert> : null}
            </Stack>
          </CardContent>
        </Card>
      </Box>
      <AnalysisRecordDialog
        record={selectedRecord}
        onClose={() => setSelectedRecord(null)}
        onLoad={loadRecord}
        onRescan={rescanRecord}
        isScanning={scanAsset.isPending}
      />
    </>
  );
}

function AnalysisRecordDialog({
  record,
  onClose,
  onLoad,
  onRescan,
  isScanning,
}: {
  record: AnalysisRecord | null;
  onClose: () => void;
  onLoad: (record: AnalysisRecord) => void;
  onRescan: (record: AnalysisRecord) => void;
  isScanning: boolean;
}) {
  return (
    <Dialog open={Boolean(record)} onClose={onClose} maxWidth="lg" fullWidth>
      <DialogTitle>Saved Analysis</DialogTitle>
      <DialogContent dividers>
        {record ? (
          <Stack spacing={2}>
            <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" spacing={1}>
              <Stack spacing={0.4} sx={{ minWidth: 0 }}>
                <Typography variant="h3" sx={{ wordBreak: 'break-word' }}>
                  {record.assetName}
                </Typography>
                <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                  {record.assetPath}
                </Typography>
              </Stack>
              <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                <Chip label={record.decision} color="warning" size="small" />
                <Chip label={formatDate(record.createdAt)} size="small" />
              </Stack>
            </Stack>
            <Grid container spacing={1.25}>
              <Grid size={{ xs: 12, md: 3 }}>
                <RecordMetric label="Video" value={record.scan.videoCodec || 'unknown'} />
              </Grid>
              <Grid size={{ xs: 12, md: 3 }}>
                <RecordMetric label="Audio tracks" value={String(record.scan.audioTracks)} />
              </Grid>
              <Grid size={{ xs: 12, md: 3 }}>
                <RecordMetric label="Subtitles" value={String(record.scan.subtitleTracks)} />
              </Grid>
              <Grid size={{ xs: 12, md: 3 }}>
                <RecordMetric label="Size" value={formatBytes(record.scan.sizeBytes)} />
              </Grid>
            </Grid>
            {record.notes ? (
              <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
                <Typography fontWeight={700} sx={{ mb: 0.5 }}>
                  Notes
                </Typography>
                <Typography color="text.secondary" sx={{ whiteSpace: 'pre-wrap' }}>
                  {record.notes}
                </Typography>
              </Box>
            ) : null}
            <MediaSnapshotDetails scan={record.scan} />
            <Stack direction="row" spacing={1} justifyContent="flex-end" flexWrap="wrap" useFlexGap>
              <Button variant="outlined" onClick={() => onLoad(record)}>
                Load in analysis form
              </Button>
              <Button startIcon={<SearchIcon />} variant="contained" onClick={() => onRescan(record)} disabled={isScanning}>
                Rescan
              </Button>
            </Stack>
          </Stack>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function RecordMetric({ label, value }: { label: string; value: string }) {
  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.25, height: '100%' }}>
      <Typography color="text.secondary" variant="body2">
        {label}
      </Typography>
      <Typography sx={{ mt: 0.4, wordBreak: 'break-word' }}>{value}</Typography>
    </Box>
  );
}

function AssetAutocomplete({ assets, value, onChange }: { assets: Asset[]; value: Asset | null; onChange: (asset: Asset | null) => void }) {
  return (
    <Autocomplete
      options={assets}
      value={value}
      onChange={(_, asset) => onChange(asset)}
      getOptionLabel={(asset) => asset.relativePath || asset.fileName}
      isOptionEqualToValue={(option, selected) => option.path === selected.path}
      filterOptions={(options, state) => {
        const query = state.inputValue.trim().toLowerCase();
        if (!query) {
          return options.slice(0, 50);
        }
        return options
          .filter((asset) =>
            [asset.fileName, asset.relativePath, asset.path].some((value) => value.toLowerCase().includes(query)),
          )
          .slice(0, 50);
      }}
      renderInput={(params) => <TextField {...params} label="Raw asset" />}
      renderOption={(props, asset) => (
        <Box component="li" {...props} key={asset.path}>
          <Stack sx={{ minWidth: 0 }}>
            <Typography fontWeight={700} noWrap>
              {asset.fileName}
            </Typography>
            <Typography color="text.secondary" variant="body2" noWrap>
              {asset.relativePath || asset.path}
            </Typography>
          </Stack>
        </Box>
      )}
      fullWidth
    />
  );
}

function ensureAssetOption(assets: Asset[], path: string): Asset[] {
  if (!path || assets.some((asset) => asset.path === path)) {
    return assets;
  }
  return [assetOptionFromPath(path), ...assets].filter((asset): asset is Asset => Boolean(asset));
}

function assetOptionFromPath(path: string): Asset | null {
  if (!path) {
    return null;
  }
  const fileName = path.split('/').filter(Boolean).pop() ?? path;
  return {
    libraryId: 0,
    libraryName: 'Queued job',
    path,
    relativePath: path,
    groupPath: '',
    fileName,
    extension: fileName.includes('.') ? `.${fileName.split('.').pop()}` : '',
    sizeBytes: 0,
    modifiedAt: new Date().toISOString(),
    status: 'unprocessed',
    missing: false,
    review: { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' },
    metadata: { categories: [], tags: [], updatedAt: '' },
    conversion: {},
  };
}

function getAnalysisRecords(settings?: AppSetting[]): AnalysisRecord[] {
  const value = settings?.find((setting) => setting.key === 'analysisRecords')?.value.records;
  if (!Array.isArray(value)) {
    return [];
  }

  return value.filter(isAnalysisRecord);
}

function isAnalysisRecord(value: unknown): value is AnalysisRecord {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const candidate = value as Partial<AnalysisRecord>;
  return Boolean(candidate.id && candidate.assetPath && candidate.assetName && candidate.scan && candidate.createdAt);
}

function filterAnalysisRecords(records: AnalysisRecord[], search: string) {
  const query = search.trim().toLowerCase();
  if (!query) {
    return records;
  }

  return records.filter((record) =>
    [
      record.assetName,
      record.assetPath,
      record.decision,
      record.notes,
      record.scan.videoCodec,
      String(record.scan.audioTracks),
      String(record.scan.subtitleTracks),
      formatDate(record.createdAt),
    ].some((value) => value.toLowerCase().includes(query)),
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(new Date(value));
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return 'Unknown';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}
