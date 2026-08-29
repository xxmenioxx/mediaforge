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
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  Grid,
  IconButton,
  InputAdornment,
  LinearProgress,
  MenuItem,
  Snackbar,
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
import DeleteSweepIcon from '@mui/icons-material/DeleteSweep';
import DriveFileMoveIcon from '@mui/icons-material/DriveFileMove';
import DriveFileRenameOutlineIcon from '@mui/icons-material/DriveFileRenameOutline';
import EditIcon from '@mui/icons-material/Edit';
import ScienceIcon from '@mui/icons-material/Science';
import { useIsMutating, useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import { Component, Fragment, useEffect, useRef, useState } from 'react';
import type { ErrorInfo, MouseEvent, ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { MediaSnapshotDetails } from '../components/MediaSnapshotDetails';
import { TrackMaintenancePanel } from '../components/TrackMaintenancePanel';
import { TestEncodeDialog } from '../components/TestEncodeDialog';
import { FrameStructureControls } from '../components/FrameStructureControls';
import { FrameCadenceControls } from '../components/FrameCadenceControls';
import { semanticMotionModes } from '../utils/motionModes';
import { HEVCLevelControls } from '../components/HEVCLevelControls';
import { PageHeader } from '../components/PageHeader';
import { ProfileSuggestionCard } from '../components/ProfileSuggestionCard';
import type { AdvisorFinding, AdvisorResponse, AppSetting, Asset, AssetConversionOverrideState, AssetGroup, AssetInventory, AssetLogicalGroup, AssetSourceGroup, AudioEnhancementProfile, ExternalSubtitle, Library, MediaStreamInfo, Profile, ProfileAssignment, ProfileInput, QueueJob, QueueJobInput, QueueSelectedAssetsResponse, QualityRecommendationResponse, ScanResult, SnapshotOperation, StreamMetadataOverride } from '../api/types';
import { getTrackProfiles, type TrackProfile } from '../trackProfiles';
import { qsvQualityHelper, qsvQualityRangeForCrf } from '../utils/qsv';
import { applyHardwareQualityPreset as applySharedHardwareQualityPreset, hardwareQualityPresetOptions, qsvAssetQualitySummary } from '../utils/hardwareQualityPresets';
import { qsvPStrategySupported, qsvSelectionWarnings, resolveQSVFeatures } from '../utils/qsvCapabilities';
import { videoToolboxRatesFromTargetMbps } from '../utils/videoToolboxRates';
import { encoderNamesForWorker, selectedWorker as resolveSelectedWorker } from '../utils/workerEncoders';
import { assetOverridePreferenceDraft, getMVForgePreferences } from '../mvforgePreferences';
import { assetDerivedGopRecommendation, reliableFrameRateForScan } from '../utils/frameStructureRecommendation';
import { formatEstimatedByteRange } from '../utils/qualityEstimate';
import { normalizeLegacyVideoCodec } from '../utils/videoCodec';
import { withStreamSelection } from '../utils/assetTrackSelection';
import { testEncodeEligibleAsset } from '../utils/testEncodeEligibility';
import { hierarchicalSelectionState, selectableAssetsForPaths, selectedSelectableAssetsForLogicalGroups } from '../utils/unprocessedAssetSelection';
import {
  mergedScopeConfigurationInput,
  profileAssignmentForScopeChange,
  scopeConfigurationEditorValues,
  type ScopeConfigurationField,
} from '../utils/scopeConfigurationEditor';

const VIDEO_PROFILE_OVERRIDE_ONLY = -1;
const VIDEO_PROFILE_AUDIO_ONLY = -2;

export function AssetsPage() {
  const [tab, setTab] = useState<'unprocessed' | 'library' | 'converted' | 'archive' | 'reports'>('unprocessed');
  const [libraryDecision, setLibraryDecision] = useState<'' | 'unverified' | 'accepted'>('');
  const [assetQuery, setAssetQuery] = useState('');
  const [mediaArea, setMediaArea] = useState('');
	const [sourceGroupId, setSourceGroupId] = useState(0);
  const jobs = useQuery({
    queryKey: ['queueJobs'],
    queryFn: api.queueJobs,
    refetchInterval: (query) => query.state.data?.some((job) => job.status === 'queued' || job.status === 'running') ? 3000 : false,
  });
  const assets = useQuery({
    queryKey: ['assets'],
    queryFn: api.assets,
    refetchInterval: jobs.data?.some((job) => job.status === 'queued' || job.status === 'running') ? 3000 : false,
  });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const snapshotOperations = useQuery({
    queryKey: ['snapshotOperations'],
    queryFn: () => api.snapshotOperations(''),
    refetchInterval: (query) => query.state.data?.operations.some((operation) => operation.status === 'running') ? 1500 : false,
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
  const acceptedCount = safeArray(assets.data?.accepted).length;
  const archiveCount = safeArray(assets.data?.archive).length;
  const sourceGroups = safeArray(assets.data?.sourceGroups);
  const statusGroups = safeArray(
    tab === 'archive' ? assets.data?.archiveGroups : tab === 'library' ? assets.data?.libraryGroups : tab === 'converted' ? assets.data?.convertedGroups : assets.data?.unprocessedGroups,
  );
	const currentGroups = statusGroups;
	const selectedSourceGroup = sourceGroups.find((group) => group.id === sourceGroupId) ?? sourceGroups[0];
  const allInventoryGroups = [
    ...safeArray(assets.data?.unprocessedGroups), ...safeArray(assets.data?.libraryGroups),
    ...safeArray(assets.data?.convertedGroups), ...safeArray(assets.data?.archiveGroups),
  ];
  const mediaAreas = [...new Set(allInventoryGroups.map((group) => mediaAreaForGroup(group, settings.data)).filter(Boolean))].sort();
  const decisionGroups = tab === 'library' && libraryDecision
    ? currentGroups.filter((group) => group.status === libraryDecision)
    : currentGroups;
  const areaGroups = mediaArea ? decisionGroups.filter((group) => mediaAreaForGroup(group, settings.data) === mediaArea) : decisionGroups;
  const filteredGroups = filterAssetGroups(areaGroups, assetQuery);
  const filteredArchiveAssets = tab === 'archive' ? archiveAssetsFromGroups(areaGroups, assetQuery) : [];
  const runningSnapshotPaths = new Set(
    (snapshotOperations.data?.operations ?? [])
      .filter((operation) => operation.status === 'running')
      .map((operation) => operation.assetPath),
  );

  return (
    <>
      <PageHeader title="Assets" eyebrow="Media inventory">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Library assets come from destination paths. Files without publication provenance remain Unverified until they are queued or explicitly Accepted as-is.
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
                  setLibraryDecision('');
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
                  <Button startIcon={<RefreshIcon />} variant="outlined" onClick={() => syncAssets.mutate()} disabled={syncAssets.isPending} sx={{ minHeight: 40 }}>
                    Sync
                  </Button>
                  {tab !== 'reports' ? (
                    tab === 'library' ? (
                      <TextField
                        select
                        value={libraryDecision}
                        onChange={(event) => setLibraryDecision(event.target.value as '' | 'unverified' | 'accepted')}
                        label="Decision"
                        size="small"
                        sx={{ width: { xs: '100%', sm: 180 } }}
                      >
                        <MenuItem value="">All decisions</MenuItem>
                        <MenuItem value="unverified">Unverified</MenuItem>
                        <MenuItem value="accepted">Accepted ({acceptedCount})</MenuItem>
                      </TextField>
                    ) : null
                  ) : null}
                  {tab !== 'reports' && tab !== 'unprocessed' ? (
                    <TextField
                      select
                      value={mediaArea}
                      onChange={(event) => setMediaArea(event.target.value)}
                      label="Media area"
                      size="small"
                      sx={{ width: { xs: '100%', sm: 210 } }}
                    >
                      <MenuItem value="">All media</MenuItem>
                      {mediaAreas.map((area) => <MenuItem key={area} value={area}>media/{area}</MenuItem>)}
                    </TextField>
                  ) : null}
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
                    {tab === 'unprocessed' ? (
                      <><Chip label={`${unprocessedCount} assets`} size="small" /><Chip label={formatBytes(safeArray(assets.data?.unprocessed).reduce((total, asset) => total + asset.sizeBytes, 0))} size="small" /></>
                    ) : tab === 'archive' ? (
                      <Chip label={`${filteredArchiveAssets.length}/${sumGroupFiles(areaGroups)} assets`} size="small" />
                    ) : (
                      <Chip label={`${filteredGroups.length}/${areaGroups.length} groups`} size="small" />
                    )}
                    {tab !== 'unprocessed' ? <Chip label={`${tab === 'archive' ? filteredArchiveAssets.length : sumGroupFiles(filteredGroups)} files`} size="small" /> : null}
                    {tab !== 'unprocessed' ? <Chip label={formatBytes(tab === 'archive' ? sumAssetBytes(filteredArchiveAssets) : sumGroupBytes(filteredGroups))} size="small" /> : null}
                    <Chip label={`Last sync: ${formatDate(assets.data?.sync?.lastSyncedAt ?? '')}`} size="small" />
                    {(assets.data?.sync?.missingActionable ?? assets.data?.sync?.missingFiles ?? 0) > 0 ? <Chip label={`${assets.data?.sync?.missingActionable ?? assets.data?.sync?.missingFiles ?? 0} missing`} color="warning" size="small" /> : null}
                    {(assets.data?.sync?.missingHistorical ?? 0) > 0 ? <Chip label={`${assets.data?.sync?.missingHistorical ?? 0} historical paths`} size="small" /> : null}
                  </Stack>
                ) : null}
              </Stack>
            </Stack>
			{sourceGroups.length > 0 && tab === 'unprocessed' ? (
				<Tabs value={selectedSourceGroup?.id ?? false} onChange={(_, value) => { setSourceGroupId(value); setAssetQuery(''); }} variant="scrollable" scrollButtons="auto" sx={{ mt: 1, borderTop: 1, borderColor: 'divider' }}>
					{sourceGroups.map((group) => <Tab key={group.id} value={group.id} label={`${group.name} (${group.fileCount})`} />)}
				</Tabs>
			) : null}
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
                Archived originals are protected here. Recovery keeps converted files; permanent deletion requires an explicit irreversible-action confirmation.
              </Alert>
            ) : null}
          </CardContent>
          {syncAssets.isError ? <Alert severity="warning" sx={{ m: 2 }}>Could not sync assets: {syncAssets.error.message}</Alert> : null}
          {tab === 'reports' ? (
            <AssetReportsPanel inventory={assets.data} />
		  ) : tab === 'unprocessed' ? (
			<AssetsErrorBoundary boundaryKey={`unprocessed-${selectedSourceGroup?.id ?? 'empty'}`}>
				<UnprocessedAssetsView
					key={selectedSourceGroup?.id ?? 'empty'}
					sourceGroup={selectedSourceGroup}
					operationalGroups={currentGroups}
					libraries={libraries.data ?? []}
					profiles={profiles.data ?? []}
					audioProfiles={audioProfiles}
					trackProfiles={trackProfiles}
					settings={settings.data ?? []}
					assetCategories={assetCategories}
					queueJobs={jobs.data ?? []}
					runningSnapshotPaths={runningSnapshotPaths}
					query={assetQuery}
				/>
			</AssetsErrorBoundary>
          ) : (
            <AssetsErrorBoundary boundaryKey={tab}>
              <AssetTable
                key={tab}
                groups={areaGroups}
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
  const queryClient = useQueryClient();
  const removeMissingAsset = useMutation({
    mutationFn: api.removeMissingAsset,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const removeAllMissingAssets = useMutation({
    mutationFn: api.removeAllMissingAssets,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
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
  const convertedSavedBytes = reports.convertedSpaceSavedBytes ?? 0;

  return (
    <CardContent>
      <Grid container spacing={1.5}>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <ReportTile label="Unprocessed" value={String(reports.unprocessedFiles)} />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <ReportTile label="Library assets" value={String(reports.libraryFiles)} helper={`${reports.unverifiedFiles} unverified · ${reports.acceptedFiles ?? 0} accepted`} />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <ReportTile label="Converted by MVForge" value={String(reports.convertedFiles)} />
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <ReportTile
            label={convertedSavedBytes >= 0 ? 'Space saved by conversion' : 'Conversion size increase'}
            value={formatBytes(Math.abs(convertedSavedBytes))}
            helper={`${reports.convertedComparedFiles ?? 0}/${reports.convertedFiles} active converted assets compared · ${formatBytes(reports.convertedOriginalBytes ?? 0)} → ${formatBytes(reports.convertedOutputBytes ?? 0)}`}
          />
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
            <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', sm: 'center' }} spacing={1}>
              <Stack>
                <Typography fontWeight={700}>Missing assets requiring attention</Typography>
                <Typography color="text.secondary" variant="body2">
                  These paths have no verified file of the same size and are not classified as renamed or archived history.
                </Typography>
              </Stack>
              <Button
                color="error"
                variant="outlined"
                size="small"
                startIcon={<DeleteForeverIcon />}
                disabled={removeAllMissingAssets.isPending || safeArray(inventory?.missing).length === 0}
                onClick={() => {
                  const count = safeArray(inventory?.missing).length;
                  if (!window.confirm(`Remove all ${count} missing asset records?\n\nFiles that reappeared and historical publication paths will be preserved. Jobs, logs, reports and snapshots will not be deleted.`)) return;
                  removeAllMissingAssets.mutate();
                }}
              >
                {removeAllMissingAssets.isPending ? 'Removing…' : 'Remove all missing'}
              </Button>
            </Stack>
          </Box>
          {removeMissingAsset.isError ? (
            <Alert severity="warning" sx={{ m: 1.5 }}>
              {removeMissingAsset.error instanceof Error ? removeMissingAsset.error.message : 'The missing asset could not be removed.'}
            </Alert>
          ) : null}
          {removeAllMissingAssets.isSuccess ? (
            <Alert severity="success" sx={{ m: 1.5 }}>
              {removeAllMissingAssets.data.removed} missing record(s) removed. {removeAllMissingAssets.data.preserved} preserved after validation.
            </Alert>
          ) : null}
          {removeAllMissingAssets.isError ? (
            <Alert severity="warning" sx={{ m: 1.5 }}>
              {removeAllMissingAssets.error instanceof Error ? removeAllMissingAssets.error.message : 'Missing records could not be removed.'}
            </Alert>
          ) : null}
          <Box sx={{ overflowX: 'auto' }}>
            <Table size="small" sx={{ minWidth: 760 }}>
              <TableHead>
                <TableRow>
                  <TableCell>Expected path</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>Source</TableCell>
                  <TableCell align="right">Recorded size</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {safeArray(inventory?.missing).map((asset) => (
                  <TableRow key={`${asset.status}-${asset.path}`}>
                    <TableCell sx={{ wordBreak: 'break-all' }}>{asset.path}</TableCell>
                    <TableCell><Chip label={statusLabel(asset.status)} color={statusColor(asset.status)} size="small" /></TableCell>
                    <TableCell>{asset.libraryName || 'Unknown'}</TableCell>
                    <TableCell align="right">{formatBytes(asset.sizeBytes)}</TableCell>
                    <TableCell align="right">
                      <Tooltip title="Remove this absent file from the inventory. Historical jobs, logs, reports and snapshots are preserved.">
                        <span>
                          <IconButton
                            color="error"
                            size="small"
                            disabled={removeMissingAsset.isPending}
                            aria-label={`Remove missing asset ${asset.fileName}`}
                            onClick={() => {
                              if (!window.confirm(`Remove the missing asset record for “${asset.fileName}”?\n\nThe media file is already absent. MVForge will preserve job history, logs, reports and snapshots.`)) return;
                              removeMissingAsset.mutate(asset.path);
                            }}
                          >
                            <DeleteForeverIcon fontSize="small" />
                          </IconButton>
                        </span>
                      </Tooltip>
                    </TableCell>
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

export function ArchiveDeleteConfirmationDialog({
  paths,
  pathLabel,
  accepted,
  pending,
  onAcceptedChange,
  onCancel,
  onConfirm,
}: {
  paths: string[];
  pathLabel: string;
  accepted: boolean;
  pending: boolean;
  onAcceptedChange: (accepted: boolean) => void;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <Dialog open={paths.length > 0} onClose={pending ? undefined : onCancel} disableEscapeKeyDown={pending} maxWidth="sm" fullWidth>
      <DialogTitle>Permanently delete from Archive?</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <Alert severity="error">
            These {paths.length} Archive asset(s) cannot be recovered after deletion. Converted Library assets, job history, reports, logs, and snapshots will remain, but the archived originals will be permanently lost.
          </Alert>
          <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
            {paths.length === 1 ? paths[0] : `${pathLabel || 'Archive path'} · ${paths.length} assets`}
          </Typography>
          <FormControlLabel
            control={<Checkbox checked={accepted} onChange={(event) => onAcceptedChange(event.target.checked)} disabled={pending} />}
            label="I understand that these Archive assets cannot be recovered."
          />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onCancel} disabled={pending}>Cancel</Button>
        <Button color="error" variant="contained" startIcon={<DeleteForeverIcon />} disabled={!accepted || pending} onClick={onConfirm}>
          {pending ? 'Deleting…' : 'Continue'}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

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

	if (mode === 'archive') {
		return <ArchivedAssetTable groups={visibleGroups} libraries={libraries} profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} assetCategories={assetCategories} queueJobs={queueJobs} runningSnapshotPaths={runningSnapshotPaths} query={query} emptyLabel={emptyLabel} />;
	}

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

type AssetCollection = {
  id: string;
  title: string;
  libraryName: string;
  groups: AssetGroup[];
  fileCount: number;
  sizeBytes: number;
};

function UnprocessedAssetsView({
	sourceGroup, operationalGroups, libraries, profiles, audioProfiles, trackProfiles, settings, assetCategories, queueJobs, runningSnapshotPaths, query,
}: {
	sourceGroup?: AssetSourceGroup;
	operationalGroups: AssetGroup[];
	libraries: Library[];
	profiles: Profile[];
	audioProfiles: AudioEnhancementProfile[];
	trackProfiles: TrackProfile[];
	settings: AppSetting[];
	assetCategories: string[];
	queueJobs: QueueJob[];
	runningSnapshotPaths: Set<string>;
	query: string;
}) {
	const [selectedAssetIds, setSelectedAssetIds] = useState<Set<number>>(() => new Set());
	if (!sourceGroup) return <CardContent><Alert severity="info">No Unprocessed Source Groups found.</Alert></CardContent>;
	const cleanQuery = query.trim().toLowerCase();
	const logicalGroups = safeArray(sourceGroup.logicalGroups).filter((logicalGroup) => sourceLogicalGroupMatches(logicalGroup, cleanQuery));
	return (
		<Box sx={{ borderTop: 1, borderColor: 'divider', px: { xs: 1, sm: 2 }, pb: 2 }}>
			<Box sx={{ px: 2, py: 1.5 }}>
				<Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}><Typography variant="h2">{sourceGroup.name}</Typography><SourceGroupConfigureButton sourceGroup={sourceGroup} profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} libraries={libraries} categories={assetCategories} /></Stack>
				<Typography variant="body2" color="text.secondary">{countLabel(sourceGroup.assetCount, 'asset')} · {countLabel(sourceGroup.titleCount, 'title')} · {countLabel(sourceGroup.pathCount, 'path')} · {formatBytes(sourceGroup.totalSizeBytes)}</Typography>
				{selectedSelectableAssetsForLogicalGroups(sourceGroup.logicalGroups, selectedAssetIds).length ? <UnprocessedSelectionToolbar selectedAssetIds={selectedAssetIds} logicalGroups={sourceGroup.logicalGroups} profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} libraries={libraries} categories={assetCategories} onClear={() => setSelectedAssetIds(new Set())} /> : null}
			</Box>
			<Stack spacing={1.25}>{logicalGroups.map((logicalGroup) => <UnprocessedLogicalGroup key={logicalGroup.id} logicalGroup={logicalGroup} operationalGroups={operationalGroups} selectedAssetIds={selectedAssetIds} onAssetSelectionChange={(assetIds, selected) => setSelectedAssetIds((current) => { const next = new Set(current); for (const assetId of assetIds) { if (selected) next.add(assetId); else next.delete(assetId); } return next; })} libraries={libraries} profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} settings={settings} assetCategories={assetCategories} queueJobs={queueJobs} runningSnapshotPaths={runningSnapshotPaths} />)}{!logicalGroups.length ? <Alert severity="info">No Unprocessed assets match this search.</Alert> : null}</Stack>
		</Box>
	);
}

type PreparedSelectionQueue = QueueSelectedAssetsResponse;

function UnprocessedLogicalGroup({ logicalGroup, operationalGroups, selectedAssetIds, onAssetSelectionChange, libraries, profiles, audioProfiles, trackProfiles, settings, assetCategories, queueJobs, runningSnapshotPaths }: {
	logicalGroup: AssetLogicalGroup; operationalGroups: AssetGroup[]; selectedAssetIds: Set<number>; onAssetSelectionChange: (ids: number[], selected: boolean) => void; libraries: Library[]; profiles: Profile[]; audioProfiles: AudioEnhancementProfile[]; trackProfiles: TrackProfile[]; settings: AppSetting[]; assetCategories: string[]; queueJobs: QueueJob[]; runningSnapshotPaths: Set<string>;
}) {
	const [expanded, setExpanded] = useState(false);
	const [expandedPaths, setExpandedPaths] = useState<Set<string>>(() => new Set());
	const [snapshotPath, setSnapshotPath] = useState<import('../api/types').AssetPath | null>(null);
	const [managedPath, setManagedPath] = useState<import('../api/types').AssetPath | null>(null);
	const paths = safeArray(logicalGroup.assetPaths);
	const assets = selectableAssetsForPaths(paths);
	const assetIds = assets.map((asset) => asset.id as number);
	const groupSelection = hierarchicalSelectionState(assets.map((asset) => asset.id as number), selectedAssetIds);
	const effectiveConfigurations = useQuery({
		queryKey: ['effectiveAssetConfiguration', 'batch', ...assetIds],
		queryFn: () => api.effectiveAssetConfigurations(assetIds),
		enabled: expanded && assetIds.length > 0,
		staleTime: 15_000,
	});
	const rootPaths = paths.filter((path) => path.isLogicalGroupRoot);
	const childPaths = paths.filter((path) => !path.isLogicalGroupRoot);
	const operationalGroup = (path: import('../api/types').AssetPath) => operationalGroups.find((group) => normalizePath(group.path) === normalizePath(path.path)) ?? assetGroupFromTreePath(path);
	const renderAssets = (path: import('../api/types').AssetPath) => {
		const visibleAssets = safeArray(path.assets).filter((asset) => asset.id);
		if (!visibleAssets.length) return <Alert severity="info">No available assets in this path.</Alert>;
		return <Box sx={{ overflowX: 'auto' }}><Table size="small" sx={{ minWidth: 720, tableLayout: 'fixed' }}><TableHead><TableRow><TableCell padding="checkbox" sx={{ width: 48 }} /><TableCell>Asset</TableCell><TableCell sx={{ width: 180 }}>Status</TableCell><TableCell sx={{ width: 100 }}>Size</TableCell><TableCell align="center" sx={{ width: 220 }}>Actions</TableCell></TableRow></TableHead><TableBody>{visibleAssets.map((asset) => {
			const effective = asset.id ? effectiveConfigurations.data?.configurations[String(asset.id)] : undefined;
			const track = trackProfiles.find((profile) => profile.key === effective?.tracks.profileKey);
			return <AssetRow key={`${asset.id}-${effective?.video.videoProfileId ?? 0}-${effective?.destination.destinationLibraryId ?? 0}`} asset={asset} compactUnprocessed libraries={libraries} profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} pathTrackProfile={track} assetCategories={assetCategories} groupRelativePath={path.relativePath} groupPath={path.path} groupCategory={effective?.category.category ?? ''} confidenceEnabled={!getDisabledConfidencePaths(settings).includes(path.path)} groupProfileId={effective?.video.videoProfileId ?? 0} groupAudioProfileKey={effective?.audio.profileKey ?? ''} groupLibraryId={effective?.destination.destinationLibraryId ?? 0} hasOpenJob={assetHasOpenJob(asset, queueJobs)} queueJobs={queueJobs} snapshotRunning={runningSnapshotPaths.has(asset.path)} mode="unprocessed" bulkSelected={selectedAssetIds.has(asset.id as number)} bulkSelectionEnabled bulkSelectionDisabled={asset.missing} onBulkSelectionChange={(_, selected) => onAssetSelectionChange([asset.id as number], selected)} />;
		})}</TableBody></Table></Box>;
	};
	return <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, overflow: 'hidden' }}>
		<Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={1} sx={{ px: 1, py: 1, bgcolor: 'rgba(255,255,255,0.035)' }}>
			<Stack direction="row" alignItems="center" spacing={0.5}><Checkbox size="small" checked={groupSelection.checked} indeterminate={groupSelection.indeterminate} disabled={!assets.length} onChange={(event) => onAssetSelectionChange(assets.map((asset) => asset.id as number), event.target.checked)} inputProps={{ 'aria-label': `Select title ${logicalGroup.name}` }} /><IconButton size="small" onClick={() => setExpanded((value) => !value)} aria-label={`${expanded ? 'Collapse' : 'Expand'} ${logicalGroup.name}`}><ExpandMoreIcon sx={{ transform: expanded ? 'rotate(180deg)' : 'none' }} /></IconButton><Box><Typography variant="h3">{logicalGroup.name}</Typography><Typography variant="body2" color="text.secondary">{countLabel(logicalGroup.assetCount, 'asset')} · {countLabel(logicalGroup.pathCount, 'path')} · {formatBytes(logicalGroup.totalSizeBytes)}</Typography></Box></Stack>
			<ScopeConfigureButton targetType="logical_group" scopeKey={logicalGroup.path} label="Configure title" profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} libraries={libraries} categories={assetCategories} />
		</Stack>
		<Collapse in={expanded} unmountOnExit><Stack spacing={1} sx={{ p: 1.25 }}>{rootPaths.map((path) => <Box key={path.id}><Stack direction="row" justifyContent="flex-end" spacing={0.5} sx={{ mb: 0.5 }}><ScopeConfigureButton targetType="path" scopeKey={path.path} label="Root path settings" profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} libraries={libraries} categories={assetCategories} /><Button size="small" startIcon={<InfoOutlinedIcon />} onClick={() => setManagedPath(path)}>Path actions</Button><Button size="small" startIcon={<ManageSearchIcon />} onClick={() => setSnapshotPath(path)}>Snapshots</Button></Stack>{renderAssets(path)}</Box>)}{childPaths.map((path) => { const pathAssets = selectableAssetsForPaths([path]); const checked = pathAssets.length > 0 && pathAssets.every((asset) => selectedAssetIds.has(asset.id as number)); const some = pathAssets.some((asset) => selectedAssetIds.has(asset.id as number)); const open = expandedPaths.has(path.id); return <Box key={path.id} sx={{ border: 1, borderColor: 'divider', borderRadius: 1 }}><Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ px: 0.75, py: 0.5 }}><Stack direction="row" alignItems="center"><Checkbox size="small" checked={checked} indeterminate={some && !checked} disabled={!pathAssets.length} onChange={(event) => onAssetSelectionChange(pathAssets.map((asset) => asset.id as number), event.target.checked)} inputProps={{ 'aria-label': `Select path ${path.displayPath || path.name}` }} /><IconButton size="small" aria-label={`${open ? 'Collapse' : 'Expand'} path ${path.displayPath || path.name}`} onClick={() => setExpandedPaths((current) => { const next = new Set(current); if (next.has(path.id)) next.delete(path.id); else next.add(path.id); return next; })}><ExpandMoreIcon sx={{ transform: open ? 'rotate(180deg)' : 'none' }} /></IconButton><Box><Typography fontWeight={700}>{path.displayPath || path.name}</Typography><Typography variant="caption" color="text.secondary">{countLabel(path.assetCount, 'asset')} · {formatBytes(path.totalSizeBytes)}</Typography></Box></Stack><Stack direction="row"><ScopeConfigureButton targetType="path" scopeKey={path.path} label="Configure" profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} libraries={libraries} categories={assetCategories} compact /><IconButton size="small" onClick={() => setManagedPath(path)} aria-label={`Path actions ${path.displayPath || path.name}`}><InfoOutlinedIcon fontSize="small" /></IconButton><IconButton size="small" onClick={() => setSnapshotPath(path)}><ManageSearchIcon fontSize="small" /></IconButton></Stack></Stack><Collapse in={open} unmountOnExit><Box sx={{ p: 1 }}>{renderAssets(path)}</Box></Collapse></Box>; })}</Stack></Collapse>
		{snapshotPath ? <PathSnapshotsDialog open group={operationalGroup(snapshotPath)} runningSnapshotPaths={runningSnapshotPaths} onClose={() => setSnapshotPath(null)} /> : null}
		<Dialog open={Boolean(managedPath)} onClose={() => setManagedPath(null)} maxWidth="xl" fullWidth><DialogTitle>Path actions · {managedPath?.displayPath || managedPath?.name}</DialogTitle><DialogContent dividers sx={{ p: 0 }}>{managedPath ? <Box sx={{ overflowX: 'auto' }}><Table size="small" sx={{ minWidth: 980 }}><TableBody><AssetGroupRow group={operationalGroup(managedPath)} libraries={libraries} profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} settings={settings} assetCategories={assetCategories} queueJobs={queueJobs} runningSnapshotPaths={runningSnapshotPaths} mode="unprocessed" /></TableBody></Table></Box> : null}</DialogContent><DialogActions><Button onClick={() => setManagedPath(null)}>Close</Button></DialogActions></Dialog>
	</Box>;
}

function assetGroupFromTreePath(path: import('../api/types').AssetPath): AssetGroup {
	const assets = safeArray(path.assets);
	return { id: path.id, libraryId: assets[0]?.libraryId ?? 0, libraryName: assets[0]?.libraryName ?? 'Originals', path: path.path, relativePath: path.relativePath, status: 'unprocessed', fileCount: path.fileCount, sizeBytes: path.totalSizeBytes, modifiedAt: assets[0]?.modifiedAt ?? '', assets, review: assets[0]?.review, pathReview: assets[0]?.review, metadata: assets[0]?.metadata, pathMetadata: assets[0]?.metadata } as AssetGroup;
}

function ScopeConfigureButton({ targetType, scopeKey, label, profiles, audioProfiles, trackProfiles, libraries, categories, compact = false }: { targetType: 'logical_group' | 'path'; scopeKey: string; label: string; profiles: Profile[]; audioProfiles: AudioEnhancementProfile[]; trackProfiles: TrackProfile[]; libraries: Library[]; categories: string[]; compact?: boolean }) {
	const queryClient = useQueryClient();
	const [open, setOpen] = useState(false);
	const [loading, setLoading] = useState(false);
	const [loadError, setLoadError] = useState('');
	const [fields, setFields] = useState<Set<ScopeConfigurationField>>(() => new Set());
	const [video, setVideo] = useState(0);
	const [audio, setAudio] = useState('__inherit__');
	const [tracks, setTracks] = useState('__inherit__');
	const [categoryMode, setCategoryMode] = useState<'inherit' | 'value' | 'disabled'>('inherit');
	const [category, setCategory] = useState('');
	const [destinationMode, setDestinationMode] = useState<'inherit' | 'value' | 'disabled'>('inherit');
	const [destination, setDestination] = useState(0);

	async function loadPersistedConfiguration() {
		setLoading(true);
		setLoadError('');
		try {
			const [assignments, configurations] = await Promise.all([
				queryClient.fetchQuery({ queryKey: ['profileAssignments'], queryFn: api.profileAssignments }),
				queryClient.fetchQuery({ queryKey: ['assetScopeConfigurations'], queryFn: api.assetScopeConfigurations }),
			]);
			const values = scopeConfigurationEditorValues(targetType, scopeKey, assignments, configurations);
			setVideo(values.videoProfileId);
			setAudio(values.audioProfileKey);
			setTracks(values.trackProfileKey);
			setCategoryMode(values.categorySelection);
			setCategory(values.category);
			setDestinationMode(values.destinationSelection);
			setDestination(values.destinationLibraryId);
		} catch (error) {
			setLoadError(error instanceof Error ? error.message : 'Could not load persisted configuration.');
		} finally {
			setLoading(false);
		}
	}

	function openConfiguration() {
		setFields(new Set());
		setOpen(true);
		void loadPersistedConfiguration();
	}

	const save = useMutation({
		mutationFn: async () => {
			if (!fields.size) throw new Error('Select at least one field to change.');
			if (fields.has('category') || fields.has('destination')) {
				const configurations = await api.assetScopeConfigurations();
				const current = configurations.find((item) => item.scopeType === targetType && normalizePath(item.scopeKey) === normalizePath(scopeKey));
				await api.updateAssetScopeConfiguration(mergedScopeConfigurationInput(targetType, scopeKey, current, fields, {
					videoProfileId: video,
					audioProfileKey: audio,
					trackProfileKey: tracks,
					categorySelection: categoryMode,
					category,
					destinationSelection: destinationMode,
					destinationLibraryId: destination,
				}));
			}
			for (const [mediaType, value] of [['video', video], ['audio', audio], ['tracks', tracks]] as const) {
				if (!fields.has(mediaType)) continue;
				await api.updateProfileAssignment(profileAssignmentForScopeChange(targetType, scopeKey, mediaType, value));
			}
		},
		onSuccess: async () => {
			await Promise.all([
				queryClient.invalidateQueries({ queryKey: ['profileAssignments'] }),
				queryClient.invalidateQueries({ queryKey: ['assetScopeConfigurations'] }),
				queryClient.invalidateQueries({ queryKey: ['effectiveAssetConfiguration'] }),
				queryClient.invalidateQueries({ queryKey: ['assets'] }),
			]);
			setOpen(false);
		},
	});

	const toggle = (field: ScopeConfigurationField) => setFields((current) => {
		const next = new Set(current);
		if (next.has(field)) next.delete(field); else next.add(field);
		return next;
	});

	return <><Button size="small" variant={compact ? 'text' : 'outlined'} startIcon={<EditIcon />} onClick={openConfiguration}>{label}</Button><Dialog open={open} onClose={() => !save.isPending && setOpen(false)} maxWidth="md" fullWidth><DialogTitle>{label}</DialogTitle><DialogContent dividers>{loading ? <Stack spacing={1}><LinearProgress /><Typography variant="body2" color="text.secondary">Loading persisted scope configuration…</Typography></Stack> : loadError ? <Alert severity="warning" action={<Button color="inherit" size="small" onClick={() => void loadPersistedConfiguration()}>Retry</Button>}>{loadError}</Alert> : <Stack spacing={1.5}><Alert severity="info">Choose exactly which dimensions to change. Unchecked dimensions remain untouched.</Alert><FormControlLabel control={<Checkbox checked={fields.has('video')} onChange={() => toggle('video')} />} label="Change video" /><ProfileAutocomplete profiles={profiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt)} value={video} onChange={setVideo} label="Video profile" allowNone allowInherit disabled={!fields.has('video')} /><FormControlLabel control={<Checkbox checked={fields.has('audio')} onChange={() => toggle('audio')} />} label="Change audio" /><AudioProfileAutocomplete profiles={audioProfiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt)} value={audio} onChange={setAudio} label="Audio profile" allowInherit disabled={!fields.has('audio')} /><FormControlLabel control={<Checkbox checked={fields.has('tracks')} onChange={() => toggle('tracks')} />} label="Change tracks" /><TrackProfileAutocomplete profiles={trackProfiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt)} value={tracks} onChange={setTracks} label="Tracks profile" allowInherit disabled={!fields.has('tracks')} /><Divider /><FormControlLabel control={<Checkbox checked={fields.has('category')} onChange={() => toggle('category')} />} label="Change category" /><Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}><TextField select fullWidth label="Category mode" value={categoryMode} onChange={(event) => setCategoryMode(event.target.value as typeof categoryMode)} disabled={!fields.has('category')}><MenuItem value="inherit">Inherit</MenuItem><MenuItem value="value">Override</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField><AssetCategorySelect value={category} options={categories} onChange={setCategory} label="Category" disabled={!fields.has('category') || categoryMode !== 'value'} /></Stack><FormControlLabel control={<Checkbox checked={fields.has('destination')} onChange={() => toggle('destination')} />} label="Change destination" /><Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}><TextField select fullWidth label="Destination mode" value={destinationMode} onChange={(event) => setDestinationMode(event.target.value as typeof destinationMode)} disabled={!fields.has('destination')}><MenuItem value="inherit">Inherit</MenuItem><MenuItem value="value">Override</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField><LibraryAutocomplete libraries={libraries} value={destination} onChange={setDestination} label="Destination" disabled={!fields.has('destination') || destinationMode !== 'value'} /></Stack>{save.isError ? <Alert severity="warning">{save.error instanceof Error ? save.error.message : 'Could not save configuration.'}</Alert> : null}</Stack>}</DialogContent><DialogActions><Button onClick={() => setOpen(false)} disabled={save.isPending}>Cancel</Button><Button variant="contained" onClick={() => save.mutate()} disabled={loading || Boolean(loadError) || save.isPending || !fields.size || (fields.has('category') && categoryMode === 'value' && !category) || (fields.has('destination') && destinationMode === 'value' && !destination)}>Apply</Button></DialogActions></Dialog></>;
}

function UnprocessedSelectionToolbar({ selectedAssetIds, logicalGroups, profiles, audioProfiles, trackProfiles, libraries, categories, onClear }: {
	selectedAssetIds: Set<number>;
	logicalGroups: AssetLogicalGroup[];
	profiles: Profile[];
	audioProfiles: AudioEnhancementProfile[];
	trackProfiles: TrackProfile[];
	libraries: Library[];
	categories: string[];
	onClear: () => void;
}) {
	const queryClient = useQueryClient();
	const navigate = useNavigate();
	const [configureOpen, setConfigureOpen] = useState(false);
	const [queuePlan, setQueuePlan] = useState<PreparedSelectionQueue | null>(null);
	const [queueCommitResult, setQueueCommitResult] = useState<QueueSelectedAssetsResponse | null>(null);
	const [videoProfileId, setVideoProfileId] = useState(0);
	const [audioProfileKey, setAudioProfileKey] = useState('__inherit__');
	const [trackProfileKey, setTrackProfileKey] = useState('__inherit__');
	const [categoryMode, setCategoryMode] = useState<'inherit' | 'value' | 'disabled'>('inherit');
	const [category, setCategory] = useState('');
	const [destinationMode, setDestinationMode] = useState<'inherit' | 'value' | 'disabled'>('inherit');
	const [destinationLibraryId, setDestinationLibraryId] = useState(0);
	const [configurationFields, setConfigurationFields] = useState<Set<string>>(() => new Set());
	const selectedAssets = selectedSelectableAssetsForLogicalGroups(logicalGroups, selectedAssetIds);
	const selectedGroups = logicalGroups.filter((group) => selectableAssetsForPaths(group.assetPaths).some((asset) => selectedAssetIds.has(asset.id as number)));
	const fullySelectedGroups = selectedGroups.filter((group) => {
		const ids = group.assetPaths.flatMap((path) => path.assets).filter((asset) => !asset.missing && asset.id).map((asset) => asset.id as number);
		return ids.length > 0 && ids.every((id) => selectedAssetIds.has(id));
	});
	const selectedSize = selectedAssets.reduce((total, asset) => total + asset.sizeBytes, 0);

	const saveSelectedConfiguration = useMutation({
		mutationFn: async () => {
			if (!fullySelectedGroups.length) throw new Error('Select at least one complete title.');
			if (!configurationFields.size) throw new Error('Select at least one field to change.');
			const profileChange = (field: 'audio' | 'tracks', value: string) => !configurationFields.has(field)
				? { mode: 'no_change' as const }
				: value === '__inherit__' ? { mode: 'inherit' as const }
					: value === '' ? { mode: 'disabled' as const }
						: { mode: 'value' as const, profileKey: value };
			await api.configureLogicalGroupsBatch({
				logicalGroupPaths: fullySelectedGroups.map((group) => group.path),
				video: !configurationFields.has('video') ? { mode: 'no_change' } : videoProfileId === 0 ? { mode: 'inherit' } : videoProfileId < 0 ? { mode: 'disabled' } : { mode: 'value', videoProfileId },
				audio: profileChange('audio', audioProfileKey),
				tracks: profileChange('tracks', trackProfileKey),
				category: !configurationFields.has('category') ? { mode: 'no_change' } : { mode: categoryMode, category: categoryMode === 'value' ? category : undefined },
				destination: !configurationFields.has('destination') ? { mode: 'no_change' } : { mode: destinationMode, destinationLibraryId: destinationMode === 'value' ? destinationLibraryId : undefined },
			});
		},
		onSuccess: async () => {
			await Promise.all([queryClient.invalidateQueries({ queryKey: ['profileAssignments'] }), queryClient.invalidateQueries({ queryKey: ['assetScopeConfigurations'] }), queryClient.invalidateQueries({ queryKey: ['effectiveAssetConfiguration'] }), queryClient.invalidateQueries({ queryKey: ['assets'] })]);
			setConfigureOpen(false);
		},
	});

	const prepareQueue = useMutation({
		mutationFn: async () => {
			return api.queueSelectedAssets({ assetIds: selectedAssets.map((asset) => asset.id as number), commit: false });
		},
		onSuccess: (response) => { setQueueCommitResult(null); setQueuePlan(response); },
	});

	const createQueue = useMutation({
		mutationFn: async () => {
			if (!queuePlan?.summary.eligible) throw new Error('No selected assets are eligible for Queue.');
			return api.queueSelectedAssets({ assetIds: queuePlan.results.filter((result) => result.outcome === 'eligible').map((result) => result.assetId), commit: true });
		},
		onSuccess: async (response) => {
			await Promise.all([queryClient.invalidateQueries({ queryKey: ['queueJobs'] }), queryClient.invalidateQueries({ queryKey: ['assets'] })]);
			if (response.summary.skipped || response.summary.failed) {
				setQueueCommitResult(response);
				setQueuePlan(response);
				return;
			}
			setQueuePlan(null); setQueueCommitResult(null); onClear();
			if (response.batches[0]) navigate(`/queue?batch=${encodeURIComponent(response.batches[0].batchId)}`);
		},
	});
	const displayedQueueResult = queueCommitResult ?? queuePlan;
	const queueReasonCount = (reason: string) => displayedQueueResult?.results.filter((result) => result.reason === reason).length ?? 0;
	const closeQueueDialog = () => { setQueuePlan(null); setQueueCommitResult(null); };

	return <>
		<Box sx={{ mt: 1.25, p: 1, border: 1, borderColor: 'primary.main', borderRadius: 1, bgcolor: 'action.selected', position: 'sticky', top: 8, zIndex: 2 }}>
			<Stack direction={{ xs: 'column', md: 'row' }} alignItems={{ xs: 'stretch', md: 'center' }} justifyContent="space-between" spacing={1}>
				<Typography variant="body2">{countLabel(fullySelectedGroups.length, 'title')} selected · {countLabel(selectedAssets.length, 'asset')} · {formatBytes(selectedSize)}</Typography>
				<Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap><Button size="small" variant="contained" startIcon={<PlaylistAddIcon />} onClick={() => prepareQueue.mutate()} disabled={prepareQueue.isPending}>{prepareQueue.isPending ? 'Resolving…' : 'Queue selected'}</Button><Button size="small" variant="outlined" startIcon={<EditIcon />} disabled={!fullySelectedGroups.length} onClick={() => { setConfigurationFields(new Set()); setConfigureOpen(true); }}>Configure selected</Button><Button size="small" onClick={onClear}>Clear</Button></Stack>
			</Stack>
			{prepareQueue.isError ? <Alert severity="warning" sx={{ mt: 1 }}>{prepareQueue.error instanceof Error ? prepareQueue.error.message : 'Could not resolve the selected assets.'}</Alert> : null}
		</Box>
		<Dialog open={Boolean(queuePlan)} onClose={() => !createQueue.isPending && closeQueueDialog()} maxWidth="sm" fullWidth><DialogTitle>Queue selected titles</DialogTitle><DialogContent dividers>{displayedQueueResult ? <Stack spacing={1}><Typography>Titles: {displayedQueueResult.summary.titleCount}</Typography><Typography>Assets selected: {displayedQueueResult.summary.selected}</Typography><Typography>{queueCommitResult ? 'Queued' : 'Will be queued'}: {queueCommitResult ? displayedQueueResult.summary.queued : displayedQueueResult.summary.eligible}</Typography><Typography>Estimated input size: {formatBytes(displayedQueueResult.summary.sizeBytes)}</Typography><Typography>Already queued: {queueReasonCount('already_queued')}</Typography><Typography>Needs review: {queueReasonCount('needs_review')}</Typography><Typography>Missing: {queueReasonCount('missing')}</Typography><Typography>Blocked or incomplete configuration: {queueReasonCount('invalid_configuration') + queueReasonCount('not_found') + queueReasonCount('active_maintenance')}</Typography><Divider /><Typography variant="body2" color="text.secondary">Destination and profiles are resolved independently for every asset and frozen in each Queue job.</Typography>{queueCommitResult && (queueCommitResult.summary.skipped || queueCommitResult.summary.failed) ? <Alert severity={queueCommitResult.summary.queued ? 'warning' : 'error'}>{queueCommitResult.summary.queued} queued · {queueCommitResult.summary.skipped} skipped · {queueCommitResult.summary.failed} failed{queueCommitResult.results.filter((result) => result.outcome === 'skipped' || result.outcome === 'failed').map((result) => <Typography key={result.assetId} component="div" variant="body2">Asset {result.assetId}: {result.message ?? result.reason}</Typography>)}</Alert> : null}{createQueue.isError ? <Alert severity="warning">{createQueue.error instanceof Error ? createQueue.error.message : 'Could not queue the selection.'}</Alert> : null}</Stack> : null}</DialogContent><DialogActions><Button onClick={closeQueueDialog} disabled={createQueue.isPending}>{queueCommitResult ? 'Close' : 'Cancel'}</Button><Button variant="contained" onClick={() => createQueue.mutate()} disabled={createQueue.isPending || Boolean(queueCommitResult) || !queuePlan?.summary.eligible}>Queue {queuePlan?.summary.eligible ?? 0} assets</Button></DialogActions></Dialog>
		<Dialog open={configureOpen} onClose={() => !saveSelectedConfiguration.isPending && setConfigureOpen(false)} maxWidth="md" fullWidth><DialogTitle>Configure {fullySelectedGroups.length} selected title{fullySelectedGroups.length === 1 ? '' : 's'}</DialogTitle><DialogContent dividers><Stack spacing={2}><Alert severity="info">Checked dimensions are written independently to each Logical Group. Unchecked dimensions remain unchanged.</Alert>{([['video','Video'],['audio','Audio'],['tracks','Tracks'],['category','Category'],['destination','Destination']] as const).map(([field, text]) => <FormControlLabel key={field} control={<Checkbox checked={configurationFields.has(field)} onChange={() => setConfigurationFields((current) => { const next = new Set(current); if (next.has(field)) next.delete(field); else next.add(field); return next; })} />} label={`Change ${text}`} />)}<Grid container spacing={2}><Grid size={{ xs: 12, md: 4 }}><ProfileAutocomplete profiles={profiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt)} value={videoProfileId} onChange={setVideoProfileId} label="Video profile" allowNone allowInherit disabled={!configurationFields.has('video')} /></Grid><Grid size={{ xs: 12, md: 4 }}><AudioProfileAutocomplete profiles={audioProfiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt)} value={audioProfileKey} onChange={setAudioProfileKey} label="Audio profile" allowInherit disabled={!configurationFields.has('audio')} /></Grid><Grid size={{ xs: 12, md: 4 }}><TrackProfileAutocomplete profiles={trackProfiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt)} value={trackProfileKey} onChange={setTrackProfileKey} label="Tracks profile" allowInherit disabled={!configurationFields.has('tracks')} /></Grid><Grid size={{ xs: 12, sm: 3 }}><TextField select fullWidth label="Category mode" value={categoryMode} onChange={(event) => setCategoryMode(event.target.value as typeof categoryMode)} disabled={!configurationFields.has('category')}><MenuItem value="inherit">Inherit</MenuItem><MenuItem value="value">Override</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField></Grid><Grid size={{ xs: 12, sm: 3 }}><AssetCategorySelect value={category} options={categories} onChange={setCategory} label="Category" disabled={!configurationFields.has('category') || categoryMode !== 'value'} /></Grid><Grid size={{ xs: 12, sm: 3 }}><TextField select fullWidth label="Destination mode" value={destinationMode} onChange={(event) => setDestinationMode(event.target.value as typeof destinationMode)} disabled={!configurationFields.has('destination')}><MenuItem value="inherit">Inherit</MenuItem><MenuItem value="value">Override</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField></Grid><Grid size={{ xs: 12, sm: 3 }}><LibraryAutocomplete libraries={libraries} value={destinationLibraryId} onChange={setDestinationLibraryId} label="Destination" disabled={!configurationFields.has('destination') || destinationMode !== 'value'} /></Grid></Grid>{saveSelectedConfiguration.isError ? <Alert severity="warning">{saveSelectedConfiguration.error instanceof Error ? saveSelectedConfiguration.error.message : 'Could not configure the selected titles.'}</Alert> : null}</Stack></DialogContent><DialogActions><Button onClick={() => setConfigureOpen(false)} disabled={saveSelectedConfiguration.isPending}>Cancel</Button><Button variant="contained" onClick={() => saveSelectedConfiguration.mutate()} disabled={saveSelectedConfiguration.isPending || !configurationFields.size || (configurationFields.has('category') && categoryMode === 'value' && !category) || (configurationFields.has('destination') && destinationMode === 'value' && !destinationLibraryId)}>Apply to selected titles</Button></DialogActions></Dialog>
	</>;
}

function SourceGroupConfigureButton({ sourceGroup, profiles, audioProfiles, trackProfiles, libraries, categories }: { sourceGroup: AssetSourceGroup; profiles: Profile[]; audioProfiles: AudioEnhancementProfile[]; trackProfiles: TrackProfile[]; libraries: Library[]; categories: string[] }) {
	const queryClient = useQueryClient();
	const assignments = useQuery({ queryKey: ['profileAssignments'], queryFn: api.profileAssignments });
	const configurations = useQuery({ queryKey: ['assetScopeConfigurations'], queryFn: api.assetScopeConfigurations });
	const [open, setOpen] = useState(false);
	const [videoProfileId, setVideoProfileId] = useState(0);
	const [audioProfileKey, setAudioProfileKey] = useState('__inherit__');
	const [trackProfileKey, setTrackProfileKey] = useState('__inherit__');
	const [categorySelection, setCategorySelection] = useState<'inherit' | 'value' | 'disabled'>('inherit');
	const [category, setCategory] = useState('');
	const [destinationSelection, setDestinationSelection] = useState<'inherit' | 'value' | 'disabled'>('inherit');
	const [destinationLibraryId, setDestinationLibraryId] = useState(0);
	const save = useMutation({
		mutationFn: async () => {
			await api.updateAssetScopeConfiguration({ scopeType: 'source_group', scopeKey: sourceGroup.sourcePath, categorySelection, category, destinationSelection, destinationLibraryId });
			for (const [mediaType, value] of [['video', videoProfileId], ['audio', audioProfileKey], ['tracks', trackProfileKey]] as const) {
				const inherit = value === 0 || value === '__inherit__';
				const disabled = value === '' || (mediaType === 'video' && Number(value) < 0);
				await api.updateProfileAssignment({ targetType: 'source_group', targetPath: sourceGroup.sourcePath, mediaType, selection: inherit ? 'inherit' : disabled ? 'disabled' : 'profile', videoProfileId: mediaType === 'video' ? Math.max(0, Number(value)) : 0, profileKey: mediaType === 'video' || inherit || disabled ? '' : String(value) });
			}
		},
		onSuccess: async () => { await Promise.all([queryClient.invalidateQueries({ queryKey: ['profileAssignments'] }), queryClient.invalidateQueries({ queryKey: ['assetScopeConfigurations'] }), queryClient.invalidateQueries({ queryKey: ['effectiveAssetConfiguration'] }), queryClient.invalidateQueries({ queryKey: ['assets'] })]); setOpen(false); },
	});
	function openConfiguration() {
		const scoped = safeArray(assignments.data).filter((item) => item.targetType === 'source_group' && normalizePath(item.targetPath) === normalizePath(sourceGroup.sourcePath));
		const video = scoped.find((item) => item.mediaType === 'video'); const audio = scoped.find((item) => item.mediaType === 'audio'); const tracks = scoped.find((item) => item.mediaType === 'tracks');
		setVideoProfileId(video?.selection === 'profile' ? video.videoProfileId ?? 0 : video?.selection === 'disabled' ? -1 : 0);
		setAudioProfileKey(audio?.selection === 'profile' ? audio.profileKey ?? '' : audio?.selection === 'disabled' ? '' : '__inherit__');
		setTrackProfileKey(tracks?.selection === 'profile' ? tracks.profileKey ?? '' : tracks?.selection === 'disabled' ? '' : '__inherit__');
		const configuration = safeArray(configurations.data).find((item) => item.scopeType === 'source_group' && normalizePath(item.scopeKey) === normalizePath(sourceGroup.sourcePath));
		setCategorySelection(configuration?.categorySelection ?? 'inherit'); setCategory(configuration?.category ?? ''); setDestinationSelection(configuration?.destinationSelection ?? 'inherit'); setDestinationLibraryId(configuration?.destinationLibraryId ?? 0); setOpen(true);
	}
	return <><Button variant="outlined" size="small" startIcon={<EditIcon />} onClick={openConfiguration}>Configure</Button><Dialog open={open} onClose={() => !save.isPending && setOpen(false)} maxWidth="md" fullWidth><DialogTitle>Configure Source Group · {sourceGroup.name}</DialogTitle><DialogContent dividers><Stack spacing={2}><Alert severity="info">Defaults apply only to Unprocessed descendants that still inherit this dimension.</Alert><Grid container spacing={2}><Grid size={{ xs: 12, md: 4 }}><ProfileAutocomplete profiles={profiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt)} value={videoProfileId} onChange={setVideoProfileId} label="Video profile" allowNone allowInherit /></Grid><Grid size={{ xs: 12, md: 4 }}><AudioProfileAutocomplete profiles={audioProfiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt)} value={audioProfileKey} onChange={setAudioProfileKey} label="Audio profile" allowInherit /></Grid><Grid size={{ xs: 12, md: 4 }}><TrackProfileAutocomplete profiles={trackProfiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt)} value={trackProfileKey} onChange={setTrackProfileKey} label="Tracks profile" allowInherit /></Grid><Grid size={{ xs: 12, sm: 3 }}><TextField select fullWidth label="Category mode" value={categorySelection} onChange={(event) => setCategorySelection(event.target.value as typeof categorySelection)}><MenuItem value="inherit">Inherit</MenuItem><MenuItem value="value">Override</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField></Grid><Grid size={{ xs: 12, sm: 3 }}><AssetCategorySelect value={category} options={categories} onChange={setCategory} label="Category" disabled={categorySelection !== 'value'} /></Grid><Grid size={{ xs: 12, sm: 3 }}><TextField select fullWidth label="Destination mode" value={destinationSelection} onChange={(event) => setDestinationSelection(event.target.value as typeof destinationSelection)}><MenuItem value="inherit">Inherit</MenuItem><MenuItem value="value">Override</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField></Grid><Grid size={{ xs: 12, sm: 3 }}><LibraryAutocomplete libraries={libraries} value={destinationLibraryId} onChange={setDestinationLibraryId} label="Destination" disabled={destinationSelection !== 'value'} /></Grid></Grid>{save.isError ? <Alert severity="warning">{save.error instanceof Error ? save.error.message : 'Could not save Source Group configuration.'}</Alert> : null}</Stack></DialogContent><DialogActions><Button onClick={() => setOpen(false)} disabled={save.isPending}>Cancel</Button><Button variant="contained" onClick={() => save.mutate()} disabled={save.isPending || (categorySelection === 'value' && !category) || (destinationSelection === 'value' && !destinationLibraryId)}>Save</Button></DialogActions></Dialog></>;
}

function sourceLogicalGroupMatches(group: AssetLogicalGroup, query: string) {
	if (!query) return true;
	return [group.name, group.path, group.relativePath, ...group.assetPaths.flatMap((path) => [path.name, path.path, path.relativePath, ...path.assets.flatMap((asset) => [asset.fileName, asset.path])])]
		.some((value) => value.toLowerCase().includes(query));
}

export function AssetCollectionRows({
  collection,
	logicalGroup,
	selectedAssetIds,
	onAssetSelectionChange,
  libraries,
  profiles,
  audioProfiles,
  trackProfiles,
  settings,
  assetCategories,
  queueJobs,
  runningSnapshotPaths,
  mode,
  columnCount,
}: {
  collection: AssetCollection;
	logicalGroup?: AssetLogicalGroup;
	selectedAssetIds?: Set<number>;
	onAssetSelectionChange?: (assetIds: number[], selected: boolean) => void;
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  trackProfiles: TrackProfile[];
  settings: AppSetting[];
  assetCategories: string[];
  queueJobs: QueueJob[];
  runningSnapshotPaths: Set<string>;
  mode: 'unprocessed' | 'library' | 'converted' | 'archive';
  columnCount: number;
}) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const profileAssignments = useQuery({ queryKey: ['profileAssignments'], queryFn: api.profileAssignments });
	const scopeConfigurations = useQuery({ queryKey: ['assetScopeConfigurations'], queryFn: api.assetScopeConfigurations });
  const [selectedPaths, setSelectedPaths] = useState<string[]>([]);
	const [logicalExpanded, setLogicalExpanded] = useState(false);
  const [configurationOpen, setConfigurationOpen] = useState(false);
  const [snapshotGroup, setSnapshotGroup] = useState<AssetGroup | null>(null);
  const pathVideoProfiles = profiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt);
  const pathAudioProfiles = audioProfiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt);
  const pathTrackProfiles = trackProfiles.filter((profile) => profile.scope === 'path' && !profile.disabled && !profile.deletedAt);
	const destinations = Object.fromEntries(collection.groups.map((group) => {
		const configuration = safeArray(scopeConfigurations.data).find((item) => item.scopeType === 'path' && normalizePath(item.scopeKey) === normalizePath(group.path));
		return [normalizePath(group.path), configuration?.destinationSelection === 'value' ? configuration.destinationLibraryId ?? 0 : 0];
	}));
  const selectedGroups = collection.groups.filter((group) => selectedPaths.includes(group.path));
  const allSelected = collection.groups.length > 0 && selectedPaths.length === collection.groups.length;
	const logicalAssets = logicalGroup?.assetPaths.flatMap((path) => path.assets).filter((asset) => !asset.missing && asset.id) ?? [];
	const allLogicalAssetsSelected = Boolean(selectedAssetIds && logicalAssets.length && logicalAssets.every((asset) => selectedAssetIds.has(asset.id ?? 0)));
	const someLogicalAssetsSelected = Boolean(selectedAssetIds && logicalAssets.some((asset) => selectedAssetIds.has(asset.id ?? 0)));
  const configuredPaths = collection.groups.filter((group) => pathConfigurationComplete(group, profileAssignments.data ?? [], destinations, mode)).length;
  const queueEligible = collection.groups.some((group) => pathCanQueue(group, mode));
  const [configuration, setConfiguration] = useState({
    category: '',
    videoProfileId: -1,
    audioProfileKey: '',
    trackProfileKey: '',
    destinationLibraryId: 0,
  });
  const [configurationFields, setConfigurationFields] = useState<string[]>([]);
	const [categoryMode, setCategoryMode] = useState<'inherit' | 'value' | 'disabled'>('inherit');
	const [destinationMode, setDestinationMode] = useState<'inherit' | 'value' | 'disabled'>('inherit');

  const saveConfiguration = useMutation({
    mutationFn: async () => {
	  if (logicalGroup) {
		const current = safeArray(scopeConfigurations.data).find((item) => item.scopeType === 'logical_group' && normalizePath(item.scopeKey) === normalizePath(logicalGroup.path));
		if (configurationFields.includes('category') || configurationFields.includes('destination')) {
			await api.updateAssetScopeConfiguration({ scopeType: 'logical_group', scopeKey: logicalGroup.path, categorySelection: configurationFields.includes('category') ? categoryMode : current?.categorySelection ?? 'inherit', category: configurationFields.includes('category') ? configuration.category : current?.category ?? '', destinationSelection: configurationFields.includes('destination') ? destinationMode : current?.destinationSelection ?? 'inherit', destinationLibraryId: configurationFields.includes('destination') ? configuration.destinationLibraryId : current?.destinationLibraryId });
		}
		for (const mediaType of ['video', 'audio', 'tracks'] as const) {
			if (!configurationFields.includes(mediaType)) continue;
			const value = mediaType === 'video' ? configuration.videoProfileId : mediaType === 'audio' ? configuration.audioProfileKey : configuration.trackProfileKey;
			const inherit = value === 0 || value === '__inherit__';
			const disabled = value === '' || (mediaType === 'video' && Number(value) < 0);
			await api.updateProfileAssignment({ targetType: 'logical_group', targetPath: logicalGroup.path, mediaType, selection: inherit ? 'inherit' : disabled ? 'disabled' : 'profile', videoProfileId: mediaType === 'video' ? Math.max(0, Number(value)) : 0, profileKey: mediaType === 'video' || inherit || disabled ? '' : String(value) });
		}
		return;
	  }
      if (!selectedGroups.length) throw new Error('Select at least one path.');
      for (const group of selectedGroups) {
        if (configurationFields.includes('category') || configurationFields.includes('destination')) {
			const current = safeArray(scopeConfigurations.data).find((item) => item.scopeType === 'path' && normalizePath(item.scopeKey) === normalizePath(group.path));
			await api.updateAssetScopeConfiguration({
				scopeType: 'path', scopeKey: group.path,
				categorySelection: configurationFields.includes('category') ? (configuration.category ? 'value' : 'disabled') : current?.categorySelection ?? 'inherit',
				category: configurationFields.includes('category') ? configuration.category : current?.category ?? '',
				destinationSelection: configurationFields.includes('destination') ? (configuration.destinationLibraryId ? 'value' : 'disabled') : current?.destinationSelection ?? 'inherit',
				destinationLibraryId: configurationFields.includes('destination') ? configuration.destinationLibraryId : current?.destinationLibraryId,
			});
        }
        if (configurationFields.includes('video')) {
          await api.updateProfileAssignment({
            targetType: 'path', targetPath: group.path, mediaType: 'video',
            selection: configuration.videoProfileId > 0 ? 'profile' : 'disabled',
            videoProfileId: Math.max(0, configuration.videoProfileId),
            profileKey: '',
          });
        }
        if (configurationFields.includes('audio')) {
          await api.updateProfileAssignment({
            targetType: 'path', targetPath: group.path, mediaType: 'audio',
            selection: configuration.audioProfileKey ? 'profile' : 'disabled',
            videoProfileId: 0,
            profileKey: configuration.audioProfileKey,
          });
        }
        if (configurationFields.includes('tracks')) {
          await api.updateProfileAssignment({
            targetType: 'path', targetPath: group.path, mediaType: 'tracks',
            selection: configuration.trackProfileKey ? 'profile' : 'disabled',
            videoProfileId: 0,
            profileKey: configuration.trackProfileKey,
          });
        }
      }
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
        queryClient.invalidateQueries({ queryKey: ['profileAssignments'] }),
		queryClient.invalidateQueries({ queryKey: ['assetScopeConfigurations'] }),
      ]);
      setConfigurationOpen(false);
    },
  });

  const queuePaths = useMutation({
    mutationFn: async () => {
      if (!selectedGroups.length) throw new Error('Select at least one path.');
      const blockedLifecycle = selectedGroups.filter((group) => !pathCanQueue(group, mode));
      if (blockedLifecycle.length) throw new Error(`These paths cannot be queued from ${mode}: ${blockedLifecycle.map(pathLabelForCollection).join(', ')}`);
      const missingDestination = selectedGroups.filter((group) => !effectivePathDestination(group, destinations, mode));
      if (missingDestination.length) throw new Error(`Select Destination for: ${missingDestination.map(pathLabelForCollection).join(', ')}`);
      const blockedAssets = selectedGroups.filter((group) => safeArray(group.assets).some((asset) => asset.review?.requiresReview || assetHasOpenJob(asset, queueJobs)));
      if (blockedAssets.length) throw new Error(`Review or active Queue jobs block: ${blockedAssets.map(pathLabelForCollection).join(', ')}`);

		const batches: Array<{ batchId: string; batchName: string; jobs: QueueJobInput[] }> = [];
      for (const group of selectedGroups) {
        const assignments = pathAssignmentsFor(group.path, profileAssignments.data ?? []);
        const video = assignments.get('video');
        const audio = assignments.get('audio');
        const tracks = assignments.get('tracks');
        const hasVideo = video?.selection === 'profile' && Boolean(video.videoProfileId);
        const hasAudio = audio?.selection === 'profile' && Boolean(audio.profileKey);
        const hasTracks = tracks?.selection === 'profile' && Boolean(tracks.profileKey);
        if (!hasVideo && !hasAudio && !hasTracks) throw new Error(`Select at least one profile for ${pathLabelForCollection(group)}.`);
        const fallbackProfileId = pathVideoProfiles[0]?.id ?? profiles.find((profile) => !profile.disabled && !profile.deletedAt)?.id ?? 0;
        const profileId = hasVideo ? video?.videoProfileId ?? 0 : fallbackProfileId;
        if (!profileId) throw new Error(`No usable video profile is available to build ${pathLabelForCollection(group)}.`);
        const destinationLibraryId = effectivePathDestination(group, destinations, mode);
        const assets = safeArray(group.assets).filter((asset) => !asset.missing);
        if (!assets.length) throw new Error(`${pathLabelForCollection(group)} has no available assets.`);
		const jobs = assets.map((asset): QueueJobInput => ({
            mediaPath: asset.path,
            publishMode: mode === 'library' ? 'replace_library_asset' : 'standard',
            libraryId: destinationLibraryId,
            profileId,
            audioProfileKey: hasAudio ? audio?.profileKey ?? '' : '',
            trackProfileKey: hasTracks ? tracks?.profileKey ?? '' : '',
            resolveProfileAssignments: true,
            processingMode: hasVideo ? 'full_encode' : 'audio_only',
            priority: 5,
            notes: queueNotes(`Queued from selected path: ${groupDisplayPath(group)}`, hasAudio ? audio?.profileKey ?? '' : ''),
		}));
		batches.push({ batchId: `path-${slugify(pathLabelForCollection(group))}-${Date.now()}-${batches.length}`, batchName: `${collection.title} · ${pathLabelForCollection(group)}`, jobs });
      }
		return Promise.all(batches.map((batch) => api.createQueueBatch(batch)));
    },
    onSuccess: async (responses) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
      ]);
		const firstBatch = responses[0];
		if (firstBatch) navigate(`/queue?batch=${encodeURIComponent(firstBatch.batchId)}`);
    },
  });

  function togglePath(path: string, selected: boolean) {
    setSelectedPaths((current) => selected ? [...new Set([...current, path])] : current.filter((candidate) => candidate !== path));
  }

  return (
    <Fragment>
      <TableRow sx={{ bgcolor: 'rgba(255,255,255,0.035)', '& td': { borderTop: 1, borderColor: 'divider' } }}>
        <TableCell colSpan={columnCount} sx={{ py: 1.25 }}>
          <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', md: 'center' }} spacing={1.25}>
            <Stack direction="row" spacing={1} alignItems="center" sx={{ minWidth: 0 }}>
              <Checkbox
                size="small"
				checked={selectedAssetIds ? allLogicalAssetsSelected : allSelected}
				indeterminate={selectedAssetIds ? someLogicalAssetsSelected && !allLogicalAssetsSelected : selectedPaths.length > 0 && !allSelected}
				onChange={(event) => selectedAssetIds
					? onAssetSelectionChange?.(logicalAssets.map((asset) => asset.id ?? 0), event.target.checked)
					: setSelectedPaths(event.target.checked ? collection.groups.map((group) => group.path) : [])}
                inputProps={{ 'aria-label': `Select all paths in ${collection.title}` }}
              />
              <Stack spacing={0.25} sx={{ minWidth: 0 }}>
				<Stack direction="row" spacing={0.5} alignItems="center"><IconButton size="small" onClick={() => setLogicalExpanded((current) => !current)}><ExpandMoreIcon sx={{ transform: logicalExpanded ? 'rotate(180deg)' : 'none' }} /></IconButton><Typography variant="h3" sx={{ overflowWrap: 'anywhere' }}>{collection.title}</Typography></Stack>
                <Typography variant="body2" color="text.secondary">
				  {logicalGroup?.assetCount ?? collection.fileCount} assets · {logicalGroup?.pathCount ?? collection.groups.length} paths · {formatBytes(logicalGroup?.totalSizeBytes ?? collection.sizeBytes)}
                </Typography>
              </Stack>
            </Stack>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems="stretch">
              <Button
                variant="outlined"
                startIcon={<EditIcon />}
				disabled={!logicalGroup && !selectedGroups.length}
                onClick={() => {
				  const scoped = logicalGroup ? safeArray(scopeConfigurations.data).find((item) => item.scopeType === 'logical_group' && normalizePath(item.scopeKey) === normalizePath(logicalGroup.path)) : undefined;
				  const scopedAssignments = logicalGroup ? safeArray(profileAssignments.data).filter((item) => item.targetType === 'logical_group' && normalizePath(item.targetPath) === normalizePath(logicalGroup.path)) : [];
				  const video = scopedAssignments.find((item) => item.mediaType === 'video'); const audio = scopedAssignments.find((item) => item.mediaType === 'audio'); const tracks = scopedAssignments.find((item) => item.mediaType === 'tracks');
				  setCategoryMode(scoped?.categorySelection ?? 'inherit'); setDestinationMode(scoped?.destinationSelection ?? 'inherit');
				  setConfiguration(logicalGroup ? { category: scoped?.category ?? '', videoProfileId: video?.selection === 'profile' ? video.videoProfileId ?? 0 : video?.selection === 'disabled' ? -1 : 0, audioProfileKey: audio?.selection === 'profile' ? audio.profileKey ?? '' : audio?.selection === 'disabled' ? '' : '__inherit__', trackProfileKey: tracks?.selection === 'profile' ? tracks.profileKey ?? '' : tracks?.selection === 'disabled' ? '' : '__inherit__', destinationLibraryId: scoped?.destinationLibraryId ?? 0 } : configurationForPathSelection(selectedGroups, profileAssignments.data ?? [], destinations, mode));
                  setConfigurationFields([]);
                  setConfigurationOpen(true);
                }}
              >
				{logicalGroup ? 'Configure' : `Configure paths · ${configuredPaths}/${collection.groups.length}`}
              </Button>
			  {queueEligible && !logicalGroup ? (
                <Button
                  variant="contained"
                  startIcon={<PlaylistAddIcon />}
                  disabled={!selectedGroups.length || queuePaths.isPending}
                  onClick={() => queuePaths.mutate()}
                >
                  Queue selected paths
                </Button>
              ) : null}
            </Stack>
          </Stack>
          {queuePaths.isError ? <Alert severity="warning" sx={{ mt: 1 }}>{queuePaths.error instanceof Error ? queuePaths.error.message : 'Could not queue selected paths.'}</Alert> : null}
        </TableCell>
      </TableRow>
	  {logicalExpanded ? collection.groups.map((group) => {
		const isRoot = logicalGroup?.assetPaths.find((path) => normalizePath(path.path) === normalizePath(group.path))?.isLogicalGroupRoot ?? false;
		return (
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
          pathSelected={selectedPaths.includes(group.path)}
          onPathSelectionChange={(selected) => togglePath(group.path, selected)}
          onOpenPathSnapshots={() => setSnapshotGroup(group)}
		  forceExpanded={isRoot}
		  hidePathHeader={isRoot}
		  selectedAssetIds={selectedAssetIds}
		  onAssetSelectionChange={onAssetSelectionChange}
        />
		);
	  }) : null}
      <Dialog open={configurationOpen} onClose={() => !saveConfiguration.isPending && setConfigurationOpen(false)} maxWidth="md" fullWidth>
		<DialogTitle>Configure {logicalGroup ? `title · ${collection.title}` : `${selectedGroups.length} path${selectedGroups.length === 1 ? '' : 's'} · ${collection.title}`}</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={2} sx={{ pt: 0.5 }}>
			<Alert severity="info">{logicalGroup ? 'These values apply at Logical Group scope. Path and Asset overrides continue to take precedence.' : 'These values are assigned to every selected path and inherited by its assets. Asset overrides continue to take precedence.'}</Alert>
			{logicalGroup ? null : <Typography variant="body2" color="text.secondary">{selectedGroups.map(pathLabelForCollection).join(' · ')}</Typography>}
            <Grid container spacing={2}>
			  {logicalGroup ? <Grid size={{ xs: 12, sm: 3 }}><TextField select fullWidth label="Category mode" value={categoryMode} onChange={(event) => { setCategoryMode(event.target.value as typeof categoryMode); setConfigurationFields((current) => [...new Set([...current, 'category'])]); }}><MenuItem value="inherit">Inherit</MenuItem><MenuItem value="value">Override</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField></Grid> : null}
			  <Grid size={{ xs: 12, sm: logicalGroup ? 3 : 6 }}><AssetCategorySelect value={configuration.category} options={assetCategories} onChange={(category) => { setConfiguration((current) => ({ ...current, category })); setConfigurationFields((current) => [...new Set([...current, 'category'])]); }} label="Category" disabled={Boolean(logicalGroup && categoryMode !== 'value')} /></Grid>
			  {logicalGroup ? <Grid size={{ xs: 12, sm: 3 }}><TextField select fullWidth label="Destination mode" value={destinationMode} onChange={(event) => { setDestinationMode(event.target.value as typeof destinationMode); setConfigurationFields((current) => [...new Set([...current, 'destination'])]); }}><MenuItem value="inherit">Inherit</MenuItem><MenuItem value="value">Override</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField></Grid> : null}
			  <Grid size={{ xs: 12, sm: logicalGroup ? 3 : 6 }}><LibraryAutocomplete libraries={libraries} value={configuration.destinationLibraryId} onChange={(destinationLibraryId) => { setConfiguration((current) => ({ ...current, destinationLibraryId })); setConfigurationFields((current) => [...new Set([...current, 'destination'])]); }} label="Destination" disabled={Boolean(logicalGroup && destinationMode !== 'value')} /></Grid>
			  <Grid size={{ xs: 12, md: 4 }}><ProfileAutocomplete profiles={pathVideoProfiles} value={configuration.videoProfileId} onChange={(videoProfileId) => { setConfiguration((current) => ({ ...current, videoProfileId })); setConfigurationFields((current) => [...new Set([...current, 'video'])]); }} label="Video profile" allowNone allowInherit={Boolean(logicalGroup)} /></Grid>
			  <Grid size={{ xs: 12, md: 4 }}><AudioProfileAutocomplete profiles={pathAudioProfiles} value={configuration.audioProfileKey} onChange={(audioProfileKey) => { setConfiguration((current) => ({ ...current, audioProfileKey })); setConfigurationFields((current) => [...new Set([...current, 'audio'])]); }} label="Audio profile" allowInherit={Boolean(logicalGroup)} /></Grid>
			  <Grid size={{ xs: 12, md: 4 }}><TrackProfileAutocomplete profiles={pathTrackProfiles} value={configuration.trackProfileKey} onChange={(trackProfileKey) => { setConfiguration((current) => ({ ...current, trackProfileKey })); setConfigurationFields((current) => [...new Set([...current, 'tracks'])]); }} label="Tracks profile" allowInherit={Boolean(logicalGroup)} /></Grid>
            </Grid>
            {saveConfiguration.isError ? <Alert severity="warning">{saveConfiguration.error instanceof Error ? saveConfiguration.error.message : 'Could not configure selected paths.'}</Alert> : null}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfigurationOpen(false)} disabled={saveConfiguration.isPending}>Cancel</Button>
		  <Button variant="contained" onClick={() => saveConfiguration.mutate()} disabled={saveConfiguration.isPending || !configurationFields.length || Boolean(logicalGroup && categoryMode === 'value' && !configuration.category) || Boolean(logicalGroup && destinationMode === 'value' && !configuration.destinationLibraryId)}>{saveConfiguration.isPending ? 'Saving…' : logicalGroup ? 'Apply to title' : 'Apply to selected paths'}</Button>
        </DialogActions>
      </Dialog>
      {snapshotGroup ? (
        <PathSnapshotsDialog
          open
          group={snapshotGroup}
          runningSnapshotPaths={runningSnapshotPaths}
          onClose={() => setSnapshotGroup(null)}
        />
      ) : null}
    </Fragment>
  );
}

function PathSnapshotsDialog({
  open,
  group,
  runningSnapshotPaths,
  onClose,
}: {
  open: boolean;
  group: AssetGroup;
  runningSnapshotPaths: Set<string>;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const assets = safeArray(group.assets).filter((asset) => !asset.missing);
  const [selectedPaths, setSelectedPaths] = useState<string[]>([]);
  const [activePaths, setActivePaths] = useState<string[]>([]);
  const snapshotQueries = useQueries({
    queries: assets.map((asset) => ({
      queryKey: ['assetSnapshot', asset.path],
      queryFn: () => api.latestSnapshot(asset.path),
      enabled: open,
      staleTime: 15_000,
    })),
  });
  const selectableAssets = assets.filter((asset) => !runningSnapshotPaths.has(asset.path) && !activePaths.includes(asset.path));
  const allSelected = selectableAssets.length > 0 && selectableAssets.every((asset) => selectedPaths.includes(asset.path));
  const generateSnapshots = useMutation({
    mutationFn: async (paths: string[]) => {
      for (const path of paths) {
        setActivePaths((current) => [...new Set([...current, path])]);
        try {
          let operation = await api.startSnapshotOperation({ path, force: true, analysisSeconds: 20 });
          while (operation.status === 'running') {
            await new Promise((resolve) => window.setTimeout(resolve, 1000));
            operation = await api.snapshotOperation(operation.id);
          }
          if (operation.status !== 'completed') throw new Error(operation.error || `Snapshot failed for ${path}.`);
          await queryClient.invalidateQueries({ queryKey: ['assetSnapshot', path] });
        } finally {
          setActivePaths((current) => current.filter((candidate) => candidate !== path));
        }
      }
    },
    onSuccess: async () => {
      setSelectedPaths([]);
      await queryClient.invalidateQueries({ queryKey: ['snapshotOperations'] });
    },
  });

  return (
    <Dialog open={open} onClose={() => !generateSnapshots.isPending && onClose()} maxWidth="lg" fullWidth>
      <DialogTitle>Snapshots · {pathLabelForCollection(group)}</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={1.5}>
          <Alert severity="info">Opening this list only reads snapshot state. Analysis starts only when you choose Generate.</Alert>
          <Box sx={{ overflowX: 'auto' }}>
            <Table size="small" sx={{ minWidth: 760 }}>
              <TableHead>
                <TableRow>
                  <TableCell padding="checkbox">
                    <Checkbox
                      size="small"
                      checked={allSelected}
                      indeterminate={selectedPaths.length > 0 && !allSelected}
                      disabled={!selectableAssets.length}
                      onChange={(event) => setSelectedPaths(event.target.checked ? selectableAssets.map((asset) => asset.path) : [])}
                      inputProps={{ 'aria-label': `Select all assets in ${group.path} for snapshot generation` }}
                    />
                  </TableCell>
                  <TableCell>Asset</TableCell>
                  <TableCell>Snapshot date</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell align="right">Action</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {assets.map((asset, index) => {
                  const query = snapshotQueries[index];
                  const state = query.data;
                  const running = runningSnapshotPaths.has(asset.path) || activePaths.includes(asset.path);
                  return (
                    <TableRow key={asset.path} hover>
                      <TableCell padding="checkbox"><Checkbox size="small" checked={selectedPaths.includes(asset.path)} disabled={running} onChange={(event) => setSelectedPaths((current) => event.target.checked ? [...new Set([...current, asset.path])] : current.filter((path) => path !== asset.path))} /></TableCell>
                      <TableCell><Typography fontWeight={700}>{asset.fileName}</Typography><Typography variant="caption" color="text.secondary" sx={{ overflowWrap: 'anywhere' }}>{asset.path}</Typography></TableCell>
                      <TableCell>{state?.snapshot?.createdAt ? formatDate(state.snapshot.createdAt) : '—'}</TableCell>
                      <TableCell>{query.isLoading ? <Chip size="small" label="Checking…" /> : <SnapshotStateChip status={running ? 'running' : state?.status ?? 'unavailable'} />}</TableCell>
                      <TableCell align="right"><Button size="small" variant="outlined" disabled={running || generateSnapshots.isPending} onClick={() => generateSnapshots.mutate([asset.path])}>{running ? 'Generating…' : 'Generate'}</Button></TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </Box>
          {generateSnapshots.isPending ? <LinearProgress /> : null}
          {generateSnapshots.isError ? <Alert severity="warning">{generateSnapshots.error instanceof Error ? generateSnapshots.error.message : 'Snapshot generation failed.'}</Alert> : null}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={generateSnapshots.isPending}>Close</Button>
        <Button variant="contained" startIcon={<ManageSearchIcon />} disabled={!selectedPaths.length || generateSnapshots.isPending} onClick={() => generateSnapshots.mutate(selectedPaths)}>Generate selected ({selectedPaths.length})</Button>
      </DialogActions>
    </Dialog>
  );
}

function SnapshotStateChip({ status }: { status: 'current' | 'missing' | 'changed' | 'stale' | 'legacy' | 'unavailable' | 'running' }) {
  const labels = { current: 'Current', missing: 'Missing', changed: 'Changed', stale: 'Stale', legacy: 'Incomplete', unavailable: 'Corrupt / unavailable', running: 'Running' };
  const color = status === 'current' ? 'success' : status === 'running' ? 'primary' : status === 'missing' || status === 'unavailable' ? 'default' : 'warning';
  return <Chip size="small" label={labels[status]} color={color} />;
}

function ArchivedAssetTable({
  groups,
  libraries,
  profiles,
  audioProfiles,
  trackProfiles,
  assetCategories,
  queueJobs,
  runningSnapshotPaths,
  query,
  emptyLabel,
}: {
  groups: AssetGroup[];
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  trackProfiles: TrackProfile[];
  assetCategories: string[];
  queueJobs: QueueJob[];
  runningSnapshotPaths: Set<string>;
  query: string;
  emptyLabel: string;
}) {
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const assets = archiveAssetsFromGroups(groups, query);
  const safePage = Math.min(page, Math.max(0, Math.ceil(assets.length / rowsPerPage) - 1));
  const pagedAssets = assets.slice(safePage * rowsPerPage, safePage * rowsPerPage + rowsPerPage);

  if (sumGroupFiles(groups) === 0) {
    return (
      <CardContent>
        <Alert severity="info">{emptyLabel}</Alert>
      </CardContent>
    );
  }

  return (
    <>
      <Box sx={{ overflowX: 'auto', borderTop: 1, borderColor: 'divider' }}>
        <Table size="small" sx={{ minWidth: 820, tableLayout: 'fixed', '& td, & th': { py: 0.85 } }}>
          <TableHead>
            <TableRow>
              <TableCell sx={{ width: 330 }}>Asset</TableCell>
              <TableCell sx={{ width: 190 }}>Status</TableCell>
              <TableCell sx={{ width: 110 }}>Size</TableCell>
              <TableCell sx={{ width: 150 }}>Modified</TableCell>
              <TableCell align="center" sx={{ width: 260 }}>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {pagedAssets.length ? pagedAssets.map(({ asset, group }) => (
              <AssetRow
                key={`${asset.status}-${asset.libraryId}-${asset.path}`}
                asset={asset}
                libraries={libraries}
                profiles={profiles}
                audioProfiles={audioProfiles}
                trackProfiles={trackProfiles}
                assetCategories={assetCategories}
                groupRelativePath={group.relativePath}
                groupPath={group.path}
                groupCategory=""
                confidenceEnabled
                groupProfileId={0}
                groupAudioProfileKey=""
                groupLibraryId={group.libraryId}
                hasOpenJob={assetHasOpenJob(asset, queueJobs)}
                queueJobs={queueJobs}
                snapshotRunning={runningSnapshotPaths.has(asset.path)}
                mode="archive"
                bulkSelected={false}
                bulkSelectionEnabled={false}
                bulkSelectionDisabled
                onBulkSelectionChange={() => undefined}
                archivePathAssets={safeArray(group.assets).filter((candidate) => !candidate.missing)}
              />
            )) : (
              <TableRow>
                <TableCell colSpan={5}>
                  <Alert severity="info">No archived assets match this search.</Alert>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </Box>
      <TablePagination
        component="div"
        count={assets.length}
        page={safePage}
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

// Retained for compatibility with older local builds that imported this view.
void ArchivedAssetTable;

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
  pathSelected = false,
  onPathSelectionChange,
  onOpenPathSnapshots,
  collectionManaged = false,
	forceExpanded = false,
	hidePathHeader = false,
	selectedAssetIds,
	onAssetSelectionChange,
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
  pathSelected?: boolean;
  onPathSelectionChange?: (selected: boolean) => void;
  onOpenPathSnapshots?: () => void;
  collectionManaged?: boolean;
	forceExpanded?: boolean;
	hidePathHeader?: boolean;
	selectedAssetIds?: Set<number>;
	onAssetSelectionChange?: (assetIds: number[], selected: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const pendingAssetTrackSaves = useIsMutating({ mutationKey: ['assetTrackSelection', normalizePath(group.path)] });
  const profileAssignments = useQuery({ queryKey: ['profileAssignments'], queryFn: api.profileAssignments });
	const pathScopeConfiguration = useQuery({
		queryKey: ['assetScopeConfigurations', 'path', normalizePath(group.path)],
		queryFn: async () => (await api.assetScopeConfigurations()).find((item) => item.scopeType === 'path' && normalizePath(item.scopeKey) === normalizePath(group.path)),
	});
  const navigate = useNavigate();
  const [expanded, setExpanded] = useState(false);
  const pathVideoProfiles = profiles.filter((profile) => profile.scope === 'path');
  const pathAudioProfiles = audioProfiles.filter((profile) => profile.scope === 'path');
  const pathTrackProfiles = trackProfiles.filter((profile) => profile.scope === 'path');
	const firstPathVideoProfileId = pathVideoProfiles[0]?.id ?? 0;
  const [selectedProfileId, setSelectedProfileId] = useState<number>(pathVideoProfiles[0]?.id ?? 0);
  const [selectedAudioProfileKey, setSelectedAudioProfileKey] = useState<string>('');
  const pathTrackAssignments = getTrackProfilePathAssignments(settings);
  const [selectedTrackProfileKey, setSelectedTrackProfileKey] = useState<string>(pathTrackAssignments[normalizePath(group.path)] ?? '');
  const [selectedAssetPaths, setSelectedAssetPaths] = useState<string[]>([]);
  const groupAssets = safeArray(group.assets);
  const pathMetadata = group.pathMetadata ?? { categories: [], tags: [], updatedAt: '' };
  const inheritedPathCategory = firstCategory(pathMetadata.categories);
  const groupReview = group.review ?? { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' };
	const configuredPathDestination = pathScopeConfiguration.data?.destinationSelection === 'value'
		? pathScopeConfiguration.data.destinationLibraryId ?? 0
		: (mode === 'library' ? group.libraryId : 0);
  const [selectedLibraryId, setSelectedLibraryId] = useState<number>(configuredPathDestination);
  const [migrationLibraryId, setMigrationLibraryId] = useState<number>(0);
  const [groupCategory, setGroupCategory] = useState<string>(inheritedPathCategory);
  const [pathAdvisorOpen, setPathAdvisorOpen] = useState(false);
  const [pathAdvisorCurrent, setPathAdvisorCurrent] = useState('');
  const [pathAdvisorResults, setPathAdvisorResults] = useState<Array<{
    asset: Asset;
    response?: AdvisorResponse;
    error?: string;
  }>>([]);
  const effectiveProfileId =
  selectedProfileId < 0
    ? 0
    : selectedProfileId || pathVideoProfiles[0]?.id || 0;
  const representativeAsset = firstAssetForGroup(groupAssets.filter((asset) => !asset.missing));
  const isConvertedGroup = group.status === 'converted';
  const isPublishedAsIsGroup = group.status === 'published_as_is' || (groupAssets.length > 0 && groupAssets.every((asset) => asset.publicationMode === 'as_is'));
  const isAcceptedGroup = group.status === 'accepted';
  const isLibraryGroup = group.status === 'unverified' || group.status === 'accepted' || group.status === 'library' || group.status === 'published_as_is';
  const isArchiveGroup = mode === 'archive' || group.status === 'archive';
  const isReadOnlyGroup = isConvertedGroup || isPublishedAsIsGroup || isAcceptedGroup || isArchiveGroup;
  const showConfidenceColumn = mode !== 'archive' && mode !== 'converted';
  const bulkSelectableAssets = isReadOnlyGroup && !isAcceptedGroup ? groupAssets.filter((asset) => !asset.missing) : [];
  const hasMultipleSelectableAssets = bulkSelectableAssets.length > 1;
	const hierarchicalSelectableAssets = selectedAssetIds ? groupAssets.filter((asset) => !asset.missing && asset.id) : bulkSelectableAssets;
	const allBulkAssetsSelected = hierarchicalSelectableAssets.length > 0 && hierarchicalSelectableAssets.every((asset) => selectedAssetIds ? selectedAssetIds.has(asset.id ?? 0) : selectedAssetPaths.includes(asset.path));
	const someHierarchicalAssetsSelected = selectedAssetIds ? hierarchicalSelectableAssets.some((asset) => selectedAssetIds.has(asset.id ?? 0)) : selectedAssetPaths.length > 0;
  const disabledConfidencePaths = getDisabledConfidencePaths(settings);
  const isConfidenceEnabled = !disabledConfidencePaths.includes(group.path);
  const updateSetting = useMutation({
    mutationFn: api.updateSetting,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });
  const updateProfileAssignment = useMutation({
    mutationFn: api.updateProfileAssignment,
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ['profileAssignments'] }),
  });
  useEffect(() => {
    if (!profileAssignments.data) return;
	const assignments = profileAssignments.data.filter((assignment) => assignment.targetType === 'path' && normalizePath(assignment.targetPath) === normalizePath(group.path));
	const video = assignments.find((assignment) => assignment.mediaType === 'video');
	const audio = assignments.find((assignment) => assignment.mediaType === 'audio');
	const tracks = assignments.find((assignment) => assignment.mediaType === 'tracks');


  
  setSelectedProfileId(
    video
      ? video.selection === 'override_only'
        ? VIDEO_PROFILE_OVERRIDE_ONLY
        : video.selection === 'audio_only' || video.selection === 'disabled'
          ? VIDEO_PROFILE_AUDIO_ONLY
          : video.videoProfileId || 0
      : 0
  );
  setSelectedAudioProfileKey(audio?.selection === 'profile' ? audio.profileKey ?? '' : '');
   setSelectedTrackProfileKey(tracks?.selection === 'profile' ? tracks.profileKey ?? '' : '');
	}, [profileAssignments.data, group.path, firstPathVideoProfileId]);
  const updateMetadata = useMutation({
    mutationFn: api.updateAssetMetadata,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });

  useEffect(() => {
    setGroupCategory(inheritedPathCategory);
  }, [inheritedPathCategory]);
  useEffect(() => {
    setSelectedLibraryId(configuredPathDestination);
  }, [configuredPathDestination, group.path]);
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
  const runPathAdvisor = useMutation({
    mutationFn: async () => {
      const candidates = groupAssets.filter((asset) => !asset.missing);
      setPathAdvisorResults([]);
      for (const asset of candidates) {
        setPathAdvisorCurrent(asset.path);
        try {
          const response = await api.evaluateAdvisor({ mediaPath: asset.path, profileId: effectiveProfileId });
          queryClient.setQueryData(['advisor', 'asset-row', asset.path, effectiveProfileId], response);
          setPathAdvisorResults((current) => [...current, { asset, response }]);
        } catch (error) {
          setPathAdvisorResults((current) => [...current, {
            asset,
            error: error instanceof Error ? error.message : 'Advisor evaluation failed.',
          }]);
        }
      }
      setPathAdvisorCurrent('');
    },
  });
  const queueGroup = useMutation({
    mutationFn: async () => {
      const trackProfile = trackProfiles.find((profile) => profile.key === selectedTrackProfileKey);
      const hasSelectedOperation = selectedProfileId > 0 || Boolean(selectedAudioProfileKey) || Boolean(trackProfile);
      const queueProfileId = effectiveProfileId || (hasSelectedOperation ? pathVideoProfiles[0]?.id ?? profiles[0]?.id ?? 0 : 0);
      const copyVideo = selectedProfileId < 0;
      if (!queueProfileId || !selectedLibraryId) {
          throw new Error('A video profile and destination library are required to queue this folder.');
      }

      const batchId = createBatchId(group);
      const batchName = groupDisplayPath(group);
	  const validations = await Promise.all(groupAssets.map(async (asset) => {
        const assignment = profileAssignments.data?.find((candidate) => candidate.targetType === 'asset' && candidate.mediaType === 'tracks' && normalizePath(candidate.targetPath) === normalizePath(asset.path));
        const effectiveTrack = assignment?.selection === 'disabled' ? undefined : assignment?.selection === 'profile' ? trackProfiles.find((profile) => profile.key === assignment.profileKey) : trackProfile;
        return { asset, profile: effectiveTrack, result: effectiveTrack ? validateTrackProfile(effectiveTrack, await api.scan({ path: asset.path, force: false })) : { applies: true, reasons: [] as string[] } };
      }));
	  const incompatible = validations.filter(({ result }) => !result.applies);
	  if (incompatible.some(({ profile }) => profile?.validationMode === 'block')) {
	    throw new Error(`Track profile blocked this path: ${incompatible.map(({ asset, result }) => `${asset.fileName}: ${result.reasons.join(', ')}`).join(' · ')}`);
	  }
	  await Promise.all(incompatible.filter(({ asset, profile }) => profile?.validationMode === 'review' && !assetReviewApproved(asset)).map(({ asset, result }) => api.updateAssetReview({ path: asset.path, requiresReview: true, source: 'track-profile', reason: result.reasons.join('; '), tags: ['track-profile-incompatible'] })));
	  const queueable = validations.filter(({ asset, profile, result }) => result.applies || profile?.validationMode === 'warn' || (profile?.validationMode === 'review' && assetReviewApproved(asset)));
	  if (queueable.length === 0) {
      throw new Error('No assets in this path are eligible for queueing.');
    }

    const batchJobs: QueueJobInput[] = queueable.map(
      ({ asset, profile: effectiveTrack, result }) => ({
        mediaPath: asset.path,
        publishMode: isLibraryGroup
          ? 'replace_library_asset'
          : 'standard',

        libraryId: isLibraryGroup
          ? asset.libraryId
          : selectedLibraryId,

        profileId: queueProfileId,
        audioProfileKey: selectedAudioProfileKey,
        trackProfileKey: effectiveTrack?.key ?? '',
        resolveProfileAssignments: true,
        processingMode: copyVideo
          ? 'audio_only'
          : 'full_encode',
        priority: 5,

        notes: queueNotes(
          `Queued from folder: ${batchName}${
            result.applies
              ? ''
              : `\nTrack profile ${effectiveTrack?.key} did not apply: ${result.reasons.join('; ')}`
          }`,
          selectedAudioProfileKey,
        ),
      }),
    );

    return api.createQueueBatch({
      batchId,
      batchName,
      jobs: batchJobs,
    });
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
    if (!selectedProfileId && pathVideoProfiles[0]) {
      setSelectedProfileId(pathVideoProfiles[0].id);
    }
    setExpanded((current) => !current);
  }

  function startPathAdvisor() {
    setPathAdvisorOpen(true);
    runPathAdvisor.mutate();
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
	updateProfileAssignment.mutate({ targetType: 'path', targetPath: group.path, mediaType: 'tracks', selection: key ? 'profile' : 'disabled', videoProfileId: 0, profileKey: key });
  }

  function selectPathVideoProfile(id: number) {
    setSelectedProfileId(id);
    updateProfileAssignment.mutate({ targetType: 'path', targetPath: group.path, mediaType: 'video', selection: id > 0 ? 'profile' : 'disabled', videoProfileId: Math.max(0, id), profileKey: '' });
  }

  function selectPathAudioProfile(key: string) {
    setSelectedAudioProfileKey(key);
    updateProfileAssignment.mutate({ targetType: 'path', targetPath: group.path, mediaType: 'audio', selection: key ? 'profile' : 'disabled', videoProfileId: 0, profileKey: key });
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
	  {!hidePathHeader ? <TableRow
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
          <Stack direction="row" spacing={0.75} alignItems="center" sx={{ minWidth: 0 }}>
			{collectionManaged || selectedAssetIds ? (
              <Checkbox
                size="small"
				checked={selectedAssetIds ? allBulkAssetsSelected : pathSelected}
				indeterminate={selectedAssetIds ? someHierarchicalAssetsSelected && !allBulkAssetsSelected : false}
                onClick={(event) => event.stopPropagation()}
				onChange={(event) => selectedAssetIds
					? onAssetSelectionChange?.(hierarchicalSelectableAssets.map((asset) => asset.id ?? 0), event.target.checked)
					: onPathSelectionChange?.(event.target.checked)}
                inputProps={{ 'aria-label': `Select path ${group.path}` }}
              />
            ) : null}
            <Stack spacing={0.25} sx={{ minWidth: 0 }}>
              <Typography fontWeight={700} sx={{ wordBreak: 'break-word', lineHeight: 1.25 }}>
                {pathLabelForCollection(group)}
              </Typography>
              <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all', lineHeight: 1.25 }}>
                {cleanPathLabel(group.path)}
              </Typography>
            </Stack>
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
          <Stack direction="row" alignItems="center" justifyContent="center" spacing={0.25}>
            {onOpenPathSnapshots ? (
              <Tooltip title="Review and generate snapshots for this path">
                <IconButton
                  size="small"
                  color="primary"
                  onClick={(event) => { event.stopPropagation(); onOpenPathSnapshots(); }}
                  aria-label={`Snapshots for ${group.path}`}
                >
                  <ManageSearchIcon fontSize="small" />
                </IconButton>
              </Tooltip>
            ) : null}
            <ExpandMoreIcon
              aria-hidden
              sx={{
                color: 'text.secondary',
                transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
                transition: (theme) => theme.transitions.create('transform', { duration: theme.transitions.duration.shortest }),
              }}
            />
          </Stack>
        </TableCell>
	  </TableRow> : null}
      <TableRow>
		<TableCell colSpan={showConfidenceColumn ? 8 : 7} sx={{ p: 0, borderBottom: (forceExpanded || expanded) ? 1 : 0, borderColor: 'divider', maxWidth: 0 }}>
		  <Collapse in={forceExpanded || expanded} timeout="auto" unmountOnExit>
            <Box sx={{ bgcolor: 'rgba(255,255,255,0.02)', px: { xs: 1.5, md: 2 }, py: 2, width: '100%', maxWidth: '100%', overflow: 'hidden' }}>
              <Stack spacing={2}>
                {collectionManaged ? null : isArchiveGroup ? null : isConvertedGroup ? null : isPublishedAsIsGroup ? (
                  <Alert severity="info">
                    Published as-is assets must be returned to Raw before they can be queued for conversion.
                  </Alert>
                ) : isAcceptedGroup ? (
                  <Alert severity="success">
                    These assets were explicitly accepted as-is. Reconsider an asset before queueing it for conversion.
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
					  <ProfileAutocomplete profiles={pathVideoProfiles} value={selectedProfileId < 0 ? -1 : effectiveProfileId} onChange={selectPathVideoProfile} label="Video profile · Path" size="small" allowNone disabled={profileAssignments.isLoading || updateProfileAssignment.isPending} />
                    </Grid>
                    <Grid size={{ xs: 12, md: 2 }}>
                      <AudioProfileAutocomplete
                        profiles={pathAudioProfiles}
                        value={selectedAudioProfileKey}
                        onChange={selectPathAudioProfile}
                        label="Audio profile · Path"
                        size="small"
						disabled={profileAssignments.isLoading || updateProfileAssignment.isPending}
                      />
                    </Grid>
                    <Grid size={{ xs: 12, md: 2 }}>
					  <TrackProfileAutocomplete profiles={pathTrackProfiles} value={selectedTrackProfileKey} onChange={selectPathTrackProfile} disabled={profileAssignments.isLoading || updateProfileAssignment.isPending} label="Tracks · Path" />
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
                          profileAssignments.isLoading || queueGroup.isPending || updateProfileAssignment.isPending || pendingAssetTrackSaves > 0 ||
                          (!effectiveProfileId && !selectedAudioProfileKey && !selectedTrackProfileKey) ||
                          !selectedLibraryId ||
                          groupAssets.length === 0 ||
                          groupAssets.some((asset) => asset.review?.requiresReview || assetHasOpenJob(asset, queueJobs))
                        }
                        fullWidth
                        sx={{ minHeight: 40, alignSelf: 'center' }}
                      >
                        {pendingAssetTrackSaves > 0 ? 'Saving Tracks…' : 'Queue Folder'}
                      </Button>
                    </Grid>
                  </Grid>
                )}
                {!isReadOnlyGroup && !isConfidenceEnabled ? (
                  <Alert severity="warning">
                    Confidence is off for this path. Advisor checks and any future confidence-based automation will be skipped here; manual queueing still works.
                  </Alert>
                ) : null}
                {!isReadOnlyGroup && mode !== 'unprocessed' && !isLibraryGroup ? (
                  <Stack direction="row" justifyContent="flex-end">
                    <Button
                      variant="outlined"
                      startIcon={<InfoOutlinedIcon />}
                      onClick={startPathAdvisor}
                      disabled={!isConfidenceEnabled || effectiveProfileId <= 0 || groupAssets.every((asset) => asset.missing) || runPathAdvisor.isPending}
                      sx={{ width: { xs: '100%', sm: 'auto' } }}
                    >
                      Run Advisor on path
                    </Button>
                  </Stack>
                ) : null}
                {groupReview.requiresReview ? (
                  <Alert severity="warning">
                    Folder queue is blocked because at least one asset in this path needs review. You can still queue approved assets individually.
                  </Alert>
                ) : null}
                {!isReadOnlyGroup && advisor.isError && representativeAsset ? <Alert severity="warning">Could not evaluate this path: {advisor.error instanceof Error ? advisor.error.message : 'unknown error'}</Alert> : null}
                {!representativeAsset && groupAssets.length > 0 ? <Alert severity="warning">This path has no physically available asset to evaluate. Run Sync Assets after restoring or removing stale records.</Alert> : null}
                {queueGroup.isSuccess ? (
                  <Alert severity="success">
                    {queueGroup.data.jobs.length} files queued atomically from this folder.
                  </Alert>
                ) : null}

                {queueGroup.isError ? <Alert severity="warning">{queueGroup.error instanceof Error ? queueGroup.error.message : 'Could not queue this folder.'}</Alert> : null}
                {mode === 'unprocessed' && !isReadOnlyGroup ? (
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} justifyContent="flex-end" alignItems={{ xs: 'stretch', sm: 'center' }}>
                    <Typography color="text.secondary" variant="body2">
                      No conversion required: validate and move these original files directly to the selected Library.
                    </Typography>
                    <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
                      <Button
                        variant="outlined"
                        size="small"
                        startIcon={<InfoOutlinedIcon />}
                        onClick={startPathAdvisor}
                        disabled={!isConfidenceEnabled || effectiveProfileId <= 0 || groupAssets.every((asset) => asset.missing) || runPathAdvisor.isPending}
                        sx={{ minHeight: 40, whiteSpace: 'nowrap' }}
                      >
                        Run Advisor on path
                      </Button>
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
                  </Stack>
                ) : null}
                {publishAsIs.isSuccess ? <Alert severity="success">{publishAsIs.data.message}</Alert> : null}
                {publishAsIs.isError ? <Alert severity="warning">{publishAsIs.error instanceof Error ? publishAsIs.error.message : 'Direct publication failed.'}</Alert> : null}
                {isLibraryGroup ? (
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} justifyContent="flex-end" alignItems={{ xs: 'stretch', sm: 'center' }}>
                    {!isReadOnlyGroup ? (
                      <Button
                        variant="outlined"
                        size="small"
                        startIcon={<InfoOutlinedIcon />}
                        onClick={startPathAdvisor}
                        disabled={!isConfidenceEnabled || effectiveProfileId <= 0 || groupAssets.every((asset) => asset.missing) || runPathAdvisor.isPending}
                        sx={{ minHeight: 40, whiteSpace: 'nowrap' }}
                      >
                        Run Advisor on path
                      </Button>
                    ) : null}
                    {migrationControls}
                  </Stack>
                ) : null}
						{(selectedAssetIds || (isReadOnlyGroup && hasMultipleSelectableAssets)) ? (
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
							  indeterminate={someHierarchicalAssetsSelected && !allBulkAssetsSelected}
                              disabled={!bulkSelectableAssets.length || bulkAssetAction.isPending}
						  onChange={(event) => selectedAssetIds
								? onAssetSelectionChange?.(hierarchicalSelectableAssets.map((asset) => asset.id ?? 0), event.target.checked)
								: setSelectedAssetPaths(event.target.checked ? bulkSelectableAssets.map((asset) => asset.path) : [])}
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
                          groupPath={group.path}
                          groupCategory={groupCategory}
                          confidenceEnabled={isConfidenceEnabled}
                          groupProfileId={selectedProfileId < 0 ? -1 : effectiveProfileId}
                          groupAudioProfileKey={selectedAudioProfileKey}
                          groupLibraryId={selectedLibraryId}
                          hasOpenJob={queueGroup.isPending || assetHasOpenJob(asset, queueJobs)}
                          queueJobs={queueJobs}
                          snapshotRunning={runningSnapshotPaths.has(asset.path)}
                          mode={mode}
						  bulkSelected={selectedAssetIds ? selectedAssetIds.has(asset.id ?? 0) : selectedAssetPaths.includes(asset.path)}
						  bulkSelectionEnabled={Boolean(selectedAssetIds) || (isReadOnlyGroup && hasMultipleSelectableAssets)}
                          bulkSelectionDisabled={asset.missing || bulkAssetAction.isPending}
						  onBulkSelectionChange={(path, selected) => selectedAssetIds
							? onAssetSelectionChange?.([groupAssets.find((candidate) => candidate.path === path)?.id ?? 0], selected)
							: toggleBulkAsset(path, selected)}
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
      <Dialog
        open={pathAdvisorOpen}
        onClose={() => {
          if (!runPathAdvisor.isPending) setPathAdvisorOpen(false);
        }}
        disableEscapeKeyDown={runPathAdvisor.isPending}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle>Advisor · {groupTitle(group)}</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ pt: 1 }}>
            <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
              {group.path}
            </Typography>
            <Stack spacing={0.75}>
              <Stack direction="row" justifyContent="space-between" spacing={1}>
                <Typography fontWeight={700}>
                  {runPathAdvisor.isPending ? 'Evaluating assets one at a time' : 'Evaluation complete'}
                </Typography>
                <Typography color="text.secondary">
                  {pathAdvisorResults.length}/{groupAssets.filter((asset) => !asset.missing).length}
                </Typography>
              </Stack>
              <LinearProgress
                variant="determinate"
                value={groupAssets.filter((asset) => !asset.missing).length
                  ? (pathAdvisorResults.length / groupAssets.filter((asset) => !asset.missing).length) * 100
                  : 0}
              />
              {pathAdvisorCurrent ? (
                <Typography color="text.secondary" variant="caption" sx={{ wordBreak: 'break-all' }}>
                  Evaluating: {pathAdvisorCurrent}
                </Typography>
              ) : null}
            </Stack>
            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
              <Chip
                color="success"
                label={`${pathAdvisorResults.filter((result) => result.response?.recommendation === 'worth_it').length} comply`}
                size="small"
              />
              <Chip
                color="warning"
                label={`${pathAdvisorResults.filter((result) => result.response?.recommendation === 'maybe').length} review`}
                size="small"
              />
              <Chip
                label={`${pathAdvisorResults.filter((result) => result.response?.recommendation === 'not_recommended').length} not recommended`}
                size="small"
              />
              {pathAdvisorResults.some((result) => result.error) ? (
                <Chip color="error" label={`${pathAdvisorResults.filter((result) => result.error).length} failed`} size="small" />
              ) : null}
            </Stack>
            <Stack spacing={1} sx={{ maxHeight: '48vh', overflowY: 'auto', pr: 0.5 }}>
              {pathAdvisorResults.map((result) => (
                <Box key={result.asset.path} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.25 }}>
                  <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ xs: 'flex-start', sm: 'center' }} spacing={1}>
                    <Stack sx={{ minWidth: 0 }}>
                      <Typography fontWeight={700} sx={{ overflowWrap: 'anywhere' }}>{result.asset.fileName}</Typography>
                      <Typography color="text.secondary" variant="caption" sx={{ overflowWrap: 'anywhere' }}>{result.asset.path}</Typography>
                    </Stack>
                    {result.response ? (
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Chip
                          label={recommendationLabel(result.response.recommendation)}
                          color={recommendationColor(result.response.recommendation)}
                          size="small"
                        />
                        <Typography fontWeight={700}>{result.response.score}%</Typography>
                      </Stack>
                    ) : (
                      <Chip label="Failed" color="error" size="small" />
                    )}
                  </Stack>
                  {result.error ? <Typography color="error" variant="body2" sx={{ mt: 0.75 }}>{result.error}</Typography> : null}
                </Box>
              ))}
            </Stack>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPathAdvisorOpen(false)} disabled={runPathAdvisor.isPending} variant="contained">
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}

function AssetRow({
  asset,
	compactUnprocessed = false,
  libraries,
  profiles,
  audioProfiles,
  trackProfiles,
  pathTrackProfile,
  assetCategories,
  groupRelativePath,
  groupPath,
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
  archivePathAssets = [],
}: {
  asset: Asset;
	compactUnprocessed?: boolean;
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  trackProfiles: TrackProfile[];
  pathTrackProfile?: TrackProfile;
  assetCategories: string[];
  groupRelativePath: string;
  groupPath: string;
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
  archivePathAssets?: Asset[];
}) {
  const queryClient = useQueryClient();
  const profileAssignments = useQuery({ queryKey: ['profileAssignments'], queryFn: api.profileAssignments });
  const assetVideoProfiles = profiles.filter((profile) => profile.scope === 'asset');
  const assetAudioProfiles = audioProfiles.filter((profile) => profile.scope === 'asset');
  const assetTrackProfiles = trackProfiles.filter((profile) => profile.scope === 'asset');
  const [selectedProfileId, setSelectedProfileId] = useState<number>(0);
  const [selectedAudioProfileKey, setSelectedAudioProfileKey] = useState<string>('__inherit__');
  const [selectedTrackProfileKey, setSelectedTrackProfileKey] = useState<string>('__inherit__');
  const effectiveProfileId =
    selectedProfileId === 0
      ? groupProfileId
      : selectedProfileId > 0
        ? selectedProfileId
        : 0;
  const effectiveAudioProfileKey = selectedAudioProfileKey === '__inherit__' ? groupAudioProfileKey : selectedAudioProfileKey;
  const effectiveTrackProfileKey = selectedTrackProfileKey === '__inherit__' ? pathTrackProfile?.key ?? '' : selectedTrackProfileKey;
  const selectedTrackProfile = trackProfiles.find((profile) => profile.key === effectiveTrackProfileKey);
  const [selectedLibraryId, setSelectedLibraryId] = useState<number>(groupLibraryId);
  const [showSnapshotDialog, setShowSnapshotDialog] = useState(false);
  const [snapshotTab, setSnapshotTab] = useState(0);
  const [renameFileName, setRenameFileName] = useState(asset.fileName);
  const [snapshotOperation, setSnapshotOperation] = useState<SnapshotOperation | null>(null);
  const [editingSubtitle, setEditingSubtitle] = useState<ExternalSubtitle | null>(null);
  const [renamingSubtitle, setRenamingSubtitle] = useState<ExternalSubtitle | null>(null);
  const [subtitleFileName, setSubtitleFileName] = useState('');
  const [subtitleContent, setSubtitleContent] = useState('');
  const [subtitleGenerations, setSubtitleGenerations] = useState<Record<string, SubtitleGenerationState>>({});
  const subtitleOperationPollers = useRef<Set<string>>(new Set());
  const [showPreviewDialog, setShowPreviewDialog] = useState(false);
  const [showAdvisorDialog, setShowAdvisorDialog] = useState(false);
  const [showTestEncodeDialog, setShowTestEncodeDialog] = useState(false);
  const [archiveDeletePaths, setArchiveDeletePaths] = useState<string[]>([]);
  const [archiveDeleteAccepted, setArchiveDeleteAccepted] = useState(false);
  const [previewMode, setPreviewMode] = useState<'compatible' | 'original'>('compatible');
  const assetReview = asset.review ?? { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' };
  const reconciliationJobId = reconciliationJobIdFromReview(assetReview);
  const assetMetadata = asset.metadata ?? { categories: [], tags: [], updatedAt: '' };
  const [reviewReason, setReviewReason] = useState(assetReview.reason || '');
  const [reviewTags, setReviewTags] = useState<string[]>(safeArray(assetReview.tags));
  const [category, setCategory] = useState<string>(firstCategory(assetMetadata.categories) || groupCategory);
  const [conversionDraft, setConversionDraft] = useState<AssetConversionOverrideState>(() => normalizeAssetConversionOverride(asset.conversion));
  const persistedConversionSignature = JSON.stringify(
    normalizeAssetConversionOverride(asset.conversion),
  );
  const profileSuggestion = useMutation({ mutationFn: api.suggestProfile });
  const storedSnapshot = useQuery({
    queryKey: ['assetSnapshot', asset.path],
    queryFn: () => api.latestSnapshot(asset.path),
    enabled: showSnapshotDialog && !asset.missing,
    staleTime: 0,
  });
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
      queryClient.setQueryData(['assetSnapshot', asset.path], {
        found: true,
        snapshot: scan,
        status: 'current',
        requiresAnalysis: false,
        staleComponents: [],
      });
      if (asset.status !== 'converted' && asset.status !== 'archive' && asset.status !== 'accepted') {
        profileSuggestion.mutate(scan.path);
      }
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
      await queryClient.invalidateQueries({ queryKey: ['snapshotOperations'] });
    },
  });
  const snapshotData = storedSnapshot.data?.snapshot ?? snapshot.data ?? null;
  const storedSnapshotExists = snapshotData !== null;
  const refreshRequired = storedSnapshot.data?.snapshot != null && storedSnapshot.data.requiresAnalysis;
  const operationPending = snapshot.isPending;
  const externalSubtitles = useQuery({
    queryKey: ['externalSubtitles', asset.path],
    queryFn: () => api.externalAssetSubtitles(asset.path),
    enabled: showSnapshotDialog && asset.status !== 'archive' && !asset.missing,
  });
  const advisor = useQuery({
    queryKey: ['advisor', 'asset-row', asset.path, effectiveProfileId],
    queryFn: () => api.evaluateAdvisor({ mediaPath: asset.path, profileId: effectiveProfileId }),
    enabled: showAdvisorDialog && mode !== 'archive' && mode !== 'converted' && asset.status !== 'archive' && asset.status !== 'converted' && asset.status !== 'published_as_is' && asset.status !== 'accepted' && confidenceEnabled && effectiveProfileId > 0,
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
      closeSnapshotDialog();
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const updateConversion = useMutation({
    mutationKey: ['assetTrackSelection', normalizePath(groupPath)],
    mutationFn: api.updateAssetConversion,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const updateProfileAssignment = useMutation({
    mutationFn: api.updateProfileAssignment,
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ['profileAssignments'] }),
  });
  const recoverAsset = useMutation({
    mutationFn: api.recoverAsset,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const deleteArchiveAssets = useMutation({
    mutationFn: api.deleteArchiveAssets,
    onSuccess: async () => {
      setArchiveDeletePaths([]);
      setArchiveDeleteAccepted(false);
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
  const updateAssetDecision = useMutation({
    mutationFn: ({ path, action }: { path: string; action: 'accept' | 'reconsider' }) =>
      action === 'accept' ? api.acceptAssetAsIs(path) : api.reconsiderAsset(path),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
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
  const renameExternalSubtitle = useMutation({
    mutationFn: api.renameExternalAssetSubtitle,
    onSuccess: async () => {
      setRenamingSubtitle(null);
      setSubtitleFileName('');
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
  const isAccepted = asset.status === 'accepted';
  const isLibraryReplacement = asset.status === 'unverified' || asset.status === 'library' || asset.status === 'published_as_is';
  const isArchive = mode === 'archive' || asset.status === 'archive';
  const canGenerateTestEncode = testEncodeEligibleAsset(asset, mode);
  const rowColumnCount = compactUnprocessed ? 5 : isConverted
    ? bulkSelectionEnabled ? 8 : 7
    : isArchive
      ? bulkSelectionEnabled ? 7 : 6
      : isAccepted ? 5 : 10;
  const associatedJob = associatedJobForAsset(asset, queueJobs);
  const rowLocked = hasOpenJob || createJob.isPending || (isConverted && !isLibraryReplacement) || isArchive || isPublishedAsIs || isAccepted;
  const pipelineState = assetPipelineState(asset, associatedJob, createJob.isPending);
  const isOverrideOnly =
    selectedProfileId === VIDEO_PROFILE_OVERRIDE_ONLY;

  const isAudioOnly =
    selectedProfileId === VIDEO_PROFILE_AUDIO_ONLY;

  const canQueueWithSelection =
    effectiveProfileId > 0 ||
    (isOverrideOnly && assetHasConversionOverride(conversionDraft)) ||
    isAudioOnly ||
    Boolean(effectiveAudioProfileKey) ||
    Boolean(selectedTrackProfile) ||
    hasTrackSelectionOverride(conversionDraft);

  useEffect(() => {
    if (!profileAssignments.data) return;
    const assignments = profileAssignments.data.filter((assignment) => assignment.targetType === 'asset' && normalizePath(assignment.targetPath) === normalizePath(asset.path));
    const video = assignments.find((assignment) => assignment.mediaType === 'video');
    const audio = assignments.find((assignment) => assignment.mediaType === 'audio');
    const tracks = assignments.find((assignment) => assignment.mediaType === 'tracks');
    setSelectedProfileId(video ? video.selection === 'disabled' ? -1 : video.videoProfileId || 0 : 0);
    setSelectedAudioProfileKey(audio ? audio.selection === 'disabled' ? '' : audio.profileKey || '__inherit__' : '__inherit__');
    setSelectedTrackProfileKey(tracks ? tracks.selection === 'disabled' ? '' : tracks.profileKey || '__inherit__' : '__inherit__');
  }, [profileAssignments.data, asset.path]);

  function selectAssetVideoProfile(id: number) {
    setSelectedProfileId(id);

    const selection =
      id === 0
        ? 'inherit'
        : id === VIDEO_PROFILE_OVERRIDE_ONLY
          ? 'override_only'
          : id === VIDEO_PROFILE_AUDIO_ONLY
            ? 'audio_only'
            : 'profile';

    updateProfileAssignment.mutate({
      targetType: 'asset',
      targetPath: asset.path,
      mediaType: 'video',
      selection,
      videoProfileId: id > 0 ? id : 0,
      profileKey: '',
    });
  }

  function selectAssetAudioProfile(key: string) {
    setSelectedAudioProfileKey(key);
    updateProfileAssignment.mutate({ targetType: 'asset', targetPath: asset.path, mediaType: 'audio', selection: key === '__inherit__' ? 'inherit' : key ? 'profile' : 'disabled', videoProfileId: 0, profileKey: key === '__inherit__' ? '' : key });
  }

  function selectAssetTrackProfile(key: string) {
    setSelectedTrackProfileKey(key);
    updateProfileAssignment.mutate({ targetType: 'asset', targetPath: asset.path, mediaType: 'tracks', selection: key === '__inherit__' ? 'inherit' : key ? 'profile' : 'disabled', videoProfileId: 0, profileKey: key === '__inherit__' ? '' : key });
  }
  useEffect(() => {
    if (!showSnapshotDialog || asset.missing) {
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
    setConversionDraft(
      normalizeAssetConversionOverride(asset.conversion),
    );
  }, [asset.path, persistedConversionSignature]);

  function openSnapshotDialog(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    setSnapshotTab(0);
    setShowSnapshotDialog(true);
  }

  function closeSnapshotDialog() {
    setShowSnapshotDialog(false);
    snapshot.reset();
    setSnapshotOperation(null);
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

  async function applySnapshotRecommendations(findings: AdvisorFinding[]) {
    let draft: AssetConversionOverrideState = { ...conversionDraft };
    findings.forEach((finding) => {
      const patch = (finding.patch ?? {}) as Partial<AssetConversionOverrideState>;
      if (typeof patch.videoFilters === 'string' && patch.videoFilters.startsWith('crop=')) {
        draft = { ...draft, ...patch, videoFilters: joinFilters(withoutCropFilters(draft.videoFilters), patch.videoFilters) };
      } else {
        draft = { ...draft, ...patch };
      }
    });
    const next = cleanConversionOverride(draft);
    await updateConversion.mutateAsync({ path: asset.path, ...next });
    setConversionDraft(next);
    setSnapshotTab(5);
    return 'Recommendations were saved to Asset Overrides.';
  }

  function toggleSnapshotStream(type: MediaStreamInfo['type'], index: number, keep: boolean) {
    const scan = snapshotData;
    if (!scan) {
      return;
    }
    const allIndexes = streamIndexesForType(scan, type);
    const current = conversionStreamIndexes(conversionDraft, scan, type);
    const next = keep ? normalizeNumberList([...current, index]) : safeArray(current).filter((candidate) => candidate !== index);
    const indexes = selectedOrUndefined(next, allIndexes);
    const nextDraft = withStreamSelection(conversionDraft, type, indexes);
    setConversionDraft(nextDraft);
    updateConversion.mutate({ path: asset.path, ...cleanConversionOverride(nextDraft) });
  }

  function updateConversionDraft<K extends keyof AssetConversionOverrideState>(key: K, value: AssetConversionOverrideState[K]) {
    const customHardwareControl = ['globalQuality', 'qsvRateControl', 'qsvLookAheadDepth', 'qsvExtendedBrc', 'qsvAdaptiveI', 'qsvAdaptiveB', 'qsvPStrategy', 'videoToolboxBitrateMbps', 'videoToolboxMaxrateMbps', 'videoToolboxBufferMbps', 'videoToolboxProfile', 'videoToolboxGop', 'videoToolboxRealtime', 'videoToolboxBFramePolicy', 'videoToolboxBFrames', 'videoToolboxAutoAdjustBitrate', 'videoToolboxPowerEfficiency', 'pixFmt'].includes(String(key));
    setConversionDraft((current) => {
      const rates = key === 'videoToolboxBitrateMbps' ? videoToolboxRatesFromTargetMbps(value) : null;
      return {
        ...current,
        [key]: value,
        ...(rates ? { videoToolboxBitrateMbps: rates.target, videoToolboxMaxrateMbps: rates.maxrate, videoToolboxBufferMbps: rates.buffer } : {}),
        ...(customHardwareControl ? { hardwareQualityPreset: 'custom' } : {}),
        ...(['frameStructureGopMode', 'frameStructureGopFrames', 'frameStructureBFrameMode', 'frameStructureMaxBFrames', 'qsvAdaptiveI', 'qsvAdaptiveB', 'qsvPStrategy'].includes(String(key)) ? { frameStructureMode: 'custom' as const } : {}),
      };
    });
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

  async function openAssetTestEncode() {
    try {
      await updateConversion.mutateAsync({ path: asset.path, ...cleanConversionOverride(conversionDraft) });
      setShowTestEncodeDialog(true);
    } catch {
      // The existing mutation alert reports the save error. Do not generate a
      // sample from a stale persisted override.
    }
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
    const queueProfileId =
      effectiveProfileId > 0
        ? effectiveProfileId
        : 0;
    if (!selectedLibraryId || !canQueueWithSelection) {
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
      audioProfileKey: effectiveAudioProfileKey,
      trackProfileKey: selectedTrackProfile?.key ?? '',
      processingMode: isAudioOnly ? 'audio_only' : 'full_encode',
      resolveProfileAssignments: true,
      priority: priorityForSize(asset.sizeBytes),
      notes: queueNotes(`Queued individually from folder view: ${relativeAssetPath(asset, libraries)}`, effectiveAudioProfileKey),
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

  function requestArchiveDeletion(event: MouseEvent<HTMLButtonElement>, paths: string[]) {
    event.stopPropagation();
    setArchiveDeleteAccepted(false);
    setArchiveDeletePaths(paths);
    deleteArchiveAssets.reset();
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
        {!compactUnprocessed && !isArchive && !isConverted && !isPublishedAsIs && !isAccepted ? (
          <TableCell>
            <Tooltip title="Click para evaluar">
              <span>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<InfoOutlinedIcon />}
                  aria-label="Click para evaluar"
                  disabled={!confidenceEnabled || effectiveProfileId <= 0}
                  onClick={() => setShowAdvisorDialog(true)}
                  sx={{ minWidth: 64 }}
                >
                  {confidenceEnabled ? advisor.data ? advisor.data.score : '?' : 'Off'}
                </Button>
              </span>
            </Tooltip>
          </TableCell>
        ) : null}
        {isConverted ? (
          <TableCell>
            <ConvertedMediaSummary
              technical={snapshotData ? {
                videoCodec: snapshotData.videoCodec,
                encoder: asset.technical?.encoder,
                width: snapshotData.width,
                height: snapshotData.height,
                duration: snapshotData.duration,
                bitrate: snapshotData.bitrate,
                hdr: snapshotData.hdr,
              } : asset.technical}
            />
          </TableCell>
        ) : null}
        <TableCell>{formatBytes(asset.sizeBytes)}</TableCell>
        {mode !== 'unprocessed' ? <TableCell>{formatDate(asset.modifiedAt)}</TableCell> : null}
        {mode === 'unprocessed' && !compactUnprocessed ? (
          <TableCell sx={{ minWidth: 180 }}>
            <AssetCategorySelect value={category} options={assetCategories} onChange={saveAssetCategory} label="Category" size="small" disabled={asset.missing || updateMetadata.isPending} />
          </TableCell>
        ) : null}
        {!compactUnprocessed && !isArchive && !isConverted && !isPublishedAsIs && !isAccepted ? (
          <>
            <TableCell sx={{ minWidth: 220 }}>
              <ProfileAutocomplete
                profiles={assetVideoProfiles}
                value={selectedProfileId}
                onChange={selectAssetVideoProfile}
                label="Video · Asset"
                size="small"
                disabled={rowLocked || updateProfileAssignment.isPending}
                allowInherit
                allowOverrideOnly
                allowAudioOnly
              />           
            </TableCell>
            <TableCell sx={{ minWidth: 220 }}>
              <AudioProfileAutocomplete profiles={assetAudioProfiles} value={selectedAudioProfileKey} onChange={selectAssetAudioProfile} label="Audio · Asset" size="small" disabled={rowLocked || updateProfileAssignment.isPending} allowInherit />
            </TableCell>
            {mode === 'unprocessed' ? (
              <TableCell sx={{ minWidth: 220 }}>
                <TrackProfileAutocomplete profiles={assetTrackProfiles} value={selectedTrackProfileKey} onChange={selectAssetTrackProfile} disabled={rowLocked || updateProfileAssignment.isPending} label="Tracks · Asset" allowInherit />
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
              <>
                <Tooltip title={asset.missing ? 'Archive file is no longer physically available' : 'Recover original without deleting converted files'}>
                  <span>
                    <IconButton
                      color="primary"
                      onClick={recoverArchivedAsset}
                      disabled={asset.missing || recoverAsset.isPending || deleteArchiveAssets.isPending}
                      aria-label={`Recover ${asset.fileName}`}
                      sx={actionIconSx}
                    >
                      <TaskAltIcon />
                    </IconButton>
                  </span>
                </Tooltip>
                <Tooltip title="Permanently delete this Archive asset">
                  <span>
                    <IconButton
                      color="error"
                      onClick={(event) => requestArchiveDeletion(event, [asset.path])}
                      disabled={asset.missing || deleteArchiveAssets.isPending}
                      aria-label={`Permanently delete ${asset.fileName}`}
                      sx={actionIconSx}
                    >
                      <DeleteForeverIcon />
                    </IconButton>
                  </span>
                </Tooltip>
                {archivePathAssets.length > 1 ? (
                  <Tooltip title={`Permanently delete all ${archivePathAssets.length} Archive assets in this path`}>
                    <span>
                      <IconButton
                        color="error"
                        onClick={(event) => requestArchiveDeletion(event, archivePathAssets.map((candidate) => candidate.path))}
                        disabled={deleteArchiveAssets.isPending}
                        aria-label={`Permanently delete Archive path ${groupRelativePath}`}
                        sx={actionIconSx}
                      >
                        <DeleteSweepIcon />
                      </IconButton>
                    </span>
                  </Tooltip>
                ) : null}
              </>
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
            ) : isAccepted ? (
              <Tooltip title="Return this asset to Unverified for another decision">
                <span onClick={(event) => event.stopPropagation()}>
                  <IconButton
                    color="primary"
                    onClick={() => updateAssetDecision.mutate({ path: asset.path, action: 'reconsider' })}
                    disabled={updateAssetDecision.isPending}
                    aria-label={`Reconsider ${asset.fileName}`}
                    sx={actionIconSx}
                  >
                    <RefreshIcon />
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
				  disabled={profileAssignments.isLoading || updateProfileAssignment.isPending || createJob.isPending || !canQueueWithSelection || !selectedLibraryId || isBlockedByReview || hasOpenJob}
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
				  disabled={profileAssignments.isLoading || updateProfileAssignment.isPending || createJob.isPending || !canQueueWithSelection || isBlockedByReview || hasOpenJob}
                  aria-label={`Replace ${asset.fileName}`}
                  sx={actionIconSx}
                >
                  <PlaylistAddIcon />
                </IconButton>
              </Tooltip>
            ) : null}
            {asset.status === 'unverified' ? (
              <Tooltip title="Accept as-is without converting, moving, or archiving this asset">
                <span onClick={(event) => event.stopPropagation()}>
                  <IconButton
                    color="success"
                    onClick={() => updateAssetDecision.mutate({ path: asset.path, action: 'accept' })}
                    disabled={updateAssetDecision.isPending || hasOpenJob}
                    aria-label={`Accept as-is ${asset.fileName}`}
                    sx={actionIconSx}
                  >
                    <TaskAltIcon />
                  </IconButton>
                </span>
              </Tooltip>
            ) : null}
          </Stack>
          <Snackbar
            open={createJob.isSuccess || createJob.isError}
            autoHideDuration={createJob.isSuccess ? 4000 : 7000}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            onClose={(_, reason) => {
              if (reason !== 'clickaway') createJob.reset();
            }}
          >
            <Alert
              severity={createJob.isError ? 'warning' : 'success'}
              variant="filled"
              onClose={() => createJob.reset()}
              sx={{ maxWidth: 520, overflowWrap: 'anywhere' }}
            >
              {createJob.isError
                ? createJob.error instanceof Error ? createJob.error.message : 'Could not queue this asset.'
                : 'Asset queued individually.'}
            </Alert>
          </Snackbar>
          <Snackbar
            open={deleteArchiveAssets.isSuccess || deleteArchiveAssets.isError}
            autoHideDuration={7000}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            onClose={(_, reason) => {
              if (reason !== 'clickaway') deleteArchiveAssets.reset();
            }}
          >
            <Alert
              severity={deleteArchiveAssets.isError || deleteArchiveAssets.data?.failures.length ? 'warning' : 'success'}
              variant="filled"
              onClose={() => deleteArchiveAssets.reset()}
            >
              {deleteArchiveAssets.isError
                ? deleteArchiveAssets.error instanceof Error ? deleteArchiveAssets.error.message : 'Archive deletion failed.'
                : `${deleteArchiveAssets.data?.completed ?? 0} Archive asset(s) permanently deleted.${deleteArchiveAssets.data?.failures.length ? ` ${deleteArchiveAssets.data.failures.length} failed.` : ''}`}
            </Alert>
          </Snackbar>
          <Snackbar
            open={updateAssetDecision.isError}
            autoHideDuration={7000}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            onClose={(_, reason) => {
              if (reason !== 'clickaway') updateAssetDecision.reset();
            }}
          >
            <Alert severity="warning" variant="filled" onClose={() => updateAssetDecision.reset()}>
              {updateAssetDecision.error instanceof Error ? updateAssetDecision.error.message : 'The asset decision could not be updated.'}
            </Alert>
          </Snackbar>
        </TableCell>
      </TableRow>
      <ArchiveDeleteConfirmationDialog
        paths={archiveDeletePaths}
        pathLabel={groupRelativePath}
        accepted={archiveDeleteAccepted}
        pending={deleteArchiveAssets.isPending}
        onAcceptedChange={setArchiveDeleteAccepted}
        onCancel={() => {
          setArchiveDeletePaths([]);
          setArchiveDeleteAccepted(false);
        }}
        onConfirm={() => deleteArchiveAssets.mutate(archiveDeletePaths)}
      />
      <TestEncodeDialog
        open={showTestEncodeDialog}
        onClose={() => setShowTestEncodeDialog(false)}
        sourcePath={asset.path}
        libraries={libraries}
        defaultLibraryId={selectedLibraryId}
        request={{
          configurationSource: 'effective_asset',
          profileId: effectiveProfileId,
          audioProfileKey: effectiveAudioProfileKey,
          trackProfileKey: selectedTrackProfile?.key ?? '',
          processingMode: isAudioOnly ? 'audio_only' : 'full_encode',
          resolveAssignments: true,
        }}
      />
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
      <Dialog open={showSnapshotDialog} onClose={closeSnapshotDialog} maxWidth="lg" fullWidth>
        <DialogTitle sx={{ pb: 0.75 }}>
          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', sm: 'flex-start' }} spacing={1}>
            <Stack sx={{ minWidth: 0 }}>
              <Typography variant="h2">Asset Snapshot</Typography>
              <Typography color="text.secondary" variant="body2" noWrap>
                {assetTitle(asset)}
              </Typography>
            </Stack>
            {storedSnapshotExists && !refreshRequired ? (
              <Button startIcon={<RefreshIcon />} variant="outlined" size="small" onClick={refreshSnapshot} disabled={asset.missing || operationPending}>
                Rescan
              </Button>
            ) : null}
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
            {storedSnapshot.isLoading && !storedSnapshotExists && !operationPending ? <LinearProgress /> : null}
            {operationPending ? (
              <Alert severity="info">
                <Stack spacing={1}>
                  <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={1}>
                    <Typography variant="body2">{snapshotOperation?.message || 'Preparing this asset snapshot…'}</Typography>
                    <Button
                      size="small"
                      color="inherit"
                      disabled={!snapshotOperation || snapshotOperation.status !== 'running'}
                      onClick={async () => {
                        if (!snapshotOperation) return;
                        setSnapshotOperation(await api.cancelSnapshotOperation(snapshotOperation.id));
                      }}
                    >
                      Cancel
                    </Button>
                  </Stack>
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
            {!asset.missing && !storedSnapshot.isLoading && !operationPending && !storedSnapshotExists ? (
              <Alert
                severity="info"
                action={<Button color="inherit" size="small" onClick={() => snapshot.mutate({ path: asset.path })}>Analyze asset</Button>}
              >
                No snapshot available
              </Alert>
            ) : null}
            {!asset.missing && !operationPending && refreshRequired ? (
              <Alert
                severity="warning"
                action={<Button color="inherit" size="small" onClick={refreshSnapshot}>Rescan</Button>}
              >
                Snapshot needs refresh
              </Alert>
            ) : null}
            {storedSnapshotExists ? (
              <>
                <Tabs value={snapshotTab} onChange={(_, value: number) => setSnapshotTab(value)} variant="scrollable" allowScrollButtonsMobile>
                  <Tab label="Asset Information" />
                  <Tab label="Technical Snapshot" />
                  <Tab label="Tracks" />
                  <Tab label="Job Information" />
                  <Tab label="MVForge Suggestions" />
                  <Tab label="Quick Asset Overrides" />
                  <Tab label="Test Encode" />
                </Tabs>
                <Box hidden={snapshotTab !== 1}>
                  <Box sx={{ pt: 1.5 }}>
                    <MediaSnapshotDetails scan={snapshotData} section="general" />
                    <DirectPlaySnapshotComparison scan={snapshotData} job={associatedJob} />
                  </Box>
                </Box>
                <Box hidden={snapshotTab !== 2}>
                  <Stack spacing={2} sx={{ pt: 1.5 }}>
                    <TrackMaintenancePanel path={asset.path} active={showSnapshotDialog && snapshotTab === 2} hasOpenJob={hasOpenJob} />
                    {!isConverted && !isArchive ? <><Typography variant="h3">Conversion track selection</Typography><MediaSnapshotDetails
                      scan={snapshotData}
                      section="tracks"
                      streamControls={
                        isConverted || isArchive
                          ? undefined
                          : {
                              video: {
                                selected: conversionStreamIndexes(conversionDraft, snapshotData, 'video'),
                                disabled: updateConversion.isPending,
                                onToggle: (index, keep) => toggleSnapshotStream('video', index, keep),
                              },
                              audio: {
                                selected: conversionStreamIndexes(conversionDraft, snapshotData, 'audio'),
                                disabled: updateConversion.isPending,
                                onToggle: (index, keep) => toggleSnapshotStream('audio', index, keep),
                              },
                              subtitle: {
                                selected: conversionStreamIndexes(conversionDraft, snapshotData, 'subtitle'),
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
                    /></> : null}
                    {isArchive ? (
                      <Alert severity="info">
                        The archived original is used as the subtitle source. Generated files are saved beside its active converted asset.
                      </Alert>
                    ) : null}
                    <EmbeddedSubtitleActions
                      streams={snapshotData.subtitleStreams}
                      generations={subtitleGenerations}
                      onGenerate={generateExternalSubtitle}
                    />
                    {!isArchive ? (
                      <ExternalSubtitleList
                        values={externalSubtitles.data ?? []}
                        loading={externalSubtitles.isLoading}
                        deleting={deleteExternalSubtitle.isPending}
                        renaming={renameExternalSubtitle.isPending}
                        onRename={(subtitle) => {
                          setRenamingSubtitle(subtitle);
                          setSubtitleFileName(subtitle.fileName);
                        }}
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
					  <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Video profile applied{profileAssignmentSource(associatedJob, 'video')}</Typography><Typography>{associatedJob ? profiles.find((profile) => profile.id === associatedJob.profileId)?.name || `Profile #${associatedJob.profileId}` : profiles.find((profile) => profile.id === effectiveProfileId)?.name || 'None'}</Typography></Grid>
					  <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Audio profile applied{profileAssignmentSource(associatedJob, 'audio')}</Typography><Typography>{associatedJob?.audioProfileKey || effectiveAudioProfileKey || 'None'}</Typography></Grid>
					  <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Tracks profile applied{profileAssignmentSource(associatedJob, 'tracks')}</Typography><Typography>{associatedJob?.trackProfileKey || selectedTrackProfile?.name || selectedTrackProfile?.key || conversionDraft.trackProfileKey || 'None'}</Typography></Grid>
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
                        onApplyRecommendations={applySnapshotRecommendations}
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
                        profile={profiles.find((profile) => profile.id === effectiveProfileId)}
                        scan={snapshotData}
                        onChange={updateConversionDraft}
                        onChangeMany={(patch) => setConversionDraft((current) => ({ ...current, ...patch }))}
                        onSave={saveConversionOverrides}
                        onReset={resetConversionOverrides}
                        saving={updateConversion.isPending}
                        readOnly={isConverted}
                      />
                    ) : <Alert severity="info">Archived originals are immutable. Recover the asset before configuring conversion overrides.</Alert>}
                  </Stack>
                </Box>
                <Box hidden={snapshotTab !== 6}>
                  <Stack spacing={2} sx={{ pt: 1.5 }}>
                    <Stack spacing={0.5}>
                      <Typography variant="h3">Test Encode</Typography>
                      <Typography color="text.secondary" variant="body2">
                        Generate a short real encode with the same effective pipeline that this asset would use in Queue. Current Asset Overrides are saved before the test snapshot is created. The original is not archived or modified.
                      </Typography>
                    </Stack>
                    <Grid container spacing={1.5}>
                      <Grid size={{ xs: 12, sm: 6 }}>
                        <ProfileAutocomplete
                          profiles={assetVideoProfiles}
                          value={selectedProfileId}
                          onChange={selectAssetVideoProfile}
                          label="Video profile"
                          size="small"
                          disabled={asset.missing || hasOpenJob || updateProfileAssignment.isPending}
                          allowInherit
                        />
                      </Grid>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Audio profile</Typography><Typography>{effectiveAudioProfileKey || 'None'}</Typography></Grid>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Tracks profile</Typography><Typography>{selectedTrackProfile?.name || selectedTrackProfile?.key || conversionDraft.trackProfileKey || 'None'}</Typography></Grid>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Destination Library</Typography><Typography>{libraries.find((library) => library.id === selectedLibraryId)?.name || 'Choose when generating'}</Typography></Grid>
                    </Grid>
                    {!canGenerateTestEncode ? (
                      <Alert severity="info">Generate Test Encode is unavailable for Converted, Archive, or Published as-is assets.</Alert>
                    ) : (
                      <Tooltip title={hasOpenJob ? 'Asset has an active Queue job' : !effectiveProfileId ? 'Select an effective video profile first' : updateConversion.isPending || updateProfileAssignment.isPending ? 'Saving the current asset configuration' : 'Save current overrides and generate a short real encode for playback testing'}>
                        <span style={{ alignSelf: 'flex-start' }}>
                          <Button
                            startIcon={<ScienceIcon />}
                            variant="contained"
                            color="secondary"
                            onClick={openAssetTestEncode}
                            disabled={asset.missing || hasOpenJob || !effectiveProfileId || updateConversion.isPending || updateProfileAssignment.isPending}
                          >
                            {updateConversion.isPending ? 'Saving overrides…' : 'Generate Test Encode'}
                          </Button>
                        </span>
                      </Tooltip>
                    )}
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
      <Dialog open={Boolean(renamingSubtitle)} onClose={() => setRenamingSubtitle(null)} maxWidth="sm" fullWidth>
        <DialogTitle>Rename external subtitle</DialogTitle>
        <DialogContent>
          <Stack spacing={1.5} sx={{ pt: 1 }}>
            <TextField
              label="File name"
              value={subtitleFileName}
              onChange={(event) => setSubtitleFileName(event.target.value)}
              helperText="Keep the asset name prefix and use an .srt or .ass extension."
              fullWidth
              autoFocus
            />
            {renameExternalSubtitle.isError ? <Alert severity="warning">{renameExternalSubtitle.error instanceof Error ? renameExternalSubtitle.error.message : 'Subtitle rename failed.'}</Alert> : null}
            <Stack direction="row" spacing={1} justifyContent="flex-end">
              <Button onClick={() => setRenamingSubtitle(null)}>Cancel</Button>
              <Button
                variant="contained"
                disabled={!renamingSubtitle || renameExternalSubtitle.isPending || !subtitleFileName.trim() || subtitleFileName.trim() === renamingSubtitle.fileName}
                onClick={() => renamingSubtitle && renameExternalSubtitle.mutate({ path: asset.path, subtitlePath: renamingSubtitle.path, fileName: subtitleFileName.trim() })}
              >
                Rename
              </Button>
            </Stack>
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
              <AdvisorSummary advisor={advisor.data} audioProfile={audioProfiles.find((profile) => profile.key === effectiveAudioProfileKey)} />
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
  renaming,
  onRename,
  onEdit,
  onDelete,
}: {
  values: ExternalSubtitle[];
  loading: boolean;
  deleting: boolean;
  renaming: boolean;
  onRename: (subtitle: ExternalSubtitle) => void;
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
              <Tooltip title="Rename subtitle file">
                <IconButton size="small" color="primary" disabled={renaming} onClick={() => onRename(subtitle)}><DriveFileRenameOutlineIcon /></IconButton>
              </Tooltip>
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
  { value: 'veryfast', label: 'Fast preview' },
  { value: 'fast', label: 'Fast' },
  { value: 'medium', label: 'Balanced' },
  { value: 'slow', label: 'Higher compression' },
  { value: 'slower', label: 'Archive patience' },
];

const colorDepthOptions: SelectOption[] = [
  { value: '', label: 'Profile default' },
  { value: 'auto', label: 'Auto / codec default' },
  { value: 'yuv420p10le', label: '10-bit Main10' },
  { value: 'p010le', label: 'Hardware 10-bit Main10 (P010)' },
  { value: 'yuv420p', label: '8-bit compatibility' },
  { value: 'nv12', label: 'QSV 8-bit Main (NV12)' },
];

const imageCleanupOptions: SelectOption[] = [
  { value: '', label: 'Profile default' },
  { value: 'hqdn3d=1.5:1.5:6:6', label: 'Light noise cleanup' },
  { value: 'hqdn3d=2:2:7:7', label: 'Medium noise cleanup' },
  { value: 'deband=1thr=0.018:2thr=0.018:3thr=0.018:4thr=0.018', label: 'Light banding cleanup' },
  { value: 'hqdn3d=1.5:1.5:6:6,deband=1thr=0.018:2thr=0.018:3thr=0.018:4thr=0.018', label: 'Anime cleanup' },
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
  onChangeMany,
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
  onChangeMany: (patch: Partial<AssetConversionOverrideState>) => void;
  onSave: () => void;
  onReset: () => void;
  saving: boolean;
  readOnly?: boolean;
}) {
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const runtimeSnapshot = useQuery({ queryKey: ['runtime-snapshot'], queryFn: api.runtimeSnapshot });
  const workerNodes = useQuery({ queryKey: ['worker-nodes'], queryFn: api.workerNodes });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const [lastEncoderRecommendation, setLastEncoderRecommendation] =
    useState<{
      path: string;
      data: QualityRecommendationResponse;
    }>();

  const encoderQualityRecommendation = useMutation({
    mutationFn: api.recommendEncoderQuality,
    onSuccess: (data, variables) => {
      if (!variables.path) return;

      setLastEncoderRecommendation({
        path: variables.path,
        data,
      });
    },
  });
  const recommendedCrop = ['detected', 'variable'].includes(scan?.cropAnalysis?.status ?? '') ? (scan?.cropAnalysis?.recommendedCrop ?? '').trim() : '';
  const manualCropCandidate = scan?.cropAnalysis?.status === 'variable' && Boolean(recommendedCrop);
  const currentCropFilter = cropFilterFromChain(draft.videoFilters);
  const suggestedCropFilter = recommendedCrop ? `crop=${recommendedCrop}` : '';
  const suggestedCropEnabled = Boolean(suggestedCropFilter) && currentCropFilter === suggestedCropFilter;
  const assetMotionModes = draft.fieldStructureMode || draft.cadenceMode || draft.deinterlaceMode
    ? semanticMotionModes(draft as Record<string, unknown>)
    : { fieldStructureMode: '', cadenceMode: '', cadenceFieldOrder: '' };
  const bitmapSubtitleCount = scan?.subtitleStreams.filter((stream) => isBitmapSubtitleCodec(stream.codec)).length ?? 0;
  const processingPreference = draft.preferredEncoder ?? '';
  const assetWorker = resolveSelectedWorker(workerNodes.data, draft.targetWorkerName ?? String(profile?.workerConfig?.targetWorkerName ?? ''));
  const assetWorkerEncoders = encoderNamesForWorker(assetWorker);
  const preferencesInitializedForPath = useRef<string>('');

    useEffect(() => {
      if (readOnly) return;

      if (settings.isLoading || workerNodes.isLoading) {
        return;
      }

      // Only initialize once for this open asset.
      if (preferencesInitializedForPath.current === assetPath) {
        return;
      }

      const preferenceDraft = assetOverridePreferenceDraft(
        getMVForgePreferences(settings.data),
        assetWorkerEncoders,
      );

      const missingPreferences =
        Object.fromEntries(
          Object.entries(preferenceDraft).filter(([key, value]) => {
            if (value === undefined) {
              return false;
            }

            const currentValue =
              draft[key as keyof AssetConversionOverrideState];

            return currentValue === undefined;
          }),
        ) as Partial<AssetConversionOverrideState>;

      if (Object.keys(missingPreferences).length > 0) {
        onChangeMany(missingPreferences);
      }

      preferencesInitializedForPath.current = assetPath;
    }, [
      assetPath,
      draft,
      readOnly,
      settings.isLoading,
      settings.data,
      workerNodes.isLoading,
      assetWorkerEncoders,
      onChangeMany,
    ]);
  const availableAssetEncoderOptions = assetEncoderOptions.filter((option) => option.value === 'auto' || assetWorkerEncoders.has(option.value));
  const assetFrameRecommendation = recommendedFrameStructureForAsset(scan);
  const effectiveVideoCodec = draft.videoCodec || profile?.videoCodec || 'copy';
  const hardwareCodecSupported = hardwareEncodingSupportedForAssetCodec(effectiveVideoCodec);
  const hardwareSelected = processingPreference === 'hardware';
  const defaultHardwareEncoder = availableAssetEncoderOptions.find(
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
  const qsvBFramesDisabled = String(draft.frameStructureBFrameMode ?? profile?.workerConfig?.frameStructureBFrameMode ?? 'auto') === 'off';
  const qsvWarnings = qsvSelectionWarnings(qsvFeatures, {
    extendedBRC: draft.qsvExtendedBrc ?? profile?.workerConfig?.qsvExtendedBRC === true,
    adaptiveI: draft.qsvAdaptiveI ?? profile?.workerConfig?.qsvAdaptiveI === true,
    adaptiveB: !qsvBFramesDisabled && (draft.qsvAdaptiveB ?? profile?.workerConfig?.qsvAdaptiveB === true),
  });

  const videoToolboxCapability = runtimeSnapshot.data?.encoders?.hevc_videotoolbox;
  const selectedHardwareQualityPreset = String(draft.hardwareQualityPreset ?? profile?.workerConfig?.hardwareQualityPreset ?? 'recommended');
  const effectiveHardwarePresetConfig = selectedHardwareQualityPreset !== 'custom'
    ? applySharedHardwareQualityPreset({ ...(profile?.workerConfig ?? {}), ...draft }, effectiveVideoEncoder, selectedHardwareQualityPreset)
    : { ...(profile?.workerConfig ?? {}), ...draft };
  const effectivePixFmt = (draft.pixFmt || stringFromRecord(profile?.workerConfig ?? {}, 'pixFmt')).toLowerCase();
  const videoToolboxMain10Selected = (draft.videoToolboxProfile ?? String(profile?.workerConfig?.videoToolboxProfile ?? '')).toLowerCase() === 'main10'
    || ['p010le', 'yuv420p10le'].includes(effectivePixFmt);
  const videoToolboxPowerAvailable = videoToolboxCapability?.videoToolboxPowerEfficient === true
    && (!videoToolboxMain10Selected || videoToolboxCapability.testedModes?.videoToolboxPowerEfficientMain10 === true);
  const effectiveEvaluationProfile = assetQualityProfile(profile, draft, effectiveVideoEncoder, String(draft.hardwareQualityPreset ?? profile?.workerConfig?.hardwareQualityPreset ?? 'custom'));
  const effectiveEvaluationSignature = JSON.stringify(effectiveEvaluationProfile);
  const displayedEncoderRecommendation =
    lastEncoderRecommendation?.path === assetPath
      ? lastEncoderRecommendation.data
      : undefined;

  const encoderRecommendationUpdating =
    Boolean(displayedEncoderRecommendation) &&
    encoderQualityRecommendation.isPending;
  useEffect(() => {
    const timer = window.setTimeout(() => encoderQualityRecommendation.mutate({ path: assetPath, profile: effectiveEvaluationProfile }), 250);
    return () => window.clearTimeout(timer);
  }, [assetPath, effectiveEvaluationSignature]);

  function selectHardwareQualityPreset(
  preset: string,
  encoder: string,
  draftOverride?: AssetConversionOverrideState,
  ) {
    onChange('hardwareQualityPreset', preset);

    if (preset === 'custom') return;

    if (encoder === 'hevc_qsv') {
      onChange('qsvRateControl', 'icq');
    }

    const effectiveDraft = draftOverride ?? draft;

    const requested = assetQualityProfile(
      profile,
      effectiveDraft,
      encoder,
      preset,
    );

    encoderQualityRecommendation.mutate(
      { path: assetPath, profile: requested },
      {
        onSuccess: (result) =>
          applyEffectiveProfileToAssetOverrides(
            onChangeMany,
            result.effectiveProfile,
          ),
      },
    );
  }

  function updateVideoToolboxCustomRate(key: 'videoToolboxBitrateMbps' | 'videoToolboxMaxrateMbps' | 'videoToolboxBufferMbps', value: number) {
    const rates = key === 'videoToolboxBitrateMbps' ? videoToolboxRatesFromTargetMbps(value) : null;
    const patch: Partial<AssetConversionOverrideState> = {
      [key]: value,
      ...(rates ? { videoToolboxBitrateMbps: rates.target, videoToolboxMaxrateMbps: rates.maxrate, videoToolboxBufferMbps: rates.buffer } : {}),
      hardwareQualityPreset: 'custom',
    };
    Object.entries(patch).forEach(([name, setting]) => onChange(name as keyof AssetConversionOverrideState, setting as never));
    const requested = assetQualityProfile(profile, { ...draft, ...patch }, effectiveVideoEncoder, 'custom');
    encoderQualityRecommendation.mutate({ path: assetPath, profile: requested });
  }

  function updateFrameStructurePolicy(patch: Record<string, unknown>) {
    const retainedQualityPreset = String(draft.hardwareQualityPreset ?? profile?.workerConfig?.hardwareQualityPreset ?? 'custom');
    Object.entries(patch).filter(([name]) => name !== 'frameStructureMode').forEach(([name, setting]) => onChange(name as keyof AssetConversionOverrideState, setting as never));
    if ('frameStructureMode' in patch) onChange('frameStructureMode', patch.frameStructureMode as AssetConversionOverrideState['frameStructureMode']);
    onChange('hardwareQualityPreset', retainedQualityPreset);
    if (effectiveVideoEncoder === 'hevc_qsv' || effectiveVideoEncoder === 'hevc_videotoolbox') {
      const requested = assetQualityProfile(profile, { ...draft, ...patch }, effectiveVideoEncoder, String(draft.hardwareQualityPreset ?? profile?.workerConfig?.hardwareQualityPreset ?? 'custom'));
      encoderQualityRecommendation.mutate({ path: assetPath, profile: requested });
    }
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
      onChange('qsvPStrategy', undefined);
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
      onChange('qsvPStrategy', undefined);
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
            <Button variant="text" onClick={() => onChangeMany(assetOverridePreferenceDraft(getMVForgePreferences(settings.data), assetWorkerEncoders))} disabled={saving || readOnly || settings.isLoading}>
              Load preferences
            </Button>
            <Button variant="outlined" onClick={onReset} disabled={saving || readOnly}>
              Remove
            </Button>
            <Button variant="contained" onClick={onSave} disabled={saving || readOnly}>
              Save
            </Button>
          </Stack>
        </Stack>
        <Divider />
        <Alert severity="info">
          <strong>copy</strong> keeps the original video stream untouched. Choose x264/x265 when you need smaller files, compatibility, or image filters. Blank override values continue using the related profile.
        </Alert>
        <Box component="fieldset" disabled={readOnly} sx={{ border: 0, p: 0, m: 0, minWidth: 0 }}>
        <Grid container spacing={2}>
          <Grid size={{ xs: 12 }}>
            <Stack spacing={0.35}>
              <Typography fontWeight={700}>Technical settings</Typography>
              <Typography variant="body2" color="text.secondary">These values override only this asset and follow the same video pipeline used by Profile Lab.</Typography>
            </Stack>
          </Grid>
          <Grid size={{ xs: 12, md: 4 }}>
            <TextField select label="Quality / storage intent" value={draft.optimizationIntent ?? ''} onChange={(event) => onChange('optimizationIntent', (event.target.value || undefined) as AssetConversionOverrideState['optimizationIntent'])} fullWidth>
              <MenuItem value="">Inherit from profile</MenuItem>
              <MenuItem value="maximum_savings">Maximum space saving</MenuItem>
              <MenuItem value="balanced">Balanced</MenuItem>
              <MenuItem value="conservative">Conservative quality</MenuItem>
              <MenuItem value="maximum_quality">Maximum quality</MenuItem>
              <MenuItem value="archive">Archive</MenuItem>
            </TextField>
          </Grid>
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
              label="Encoding speed"
              value={draft.videoPreset ?? ''}
              onChange={(event) => onChange('videoPreset', event.target.value)}
              helperText={draft.videoPreset
                ? draft.videoPreset === 'veryfast'
                  ? 'Faster conversions and larger files. Useful for quick tests.'
                  : draft.videoPreset === 'fast'
                    ? 'Faster than Balanced while retaining more compression efficiency than Fast preview.'
                  : draft.videoPreset === 'medium'
                    ? 'Recommended balance of quality, size, and speed.'
                    : draft.videoPreset === 'slow'
                      ? 'Slower, usually smaller files at the same quality.'
                      : 'Very slow. Use when size matters and time is acceptable.'
                : 'Uses the encoding speed from the selected video profile.'}
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
          <Grid size={{ xs: 12 }}>
            <Stack spacing={0.35} sx={{ mb: 1 }}>
              <Typography fontWeight={700}>Image cleanup</Typography>
              <Typography variant="body2" color="text.secondary">Crop, field handling, and cleanup filters are applied to this asset only.</Typography>
            </Stack>
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
          <FrameCadenceControls
            {...assetMotionModes}
            scan={scan}
            allowProfileDefault
            onFieldStructureChange={(value) => onChange('fieldStructureMode', value || undefined)}
            onCadenceChange={(value) => onChange('cadenceMode', value || undefined)}
            onCadenceFieldOrderChange={(value) => onChange('cadenceFieldOrder', value || undefined)}
          />
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
                <TextField
                  select
                  label="Crop aspect handling"
                  value={draft.cropAspectPolicy ?? (stringFromRecord(profile?.workerConfig ?? {}, 'cropAspectPolicy') || 'source_sar')}
                  onChange={(event) => onChange('cropAspectPolicy', event.target.value as AssetConversionOverrideState['cropAspectPolicy'])}
                  disabled={!currentCropFilter}
                  helperText="Preserve source SAR is recommended; preserving the old DAR may stretch the cropped image."
                  size="small"
                  sx={{ maxWidth: 360 }}
                >
                  <MenuItem value="source_sar">Preserve source SAR (recommended)</MenuItem>
                  <MenuItem value="preserve_dar">Preserve original DAR</MenuItem>
                </TextField>
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
              <Stack spacing={0.35} sx={{ mb: 2 }}>
                <Typography fontWeight={700}>Encoder and frame structure</Typography>
                <Typography variant="body2" color="text.secondary">Requested settings are validated against the selected worker; effective values may be downgraded when the runtime does not support them.</Typography>
              </Stack>
              <Grid container spacing={2} alignItems="flex-start">
                <Grid size={{ xs: 12, md: 4 }}><TextField select label="Execution worker" value={assetWorker?.name ?? ''} onChange={(event) => onChange('targetWorkerName', event.target.value)} helperText={assetWorker ? `${assetWorkerEncoders.size} usable encoder(s) reported` : 'No online worker is reporting encoders'} size="small" fullWidth>{(workerNodes.data ?? []).filter((worker) => worker.status === 'online').map((worker) => <MenuItem key={worker.id} value={worker.name}>{worker.name} · {encoderNamesForWorker(worker).size} encoders</MenuItem>)}</TextField></Grid>
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
                    <MenuItem value="software" disabled={!assetWorkerEncoders.has(softwareEncoderForAssetCodec(effectiveVideoCodec))}>Software · match Video Codec</MenuItem>
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
                      const encoder = event.target.value;

                      const pixFmt =
                        defaultHardwareMain10PixelFormatForAsset(encoder);

                      const nextDraft: AssetConversionOverrideState = {
                        ...draft,
                        videoEncoder: encoder,
                        pixFmt,
                      };

                      onChange('videoEncoder', encoder);
                      onChange('pixFmt', pixFmt);

                      selectHardwareQualityPreset(
                        'recommended',
                        encoder,
                        nextDraft,
                      );
                    }}
                    helperText="Only encoders reported by the current runtime are selectable."
                    size="small"
                    fullWidth
                  >
                    {availableAssetEncoderOptions.filter((option) => isHardwareAssetEncoder(option.value)).map((option) => (
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
                {hardwareSelected ? <Grid size={{ xs: 12, md: 4 }}><TextField title="The backend translates this intent using the active worker probe." select label="Quality preset" value={String(draft.hardwareQualityPreset ?? profile?.workerConfig?.hardwareQualityPreset ?? 'recommended')} onChange={(event) => selectHardwareQualityPreset(event.target.value, effectiveVideoEncoder)} helperText="Recommended and Best use Main10 when supported by the selected worker." size="small" fullWidth>{hardwareQualityPresetOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}</TextField></Grid> : null}
                <Grid size={{ xs: 12 }}>
                  <FrameStructureControls
                    config={{ ...(profile?.workerConfig ?? {}), ...draft }}
                    recommendedGop={assetFrameRecommendation?.gop}
                    recommendedBFrames={assetFrameRecommendation?.bFrames}
                    recommendedGopByMode={assetFrameRecommendation?.gopByMode}
                    frameRate={assetFrameRecommendation?.fps}
                    onChange={(key, value) => onChange(key as keyof AssetConversionOverrideState, value as never)}
                    onChangeMany={updateFrameStructurePolicy}
                    encoder={hardwareSelected ? effectiveVideoEncoder : softwareEncoderForAssetCodec(draft.videoCodec ?? profile?.videoCodec ?? 'x265')}
                    disabled={readOnly || (draft.videoCodec ?? profile?.videoCodec) === 'copy'}
                    compact
                  />
                </Grid>
                {['libx265', 'hevc_qsv'].includes(hardwareSelected ? effectiveVideoEncoder : softwareEncoderForAssetCodec(draft.videoCodec ?? profile?.videoCodec ?? 'x265')) ? (
                  <Grid size={{ xs: 12 }}>
                    <HEVCLevelControls
                      config={{ ...(profile?.workerConfig ?? {}), ...draft }}
                      recommendation={scan?.hevcLevelRecommendation}
                      onChange={(key, value) => onChange(key as keyof AssetConversionOverrideState, value as never)}
                      onChangeMany={(patch) => onChangeMany(patch as Partial<AssetConversionOverrideState>)}
                      encoder={hardwareSelected ? effectiveVideoEncoder : softwareEncoderForAssetCodec(draft.videoCodec ?? profile?.videoCodec ?? 'x265')}
                      disabled={readOnly || (draft.videoCodec ?? profile?.videoCodec) === 'copy'}
                      compact
                    />
                  </Grid>
                ) : null}
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
                  <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField label="P strategy" select value={draft.qsvPStrategy ?? Number(profile?.workerConfig?.qsvPStrategy ?? 0)} onChange={(event) => onChange('qsvPStrategy', Number(event.target.value) as 0 | 1 | 2)} helperText="Available only when B-frames are Off and validated by the worker." size="small" fullWidth><MenuItem value={0}>Default</MenuItem><MenuItem value={1} disabled={String(draft.frameStructureBFrameMode ?? profile?.workerConfig?.frameStructureBFrameMode ?? 'auto') !== 'off' || !qsvPStrategySupported(qsvCapability, qsvMain10Selected, 1)}>Simple · requires Off</MenuItem><MenuItem value={2} disabled={String(draft.frameStructureBFrameMode ?? profile?.workerConfig?.frameStructureBFrameMode ?? 'auto') !== 'off' || !qsvPStrategySupported(qsvCapability, qsvMain10Selected, 2)}>Pyramid · requires Off</MenuItem></TextField></Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                    <TextField select label="QSV rate control" value={draft.qsvRateControl || stringFromRecord(profile?.workerConfig ?? {}, 'qsvRateControl') || 'icq'} onChange={(event) => onChange('qsvRateControl', event.target.value as 'icq' | 'la_icq')} size="small" fullWidth>
                      <MenuItem value="icq">ICQ</MenuItem>
                      <MenuItem value="la_icq" disabled={!qsvFeatures.rateControls.laIcq}>LA-ICQ · worker validation required</MenuItem>
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
                          disabled={qsvBFramesDisabled || (!qsvFeatures.adaptiveB && !(draft.qsvAdaptiveB ?? profile?.workerConfig?.qsvAdaptiveB === true))}
                          checked={!qsvBFramesDisabled && (draft.qsvAdaptiveB ?? profile?.workerConfig?.qsvAdaptiveB === true)}
                          onChange={(event) => onChange('qsvAdaptiveB', event.target.checked)
                        }
                        />
                      } label={qsvBFramesDisabled ? 'Adaptive B · disabled by BF0' : 'Adaptive B'} />
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
                    <TextField title="Average target bitrate. Higher values preserve more detail and create larger files." label="VideoToolbox bitrate (Mbps)" type="number" value={Number(effectiveHardwarePresetConfig.videoToolboxBitrateMbps ?? 2)} onChange={(event) => updateVideoToolboxCustomRate('videoToolboxBitrateMbps', Number(event.target.value))} helperText="Updates maxrate ×1.5, buffer ×2.5, and the effective estimate." inputProps={{ min: 0.01, max: 200, step: 0.01 }} size="small" fullWidth />
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <TextField title="Maximum short-term bitrate allowed during complex scenes." label="VideoToolbox maxrate (Mbps)" type="number" value={Number(effectiveHardwarePresetConfig.videoToolboxMaxrateMbps ?? 3)} onChange={(event) => updateVideoToolboxCustomRate('videoToolboxMaxrateMbps', Number(event.target.value))} inputProps={{ min: 0.01, max: 250, step: 0.01 }} size="small" fullWidth />
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <TextField title="Rate-control buffer. Larger values give the encoder more freedom in complex scenes." label="VideoToolbox buffer (Mbps)" type="number" value={Number(effectiveHardwarePresetConfig.videoToolboxBufferMbps ?? 5)} onChange={(event) => updateVideoToolboxCustomRate('videoToolboxBufferMbps', Number(event.target.value))} inputProps={{ min: 0.01, max: 500, step: 0.01 }} size="small" fullWidth />
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}><TextField title="HEVC Main uses 8-bit output; Main10 uses 10-bit output and requires a compatible pixel format." label="Profile" value={draft.videoToolboxProfile ?? String(profile?.workerConfig?.videoToolboxProfile ?? '')} onChange={(event) => onChange('videoToolboxProfile', event.target.value)} placeholder="main or main10" helperText="Blank follows bit depth" size="small" fullWidth /></Grid>
                  <Grid size={{ xs: 12 }}><Stack direction="row" spacing={2} flexWrap="wrap"><FormControlLabel title="Off is the default for offline conversion. Enable only for explicit low-latency work." control={<Checkbox checked={draft.videoToolboxRealtime ?? profile?.workerConfig?.videoToolboxRealtime === true} onChange={(event) => onChange('videoToolboxRealtime', event.target.checked)} />} label="Realtime" /><FormControlLabel title="Adjust target, maxrate and buffer for the effective B-frame strategy." control={<Checkbox checked={draft.videoToolboxAutoAdjustBitrate ?? profile?.workerConfig?.videoToolboxAutoAdjustBitrate === true} onChange={(event) => onChange('videoToolboxAutoAdjustBitrate', event.target.checked)} />} label="Auto-adjust bitrate for encoder strategy" /><FormControlLabel title="Available only after the matching VideoToolbox Main/Main10 power-efficiency probe succeeds." control={<Checkbox disabled={!videoToolboxPowerAvailable} checked={draft.videoToolboxPowerEfficiency ?? profile?.workerConfig?.videoToolboxPowerEfficiency === true} onChange={(event) => onChange('videoToolboxPowerEfficiency', event.target.checked)} />} label="Power efficiency" /></Stack></Grid>
                </Grid>
              ) : null}
              {encoderQualityRecommendation.isError ? <Alert severity="warning">Quality recommendation failed: {encoderQualityRecommendation.error instanceof Error ? encoderQualityRecommendation.error.message : 'unknown error'}</Alert> : null}
              {displayedEncoderRecommendation ? (
                <Stack spacing={1} sx={{ mt: 1.5 }}>
                  <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                    <Chip size="small" label={`Effective · ${displayedEncoderRecommendation.recommendation.effectiveRateControl || 'bitrate'}`} />
                    <Chip size="small" label={`Confidence · ${displayedEncoderRecommendation.recommendation.estimateConfidence}`} />
                    {displayedEncoderRecommendation.recommendation.effectiveBFramePolicy ? <Chip size="small" label={`B-frames · ${displayedEncoderRecommendation.recommendation.requestedBFramePolicy} → ${displayedEncoderRecommendation.recommendation.effectiveBFramePolicy}`} /> : null}
                    {displayedEncoderRecommendation.recommendation.bFrameEfficiencyMultiplier ? <Chip size="small" label={`Efficiency · ×${displayedEncoderRecommendation.recommendation.bFrameEfficiencyMultiplier.toFixed(2)}`} /> : null}
                    {displayedEncoderRecommendation.recommendation.baseTargetBitrate ? <Chip size="small" label={`Base · ${(displayedEncoderRecommendation.recommendation.baseTargetBitrate / 1_000_000).toFixed(2)} Mbps`} /> : null}
                    {displayedEncoderRecommendation.recommendation.targetBitrate ? <Chip size="small" label={`Effective target · ${(displayedEncoderRecommendation.recommendation.targetBitrate / 1_000_000).toFixed(2)} Mbps`} /> : null}
                    {displayedEncoderRecommendation.estimatedOutputMaxBytes > 0 ? <Chip size="small" label={`Estimated output · ${formatEstimatedByteRange(
                        displayedEncoderRecommendation.estimatedOutputMinBytes,
                        displayedEncoderRecommendation.estimatedOutputMaxBytes,
                        formatBytes,
                      )}`} /> : null}
                    {displayedEncoderRecommendation.estimatedSavingsMaxBytes > 0 ? <Chip size="small" color="success" label={`Estimated saving · ${formatBytes(displayedEncoderRecommendation.estimatedSavingsMinBytes)}–${formatBytes(displayedEncoderRecommendation.estimatedSavingsMaxBytes)}`} /> : null}
                  </Stack>
                  {displayedEncoderRecommendation.recommendation.bFrameDowngradeReason ? <Alert severity="warning">{displayedEncoderRecommendation.recommendation.bFrameDowngradeReason}</Alert> : null}
                  <Typography component="code" variant="caption" sx={{ overflowWrap: 'anywhere' }}>FFmpeg video: {displayedEncoderRecommendation.ffmpegVideoArguments.join(' ')}</Typography>
                </Stack>
              ) : null}
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
  allowInherit = false,
  allowOverrideOnly = false,
  allowAudioOnly = false,
}: {
  profiles: Profile[];
  value: number;
  onChange: (id: number) => void;
  label: string;
  size?: 'small' | 'medium';
  disabled?: boolean;
  allowNone?: boolean;
  allowInherit?: boolean;
  allowOverrideOnly?: boolean;
  allowAudioOnly?: boolean;
}) {
  const options = [
    ...(allowInherit ? [{ id: 0, name: 'Inherit from path', videoCodec: 'Path profile', search: 'inherit path profile' }] : []),
    ...(allowOverrideOnly
    ? [{
        id: VIDEO_PROFILE_OVERRIDE_ONLY,
        name: 'Override only',
        videoCodec: 'Asset overrides',
        search: 'override only asset overrides no path profile',
      }]
    : []),

    ...(allowAudioOnly
    ? [{
        id: VIDEO_PROFILE_AUDIO_ONLY,
        name: 'Audio only',
        videoCodec: 'No video processing',
        search: 'audio only no video',
      }]
    : []),
    ...(allowNone ? [{ id: -1, name: 'Disabled', videoCodec: 'No video profile', search: 'disabled none no profile' }] : []),
    ...profiles.map((profile) => ({ id: profile.id, name: profile.name, videoCodec: profile.videoCodec, search: `${profile.name} ${profile.description} ${profile.container} ${profile.videoCodec} ${profile.audioCodec}` })),
  ];
  return (
    <Autocomplete
      options={options}
      value={options.find((profile) => profile.id === value) ?? null}
      onChange={(_, profile) => onChange(profile?.id ?? 0)}
      getOptionLabel={(profile) =>
        profile.id === 0
          ? 'Inherit from path'
		  : profile.id === -1 && allowNone && !allowOverrideOnly
			? 'Disabled'
          : profile.id === VIDEO_PROFILE_OVERRIDE_ONLY
            ? 'Override only'
            : profile.id === VIDEO_PROFILE_AUDIO_ONLY
              ? 'Audio only'
			  : profile.id === -3
				? 'Disabled'
				: `${profile.name} · ${profile.videoCodec}`
      }
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
  allowInherit = false,
}: {
  profiles: AudioEnhancementProfile[];
  value: string;
  onChange: (key: string) => void;
  label: string;
  size?: 'small' | 'medium';
  disabled?: boolean;
  allowInherit?: boolean;
}) {
  const none: AudioEnhancementProfile = {
    key: '', name: 'Disabled', description: '', intent: '', filters: '', rnnoiseModelPath: '', channelMode: 'preserve',
    forceStereoMode: 'auto', stereoDelayMs: 0, stereoWidth: 0, eqBands: {}, preserveOriginalTrack: true,
    outputCodec: 'copy', targetLoudness: 0, truePeak: 0, notes: '',
  };
  const inherit: AudioEnhancementProfile = { ...none, key: '__inherit__', name: 'Inherit from path' };
  const options = [...(allowInherit ? [inherit] : []), none, ...profiles];
  return (
    <Autocomplete
      options={options}
      value={options.find((profile) => profile.key === value) ?? none}
      onChange={(_, profile) => onChange(profile?.key ?? '')}
      getOptionLabel={(profile) => profile.key === '__inherit__' ? 'Inherit from path' : profile.key ? `${profile.name} · ${profile.outputCodec || 'copy'}` : 'Disabled'}
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

function TrackProfileAutocomplete({ profiles, value, onChange, disabled, label = 'Tracks for path', allowInherit = false }: { profiles: TrackProfile[]; value: string; onChange: (key: string) => void; disabled?: boolean; label?: string; allowInherit?: boolean }) {
  const none: TrackProfile = {
    key: '', name: 'Disabled', description: '', videoMode: 'first', audioMode: 'all', audioLanguages: [], audioRequired: false,
    dropCommentary: false, defaultAudioLanguage: '', subtitleMode: 'all', subtitleLanguages: [], subtitlesRequired: false,
    defaultSubtitleLanguage: '', validationMode: 'warn', notes: '',
  };
  const inherit: TrackProfile = { ...none, key: '__inherit__', name: 'Inherit from path' };
  const options = [...(allowInherit ? [inherit] : []), none, ...profiles];
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
	if (profile.scope !== 'path') {
	  checkIndexes('video', profile.keepVideoStreams, scan.videoStreams);
	  checkIndexes('audio', profile.keepAudioStreams, scan.audioStreams);
	  checkIndexes('subtitle', profile.keepSubtitleStreams, scan.subtitleStreams);
	}
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

function pathLabelForCollection(group: AssetGroup) {
	const asset = firstAssetForGroup(group.assets);
	if (asset?.logicalGroupPath) {
		const sourcePath = normalizePath(asset.sourcePath || asset.path);
		const logicalPath = normalizePath(asset.logicalGroupPath);
		const actualPath = sourcePath.includes('/') ? sourcePath.slice(0, sourcePath.lastIndexOf('/')) : sourcePath;
		if (actualPath === logicalPath) return 'Root';
		if (actualPath.startsWith(`${logicalPath}/`)) return actualPath.slice(logicalPath.length + 1);
	}
  const relative = cleanRelativePath(group.relativePath || groupDisplayPath(group));
  const parts = relative.split('/').filter(Boolean);
  if (parts.length <= 1) return 'Root';
  return parts.slice(1).join('/');
}

function pathAssignmentsFor(path: string, assignments: ProfileAssignment[]) {
  const normalizedPath = normalizePath(path);
  return new Map(
    assignments
      .filter((assignment) => assignment.targetType === 'path' && normalizePath(assignment.targetPath) === normalizedPath)
      .map((assignment) => [assignment.mediaType, assignment]),
  );
}

function effectivePathDestination(group: AssetGroup, destinations: Record<string, number>, mode: 'unprocessed' | 'library' | 'converted' | 'archive') {
  return destinations[normalizePath(group.path)] ?? (mode === 'library' ? group.libraryId : 0);
}

function pathConfigurationComplete(group: AssetGroup, assignments: ProfileAssignment[], destinations: Record<string, number>, mode: 'unprocessed' | 'library' | 'converted' | 'archive') {
  const pathAssignments = pathAssignmentsFor(group.path, assignments);
  const hasProfile = [...pathAssignments.values()].some((assignment) => assignment.selection === 'profile');
  return hasProfile && effectivePathDestination(group, destinations, mode) > 0;
}

function pathCanQueue(group: AssetGroup, mode: 'unprocessed' | 'library' | 'converted' | 'archive') {
  if (mode === 'unprocessed') return group.status === 'unprocessed';
  if (mode !== 'library') return false;
  if (group.status === 'accepted' || group.status === 'published_as_is') return false;
  return group.status === 'library' || group.status === 'unverified';
}

function configurationForPathSelection(groups: AssetGroup[], assignments: ProfileAssignment[], destinations: Record<string, number>, mode: 'unprocessed' | 'library' | 'converted' | 'archive') {
  const values = groups.map((group) => {
    const pathAssignments = pathAssignmentsFor(group.path, assignments);
    const video = pathAssignments.get('video');
    const audio = pathAssignments.get('audio');
    const tracks = pathAssignments.get('tracks');
    return {
      category: firstCategory(group.pathMetadata?.categories),
      videoProfileId: video?.selection === 'profile' ? video.videoProfileId ?? -1 : -1,
      audioProfileKey: audio?.selection === 'profile' ? audio.profileKey ?? '' : '',
      trackProfileKey: tracks?.selection === 'profile' ? tracks.profileKey ?? '' : '',
      destinationLibraryId: effectivePathDestination(group, destinations, mode),
    };
  });
  const first = values[0] ?? { category: '', videoProfileId: -1, audioProfileKey: '', trackProfileKey: '', destinationLibraryId: 0 };
  return {
    category: values.every((value) => value.category === first.category) ? first.category : '',
    videoProfileId: values.every((value) => value.videoProfileId === first.videoProfileId) ? first.videoProfileId : -1,
    audioProfileKey: values.every((value) => value.audioProfileKey === first.audioProfileKey) ? first.audioProfileKey : '',
    trackProfileKey: values.every((value) => value.trackProfileKey === first.trackProfileKey) ? first.trackProfileKey : '',
    destinationLibraryId: values.every((value) => value.destinationLibraryId === first.destinationLibraryId) ? first.destinationLibraryId : 0,
  };
}

function slugify(value: string) {
  return value.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'assets';
}

function archiveAssetsFromGroups(groups: AssetGroup[], query: string) {
  const cleanQuery = query.trim().toLowerCase();
  return safeArray(groups)
    .flatMap((group) => safeArray(group.assets).map((asset) => ({ asset, group })))
    .filter(({ asset, group }) => {
      if (!cleanQuery) return true;
      return [
        asset.path,
        asset.relativePath,
        asset.fileName,
        asset.status,
        asset.libraryName,
        group.path,
        group.relativePath,
        group.libraryName,
      ].some((value) => (value ?? '').toLowerCase().includes(cleanQuery));
    })
    .sort((left, right) => left.asset.path.localeCompare(right.asset.path));
}

function mediaAreaForGroup(group: AssetGroup, settings?: AppSetting[]) {
  const configuredPaths = settings?.find((setting) => setting.key === 'paths')?.value ?? {};
  const roots = ['rawRoot', 'libraryRoot', 'originalsArchivePath']
    .map((key) => typeof configuredPaths[key] === 'string' ? normalizePath(configuredPaths[key] as string) : '')
    .filter(Boolean)
    .sort((left, right) => right.length - left.length);
  const groupPath = normalizePath(group.path);
  for (const root of roots) {
    if (groupPath !== root && !groupPath.startsWith(`${root}/`)) continue;
    const parts = groupPath.slice(root.length).split('/').filter(Boolean);
    const archiveWrapper = parts[0];
    while (parts[0] === 'processed-originals' || parts[0] === 'library-replacements') parts.shift();
    if (archiveWrapper === 'library-replacements' && /^library-\d+$/i.test(parts[0] ?? '')) parts.shift();
    if (parts[0]) return parts[0].toLowerCase();
  }
  const relativeParts = normalizePath(group.relativePath).split('/').filter(Boolean);
  return relativeParts[0]?.toLowerCase() ?? '';
}

function sumGroupFiles(groups: AssetGroup[]) {
  return groups.reduce((total, group) => total + (group.fileCount || safeArray(group.assets).length), 0);
}

function countLabel(count: number, singular: string) {
	return `${count} ${singular}${count === 1 ? '' : 's'}`;
}

function sumGroupBytes(groups: AssetGroup[]) {
  return groups.reduce((total, group) => total + (Number.isFinite(group.sizeBytes) ? group.sizeBytes : 0), 0);
}

function sumAssetBytes(assets: Array<{ asset: Asset }>) {
  return assets.reduce((total, { asset }) => total + (Number.isFinite(asset.sizeBytes) ? asset.sizeBytes : 0), 0);
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
    case 'accepted':
      return 'Accepted';
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
    case 'accepted':
      return 'success';
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

void assetDisplayPath;

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
  const basePath = asset.status === 'converted' || asset.status === 'unverified' || asset.status === 'accepted' || asset.status === 'library' || asset.status === 'published_as_is' ? library?.destinationPath : library?.sourcePath;

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

void groupSubpath;

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
  const assetPath = normalizePath(asset.path);
  return safeArray(jobs).some((job) =>
    normalizePath(job.mediaPath) === assetPath && queueJobIsActive(job),
  );
}

function queueJobIsActive(job: QueueJob) {
  return job.status === 'queued' || job.status === 'running' || (job.status === 'completed' && !job.publishedAt);
}

function assetReviewApproved(asset: Asset) {
  return !asset.review?.requiresReview && asset.review?.source === 'manual' && Boolean(asset.review.updatedAt);
}

function associatedJobForAsset(asset: Asset, jobs: QueueJob[]) {
  const normalizedAssetPath = normalizePath(asset.path);
  return [...safeArray(jobs)]
    .sort((left, right) => right.id - left.id)
    .filter((job) => !job.publicationRetiredAt)
    .filter((job) => asset.status !== 'unprocessed' || job.status === 'queued' || job.status === 'running')
    .find((job) => {
      const candidates = [job.publishedPath, job.outputPath, job.mediaPath].map(normalizePath).filter(Boolean);
      return candidates.includes(normalizedAssetPath);
    });
}

function directPlayScoreLabel(job: QueueJob) {
  const directPlay = recordFromRecord(job.validationReport ?? {}, 'directPlay');
  const enabled = directPlay.enabled === true;
  const score = Number(directPlay.lowestScore);
  if (!enabled || !Number.isFinite(score)) return 'Not evaluated';
  return `${score}/100`;
}

function profileAssignmentSource(job: QueueJob | undefined, mediaType: 'video' | 'audio' | 'tracks') {
	if (!job?.profileResolution) return '';
	const resolution = recordFromRecord(job.profileResolution, mediaType);
	const source = String(resolution.source ?? '');
	return source === 'asset' ? ' · Asset' : source === 'path' ? ' · Path' : '';
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
  if (asset.status === 'accepted') {
    return { label: 'Accepted', color: 'success' };
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

function recommendedFrameStructureForAsset(scan?: ScanResult) {
  if (!scan) return undefined;

  const analysis = scan.frameStructureAnalysis;
  const fps = reliableFrameRateForScan(scan);

  if (!fps) return undefined;

  const sourceAverageGop =
    analysis && analysis.framesAnalyzed > 0
      ? analysis.averageGopLength
      : undefined;

  const confidence =
    analysis && analysis.framesAnalyzed > 0
      ? analysis.confidence
      : 'low';

  const recommendations = {
    compatible: assetDerivedGopRecommendation({
      fps,
      sourceAverageGop,
      confidence,
      mode: 'compatible',
    }),

    balanced: assetDerivedGopRecommendation({
      fps,
      sourceAverageGop,
      confidence,
      mode: 'balanced',
    }),

    maximum_compression: assetDerivedGopRecommendation({
      fps,
      sourceAverageGop,
      confidence,
      mode: 'maximum_compression',
    }),
  };

  const bFrames =
    analysis &&
    analysis.maxConsecutiveBFrames >= 1 &&
    analysis.maxConsecutiveBFrames <= 4
      ? analysis.maxConsecutiveBFrames
      : 3;

  return {
    fps,

    // AUTO intentionally uses Balanced.
    gop: recommendations.balanced.targetFrames,
    gopSeconds: recommendations.balanced.targetSeconds,

    bFrames,

    gopByMode: {
      compatible: recommendations.compatible.targetFrames,
      balanced: recommendations.balanced.targetFrames,
      maximum_compression:
        recommendations.maximum_compression.targetFrames,
    },
  };
}

function softwareEncoderForAssetCodec(codec: string) {
  const value = codec.toLowerCase();
  if (value.includes('264') || value === 'h264') return 'libx264';
  if (value.includes('av1')) return 'libsvtav1';
  if (value === 'copy') return 'copy';
  return 'libx265';
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
  if (normalized.videoCodec) {
    const legacyCodec = normalizeLegacyVideoCodec(
      normalized.videoCodec,
      {
        pixFmt: normalized.pixFmt,
        videoEncoder: normalized.videoEncoder,
        preferredEncoder: normalized.preferredEncoder,
        useHardwareIfAvailable:
          normalized.useHardwareIfAvailable,
      },
    );

    normalized.videoCodec = legacyCodec.videoCodec;

    if (
      typeof legacyCodec.workerConfig.pixFmt === 'string' &&
      legacyCodec.workerConfig.pixFmt.trim()
    ) {
      normalized.pixFmt =
        legacyCodec.workerConfig.pixFmt;
    }
  }
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
	'hevcLevel',
  ] as const).forEach((key) => {
    const text = value[key]?.trim();
    if (text) {
      clean[key] = text;
    }
  });
  if (value.cropAspectPolicy === 'source_sar' || value.cropAspectPolicy === 'preserve_dar') {
    clean.cropAspectPolicy = value.cropAspectPolicy;
  }
  if (value.optimizationIntent === 'maximum_savings' || value.optimizationIntent === 'balanced' || value.optimizationIntent === 'conservative' || value.optimizationIntent === 'maximum_quality' || value.optimizationIntent === 'archive') {
    clean.optimizationIntent = value.optimizationIntent;
  }
  if (value.externalSubtitleFormat === 'disabled' || value.externalSubtitleFormat === 'source' || value.externalSubtitleFormat === 'srt' || value.externalSubtitleFormat === 'ass' || value.externalSubtitleFormat === 'remove') {
    clean.externalSubtitleFormat = value.externalSubtitleFormat;
  }
  if (value.finalColorPolicy === 'automatic' || value.finalColorPolicy === 'preserve' || value.finalColorPolicy === 'normalize_bt709') {
    clean.finalColorPolicy = value.finalColorPolicy;
  }
  if (value.deinterlaceMode === 'auto' || value.deinterlaceMode === 'off' || value.deinterlaceMode === 'force' || value.deinterlaceMode === 'ivtc_tff' || value.deinterlaceMode === 'ivtc_bff') {
    clean.deinterlaceMode = value.deinterlaceMode;
  }
  if (value.fieldStructureMode === 'preserve' || value.fieldStructureMode === 'auto' || value.fieldStructureMode === 'deinterlace') clean.fieldStructureMode = value.fieldStructureMode;
  if (value.cadenceMode === 'preserve' || value.cadenceMode === 'auto' || value.cadenceMode === 'remove_soft_telecine' || value.cadenceMode === 'inverse_telecine') clean.cadenceMode = value.cadenceMode;
  if (value.cadenceFieldOrder === 'auto' || value.cadenceFieldOrder === 'tff' || value.cadenceFieldOrder === 'bff') clean.cadenceFieldOrder = value.cadenceFieldOrder;
  if (value.frameStructureMode === 'auto' || value.frameStructureMode === 'off' || value.frameStructureMode === 'compatible' || value.frameStructureMode === 'balanced' || value.frameStructureMode === 'maximum_compression' || value.frameStructureMode === 'custom') clean.frameStructureMode = value.frameStructureMode;
  if (value.frameStructureGopMode === 'auto' || value.frameStructureGopMode === 'recommended' || value.frameStructureGopMode === 'custom') clean.frameStructureGopMode = value.frameStructureGopMode;
  if (typeof value.frameStructureGopFrames === 'number' && Number.isFinite(value.frameStructureGopFrames) && value.frameStructureGopFrames > 0) clean.frameStructureGopFrames = Math.min(1000, Math.round(value.frameStructureGopFrames));
  if (value.frameStructureBFrameMode === 'auto' || value.frameStructureBFrameMode === 'recommended' || value.frameStructureBFrameMode === 'custom' || value.frameStructureBFrameMode === 'off') clean.frameStructureBFrameMode = value.frameStructureBFrameMode;
  if (value.hevcLevelMode === 'auto' || value.hevcLevelMode === 'recommended' || value.hevcLevelMode === 'custom') clean.hevcLevelMode = value.hevcLevelMode;
  if (typeof value.frameStructureMaxBFrames === 'number' && Number.isFinite(value.frameStructureMaxBFrames) && value.frameStructureMaxBFrames >= 0) clean.frameStructureMaxBFrames = Math.min(16, Math.round(value.frameStructureMaxBFrames));
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
  if (typeof value.qsvPStrategy === 'number' && Number.isInteger(value.qsvPStrategy) && value.qsvPStrategy >= 0 && value.qsvPStrategy <= 2) clean.qsvPStrategy = value.qsvPStrategy as 0 | 1 | 2;
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
	  scope: candidate.scope === 'path' ? 'path' : 'asset',
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
    optimizationIntent: draft.optimizationIntent ?? profile?.optimizationIntent,
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
      hardwareQualityPresetScale: 2,
    },
  };
}
function applyEffectiveProfileToAssetOverrides(
  onChangeMany: (
    patch: Partial<AssetConversionOverrideState>,
  ) => void,
  profile: Profile,
) {
  const config = profile.workerConfig ?? {};

  const patch: Partial<AssetConversionOverrideState> = {};

  const mapping: Array<
    [keyof AssetConversionOverrideState, unknown]
  > = [
    ['hardwareQualityPreset', config.hardwareQualityPreset],
    ['globalQuality', config.globalQuality],
    ['qsvRateControl', config.qsvRateControl],
    ['qsvLookAheadDepth', config.qsvLookAheadDepth],
    ['qsvExtendedBrc', config.qsvExtendedBRC],
    ['qsvAdaptiveI', config.qsvAdaptiveI],
    ['qsvAdaptiveB', config.qsvAdaptiveB],
    ['videoToolboxBitrateMbps', config.videoToolboxBitrateMbps],
    ['videoToolboxMaxrateMbps', config.videoToolboxMaxrateMbps],
    ['videoToolboxBufferMbps', config.videoToolboxBufferMbps],
    ['videoToolboxProfile', config.videoToolboxProfile],
    ['pixFmt', config.pixFmt],
  ];

  mapping.forEach(([key, value]) => {
    if (value !== undefined) {
      patch[key] = value as never;
    }
  });

  onChangeMany(patch);
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
