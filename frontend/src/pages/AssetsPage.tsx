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
import SubtitlesIcon from '@mui/icons-material/Subtitles';
import DriveFileMoveIcon from '@mui/icons-material/DriveFileMove';
import EditIcon from '@mui/icons-material/Edit';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Component, useEffect, useState } from 'react';
import type { ErrorInfo, MouseEvent, ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { MediaSnapshotDetails } from '../components/MediaSnapshotDetails';
import { PageHeader } from '../components/PageHeader';
import { ProfileSuggestionCard } from '../components/ProfileSuggestionCard';
import type { AdvisorResponse, AppSetting, Asset, AssetConversionOverrideState, AssetGroup, AssetInventory, AudioEnhancementProfile, ExternalSubtitle, Library, MediaStreamInfo, Profile, ProfileSuggestion, QueueJob, ScanResult, StreamMetadataOverride } from '../api/types';
import { getTrackProfiles, trackProfileOverride, type TrackProfile } from '../trackProfiles';
import { qsvQualityHelper, qsvQualityRangeForCrf } from '../utils/qsv';

export function AssetsPage() {
  const [tab, setTab] = useState<'unprocessed' | 'library' | 'converted' | 'archive' | 'reports'>('unprocessed');
  const [assetQuery, setAssetQuery] = useState('');
  const assets = useQuery({ queryKey: ['assets'], queryFn: api.assets });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const jobs = useQuery({ queryKey: ['queueJobs'], queryFn: api.queueJobs });
  const queryClient = useQueryClient();
  const syncAssets = useMutation({
    mutationFn: api.syncAssets,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
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
      <Box sx={{ overflowX: 'auto', borderTop: 1, borderColor: 'divider' }}>
        <Table size="small" sx={{ minWidth: 980, tableLayout: 'fixed', '& td, & th': { py: 0.85 } }}>
          <TableHead>
            <TableRow>
              <TableCell sx={{ width: 390 }}>Asset group</TableCell>
              <TableCell sx={{ width: 130 }}>Library</TableCell>
              <TableCell sx={{ width: 140 }}>Status</TableCell>
              {showConfidence ? <TableCell sx={{ width: 130 }}>Confidence</TableCell> : null}
              <TableCell sx={{ width: 70 }}>Files</TableCell>
              <TableCell sx={{ width: 120 }}>Size</TableCell>
              <TableCell sx={{ width: 165 }}>Modified</TableCell>
              <TableCell padding="checkbox" sx={{ width: 52 }} />
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
  const isLibraryGroup = group.status === 'unverified' || group.status === 'library';
  const isArchiveGroup = mode === 'archive' || group.status === 'archive';
  const isReadOnlyGroup = isConvertedGroup || isArchiveGroup;
  const bulkSelectableAssets = isReadOnlyGroup ? groupAssets.filter((asset) => !asset.missing) : [];
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
	      await api.updateAssetConversion({ path: asset.path, ...asset.conversion, ...trackProfileOverride(trackProfile), processingMode: copyVideo ? 'audio_only' : 'full_encode', trackProfileKey: trackProfile.key });
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

  function runBulkAssetAction() {
    if (!selectedAssetPaths.length) return;
    const action = isArchiveGroup ? 'recover' : 'safely delete and restore';
    const confirmed = window.confirm(
      `${action === 'recover' ? 'Recover' : 'Safely delete'} ${selectedAssetPaths.length} selected asset(s) from this path? ${isArchiveGroup ? 'Converted files will not be deleted.' : 'Each converted file will be deleted only if its archived original can be restored to Raw.'}`,
    );
    if (confirmed) bulkAssetAction.mutate(selectedAssetPaths);
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
        <TableCell>{group.libraryName}</TableCell>
        <TableCell>
          <Stack spacing={0.75} alignItems="flex-start">
            <Chip label={statusLabel(group.status)} color={statusColor(group.status)} size="small" />
            {groupReview.requiresReview ? <Chip label="Some need review" color="error" size="small" /> : null}
          </Stack>
        </TableCell>
        {!isReadOnlyGroup ? (
          <TableCell>
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
          </TableCell>
        ) : null}
        <TableCell>{group.fileCount}</TableCell>
        <TableCell>{formatBytes(group.sizeBytes)}</TableCell>
        <TableCell>{formatDate(group.modifiedAt)}</TableCell>
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
        <TableCell colSpan={isReadOnlyGroup ? 7 : 8} sx={{ p: 0, borderBottom: expanded ? 1 : 0, borderColor: 'divider', maxWidth: 0 }}>
          <Collapse in={expanded} timeout="auto" unmountOnExit>
            <Box sx={{ bgcolor: 'rgba(255,255,255,0.02)', px: { xs: 1.5, md: 2 }, py: 2, width: '100%', maxWidth: '100%', overflow: 'hidden' }}>
              <Stack spacing={2}>
                {isArchiveGroup ? null : isConvertedGroup ? (
                  <Alert severity="info">
                    Converted assets can be inspected, moved to another library, or safely deleted and restored. Re-processing should start from Original Archive.
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
                {isLibraryGroup ? (
                  <Stack direction="row" justifyContent="flex-end">
                    {migrationControls}
                  </Stack>
                ) : null}
                {isReadOnlyGroup ? (
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
                        onClick={runBulkAssetAction}
                        sx={{ minHeight: 40, whiteSpace: 'nowrap' }}
                      >
                        {bulkAssetAction.isPending ? 'Processing...' : isArchiveGroup ? 'Recover selected' : 'Delete selected'}
                      </Button>
                    </Stack>
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
                  <Table size="small" sx={{ minWidth: isReadOnlyGroup ? 820 : 1320, tableLayout: 'fixed' }}>
                    <TableHead>
                      <TableRow>
                        {isReadOnlyGroup ? (
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
                        <TableCell sx={{ width: 100 }}>Size</TableCell>
                        <TableCell sx={{ width: 128 }}>Modified</TableCell>
                        <TableCell sx={{ width: 145 }}>Category</TableCell>
                        {!isReadOnlyGroup ? <TableCell sx={{ width: 165 }}>Video profile</TableCell> : null}
                        {!isReadOnlyGroup ? <TableCell sx={{ width: 165 }}>Audio profile</TableCell> : null}
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
                          mode={mode}
                          bulkSelected={selectedAssetPaths.includes(asset.path)}
                          bulkSelectionEnabled={isReadOnlyGroup}
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
  mode: 'unprocessed' | 'library' | 'converted' | 'archive';
  bulkSelected: boolean;
  bulkSelectionEnabled: boolean;
  bulkSelectionDisabled: boolean;
  onBulkSelectionChange: (path: string, selected: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const [selectedProfileId, setSelectedProfileId] = useState<number>(groupProfileId);
  const [selectedAudioProfileKey, setSelectedAudioProfileKey] = useState<string>(groupAudioProfileKey);
  const [selectedLibraryId, setSelectedLibraryId] = useState<number>(groupLibraryId);
  const [showSnapshotDialog, setShowSnapshotDialog] = useState(false);
  const [snapshotTab, setSnapshotTab] = useState(0);
  const [editingSubtitle, setEditingSubtitle] = useState<ExternalSubtitle | null>(null);
  const [subtitleContent, setSubtitleContent] = useState('');
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
    mutationFn: api.scan,
    onSuccess: (scan) => {
      if (asset.status !== 'converted' && asset.status !== 'archive') {
        profileSuggestion.mutate(scan.path);
      }
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
    enabled: mode !== 'archive' && mode !== 'converted' && asset.status !== 'archive' && asset.status !== 'converted' && confidenceEnabled && selectedProfileId > 0,
  });
  const createJob = useMutation({
    mutationFn: async (input: Parameters<typeof api.createQueueJob>[0]) => {
      if (pathTrackProfile) {
        const result = validateTrackProfile(pathTrackProfile, await api.scan({ path: asset.path, force: false }));
        if (!result.applies && pathTrackProfile.validationMode === 'review' && !assetReviewApproved(asset)) {
          await api.updateAssetReview({ path: asset.path, requiresReview: true, source: 'track-profile', reason: result.reasons.join('; '), tags: ['track-profile-incompatible'] });
          throw new Error(`Track profile does not apply; asset marked for review: ${result.reasons.join('; ')}`);
        }
        if (!result.applies && pathTrackProfile.validationMode === 'block') {
          throw new Error(`Track profile blocked this asset: ${result.reasons.join('; ')}`);
        }
        await api.updateAssetConversion({ path: asset.path, ...conversionDraft, ...trackProfileOverride(pathTrackProfile), processingMode: selectedProfileId < 0 ? 'audio_only' : 'full_encode', trackProfileKey: pathTrackProfile.key });
        if (!result.applies) {
          input = { ...input, notes: `${input.notes ?? ''}\nTrack profile ${pathTrackProfile.key} did not apply: ${result.reasons.join('; ')}`.trim() };
        }
      } else if (hasTrackSelectionOverride(conversionDraft)) {
        await api.updateAssetConversion({
          path: asset.path,
          ...cleanConversionOverride(conversionDraft),
          processingMode: selectedProfileId < 0 ? 'audio_only' : 'full_encode',
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
  const extractSubtitles = useMutation({
    mutationFn: api.extractAssetSubtitles,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['externalSubtitles', asset.path] });
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
  const isLibraryReplacement = asset.status === 'unverified' || asset.status === 'library';
  const isArchive = mode === 'archive' || asset.status === 'archive';
  const rowColumnCount = bulkSelectionEnabled ? 7 : 10;
  const associatedJob = associatedJobForAsset(asset, queueJobs);
  const rowLocked = hasOpenJob || createJob.isPending || (isConverted && !isLibraryReplacement) || isArchive;
  const pipelineState = assetPipelineState(asset, associatedJob, createJob.isPending);
  const canQueueWithSelection = selectedProfileId > 0 || Boolean(selectedAudioProfileKey) || Boolean(pathTrackProfile) || hasTrackSelectionOverride(conversionDraft);

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
      ? { deinterlaceMode: 'off', videoFilters: withoutMotion }
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
      processingMode: stringFromRecord(workerConfig, 'processingMode'),
      preserveHdr: proposed.preserveHdr,
      preserveSubtitles: proposed.preserveSubtitles,
      preserveChapters: proposed.preserveChapters,
      addAacStereoTrack: typeof workerConfig.addAacStereoTrack === 'boolean' ? workerConfig.addAacStereoTrack : conversionDraft.addAacStereoTrack,
      aacStereoDefault: typeof workerConfig.addAacStereoDefault === 'boolean' ? workerConfig.addAacStereoDefault : conversionDraft.aacStereoDefault,
      useHardwareIfAvailable: hardwareEnabled,
      videoEncoder: stringFromRecord(workerConfig, 'videoEncoder') || (hardwareEnabled ? 'auto' : 'libx265'),
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
    setSnapshotTab(2);
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
    setConversionDraft((current) => ({ ...current, [key]: value }));
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
      trackProfileKey: pathTrackProfile?.key ?? '',
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
        {!isArchive && !isConverted ? (
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
        <TableCell>{formatBytes(asset.sizeBytes)}</TableCell>
        <TableCell>{formatDate(asset.modifiedAt)}</TableCell>
        <TableCell sx={{ minWidth: 180 }}>
          <AssetCategorySelect value={category} options={assetCategories} onChange={saveAssetCategory} label="Category" size="small" disabled={rowLocked} />
        </TableCell>
        {!isArchive && !isConverted ? (
          <>
            <TableCell sx={{ minWidth: 220 }}>
              <ProfileAutocomplete profiles={profiles} value={selectedProfileId} onChange={setSelectedProfileId} label="Video" size="small" disabled={rowLocked} allowNone />
            </TableCell>
            <TableCell sx={{ minWidth: 220 }}>
              <AudioProfileAutocomplete profiles={audioProfiles} value={selectedAudioProfileKey} onChange={setSelectedAudioProfileKey} label="Audio" size="small" disabled={rowLocked} />
            </TableCell>
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
            <Tooltip title="Scan snapshot">
              <IconButton color="primary" onClick={openSnapshotDialog} aria-label={`Scan ${asset.fileName}`} sx={actionIconSx}>
                <ManageSearchIcon />
              </IconButton>
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
            {isLibraryReplacement ? (
              <>
                <Tooltip title={extractSubtitles.isPending ? 'Generating subtitle files' : 'Generate external SRT/ASS files from this Library asset'}>
                  <span onClick={(event) => event.stopPropagation()}>
                    <IconButton
                      color="primary"
                      onClick={() => extractSubtitles.mutate(asset.path)}
                      disabled={asset.missing || extractSubtitles.isPending}
                      aria-label={`Generate subtitles for ${asset.fileName}`}
                      sx={actionIconSx}
                    >
                      <SubtitlesIcon />
                    </IconButton>
                  </span>
                </Tooltip>
              </>
            ) : null}
            {isConverted ? (
              <Tooltip title={asset.missing ? 'Complete safe restoration from the archived original' : deleteConvertedAsset.isPending ? 'Safe deletion is in progress' : 'Safely delete converted asset and restore archived original to Raw'}>
                <span onClick={(event) => event.stopPropagation()}>
                  <IconButton color="error" onClick={safelyDeleteConvertedAsset} disabled={deleteConvertedAsset.isPending} aria-label={`Safely delete ${asset.fileName}`} sx={actionIconSx}>
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
            {isLibraryReplacement ? (
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
      {extractSubtitles.isSuccess ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ maxWidth: 0 }}>
            <Alert severity="success" sx={{ overflowWrap: 'anywhere' }}>
              Generated {extractSubtitles.data.created.length} subtitle file(s).
              {extractSubtitles.data.existing.length > 0 ? ` ${extractSubtitles.data.existing.length} already existed and were preserved.` : ''}
              {extractSubtitles.data.unsupported.length > 0 ? ` ${extractSubtitles.data.unsupported.length} bitmap track(s) require OCR.` : ''}
            </Alert>
          </TableCell>
        </TableRow>
      ) : null}
      {extractSubtitles.isError ? (
        <TableRow>
          <TableCell colSpan={rowColumnCount} sx={{ maxWidth: 0 }}><Alert severity="warning" sx={{ overflowWrap: 'anywhere' }}>Subtitle generation failed: {extractSubtitles.error instanceof Error ? extractSubtitles.error.message : 'unknown error'}</Alert></TableCell>
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
            {snapshot.isPending ? <Alert severity="info">Reading asset snapshot...</Alert> : null}
            {snapshot.isError ? (
              <Alert severity="warning">Could not scan this asset: {snapshot.error instanceof Error ? snapshot.error.message : 'unknown backend error'}</Alert>
            ) : null}
            {snapshot.data ? (
              <>
                <Tabs value={snapshotTab} onChange={(_, value: number) => setSnapshotTab(value)} variant="scrollable" allowScrollButtonsMobile>
                  <Tab label="General & Tracks" />
                  <Tab label="Suggested profile" />
                  <Tab label="Asset Overrides" />
                </Tabs>
                <Box hidden={snapshotTab !== 0}>
                  <Stack spacing={2} sx={{ pt: 1.5 }}>
                    <MediaSnapshotDetails scan={snapshot.data} section="general" />
                    <Divider />
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
                        pending={extractSubtitles.isPending}
                        onGenerate={(streamIndex, format, ocrLanguage) => extractSubtitles.mutate({ path: asset.path, streamIndex, format, ocrLanguage })}
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
                    {extractSubtitles.isSuccess ? (
                      <Alert severity="success">
                        Generated {extractSubtitles.data.created.length} subtitle file(s).
                        {extractSubtitles.data.existing.length ? ` ${extractSubtitles.data.existing.length} already existed.` : ''}
                      </Alert>
                    ) : null}
                    {extractSubtitles.isError ? <Alert severity="warning">{extractSubtitles.error instanceof Error ? extractSubtitles.error.message : 'Subtitle generation failed.'}</Alert> : null}
                    {deleteExternalSubtitle.isError ? <Alert severity="warning">{deleteExternalSubtitle.error instanceof Error ? deleteExternalSubtitle.error.message : 'Subtitle deletion failed.'}</Alert> : null}
                  </Stack>
                </Box>
                <Box hidden={snapshotTab !== 1}>
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
                      />
                    ) : null}
                    {isConverted ? <Alert severity="info">This is already a converted asset. Re-processing recommendations should be evaluated from its archived original.</Alert> : null}
                    {isArchive ? <Alert severity="info">Recover the archived original before applying a suggested profile.</Alert> : null}
                  </Stack>
                </Box>
                <Box hidden={snapshotTab !== 2}>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    {!isArchive ? (
                      <AssetConversionOverridePanel
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
            {isConverted ? (
              <FinalDetailsSummary asset={asset} job={associatedJob} compact />
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
  pending,
  onGenerate,
}: {
  streams: MediaStreamInfo[];
  pending: boolean;
  onGenerate: (streamIndex: number, format: 'srt' | 'ass', ocrLanguage?: string) => void;
}) {
  const [ocrLanguages, setOcrLanguages] = useState<Record<number, string>>({});
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
          return (
            <Stack
              key={stream.index}
              direction={{ xs: 'column', sm: 'row' }}
              alignItems={{ xs: 'stretch', sm: 'center' }}
              justifyContent="space-between"
              spacing={1}
              sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1 }}
            >
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
                  </TextField>
                ) : null}
                <Button size="small" variant="outlined" disabled={pending} onClick={() => onGenerate(stream.index, 'srt', bitmap ? (ocrLanguages[stream.index] || defaultOCRLanguage(stream.language)) : undefined)}>Generate SRT</Button>
                <Button size="small" variant="outlined" disabled={pending} onClick={() => onGenerate(stream.index, 'ass', bitmap ? (ocrLanguages[stream.index] || defaultOCRLanguage(stream.language)) : undefined)}>Generate ASS</Button>
              </Stack>
            </Stack>
          );
        })}
      </Stack>
    </Box>
  );
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
  { value: 'force', label: 'Force deinterlace' },
  { value: 'ivtc_tff', label: 'Inverse telecine · TFF' },
  { value: 'ivtc_bff', label: 'Inverse telecine · BFF' },
];

const assetEncoderOptions: SelectOption[] = [
  { value: 'auto', label: 'Auto' },
  { value: 'libx265', label: 'Software x265' },
  { value: 'hevc_qsv', label: 'Intel Quick Sync' },
  { value: 'hevc_nvenc', label: 'NVIDIA NVENC' },
  { value: 'hevc_videotoolbox', label: 'Apple VideoToolbox' },
  { value: 'hevc_amf', label: 'AMD AMF' },
];

function AssetConversionOverridePanel({
  draft,
  profile,
  scan,
  onChange,
  onSave,
  onReset,
  saving,
  readOnly = false,
}: {
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
  const recommendedCrop = scan?.cropAnalysis?.status === 'detected' ? (scan.cropAnalysis.recommendedCrop ?? '').trim() : '';
  const currentCropFilter = cropFilterFromChain(draft.videoFilters);
  const suggestedCropFilter = recommendedCrop ? `crop=${recommendedCrop}` : '';
  const suggestedCropEnabled = Boolean(suggestedCropFilter) && currentCropFilter === suggestedCropFilter;
  const bitmapSubtitleCount = scan?.subtitleStreams.filter((stream) => isBitmapSubtitleCodec(stream.codec)).length ?? 0;
  const hardwareEnabled = draft.useHardwareIfAvailable ?? profile?.workerConfig?.useHardwareIfAvailable === true;
  const effectiveVideoEncoder = draft.videoEncoder || stringFromRecord(profile?.workerConfig ?? {}, 'videoEncoder') || 'auto';
  const qsvSelected = hardwareEnabled && effectiveVideoEncoder === 'hevc_qsv';

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
              onChange={(event) => onChange('videoCodec', event.target.value)}
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
                label="Enhanced audio source"
                value={draft.enhancedAudioSourceStreamIndex ?? ''}
                onChange={(event) => onChange('enhancedAudioSourceStreamIndex', event.target.value === '' ? undefined : Number(event.target.value))}
                size="small"
                fullWidth
              >
                <MenuItem value="">Default audio track</MenuItem>
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
              label="Audio codec"
              value={draft.audioCodec ?? ''}
              onChange={(event) => onChange('audioCodec', event.target.value)}
              size="small"
              fullWidth
            >
              {optionItems(audioCodecOptions, draft.audioCodec).map((option) => (
                <MenuItem key={option.value} value={option.value}>
                  {option.label}
                </MenuItem>
              ))}
            </TextField>
          </Grid>
          <Grid size={{ xs: 12, md: 4 }}>
            <TextField
              select
              label="Work mode"
              value={draft.processingMode ?? ''}
              onChange={(event) => onChange('processingMode', event.target.value)}
              size="small"
              fullWidth
            >
              <MenuItem value="">Profile default</MenuItem>
              <MenuItem value="full_encode">Re-encode video</MenuItem>
              <MenuItem value="audio_only">Audio/subtitle fixes only</MenuItem>
            </TextField>
          </Grid>
          <Grid size={{ xs: 12 }}>
            <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, px: 2, pt: 1.25, pb: 1 }}>
            <Stack spacing={0.5}>
              <Stack direction="row" justifyContent="space-between" alignItems="center">
                <Typography fontWeight={700}>Software quality · CRF {draft.qualityValue ?? profile?.qualityValue ?? 22}</Typography>
                {draft.qualityValue ? <Button size="small" onClick={() => onChange('qualityValue', undefined)}>Use profile</Button> : <Chip label="Profile default" size="small" />}
              </Stack>
              <Slider value={draft.qualityValue ?? profile?.qualityValue ?? 22} min={14} max={30} step={1} onChange={(_, value) => onChange('qualityValue', Array.isArray(value) ? value[0] : value)} valueLabelDisplay="auto" size="small" sx={{ mx: 0.5, width: 'calc(100% - 8px)' }} />
              <Stack direction="row" justifyContent="space-between">
                <Typography variant="caption" color="text.secondary">14 · higher quality</Typography>
                <Typography variant="caption" color="text.secondary">30 · smaller file</Typography>
              </Stack>
            </Stack>
            </Box>
          </Grid>
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
          <Grid size={{ xs: 12, md: 4 }}>
            <TextField
              select
              label="Color depth"
              value={draft.pixFmt ?? ''}
              onChange={(event) => onChange('pixFmt', event.target.value)}
              size="small"
              fullWidth
            >
              {optionItems(colorDepthOptions, draft.pixFmt).map((option) => (
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
                  {recommendedCrop ? <Chip size="small" color={suggestedCropEnabled ? 'primary' : 'default'} label={`crop=${recommendedCrop}`} /> : <Chip size="small" label="No stable crop suggested" />}
                </Stack>
                {scan?.cropAnalysis?.reason ? <Typography variant="body2" color="text.secondary">{scan.cropAnalysis.reason}</Typography> : null}
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
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={draft.useHardwareIfAvailable ?? profile?.workerConfig?.useHardwareIfAvailable === true}
                        onChange={(event) => {
                          onChange('useHardwareIfAvailable', event.target.checked);
                          if (!event.target.checked) onChange('videoEncoder', 'libx265');
                        }}
                      />
                    }
                    label="Use hardware if available"
                  />
                </Grid>
                <Grid size={{ xs: 12, md: 4 }}>
                  <TextField
                    select
                    label="Encoder"
                    value={draft.videoEncoder || stringFromRecord(profile?.workerConfig ?? {}, 'videoEncoder') || 'auto'}
                    onChange={(event) => onChange('videoEncoder', event.target.value)}
                    disabled={!(draft.useHardwareIfAvailable ?? profile?.workerConfig?.useHardwareIfAvailable === true)}
                    helperText="Hardware encoders are enabled only when hardware use is allowed."
                    size="small"
                    fullWidth
                  >
                    {assetEncoderOptions.map((option) => (
                      <MenuItem
                        key={option.value}
                        value={option.value}
                        disabled={isHardwareAssetEncoder(option.value) && !(draft.useHardwareIfAvailable ?? profile?.workerConfig?.useHardwareIfAvailable === true)}
                      >
                        {option.label}
                      </MenuItem>
                    ))}
                  </TextField>
                </Grid>
                <Grid size={{ xs: 12, md: 4 }}>
                  <TextField
                    label="Hardware quality"
                    type="number"
                    value={draft.globalQuality ?? Number(profile?.workerConfig?.globalQuality ?? qsvQualityRangeForCrf(draft.qualityValue ?? profile?.qualityValue ?? 22).recommended)}
                    onChange={(event) => onChange('globalQuality', Number(event.target.value))}
                    disabled={!(draft.useHardwareIfAvailable ?? profile?.workerConfig?.useHardwareIfAvailable === true)}
                    inputProps={{ min: 15, max: 35 }}
                    helperText={qsvQualityHelper(draft.qualityValue ?? profile?.qualityValue ?? 22)}
                    size="small"
                    fullWidth
                  />
                </Grid>
              </Grid>
              {qsvSelected ? (
                <Grid container spacing={1.5} sx={{ mt: 1 }}>
                  <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                    <TextField select label="QSV rate control" value={draft.qsvRateControl || stringFromRecord(profile?.workerConfig ?? {}, 'qsvRateControl') || 'icq'} onChange={(event) => onChange('qsvRateControl', event.target.value as 'icq' | 'la_icq')} size="small" fullWidth>
                      <MenuItem value="icq">ICQ</MenuItem>
                      <MenuItem value="la_icq">LA-ICQ</MenuItem>
                    </TextField>
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                    <TextField label="Look-ahead depth" type="number" value={draft.qsvLookAheadDepth ?? Number(profile?.workerConfig?.qsvLookAheadDepth ?? 40)} onChange={(event) => onChange('qsvLookAheadDepth', Number(event.target.value))} inputProps={{ min: 10, max: 100 }} size="small" fullWidth />
                  </Grid>
                  <Grid size={{ xs: 12, md: 6 }}>
                    <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                      <FormControlLabel control={<Checkbox checked={draft.qsvExtendedBrc ?? profile?.workerConfig?.qsvExtendedBRC === true} onChange={(event) => onChange('qsvExtendedBrc', event.target.checked)} />} label="Extended BRC" />
                      <FormControlLabel control={<Checkbox checked={draft.qsvAdaptiveI ?? profile?.workerConfig?.qsvAdaptiveI === true} onChange={(event) => onChange('qsvAdaptiveI', event.target.checked)} />} label="Adaptive I" />
                      <FormControlLabel control={<Checkbox checked={draft.qsvAdaptiveB ?? profile?.workerConfig?.qsvAdaptiveB === true} onChange={(event) => onChange('qsvAdaptiveB', event.target.checked)} />} label="Adaptive B" />
                    </Stack>
                  </Grid>
                  <Grid size={{ xs: 12 }}>
                    <Typography variant="caption" color="text.secondary">
                      QSV low-power mode remains disabled for NAS compatibility. Unsupported features are omitted after the runtime smoke test.
                    </Typography>
                  </Grid>
                </Grid>
              ) : null}
            </Box>
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

function FinalDetailsSummary({ asset, job, compact = false }: { asset: Asset; job?: QueueJob; compact?: boolean }) {
  const report = job?.validationReport ?? {};
  const mode = stringFromRecord(report, 'processingMode') || modeFromNotes(job?.notes ?? '');
  const audioProfile = job?.audioProfileKey || audioProfileFromNotes(job?.notes ?? '');
  const status = jobPublishesAsset(asset, job) ? 'Published' : job?.publicationRetiredAt ? 'Publication retired' : job?.status === 'completed' ? 'Converted' : job?.status || 'Converted';

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
    ...(allowNone ? [{ id: -1, name: 'None', videoCodec: 'No video profile', search: 'none no profile' }] : []),
    ...profiles.map((profile) => ({ id: profile.id, name: profile.name, videoCodec: profile.videoCodec, search: `${profile.name} ${profile.description} ${profile.container} ${profile.videoCodec} ${profile.audioCodec}` })),
  ];
  return (
    <Autocomplete
      options={options}
      value={options.find((profile) => profile.id === value) ?? null}
      onChange={(_, profile) => onChange(profile?.id ?? 0)}
      getOptionLabel={(profile) => profile.id === -1 ? 'None' : `${profile.name} · ${profile.videoCodec}`}
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
    key: '', name: 'None', description: '', intent: '', filters: '', rnnoiseModelPath: '', channelMode: 'preserve',
    forceStereoMode: 'auto', stereoDelayMs: 0, stereoWidth: 0, eqBands: {}, preserveOriginalTrack: true,
    outputCodec: 'copy', targetLoudness: 0, truePeak: 0, notes: '',
  };
  const options = [none, ...profiles];
  return (
    <Autocomplete
      options={options}
      value={options.find((profile) => profile.key === value) ?? none}
      onChange={(_, profile) => onChange(profile?.key ?? '')}
      getOptionLabel={(profile) => profile.key ? `${profile.name} · ${profile.outputCodec || 'copy'}` : 'None'}
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

function TrackProfileAutocomplete({ profiles, value, onChange, disabled }: { profiles: TrackProfile[]; value: string; onChange: (key: string) => void; disabled?: boolean }) {
  const none: TrackProfile = {
    key: '', name: 'None', description: '', videoMode: 'first', audioMode: 'all', audioLanguages: [], audioRequired: false,
    dropCommentary: false, defaultAudioLanguage: '', subtitleMode: 'all', subtitleLanguages: [], subtitlesRequired: false,
    defaultSubtitleLanguage: '', validationMode: 'warn', notes: '',
  };
  const options = [none, ...profiles];
  return <Autocomplete options={options} value={options.find((profile) => profile.key === value) ?? none} onChange={(_, profile) => onChange(profile?.key ?? '')} getOptionLabel={(profile) => profile.name} isOptionEqualToValue={(option, selected) => option.key === selected.key} disabled={disabled} renderInput={(params) => <TextField {...params} label="Tracks for path" size="small" />} fullWidth />;
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
  const basePath = asset.status === 'converted' || asset.status === 'unverified' || asset.status === 'library' ? library?.destinationPath : library?.sourcePath;

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
  if (asset.status === 'unverified' || asset.status === 'library') {
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
  return ['hevc_qsv', 'hevc_nvenc', 'hevc_videotoolbox', 'hevc_amf'].includes(value);
}

function stringFromRecord(record: Record<string, unknown>, key: string) {
  const value = record[key];
  return typeof value === 'string' ? value : '';
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
  return cleanConversionOverride({
    ...value,
    keepVideoStreams: Array.isArray(value.keepVideoStreams) ? normalizeNumberList(value.keepVideoStreams) : undefined,
    keepAudioStreams: Array.isArray(value.keepAudioStreams) ? normalizeNumberList(value.keepAudioStreams) : undefined,
    keepSubtitleStreams: Array.isArray(value.keepSubtitleStreams) ? normalizeNumberList(value.keepSubtitleStreams) : undefined,
  });
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
    'processingMode',
    'videoEncoder',
  ] as const).forEach((key) => {
    const text = value[key]?.trim();
    if (text) {
      clean[key] = text;
    }
  });
  if (value.deinterlaceMode === 'auto' || value.deinterlaceMode === 'off' || value.deinterlaceMode === 'force' || value.deinterlaceMode === 'ivtc_tff' || value.deinterlaceMode === 'ivtc_bff') {
    clean.deinterlaceMode = value.deinterlaceMode;
  }
  if (value.qsvRateControl === 'icq' || value.qsvRateControl === 'la_icq') {
    clean.qsvRateControl = value.qsvRateControl;
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

function createBatchId(group: AssetGroup) {
  const random = Math.random().toString(36).slice(2, 8);
  return `batch-${group.libraryId}-${Date.now()}-${random}`;
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
