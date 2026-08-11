import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  Collapse,
  Dialog,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  Grid,
  IconButton,
  InputAdornment,
  LinearProgress,
  MenuItem,
  Slider,
  Stack,
  Switch,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TablePagination,
  TableRow,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import ManageSearchIcon from '@mui/icons-material/ManageSearch';
import PlayCircleIcon from '@mui/icons-material/PlayCircle';
import PlaylistAddIcon from '@mui/icons-material/PlaylistAdd';
import RefreshIcon from '@mui/icons-material/Refresh';
import ReportProblemIcon from '@mui/icons-material/ReportProblem';
import SearchIcon from '@mui/icons-material/Search';
import TaskAltIcon from '@mui/icons-material/TaskAlt';
import DeleteForeverIcon from '@mui/icons-material/DeleteForever';
import DriveFileMoveIcon from '@mui/icons-material/DriveFileMove';
import EditIcon from '@mui/icons-material/Edit';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Component, useEffect, useRef, useState } from 'react';
import type { ErrorInfo, MouseEvent, ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { MediaSnapshotDetails } from '../components/MediaSnapshotDetails';
import { PageHeader } from '../components/PageHeader';
import { ProfileSuggestionCard } from '../components/ProfileSuggestionCard';
import type { AdvisorResponse, AppSetting, Asset, AssetConversionOverrideState, AssetGroup, AssetInventory, AudioEnhancementProfile, ExternalSubtitle, Library, MediaStreamInfo, Profile, ProfileInput, ProfileSuggestion, QueueJob, ScanResult, SnapshotOperation, StreamMetadataOverride } from '../api/types';
import { getTrackProfiles, trackProfileOverride, type TrackProfile } from '../trackProfiles';
import { qsvQualityHelper, qsvQualityRangeForCrf } from '../utils/qsv';
import { applyHardwareQualityPreset as applySharedHardwareQualityPreset, hardwareQualityPresetOptions, qsvAssetQualitySummary } from '../utils/hardwareQualityPresets';
import { qsvSelectionWarnings, resolveQSVFeatures } from '../utils/qsvCapabilities';

export function AssetsPage() {
  const [tab, setTab] = useState<'unprocessed' | 'library' | 'converted' | 'archive' | 'reports'>('unprocessed');
  const [assetQuery, setAssetQuery] = useState('');
  const assets = useQuery({ queryKey: ['assets'], queryFn: api.assets });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const jobs = useQuery({ queryKey: ['queueJobs'], queryFn: api.queueJobs });
  const snapshotOperations = useQuery({
    queryKey: ['snapshotOperations'],
    queryFn: () => api.snapshotOperations(''),
    refetchInterval: 1500,
  });
  const queryClient = useQueryClient();
  const syncAssets = useMutation({
    mutationFn: api.syncAssets,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  //const operations = await api.subtitleExtractionOperations(asset.path);
  const audioProfiles = getAudioProfiles(settings.data);
  const trackProfiles = getTrackProfiles(settings.data);
  const assetCategories = getAssetCategories(settings.data);
  const unprocessedCount = safeArray(assets.data?.unprocessed).length;
  const libraryCount = safeArray(assets.data?.library).length;
  const convertedCount = safeArray(assets.data?.converted).length;
  const archiveCount = safeArray(assets.data?.archive).length;
  const currentGroups = safeArray(
    tab === 'archive' ? assets.data?.archiveGroups : tab === 'library' ? assets.data?.libraryGroups : tab === 'converted' ? assets.data?.convertedGroups : assets.data?.unprocessedGroups,
  );
  const filteredGroups = filterAssetGroups(currentGroups, assetQuery);
  const runningSnapshotPaths = new Set(
    (snapshotOperations.data?.operations ?? [])
      .filter((operation) => operation.status === 'running')
      .map((operation) => operation.assetPath),
  );

  return (
    <>
      <PageHeader title="Assets" eyebrow="Media inventory">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Library assets come from destination paths. Only outputs with MVForge job provenance are marked Converted; the rest remain Unverified.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {assets.isError ? <Alert severity="warning">Unable to read library paths from the backend container.</Alert> : null}

        <Card>
          <CardContent sx={{ py: 1.25 }}>
            <Stack
              direction={{ xs: 'column', lg: 'row' }}
              alignItems={{ xs: 'stretch', lg: 'flex-start' }}
              justifyContent="space-between"
              spacing={1.5}
            >
              <Tabs
                value={tab}
                onChange={(_, value) => {
                  setTab(value);
                  setAssetQuery('');
                }}
                variant="scrollable"
                scrollButtons="auto"
                sx={{ minHeight: 48, flexShrink: 0 }}
              >
                <Tab label={<AssetTabLabel label="Unprocessed" count={unprocessedCount} color="warning" />} value="unprocessed" />
                <Tab label={<AssetTabLabel label="Library" count={libraryCount} color="primary" />} value="library" />
                <Tab label={<AssetTabLabel label="Converted" count={convertedCount} color="success" />} value="converted" />
                <Tab label={<AssetTabLabel label="Archive" count={archiveCount} color="default" />} value="archive" />
                <Tab label="Reports" value="reports" />
              </Tabs>
              <Stack spacing={1} alignItems={{ xs: 'stretch', sm: 'flex-end' }} sx={{ flex: 1 }}>
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} justifyContent="flex-end" alignItems={{ xs: 'stretch', sm: 'center' }} sx={{ width: '100%' }}>
                  <Chip label={`Last sync: ${formatDate(assets.data?.sync?.lastSyncedAt ?? '')}`} size="small" />
                  <Button startIcon={<RefreshIcon />} variant="outlined" onClick={() => syncAssets.mutate()} disabled={syncAssets.isPending} sx={{ minHeight: 40 }}>
                    Sync
                  </Button>
                  {tab !== 'reports' ? (
                    <TextField
                      value={assetQuery}
                      onChange={(event) => setAssetQuery(event.target.value)}
                      placeholder="Search path, file, library, status"
                      size="small"
                      sx={{ width: { xs: '100%', sm: 330 } }}
                      InputProps={{
                        startAdornment: (
                          <InputAdornment position="start">
                            <SearchIcon fontSize="small" />
                          </InputAdornment>
                        ),
                      }}
                    />
                  ) : null}
                </Stack>
                {tab !== 'reports' ? (
                  <Stack direction="row" spacing={1} flexWrap="wrap" justifyContent={{ xs: 'flex-start', sm: 'flex-end' }} useFlexGap>
                    <Chip label={`${filteredGroups.length}/${currentGroups.length} groups`} size="small" />
                    <Chip label={`${sumGroupFiles(filteredGroups)} files`} size="small" />
                    <Chip label={formatBytes(sumGroupBytes(filteredGroups))} size="small" />
                    {(assets.data?.sync?.missingActionable ?? assets.data?.sync?.missingFiles ?? 0) > 0 ? <Chip label={`${assets.data?.sync?.missingActionable ?? assets.data?.sync?.missingFiles ?? 0} missing`} color="warning" size="small" /> : null}
                    {(assets.data?.sync?.missingHistorical ?? 0) > 0 ? <Chip label={`${assets.data?.sync?.missingHistorical ?? 0} historical paths`} size="small" /> : null}
                  </Stack>
                ) : null}
              </Stack>
            </Stack>
            {tab === 'converted' ? (
              <Alert severity="info" sx={{ mt: 1.25 }}>
                Converted assets can be inspected, moved to another library, or safely deleted and restored. Re-processing should start from Original Archive.
              </Alert>
            ) : null}
            {syncAssets.isSuccess ? (
              <Alert severity={syncAssets.data.reviewMatches > 0 ? 'warning' : 'success'} sx={{ mt: 1 }}>
                Inventory synced. {syncAssets.data.reconciledFiles} relocated asset(s) reconciled automatically.
                {syncAssets.data.reviewMatches > 0 ? ` ${syncAssets.data.reviewMatches} possible match(es) were sent to Review.` : ''}
              </Alert>
            ) : null}
            {tab === 'archive' ? (
              <Alert severity="info" sx={{ mt: 1 }}>
                Archived originals are protected here. Recovering an original will not delete converted files.
              </Alert>
            ) : null}
          </CardContent>
          {syncAssets.isError ? <Alert severity="warning" sx={{ m: 2 }}>Could not sync assets: {syncAssets.error.message}</Alert> : null}
          {tab === 'reports' ? (
            <AssetReportsPanel inventory={assets.data} />
          ) : (
            <AssetsErrorBoundary boundaryKey={tab}>
              <AssetTable
                key={tab}
                groups={currentGroups}
                libraries={libraries.data ?? []}
                profiles={profiles.data ?? []}
                audioProfiles={audioProfiles}
                trackProfiles={trackProfiles}
                settings={settings.data ?? []}
                assetCategories={assetCategories}
                queueJobs={jobs.data ?? []}
                runningSnapshotPaths={runningSnapshotPaths}
                mode={tab}
                query={assetQuery}
                emptyLabel={
                  tab === 'archive'
                    ? 'No archived originals found in the inventory.'
                    : tab === 'unprocessed'
                      ? 'No pending asset groups found.'
                      : tab === 'library' ? 'No library asset groups found.' : 'No MVForge-converted asset groups found.'
                }
              />
            </AssetsErrorBoundary>
          )}
        </Card>
      </Box>
    </>
  );
}

function AssetTabLabel({ label, count, color }: { label: string; count: number; color: 'warning' | 'success' | 'primary' | 'default' }) {
  return (
    <Stack direction="row" alignItems="center" spacing={1}>
      <Typography fontWeight={700}>{label}</Typography>
      <Chip label={count} color={color} size="small" />
    </Stack>
  );
}

function AssetReportsPanel({ inventory }: { inventory?: AssetInventory }) {
  const reports = inventory?.reports;
  if (!reports) {
    return (
      <CardContent>
        <Alert severity="info">Sync assets to build inventory reports.</Alert>
      </CardContent>
    );
  }
  const missingActionable = reports.missingActionable ?? reports.missingFiles ?? 0;
  const missingHistorical = reports.missingHistorical ?? 0;

  return (
    <CardContent>
      <Grid container spacing={1.5}>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <ReportTile label="Unprocessed" value={String(reports.unprocessedFiles)} />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <ReportTile label="Library assets" value={String(reports.libraryFiles)} helper={`${reports.unverifiedFiles} unverified`} />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <ReportTile label="Converted by MVForge" value={String(reports.convertedFiles)} />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <ReportTile label="Archive originals" value={String(reports.archiveFiles)} helper={formatBytes(reports.archiveBytes)} />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <ReportTile label="Needs attention" value={String(missingActionable + reports.expiredArchive)} helper={`${missingActionable} missing / ${reports.expiredArchive} expired`} />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <ReportTile label="Historical paths" value={String(missingHistorical)} helper="Moved, renamed, or archived records retained for provenance" />
        </Grid>
      </Grid>
      {safeArray(inventory?.missing).length ? (
        <Box sx={{ mt: 2, border: 1, borderColor: 'divider', borderRadius: 1, overflow: 'hidden' }}>
          <Box sx={{ px: 2, py: 1.5, bgcolor: 'rgba(255,255,255,0.02)' }}>
            <Typography fontWeight={700}>Missing assets requiring attention</Typography>
            <Typography color="text.secondary" variant="body2">
              These paths have no verified file of the same size and are not classified as renamed or archived history.
            </Typography>
          </Box>
          <Box sx={{ overflowX: 'auto' }}>
            <Table size="small" sx={{ minWidth: 760 }}>
              <TableHead>
                <TableRow>
                  <TableCell>Expected path</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>Source</TableCell>
                  <TableCell align="right">Recorded size</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {safeArray(inventory?.missing).map((asset) => (
                  <TableRow key={`${asset.status}-${asset.path}`}>
                    <TableCell sx={{ wordBreak: 'break-all' }}>{asset.path}</TableCell>
                    <TableCell><Chip label={statusLabel(asset.status)} color={statusColor(asset.status)} size="small" /></TableCell>
                    <TableCell>{asset.libraryName || 'Unknown'}</TableCell>
                    <TableCell align="right">{formatBytes(asset.sizeBytes)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Box>
        </Box>
      ) : null}
    </CardContent>
  );
}

function ReportTile({ label, value, helper }: { label: string; value: string; helper?: string }) {
  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5, height: '100%' }}>
      <Typography color="text.secondary" variant="body2">{label}</Typography>
      <Typography variant="h2" sx={{ mt: 0.5 }}>{value}</Typography>
      {helper ? <Typography color="text.secondary" variant="body2" sx={{ mt: 0.5 }}>{helper}</Typography> : null}
    </Box>
  );
}

const actionIconSx = {
  border: 1,
  borderColor: 'divider',
  width: 38,
  height: 38,
};

class AssetsErrorBoundary extends Component<
  { boundaryKey: string; children: ReactNode },
  { error: Error | null; boundaryKey: string }
> {
  state: { error: Error | null; boundaryKey: string } = { error: null, boundaryKey: this.props.boundaryKey };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  static getDerivedStateFromProps(
    props: { boundaryKey: string; children: ReactNode },
    state: { error: Error | null; boundaryKey: string },
  ) {
    if (props.boundaryKey !== state.boundaryKey) {
      return { error: null, boundaryKey: props.boundaryKey };
    }
    return null;
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Assets render error', error, info.componentStack);
  }

  render() {
    if (this.state.error) {
      return (
        <CardContent>
          <Alert severity="error">
            Assets could not render this path: {this.state.error.message}
          </Alert>
        </CardContent>
      );
    }
    return this.props.children;
  }
}

function AssetTable({
  groups,
  libraries,
  profiles,
  audioProfiles,
  trackProfiles,
  settings,
  assetCategories,
  queueJobs,
  runningSnapshotPaths,
  mode,
  query,
  emptyLabel,
}: {
  groups: AssetGroup[];
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  trackProfiles: TrackProfile[];
  settings: AppSetting[];
  assetCategories: string[];
  queueJobs: QueueJob[];
  runningSnapshotPaths: Set<string>;
  mode: 'unprocessed' | 'library' | 'converted' | 'archive';
  query: string;
  emptyLabel: string;
}) {
  const visibleGroups = safeArray(groups);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const filteredGroups = filterAssetGroups(visibleGroups, query);
  const pagedGroups = filteredGroups.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);
  const showConfidence = mode !== 'archive' && mode !== 'converted';

  useEffect(() => {
    setPage(0);
  }, [query, mode, visibleGroups.length]);

  if (visibleGroups.length === 0) {
    return (
      <CardContent>
        <Alert severity="info">{emptyLabel}</Alert>
      </CardContent>
    );
  }

  return (
    <>
      <Box sx={{ overflowX: { xs: 'hidden', sm: 'auto' }, borderTop: 1, borderColor: 'divider' }}>
        <Table size="small" sx={{ minWidth: { xs: 0, sm: 980 }, tableLayout: 'fixed', '& td, & th': { py: 0.85 } }}>
          <TableHead>
            <TableRow>
              <TableCell sx={{ width: { xs: '58%', sm: 390 } }}>Asset group</TableCell>
              <TableCell sx={{ width: 130, display: { xs: 'none', sm: 'table-cell' } }}>Library</TableCell>
              <TableCell sx={{ width: { xs: '32%', sm: 140 } }}>Status</TableCell>
              {showConfidence ? <TableCell sx={{ width: 130, display: { xs: 'none', sm: 'table-cell' } }}>Confidence</TableCell> : null}
              <TableCell sx={{ width: 70, display: { xs: 'none', md: 'table-cell' } }}>Files</TableCell>
              <TableCell sx={{ width: 120, display: { xs: 'none', md: 'table-cell' } }}>Size</TableCell>
              <TableCell sx={{ width: 165, display: { xs: 'none', md: 'table-cell' } }}>Modified</TableCell>
              <TableCell padding="checkbox" sx={{ width: { xs: '10%', sm: 52 } }} />
            </TableRow>
          </TableHead>
          <TableBody>
            {pagedGroups.length ? (
              pagedGroups.map((group) => (
                <AssetGroupRow
                  key={group.id}
                  group={group}
                  libraries={libraries}
                  profiles={profiles}
                  audioProfiles={audioProfiles}
                  trackProfiles={trackProfiles}
                  settings={settings}
                  assetCategories={assetCategories}
                  queueJobs={queueJobs}
                  runningSnapshotPaths={runningSnapshotPaths}
                  mode={mode}
                />
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={showConfidence ? 8 : 7}>
                  <Alert severity="info">No asset groups match this search.</Alert>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Box>
      <TablePagination
        component="div"
        count={filteredGroups.length}
        page={Math.min(page, Math.max(0, Math.ceil(filteredGroups.length / rowsPerPage) - 1))}
        rowsPerPage={rowsPerPage}
        rowsPerPageOptions={[10, 25, 50, 100]}
        onPageChange={(_, nextPage) => setPage(nextPage)}
        onRowsPerPageChange={(event) => {
          setRowsPerPage(Number(event.target.value));
          setPage(0);
        }}
        sx={{
          '& .MuiTablePagination-toolbar': { px: { xs: 1, sm: 2 } },
          '& .MuiTablePagination-selectLabel': { display: { xs: 'none', sm: 'block' } },
        }}
      />
    </>
  );
}

function AssetGroupRow({
  group,
  libraries,
  profiles,
  audioProfiles,
  trackProfiles,
  settings,
  assetCategories,
  queueJobs,
  runningSnapshotPaths,
  mode,
}: {
  group: AssetGroup;
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  trackProfiles: TrackProfile[];
  settings: AppSetting[];
  assetCategories: string[];
  queueJobs: QueueJob[];
  runningSnapshotPaths: Set<string>;
  mode: 'unprocessed' | 'library' | 'converted' | 'archive';
}) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [expanded, setExpanded] = useState(false);
  const [selectedProfileId, setSelectedProfileId] = useState<number>(profiles[0]?.id ?? 0);
  const [selectedAudioProfileKey, setSelectedAudioProfileKey] = useState<string>('');
  const pathTrackAssignments = getTrackProfilePathAssignments(settings);
  const [selectedTrackProfileKey, setSelectedTrackProfileKey] = useState<string>(pathTrackAssignments[normalizePath(group.path)] ?? '');
  const [selectedAssetPaths, setSelectedAssetPaths] = useState<string[]>([]);
  const groupAssets = safeArray(group.assets);
  const pathMetadata = group.pathMetadata ?? { categories: [], tags: [], updatedAt: '' };
  const groupReview = group.review ?? { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' };
  const [selectedLibraryId, setSelectedLibraryId] = useState<number>(group.libraryId);
  const [migrationLibraryId, setMigrationLibraryId] = useState<number>(0);
  const [groupCategory, setGroupCategory] = useState<string>(firstCategory(pathMetadata.categories));
  const effectiveProfileId = selectedProfileId < 0 ? 0 : selectedProfileId || profiles[0]?.id || 0;
  const representativeAsset = firstAssetForGroup(groupAssets.filter((asset) => !asset.missing));
  const isConvertedGroup = group.status === 'converted';
  const isPublishedAsIsGroup = group.status === 'published_as_is' || (groupAssets.length > 0 && groupAssets.every((asset) => asset.publicationMode === 'as_is'));
  const isLibraryGroup = group.status === 'unverified' || group.status === 'library' || group.status === 'published_as_is';
  const isArchiveGroup = mode === 'archive' || group.status === 'archive';
  const isReadOnlyGroup = isConvertedGroup || isPublishedAsIsGroup || isArchiveGroup;
  const showConfidenceColumn = mode !== 'archive' && mode !== 'converted';
  const bulkSelectableAssets = isReadOnlyGroup ? groupAssets.filter((asset) => !asset.missing) : [];
  const hasMultipleSelectableAssets = bulkSelectableAssets.length > 1;
  const allBulkAssetsSelected = bulkSelectableAssets.length > 0 && bulkSelectableAssets.every((asset) => selectedAssetPaths.includes(asset.path));
  const disabledConfidencePaths = getDisabledConfidencePaths(settings);
  const isConfidenceEnabled = !disabledConfidencePaths.includes(group.path);
  const updateSetting = useMutation({
    mutationFn: api.updateSetting,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });
  const updateMetadata = useMutation({
    mutationFn: api.updateAssetMetadata,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const migratePath = useMutation({
    mutationFn: api.migrateAssetPath,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
      ]);
    },
  });
  const publishAsIs = useMutation({
    mutationFn: api.publishAssetsAsIs,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
      ]);
    },
  });
  const advisor = useQuery({
    queryKey: ['advisor', group.id, representativeAsset?.path, effectiveProfileId],
    queryFn: () =>
      api.evaluateAdvisor({
        mediaPath: representativeAsset?.path ?? '',
        profileId: effectiveProfileId,
      }),
    enabled: expanded && !isReadOnlyGroup && isConfidenceEnabled && Boolean(effectiveProfileId && representativeAsset),
  });
  const queueGroup = useMutation({
    mutationFn: async () => {
      const trackProfile = trackProfiles.find((profile) => profile.key === selectedTrackProfileKey);
      const hasSelectedOperation = selectedProfileId > 0 || Boolean(selectedAudioProfileKey) || Boolean(trackProfile);
      const queueProfileId = effectiveProfileId || (hasSelectedOperation ? profiles[0]?.id ?? 0 : 0);
      const copyVideo = selectedProfileId < 0;
      if (!queueProfileId || !selectedLibraryId) {
        return [];
      }

      const batchId = createBatchId(group);
      const batchName = groupDisplayPath(group);
	  const validations = trackProfile ? await Promise.all(groupAssets.map(async (asset) => ({ asset, result: validateTrackProfile(trackProfile, await api.scan({ path: asset.path, force: false })) }))) : groupAssets.map((asset) => ({ asset, result: { applies: true, reasons: [] as string[] } }));
	  const incompatible = validations.filter(({ result }) => !result.applies);
	  if (trackProfile?.validationMode === 'block' && incompatible.length) {
	    throw new Error(`Track profile blocked this path: ${incompatible.map(({ asset, result }) => `${asset.fileName}: ${result.reasons.join(', ')}`).join(' · ')}`);
	  }
	  if (trackProfile?.validationMode === 'review') {
	    await Promise.all(incompatible.filter(({ asset }) => !assetReviewApproved(asset)).map(({ asset, result }) => api.updateAssetReview({ path: asset.path, requiresReview: true, source: 'track-profile', reason: result.reasons.join('; '), tags: ['track-profile-incompatible'] })));
	  }
	  const queueable = validations.filter(({ asset, result }) => result.applies || trackProfile?.validationMode === 'warn' || (trackProfile?.validationMode === 'review' && assetReviewApproved(asset)));
	  return Promise.all(queueable.map(async ({ asset, result }) => {
	    if (trackProfile) {
	      await api.updateAssetConversion({ path: asset.path, ...asset.conversion, ...trackProfileOverride(trackProfile), trackProfileKey: trackProfile.key });
	    }
	    return api.createQueueJob({
            mediaPath: asset.path,
            publishMode: isLibraryGroup ? 'replace_library_asset' : 'standard',
            batchId,
            batchName,
            libraryId: isLibraryGroup ? asset.libraryId : selectedLibraryId,
            profileId: queueProfileId,
            audioProfileKey: selectedAudioProfileKey,
            trackProfileKey: trackProfile?.key ?? '',
            processingMode: copyVideo ? 'audio_only' : 'full_encode',
            priority: priorityForSize(asset.sizeBytes),
            notes: queueNotes(`Queued from folder: ${batchName}${result.applies ? '' : `\nTrack profile ${trackProfile?.key} did not apply: ${result.reasons.join('; ')}`}`, selectedAudioProfileKey),
          });
	  }));
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
      ]);
      navigate(`/queue?path=${encodeURIComponent(group.path)}`);
    },
  });
  const bulkAssetAction = useMutation({
    mutationFn: async (paths: string[]) => {
      const failures: string[] = [];
      let completed = 0;
      for (const assetPath of paths) {
        try {
          if (isArchiveGroup) {
            await api.recoverAsset(assetPath);
          } else if (isPublishedAsIsGroup) {
            await api.returnPublishedAsIsAsset(assetPath);
          } else {
            await api.deleteConvertedAsset(assetPath);
          }
          completed += 1;
        } catch (error) {
          failures.push(`${assetPath}: ${error instanceof Error ? error.message : 'unknown error'}`);
        }
      }
      return { completed, failures };
    },
    onSettled: async () => {
      setSelectedAssetPaths([]);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
      ]);
    },
  });
  function toggleExpanded() {
    if (!selectedProfileId && profiles[0]) {
      setSelectedProfileId(profiles[0].id);
    }
    setExpanded((current) => !current);
  }

  function toggleConfidence(enabled: boolean) {
    const nextDisabledPaths = enabled
      ? disabledConfidencePaths.filter((path) => path !== group.path)
      : [...new Set([...disabledConfidencePaths, group.path])];

    updateSetting.mutate({
      key: 'advisorAutomationPathOverrides',
      value: { disabledPaths: nextDisabledPaths },
    });
  }

  function selectPathTrackProfile(key: string) {
	setSelectedTrackProfileKey(key);
	const assignments = { ...pathTrackAssignments };
	if (key) assignments[normalizePath(group.path)] = key;
	else delete assignments[normalizePath(group.path)];
	updateSetting.mutate({ key: 'trackProfilePathAssignments', value: { assignments } });
  }

  function toggleBulkAsset(path: string, selected: boolean) {
    setSelectedAssetPaths((current) => selected
      ? Array.from(new Set([...current, path]))
      : current.filter((candidate) => candidate !== path));
  }

  function runBulkAssetAction(paths = selectedAssetPaths) {
    if (!paths.length) return;
    const action = isArchiveGroup
      ? 'recover'
      : isPublishedAsIsGroup
        ? 'return to Raw'
        : 'safely delete and restore';
    const confirmed = window.confirm(
      `${action === 'recover' ? 'Recover' : isPublishedAsIsGroup ? 'Return' : 'Safely delete'} ${paths.length} asset(s) from this path? ${
        isArchiveGroup
          ? 'Converted files will not be deleted.'
          : isPublishedAsIsGroup
            ? 'Each Published as-is asset and its external subtitles will return to the original Raw path.'
            : 'Each converted file will be deleted only if its archived original can be restored to Raw.'
      }`,
    );
    if (confirmed) bulkAssetAction.mutate(paths);
  }

  const migrationControls = (isLibraryGroup || isConvertedGroup) ? (
    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }}>
      <Box sx={{ width: { xs: '100%', sm: 280 } }}>
        <LibraryAutocomplete
          libraries={libraries.filter((library) => library.id !== group.libraryId)}
          value={migrationLibraryId}
          onChange={setMigrationLibraryId}
          label="Move path to library"
          size="small"
        />
      </Box>
      <Button
        variant="outlined"
        size="small"
        startIcon={<DriveFileMoveIcon />}
        disabled={!migrationLibraryId || migratePath.isPending || groupAssets.some((asset) => assetHasOpenJob(asset, queueJobs))}
        onClick={() => {
          const destination = libraries.find((library) => library.id === migrationLibraryId);
          if (!destination || !window.confirm(`Move ${group.path} and all files inside it to ${destination.name}? MVForge will update inventory and publication paths.`)) return;
          migratePath.mutate({ sourcePath: group.path, destinationLibraryId: migrationLibraryId });
        }}
        sx={{ minHeight: 40, whiteSpace: 'nowrap' }}
      >
        {migratePath.isPending ? 'Moving...' : 'Move path'}
      </Button>
    </Stack>
  ) : null;

  return (
    <>
      <TableRow
        hover
        onClick={toggleExpanded}
        onKeyDown={(event) => {
          if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            toggleExpanded();
          }
        }}
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        sx={{ cursor: 'pointer' }}
      >
        <TableCell>
          <Stack spacing={0.25} sx={{ minWidth: 0 }}>
            <Typography fontWeight={700} sx={{ wordBreak: 'break-word', lineHeight: 1.25 }}>
              {groupTitle(group)}
            </Typography>
            <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all', lineHeight: 1.25 }}>
              {groupSubpath(group)}
            </Typography>
          </Stack>
        </TableCell>
        <TableCell sx={{ display: { xs: 'none', sm: 'table-cell' } }}>{group.libraryName}</TableCell>
        <TableCell>
          <Stack spacing={0.75} alignItems="flex-start">
            <Chip
              label={groupAssets.every((asset) => asset.publicationMode === 'as_is') ? 'Published as-is' : statusLabel(group.status)}
              color={groupAssets.every((asset) => asset.publicationMode === 'as_is') ? 'success' : statusColor(group.status)}
              size="small"
            />
            {groupReview.requiresReview ? <Chip label="Some need review" color="error" size="small" /> : null}
          </Stack>
        </TableCell>
        {showConfidenceColumn ? (
          <TableCell sx={{ display: { xs: 'none', sm: 'table-cell' } }}>
            {!isReadOnlyGroup ? (
              <Stack direction="row" spacing={1} alignItems="center" onClick={(event) => event.stopPropagation()}>
                <Switch
                  checked={isConfidenceEnabled}
                  onChange={(event) => toggleConfidence(event.target.checked)}
                  disabled={updateSetting.isPending}
                  size="small"
                />
                <Chip
                  label={isConfidenceEnabled ? 'On' : 'Off'}
                  color={isConfidenceEnabled ? 'success' : 'default'}
                  size="small"
                />
              </Stack>
            ) : null}
          </TableCell>
        ) : null}
        <TableCell sx={{ display: { xs: 'none', md: 'table-cell' } }}>{group.fileCount}</TableCell>
        <TableCell sx={{ display: { xs: 'none', md: 'table-cell' } }}>{formatBytes(group.sizeBytes)}</TableCell>
        <TableCell sx={{ display: { xs: 'none', md: 'table-cell' } }}>{formatDate(group.modifiedAt)}</TableCell>
        <TableCell padding="checkbox" align="center">
          <ExpandMoreIcon
            aria-hidden
            sx={{
              color: 'text.secondary',
              transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
              transition: (theme) => theme.transitions.create('transform', { duration: theme.transitions.duration.shortest }),
            }}
          />
        </TableCell>
      </TableRow>
      <TableRow>
        <TableCell colSpan={showConfidenceColumn ? 8 : 7} sx={{ p: 0, borderBottom: expanded ? 1 : 0, borderColor: 'divider', maxWidth: 0 }}>
          <Collapse in={expanded} timeout="auto" unmountOnExit>
            <Box sx={{ bgcolor: 'rgba(255,255,255,0.02)', px: { xs: 1.5, md: 2 }, py: 2, width: '100%', maxWidth: '100%', overflow: 'hidden' }}>
              <Stack spacing={2}>
                {isArchiveGroup ? null : isConvertedGroup ? null : isPublishedAsIsGroup ? (
                  <Alert severity="info">
                    Published as-is assets must be returned to Raw before they can be queued for conversion.
                  </Alert>
                ) : (
                  <Grid container spacing={2} alignItems="stretch">
                    <Grid size={{ xs: 12, md: 2 }}>
                      <AssetCategorySelect
                        value={groupCategory}
                        options={assetCategories}
                        onChange={(category) => {
                          setGroupCategory(category);
                          updateMetadata.mutate({ path: group.path, categories: category ? [category] : [], tags: safeArray(pathMetadata.tags) });
                        }}
                        label="Category"
                        size="small"
                      />
                    </Grid>
                    <Grid size={{ xs: 12, md: 2 }}>
                      <ProfileAutocomplete profiles={profiles} value={selectedProfileId < 0 ? -1 : effectiveProfileId} onChange={setSelectedProfileId} label="Video profile" size="small" allowNone />
                    </Grid>
                    <Grid size={{ xs: 12, md: 2 }}>
                      <AudioProfileAutocomplete
                        profiles={audioProfiles}
                        value={selectedAudioProfileKey}
                        onChange={setSelectedAudioProfileKey}
                        label="Audio profile"
                        size="small"
                      />
                    </Grid>
                    <Grid size={{ xs: 12, md: 2 }}>
                      <TrackProfileAutocomplete profiles={trackProfiles} value={selectedTrackProfileKey} onChange={selectPathTrackProfile} disabled={updateSetting.isPending} />
                    </Grid>
                    <Grid size={{ xs: 12, md: 2 }}>
                      <LibraryAutocomplete libraries={libraries} value={isLibraryGroup ? group.libraryId : selectedLibraryId} onChange={setSelectedLibraryId} label="Destination library" size="small" disabled={isLibraryGroup} />
                    </Grid>
                    <Grid size={{ xs: 12, md: 2 }}>
                      <Button
                        startIcon={<PlaylistAddIcon />}
                        variant="contained"
                        size="small"
                        onClick={() => {
                          if (isLibraryGroup && !window.confirm(`Convert and safely replace ${groupAssets.length} Library asset(s) after validation? Every original will be archived.`)) return;
                          queueGroup.mutate();
                        }}
                        disabled={
                          queueGroup.isPending ||
                          (!effectiveProfileId && !selectedAudioProfileKey && !selectedTrackProfileKey) ||
                          !selectedLibraryId ||
                          groupAssets.length === 0 ||
                          groupAssets.some((asset) => asset.review?.requiresReview || assetHasOpenJob(asset, queueJobs))
                        }
                        fullWidth
                        sx={{ minHeight: 40, alignSelf: 'center' }}
                      >
                        Queue Folder
                      </Button>
                    </Grid>
                  </Grid>
                )}
                {!isReadOnlyGroup && !isConfidenceEnabled ? (
                  <Alert severity="warning">
                    Confidence is off for this path. Advisor checks and any future confidence-based automation will be skipped here; manual queueing still works.
                  </Alert>
                ) : null}
                {groupReview.requiresReview ? (
                  <Alert severity="warning">
                    Folder queue is blocked because at least one asset in this path needs review. You can still queue approved assets individually.
                  </Alert>
                ) : null}
                {!isReadOnlyGroup && advisor.isError && representativeAsset ? <Alert severity="warning">Could not evaluate this path: {advisor.error instanceof Error ? advisor.error.message : 'unknown error'}</Alert> : null}
                {!representativeAsset && groupAssets.length > 0 ? <Alert severity="warning">This path has no physically available asset to evaluate. Run Sync Assets after restoring or removing stale records.</Alert> : null}
                {queueGroup.isSuccess ? <Alert severity="success">{groupAssets.length} files queued from this folder.</Alert> : null}
                {queueGroup.isError ? <Alert severity="warning">{queueGroup.error instanceof Error ? queueGroup.error.message : 'Could not queue this folder.'}</Alert> : null}
                {mode === 'unprocessed' && !isReadOnlyGroup ? (
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} justifyContent="flex-end" alignItems={{ xs: 'stretch', sm: 'center' }}>
                    <Typography color="text.secondary" variant="body2">
                      No conversion required: validate and move these original files directly to the selected Library.
                    </Typography>
                    <Button
                      variant="outlined"
                      color="success"
                      size="small"
                      disabled={!selectedLibraryId || publishAsIs.isPending || groupAssets.length === 0 || groupAssets.some((asset) => asset.review?.requiresReview || assetHasOpenJob(asset, queueJobs))}
                      onClick={() => {
                        const destination = libraries.find((library) => library.id === selectedLibraryId);
                        if (!destination || !window.confirm(`Publish ${groupAssets.length} original asset(s) as-is to ${destination.name}? FFmpeg will not run and the files will be moved from Raw to Library.`)) return;
                        publishAsIs.mutate({ sourcePath: group.path, destinationLibraryId: selectedLibraryId });
                      }}
                      sx={{ minHeight: 40, whiteSpace: 'nowrap' }}
                    >
                      {publishAsIs.isPending ? 'Publishing...' : 'Publish as-is'}
                    </Button>
                  </Stack>
                ) : null}
                {publishAsIs.isSuccess ? <Alert severity="success">{publishAsIs.data.message}</Alert> : null}
                {publishAsIs.isError ? <Alert severity="warning">{publishAsIs.error instanceof Error ? publishAsIs.error.message : 'Direct publication failed.'}</Alert> : null}
                {isLibraryGroup ? (
                  <Stack direction="row" justifyContent="flex-end">
                    {migrationControls}
                  </Stack>
                ) : null}
                {isReadOnlyGroup && hasMultipleSelectableAssets ? (
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between">
                    <Typography color="text.secondary" variant="body2">
                      {selectedAssetPaths.length} of {bulkSelectableAssets.length} selected in this path
                    </Typography>
                    <Stack direction={{ xs: 'column', md: 'row' }} spacing={1} alignItems={{ xs: 'stretch', md: 'center' }}>
                      {isConvertedGroup ? migrationControls : null}
                      <Button
                        color={isArchiveGroup ? 'primary' : 'error'}
                        variant="outlined"
                        size="small"
                        startIcon={isArchiveGroup ? <TaskAltIcon /> : <DeleteForeverIcon />}
                        disabled={!selectedAssetPaths.length || bulkAssetAction.isPending}
                        onClick={() => runBulkAssetAction()}
                        sx={{ minHeight: 40, whiteSpace: 'nowrap' }}
                      >
                        {bulkAssetAction.isPending
                          ? 'Processing...'
                          : isArchiveGroup
                            ? 'Recover selected'
                            : isPublishedAsIsGroup
                              ? 'Return selected'
                              : 'Delete selected'}
                      </Button>
                      {!isArchiveGroup && (
                        <Button
                          color="error"
                          variant="contained"
                          size="small"
                          startIcon={<DeleteForeverIcon />}
                          disabled={bulkAssetAction.isPending || bulkSelectableAssets.some((asset) => assetHasOpenJob(asset, queueJobs))}
                          onClick={() => runBulkAssetAction(bulkSelectableAssets.map((asset) => asset.path))}
                          sx={{ minHeight: 40, whiteSpace: 'nowrap' }}
                        >
                          {bulkAssetAction.isPending
                            ? 'Processing...'
                            : isPublishedAsIsGroup
                              ? 'Return path to Raw'
                              : 'Delete path'}
                        </Button>
                      )}
                    </Stack>
                  </Stack>
                ) : isConvertedGroup ? (
                  <Stack direction="row" justifyContent="flex-end">
                    {migrationControls}
                  </Stack>
                ) : null}
                {migratePath.isSuccess ? <Alert severity="success">Path moved to {migratePath.data.destinationPath}. {migratePath.data.assetsMoved} asset(s) reconciled.</Alert> : null}
                {migratePath.isError ? <Alert severity="warning">Path migration failed: {migratePath.error instanceof Error ? migratePath.error.message : 'unknown error'}</Alert> : null}
                {bulkAssetAction.data ? (
                  <Alert severity={bulkAssetAction.data.failures.length ? 'warning' : 'success'}>
                    {bulkAssetAction.data.completed} asset(s) completed.
                    {bulkAssetAction.data.failures.length ? ` ${bulkAssetAction.data.failures.length} failed: ${bulkAssetAction.data.failures.join(' · ')}` : ''}
                  </Alert>
                ) : null}
                <Box sx={{ width: '100%', maxWidth: '100%', overflowX: 'auto', pb: 0.5 }}>
                  <Table
                    size="small"
                    sx={{
                      width: '100%',
                      minWidth: isConvertedGroup ? 1120 : isReadOnlyGroup ? 820 : 1320,
                      tableLayout: 'fixed',
                    }}
                  >
                    <TableHead>
                      <TableRow>
                        {isReadOnlyGroup && hasMultipleSelectableAssets ? (
                          <TableCell padding="checkbox" sx={{ width: 48 }}>
                            <Checkbox
                              size="small"
                              checked={allBulkAssetsSelected}
                              indeterminate={selectedAssetPaths.length > 0 && !allBulkAssetsSelected}
                              disabled={!bulkSelectableAssets.length || bulkAssetAction.isPending}
                              onChange={(event) => setSelectedAssetPaths(event.target.checked ? bulkSelectableAssets.map((asset) => asset.path) : [])}
                              inputProps={{ 'aria-label': `Select all assets in ${group.path}` }}
                            />
                          </TableCell>
                        ) : null}
                        <TableCell sx={{ width: 230 }}>Asset</TableCell>
                        <TableCell sx={{ width: 198 }}>Status</TableCell>
                        {!isReadOnlyGroup ? <TableCell sx={{ width: 95 }}>Score</TableCell> : null}
                        {isConvertedGroup ? <TableCell sx={{ width: 360 }}>Media</TableCell> : null}
                        <TableCell sx={{ width: 100 }}>Size</TableCell>
                        {mode !== 'unprocessed' ? <TableCell sx={{ width: 128 }}>Modified</TableCell> : null}
                        {mode === 'unprocessed' ? <TableCell sx={{ width: 145 }}>Category</TableCell> : null}
                        {!isReadOnlyGroup ? <TableCell sx={{ width: 165 }}>Video profile</TableCell> : null}
                        {!isReadOnlyGroup ? <TableCell sx={{ width: 165 }}>Audio profile</TableCell> : null}
                        {mode === 'unprocessed' ? <TableCell sx={{ width: 165 }}>Tracks profile</TableCell> : null}
                        {!isReadOnlyGroup ? <TableCell sx={{ width: 160 }}>Destination</TableCell> : null}
                        <TableCell align="center" sx={{ width: 180 }}>Actions</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {groupAssets.map((asset) => (
                        <AssetRow
                          key={`${asset.status}-${asset.libraryId}-${asset.path}-${selectedProfileId}-${selectedLibraryId}`}
                          asset={asset}
                          libraries={libraries}
                          profiles={profiles}
                          audioProfiles={audioProfiles}
                          trackProfiles={trackProfiles}
                          pathTrackProfile={trackProfiles.find((profile) => profile.key === selectedTrackProfileKey)}
                          assetCategories={assetCategories}
                          groupRelativePath={group.relativePath}
                          groupCategory={groupCategory}
                          confidenceEnabled={isConfidenceEnabled}
                          groupProfileId={selectedProfileId < 0 ? -1 : effectiveProfileId}
                          groupAudioProfileKey={selectedAudioProfileKey}
                          groupLibraryId={selectedLibraryId}
                          hasOpenJob={queueGroup.isPending || assetHasOpenJob(asset, queueJobs)}
                          queueJobs={queueJobs}
                          snapshotRunning={runningSnapshotPaths.has(asset.path)}
                          mode={mode}
                          bulkSelected={selectedAssetPaths.includes(asset.path)}
                          bulkSelectionEnabled={isReadOnlyGroup && hasMultipleSelectableAssets}
                          bulkSelectionDisabled={asset.missing || bulkAssetAction.isPending}
                          onBulkSelectionChange={toggleBulkAsset}
                        />
                      ))}
                    </TableBody>
                  </Table>
                </Box>
              </Stack>
            </Box>
          </Collapse>
        </TableCell>
      </TableRow>
    </>
  );
}

function AssetRow({
  asset,
  libraries,
  profiles,
  audioProfiles,
  trackProfiles,
  pathTrackProfile,
  assetCategories,
  groupRelativePath,
  groupCategory,
  confidenceEnabled,
  groupProfileId,
  groupAudioProfileKey,
  groupLibraryId,
  hasOpenJob,
  queueJobs,
  snapshotRunning,
  mode,
  bulkSelected,
  bulkSelectionEnabled,
  bulkSelectionDisabled,
  onBulkSelectionChange,
}: {
  asset: Asset;
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  trackProfiles: TrackProfile[];
  pathTrackProfile?: TrackProfile;
  assetCategories: string[];
  groupRelativePath: string;
  groupCategory: string;
  confidenceEnabled: boolean;
  groupProfileId: number;
  groupAudioProfileKey: string;
  groupLibraryId: number;
  hasOpenJob: boolean;
  queueJobs: QueueJob[];
  snapshotRunning: boolean;
  mode: 'unprocessed' | 'library' | 'converted' | 'archive';
  bulkSelected: boolean;
  bulkSelectionEnabled: boolean;
  bulkSelectionDisabled: boolean;
  onBulkSelectionChange: (path: string, selected: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const [selectedProfileId, setSelectedProfileId] = useState<number>(() => relatedVideoProfile(profiles, asset)?.id ?? groupProfileId);
  const [selectedAudioProfileKey, setSelectedAudioProfileKey] = useState<string>(() => relatedAudioProfile(audioProfiles, asset)?.key ?? groupAudioProfileKey);
  const [selectedTrackProfileKey, setSelectedTrackProfileKey] = useState<string>(() => relatedTrackProfile(trackProfiles, asset)?.key ?? pathTrackProfile?.key ?? '');
  const selectedTrackProfile = trackProfiles.find((profile) => profile.key === selectedTrackProfileKey);
  const [selectedLibraryId, setSelectedLibraryId] = useState<number>(groupLibraryId);
  const [showSnapshotDialog, setShowSnapshotDialog] = useState(false);
  const [snapshotTab, setSnapshotTab] = useState(0);
  const [renameFileName, setRenameFileName] = useState(asset.fileName);
  const [snapshotOperation, setSnapshotOperation] = useState<SnapshotOperation | null>(null);
  const [editingSubtitle, setEditingSubtitle] = useState<ExternalSubtitle | null>(null);
  const [subtitleContent, setSubtitleContent] = useState('');
  const [subtitleGenerations, setSubtitleGenerations] = useState<Record<string, SubtitleGenerationState>>({});
  const subtitleOperationPollers = useRef<Set<string>>(new Set());
  const [showPreviewDialog, setShowPreviewDialog] = useState(false);
  const [showAdvisorDialog, setShowAdvisorDialog] = useState(false);
  const [previewMode, setPreviewMode] = useState<'compatible' | 'original'>('compatible');
  const assetReview = asset.review ?? { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' };
  const reconciliationJobId = reconciliationJobIdFromReview(assetReview);
  const assetMetadata = asset.metadata ?? { categories: [], tags: [], updatedAt: '' };
  const [reviewReason, setReviewReason] = useState(assetReview.reason || '');
  const [reviewTags, setReviewTags] = useState<string[]>(safeArray(assetReview.tags));
  const [category, setCategory] = useState<string>(firstCategory(assetMetadata.categories) || groupCategory);
  const [conversionDraft, setConversionDraft] = useState<AssetConversionOverrideState>(() => normalizeAssetConversionOverride(asset.conversion));
  const profileSuggestion = useMutation({ mutationFn: api.suggestProfile });
  const snapshot = useMutation({
    mutationFn: async (input: { path: string; force?: boolean; analysisSeconds?: 10 | 20 }) => {
      let operation = await api.startSnapshotOperation(input);
      setSnapshotOperation(operation);
      while (operation.status === 'running') {
        await new Promise((resolve) => window.setTimeout(resolve, 750));
        operation = await api.snapshotOperation(operation.id);
        setSnapshotOperation(operation);
      }
      if (operation.status === 'error' || !operation.result) {
        throw new Error(operation.error || 'Asset snapshot did not return a result');
      }
      return operation.result;
    },
    onSuccess: async (scan) => {
      if (asset.status !== 'converted' && asset.status !== 'archive') {
        profileSuggestion.mutate(scan.path);
      }
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
      await queryClient.invalidateQueries({ queryKey: ['snapshotOperations'] });
    },
  });
  const externalSubtitles = useQuery({
    queryKey: ['externalSubtitles', asset.path],
    queryFn: () => api.externalAssetSubtitles(asset.path),
    enabled: showSnapshotDialog && asset.status !== 'archive' && !asset.missing,
  });
  const advisor = useQuery({
    queryKey: ['advisor', 'asset-row', asset.path, selectedProfileId],
    queryFn: () => api.evaluateAdvisor({ mediaPath: asset.path, profileId: selectedProfileId }),
    enabled: mode !== 'archive' && mode !== 'converted' && asset.status !== 'archive' && asset.status !== 'converted' && asset.status !== 'published_as_is' && confidenceEnabled && selectedProfileId > 0,
  });
  const createJob = useMutation({
    mutationFn: async (input: Parameters<typeof api.createQueueJob>[0]) => {
      if (selectedTrackProfile) {
        const result = validateTrackProfile(selectedTrackProfile, await api.scan({ path: asset.path, force: false }));
        if (!result.applies && selectedTrackProfile.validationMode === 'review' && !assetReviewApproved(asset)) {
          await api.updateAssetReview({ path: asset.path, requiresReview: true, source: 'track-profile', reason: result.reasons.join('; '), tags: ['track-profile-incompatible'] });
          throw new Error(`Track profile does not apply; asset marked for review: ${result.reasons.join('; ')}`);
        }
        if (!result.applies && selectedTrackProfile.validationMode === 'block') {
          throw new Error(`Track profile blocked this asset: ${result.reasons.join('; ')}`);
        }
        await api.updateAssetConversion({ path: asset.path, ...conversionDraft, ...trackProfileOverride(selectedTrackProfile), trackProfileKey: selectedTrackProfile.key });
        if (!result.applies) {
          input = { ...input, notes: `${input.notes ?? ''}\nTrack profile ${selectedTrackProfile.key} did not apply: ${result.reasons.join('; ')}`.trim() };
        }
      } else if (hasTrackSelectionOverride(conversionDraft)) {
        await api.updateAssetConversion({
          path: asset.path,
          ...cleanConversionOverride(conversionDraft),
        });
      }
      return api.createQueueJob(input);
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
      ]);
    },
  });
  const updateReview = useMutation({
    mutationFn: api.updateAssetReview,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const reconcilePublication = useMutation({
    mutationFn: api.confirmPublicationReconciliation,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
      ]);
    },
  });
  const updateMetadata = useMutation({
    mutationFn: api.updateAssetMetadata,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const renameAsset = useMutation({
    mutationFn: api.renameAsset,
    onSuccess: async () => {
      setShowSnapshotDialog(false);
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const updateConversion = useMutation({
    mutationFn: api.updateAssetConversion,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const recoverAsset = useMutation({
    mutationFn: api.recoverAsset,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const deleteConvertedAsset = useMutation({
    mutationFn: api.deleteConvertedAsset,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
      ]);
    },
  });
  const returnPublishedAsIs = useMutation({
    mutationFn: api.returnPublishedAsIsAsset,
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
      ]);
    },
  });
  const loadSubtitleContent = useMutation({
    mutationFn: api.externalAssetSubtitleContent,
    onSuccess: (result) => setSubtitleContent(result.content),
  });
  const saveSubtitleContent = useMutation({
    mutationFn: api.updateExternalAssetSubtitle,
    onSuccess: async () => {
      setEditingSubtitle(null);
      setSubtitleContent('');
      await queryClient.invalidateQueries({ queryKey: ['externalSubtitles', asset.path] });
    },
  });
  const deleteExternalSubtitle = useMutation({
    mutationFn: api.deleteExternalAssetSubtitle,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['externalSubtitles', asset.path] });
    },
  });
  const isBlockedByReview = assetReview.requiresReview;
  const isConverted = asset.status === 'converted';
  const isPublishedAsIs = asset.status === 'published_as_is';
  const isLibraryReplacement = asset.status === 'unverified' || asset.status === 'library' || asset.status === 'published_as_is';
  const isArchive = mode === 'archive' || asset.status === 'archive';
  const rowColumnCount = isConverted
    ? bulkSelectionEnabled ? 8 : 7
    : isArchive
      ? bulkSelectionEnabled ? 7 : 6
      : 10;
  const associatedJob = associatedJobForAsset(asset, queueJobs);
  const rowLocked = hasOpenJob || createJob.isPending || (isConverted && !isLibraryReplacement) || isArchive || isPublishedAsIs;
  const pipelineState = assetPipelineState(asset, associatedJob, createJob.isPending);
  const canQueueWithSelection = selectedProfileId > 0 || Boolean(selectedAudioProfileKey) || Boolean(selectedTrackProfile) || hasTrackSelectionOverride(conversionDraft);
  useEffect(() => {
    if (!showSnapshotDialog || asset.status === 'archive' || asset.missing) {
      return;
    }

  void restoreSubtitleOperations();
  }, [showSnapshotDialog, asset.path]);
  useEffect(() => {
    if (!firstCategory(assetMetadata.categories)) {
      setCategory(groupCategory);
    }
  }, [assetMetadata.categories, groupCategory]);

  useEffect(() => {
    setConversionDraft(normalizeAssetConversionOverride(asset.conversion));
  }, [asset.conversion]);

  function openSnapshotDialog(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    setSnapshotTab(0);
    setShowSnapshotDialog(true);
    if (!asset.missing && !snapshot.data && !snapshot.isPending) {
      snapshot.mutate({ path: asset.path });
    }
  }

  function openPreviewDialog(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    setPreviewMode('compatible');
    setShowPreviewDialog(true);
  }

  function refreshSnapshot() {
    if (asset.missing) {
      return;
    }
    snapshot.mutate({ path: asset.path, force: true });
  }

  async function applySnapshotRecommendations(suggestion: ProfileSuggestion) {
    const scan = suggestion.scan;
    const proposed = suggestion.proposedProfile;
    const workerConfig = proposed.workerConfig ?? {};
    const status = scan.interlaceAnalysis.status;
    const withoutMotion = withoutMotionFilters(conversionDraft.videoFilters);
    const recommendedMotionFilter = scan.interlaceAnalysis.recommendedFilter || 'fieldmatch,decimate';
    const motionPatch: AssetConversionOverrideState = status === 'progressive'
      ? {
          deinterlaceMode: 'off',
          videoFilters: withoutMotion,
        }
      : status === 'interlaced'
        ? { deinterlaceMode: 'force', videoFilters: withoutMotion }
        : status === 'telecine_suspected'
          ? {
              deinterlaceMode: scan.interlaceAnalysis.recommendedMode === 'ivtc_bff' ? 'ivtc_bff' : 'ivtc_tff',
              videoFilters: joinFilters(recommendedMotionFilter, withoutMotion),
            }
          : {};
    const hardwareEnabled = workerConfig.useHardwareIfAvailable === true;
    const next = cleanConversionOverride({
      ...conversionDraft,
      videoCodec: proposed.videoCodec,
      audioCodec: proposed.audioCodec,
      qualityMode: proposed.qualityMode,
      qualityValue: suggestion.insights.recommendedCrf || proposed.qualityValue,
      videoPreset: stringFromRecord(workerConfig, 'videoPreset'),
      pixFmt: stringFromRecord(workerConfig, 'pixFmt') || stringFromRecord(workerConfig, 'pixelFormat'),
      preserveHdr: proposed.preserveHdr,
      preserveSubtitles: proposed.preserveSubtitles,
      preserveChapters: proposed.preserveChapters,
      addAacStereoTrack: typeof workerConfig.addAacStereoTrack === 'boolean' ? workerConfig.addAacStereoTrack : conversionDraft.addAacStereoTrack,
      aacStereoDefault: typeof workerConfig.addAacStereoDefault === 'boolean' ? workerConfig.addAacStereoDefault : conversionDraft.aacStereoDefault,
      useHardwareIfAvailable: hardwareEnabled,
      preferredEncoder: hardwareEnabled
        ? (stringFromRecord(workerConfig, 'preferredEncoder') === 'auto' ? 'auto' : 'hardware')
        : 'software',
      videoEncoder: stringFromRecord(workerConfig, 'videoEncoder') || 'auto',
      globalQuality: Number(workerConfig.globalQuality || qsvQualityRangeForCrf(suggestion.insights.recommendedCrf || proposed.qualityValue).recommended),
      qsvRateControl: stringFromRecord(workerConfig, 'qsvRateControl') === 'la_icq' ? 'la_icq' : 'icq',
      qsvLookAheadDepth: Number(workerConfig.qsvLookAheadDepth || 40),
      qsvExtendedBrc: workerConfig.qsvExtendedBRC === true,
      qsvAdaptiveI: workerConfig.qsvAdaptiveI === true,
      qsvAdaptiveB: workerConfig.qsvAdaptiveB === true,
      ...motionPatch,
    });
    await updateConversion.mutateAsync({ path: asset.path, ...next });
    setConversionDraft(next);
    setSnapshotTab(5);
    return 'Recommendations were saved to Asset Overrides.';
  }

  function toggleSnapshotStream(type: MediaStreamInfo['type'], index: number, keep: boolean) {
    const scan = snapshot.data;
    if (!scan) {
      return;
    }
    const allIndexes = streamIndexesForType(scan, type);
    const current = conversionStreamIndexes(conversionDraft, scan, type);
    const next = keep ? normalizeNumberList([...current, index]) : safeArray(current).filter((candidate) => candidate !== index);
    updateConversionDraftStream(type, selectedOrUndefined(next, allIndexes));
  }

  function updateConversionDraftStream(type: MediaStreamInfo['type'], indexes: number[] | undefined) {
    setConversionDraft((current) => {
      if (type === 'video') {
        return { ...current, keepVideoStreams: indexes };
      }
      if (type === 'audio') {
        return { ...current, keepAudioStreams: indexes };
      }
      return { ...current, keepSubtitleStreams: indexes };
    });
  }

  function updateConversionDraft<K extends keyof AssetConversionOverrideState>(key: K, value: AssetConversionOverrideState[K]) {
    const customHardwareControl = ['globalQuality', 'qsvRateControl', 'qsvLookAheadDepth', 'qsvExtendedBrc', 'qsvAdaptiveI', 'qsvAdaptiveB', 'videoToolboxBitrateMbps', 'videoToolboxMaxrateMbps', 'videoToolboxBufferMbps', 'videoToolboxProfile', 'videoToolboxGop', 'videoToolboxRealtime', 'videoToolboxBFramePolicy', 'videoToolboxBFrames', 'videoToolboxAutoAdjustBitrate', 'videoToolboxPowerEfficiency', 'pixFmt'].includes(String(key));
    setConversionDraft((current) => ({ ...current, [key]: value, ...(customHardwareControl ? { hardwareQualityPreset: 'custom' } : {}) }));
  }

  function updateStreamMetadata(type: 'video' | 'audio' | 'subtitle', index: number, patch: StreamMetadataOverride) {
    setConversionDraft((current) => {
      const key = type === 'video' ? 'videoMetadata' : type === 'audio' ? 'audioMetadata' : 'subtitleMetadata';
      const currentMap = current[key] ?? {};
      const nextItem = cleanStreamMetadataOverride({ ...(currentMap[String(index)] ?? {}), ...patch });
      const nextMap = { ...currentMap };
      if (streamMetadataOverrideEmpty(nextItem)) {
        delete nextMap[String(index)];
      } else {
        nextMap[String(index)] = nextItem;
      }
      return { ...current, [key]: Object.keys(nextMap).length ? nextMap : undefined };
    });
  }

  function saveConversionOverrides() {
    updateConversion.mutate({ path: asset.path, ...cleanConversionOverride(conversionDraft) });
  }

  function delay(ms: number) {
    return new Promise<void>((resolve) => {
      window.setTimeout(resolve, ms);
    });
  }

 async function waitForSubtitleOperation(
  operationId: string,
  key: string,
  streamIndex: number,
  format: 'srt' | 'ass',
) {
  if (subtitleOperationPollers.current.has(operationId)) {
    return;
  }

  subtitleOperationPollers.current.add(operationId);

  try {
    while (true) {
      const operation =
        await api.subtitleExtractionOperation(operationId);

      setSubtitleGenerations((current) => ({
        ...current,
        [key]: {
          status: 'running',
          streamIndex,
          format,
          progress: operation.progress,
          phase: operation.phase,
          processed: operation.processed,
          total: operation.total,
          message: operation.message,
        },
      }));

      if (operation.status === 'completed') {
        return operation.result;
      }

      if (operation.status === 'error') {
        throw new Error(
          operation.error ||
            operation.message ||
            'Subtitle OCR failed.',
        );
      }

      await delay(500);
    }
  } finally {
    subtitleOperationPollers.current.delete(operationId);
  }
}

async function restoreSubtitleOperations() {
  try {
    const response = await api.subtitleExtractionOperations(asset.path);

    const runningOperations = response.operations.filter(
      (operation) => operation.status === 'running',
    );

    for (const operation of runningOperations) {
      const format = operation.format;
      const key = subtitleGenerationKey(operation.streamIndex, format);

      setSubtitleGenerations((current) => ({
        ...current,
        [key]: {
          status: 'running',
          streamIndex: operation.streamIndex,
          format,
          phase: operation.phase,
          progress: operation.progress,
          processed: operation.processed,
          total: operation.total,
          message: operation.message,
        },
      }));

      void waitForSubtitleOperation(
        operation.id,
        key,
        operation.streamIndex,
        format,
      );
    }
  } catch (error) {
    console.error(
      'Failed to restore subtitle extraction operations',
      error,
    );
  }
}

async function generateExternalSubtitle(
  streamIndex: number,
  format: 'srt' | 'ass',
  ocrLanguage?: string,
  ocrMode?: 'raw' | 'clean' | 'accurate',
) {
  const key = subtitleGenerationKey(streamIndex, format);

  setSubtitleGenerations((current) => ({
    ...current,
    [key]: {
      status: 'running',
      streamIndex,
      format,
      progress: 0,
      phase: 'preparing',
      processed: 0,
      total: 0,
      message: 'Preparing subtitle generation',
    },
  }));

  try {
    const response = await api.extractAssetSubtitles({
      path: asset.path,
      streamIndex,
      format,
      ocrLanguage,
      ocrMode,
    });

    let result;

    if ('operationId' in response) {
      result = await waitForSubtitleOperation(
        response.operationId,
        key,
        streamIndex,
        format,
      );
    } else {
      result = response;
    }

    setSubtitleGenerations((current) => ({
        ...current,
        [key]: {
          status: 'success',
          streamIndex,
          format,
          progress: 100,
          phase: 'completed',
          message: result?.created.length
            ? `Generated ${result.created.length} ${format.toUpperCase()} file(s).`
            : `${result?.existing.length ?? 0} ${format.toUpperCase()} file(s) already existed.`,
        },
      }));

      await queryClient.invalidateQueries({
        queryKey: ['externalSubtitles', asset.path],
      });
    } catch (error) {
      setSubtitleGenerations((current) => ({
        ...current,
        [key]: {
          status: 'error',
          streamIndex,
          format,
          message:
            error instanceof Error
              ? error.message
              : 'Subtitle generation failed.',
        },
      }));
    }
  }

  function resetConversionOverrides() {
    const empty: AssetConversionOverrideState = {};
    setConversionDraft(empty);
    updateConversion.mutate({ path: asset.path, ...empty });
  }

  function queueAsset(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    const queueProfileId = selectedProfileId > 0 ? selectedProfileId : canQueueWithSelection ? profiles[0]?.id ?? 0 : 0;
    if (!queueProfileId || !selectedLibraryId) {
      return;
    }
    if (isLibraryReplacement && !window.confirm('Convert this Library asset and replace it only after validation? The current file will be moved to Originals Archive.')) {
      return;
    }

    createJob.mutate({
      mediaPath: asset.path,
      publishMode: isLibraryReplacement ? 'replace_library_asset' : 'standard',
      libraryId: isLibraryReplacement ? asset.libraryId : selectedLibraryId,
      profileId: queueProfileId,
      audioProfileKey: selectedAudioProfileKey,
      trackProfileKey: selectedTrackProfile?.key ?? '',
      processingMode: selectedProfileId < 0 ? 'audio_only' : 'full_encode',
      priority: priorityForSize(asset.sizeBytes),
      notes: queueNotes(`Queued individually from folder view: ${relativeAssetPath(asset, libraries)}`, selectedAudioProfileKey),
    });
  }

  function toggleAssetReview(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    const nextRequiresReview = !assetReview.requiresReview;
    const nextReason = nextRequiresReview ? reviewReason || 'Needs manual review before conversion' : '';
    const nextTags = nextRequiresReview ? reviewTags : [];
    if (!nextRequiresReview) {
      setReviewReason('');
      setReviewTags([]);
    }
    updateReview.mutate({
      path: asset.path,
      requiresReview: nextRequiresReview,
      reason: nextReason,
      source: 'manual',
      tags: nextTags,
    });
  }

  function saveAssetReview(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    updateReview.mutate({
      path: asset.path,
      requiresReview: assetReview.requiresReview,
      reason: reviewReason,
      source: 'manual',
      tags: reviewTags,
    });
  }

  function saveAssetCategory(nextCategory: string) {
    setCategory(nextCategory);
    updateMetadata.mutate({
      path: asset.path,
      categories: nextCategory ? [nextCategory] : [],
      tags: safeArray(assetMetadata.tags),
    });
  }

  function recoverArchivedAsset(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    const confirmed = window.confirm('Recover this original from Archive? Converted files will not be deleted.');
    if (confirmed) {
      recoverAsset.mutate(asset.path);
    }
  }

  function safelyDeleteConvertedAsset(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    const confirmed = window.confirm('Delete this converted Library asset? MVForge will proceed only if its archived original exists and can be restored to the original Raw path. Reports, logs, and job history will be preserved.');
    if (confirmed) {
      deleteConvertedAsset.mutate(asset.path);
    }
  }

  function returnPublishedAsIsAsset(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    const confirmed = window.confirm('Remove this Published as-is asset from Library and return it to its original Raw path? Its original filename and external subtitle names will be restored.');
    if (confirmed) {
      returnPublishedAsIs.mutate(asset.path);
    }
  }

  return (
    <>
      <TableRow hover>
        {bulkSelectionEnabled ? (
          <TableCell padding="checkbox">
            <Checkbox
              size="small"
              checked={bulkSelected}
              disabled={bulkSelectionDisabled}
              onChange={(event) => onBulkSelectionChange(asset.path, event.target.checked)}
              inputProps={{ 'aria-label': `Select ${asset.fileName}` }}
            />
          </TableCell>
        ) : null}
        <TableCell>
          <Stack spacing={0.25} sx={{ minWidth: 0 }}>
            <Typography fontWeight={700} sx={{ wordBreak: 'break-word', lineHeight: 1.25 }}>
              {assetTitle(asset)}
            </Typography>
            <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all', lineHeight: 1.25 }}>
              {assetSubpath(asset, libraries)}
            </Typography>
          </Stack>
        </TableCell>
        <TableCell>
          <Stack spacing={0.75} alignItems="flex-start">
            <Chip
              label={pipelineState.label}
              color={pipelineState.color}
              size="small"
            />
            {isBlockedByReview ? <Chip label="Needs review" color="error" size="small" /> : null}
            {assetHasConversionOverride(asset.conversion) ? <Chip label="Overrides" color="primary" size="small" /> : null}
            {asset.missing ? <Chip label="Missing file" color="warning" size="small" /> : null}
            {asset.expiresAt ? (
              <Tooltip title={asset.expiresAt}>
                <Chip
                  label={`Expires ${formatDate(asset.expiresAt)}`}
                  size="small"
                  sx={{ height: 'auto', maxWidth: '100%', '& .MuiChip-label': { display: 'block', whiteSpace: 'normal', py: 0.5 } }}
                />
              </Tooltip>
            ) : null}
          </Stack>
        </TableCell>
        {!isArchive && !isConverted && !isPublishedAsIs ? (
          <TableCell>
            <Button
              size="small"
              variant="outlined"
              startIcon={<InfoOutlinedIcon />}
              disabled={!confidenceEnabled || advisor.isPending || advisor.isError || !advisor.data}
              onClick={() => setShowAdvisorDialog(true)}
              sx={{ minWidth: 86 }}
            >
              {confidenceEnabled ? advisor.isPending ? '...' : advisor.data ? advisor.data.score : 'N/A' : 'Off'}
            </Button>
          </TableCell>
        ) : null}
        {isConverted ? (
          <TableCell>
            <ConvertedMediaSummary
              technical={snapshot.data ? {
                videoCodec: snapshot.data.videoCodec,
                encoder: asset.technical?.encoder,
                width: snapshot.data.width,
                height: snapshot.data.height,
                duration: snapshot.data.duration,
                bitrate: snapshot.data.bitrate,
                hdr: snapshot.data.hdr,
              } : asset.technical}
            />
          </TableCell>
        ) : null}
        <TableCell>{formatBytes(asset.sizeBytes)}</TableCell>
        {mode !== 'unprocessed' ? <TableCell>{formatDate(asset.modifiedAt)}</TableCell> : null}
        {mode === 'unprocessed' ? (
          <TableCell sx={{ minWidth: 180 }}>
            <AssetCategorySelect value={category} options={assetCategories} onChange={saveAssetCategory} label="Category" size="small" disabled={asset.missing || updateMetadata.isPending} />
          </TableCell>
        ) : null}
        {!isArchive && !isConverted && !isPublishedAsIs ? (
          <>
            <TableCell sx={{ minWidth: 220 }}>
              <ProfileAutocomplete profiles={profiles} value={selectedProfileId} onChange={setSelectedProfileId} label="Video" size="small" disabled={rowLocked} allowNone />
            </TableCell>
            <TableCell sx={{ minWidth: 220 }}>
              <AudioProfileAutocomplete profiles={audioProfiles} value={selectedAudioProfileKey} onChange={setSelectedAudioProfileKey} label="Audio" size="small" disabled={rowLocked} />
            </TableCell>
            {mode === 'unprocessed' ? (
              <TableCell sx={{ minWidth: 220 }}>
                <TrackProfileAutocomplete profiles={trackProfiles} value={selectedTrackProfileKey} onChange={setSelectedTrackProfileKey} disabled={rowLocked} label="Tracks" />
              </TableCell>
            ) : null}
            <TableCell sx={{ minWidth: 220 }}>
              <LibraryAutocomplete libraries={libraries} value={isLibraryReplacement ? asset.libraryId : selectedLibraryId} onChange={setSelectedLibraryId} label="Destination" size="small" disabled={rowLocked || isLibraryReplacement} />
            </TableCell>
          </>
        ) : null}
        <TableCell align="center">
          <Stack direction="row" spacing={1} justifyContent="center">
            <Tooltip title="Preview asset">
              <IconButton color="primary" onClick={openPreviewDialog} aria-label={`Preview ${asset.fileName}`} sx={actionIconSx}>
                <PlayCircleIcon />
              </IconButton>
            </Tooltip>
            <Tooltip title={snapshotRunning || snapshot.isPending ? 'Snapshot analysis is already running for this asset' : 'Asset Info'}>
              <span>
                <IconButton color="primary" onClick={openSnapshotDialog} disabled={snapshotRunning || snapshot.isPending} aria-label={`Asset Info ${asset.fileName}`} sx={actionIconSx}>
                  <ManageSearchIcon />
                </IconButton>
              </span>
            </Tooltip>
            {isArchive ? (
              <Tooltip title={asset.missing ? 'Archive file is no longer physically available' : 'Recover original without deleting converted files'}>
                <span>
                  <IconButton
                    color="primary"
                    onClick={recoverArchivedAsset}
                    disabled={asset.missing || recoverAsset.isPending}
                    aria-label={`Recover ${asset.fileName}`}
                    sx={actionIconSx}
                  >
                    <TaskAltIcon />
                  </IconButton>
                </span>
              </Tooltip>
            ) : reconciliationJobId > 0 ? (
              <Tooltip title="Confirm that this asset is the relocated output for its MVForge job">
                <span onClick={(event) => event.stopPropagation()}>
                  <IconButton
                    color="warning"
                    onClick={() => reconcilePublication.mutate({ jobId: reconciliationJobId, path: asset.path })}
                    disabled={reconcilePublication.isPending}
                    aria-label={`Confirm relocated publication ${asset.fileName}`}
                    sx={actionIconSx}
                  >
                    <TaskAltIcon />
                  </IconButton>
                </span>
              </Tooltip>
            ) : !isConverted || isLibraryReplacement ? (
              <Tooltip title={assetReview.requiresReview ? 'Disable review block' : 'Enable review block'}>
                <IconButton
                  color={assetReview.requiresReview ? 'warning' : 'primary'}
                  onClick={toggleAssetReview}
                  disabled={updateReview.isPending}
                  aria-label={`Toggle review for ${asset.fileName}`}
                  sx={actionIconSx}
                >
                  <ReportProblemIcon />
                </IconButton>
              </Tooltip>
            ) : null}
            {isConverted ? (
              <Tooltip title={asset.missing ? 'Complete safe restoration from the archived original' : deleteConvertedAsset.isPending ? 'Safe deletion is in progress' : 'Safely delete converted asset and restore archived original to Raw'}>
                <span onClick={(event) => event.stopPropagation()}>
                  <IconButton color="error" onClick={safelyDeleteConvertedAsset} disabled={deleteConvertedAsset.isPending} aria-label={`Safely delete ${asset.fileName}`} sx={actionIconSx}>
                    <DeleteForeverIcon />
                  </IconButton>
                </span>
              </Tooltip>
            ) : isPublishedAsIs ? (
              <Tooltip title={returnPublishedAsIs.isPending ? 'Returning asset to Raw' : 'Remove from Library and return the original file to Raw'}>
                <span onClick={(event) => event.stopPropagation()}>
                  <IconButton color="error" onClick={returnPublishedAsIsAsset} disabled={returnPublishedAsIs.isPending} aria-label={`Return ${asset.fileName} to Raw`} sx={actionIconSx}>
                    <DeleteForeverIcon />
                  </IconButton>
                </span>
              </Tooltip>
            ) : isArchive || isLibraryReplacement ? null : (
              <Tooltip title={hasOpenJob ? 'This asset already has an open job' : isBlockedByReview ? 'Resolve review before queueing' : 'Queue asset'}>
                <IconButton
                  color="primary"
                  onClick={queueAsset}
                  disabled={createJob.isPending || !canQueueWithSelection || !selectedLibraryId || isBlockedByReview || hasOpenJob}
                  aria-label={`Queue ${asset.fileName}`}
                  sx={actionIconSx}
                >
                  <PlaylistAddIcon />
                </IconButton>
              </Tooltip>
            )}
            {isLibraryReplacement && !isPublishedAsIs ? (
              <Tooltip title={hasOpenJob ? 'This asset already has an open job' : isBlockedByReview ? 'Resolve review before queueing' : 'Convert and safely replace this library asset'}>
                <IconButton
                  color="warning"
                  onClick={queueAsset}
                  disabled={createJob.isPending || !canQueueWithSelection || isBlockedByReview || hasOpenJob}
                  aria-label={`Replace ${asset.fileName}`}
                  sx={actionIconSx}
                >
                  <PlaylistAddIcon />
                </IconButton>
              </Tooltip>
            ) : null}
          </Stack>
        </TableCell>
      </TableRow>
      {createJob.isSuccess ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ bgcolor: 'rgba(102,217,168,0.05)', maxWidth: 0 }}>
            <Alert severity="success">Asset queued individually.</Alert>
          </TableCell>
        </TableRow>
      ) : null}
      {createJob.isError ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ bgcolor: 'rgba(246,180,75,0.05)', maxWidth: 0 }}>
            <Alert severity="warning">{createJob.error instanceof Error ? createJob.error.message : 'Could not queue this asset.'}</Alert>
          </TableCell>
        </TableRow>
      ) : null}
      {deleteConvertedAsset.isSuccess ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ maxWidth: 0 }}><Alert severity="success" sx={{ overflowWrap: 'anywhere' }}>{deleteConvertedAsset.data.message} Restored to: {deleteConvertedAsset.data.restoredPath}</Alert></TableCell>
        </TableRow>
      ) : null}
      {deleteConvertedAsset.isError ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ maxWidth: 0 }}><Alert severity="warning" sx={{ overflowWrap: 'anywhere' }}>Safe deletion was blocked: {deleteConvertedAsset.error instanceof Error ? deleteConvertedAsset.error.message : 'unknown error'}</Alert></TableCell>
        </TableRow>
      ) : null}
      {returnPublishedAsIs.isSuccess ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ maxWidth: 0 }}>
            <Alert severity="success" sx={{ overflowWrap: 'anywhere' }}>{returnPublishedAsIs.data.message} Restored to: {returnPublishedAsIs.data.restoredPath}</Alert>
          </TableCell>
        </TableRow>
      ) : null}
      {returnPublishedAsIs.isError ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ maxWidth: 0 }}><Alert severity="warning" sx={{ overflowWrap: 'anywhere' }}>Return to Raw was blocked: {returnPublishedAsIs.error instanceof Error ? returnPublishedAsIs.error.message : 'unknown error'}</Alert></TableCell>
        </TableRow>
      ) : null}
      {reconcilePublication.isSuccess ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ maxWidth: 0 }}><Alert severity="success">Relocated publication reconciled with job {reconcilePublication.data.jobId}.</Alert></TableCell>
        </TableRow>
      ) : null}
      {reconcilePublication.isError ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ maxWidth: 0 }}><Alert severity="warning">Could not reconcile publication: {reconcilePublication.error instanceof Error ? reconcilePublication.error.message : 'unknown error'}</Alert></TableCell>
        </TableRow>
      ) : null}
      {isBlockedByReview || reviewReason || reviewTags.length > 0 ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ bgcolor: 'rgba(246,180,75,0.05)', maxWidth: 0 }}>
            <Grid container spacing={1.5} alignItems="center">
              <Grid size={{ xs: 12, md: 5 }}>
                <TextField
                  label="Review reason"
                  value={reviewReason}
                  onChange={(event) => setReviewReason(event.target.value)}
                  size="small"
                  fullWidth
                />
              </Grid>
              <Grid size={{ xs: 12, md: 5 }}>
                <Autocomplete
                  multiple
                  freeSolo
                  options={[]}
                  value={reviewTags}
                  onChange={(_, value) => setReviewTags(value.map((tag) => tag.trim()).filter(Boolean))}
                  renderTags={(value, getTagProps) =>
                    value.map((option, index) => {
                      const { key, ...tagProps } = getTagProps({ index });
                      return <Chip key={key} label={option} size="small" {...tagProps} />;
                    })
                  }
                  renderInput={(params) => (
                    <TextField {...params} label="Review tags" placeholder="Add tag" size="small" />
                  )}
                  size="small"
                  fullWidth
                />
              </Grid>
              <Grid size={{ xs: 12, md: 2 }}>
                <Button
                  startIcon={<TaskAltIcon />}
                  color="primary"
                  variant="outlined"
                  onClick={saveAssetReview}
                  disabled={updateReview.isPending}
                  fullWidth
                >
                  Save Review
                </Button>
              </Grid>
            </Grid>
          </TableCell>
        </TableRow>
      ) : null}
      <Dialog open={showPreviewDialog} onClose={() => setShowPreviewDialog(false)} maxWidth="md" fullWidth>
        <DialogTitle>Asset Preview</DialogTitle>
        <DialogContent>
          <Stack spacing={2}>
            <Stack sx={{ minWidth: 0 }}>
              <Typography fontWeight={700} sx={{ wordBreak: 'break-word' }}>
                {assetTitle(asset)}
              </Typography>
              <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                {assetSubpath(asset, libraries)}
              </Typography>
            </Stack>
            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
              <Button
                variant={previewMode === 'compatible' ? 'contained' : 'outlined'}
                onClick={() => setPreviewMode('compatible')}
              >
                Compatible
              </Button>
              <Button variant={previewMode === 'original' ? 'contained' : 'outlined'} onClick={() => setPreviewMode('original')}>
                Original
              </Button>
            </Stack>
            <Box
              key={`${asset.path}-${previewMode}`}
              component="video"
              controls
              preload="metadata"
              src={previewMode === 'compatible' ? api.compatibleAssetPreviewUrl({ path: asset.path }) : api.assetPreviewUrl(asset.path)}
              sx={{
                width: '100%',
                maxHeight: '62vh',
                bgcolor: 'black',
                borderRadius: 1,
              }}
            />
            <Alert severity="info">
              Compatible mode transcodes a temporary MP4/H.264/AAC stream for browser playback. Original mode streams the
              file directly and supports seeking when the browser can decode it.
            </Alert>
          </Stack>
        </DialogContent>
      </Dialog>
      <Dialog open={showSnapshotDialog} onClose={() => setShowSnapshotDialog(false)} maxWidth="lg" fullWidth>
        <DialogTitle sx={{ pb: 0.75 }}>
          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', sm: 'flex-start' }} spacing={1}>
            <Stack sx={{ minWidth: 0 }}>
              <Typography variant="h2">Asset Snapshot</Typography>
              <Typography color="text.secondary" variant="body2" noWrap>
                {assetTitle(asset)}
              </Typography>
            </Stack>
            <Button startIcon={<RefreshIcon />} variant="outlined" size="small" onClick={refreshSnapshot} disabled={asset.missing || snapshot.isPending}>
              Rescan
            </Button>
          </Stack>
        </DialogTitle>
        <DialogContent sx={{ pt: 1 }}>
          <Stack spacing={1.25}>
            <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
              <Stack sx={{ minWidth: 0 }}>
                <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                  {assetSubpath(asset, libraries)}
                </Typography>
              </Stack>
            </Stack>
            {asset.missing ? <Alert severity="warning">This indexed asset is marked as missing. Synchronize Assets and verify that the backend media mount contains the file before scanning it.</Alert> : null}
            {snapshot.isPending ? (
              <Alert severity="info">
                <Stack spacing={1}>
                  <Typography variant="body2">{snapshotOperation?.message || 'Preparing this asset snapshot…'}</Typography>
                  <LinearProgress
                    variant={snapshotOperation && snapshotOperation.progress > 0 ? 'determinate' : 'indeterminate'}
                    value={snapshotOperation?.progress ?? 0}
                  />
                  <Typography variant="caption">Only {asset.fileName} is being analyzed.</Typography>
                </Stack>
              </Alert>
            ) : null}
            {snapshot.isError ? (
              <Alert severity="warning">Could not scan this asset: {snapshot.error instanceof Error ? snapshot.error.message : 'unknown backend error'}</Alert>
            ) : null}
            {snapshot.data ? (
              <>
                <Tabs value={snapshotTab} onChange={(_, value: number) => setSnapshotTab(value)} variant="scrollable" allowScrollButtonsMobile>
                  <Tab label="Asset Information" />
                  <Tab label="Technical Snapshot" />
                  <Tab label="Tracks" />
                  <Tab label="Job Information" />
                  <Tab label="MVForge Suggestions" />
                  <Tab label="Quick Asset Overrides" />
                </Tabs>
                <Box hidden={snapshotTab !== 1}>
                  <Box sx={{ pt: 1.5 }}>
                    <MediaSnapshotDetails scan={snapshot.data} section="general" />
                    <DirectPlaySnapshotComparison scan={snapshot.data} job={associatedJob} />
                  </Box>
                </Box>
                <Box hidden={snapshotTab !== 2}>
                  <Stack spacing={2} sx={{ pt: 1.5 }}>
                    <Typography variant="h3">Embedded tracks</Typography>
                    <MediaSnapshotDetails
                      scan={snapshot.data}
                      section="tracks"
                      streamControls={
                        isConverted || isArchive
                          ? undefined
                          : {
                              video: {
                                selected: conversionStreamIndexes(conversionDraft, snapshot.data, 'video'),
                                disabled: updateConversion.isPending,
                                onToggle: (index, keep) => toggleSnapshotStream('video', index, keep),
                              },
                              audio: {
                                selected: conversionStreamIndexes(conversionDraft, snapshot.data, 'audio'),
                                disabled: updateConversion.isPending,
                                onToggle: (index, keep) => toggleSnapshotStream('audio', index, keep),
                              },
                              subtitle: {
                                selected: conversionStreamIndexes(conversionDraft, snapshot.data, 'subtitle'),
                                disabled: updateConversion.isPending,
                                onToggle: (index, keep) => toggleSnapshotStream('subtitle', index, keep),
                              },
                            }
                      }
                      metadataControls={
                        isConverted || isArchive
                          ? undefined
                          : {
                              video: {
                                values: conversionDraft.videoMetadata ?? {},
                                disabled: updateConversion.isPending,
                                onChange: (index, patch) => updateStreamMetadata('video', index, patch),
                              },
                              audio: {
                                values: conversionDraft.audioMetadata ?? {},
                                disabled: updateConversion.isPending,
                                onChange: (index, patch) => updateStreamMetadata('audio', index, patch),
                              },
                              subtitle: {
                                values: conversionDraft.subtitleMetadata ?? {},
                                disabled: updateConversion.isPending,
                                onChange: (index, patch) => updateStreamMetadata('subtitle', index, patch),
                              },
                            }
                      }
                    />
                    {!isArchive ? (
                      <EmbeddedSubtitleActions
                        streams={snapshot.data.subtitleStreams}
                        generations={subtitleGenerations}
                        onGenerate={generateExternalSubtitle}
                      />
                    ) : null}
                    {!isArchive ? (
                      <ExternalSubtitleList
                        values={externalSubtitles.data ?? []}
                        loading={externalSubtitles.isLoading}
                        deleting={deleteExternalSubtitle.isPending}
                        onEdit={(subtitle) => {
                          setEditingSubtitle(subtitle);
                          setSubtitleContent('');
                          loadSubtitleContent.mutate({ path: asset.path, subtitlePath: subtitle.path });
                        }}
                        onDelete={(subtitle) => {
                          if (window.confirm(`Delete external subtitle ${subtitle.fileName}? This cannot be undone.`)) {
                            deleteExternalSubtitle.mutate({ path: asset.path, subtitlePath: subtitle.path });
                          }
                        }}
                      />
                    ) : null}
                    {externalSubtitles.isError ? (
                      <Alert severity="warning">{externalSubtitles.error instanceof Error ? externalSubtitles.error.message : 'Could not list external subtitles.'}</Alert>
                    ) : null}
                    {deleteExternalSubtitle.isError ? <Alert severity="warning">{deleteExternalSubtitle.error instanceof Error ? deleteExternalSubtitle.error.message : 'Subtitle deletion failed.'}</Alert> : null}
                  </Stack>
                </Box>
                <Box hidden={snapshotTab !== 0}>
                  <Stack spacing={2} sx={{ pt: 1.5 }}>
                    <Typography variant="h3">Asset information</Typography>
                    <Grid container spacing={1.5} alignItems="flex-start">
                      <Grid size={{ xs: 12, md: 6 }}>
                        <Stack direction="row" spacing={1} alignItems="flex-start">
                          <TextField size="small" label="Rename file" value={renameFileName} onChange={(event) => setRenameFileName(event.target.value)} disabled={asset.missing || isArchive || renameAsset.isPending} fullWidth helperText="File name only; the asset remains in its current folder." />
                          <Button size="small" variant="contained" onClick={() => renameAsset.mutate({ path: asset.path, fileName: renameFileName })} disabled={asset.missing || isArchive || renameAsset.isPending || !renameFileName.trim() || renameFileName.trim() === asset.fileName}>Rename</Button>
                        </Stack>
                      </Grid>
                      <Grid size={{ xs: 12, md: 6 }}>
                        <AssetCategorySelect value={category} options={assetCategories} onChange={saveAssetCategory} label="Category" size="small" disabled={asset.missing || updateMetadata.isPending} />
                      </Grid>
                    </Grid>
                    {renameAsset.isError ? <Alert severity="warning">{renameAsset.error.message}</Alert> : null}
                    <Grid container spacing={1.5}>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Video profile applied</Typography><Typography>{associatedJob ? profiles.find((profile) => profile.id === associatedJob.profileId)?.name || `Profile #${associatedJob.profileId}` : profiles.find((profile) => profile.id === selectedProfileId)?.name || 'None'}</Typography></Grid>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Audio profile applied</Typography><Typography>{associatedJob?.audioProfileKey || selectedAudioProfileKey || 'None'}</Typography></Grid>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Tracks profile applied</Typography><Typography>{conversionDraft.trackProfileKey || pathTrackProfile?.name || pathTrackProfile?.key || 'None'}</Typography></Grid>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Advisor score</Typography><Typography>{advisor.data ? `${advisor.data.score}/100` : 'Not evaluated'}</Typography></Grid>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Direct Play score</Typography><Typography>{associatedJob ? directPlayScoreLabel(associatedJob) : 'Not evaluated'}</Typography></Grid>
                    </Grid>
                  </Stack>
                </Box>
                <Box hidden={snapshotTab !== 3}>
                  <Box sx={{ pt: 1.5 }}>
                    {associatedJob ? (
                      <FinalDetailsSummary asset={asset} job={associatedJob} />
                    ) : (
                      <Alert severity="info">No job is currently associated with this asset.</Alert>
                    )}
                  </Box>
                </Box>
                <Box hidden={snapshotTab !== 4}>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    {!isConverted && !isArchive && profileSuggestion.isPending ? <Alert severity="info">Comparing the snapshot with available profiles…</Alert> : null}
                    {!isConverted && !isArchive && profileSuggestion.isError ? (
                      <Alert severity="warning">The snapshot was created, but a profile could not be suggested: {profileSuggestion.error instanceof Error ? profileSuggestion.error.message : 'unknown backend error'}</Alert>
                    ) : null}
                    {!isConverted && !isArchive && profileSuggestion.data ? (
                      <ProfileSuggestionCard
                        suggestion={profileSuggestion.data}
                        onSelect={(profile) => setSelectedProfileId(profile.id)}
                        onApplyMotionRecommendation={() => applySnapshotRecommendations(profileSuggestion.data!)}
                        onReviewInLab={() => { window.location.href = `/profile-lab?assetPath=${encodeURIComponent(asset.path)}`; }}
                      />
                    ) : null}
                    {isConverted ? <Alert severity="info">This is already a converted asset. Re-processing recommendations should be evaluated from its archived original.</Alert> : null}
                    {isArchive ? <Alert severity="info">Recover the archived original before applying a suggested profile.</Alert> : null}
                  </Stack>
                </Box>
                <Box hidden={snapshotTab !== 5}>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    {!isArchive ? (
                      <AssetConversionOverridePanel
                        assetPath={asset.path}
                        draft={conversionDraft}
                        profile={profiles.find((profile) => profile.id === selectedProfileId)}
                        scan={snapshot.data}
                        onChange={updateConversionDraft}
                        onSave={saveConversionOverrides}
                        onReset={resetConversionOverrides}
                        saving={updateConversion.isPending}
                        readOnly={isConverted}
                      />
                    ) : <Alert severity="info">Archived originals are immutable. Recover the asset before configuring conversion overrides.</Alert>}
                  </Stack>
                </Box>
              </>
            ) : null}
            {updateConversion.isSuccess ? <Alert severity="success">Asset conversion overrides saved.</Alert> : null}
            {updateConversion.isError ? (
              <Alert severity="warning">Could not save conversion overrides: {updateConversion.error.message}</Alert>
            ) : null}
          </Stack>
        </DialogContent>
      </Dialog>
      <Dialog open={Boolean(editingSubtitle)} onClose={() => setEditingSubtitle(null)} maxWidth="md" fullWidth>
        <DialogTitle>Edit external subtitle</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ pt: 1 }}>
            <Typography color="text.secondary" sx={{ wordBreak: 'break-all' }}>{editingSubtitle?.fileName}</Typography>
            {loadSubtitleContent.isPending ? <Alert severity="info">Loading subtitle…</Alert> : null}
            {loadSubtitleContent.isError ? <Alert severity="warning">{loadSubtitleContent.error instanceof Error ? loadSubtitleContent.error.message : 'Could not load subtitle.'}</Alert> : null}
            <TextField
              value={subtitleContent}
              onChange={(event) => setSubtitleContent(event.target.value)}
              multiline
              minRows={16}
              fullWidth
              disabled={loadSubtitleContent.isPending || loadSubtitleContent.isError}
              inputProps={{ spellCheck: false }}
            />
            <Stack direction="row" justifyContent="flex-end" spacing={1}>
              <Button onClick={() => setEditingSubtitle(null)}>Cancel</Button>
              <Button
                variant="contained"
                disabled={!editingSubtitle || !subtitleContent.trim() || saveSubtitleContent.isPending}
                onClick={() => editingSubtitle && saveSubtitleContent.mutate({ path: asset.path, subtitlePath: editingSubtitle.path, content: subtitleContent })}
              >
                Save subtitle
              </Button>
            </Stack>
            {saveSubtitleContent.isError ? <Alert severity="warning">{saveSubtitleContent.error instanceof Error ? saveSubtitleContent.error.message : 'Could not save subtitle.'}</Alert> : null}
          </Stack>
        </DialogContent>
      </Dialog>
      <Dialog open={showAdvisorDialog} onClose={() => setShowAdvisorDialog(false)} maxWidth="md" fullWidth>
        <DialogTitle>Advisor Score</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ pt: 1 }}>
            <Stack>
              <Typography fontWeight={700} sx={{ wordBreak: 'break-word' }}>
                {assetTitle(asset)}
              </Typography>
              <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                {assetSubpath(asset, libraries)}
              </Typography>
            </Stack>
            {advisor.data ? (
              <AdvisorSummary advisor={advisor.data} audioProfile={audioProfiles.find((profile) => profile.key === selectedAudioProfileKey)} />
            ) : advisor.isError ? (
              <Alert severity="warning">Could not evaluate this asset with the selected profile.</Alert>
            ) : (
              <Alert severity="info">Advisor is evaluating this asset...</Alert>
            )}
          </Stack>
        </DialogContent>
      </Dialog>
    </>
  );
}

function AdvisorSummary({ advisor, audioProfile }: { advisor: AdvisorResponse; audioProfile?: AudioEnhancementProfile }) {
  const primaryAudio = safeArray(advisor.scan.audioStreams)[0];
  const outputCodec = audioProfile?.outputCodec || advisor.profile.audioCodec;
  const sourceCodec = primaryAudio?.codec || 'unknown';
  const audioCodecChanges = outputCodec && outputCodec !== 'copy' && outputCodec.toLowerCase() !== sourceCodec.toLowerCase();

  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, bgcolor: 'rgba(79,179,255,0.04)', p: 2 }}>
      <Stack spacing={2}>
        <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap" useFlexGap>
          <Chip label={recommendationLabel(advisor.recommendation)} color={recommendationColor(advisor.recommendation)} />
          <Typography variant="h2">{advisor.score}%</Typography>
          <Typography color="text.secondary">{advisor.summary}</Typography>
        </Stack>
        <Grid container spacing={2}>
          <Grid size={{ xs: 12 }}>
            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
              <Chip label={`Source audio: ${sourceCodec}`} size="small" />
              {primaryAudio?.language ? <Chip label={`Language: ${primaryAudio.language}`} size="small" /> : null}
              {primaryAudio?.channelLayout ? <Chip label={primaryAudio.channelLayout} size="small" /> : null}
              {primaryAudio?.sampleRate ? <Chip label={`${primaryAudio.sampleRate} Hz`} size="small" /> : null}
              <Chip
                label={
                  audioProfile
                    ? `Audio profile output: ${outputCodec}${audioCodecChanges ? ' (codec changes)' : ' (same/copy)'}`
                    : `Audio output: ${outputCodec || 'copy'}`
                }
                color={audioCodecChanges ? 'warning' : 'default'}
                size="small"
              />
            </Stack>
          </Grid>
          <Grid size={{ xs: 12, md: 6 }}>
            <Typography variant="h3" sx={{ mb: 1 }}>
              Reasons
            </Typography>
            <Stack spacing={0.8}>
              {safeArray(advisor.reasons).map((reason) => (
                <Typography key={reason} color="text.secondary">
                  {reason}
                </Typography>
              ))}
            </Stack>
          </Grid>
          <Grid size={{ xs: 12, md: 6 }}>
            <Typography variant="h3" sx={{ mb: 1 }}>
              Warnings
            </Typography>
            {safeArray(advisor.warnings).length ? (
              <Stack spacing={0.8}>
                {safeArray(advisor.warnings).map((warning) => (
                  <Typography key={warning} color="warning.main">
                    {warning}
                  </Typography>
                ))}
              </Stack>
            ) : (
              <Typography color="text.secondary">No warnings for the selected profile.</Typography>
            )}
          </Grid>
        </Grid>
      </Stack>
    </Box>
  );
}

function EmbeddedSubtitleActions({
  streams,
  generations,
  onGenerate,
}: {
  streams: MediaStreamInfo[];
  generations: Record<string, SubtitleGenerationState>;
  onGenerate: (streamIndex: number, format: 'srt' | 'ass', ocrLanguage?: string, ocrMode?: 'raw' | 'clean' | 'accurate') => void;
}) {
  const [ocrLanguages, setOcrLanguages] = useState<Record<number, string>>({});
  const [ocrModes, setOcrModes] = useState<Record<number, 'raw' | 'clean' | 'accurate'>>({});
  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
      <Stack spacing={1.25}>
        <Stack>
          <Typography variant="h3">Generate external subtitles</Typography>
          <Typography color="text.secondary" variant="body2">Create an SRT or ASS sidecar from a selected embedded track. Bitmap tracks use OCR only when you press a generate button; the embedded track is not removed.</Typography>
        </Stack>
        {streams.length === 0 ? <Alert severity="info">This asset has no embedded subtitle tracks.</Alert> : null}
        {streams.map((stream) => {
          const bitmap = isBitmapSubtitleCodec(stream.codec);
          const streamGenerations = (['srt', 'ass'] as const)
            .map((format) => generations[subtitleGenerationKey(stream.index, format)])
            .filter((value): value is SubtitleGenerationState => Boolean(value));
          return (
            <Stack
              key={stream.index}
              spacing={1}
              sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1 }}
            >
              <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={1}>
                <Stack sx={{ minWidth: 0 }}>
                  <Typography fontWeight={700}>#{stream.index} · {(stream.language || 'und').toUpperCase()} · {stream.codec.toUpperCase()}</Typography>
                  {stream.title ? <Typography color="text.secondary" variant="body2">{stream.title}</Typography> : null}
                  {bitmap ? <Typography color="warning.main" variant="caption">Bitmap track: generation runs local OCR and may require text corrections afterward.</Typography> : null}
                </Stack>
                <Stack direction="row" spacing={1}>
                  {bitmap ? (
                    <TextField
                      select
                      size="small"
                      label="OCR language"
                      value={ocrLanguages[stream.index] || defaultOCRLanguage(stream.language)}
                      onChange={(event) => setOcrLanguages((current) => ({ ...current, [stream.index]: event.target.value }))}
                      sx={{ minWidth: 130 }}
                    >
                      <MenuItem value="eng">English</MenuItem>
                      <MenuItem value="spa">Spanish</MenuItem>
                      <MenuItem value="jpn">Japanese</MenuItem>
                      <MenuItem value="jpn_vert">Japanese vertical</MenuItem>
                    </TextField>
                  ) : null}
                  {bitmap ? <TextField select size="small" label="OCR quality" value={ocrModes[stream.index] || 'accurate'} onChange={(event) => setOcrModes((current) => ({ ...current, [stream.index]: event.target.value as 'raw' | 'clean' | 'accurate' }))} sx={{ minWidth: 155 }}><MenuItem value="raw">Raw</MenuItem><MenuItem value="clean">Clean</MenuItem><MenuItem value="accurate">Accurate</MenuItem></TextField> : null}
                  <Button size="small" variant="outlined" disabled={generations[subtitleGenerationKey(stream.index, 'srt')]?.status === 'running'} onClick={() => onGenerate(stream.index, 'srt', bitmap ? (ocrLanguages[stream.index] || defaultOCRLanguage(stream.language)) : undefined, bitmap ? (ocrModes[stream.index] || 'accurate') : undefined)}>Generate SRT</Button>
                  <Button size="small" variant="outlined" disabled={generations[subtitleGenerationKey(stream.index, 'ass')]?.status === 'running'} onClick={() => onGenerate(stream.index, 'ass', bitmap ? (ocrLanguages[stream.index] || defaultOCRLanguage(stream.language)) : undefined, bitmap ? (ocrModes[stream.index] || 'accurate') : undefined)}>Generate ASS</Button>
                </Stack>
              </Stack>
              {streamGenerations.map((generation) => (
                <Box key={subtitleGenerationKey(stream.index, generation.format)}>
                  {generation.status === 'running' ? (
                    <Stack spacing={0.5}>
                      <Stack direction="row" justifyContent="space-between">
                        <Typography variant="caption" color="text.secondary">
                          {generation.phase === 'extracting'
                            ? 'Extracting bitmap subtitle…'
                            : generation.phase === 'ocr'
                              ? generation.total
                                ? `Running OCR on ${generation.total} images…`
                                : 'Running OCR…'
                              : generation.phase === 'cleanup'
                                ? 'Cleaning OCR text…'
                                : generation.phase === 'publishing'
                                  ? 'Publishing subtitle…'
                                  : `Preparing ${generation.format.toUpperCase()}…`}
                        </Typography>
                        {typeof generation.progress === 'number' ? (
                          <Typography variant="caption" color="text.secondary">
                            {Math.round(generation.progress)}%
                          </Typography>
                        ) : null}
                      </Stack>

                      <LinearProgress
                        variant={
                          typeof generation.progress === 'number'
                            ? 'determinate'
                            : 'indeterminate'
                        }
                        value={generation.progress}
                        aria-label={`Generating ${generation.format.toUpperCase()} subtitle for stream ${stream.index}`}
                      />
                    </Stack>
                  ) : (
                    <Alert severity={generation.status === 'success' ? 'success' : 'warning'} sx={{ py: 0 }}>
                      {generation.message}
                    </Alert>
                  )}
                </Box>
              ))}
            </Stack>
          );
        })}
      </Stack>
    </Box>
  );
}

type SubtitleGenerationState = {
  status: 'running' | 'success' | 'error';
  streamIndex: number;
  format: 'srt' | 'ass';
  message?: string;

  progress?: number;
  phase?:
    | 'preparing'
    | 'extracting'
    | 'ocr'
    | 'cleanup'
    | 'publishing'
    | 'completed'
    | 'error';
  processed?: number;
  total?: number;
};

function subtitleGenerationKey(streamIndex: number, format: 'srt' | 'ass') {
  return `${streamIndex}:${format}`;
}

function defaultOCRLanguage(language: string) {
  const value = (language || '').trim().toLowerCase();
  if (value === 'spa' || value === 'es' || value === 'esp') return 'spa';
  if (value === 'jpn' || value === 'ja' || value === 'jp') return 'jpn';
  return 'eng';
}

function ExternalSubtitleList({
  values,
  loading,
  deleting,
  onEdit,
  onDelete,
}: {
  values: ExternalSubtitle[];
  loading: boolean;
  deleting: boolean;
  onEdit: (subtitle: ExternalSubtitle) => void;
  onDelete: (subtitle: ExternalSubtitle) => void;
}) {
  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
      <Stack spacing={1.25}>
        <Typography variant="h3">External SRT / ASS files</Typography>
        {loading ? <Alert severity="info">Reading external subtitles…</Alert> : null}
        {!loading && values.length === 0 ? <Alert severity="info">No external SRT or ASS sidecars were found beside this asset.</Alert> : null}
        {values.map((subtitle) => (
          <Stack
            key={subtitle.path}
            direction={{ xs: 'column', sm: 'row' }}
            alignItems={{ xs: 'stretch', sm: 'center' }}
            justifyContent="space-between"
            spacing={1}
            sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1 }}
          >
            <Stack sx={{ minWidth: 0 }}>
              <Typography fontWeight={700} sx={{ overflowWrap: 'anywhere' }}>{subtitle.fileName}</Typography>
              <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                <Chip label={subtitle.format.toUpperCase()} size="small" />
                {subtitle.language ? <Chip label={subtitle.language.toUpperCase()} size="small" /> : null}
                {subtitle.default ? <Chip label="Default" color="primary" size="small" /> : null}
                {subtitle.forced ? <Chip label="Forced" color="warning" size="small" /> : null}
                <Chip label={formatBytes(subtitle.sizeBytes)} size="small" />
              </Stack>
            </Stack>
            <Stack direction="row" spacing={0.5}>
              <Tooltip title="Edit subtitle text">
                <IconButton size="small" color="primary" onClick={() => onEdit(subtitle)}><EditIcon /></IconButton>
              </Tooltip>
              <Tooltip title="Delete external subtitle">
                <IconButton size="small" color="error" disabled={deleting} onClick={() => onDelete(subtitle)}><DeleteForeverIcon /></IconButton>
              </Tooltip>
            </Stack>
          </Stack>
        ))}
      </Stack>
    </Box>
  );
}

type SelectOption = {
  value: string;
  label: string;
};

const videoCodecOptions: SelectOption[] = [
  { value: '', label: 'Profile default' },
  { value: 'copy', label: 'Keep original video' },
  { value: 'x265_10bit', label: 'HEVC / x265 10-bit' },
  { value: 'x265', label: 'HEVC / x265' },
  { value: 'x264', label: 'H.264 / x264' },
];

const audioCodecOptions: SelectOption[] = [
  { value: '', label: 'Profile default' },
  { value: 'copy', label: 'Keep original audio' },
  { value: 'aac', label: 'AAC (best compatibility)' },
  { value: 'ac3', label: 'AC-3 (home theater)' },
  { value: 'opus', label: 'Opus (small files)' },
];

const speedOptions: SelectOption[] = [
  { value: '', label: 'Profile default' },
  { value: 'fast', label: 'Fast' },
  { value: 'medium', label: 'Balanced' },
  { value: 'slow', label: 'Slow / smaller file' },
  { value: 'slower', label: 'Very slow / smallest file' },
];

const colorDepthOptions: SelectOption[] = [
  { value: '', label: 'Profile default' },
  { value: 'yuv420p', label: '8-bit SDR compatibility' },
  { value: 'yuv420p10le', label: '10-bit / HDR friendly' },
];

const imageCleanupOptions: SelectOption[] = [
  { value: '', label: 'Profile default' },
  { value: 'bwdif=mode=send_frame', label: 'Fix interlacing' },
  { value: 'hqdn3d=1.5:1.5:6:6', label: 'Light noise cleanup' },
  { value: 'hqdn3d=2:2:7:7', label: 'Medium noise cleanup' },
  { value: 'deband=1thr=0.018:2thr=0.018:3thr=0.018:4thr=0.018', label: 'Light banding cleanup' },
  { value: 'bwdif=mode=send_frame,hqdn3d=1.5:1.5:6:6', label: 'DVD cleanup' },
  { value: 'hqdn3d=1.5:1.5:6:6,deband=1thr=0.018:2thr=0.018:3thr=0.018:4thr=0.018', label: 'Anime cleanup' },
];

const deinterlaceOptions: SelectOption[] = [
  { value: '', label: 'Profile default' },
  { value: 'auto', label: 'Auto at conversion (uses Analysis)' },
  { value: 'off', label: 'Never deinterlace' },
  { value: 'force', label: 'Force · bwdif (single-rate)' },
  { value: 'ivtc_tff', label: 'IVTC · fieldmatch + decimate (TFF)' },
  { value: 'ivtc_bff', label: 'IVTC · fieldmatch + decimate (BFF)' },
];

const assetEncoderOptions: SelectOption[] = [
  { value: 'auto', label: 'Auto' },
  { value: 'libx265', label: 'Software x265' },
  { value: 'hevc_qsv', label: 'Intel Quick Sync' },
  { value: 'hevc_vaapi', label: 'VAAPI' },
  { value: 'hevc_nvenc', label: 'NVIDIA NVENC' },
  { value: 'hevc_videotoolbox', label: 'Apple VideoToolbox' },
  { value: 'hevc_amf', label: 'AMD AMF' },
];

function AssetConversionOverridePanel({
  assetPath,
  draft,
  profile,
  scan,
  onChange,
  onSave,
  onReset,
  saving,
  readOnly = false,
}: {
  assetPath: string;
  draft: AssetConversionOverrideState;
  profile?: Profile;
  scan?: ScanResult;
  onChange: <K extends keyof AssetConversionOverrideState>(key: K, value: AssetConversionOverrideState[K]) => void;
  onSave: () => void;
  onReset: () => void;
  saving: boolean;
  readOnly?: boolean;
}) {
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const runtimeSnapshot = useQuery({ queryKey: ['runtime-snapshot'], queryFn: api.runtimeSnapshot });
  const encoderQualityRecommendation = useMutation({ mutationFn: api.recommendEncoderQuality });
  const recommendedCrop = ['detected', 'variable'].includes(scan?.cropAnalysis?.status ?? '') ? (scan?.cropAnalysis?.recommendedCrop ?? '').trim() : '';
  const manualCropCandidate = scan?.cropAnalysis?.status === 'variable' && Boolean(recommendedCrop);
  const currentCropFilter = cropFilterFromChain(draft.videoFilters);
  const suggestedCropFilter = recommendedCrop ? `crop=${recommendedCrop}` : '';
  const suggestedCropEnabled = Boolean(suggestedCropFilter) && currentCropFilter === suggestedCropFilter;
  const bitmapSubtitleCount = scan?.subtitleStreams.filter((stream) => isBitmapSubtitleCodec(stream.codec)).length ?? 0;
  const processingPreference = draft.preferredEncoder ?? '';
  const effectiveVideoCodec = draft.videoCodec || profile?.videoCodec || 'copy';
  const hardwareCodecSupported = hardwareEncodingSupportedForAssetCodec(effectiveVideoCodec);
  const hardwareSelected = processingPreference === 'hardware';
  const defaultHardwareEncoder = assetEncoderOptions.find(
    (option) => isHardwareAssetEncoder(option.value) && runtimeSnapshot.data?.encoders?.[option.value]?.usable,
  )?.value ?? '';
  const effectiveVideoEncoder = draft.videoEncoder || defaultHardwareEncoder;
  const qsvSelected = hardwareSelected && effectiveVideoEncoder === 'hevc_qsv';
  const videoToolboxSelected = hardwareSelected && effectiveVideoEncoder === 'hevc_videotoolbox';
  const qsvCapability = runtimeSnapshot.data?.encoders?.hevc_qsv;
  const qsvMain10Selected = ['p010le', 'yuv420p10le'].includes((draft.pixFmt || stringFromRecord(profile?.workerConfig ?? {}, 'pixFmt')).toLowerCase());
  const qsvRateControl =
    draft.qsvRateControl ||
    stringFromRecord(profile?.workerConfig ?? {}, 'qsvRateControl') ||
    'icq';

  const qsvFeatures = resolveQSVFeatures(qsvCapability, {
    main10: qsvMain10Selected,
    rateControl: qsvRateControl,
  });
  const qsvWarnings = qsvSelectionWarnings(qsvFeatures, {
    extendedBRC: draft.qsvExtendedBrc ?? profile?.workerConfig?.qsvExtendedBRC === true,
    adaptiveI: draft.qsvAdaptiveI ?? profile?.workerConfig?.qsvAdaptiveI === true,
    adaptiveB: draft.qsvAdaptiveB ?? profile?.workerConfig?.qsvAdaptiveB === true,
  });

  const videoToolboxCapability = runtimeSnapshot.data?.encoders?.hevc_videotoolbox;
  const selectedHardwareQualityPreset = String(draft.hardwareQualityPreset ?? profile?.workerConfig?.hardwareQualityPreset ?? 'recommended');
  const effectiveHardwarePresetConfig = selectedHardwareQualityPreset !== 'custom'
    ? applySharedHardwareQualityPreset({ ...(profile?.workerConfig ?? {}), ...draft }, effectiveVideoEncoder, selectedHardwareQualityPreset)
    : { ...(profile?.workerConfig ?? {}), ...draft };
  const effectivePixFmt = (draft.pixFmt || stringFromRecord(profile?.workerConfig ?? {}, 'pixFmt')).toLowerCase();
  const videoToolboxMain10Selected = (draft.videoToolboxProfile ?? String(profile?.workerConfig?.videoToolboxProfile ?? '')).toLowerCase() === 'main10'
    || ['p010le', 'yuv420p10le'].includes(effectivePixFmt);
  const videoToolboxBFramesAvailable = videoToolboxCapability?.videoToolboxBFrames === true
    && (!videoToolboxMain10Selected || videoToolboxCapability.testedModes?.videoToolboxBFramesMain10 === true);
  const videoToolboxPowerAvailable = videoToolboxCapability?.videoToolboxPowerEfficient === true
    && (!videoToolboxMain10Selected || videoToolboxCapability.testedModes?.videoToolboxPowerEfficientMain10 === true);

  function selectHardwareQualityPreset(preset: string, encoder: string) {
    onChange('hardwareQualityPreset', preset);
    if (preset === 'custom') return;
    const requested = assetQualityProfile(profile, draft, encoder, preset);
    encoderQualityRecommendation.mutate({ path: assetPath, profile: requested }, {
      onSuccess: (result) => applyEffectiveProfileToAssetOverrides(onChange, result.effectiveProfile),
    });
  }

  function changeProcessingPreference(value: '' | 'software' | 'hardware') {
    if (value === '') {
      onChange('preferredEncoder', undefined);
      onChange('useHardwareIfAvailable', undefined);
      onChange('videoEncoder', undefined);
      onChange('globalQuality', undefined);
      onChange('qsvRateControl', undefined);
      onChange('qsvLookAheadDepth', undefined);
      onChange('qsvExtendedBrc', undefined);
      onChange('qsvAdaptiveI', undefined);
      onChange('qsvAdaptiveB', undefined);
      return;
    }
    onChange('preferredEncoder', value);
    if (value === 'software') {
      onChange('useHardwareIfAvailable', false);
      onChange('videoEncoder', 'auto');
      onChange('globalQuality', undefined);
      onChange('qsvRateControl', undefined);
      onChange('qsvLookAheadDepth', undefined);
      onChange('qsvExtendedBrc', undefined);
      onChange('qsvAdaptiveI', undefined);
      onChange('qsvAdaptiveB', undefined);
      onChange('pixFmt', 'yuv420p10le');
      return;
    }
    onChange('useHardwareIfAvailable', hardwareCodecSupported);
    const encoder = isHardwareAssetEncoder(effectiveVideoEncoder) ? effectiveVideoEncoder : defaultHardwareEncoder;
    onChange('videoEncoder', encoder);
    selectHardwareQualityPreset('recommended', encoder);
  }

  function changeAssetVideoCodec(value: string) {
    onChange('videoCodec', value);
    if ((value === 'copy' || !hardwareEncodingSupportedForAssetCodec(value)) && processingPreference === 'hardware') {
      changeProcessingPreference('software');
    }
  }

  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, bgcolor: 'rgba(79,179,255,0.035)' }}>
      <Stack spacing={2}>
        <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={1}>
          <Stack>
            <Typography variant="h3">Asset Overrides</Typography>
            <Typography color="text.secondary" variant="body2">
              {readOnly ? 'Recorded overrides for this converted asset. Re-process from its archived original to apply changes.' : 'Per-asset changes applied at conversion time.'}
            </Typography>
          </Stack>
          <Stack direction="row" spacing={1}>
            <Button variant="outlined" onClick={onReset} disabled={saving || readOnly}>
              Remove
            </Button>
            <Button variant="contained" onClick={onSave} disabled={saving || readOnly}>
              Save
            </Button>
          </Stack>
        </Stack>
        <Divider />
        <Box component="fieldset" disabled={readOnly} sx={{ border: 0, p: 0, m: 0, minWidth: 0 }}>
        <Grid container spacing={2}>
          <Grid size={{ xs: 12, md: 4 }}>
            <TextField
              select
              label="Video codec"
              value={draft.videoCodec ?? ''}
              onChange={(event) => changeAssetVideoCodec(event.target.value)}
              size="small"
              fullWidth
            >
              {optionItems(videoCodecOptions, draft.videoCodec).map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {option.label}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          {scan && scan.audioStreams.length > 0 ? (
            <Grid size={{ xs: 12, md: 4 }}>
              <TextField
                select
                label="AAC / enhanced audio source"
                value={draft.enhancedAudioSourceStreamIndex ?? Number(profile?.workerConfig?.aacStereoSourceStreamIndex ?? -1)}
                onChange={(event) => onChange('enhancedAudioSourceStreamIndex', event.target.value === '' ? undefined : Number(event.target.value))}
                helperText="Asset override takes priority over the profile AAC source."
                size="small"
                fullWidth
              >
                <MenuItem value={-1}>Automatic / default audio track</MenuItem>
                {scan.audioStreams.map((stream) => (
                  <MenuItem key={stream.index} value={stream.index}>
                    #{stream.index} · {(stream.language || 'und').toUpperCase()} · {stream.codec.toUpperCase()}
                  </MenuItem>
                ))}
              </TextField>
            </Grid>
          ) : null}
          <Grid size={{ xs: 12, md: 4 }}>
            <TextField
              select
              label="Speed"
              value={draft.videoPreset ?? ''}
              onChange={(event) => onChange('videoPreset', event.target.value)}
              size="small"
              fullWidth
            >
              {optionItems(speedOptions, draft.videoPreset).map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {option.label}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          <Grid size={{ xs: 12, md: 8 }}>
            <TextField
              select
              label="Image cleanup filters"
              value={withoutCropFilters(draft.videoFilters)}
              onChange={(event) => onChange('videoFilters', joinFilters(event.target.value, cropFilterFromChain(draft.videoFilters)))}
              size="small"
              fullWidth
            >
              {optionItems(imageCleanupOptions, withoutCropFilters(draft.videoFilters)).map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {option.label}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          <Grid size={{ xs: 12, md: 4 }}>
            <TextField
              select
              label="Deinterlacing"
              value={draft.deinterlaceMode ?? ''}
              onChange={(event) => onChange('deinterlaceMode', event.target.value as AssetConversionOverrideState['deinterlaceMode'])}
              helperText={draft.deinterlaceMode === 'force'
                ? 'bwdif=mode=send_frame:parity=auto:deint=all · output is marked progressive automatically.'
                : 'Field metadata is updated automatically when Analysis applies a correction.'}
              size="small"
              fullWidth
            >
              {optionItems(deinterlaceOptions, draft.deinterlaceMode).map((option) => (
                <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>
              ))}
            </TextField>
          </Grid>
          <Grid size={{ xs: 12 }}>
            <Box sx={{ border: 1, borderColor: suggestedCropEnabled ? 'primary.main' : 'divider', borderRadius: 1, p: 1.5, bgcolor: suggestedCropEnabled ? 'rgba(79,179,255,0.055)' : 'rgba(255,255,255,0.02)' }}>
              <Stack spacing={1}>
                <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'flex-start', sm: 'center' }} justifyContent="space-between" spacing={1}>
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={suggestedCropEnabled}
                        disabled={!recommendedCrop}
                        onChange={(event) => onChange(
                          'videoFilters',
                          event.target.checked
                            ? joinFilters(withoutCropFilters(draft.videoFilters), suggestedCropFilter)
                            : withoutCropFilters(draft.videoFilters),
                        )}
                      />
                    }
                    label="Enable suggested crop"
                  />
                  {recommendedCrop ? <Chip size="small" color={suggestedCropEnabled ? 'primary' : 'default'} label={`${manualCropCandidate ? 'Manual candidate · ' : ''}crop=${recommendedCrop}`} /> : <Chip size="small" label="No stable crop suggested" />}
                </Stack>
                {scan?.cropAnalysis?.reason ? <Typography variant="body2" color="text.secondary">{scan.cropAnalysis.reason}</Typography> : null}
                {manualCropCandidate ? <Alert severity="warning">This value groups similar borders from multiple scenes. It is intentionally disabled until you review framing and subtitles in LAB.</Alert> : null}
                {currentCropFilter && !suggestedCropEnabled ? (
                  <Alert severity="info">A custom crop is currently active: {currentCropFilter}. Enabling the suggestion will replace it.</Alert>
                ) : null}
                {suggestedCropEnabled && bitmapSubtitleCount > 0 ? (
                  <Alert severity="warning">
                    {bitmapSubtitleCount} bitmap subtitle track(s) may place text inside the cropped bars. Verify representative subtitled scenes before processing.
                  </Alert>
                ) : null}
              </Stack>
            </Box>
          </Grid>
          <Grid size={{ xs: 12 }}>
            <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5, bgcolor: 'rgba(255,255,255,0.02)' }}>
              <Grid container spacing={2} alignItems="flex-start">
                <Grid size={{ xs: 12, md: 4 }}>
                  <TextField
                    select
                    label="Processing preference"
                    value={processingPreference}
                    onChange={(event) => changeProcessingPreference(event.target.value as '' | 'software' | 'hardware')}
                    helperText="Use the profile default, force software, or select validated hardware."
                    size="small"
                    fullWidth
                  >
                    <MenuItem value="">Profile default</MenuItem>
                    <MenuItem value="software">Software · match Video Codec</MenuItem>
                    <MenuItem value="hardware" disabled={!hardwareCodecSupported || runtimeSnapshot.isLoading || !defaultHardwareEncoder}>Hardware</MenuItem>
                  </TextField>
                </Grid>
                <Grid size={{ xs: 12, md: 4 }}>
                  <TextField
                    select
                    label="Final color policy"
                    value={draft.finalColorPolicy || stringFromRecord(profile?.workerConfig ?? {}, 'finalColorPolicy') || 'preserve'}
                    onChange={(event) => onChange('finalColorPolicy', event.target.value as 'automatic' | 'preserve' | 'normalize_bt709')}
                    helperText="Defaults to no correction; override it only when analysis or visual validation justifies a change."
                    size="small"
                    fullWidth
                  >
                    <MenuItem value="preserve">No correction · preserve source</MenuItem>
                    <MenuItem value="automatic">Automatic correction when justified</MenuItem>
                    <MenuItem value="normalize_bt709">Normalize mathematically to BT.709</MenuItem>
                  </TextField>
                </Grid>
                {hardwareSelected ? <Grid size={{ xs: 12, md: 4 }}>
                  <TextField
                    select
                    label="Hardware encoder"
                    value={effectiveVideoEncoder}
                    onChange={(event) => {
                      onChange('videoEncoder', event.target.value);
                      selectHardwareQualityPreset('recommended', event.target.value);
                    }}
                    helperText="Only encoders reported by the current runtime are selectable."
                    size="small"
                    fullWidth
                  >
                    {assetEncoderOptions.filter((option) => isHardwareAssetEncoder(option.value)).map((option) => (
                      <MenuItem key={option.value} value={option.value} disabled={runtimeSnapshot.data?.encoders?.[option.value]?.usable === false}>
                        {option.label}
                      </MenuItem>
                    ))}
                  </TextField>
                </Grid> : null}
                <Grid size={{ xs: 12, md: 4 }}>
                  <TextField
                    select
                    label="Color depth"
                    value={draft.pixFmt || stringFromRecord(profile?.workerConfig ?? {}, 'pixFmt') || (hardwareSelected ? defaultHardwareMain10PixelFormatForAsset(effectiveVideoEncoder) : 'yuv420p10le')}
                    onChange={(event) => onChange('pixFmt', event.target.value)}
                    size="small"
                    fullWidth
                  >
                    {compatibleAssetColorDepthOptions(hardwareSelected, effectiveVideoEncoder).map((option) => (
                      <MenuItem key={option.value} value={option.value} disabled={assetHardwareTenBitUnavailable(effectiveVideoEncoder, option.value, runtimeSnapshot.data?.encoders)}>{option.label}</MenuItem>
                    ))}
                  </TextField>
                </Grid>
                <Grid size={{ xs: 12, md: 4 }}>
                  <TextField
                    label={hardwareSelected ? 'Software fallback CRF' : 'Software CRF'}
                    type="number"
                    value={draft.qualityValue ?? profile?.qualityValue ?? 22}
                    onChange={(event) => onChange('qualityValue', Math.min(30, Math.max(14, Number(event.target.value))))}
                    inputProps={{ min: 14, max: 30, step: 1 }}
                    helperText={hardwareSelected ? 'Used only if hardware falls back to software.' : 'Lower preserves more detail.'}
                    size="small"
                    fullWidth
                  />
                </Grid>
                {hardwareSelected && effectiveVideoEncoder !== 'hevc_videotoolbox' ? <Grid size={{ xs: 12, md: 4 }}>
                  <TextField
                    label={qsvSelected ? 'QSV quality (ICQ)' : 'Hardware quality'}
                    type="number"
                    value={draft.globalQuality ?? Number(profile?.workerConfig?.globalQuality ?? qsvQualityRangeForCrf(draft.qualityValue ?? profile?.qualityValue ?? 22).recommended)}
                    onChange={(event) => onChange('globalQuality', Number(event.target.value))}
                    inputProps={{ min: 15, max: 35 }}
                    helperText={qsvAssetQualitySummary({ ...(profile?.workerConfig ?? {}), ...draft }) ?? qsvQualityHelper(draft.qualityValue ?? profile?.qualityValue ?? 22)}
                    size="small"
                    fullWidth
                  />
                </Grid> : null}
              </Grid>
              {qsvSelected ? (
                <Grid container spacing={1.5} sx={{ mt: 1 }}>
                  <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField title="The backend translates this intent using the active worker probe." select label="Quality preset" value={String(draft.hardwareQualityPreset ?? profile?.workerConfig?.hardwareQualityPreset ?? 'recommended')} onChange={(event) => selectHardwareQualityPreset(event.target.value, 'hevc_qsv')} size="small" fullWidth>{hardwareQualityPresetOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}</TextField></Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                    <TextField select label="QSV rate control" value={draft.qsvRateControl || stringFromRecord(profile?.workerConfig ?? {}, 'qsvRateControl') || 'icq'} onChange={(event) => onChange('qsvRateControl', event.target.value as 'icq' | 'la_icq')} size="small" fullWidth>
                      <MenuItem value="icq">ICQ</MenuItem>
                      <MenuItem value="la_icq" disabled={!qsvFeatures.rateControls.laIcq}>LA-ICQ · Main10 required</MenuItem>
                    </TextField>
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                    <TextField label="Look-ahead depth" type="number" value={draft.qsvLookAheadDepth ?? Number(profile?.workerConfig?.qsvLookAheadDepth ?? 40)} onChange={(event) => onChange('qsvLookAheadDepth', Number(event.target.value))} inputProps={{ min: 10, max: 100 }} disabled={!qsvFeatures.lookAhead} size="small" fullWidth />
                  </Grid>
                  <Grid size={{ xs: 12, md: 6 }}>
                    <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                      <FormControlLabel control={<Checkbox disabled={!qsvFeatures.extBrc && !(draft.qsvExtendedBrc ?? profile?.workerConfig?.qsvExtendedBRC === true)} checked={draft.qsvExtendedBrc ?? profile?.workerConfig?.qsvExtendedBRC === true} onChange={(event) => onChange('qsvExtendedBrc', event.target.checked)} />} label="Extended BRC" />
                      <FormControlLabel control={
                        <Checkbox
                          disabled={!qsvFeatures.adaptiveI && !(draft.qsvAdaptiveI ?? profile?.workerConfig?.qsvAdaptiveI === true)}
                          checked={draft.qsvAdaptiveI ?? profile?.workerConfig?.qsvAdaptiveI === true} 
                          onChange={(event) => onChange('qsvAdaptiveI', event.target.checked)
                        } 
                        />
                      } label="Adaptive I" />
                      <FormControlLabel control={
                        <Checkbox
                          disabled={!qsvFeatures.adaptiveB && !(draft.qsvAdaptiveB ?? profile?.workerConfig?.qsvAdaptiveB === true)}
                          checked={draft.qsvAdaptiveB ?? profile?.workerConfig?.qsvAdaptiveB === true} 
                          onChange={(event) => onChange('qsvAdaptiveB', event.target.checked)
                        }
                        />
                      } label="Adaptive B" />
                    </Stack>
                  </Grid>
                  <Grid size={{ xs: 12 }}>
                    <Typography variant="caption" color="text.secondary">
                      QSV low-power mode remains disabled for NAS compatibility. Unsupported features are omitted after the runtime smoke test.
                    </Typography>
                  </Grid>
                  {qsvWarnings.map((warning) => <Grid key={warning} size={{ xs: 12 }}><Alert severity="warning">{warning}</Alert></Grid>)}
                </Grid>
              ) : null}
              {videoToolboxSelected ? (
                <Grid container spacing={1.5} sx={{ mt: 1 }}>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <TextField title="Average target bitrate. Higher values preserve more detail and create larger files." label="VideoToolbox bitrate (Mbps)" type="number" value={Number(effectiveHardwarePresetConfig.videoToolboxBitrateMbps ?? 2)} onChange={(event) => onChange('videoToolboxBitrateMbps', Number(event.target.value))} inputProps={{ min: 0.01, max: 200, step: 0.01 }} size="small" fullWidth />
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <TextField title="Maximum short-term bitrate allowed during complex scenes." label="VideoToolbox maxrate (Mbps)" type="number" value={Number(effectiveHardwarePresetConfig.videoToolboxMaxrateMbps ?? 3)} onChange={(event) => onChange('videoToolboxMaxrateMbps', Number(event.target.value))} inputProps={{ min: 0.01, max: 250, step: 0.01 }} size="small" fullWidth />
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <TextField title="Rate-control buffer. Larger values give the encoder more freedom in complex scenes." label="VideoToolbox buffer (Mbps)" type="number" value={Number(effectiveHardwarePresetConfig.videoToolboxBufferMbps ?? 5)} onChange={(event) => onChange('videoToolboxBufferMbps', Number(event.target.value))} inputProps={{ min: 0.01, max: 500, step: 0.01 }} size="small" fullWidth />
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}><TextField label="Quality preset" select value={String(draft.hardwareQualityPreset ?? profile?.workerConfig?.hardwareQualityPreset ?? 'recommended')} onChange={(event) => selectHardwareQualityPreset(event.target.value, 'hevc_videotoolbox')} size="small" fullWidth>{hardwareQualityPresetOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}</TextField></Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}><TextField title="HEVC Main uses 8-bit output; Main10 uses 10-bit output and requires a compatible pixel format." label="Profile" value={draft.videoToolboxProfile ?? String(profile?.workerConfig?.videoToolboxProfile ?? '')} onChange={(event) => onChange('videoToolboxProfile', event.target.value)} placeholder="main or main10" helperText="Blank follows bit depth" size="small" fullWidth /></Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}><TextField title="Maximum distance between keyframes. Smaller values improve seeking but increase file size." label="GOP" type="number" value={draft.videoToolboxGop ?? Number(profile?.workerConfig?.videoToolboxGop ?? 0)} onChange={(event) => onChange('videoToolboxGop', Number(event.target.value))} inputProps={{ min: 0, max: 1000 }} helperText="0 = automatic" size="small" fullWidth /></Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}><TextField label="B-frames" select value={draft.videoToolboxBFramePolicy ?? String(profile?.workerConfig?.videoToolboxBFramePolicy ?? 'auto')} onChange={(event) => onChange('videoToolboxBFramePolicy', event.target.value as 'auto' | 'enabled' | 'disabled')} helperText="Auto emits no -bf option" size="small" fullWidth><MenuItem value="auto">Auto</MenuItem><MenuItem value="enabled" disabled={!videoToolboxBFramesAvailable}>Enabled</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField></Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}><TextField label="Maximum B-frames" type="number" disabled={(draft.videoToolboxBFramePolicy ?? String(profile?.workerConfig?.videoToolboxBFramePolicy ?? 'auto')) !== 'enabled'} value={draft.videoToolboxBFrames ?? Number(profile?.workerConfig?.videoToolboxBFrames ?? 3)} onChange={(event) => onChange('videoToolboxBFrames', Math.max(1, Math.min(4, Number(event.target.value))))} inputProps={{ min: 1, max: 4, step: 1 }} size="small" fullWidth /></Grid>
                  <Grid size={{ xs: 12 }}><Stack direction="row" spacing={2} flexWrap="wrap"><FormControlLabel title="Off is the default for offline conversion. Enable only for explicit low-latency work." control={<Checkbox checked={draft.videoToolboxRealtime ?? profile?.workerConfig?.videoToolboxRealtime === true} onChange={(event) => onChange('videoToolboxRealtime', event.target.checked)} />} label="Realtime" /><FormControlLabel title="Adjust target, maxrate and buffer for the effective B-frame strategy." control={<Checkbox checked={draft.videoToolboxAutoAdjustBitrate ?? profile?.workerConfig?.videoToolboxAutoAdjustBitrate === true} onChange={(event) => onChange('videoToolboxAutoAdjustBitrate', event.target.checked)} />} label="Auto-adjust bitrate for encoder strategy" /><FormControlLabel title="Available only after the matching VideoToolbox Main/Main10 power-efficiency probe succeeds." control={<Checkbox disabled={!videoToolboxPowerAvailable} checked={draft.videoToolboxPowerEfficiency ?? profile?.workerConfig?.videoToolboxPowerEfficiency === true} onChange={(event) => onChange('videoToolboxPowerEfficiency', event.target.checked)} />} label="Power efficiency" /></Stack></Grid>
                </Grid>
              ) : null}
              {encoderQualityRecommendation.isPending ? <Alert severity="info">Resolving this asset against the active worker probe…</Alert> : null}
              {encoderQualityRecommendation.isError ? <Alert severity="warning">Quality recommendation failed: {encoderQualityRecommendation.error instanceof Error ? encoderQualityRecommendation.error.message : 'unknown error'}</Alert> : null}
              {encoderQualityRecommendation.data ? <Stack spacing={1}><Stack direction="row" spacing={1} flexWrap="wrap"><Chip size="small" label={`Effective · ${encoderQualityRecommendation.data.recommendation.effectiveRateControl || 'bitrate'}`} /><Chip size="small" label={`Confidence · ${encoderQualityRecommendation.data.recommendation.estimateConfidence}`} />{encoderQualityRecommendation.data.estimatedOutputMaxBytes > 0 ? <Chip size="small" label={`Estimated · ${formatBytes(encoderQualityRecommendation.data.estimatedOutputMinBytes)}–${formatBytes(encoderQualityRecommendation.data.estimatedOutputMaxBytes)}`} /> : null}</Stack><Typography component="code" variant="caption" sx={{ overflowWrap: 'anywhere' }}>FFmpeg video: {encoderQualityRecommendation.data.ffmpegVideoArguments.join(' ')}</Typography></Stack> : null}
              {encoderQualityRecommendation.data?.recommendation.effectiveBFramePolicy ? <Alert severity={encoderQualityRecommendation.data.recommendation.bFrameDowngradeReason ? 'warning' : 'info'}>VideoToolbox B-frames: {encoderQualityRecommendation.data.recommendation.requestedBFramePolicy} → {encoderQualityRecommendation.data.recommendation.effectiveBFramePolicy} · efficiency ×{encoderQualityRecommendation.data.recommendation.bFrameEfficiencyMultiplier?.toFixed(2)} · target {((encoderQualityRecommendation.data.recommendation.targetBitrate ?? 0) / 1_000_000).toFixed(2)} Mbps{encoderQualityRecommendation.data.recommendation.bFrameDowngradeReason ? ` · ${encoderQualityRecommendation.data.recommendation.bFrameDowngradeReason}` : ''}</Alert> : null}
            </Box>
          </Grid>
          <Grid size={{ xs: 12, md: 4 }}>
            <TextField
              select
              label="Convert all subtitles"
              value={draft.externalSubtitleFormat || stringFromRecord(profile?.workerConfig ?? {}, 'externalSubtitleFormat') || 'source'}
              onChange={(event) => onChange('externalSubtitleFormat', event.target.value as 'disabled' | 'source' | 'srt' | 'ass' | 'remove')}
              helperText="Creates validated sidecars and removes embedded tracks. A Tracks Profile takes priority."
              size="small"
              fullWidth
            >
              <MenuItem value="disabled">Disabled · defer to Tracks Profile</MenuItem>
              <MenuItem value="source">Keep embedded tracks</MenuItem>
              <MenuItem value="srt">External SRT · remove embedded</MenuItem>
              <MenuItem value="ass">External ASS · remove embedded</MenuItem>
              <MenuItem value="remove">Remove embedded tracks</MenuItem>
            </TextField>
          </Grid>
          <Grid size={{ xs: 12 }}>
            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
              <OverrideSwitch
                label="Keep HDR"
                value={draft.preserveHdr}
                fallback={profile?.preserveHdr}
                onChange={(value) => onChange('preserveHdr', value)}
              />
              <OverrideSwitch
                label="Keep subtitles"
                value={draft.preserveSubtitles}
                fallback={profile?.preserveSubtitles}
                onChange={(value) => onChange('preserveSubtitles', value)}
              />
              <OverrideSwitch
                label="Keep chapters"
                value={draft.preserveChapters}
                fallback={profile?.preserveChapters}
                onChange={(value) => onChange('preserveChapters', value)}
              />
              <OverrideSwitch
                label="Add AAC stereo track"
                value={draft.addAacStereoTrack}
                fallback={profileAACTrackEnabled(profile)}
                onChange={(value) => onChange('addAacStereoTrack', value)}
              />
              <OverrideSwitch
                label="Make AAC stereo default"
                value={draft.aacStereoDefault}
                fallback={profileAACTrackDefault(profile)}
                onChange={(value) => onChange('aacStereoDefault', value)}
              />
            </Stack>
          </Grid>
          <Grid size={{ xs: 12 }}>
            <Button
              variant="text"
              onClick={() => setAdvancedOpen((current) => !current)}
              endIcon={
                <ExpandMoreIcon
                  sx={{
                    transform: advancedOpen ? 'rotate(180deg)' : 'rotate(0deg)',
                    transition: (theme) => theme.transitions.create('transform', { duration: theme.transitions.duration.shortest }),
                  }}
                />
              }
              sx={{ px: 0 }}
            >
              Advanced
            </Button>
            <Collapse in={advancedOpen} timeout="auto" unmountOnExit>
              <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5, mt: 1, bgcolor: 'rgba(255,255,255,0.025)' }}>
                <Grid container spacing={1.5}>
                  <Grid size={{ xs: 12, md: 6 }}>
                    <TextField
                      label="Custom FFmpeg video filters"
                      value={draft.videoFilters ?? ''}
                      onChange={(event) => onChange('videoFilters', event.target.value)}
                      placeholder={stringFromRecord(profile?.workerConfig ?? {}, 'videoFilters') || 'No custom filters'}
                      size="small"
                      fullWidth
                    />
                  </Grid>
                  <Grid size={{ xs: 12, md: 6 }}>
                    <TextField
                      label="Custom x265 params"
                      value={draft.x265Params ?? ''}
                      onChange={(event) => onChange('x265Params', event.target.value)}
                      placeholder={stringFromRecord(profile?.workerConfig ?? {}, 'x265Params') || 'No custom params'}
                      size="small"
                      fullWidth
                    />
                  </Grid>
                </Grid>
              </Box>
            </Collapse>
          </Grid>
        </Grid>
        </Box>
      </Stack>
    </Box>
  );
}

function OverrideSwitch({
  label,
  value,
  fallback,
  onChange,
}: {
  label: string;
  value?: boolean;
  fallback?: boolean;
  onChange: (value: boolean | undefined) => void;
}) {
  const effective = value ?? fallback ?? false;
  return (
    <Stack direction="row" spacing={0.5} alignItems="center" sx={{ border: 1, borderColor: 'divider', borderRadius: 1, px: 1, py: 0.5 }}>
      <Switch checked={effective} onChange={(event) => onChange(event.target.checked)} size="small" />
      <Typography variant="body2">{label}</Typography>
      {value === undefined ? <Chip label="Profile default" size="small" /> : <Chip label="Custom" color="primary" size="small" />}
      {value !== undefined ? (
        <Button size="small" onClick={() => onChange(undefined)}>
          Use profile
        </Button>
      ) : null}
    </Stack>
  );
}

function DirectPlaySnapshotComparison({ scan, job }: { scan: ScanResult; job?: QueueJob }) {
  const persistedSource = job ? recordFromRecord(job.validationReport ?? {}, 'directPlaySource') : {};
  const source = job ? persistedSource : scan.directPlayAnalysis;
  const output = job ? recordFromRecord(job.validationReport ?? {}, 'directPlay') : {};
  const scoreCard = (title: string, value: Record<string, unknown> | ScanResult['directPlayAnalysis'], missingText: string) => {
    const enabled = value?.enabled === true;
    const score = Number(value?.lowestScore);
    const available = enabled && Number.isFinite(score);
    return (
      <Card variant="outlined" sx={{ height: '100%' }}>
        <CardContent>
          <Stack spacing={1}>
            <Typography fontWeight={700}>{title}</Typography>
            <Typography variant="h2">{available ? `${score}/100` : 'Not evaluated'}</Typography>
            {available ? (
              <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                <Chip size="small" label={`Risk: ${String(value?.risk || 'unknown')}`} color={score >= Number(value?.minimumScore || 0) ? 'success' : 'warning'} />
                <Chip size="small" label={`Minimum: ${Number(value?.minimumScore || 0)}`} />
                <Chip size="small" label={String(value?.strategy || 'policy')} />
              </Stack>
            ) : <Typography variant="body2" color="text.secondary">{String(value?.error || missingText)}</Typography>}
          </Stack>
        </CardContent>
      </Card>
    );
  };
  return (
    <Stack spacing={1.25} sx={{ mt: 2 }}>
      <Typography variant="h3">Direct Play policy</Typography>
      <Typography variant="body2" color="text.secondary">Source and converted output are evaluated with the configured client targets, strategy, and minimum score.</Typography>
      <Grid container spacing={1.5}>
        <Grid size={{ xs: 12, md: 6 }}>{scoreCard('Original asset', source, job ? 'This older job did not preserve a Direct Play evaluation of its original.' : 'Process Asset to evaluate the original source.')}</Grid>
        <Grid size={{ xs: 12, md: 6 }}>{scoreCard('Converted output', output, job ? 'The job has no Direct Play result for its output.' : 'Available after conversion and job validation.')}</Grid>
      </Grid>
    </Stack>
  );
}

function FinalDetailsSummary({ asset, job, compact = false }: { asset: Asset; job?: QueueJob; compact?: boolean }) {
  const report = job?.validationReport ?? {};
  const mode = stringFromRecord(report, 'processingMode') || modeFromNotes(job?.notes ?? '');
  const audioProfile = job?.audioProfileKey || audioProfileFromNotes(job?.notes ?? '');
  const status = jobPublishesAsset(asset, job) ? 'Published' : job?.publicationRetiredAt ? 'Publication retired' : job?.status === 'completed' ? 'Converted' : job?.status || 'Converted';
  const command = conversionCommandFromNotes(job?.notes ?? '');
  const effectiveEncoder = encoderFromFFmpegCommand(command);
  const workerConfig = recordFromRecord(job?.profileSnapshot ?? {}, 'workerConfig');
  const requestedEncoder = stringFromRecord(workerConfig, 'videoEncoder') ||
    stringFromRecord(job?.profileSnapshot ?? {}, 'preferredEncoder') ||
    'Unknown';
  const colorPolicy = recordFromRecord(report, 'colorPolicy');
  const sourceColor = recordFromRecord(colorPolicy, 'source');
  const outputColor = recordFromRecord(colorPolicy, 'output');
  const frameFidelity = recordFromRecord(colorPolicy, 'frameFidelity');
  const frameFidelityFields = recordFromRecord(frameFidelity, 'fields');

  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, bgcolor: 'rgba(79,179,255,0.035)' }}>
      <Stack spacing={1.5}>
        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
          <Chip label={status} color={job?.status === 'failed' ? 'error' : 'success'} size="small" />
          {job ? <Chip label={job.executionNumber ? `Job #${job.executionNumber}` : 'Pending'} size="small" /> : null}
          {job?.validationStatus ? <Chip label={`Validation: ${job.validationStatus} ${job.validationScore}/100`} size="small" /> : null}
        </Stack>
        <Typography fontWeight={700}>What changed</Typography>
        <Stack spacing={0.75}>
          <Typography color="text.secondary">
            Video: {mode === 'audio_only' ? 'kept original video stream' : 'processed with the selected video profile'}.
          </Typography>
          <Typography color="text.secondary">
            Audio: {audioProfile ? `enhanced with ${audioProfile}; original audio may be preserved depending on the profile.` : 'kept or converted according to the selected profile.'}
          </Typography>
          <Typography color="text.secondary">
            Container/output: final asset is available as {asset.fileName}.
          </Typography>
          {job?.publishedPath ? (
            <Typography color="text.secondary" sx={{ wordBreak: 'break-all' }}>
              Published path: {job.publishedPath}
            </Typography>
          ) : null}
          {job?.outputPath && !compact ? (
            <Typography color="text.secondary" sx={{ wordBreak: 'break-all' }}>
              Staging/output path: {job.outputPath}
            </Typography>
          ) : null}
        </Stack>
        {job ? (
          <>
            <Divider />
            <Typography fontWeight={700}>Conversion execution</Typography>
            <Grid container spacing={1}>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Typography variant="caption" color="text.secondary">Requested encoder</Typography>
                <Typography>{requestedEncoder}</Typography>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Typography variant="caption" color="text.secondary">Effective encoder</Typography>
                <Typography>{effectiveEncoder || 'Unknown'}</Typography>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Typography variant="caption" color="text.secondary">Worker</Typography>
                <Typography>{job.workerName || 'Unknown'}</Typography>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Typography variant="caption" color="text.secondary">Processing mode</Typography>
                <Typography>{mode || 'Unknown'}</Typography>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Typography variant="caption" color="text.secondary">Profile</Typography>
                <Typography>#{job.profileId} · version {job.profileVersion || 1}</Typography>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                <Typography variant="caption" color="text.secondary">Completed</Typography>
                <Typography>{job.finishedAt ? new Date(job.finishedAt).toLocaleString() : 'Unknown'}</Typography>
              </Grid>
            </Grid>
            {command ? (
              <Box
                component="pre"
                sx={{
                  border: 1,
                  borderColor: 'divider',
                  borderRadius: 1,
                  m: 0,
                  p: 1.5,
                  whiteSpace: 'pre-wrap',
                  wordBreak: 'break-word',
                  color: 'text.secondary',
                  bgcolor: 'rgba(255,255,255,0.025)',
                  maxHeight: 240,
                  overflow: 'auto',
                }}
              >
                {command}
              </Box>
            ) : (
              <Alert severity="warning">This job record does not contain its FFmpeg conversion command.</Alert>
            )}
            {Object.keys(colorPolicy).length > 0 ? (
              <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
                <Stack spacing={1}>
                  <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                    <Typography fontWeight={700}>Final color validation</Typography>
                    <Chip
                      size="small"
                      label={stringFromRecord(colorPolicy, 'status') || 'unverified'}
                      color={stringFromRecord(colorPolicy, 'status') === 'passed' ? 'success' : 'warning'}
                    />
                    <Chip size="small" label={`Requested: ${stringFromRecord(colorPolicy, 'requestedPolicy') || 'automatic'}`} />
                    <Chip size="small" label={`Effective: ${stringFromRecord(colorPolicy, 'effectivePolicy') || 'unknown'}`} />
                  </Stack>
                  <Typography variant="body2" color="text.secondary">
                    Source: {colorCharacteristicsLabel(sourceColor)}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    Output: {colorCharacteristicsLabel(outputColor)}
                  </Typography>
                </Stack>
              </Box>
            ) : null}
            {Object.keys(frameFidelityFields).length > 0 ? (
              <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
                <Stack spacing={1}>
                  <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
                    <Typography fontWeight={700}>Frame fidelity validation</Typography>
                    <Chip size="small" label={stringFromRecord(frameFidelity, 'status') || 'unverified'} color={stringFromRecord(frameFidelity, 'status') === 'passed' ? 'success' : 'warning'} />
                  </Stack>
                  <Grid container spacing={1}>
                    {Object.entries(frameFidelityFields).map(([key, raw]) => {
                      const field = raw && typeof raw === 'object' && !Array.isArray(raw) ? raw as Record<string, unknown> : {};
                      const status = stringFromRecord(field, 'status') || 'unverified';
                      return (
                        <Grid key={key} size={{ xs: 12, sm: 6, md: 4 }}>
                          <Typography variant="caption" color="text.secondary">{frameFidelityLabel(key)}</Typography>
                          <Typography variant="body2">{stringFromRecord(field, 'source') || 'Unknown'} → {stringFromRecord(field, 'output') || 'Unknown'}</Typography>
                          <Chip size="small" label={status.replaceAll('_', ' ')} color={status === 'changed_unexpectedly' ? 'warning' : 'default'} />
                        </Grid>
                      );
                    })}
                  </Grid>
                </Stack>
              </Box>
            ) : null}
          </>
        ) : null}
        <Alert severity="info">
          AS-IS and result JSON reports are saved in the configured reports folders. This view summarizes the final job metadata and current asset snapshot for quick human review.
        </Alert>
        {!job ? <Alert severity="warning">No matching job record was found for this converted asset yet.</Alert> : null}
        {job?.notes && !compact ? (
          <Box
            component="pre"
            sx={{
              border: 1,
              borderColor: 'divider',
              borderRadius: 1,
              m: 0,
              p: 1.5,
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              color: 'text.secondary',
              bgcolor: 'rgba(255,255,255,0.025)',
            }}
          >
            {job.notes}
          </Box>
        ) : null}
      </Stack>
    </Box>
  );
}

function AssetCategorySelect({
  value,
  options,
  onChange,
  label,
  size,
  disabled,
}: {
  value: string;
  options: string[];
  onChange: (value: string) => void;
  label: string;
  size?: 'small' | 'medium';
  disabled?: boolean;
}) {
  return (
    <Autocomplete
      options={normalizeStringList([...options, value])}
      value={value || null}
      onChange={(_, nextValue) => onChange(nextValue ?? '')}
      disabled={disabled}
      renderInput={(params) => <TextField {...params} label={label} placeholder="Choose category" size={size} />}
      size={size}
      fullWidth
    />
  );
}

function ProfileAutocomplete({
  profiles,
  value,
  onChange,
  label,
  size,
  disabled,
  allowNone = false,
}: {
  profiles: Profile[];
  value: number;
  onChange: (id: number) => void;
  label: string;
  size?: 'small' | 'medium';
  disabled?: boolean;
  allowNone?: boolean;
}) {
  const options = [
    ...(allowNone ? [{ id: -1, name: 'Disabled', videoCodec: 'No video profile', search: 'disabled none no profile' }] : []),
    ...profiles.map((profile) => ({ id: profile.id, name: profile.name, videoCodec: profile.videoCodec, search: `${profile.name} ${profile.description} ${profile.container} ${profile.videoCodec} ${profile.audioCodec}` })),
  ];
  return (
    <Autocomplete
      options={options}
      value={options.find((profile) => profile.id === value) ?? null}
      onChange={(_, profile) => onChange(profile?.id ?? 0)}
      getOptionLabel={(profile) => profile.id === -1 ? 'Disabled' : `${profile.name} · ${profile.videoCodec}`}
      isOptionEqualToValue={(option, selected) => option.id === selected.id}
      filterOptions={(options, state) =>
        filterByText(options, state.inputValue, (profile) => [profile.search])
      }
      disabled={disabled}
      renderInput={(params) => <TextField {...params} label={label} size={size} />}
      fullWidth
    />
  );
}

function AudioProfileAutocomplete({
  profiles,
  value,
  onChange,
  label,
  size,
  disabled,
}: {
  profiles: AudioEnhancementProfile[];
  value: string;
  onChange: (key: string) => void;
  label: string;
  size?: 'small' | 'medium';
  disabled?: boolean;
}) {
  const none: AudioEnhancementProfile = {
    key: '', name: 'Disabled', description: '', intent: '', filters: '', rnnoiseModelPath: '', channelMode: 'preserve',
    forceStereoMode: 'auto', stereoDelayMs: 0, stereoWidth: 0, eqBands: {}, preserveOriginalTrack: true,
    outputCodec: 'copy', targetLoudness: 0, truePeak: 0, notes: '',
  };
  const options = [none, ...profiles];
  return (
    <Autocomplete
      options={options}
      value={options.find((profile) => profile.key === value) ?? none}
      onChange={(_, profile) => onChange(profile?.key ?? '')}
      getOptionLabel={(profile) => profile.key ? `${profile.name} · ${profile.outputCodec || 'copy'}` : 'Disabled'}
      isOptionEqualToValue={(option, selected) => option.key === selected.key}
      filterOptions={(options, state) =>
        filterByText(options, state.inputValue, (profile) => [
          profile.name,
          profile.description,
          profile.intent,
          profile.outputCodec,
          profile.filters,
        ])
      }
      disabled={disabled}
      renderInput={(params) => <TextField {...params} label={label} placeholder="No audio enhancement" size={size} />}
      fullWidth
    />
  );
}

function TrackProfileAutocomplete({ profiles, value, onChange, disabled, label = 'Tracks for path' }: { profiles: TrackProfile[]; value: string; onChange: (key: string) => void; disabled?: boolean; label?: string }) {
  const none: TrackProfile = {
    key: '', name: 'Disabled', description: '', videoMode: 'first', audioMode: 'all', audioLanguages: [], audioRequired: false,
    dropCommentary: false, defaultAudioLanguage: '', subtitleMode: 'all', subtitleLanguages: [], subtitlesRequired: false,
    defaultSubtitleLanguage: '', validationMode: 'warn', notes: '',
  };
  const options = [none, ...profiles];
  return <Autocomplete options={options} value={options.find((profile) => profile.key === value) ?? none} onChange={(_, profile) => onChange(profile?.key ?? '')} getOptionLabel={(profile) => profile.name} isOptionEqualToValue={(option, selected) => option.key === selected.key} disabled={disabled} renderInput={(params) => <TextField {...params} label={label} size="small" />} fullWidth />;
}

function getTrackProfilePathAssignments(settings: AppSetting[]) {
  const value = settings.find((setting) => setting.key === 'trackProfilePathAssignments')?.value.assignments;
  if (!value || typeof value !== 'object') return {} as Record<string, string>;
  return Object.fromEntries(Object.entries(value as Record<string, unknown>).filter((entry): entry is [string, string] => typeof entry[1] === 'string'));
}

function validateTrackProfile(profile: TrackProfile, scan: ScanResult) {
  const reasons: string[] = [];
  const checkIndexes = (label: string, requested: number[] | undefined, streams: MediaStreamInfo[]) => {
    if (!requested) return;
    const available = new Set(streams.map((stream) => stream.index));
    const missing = requested.filter((index) => !available.has(index));
    if (missing.length) reasons.push(`${label} streams missing: ${missing.join(', ')}`);
  };
  checkIndexes('video', profile.keepVideoStreams, scan.videoStreams);
  checkIndexes('audio', profile.keepAudioStreams, scan.audioStreams);
  checkIndexes('subtitle', profile.keepSubtitleStreams, scan.subtitleStreams);
  if (profile.videoMode === 'require-one' && scan.videoStreams.length !== 1) reasons.push(`requires exactly one video stream; found ${scan.videoStreams.length}`);
  const languages = (streams: MediaStreamInfo[]) => new Set(streams.map((stream) => stream.language.toLowerCase()).filter(Boolean));
  if (profile.audioRequired && scan.audioStreams.length === 0) reasons.push('requires an audio stream');
  if (profile.audioMode === 'languages' && profile.audioRequired && !profile.audioLanguages.some((language) => languages(scan.audioStreams).has(language))) reasons.push(`required audio languages not found: ${profile.audioLanguages.join(', ')}`);
  if (profile.subtitlesRequired && scan.subtitleStreams.length === 0) reasons.push('requires a subtitle stream');
  if ((profile.subtitleMode === 'languages' || profile.subtitleMode === 'forced-or-languages') && profile.subtitlesRequired) {
    const hasLanguage = profile.subtitleLanguages.some((language) => languages(scan.subtitleStreams).has(language));
    const hasForced = scan.subtitleStreams.some((stream) => stream.forced);
    if (!hasLanguage && !(profile.subtitleMode === 'forced-or-languages' && hasForced)) reasons.push(`required subtitle languages not found: ${profile.subtitleLanguages.join(', ')}`);
  }
  return { applies: reasons.length === 0, reasons };
}

function LibraryAutocomplete({
  libraries,
  value,
  onChange,
  label,
  size,
  disabled,
}: {
  libraries: Library[];
  value: number;
  onChange: (id: number) => void;
  label: string;
  size?: 'small' | 'medium';
  disabled?: boolean;
}) {
  return (
    <Autocomplete
      options={libraries}
      value={libraries.find((library) => library.id === value) ?? null}
      onChange={(_, library) => onChange(library?.id ?? 0)}
      getOptionLabel={(library) => library.name}
      isOptionEqualToValue={(option, selected) => option.id === selected.id}
      filterOptions={(options, state) =>
        filterByText(options, state.inputValue, (library) => [library.name, library.type, library.destinationPath])
      }
      disabled={disabled}
      renderInput={(params) => <TextField {...params} label={label} size={size} />}
      fullWidth
    />
  );
}

function filterByText<T>(items: T[] | null | undefined, inputValue: string, getValues: (item: T) => Array<string | null | undefined>) {
  const visibleItems = safeArray(items);
  const query = inputValue.trim().toLowerCase();
  if (!query) {
    return visibleItems.slice(0, 50);
  }

  return visibleItems
    .filter((item) => getValues(item).some((value) => (value ?? '').toLowerCase().includes(query)))
    .slice(0, 50);
}

function safeArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

function filterAssetGroups(groups: AssetGroup[], query: string) {
  const cleanQuery = query.trim().toLowerCase();
  if (!cleanQuery) {
    return groups;
  }
  return groups.filter((group) => {
    const values = [
      group.path,
      group.relativePath,
      group.libraryName,
      group.status,
      ...safeArray(group.assets).flatMap((asset) => [asset.fileName, asset.path, asset.relativePath, asset.status]),
    ];
    return values.some((value) => value.toLowerCase().includes(cleanQuery));
  });
}

function sumGroupFiles(groups: AssetGroup[]) {
  return groups.reduce((total, group) => total + (group.fileCount || safeArray(group.assets).length), 0);
}

function sumGroupBytes(groups: AssetGroup[]) {
  return groups.reduce((total, group) => total + (Number.isFinite(group.sizeBytes) ? group.sizeBytes : 0), 0);
}

function recommendationLabel(recommendation: AdvisorResponse['recommendation']) {
  switch (recommendation) {
    case 'worth_it':
      return 'Worth it';
    case 'maybe':
      return 'Maybe';
    default:
      return 'Not recommended';
  }
}

function recommendationColor(recommendation: AdvisorResponse['recommendation']) {
  switch (recommendation) {
    case 'worth_it':
      return 'success';
    case 'maybe':
      return 'warning';
    default:
      return 'default';
  }
}

function statusLabel(status: Asset['status']) {
  switch (status) {
    case 'converted':
      return 'Converted';
    case 'library':
      return 'Library';
    case 'published_as_is':
      return 'Published as-is';
    case 'unverified':
      return 'Unverified';
    case 'archive':
      return 'Archive';
    default:
      return 'Unprocessed';
  }
}

function statusColor(status: Asset['status']): 'default' | 'success' | 'warning' {
  switch (status) {
    case 'converted':
      return 'success';
    case 'library':
      return 'default';
    case 'published_as_is':
      return 'success';
    case 'unverified':
      return 'warning';
    case 'archive':
      return 'default';
    default:
      return 'warning';
  }
}

function firstCategory(categories?: string[] | null) {
  return safeArray(categories).find((category) => category.trim()) ?? '';
}

function assetDisplayPath(asset: Asset, groupRelativePath: string, libraries: Library[]) {
  const relativePath = cleanRelativePath(relativeAssetPath(asset, libraries));
  const groupPrefix = groupRelativePath ? `${groupRelativePath}/` : '';
  if (groupPrefix && relativePath.startsWith(groupPrefix)) {
    return relativePath.slice(groupPrefix.length);
  }

  const parts = relativePath.split('/').filter(Boolean);
  if (parts.length <= 2) {
    return asset.fileName;
  }
  return parts.slice(2).join('/');
}

function assetTitle(asset: Asset) {
  return cleanRelativePath(asset.fileName || relativeAssetPath(asset, []));
}

function assetSubpath(asset: Asset, libraries: Library[]) {
  return shortSubpath(relativeAssetPath(asset, libraries));
}

function relativeAssetPath(asset: Asset, libraries: Library[]) {
  if (asset.relativePath) {
    return cleanRelativePath(asset.relativePath);
  }

  const library = libraries.find((candidate) => candidate.id === asset.libraryId);
  const basePath = asset.status === 'converted' || asset.status === 'unverified' || asset.status === 'library' || asset.status === 'published_as_is' ? library?.destinationPath : library?.sourcePath;

  if (!basePath) {
    return asset.fileName;
  }

  const normalizedBase = basePath.endsWith('/') ? basePath : `${basePath}/`;
  if (asset.path.startsWith(normalizedBase)) {
    return cleanRelativePath(asset.path.slice(normalizedBase.length));
  }

  return asset.fileName;
}

function groupDisplayPath(group: AssetGroup) {
  if (group.relativePath) {
    return cleanRelativePath(group.relativePath);
  }

  const assets = safeArray(group.assets);
  if (assets.length === 1) {
    return assets[0].fileName;
  }

  return 'Library root';
}

function groupTitle(group: AssetGroup) {
  const relativePath = cleanRelativePath(group.relativePath || groupDisplayPath(group));
  if (!relativePath || relativePath === 'Library root') {
    return 'Library root';
  }
  const parts = relativePath.split('/').filter(Boolean);
  return parts[parts.length - 1] ?? relativePath;
}

function groupSubpath(group: AssetGroup) {
  const relativePath = cleanRelativePath(group.relativePath || groupDisplayPath(group));
  if (!relativePath || relativePath === 'Library root') {
    return '/';
  }
  return shortSubpath(relativePath);
}

function cleanRelativePath(value: string) {
  return value.replace(/\\/g, '/').replace(/\/{2,}/g, '/').replace(/^\/+/, '');
}

function cleanPathLabel(value: string) {
  return value.replace(/\\/g, '/').replace(/\/{2,}/g, '/');
}

function shortSubpath(value: string) {
  const clean = cleanRelativePath(value);
  return clean ? `/${clean}` : '/';
}

function firstAssetForGroup(assets: Asset[]) {
  return [...safeArray(assets)].sort((left, right) => left.path.localeCompare(right.path))[0];
}

function assetHasOpenJob(asset: Asset, jobs: QueueJob[]) {
  return safeArray(jobs).some((job) => job.mediaPath === asset.path && job.status !== 'canceled' && !job.publishedAt);
}

function assetReviewApproved(asset: Asset) {
  return !asset.review?.requiresReview && asset.review?.source === 'manual' && Boolean(asset.review.updatedAt);
}

function associatedJobForAsset(asset: Asset, jobs: QueueJob[]) {
  const normalizedAssetPath = normalizePath(asset.path);
  return [...safeArray(jobs)]
    .sort((left, right) => right.id - left.id)
    .filter((job) => !job.publicationRetiredAt)
    .find((job) => {
      const candidates = [job.publishedPath, job.outputPath, job.mediaPath].map(normalizePath).filter(Boolean);
      return candidates.includes(normalizedAssetPath);
    });
}

function relatedVideoProfile(profiles: Profile[], asset: Asset) {
  const assetPath = normalizePath(asset.path);
  return [...profiles].reverse().find((profile) =>
    normalizePath(stringFromRecord(profile.workerConfig, 'derivedFromAsset')) === assetPath,
  );
}

function relatedAudioProfile(profiles: AudioEnhancementProfile[], asset: Asset) {
  const marker = `Lab asset: ${asset.path}`;
  return [...profiles].reverse().find((profile) => profile.notes.split('\n').some((line) => line.trim() === marker));
}

function relatedTrackProfile(profiles: TrackProfile[], asset: Asset) {
  const assetPath = normalizePath(asset.path);
  return [...profiles].reverse().find((profile) =>
    normalizePath(profile.sourceAssetPath ?? '') === assetPath ||
    (!profile.sourceAssetPath && Boolean(profile.sourceAssetName) && profile.sourceAssetName === asset.fileName),
  );
}

function directPlayScoreLabel(job: QueueJob) {
  const directPlay = recordFromRecord(job.validationReport ?? {}, 'directPlay');
  const enabled = directPlay.enabled === true;
  const score = Number(directPlay.lowestScore);
  if (!enabled || !Number.isFinite(score)) return 'Not evaluated';
  return `${score}/100`;
}

function assetPipelineState(asset: Asset, job: QueueJob | undefined, pendingQueue: boolean): { label: string; color: 'default' | 'primary' | 'success' | 'warning' | 'error' } {
  if (asset.status === 'archive') {
    return { label: 'Archive', color: 'default' };
  }
  if (pendingQueue) {
    return { label: 'Queueing', color: 'primary' };
  }
  if (jobPublishesAsset(asset, job)) {
    return { label: 'Published', color: 'success' };
  }
  const activeJob = job?.publishedAt && !job.publicationRetiredAt ? undefined : job;
  if (activeJob?.status === 'running') {
    return { label: 'Worker', color: 'primary' };
  }
  if (activeJob?.status === 'queued') {
    return { label: 'Queued', color: 'primary' };
  }
  if (activeJob?.status === 'failed') {
    return { label: 'Failed', color: 'error' };
  }
  if (activeJob?.status === 'canceled') {
    return { label: 'Canceled', color: 'default' };
  }
  if (activeJob?.status === 'completed' && !activeJob.validationStatus) {
    return { label: 'Analysis', color: 'warning' };
  }
  if (activeJob?.validationStatus === 'passed' || activeJob?.validationStatus === 'warning') {
    return { label: 'Publisher', color: 'primary' };
  }
  if (asset.status === 'converted') {
    return { label: 'Converted', color: 'success' };
  }
  if (asset.status === 'unverified' || asset.status === 'library' || asset.status === 'published_as_is') {
    return { label: 'Unverified', color: 'warning' };
  }
  return { label: 'Unprocessed', color: 'warning' };
}

function jobPublishesAsset(asset: Asset, job?: QueueJob) {
  return Boolean(
    job?.publishedAt
    && !job.publicationRetiredAt
    && normalizePath(job.publishedPath) === normalizePath(asset.path),
  );
}

function queueNotes(base: string, audioProfileKey: string) {
  if (!audioProfileKey) {
    return base;
  }
  return `${base}\nAudio profile: ${audioProfileKey}`;
}

function priorityForSize(sizeBytes: number) {
  const gib = sizeBytes / 1024 / 1024 / 1024;
  if (gib >= 8) {
    return 1;
  }
  if (gib >= 4) {
    return 2;
  }
  if (gib >= 1) {
    return 3;
  }
  if (gib >= 0.25) {
    return 5;
  }
  return 8;
}

function normalizePath(value: string) {
  return value.replace(/\\/g, '/').replace(/\/+$/, '');
}

function isBitmapSubtitleCodec(codec: string) {
  return ['dvd_subtitle', 'hdmv_pgs_subtitle', 'pgssub', 'dvb_subtitle', 'xsub'].includes(codec.toLowerCase());
}

function isHardwareAssetEncoder(value: string) {
  return ['hevc_qsv', 'hevc_vaapi', 'hevc_nvenc', 'hevc_videotoolbox', 'hevc_amf'].includes(value);
}

function hardwareEncodingSupportedForAssetCodec(codec: string) {
  const normalized = codec.toLowerCase();
  return normalized.includes('265') || normalized.includes('hevc');
}

function defaultHardwareMain10PixelFormatForAsset(encoder: string) {
  return encoder === 'hevc_qsv' || encoder === 'hevc_vaapi' || encoder === 'hevc_videotoolbox' ? 'p010le' : 'yuv420p10le';
}

function compatibleAssetColorDepthOptions(hardware: boolean, encoder: string) {
  if (!hardware) {
    return colorDepthOptions.filter((option) => ['', 'auto', 'yuv420p10le', 'yuv420p'].includes(option.value));
  }
  if (encoder === 'hevc_qsv' || encoder === 'hevc_vaapi') {
    return colorDepthOptions.filter((option) => ['', 'auto', 'p010le', 'nv12'].includes(option.value));
  }
  if (encoder === 'hevc_videotoolbox') {
    return colorDepthOptions.filter((option) => ['', 'auto', 'p010le', 'yuv420p'].includes(option.value));
  }
  return colorDepthOptions.filter((option) => ['', 'auto', 'yuv420p10le', 'yuv420p'].includes(option.value));
}

function assetHardwareTenBitUnavailable(encoder: string, pixelFormat: string, encoders?: Record<string, { main10?: boolean; videoToolboxMain10?: boolean }>) {
  if (pixelFormat !== 'p010le') return false;
  if (encoder === 'hevc_qsv') return encoders?.hevc_qsv?.main10 === false;
  if (encoder === 'hevc_videotoolbox') return encoders?.hevc_videotoolbox?.videoToolboxMain10 === false;
  return false;
}

function stringFromRecord(record: Record<string, unknown>, key: string) {
  const value = record[key];
  return typeof value === 'string' ? value : '';
}

function recordFromRecord(record: Record<string, unknown>, key: string): Record<string, unknown> {
  const value = record[key];
  return value && typeof value === 'object' && !Array.isArray(value)
    ? value as Record<string, unknown>
    : {};
}

function conversionCommandFromNotes(notes: string) {
  const match = notes.match(/Conversion command:\s*(ffmpeg[^\n\r]*)/i);
  return match?.[1]?.trim() ?? '';
}

function encoderFromFFmpegCommand(command: string) {
  const matches = [...command.matchAll(/(?:^|\s)-(?:c:v|codec:v|vcodec)\s+(?:"([^"]+)"|'([^']+)'|([^\s]+))/gi)];
  const match = matches.at(-1);
  return match ? (match[1] || match[2] || match[3] || '').trim() : '';
}

function colorCharacteristicsLabel(value: Record<string, unknown>) {
  if (!Object.keys(value).length) return 'Unknown';
  return [
    stringFromRecord(value, 'colorSpace') || 'space unknown',
    stringFromRecord(value, 'colorTransfer') || 'transfer unknown',
    stringFromRecord(value, 'colorPrimaries') || 'primaries unknown',
    stringFromRecord(value, 'colorRange') || 'range unknown',
    stringFromRecord(value, 'pixelFormat') || '',
  ].filter(Boolean).join(' · ');
}

function frameFidelityLabel(key: string) {
  return ({ sampleAspectRatio: 'SAR', displayAspectRatio: 'DAR', frameRate: 'Frame rate', chromaLocation: 'Chroma location', fieldOrder: 'Field order', pixelFormat: 'Pixel format', bitDepth: 'Bit depth', frameSize: 'Frame size' } as Record<string, string>)[key] || key;
}

function modeFromNotes(notes: string) {
  const match = notes.match(/Processing mode:\s*([^\n]+)/i);
  return match?.[1]?.trim() ?? '';
}

function audioProfileFromNotes(notes: string) {
  const match = notes.match(/Audio (?:enhancement )?profile:\s*([^\n]+)/i);
  return match?.[1]?.trim() ?? '';
}

function normalizeAssetConversionOverride(value?: AssetConversionOverrideState): AssetConversionOverrideState {
  if (!value) {
    return {};
  }
  const normalized = cleanConversionOverride({
    ...value,
    keepVideoStreams: Array.isArray(value.keepVideoStreams) ? normalizeNumberList(value.keepVideoStreams) : undefined,
    keepAudioStreams: Array.isArray(value.keepAudioStreams) ? normalizeNumberList(value.keepAudioStreams) : undefined,
    keepSubtitleStreams: Array.isArray(value.keepSubtitleStreams) ? normalizeNumberList(value.keepSubtitleStreams) : undefined,
  });
  if (!normalized.preferredEncoder && typeof normalized.useHardwareIfAvailable === 'boolean') {
    normalized.preferredEncoder = normalized.useHardwareIfAvailable ? 'hardware' : 'software';
    if (normalized.useHardwareIfAvailable && !normalized.videoEncoder) {
      normalized.videoEncoder = 'auto';
    }
  }
  return normalized;
}

function cleanConversionOverride(value: AssetConversionOverrideState): AssetConversionOverrideState {
  const clean: AssetConversionOverrideState = {};
  if (value.trackProfileKey?.trim()) {
    clean.trackProfileKey = value.trackProfileKey.trim();
  }
  if (Array.isArray(value.keepVideoStreams)) {
    clean.keepVideoStreams = normalizeNumberList(value.keepVideoStreams);
  }
  if (Array.isArray(value.keepAudioStreams)) {
    clean.keepAudioStreams = normalizeNumberList(value.keepAudioStreams);
  }
  if (Array.isArray(value.keepSubtitleStreams)) {
    clean.keepSubtitleStreams = normalizeNumberList(value.keepSubtitleStreams);
  }
  const audioMetadata = cleanStreamMetadataMap(value.audioMetadata);
  const subtitleMetadata = cleanStreamMetadataMap(value.subtitleMetadata);
  const videoMetadata = cleanStreamMetadataMap(value.videoMetadata);
  if (videoMetadata) {
    clean.videoMetadata = videoMetadata;
  }
  if (audioMetadata) {
    clean.audioMetadata = audioMetadata;
  }
  if (subtitleMetadata) {
    clean.subtitleMetadata = subtitleMetadata;
  }
  if (Array.isArray(value.subtitleTransforms) && value.subtitleTransforms.length) {
    clean.subtitleTransforms = value.subtitleTransforms;
  }
  ([
    'videoCodec',
    'audioCodec',
    'qualityMode',
    'videoPreset',
    'pixFmt',
    'videoFilters',
    'x265Params',
    'videoEncoder',
  ] as const).forEach((key) => {
    const text = value[key]?.trim();
    if (text) {
      clean[key] = text;
    }
  });
  if (value.externalSubtitleFormat === 'disabled' || value.externalSubtitleFormat === 'source' || value.externalSubtitleFormat === 'srt' || value.externalSubtitleFormat === 'ass' || value.externalSubtitleFormat === 'remove') {
    clean.externalSubtitleFormat = value.externalSubtitleFormat;
  }
  if (value.finalColorPolicy === 'automatic' || value.finalColorPolicy === 'preserve' || value.finalColorPolicy === 'normalize_bt709') {
    clean.finalColorPolicy = value.finalColorPolicy;
  }
  if (value.deinterlaceMode === 'auto' || value.deinterlaceMode === 'off' || value.deinterlaceMode === 'force' || value.deinterlaceMode === 'ivtc_tff' || value.deinterlaceMode === 'ivtc_bff') {
    clean.deinterlaceMode = value.deinterlaceMode;
  }
  if (value.qsvRateControl === 'icq' || value.qsvRateControl === 'la_icq') {
    clean.qsvRateControl = value.qsvRateControl;
  }
  if (value.preferredEncoder === 'software' || value.preferredEncoder === 'hardware' || value.preferredEncoder === 'auto') {
    clean.preferredEncoder = value.preferredEncoder;
  }
  if (typeof value.qualityValue === 'number' && Number.isFinite(value.qualityValue) && value.qualityValue > 0) {
    clean.qualityValue = value.qualityValue;
  }
  if (typeof value.preserveHdr === 'boolean') {
    clean.preserveHdr = value.preserveHdr;
  }
  if (typeof value.preserveSubtitles === 'boolean') {
    clean.preserveSubtitles = value.preserveSubtitles;
  }
  if (typeof value.preserveChapters === 'boolean') {
    clean.preserveChapters = value.preserveChapters;
  }
  if (typeof value.addAacStereoTrack === 'boolean') {
    clean.addAacStereoTrack = value.addAacStereoTrack;
  }
  if (typeof value.aacStereoDefault === 'boolean') {
    clean.aacStereoDefault = value.aacStereoDefault;
  }
  if (typeof value.enhancedAudioSourceStreamIndex === 'number' && Number.isInteger(value.enhancedAudioSourceStreamIndex) && value.enhancedAudioSourceStreamIndex >= 0) {
    clean.enhancedAudioSourceStreamIndex = value.enhancedAudioSourceStreamIndex;
  }
  if (typeof value.useHardwareIfAvailable === 'boolean') {
    clean.useHardwareIfAvailable = value.useHardwareIfAvailable;
  }
  if (typeof value.globalQuality === 'number' && Number.isFinite(value.globalQuality) && value.globalQuality > 0) {
    clean.globalQuality = value.globalQuality;
  }
  if (typeof value.qsvLookAheadDepth === 'number' && Number.isFinite(value.qsvLookAheadDepth) && value.qsvLookAheadDepth > 0) {
    clean.qsvLookAheadDepth = Math.min(100, Math.max(10, Math.round(value.qsvLookAheadDepth)));
  }
  if (typeof value.qsvExtendedBrc === 'boolean') clean.qsvExtendedBrc = value.qsvExtendedBrc;
  if (typeof value.qsvAdaptiveI === 'boolean') clean.qsvAdaptiveI = value.qsvAdaptiveI;
  if (typeof value.qsvAdaptiveB === 'boolean') clean.qsvAdaptiveB = value.qsvAdaptiveB;
  if (typeof value.videoToolboxBitrateMbps === 'number' && Number.isFinite(value.videoToolboxBitrateMbps) && value.videoToolboxBitrateMbps > 0) clean.videoToolboxBitrateMbps = Math.min(200, Math.round(value.videoToolboxBitrateMbps * 100) / 100);
  if (typeof value.videoToolboxMaxrateMbps === 'number' && Number.isFinite(value.videoToolboxMaxrateMbps) && value.videoToolboxMaxrateMbps > 0) clean.videoToolboxMaxrateMbps = Math.min(250, Math.round(value.videoToolboxMaxrateMbps * 100) / 100);
  if (typeof value.videoToolboxBufferMbps === 'number' && Number.isFinite(value.videoToolboxBufferMbps) && value.videoToolboxBufferMbps > 0) clean.videoToolboxBufferMbps = Math.min(500, Math.round(value.videoToolboxBufferMbps * 100) / 100);
  return clean;
}

function hasTrackSelectionOverride(value: AssetConversionOverrideState) {
  return Array.isArray(value.keepVideoStreams) ||
    Array.isArray(value.keepAudioStreams) ||
    Array.isArray(value.keepSubtitleStreams);
}

function reconciliationJobIdFromReview(review: Asset['review']) {
  if (review.source !== 'sync-reconciliation') return 0;
  for (const tag of safeArray(review.tags)) {
    const match = tag.match(/^reconciliation-job-(\d+)$/);
    if (match) return Number(match[1]);
  }
  return 0;
}

function profileAACTrackEnabled(profile?: Profile) {
  if (!profile) return false;
  const config = profile.workerConfig ?? {};
  return typeof config.addAacStereoTrack === 'boolean'
    ? config.addAacStereoTrack
    : config.addAacStereoDefault === true;
}

function profileAACTrackDefault(profile?: Profile) {
  if (!profile) return false;
  const config = profile.workerConfig ?? {};
  return config.aacStereoDefault === true;
}

function assetHasConversionOverride(value?: AssetConversionOverrideState) {
  const clean = cleanConversionOverride(value ?? {});
  return Object.keys(clean).some((key) => key !== 'updatedAt');
}

function cleanStreamMetadataMap(value?: Record<string, StreamMetadataOverride>) {
  if (!value) {
    return undefined;
  }
  const clean: Record<string, StreamMetadataOverride> = {};
  Object.entries(value).forEach(([index, metadata]) => {
    const numericIndex = Number(index);
    if (!Number.isInteger(numericIndex) || numericIndex < 0) {
      return;
    }
    const item = cleanStreamMetadataOverride(metadata);
    if (!streamMetadataOverrideEmpty(item)) {
      clean[String(numericIndex)] = item;
    }
  });
  return Object.keys(clean).length ? clean : undefined;
}

function cleanStreamMetadataOverride(value: StreamMetadataOverride): StreamMetadataOverride {
  const clean: StreamMetadataOverride = {};
  const title = value.title?.trim();
  const language = value.language?.trim().toLowerCase();
  if (title) {
    clean.title = title;
  }
  if (language) {
    clean.language = language;
  }
  if (typeof value.default === 'boolean') {
    clean.default = value.default;
  }
  if (typeof value.forced === 'boolean') {
    clean.forced = value.forced;
  }
  return clean;
}

function streamMetadataOverrideEmpty(value: StreamMetadataOverride) {
  return !value.title && !value.language && value.default === undefined && value.forced === undefined;
}

function conversionStreamIndexes(value: AssetConversionOverrideState, scan: ScanResult, type: MediaStreamInfo['type']) {
  const allIndexes = streamIndexesForType(scan, type);
  const selected =
    type === 'video'
      ? value.keepVideoStreams
      : type === 'audio'
        ? value.keepAudioStreams
        : value.keepSubtitleStreams;
  return Array.isArray(selected) ? selected : allIndexes;
}

function streamIndexesForType(scan: ScanResult, type: MediaStreamInfo['type']) {
  const streams = type === 'video' ? scan.videoStreams : type === 'audio' ? scan.audioStreams : scan.subtitleStreams;
  return normalizeNumberList(safeArray(streams).map((stream) => stream.index));
}

function selectedOrUndefined(selected: number[], allIndexes: number[]) {
  const normalizedSelected = normalizeNumberList(selected);
  const normalizedAll = normalizeNumberList(allIndexes);
  if (normalizedSelected.length === normalizedAll.length && normalizedSelected.every((value, index) => value === normalizedAll[index])) {
    return undefined;
  }
  return normalizedSelected;
}

function normalizeNumberList(values?: number[] | null) {
  return Array.from(new Set(safeArray(values).filter((value) => Number.isInteger(value) && value >= 0))).sort((left, right) => left - right);
}

function numberOrUndefined(value: string) {
  if (value.trim() === '') {
    return undefined;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function optionItems(options: SelectOption[], currentValue?: string) {
  const value = currentValue ?? '';
  if (!value || options.some((option) => option.value === value)) {
    return options;
  }
  return [...options, { value, label: `Custom (advanced): ${value}` }];
}

function withoutMotionFilters(value?: string) {
  return (value ?? '').split(',').map((item) => item.trim()).filter((item) => item && !item.startsWith('bwdif') && !item.startsWith('fieldmatch') && item !== 'decimate').join(',');
}

function cropFilterFromChain(value?: string) {
  return (value ?? '').split(',').map((item) => item.trim()).find((item) => item.startsWith('crop=')) ?? '';
}

function withoutCropFilters(value?: string) {
  return (value ?? '').split(',').map((item) => item.trim()).filter((item) => item && !item.startsWith('crop=')).join(',');
}

function joinFilters(...values: Array<string | undefined>) {
  return values.map((value) => value?.trim()).filter(Boolean).join(',');
}

function friendlyCodec(value?: string) {
  switch ((value ?? '').toLowerCase()) {
    case 'copy':
      return 'Keep original';
    case 'x265_10bit':
    case 'libx265':
      return 'HEVC / x265';
    case 'x264':
    case 'libx264':
      return 'H.264 / x264';
    case 'aac':
      return 'AAC';
    case 'ac3':
      return 'AC-3';
    default:
      return value ?? '';
  }
}

function friendlyPreset(value?: string) {
  if (!value) {
    return '';
  }
  return value.replace(/-/g, ' ');
}

function friendlyPixFmt(value?: string) {
  switch (value) {
    case 'yuv420p10le':
      return '10-bit';
    case 'yuv420p':
      return '8-bit';
    default:
      return value ?? '';
  }
}

function getAudioProfiles(settings?: AppSetting[]) {
  const value = settings?.find((setting) => setting.key === 'audioEnhancementProfiles')?.value.profiles;
  if (!Array.isArray(value)) {
    return [];
  }

  return value
    .map((profile) => normalizeAudioProfile(profile))
    .filter((profile): profile is AudioEnhancementProfile => Boolean(profile))
    .filter((profile) => !profile.disabled && !profile.deletedAt);
}

function getDisabledConfidencePaths(settings?: AppSetting[]) {
  const value = settings?.find((setting) => setting.key === 'advisorAutomationPathOverrides')?.value.disabledPaths;
  if (!Array.isArray(value)) {
    return [];
  }

  return value.filter((path): path is string => typeof path === 'string');
}

function getAssetCategories(settings?: AppSetting[]) {
  const value = settings?.find((setting) => setting.key === 'assetCategories')?.value.categories;
  if (!Array.isArray(value)) {
    return ['movie', 'anime', 'series', 'episode', 'season', 'extras', 'special', 'concert', 'music-video', 'documentary'];
  }
  return normalizeStringList(value.filter((category): category is string => typeof category === 'string'));
}

function normalizeStringList(values: string[]) {
  const seen = new Set<string>();
  const normalized: string[] = [];
  values.forEach((value) => {
    const clean = value.trim();
    if (!clean) {
      return;
    }
    const key = clean.toLowerCase();
    if (seen.has(key)) {
      return;
    }
    seen.add(key);
    normalized.push(clean);
  });
  return normalized;
}

function normalizeAudioProfile(value: unknown): AudioEnhancementProfile | null {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const candidate = value as Record<string, unknown>;
  if (typeof candidate.key !== 'string' || typeof candidate.name !== 'string') {
    return null;
  }

  return {
    key: candidate.key,
    name: candidate.name,
    description: stringValue(candidate.description),
    intent: stringValue(candidate.intent),
    filters: stringValue(candidate.filters),
    rnnoiseModelPath: stringValue(candidate.rnnoiseModelPath),
    channelMode: channelModeValue(candidate.channelMode),
    forceStereoMode: forceStereoModeValue(candidate.forceStereoMode),
    stereoDelayMs: numberValue(candidate.stereoDelayMs, 12),
    stereoWidth: numberValue(candidate.stereoWidth, 20),
    eqBands: objectNumberMap(candidate.eqBands),
    preserveOriginalTrack: booleanValue(candidate.preserveOriginalTrack, true),
    outputCodec: stringValue(candidate.outputCodec, 'aac'),
    targetLoudness: numberValue(candidate.targetLoudness, -18),
    truePeak: numberValue(candidate.truePeak, -2),
    notes: stringValue(candidate.notes),
    disabled: booleanValue(candidate.disabled, false),
    deletedAt: stringValue(candidate.deletedAt),
  };
}

function objectNumberMap(value: unknown) {
  if (!value || typeof value !== 'object') {
    return {};
  }

  return Object.entries(value as Record<string, unknown>).reduce<Record<string, number>>((bands, [key, gain]) => {
    if (typeof gain === 'number' && Number.isFinite(gain)) {
      bands[key] = gain;
    }
    return bands;
  }, {});
}

function stringValue(value: unknown, fallback = '') {
  return typeof value === 'string' ? value : fallback;
}

function channelModeValue(value: unknown): AudioEnhancementProfile['channelMode'] {
  if (value === 'dual-mono' || value === 'force-stereo' || value === 'downmix-mono' || value === 'light-stereo') {
    return value;
  }
  return 'preserve';
}

function forceStereoModeValue(value: unknown): AudioEnhancementProfile['forceStereoMode'] {
  if (value === 'first-two' || value === 'duplicate-first') {
    return value;
  }
  return 'auto';
}

function numberValue(value: unknown, fallback: number) {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function booleanValue(value: unknown, fallback: boolean) {
  return typeof value === 'boolean' ? value : fallback;
}

function assetQualityProfile(profile: Profile | undefined, draft: AssetConversionOverrideState, encoder: string, preset: string): ProfileInput {
  const pixelFormat = draft.pixFmt || String(profile?.workerConfig?.pixFmt ?? (encoder === 'hevc_qsv' ? 'nv12' : 'yuv420p'));
  return {
    name: profile?.name ?? 'Asset quality recommendation',
    description: profile?.description ?? '',
    container: profile?.container ?? 'mkv',
    videoCodec: draft.videoCodec || profile?.videoCodec || 'x265',
    codecFamily: profile?.codecFamily ?? 'hevc',
    encoderPolicy: profile?.encoderPolicy ?? 'locked',
    preferredEncoder: encoder,
    allowedEncoders: [encoder],
    fallbackPolicy: profile?.fallbackPolicy ?? 'wait',
    bitDepth: pixelFormat.includes('10') || pixelFormat.includes('p010') ? 10 : 8,
    pixelFormat,
    qualityStrategy: profile?.qualityStrategy ?? 'hardware',
    audioCodec: draft.audioCodec || profile?.audioCodec || 'copy',
    qualityMode: draft.qualityMode || profile?.qualityMode || 'crf',
    qualityValue: draft.qualityValue ?? profile?.qualityValue ?? 20,
    preserveHdr: draft.preserveHdr ?? profile?.preserveHdr ?? true,
    preserveSubtitles: draft.preserveSubtitles ?? profile?.preserveSubtitles ?? true,
    preserveChapters: draft.preserveChapters ?? profile?.preserveChapters ?? true,
    disabled: false,
    workerConfig: {
      ...(profile?.workerConfig ?? {}),
      ...draft,
      qsvExtendedBRC: draft.qsvExtendedBrc ?? profile?.workerConfig?.qsvExtendedBRC,
      videoEncoder: encoder,
      useHardwareIfAvailable: true,
      preferredEncoder: 'hardware',
      pixFmt: pixelFormat,
      hardwareQualityPreset: preset,
    },
  };
}

function applyEffectiveProfileToAssetOverrides(onChange: <K extends keyof AssetConversionOverrideState>(key: K, value: AssetConversionOverrideState[K]) => void, profile: Profile) {
  const config = profile.workerConfig ?? {};
  const mapping: Array<[keyof AssetConversionOverrideState, unknown]> = [
    ['hardwareQualityPreset', config.hardwareQualityPreset], ['globalQuality', config.globalQuality],
    ['qsvRateControl', config.qsvRateControl], ['qsvLookAheadDepth', config.qsvLookAheadDepth],
    ['qsvExtendedBrc', config.qsvExtendedBRC], ['qsvAdaptiveI', config.qsvAdaptiveI], ['qsvAdaptiveB', config.qsvAdaptiveB],
    ['videoToolboxBitrateMbps', config.videoToolboxBitrateMbps], ['videoToolboxMaxrateMbps', config.videoToolboxMaxrateMbps],
    ['videoToolboxBufferMbps', config.videoToolboxBufferMbps], ['videoToolboxProfile', config.videoToolboxProfile],
    ['pixFmt', config.pixFmt],
  ];
  mapping.forEach(([key, value]) => {
    if (value !== undefined) onChange(key, value as never);
  });
}

function createBatchId(group: AssetGroup) {
  const random = Math.random().toString(36).slice(2, 8);
  return `batch-${group.libraryId}-${Date.now()}-${random}`;
}

function ConvertedMediaSummary({ technical }: { technical?: Asset['technical'] }) {
  const items = [
    {
      label: 'Video',
      value: technical
        ? [technical.videoCodec || 'Unknown', technical.width && technical.height ? `${technical.width}×${technical.height}` : '']
            .filter(Boolean)
            .join(' ')
        : 'Not scanned',
    },
    { label: 'Encoder', value: technical?.encoder || 'Unknown' },
    { label: 'Duration', value: technical ? formatMediaDuration(technical.duration) : '—' },
    { label: 'Bitrate', value: technical ? formatMediaBitrate(technical.bitrate) : '—' },
    { label: 'Range', value: technical ? technical.hdr ? 'HDR' : 'SDR' : '—' },
  ];

  return (
    <Box
      sx={{
        display: 'grid',
        gridTemplateColumns: '1.35fr 1fr 1fr 1fr 0.65fr',
        gap: 1.25,
        alignItems: 'start',
      }}
    >
      {items.map((item) => (
        <Stack key={item.label} spacing={0.15} sx={{ minWidth: 0 }}>
          <Typography variant="caption" color="text.secondary" fontWeight={700}>
            {item.label}
          </Typography>
          <Typography variant="body2" sx={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
            {item.value}
          </Typography>
        </Stack>
      ))}
    </Box>
  );
}

function formatMediaDuration(seconds: number) {
  if (!Number.isFinite(seconds) || seconds <= 0) return '—';
  const totalSeconds = Math.round(seconds);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const remainingSeconds = totalSeconds % 60;
  return [hours, minutes, remainingSeconds].map((value) => String(value).padStart(2, '0')).join(':');
}

function formatMediaBitrate(bitsPerSecond: number) {
  if (!Number.isFinite(bitsPerSecond) || bitsPerSecond <= 0) return '—';
  return `${(bitsPerSecond / 1_000_000).toFixed(2)} Mbps`;
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) {
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
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) {
    return 'Unknown';
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(date);
}
