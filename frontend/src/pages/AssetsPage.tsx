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
  Menu,
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
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import PlaylistAddIcon from '@mui/icons-material/PlaylistAdd';
import RefreshIcon from '@mui/icons-material/Refresh';
import ReportProblemIcon from '@mui/icons-material/ReportProblem';
import SearchIcon from '@mui/icons-material/Search';
import TaskAltIcon from '@mui/icons-material/TaskAlt';
import DeleteForeverIcon from '@mui/icons-material/DeleteForever';
import DriveFileMoveIcon from '@mui/icons-material/DriveFileMove';
import DriveFileRenameOutlineIcon from '@mui/icons-material/DriveFileRenameOutline';
import EditIcon from '@mui/icons-material/Edit';
import ScienceIcon from '@mui/icons-material/Science';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import SettingsOutlinedIcon from '@mui/icons-material/SettingsOutlined';
import InsertDriveFileOutlinedIcon from '@mui/icons-material/InsertDriveFileOutlined';
import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query';
import { Component, Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import type { ErrorInfo, MouseEvent, ReactNode } from 'react';
import { Link as RouterLink, useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import { filterAssetGroups, subtitleOCRReviewKey, subtitleOCRSidecarSelection, subtitleOperationStopsPolling } from '../utils/assetPageHelpers';
import { MediaSnapshotDetails } from '../components/MediaSnapshotDetails';
import { TrackMaintenancePanel } from '../components/TrackMaintenancePanel';
import { TestEncodeDialog } from '../components/TestEncodeDialog';
import { LibraryPathRenameDialog } from '../components/LibraryPathRenameDialog';
import { FrameStructureControls } from '../components/FrameStructureControls';
import { FrameCadenceControls } from '../components/FrameCadenceControls';
import { semanticMotionModes } from '../utils/motionModes';
import { HEVCLevelControls } from '../components/HEVCLevelControls';
import { SmartUpscaleControls } from '../components/SmartUpscaleControls';
import { AssetsHierarchyPanel, AssetsSummaryStrip, SelectedAssetDetail, WorkspaceBreadcrumbs } from '../components/AssetsWorkspace';
import { AssetsInventoryTable } from '../components/AssetsInventoryTable';
import { AssetsV2PathToolbar } from '../components/AssetsV2PathToolbar';
import { AssetsV2InventoryControls } from '../components/AssetsV2InventoryControls';
import { assetLabHref } from '../components/assetsActionRoutes';
import { assetPathWithin, formatDurationSummary, type LibraryTreeSelection, type RawHierarchySelection } from '../components/assetsWorkspaceModel';
import { ProfileSuggestionCard } from '../components/ProfileSuggestionCard';
import { SourceBitrateQualityEvidence } from '../components/SourceBitrateQualityEvidence';
import { SubtitleTranslationDialog } from '../components/SubtitleTranslationDialog';
import { SubtitleOCRReviewDialog } from '../components/SubtitleOCRReviewDialog';
import { SubtitleCompositionEligibilityPanel } from '../components/SubtitleCompositionEligibilityPanel';
import { BatchPreflightDialog } from '../components/BatchPreflightDialog';
import { invalidateQueueBatchPreflightReference, getSavedQueueBatchPreflightCandidateID, openQueueBatchPreflight, QUEUE_BATCH_PREFLIGHT_OPEN_EVENT } from '../utils/queueBatchPreflightEvents';
import type { AdvisorFinding, AdvisorResponse, AppSetting, Asset, AssetArchivedOriginalProjection, AssetConversionOverrideState, AssetGroup, AssetInventory, AssetLogicalGroup, AssetSourceGroup, AssetMetadataSidecar, AssetTracksUpdateInput, AudioEnhancementProfile, CanonicalOCRReviewTarget, EffectiveAssetConfiguration, ExternalSubtitle, Library, MediaStreamInfo, Profile, ProfileAssignment, ProfileInput, QueueBatchPreflightResponse, QueueJob, QueueJobInput, QueueSelectedAssetsResponse, QualityRecommendationResponse, ScanResult, SnapshotOperation, StreamMetadataOverride, SubtitleCompositionInput, SubtitleExtractionOperation, SubtitleOCRQualityFinding, SubtitleOCRQualityReport, SubtitleSemanticRole, SubtitleSourceEligibility, SubtitleTranslationSourceSelector, UpscaleMode, UpscaleSharpen } from '../api/types';
import { getTrackProfiles, subtitleSRTFallbackEnabled, type TrackProfile } from '../trackProfiles';
import { qsvQualityHelper, qsvQualityRangeForCrf } from '../utils/qsv';
import { applyHardwareQualityPreset as applySharedHardwareQualityPreset, hardwareQualityPresetOptions, qsvAssetQualitySummary } from '../utils/hardwareQualityPresets';
import { qsvPStrategySupported, qsvSelectionWarnings, resolveQSVFeatures } from '../utils/qsvCapabilities';
import { videoToolboxRatesFromTargetMbps } from '../utils/videoToolboxRates';
import { encoderNamesForWorker, selectedWorker as resolveSelectedWorker } from '../utils/workerEncoders';
import { assetOverridePreferenceDraft, getMVForgePreferences } from '../mvforgePreferences';
import { formatEstimatedByteRange } from '../utils/qualityEstimate';
import { normalizeLegacyVideoCodec } from '../utils/videoCodec';
import { withStreamSelection } from '../utils/assetTrackSelection';
import { assetInfoDiscardDraft, assetInfoDocumentsEqual, assetInfoEditDocument, assetInfoHydrationReconciliation, assetInfoRefreshReconciliation, assetInfoSaveFailureReconciliation, assetInfoSaveReconciliation, assetInfoTracksSaveReconciliation, assetTracksPatch, mergeAssetTracks, type AssetInfoEditDocument } from '../utils/assetInfoEditing';
import { hasNonTerminalQueueLifecycle, originalArchiveStatusLabel } from '../utils/queueLifecycle';
import { patchAssetInventoryAsset } from '../utils/assetInventoryPatch';
import { effectiveVideoProfileLabel, hidePreConversionActionsForAsset, testEncodeConfigurationReason, testEncodeEligibleAsset } from '../utils/testEncodeEligibility';
import { customUpscaleHeightError, smartUpscaleControlsDisabled } from '../upscale';
import { hierarchicalSelectionState, selectableAssetsForPaths, selectedSelectableAssetsForLogicalGroups } from '../utils/unprocessedAssetSelection';
import {
  mergedScopeConfigurationInput,
  profileAssignmentForScopeChange,
  scopeConfigurationEditorValues,
  type ScopeConfigurationField,
} from '../utils/scopeConfigurationEditor';

const VIDEO_PROFILE_OVERRIDE_ONLY = -1;
const VIDEO_PROFILE_AUDIO_ONLY = -2;

function rawSelectionFromContext(sourceGroups: AssetSourceGroup[], query: string, legacyAssetPath: string, sourceGroupParam: string | null, logicalGroupParam: string | null, pathParam: string | null, rawScopeParam: string | null): RawHierarchySelection {
  if (rawScopeParam === 'all') return { kind: 'all' };
  const requestedSourceGroupId = sourceGroupParam ? Number(sourceGroupParam) : null;
  const sourceGroup = requestedSourceGroupId === null ? undefined : sourceGroups.find((candidate) => candidate.id === requestedSourceGroupId);
  const requestedLogicalGroup = logicalGroupParam
    ? sourceGroups.flatMap((candidate) => candidate.logicalGroups.map((logicalGroup) => ({ candidate, logicalGroup })))
      .find(({ candidate, logicalGroup }) => (!sourceGroup || candidate.id === sourceGroup.id) && logicalGroup.id === logicalGroupParam)
    : undefined;
  const selectionSourceGroup = sourceGroup ?? requestedLogicalGroup?.candidate;
  if (selectionSourceGroup) {
    const logicalGroup = requestedLogicalGroup?.candidate.id === selectionSourceGroup.id
      ? requestedLogicalGroup.logicalGroup
      : undefined;
    const path = pathParam && logicalGroup
      ? logicalGroup.assetPaths.find((candidate) => candidate.id === pathParam || normalizePath(candidate.path) === normalizePath(pathParam))
      : undefined;
    if (path && logicalGroup) return { kind: 'path', sourceGroupId: selectionSourceGroup.id, logicalGroupId: logicalGroup.id, pathId: path.id };
    if (logicalGroup) return { kind: 'logical', sourceGroupId: selectionSourceGroup.id, logicalGroupId: logicalGroup.id };
    return { kind: 'source', sourceGroupId: selectionSourceGroup.id };
  }

  if (legacyAssetPath) {
    const linked = sourceGroups.flatMap((candidate) => candidate.logicalGroups.flatMap((logicalGroup) => logicalGroup.assetPaths.map((path) => ({ candidate, logicalGroup, path }))))
      .find(({ path }) => path.assets.some((asset) => normalizePath(asset.path) === normalizePath(legacyAssetPath)));
    if (linked) return { kind: 'path', sourceGroupId: linked.candidate.id, logicalGroupId: linked.logicalGroup.id, pathId: linked.path.id };
  }

  const cleanQuery = query.trim().toLowerCase();
  const firstLogical = sourceGroups
    .map((candidate) => ({ candidate, logicalGroup: candidate.logicalGroups.find((item) => sourceLogicalGroupMatches(item, cleanQuery)) }))
    .find((item) => item.logicalGroup)?.logicalGroup;
  if (firstLogical) {
    const candidate = sourceGroups.find((source) => source.logicalGroups.some((item) => item.id === firstLogical.id));
    if (candidate) return { kind: 'logical', sourceGroupId: candidate.id, logicalGroupId: firstLogical.id };
  }
  return { kind: 'all' };
}

function librarySelectionFromContext(libraries: Library[], groups: AssetGroup[], libraryIdParam: string | null, libraryPathParam: string | null): LibraryTreeSelection {
  const id = libraryIdParam ? Number(libraryIdParam) : null;
  if (id === null || !Number.isInteger(id) || (!libraries.some((library) => library.id === id) && !groups.some((group) => group.libraryId === id))) return { libraryId: null, path: '' };
  return { libraryId: id, path: libraryPathParam ?? '' };
}

export function AssetsPage() {
  const [queuePreflightCandidate, setQueuePreflightCandidate] = useState<QueueBatchPreflightResponse | null>(null);
  const [savedQueuePreflightID, setSavedQueuePreflightID] = useState(() => getSavedQueueBatchPreflightCandidateID());
  const [queuePreflightResumeMessage, setQueuePreflightResumeMessage] = useState('');
  const [queuePreflightResumePending, setQueuePreflightResumePending] = useState(false);
  const queuePreflightRequestGeneration = useRef(0);
  const [searchParams, setSearchParams] = useSearchParams();
  const legacyTabValue = searchParams.get('tab') ?? '';
  const tab: 'raw' | 'library' = searchParams.get('location') === 'library' || ['library', 'converted', 'archive'].includes(legacyTabValue) ? 'library' : 'raw';
  const initialProvenance = searchParams.get('provenance') ?? (searchParams.get('tab') === 'converted' ? 'converted' : '');
  useEffect(() => {
    const handleCandidate = (event: Event) => {
      const candidate = (event as CustomEvent<QueueBatchPreflightResponse>).detail;
      if (candidate.status && candidate.status !== 'active') {
        invalidateQueueBatchPreflightReference(candidate.id);
        setSavedQueuePreflightID(getSavedQueueBatchPreflightCandidateID());
        return;
      }
      const savedID = getSavedQueueBatchPreflightCandidateID();
      if (savedID && savedID !== candidate.id) return;
      queuePreflightRequestGeneration.current += 1;
      setQueuePreflightResumePending(false);
      setQueuePreflightResumeMessage('');
      setSavedQueuePreflightID(savedID ?? candidate.id);
      setQueuePreflightCandidate(candidate);
    };
    window.addEventListener(QUEUE_BATCH_PREFLIGHT_OPEN_EVENT, handleCandidate);
    return () => {
      queuePreflightRequestGeneration.current += 1;
      window.removeEventListener(QUEUE_BATCH_PREFLIGHT_OPEN_EVENT, handleCandidate);
    };
  }, []);
  const resumeQueueBatchPreflight = async () => {
    const candidateID = getSavedQueueBatchPreflightCandidateID() ?? savedQueuePreflightID;
    if (!candidateID) {
      setSavedQueuePreflightID(null);
      setQueuePreflightResumeMessage('No saved Batch Preflight candidate is available. Start a new Preflight from Queue actions.');
      return;
    }
    const generation = ++queuePreflightRequestGeneration.current;
    setQueuePreflightResumePending(true);
    setQueuePreflightResumeMessage('');
    try {
      const candidate = await api.getQueueBatchPreflight(candidateID);
      const currentSavedID = getSavedQueueBatchPreflightCandidateID();
      if (generation !== queuePreflightRequestGeneration.current || (currentSavedID !== null ? currentSavedID !== candidateID : savedQueuePreflightID !== candidateID)) return;
      if (candidate.status !== 'active') {
        invalidateQueueBatchPreflightReference(candidateID);
        setSavedQueuePreflightID(getSavedQueueBatchPreflightCandidateID());
        setQueuePreflightResumeMessage(candidate.status === 'discarded'
          ? 'This candidate was discarded and cannot be resumed. Start a new Preflight when ready.'
          : 'This candidate is complete and has no pending assets.');
        return;
      }
      setSavedQueuePreflightID(candidateID);
      setQueuePreflightCandidate(candidate);
    } catch (caught) {
      const currentSavedID = getSavedQueueBatchPreflightCandidateID();
      if (generation !== queuePreflightRequestGeneration.current || (currentSavedID !== null ? currentSavedID !== candidateID : savedQueuePreflightID !== candidateID)) return;
      const status = caught instanceof Error ? (caught as Error & { status?: number }).status : undefined;
      if (status === 404 || status === 410) {
        invalidateQueueBatchPreflightReference(candidateID);
        setSavedQueuePreflightID(getSavedQueueBatchPreflightCandidateID());
        setQueuePreflightResumeMessage('The saved candidate no longer exists. Start a new Preflight from Queue actions.');
      } else {
        setQueuePreflightResumeMessage(`Could not resume Batch Preflight: ${caught instanceof Error ? caught.message : String(caught)}`);
      }
    } finally {
      if (generation === queuePreflightRequestGeneration.current) setQueuePreflightResumePending(false);
    }
  };
  const originalFilterValue = searchParams.get('original') ?? '';
  const originalFilter = originalFilterValue === 'archived' ? '' : originalFilterValue;
  const [libraryDecision, setLibraryDecision] = useState<string>(initialProvenance);
  const [validationFilter, setValidationFilter] = useState(searchParams.get('validation') ?? '');
  const [workflowFilter, setWorkflowFilter] = useState(searchParams.get('workflow') ?? '');
  const [attentionFilter, setAttentionFilter] = useState(searchParams.get('attention') ?? '');
  const [assetQuery, setAssetQuery] = useState(searchParams.get('search') ?? searchParams.get('asset') ?? '');
  const [mediaArea, setMediaArea] = useState(searchParams.get('mediaArea') ?? '');
  const activeFilterCount = [libraryDecision, mediaArea, validationFilter, originalFilter, workflowFilter, attentionFilter].filter(Boolean).length;
  const legacyTab = searchParams.get('tab');
  const unsupportedLegacyTab = legacyTab && !['unprocessed', 'library', 'converted', 'archive'].includes(legacyTab);
  useEffect(() => {
    if (unsupportedLegacyTab) return;
    const next = new URLSearchParams(searchParams);
    if (legacyTab) {
      next.delete('tab');
      if (legacyTab === 'unprocessed') next.set('location', 'raw');
      if (legacyTab === 'library' || legacyTab === 'archive') next.set('location', 'library');
      if (legacyTab === 'converted') { next.set('location', 'library'); next.set('provenance', 'converted'); }
      if (legacyTab === 'archive') next.delete('original');
    }
    if (next.get('original') === 'archived') next.delete('original');
    if (legacyTab || searchParams.get('original') === 'archived') {
      setSearchParams(next, { replace: true });
    }
  }, [legacyTab, unsupportedLegacyTab, searchParams, setSearchParams]);
  const updateLibraryFilter = (key: string, value: string) => {
    const next = new URLSearchParams(searchParams);
    next.set('location', 'library');
    if (value) next.set(key, value); else next.delete(key);
    setSearchParams(next);
  };
  const updateSearchFilter = (value: string) => {
    const next = new URLSearchParams(searchParams);
    if (value) next.set('search', value); else next.delete('search');
    // `asset` remains a supported legacy deep link that selects a specific asset.
    // Once the search field is edited, keep the search term separate from that selection.
    if (value !== (searchParams.get('search') ?? searchParams.get('asset') ?? '')) next.delete('asset');
    setSearchParams(next);
  };
  const switchLocation = (location: 'raw' | 'library', selection?: RawHierarchySelection | LibraryTreeSelection) => {
    const next = new URLSearchParams(searchParams);
    const commonSearch = next.get('search') ?? next.get('asset') ?? '';
    next.set('location', location);
    next.delete('tab');
    next.delete('asset');
    if (commonSearch) next.set('search', commonSearch);
    if (location === 'raw') {
      for (const key of ['sourceGroup', 'logicalGroup', 'path', 'rawScope']) next.delete(key);
      for (const key of ['provenance', 'mediaArea', 'validation', 'original', 'workflow', 'attention', 'evidence', 'libraryId', 'libraryPath']) next.delete(key);
      setLibraryDecision('');
      setValidationFilter('');
      setWorkflowFilter('');
      setAttentionFilter('');
      setMediaArea('');
      const rawSelection = selection && 'kind' in selection ? selection : null;
      if (rawSelection?.kind === 'all') next.set('rawScope', 'all');
      if (rawSelection && rawSelection.kind !== 'all') next.set('sourceGroup', String(rawSelection.sourceGroupId));
      if (rawSelection && (rawSelection.kind === 'logical' || rawSelection.kind === 'path')) next.set('logicalGroup', rawSelection.logicalGroupId);
      if (rawSelection?.kind === 'path') {
        const path = sourceGroups.flatMap((group) => group.logicalGroups.flatMap((logicalGroup) => logicalGroup.assetPaths))
          .find((candidate) => candidate.id === rawSelection.pathId);
        next.set('path', path?.path ?? rawSelection.pathId);
      }
    } else {
      for (const key of ['sourceGroup', 'logicalGroup', 'path', 'rawScope']) next.delete(key);
      const librarySelection = selection && !('kind' in selection) ? selection : null;
      if (librarySelection?.libraryId !== null && librarySelection?.libraryId !== undefined) next.set('libraryId', String(librarySelection.libraryId));
      else next.delete('libraryId');
      if (librarySelection?.path) next.set('libraryPath', librarySelection.path);
      else next.delete('libraryPath');
    }
    setSearchParams(next);
  };
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const autoPublisherEnabled = settings.data?.find((setting) => setting.key === 'pipelineAutomation')?.value.autoPublisherEnabled === true;
  const jobs = useQuery({
    queryKey: ['queueJobs'],
    queryFn: api.queueJobs,
    refetchInterval: (query) => hasNonTerminalQueueLifecycle(query.state.data, autoPublisherEnabled) ? 3000 : false,
  });
  const assets = useQuery({
    queryKey: ['assets'],
    queryFn: api.assets,
    refetchInterval: () => hasNonTerminalQueueLifecycle(jobs.data, autoPublisherEnabled) ? 3000 : false,
  });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const runtimeState = useQuery({ queryKey: ['runtime-state'], queryFn: api.runtimeState });
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
  const convertedCount = assets.data?.counts?.converted ?? safeArray(assets.data?.converted).length;
  const acceptedCount = safeArray(assets.data?.accepted).length;
  const sourceGroups = safeArray(assets.data?.sourceGroups);
  const legacyAssetPath = searchParams.get('asset') ?? '';
  const libraryGroups = safeArray(assets.data?.libraryGroups);
  const canonicalLibraryGroups = libraryGroups.length ? libraryGroups : safeArray(assets.data?.convertedGroups);
  const sourceGroupParam = searchParams.get('sourceGroup');
  const logicalGroupParam = searchParams.get('logicalGroup');
  const rawPathParam = searchParams.get('path');
  const rawScopeParam = searchParams.get('rawScope');
  const libraryIdParam = searchParams.get('libraryId');
  const libraryPathParam = searchParams.get('libraryPath');
  const rawSelection = useMemo(() => rawSelectionFromContext(
    sourceGroups,
    assetQuery,
    legacyAssetPath,
    sourceGroupParam,
    logicalGroupParam,
    rawPathParam,
    rawScopeParam,
  ), [sourceGroups, assetQuery, legacyAssetPath, sourceGroupParam, logicalGroupParam, rawPathParam, rawScopeParam]);
  const libraryTreeSelection = useMemo(() => librarySelectionFromContext(
    libraries.data ?? [],
    canonicalLibraryGroups,
    libraryIdParam,
    libraryPathParam,
  ), [libraries.data, canonicalLibraryGroups, libraryIdParam, libraryPathParam]);
  const statusGroups = safeArray(
    tab === 'library' ? canonicalLibraryGroups : assets.data?.rawGroups ?? assets.data?.unprocessedGroups,
  );
	const currentGroups = statusGroups;
  const allInventoryGroups = [
    ...safeArray(assets.data?.unprocessedGroups), ...safeArray(assets.data?.libraryGroups),
    ...safeArray(assets.data?.convertedGroups),
  ];
  const mediaAreas = [...new Set(allInventoryGroups.map((group) => mediaAreaForGroup(group, settings.data)).filter(Boolean))].sort();
  const areaGroups = mediaArea ? currentGroups.filter((group) => mediaAreaForGroup(group, settings.data) === mediaArea) : currentGroups;
  const filteredGroups = filterAssetGroups(areaGroups, assetQuery, { provenance: libraryDecision, original: originalFilter, validation: validationFilter, workflow: workflowFilter, attention: attentionFilter });
  const runningSnapshotPaths = new Set(
    (snapshotOperations.data?.operations ?? [])
      .filter((operation) => operation.status === 'running')
      .map((operation) => operation.assetPath),
  );
  const summaryCounts = assets.data?.counts;
  const totalAssetsSummary = summaryCounts
    ? summaryCounts.raw + summaryCounts.library
    : unprocessedCount + libraryCount;
  const processingSummary = summaryCounts?.processing
    ?? safeArray(jobs.data).filter((job) => job.status === 'queued' || job.status === 'running').length;
  const needsReviewSummary = summaryCounts?.needsReview
    ?? [...safeArray(assets.data?.unprocessed), ...safeArray(assets.data?.library)].filter((asset) => asset.review?.requiresReview).length;
  const syncDetails = [
    `Last sync: ${formatDate(assets.data?.sync?.lastSyncedAt ?? '')}`,
    `${assets.data?.sync?.missingActionable ?? assets.data?.sync?.missingFiles ?? 0} missing assets`,
    `${assets.data?.sync?.missingHistorical ?? 0} historical paths`,
  ].join(' · ');
  const filterContent = tab === 'library' ? (
    <Grid container spacing={1}>
      <Grid size={{ xs: 12, sm: 6 }}>
        <TextField select fullWidth value={libraryDecision} onChange={(event) => { setLibraryDecision(event.target.value); updateLibraryFilter('provenance', event.target.value); }} label="Provenance" size="small">
          <MenuItem value="">All provenance</MenuItem>
          <MenuItem value="converted">Converted ({convertedCount})</MenuItem>
          <MenuItem value="published_as_is">Published as-is</MenuItem>
          <MenuItem value="imported">Imported / accepted ({acceptedCount})</MenuItem>
          <MenuItem value="legacy_or_unknown">Legacy / unknown</MenuItem>
        </TextField>
      </Grid>
      <Grid size={{ xs: 12, sm: 6 }}>
        <TextField select fullWidth value={mediaArea} onChange={(event) => { setMediaArea(event.target.value); updateLibraryFilter('mediaArea', event.target.value); }} label="Media area" size="small">
          <MenuItem value="">All media</MenuItem>
          {mediaAreas.map((area) => <MenuItem key={area} value={area}>media/{area}</MenuItem>)}
        </TextField>
      </Grid>
      <Grid size={{ xs: 12, sm: 6 }}>
        <TextField select fullWidth value={validationFilter} onChange={(event) => { setValidationFilter(event.target.value); updateLibraryFilter('validation', event.target.value); }} label="Validation" size="small">
          <MenuItem value="">All validation</MenuItem><MenuItem value="passed">Passed</MenuItem><MenuItem value="passed_with_warnings">Warnings</MenuItem><MenuItem value="review_required">Review</MenuItem><MenuItem value="failed">Failed</MenuItem><MenuItem value="unavailable">Unavailable</MenuItem>
        </TextField>
      </Grid>
      <Grid size={{ xs: 12, sm: 6 }}>
        <TextField select fullWidth value={originalFilter} onChange={(event) => updateLibraryFilter('original', event.target.value)} label="Original" size="small">
          <MenuItem value="">All originals</MenuItem><MenuItem value="available">Available</MenuItem><MenuItem value="deleted">Deleted</MenuItem><MenuItem value="unknown">Unknown</MenuItem>
        </TextField>
      </Grid>
      <Grid size={{ xs: 12, sm: 6 }}>
        <TextField select fullWidth value={workflowFilter} onChange={(event) => { setWorkflowFilter(event.target.value); updateLibraryFilter('workflow', event.target.value); }} label="Workflow" size="small">
          <MenuItem value="">All workflow</MenuItem><MenuItem value="available">Ready</MenuItem><MenuItem value="queued">Queued</MenuItem><MenuItem value="processing">Processing</MenuItem><MenuItem value="publishing">Publishing</MenuItem><MenuItem value="needs_review">Needs attention</MenuItem><MenuItem value="failed">Failed</MenuItem>
        </TextField>
      </Grid>
      <Grid size={{ xs: 12, sm: 6 }}>
        <TextField select fullWidth value={attentionFilter} onChange={(event) => { setAttentionFilter(event.target.value); updateLibraryFilter('attention', event.target.value); }} label="Attention" size="small">
          <MenuItem value="">All attention</MenuItem><MenuItem value="needs_review">Needs review</MenuItem><MenuItem value="publication_conflict">Publication conflict</MenuItem>
        </TextField>
      </Grid>
    </Grid>
  ) : (
    <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 480 }}>
      Provenance, media area, validation, original, workflow, and attention filters apply to Library inventory. Search remains available in both RAW and Library.
    </Typography>
  );
  const summaryStrip = (
    <AssetsSummaryStrip
      total={totalAssetsSummary}
      processing={processingSummary}
      completed={summaryCounts?.converted ?? convertedCount}
      needsReview={needsReviewSummary}
    />
  );
  const hierarchyPanel = (
    <AssetsHierarchyPanel
      sourceGroups={sourceGroups}
      groups={canonicalLibraryGroups}
      libraries={libraries.data ?? []}
      location={tab}
      rawSelection={rawSelection}
      librarySelection={libraryTreeSelection}
      onSelectRaw={(selection) => switchLocation('raw', selection)}
      onSelectLibrary={(selection) => switchLocation('library', selection)}
      storageSnapshot={runtimeState.data}
      storageLoading={runtimeState.isLoading}
      storageError={runtimeState.isError}
    />
  );

  return (
    <Box
      data-testid="assets-page-layout"
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: { xs: 'calc(100dvh - 56px)', md: '100dvh' },
        minHeight: 0,
        overflow: 'hidden',
        bgcolor: '#04101d',
      }}
    >
      <Box
        component="header"
        data-testid="assets-topbar"
        sx={{
          minHeight: { xs: 72, md: 64 },
          px: { xs: 1.25, md: 2.5 },
          py: { xs: 0.75, md: 0 },
          display: 'flex',
          alignItems: 'center',
          gap: { xs: 1, md: 1.5 },
          flexWrap: { xs: 'wrap', md: 'nowrap' },
          flexShrink: 0,
          borderBottom: 1,
          borderColor: '#15304a',
          bgcolor: 'rgba(5, 17, 30, 0.92)',
        }}
      >
        <Typography component="h1" variant="h1" sx={{ width: { xs: 'auto', md: 152 }, flex: { xs: '1 0 auto', md: '0 0 152px' }, fontSize: '1.25rem', lineHeight: 1.2, letterSpacing: '-0.025em', color: '#e7eef8' }}>
          Assets
        </Typography>
        <TextField
          value={assetQuery}
          onChange={(event) => { setAssetQuery(event.target.value); updateSearchFilter(event.target.value); }}
          placeholder="Search assets, series, movies…"
          size="small"
          inputProps={{ 'aria-label': 'Search assets, series, movies' }}
          sx={{
            width: { xs: '100%', md: 'min(44vw, 590px)' },
            maxWidth: 590,
            order: { xs: 3, md: 0 },
            '& .MuiOutlinedInput-root': { height: 36, borderRadius: 1, bgcolor: '#091a2c', color: '#e7eef8', fontSize: '0.75rem', '& fieldset': { borderColor: '#15304a' }, '&:hover fieldset': { borderColor: '#1f4467' }, '&.Mui-focused fieldset': { borderColor: '#6c4cf6' } },
            '& input::placeholder': { color: '#59718b', opacity: 1 },
          }}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon fontSize="small" sx={{ color: '#59718b', fontSize: 18 }} />
              </InputAdornment>
            ),
          }}
        />
        <Tooltip title="Assets Settings">
          <IconButton component={RouterLink} to="/settings?section=assets" aria-label="Open Assets Settings" sx={{ ml: 'auto', width: 34, height: 34, color: '#9cb0c8', '&:hover': { bgcolor: '#112a44', color: '#e7eef8' } }}>
            <SettingsOutlinedIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      </Box>
      <Box data-testid="assets-page-body" sx={{ px: { xs: 0.75, md: 1.5 }, pt: { xs: 0.75, md: 1.5 }, pb: { xs: 0.75, md: 1.25 }, minWidth: 0, minHeight: 0, flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
        <Stack spacing={0.5} sx={{ flexShrink: 0, mb: 0.5 }}>
          {savedQueuePreflightID && !queuePreflightCandidate ? <Button size="small" variant="outlined" disabled={queuePreflightResumePending} onClick={() => void resumeQueueBatchPreflight()} sx={{ alignSelf: 'flex-start' }}>
            {queuePreflightResumePending ? 'Loading Preflight…' : 'Resume Preflight'}
          </Button> : null}
          {queuePreflightResumeMessage ? <Alert severity={queuePreflightResumeMessage.startsWith('Could not') ? 'error' : 'info'} action={savedQueuePreflightID && !queuePreflightCandidate ? <Button color="inherit" size="small" disabled={queuePreflightResumePending} onClick={() => void resumeQueueBatchPreflight()}>Retry</Button> : undefined} sx={{ py: 0 }}>
            {queuePreflightResumeMessage}
          </Alert> : null}
          {assets.isError ? <Alert severity="warning" sx={{ py: 0 }}>Unable to read library paths from the backend container.</Alert> : null}
          {unsupportedLegacyTab ? <Alert severity="info" sx={{ py: 0 }}>This Assets link is no longer a supported inventory context. Open RAW or Library and use the persisted lifecycle filters.</Alert> : null}
          {tab === 'library' && libraryDecision === 'converted' ? <Alert severity="info" sx={{ py: 0 }}>Converted is a Library provenance projection. Re-processing and original retention remain independent lifecycle evidence.</Alert> : null}
          {syncAssets.isSuccess ? <Alert severity={syncAssets.data.reviewMatches > 0 ? 'warning' : 'success'} sx={{ py: 0 }}>
            Inventory synced. {syncAssets.data.reconciledFiles} relocated asset(s) reconciled automatically.{syncAssets.data.reviewMatches > 0 ? ` ${syncAssets.data.reviewMatches} possible match(es) were sent to Review.` : ''}
          </Alert> : null}
          {syncAssets.isError ? <Alert severity="warning" sx={{ py: 0 }}>Could not sync assets: {syncAssets.error.message}</Alert> : null}
        </Stack>
        <Box data-testid="assets-workspace-region" sx={{ flex: 1, minHeight: 0, display: 'flex', overflow: 'hidden' }}>
          {tab === 'raw' ? (
            <AssetsErrorBoundary boundaryKey="raw">
              <RawAssetsView
                key={assetQuery}
                summaryStrip={summaryStrip}
                filterContent={filterContent}
                activeFilterCount={activeFilterCount}
                syncDetails={syncDetails}
                syncPending={syncAssets.isPending}
                onSync={() => syncAssets.mutate()}
                sourceGroups={sourceGroups}
                libraries={libraries.data ?? []}
                profiles={profiles.data ?? []}
                audioProfiles={audioProfiles}
                trackProfiles={trackProfiles}
                settings={settings.data ?? []}
                assetCategories={assetCategories}
                queueJobs={jobs.data ?? []}
                runningSnapshotPaths={runningSnapshotPaths}
                query={assetQuery}
                legacyAssetPath={legacyAssetPath}
                selection={rawSelection}
                hierarchyPanel={hierarchyPanel}
              />
            </AssetsErrorBoundary>
          ) : (
            <AssetsErrorBoundary boundaryKey={tab}>
              <AssetTable
                key={tab}
                summaryStrip={summaryStrip}
                filterContent={filterContent}
                activeFilterCount={activeFilterCount}
                syncDetails={syncDetails}
                syncPending={syncAssets.isPending}
                onSync={() => syncAssets.mutate()}
                groups={filteredGroups}
                libraries={libraries.data ?? []}
                profiles={profiles.data ?? []}
                audioProfiles={audioProfiles}
                trackProfiles={trackProfiles}
                settings={settings.data ?? []}
                assetCategories={assetCategories}
                queueJobs={jobs.data ?? []}
                runningSnapshotPaths={runningSnapshotPaths}
                mode={tab === 'library' && libraryDecision === 'converted' ? 'converted' : tab}
                query={assetQuery}
                libraryTreeSelection={libraryTreeSelection}
                hierarchyPanel={hierarchyPanel}
                emptyLabel="No Library asset groups match the persisted filters."
              />
            </AssetsErrorBoundary>
          )}
        </Box>
      </Box>
      <BatchPreflightDialog
        candidate={queuePreflightCandidate}
        onClose={() => {
          queuePreflightRequestGeneration.current += 1;
          setQueuePreflightResumePending(false);
          setQueuePreflightCandidate(null);
        }}
        onAccepted={(accepted) => {
          setQueuePreflightCandidate(accepted);
          if (accepted.status === 'completed' || accepted.assets.every((asset) => asset.status === 'queued')) {
            invalidateQueueBatchPreflightReference(accepted.id);
            setSavedQueuePreflightID(getSavedQueueBatchPreflightCandidateID());
          }
          void queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
          void queryClient.invalidateQueries({ queryKey: ['assets'] });
        }}
        onDiscarded={(terminal) => {
          queuePreflightRequestGeneration.current += 1;
          setQueuePreflightResumePending(false);
          invalidateQueueBatchPreflightReference(terminal.id);
          setSavedQueuePreflightID(getSavedQueueBatchPreflightCandidateID());
          setQueuePreflightCandidate(null);
          setQueuePreflightResumeMessage(terminal.status === 'discarded' ? 'Candidate discarded. Previously accepted Queue work is unchanged.' : 'Candidate completed.');
        }}
      />
    </Box>
  );
}

export function ReportsPage() {
  const inventory = useQuery({ queryKey: ['assets'], queryFn: api.assets });

  return (
    <Box component="main" aria-labelledby="reports-page-title" sx={{ px: { xs: 2, md: 4 }, pt: { xs: 1.5, md: 2 }, pb: 3 }}>
      <Box component="header" sx={{ mb: 2 }}>
        <Typography component="h1" id="reports-page-title" variant="h1" sx={{ fontSize: '1.45rem', lineHeight: 1.2 }}>Reports</Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>Inventory totals, missing assets, and retained path evidence</Typography>
      </Box>
      <Card>
        {inventory.isPending ? (
          <CardContent><Typography role="status" color="text.secondary">Loading reports…</Typography></CardContent>
        ) : inventory.isError ? (
          <Alert severity="warning" sx={{ m: 2 }}>Unable to load inventory reports from the backend.</Alert>
        ) : (
          <AssetReportsPanel inventory={inventory.data} />
        )}
      </Card>
    </Box>
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

type AssetsWorkspaceChromeProps = {
  summaryStrip: ReactNode;
  filterContent: ReactNode;
  activeFilterCount: number;
  syncDetails: string;
  syncPending: boolean;
  onSync: () => void;
};

type AssetTableProps = AssetsWorkspaceChromeProps & {
  groups: AssetGroup[];
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  trackProfiles: TrackProfile[];
  settings: AppSetting[];
  assetCategories: string[];
  queueJobs: QueueJob[];
  runningSnapshotPaths: Set<string>;
  mode: 'unprocessed' | 'library' | 'converted';
  query: string;
  libraryTreeSelection: LibraryTreeSelection;
  hierarchyPanel: ReactNode;
  emptyLabel: string;
};

function AssetTable(props: AssetTableProps) {
  return props.mode !== 'unprocessed'
    ? <LibraryAssetTable {...props} />
    : <LegacyAssetTable {...props} />;
}

function LegacyAssetTable({ groups, libraries, profiles, audioProfiles, trackProfiles, settings, assetCategories, queueJobs, runningSnapshotPaths, mode, query, emptyLabel }: AssetTableProps) {
  const visibleGroups = safeArray(groups);
  const filteredGroups = filterAssetGroups(visibleGroups, query);
  const pageBasis = `${query}\u0000${mode}\u0000${visibleGroups.length}`;
  const [pageState, setPageState] = useState({ basis: pageBasis, page: 0 });
  const page = pageState.basis === pageBasis ? pageState.page : 0;
  const setPage = (nextPage: number) => setPageState({ basis: pageBasis, page: nextPage });
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const pagedGroups = filteredGroups.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);
  const showConfidence = mode !== 'converted';

  if (visibleGroups.length === 0) {
    return <CardContent><Alert severity="info">{emptyLabel}</Alert></CardContent>;
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
            {pagedGroups.length ? pagedGroups.map((group) => (
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
            )) : (
              <TableRow>
                <TableCell colSpan={showConfidence ? 8 : 7}><Alert severity="info">No asset groups match this search.</Alert></TableCell>
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
        onRowsPerPageChange={(event) => { setRowsPerPage(Number(event.target.value)); setPage(0); }}
        sx={{ '& .MuiTablePagination-toolbar': { px: { xs: 1, sm: 2 } }, '& .MuiTablePagination-selectLabel': { display: { xs: 'none', sm: 'block' } } }}
      />
    </>
  );
}

function LibraryAssetTable({
  summaryStrip,
  filterContent,
  activeFilterCount,
  syncDetails,
  syncPending,
  onSync,
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
  libraryTreeSelection: treeSelection,
  hierarchyPanel,
  emptyLabel,
}: AssetTableProps) {
  const filterIdentity = `${mode}\u0000${query}\u0000${treeSelection.libraryId ?? ''}\u0000${treeSelection.path}`;
  const [selectedAssetSelection, setSelectedAssetSelection] = useState({ filterIdentity, path: '' });
  const selectedAssetPath = selectedAssetSelection.filterIdentity === filterIdentity ? selectedAssetSelection.path : '';
  const setSelectedAssetPath = (path: string) => setSelectedAssetSelection({ filterIdentity, path });
  const [bulkSelectionState, setBulkSelectionState] = useState({ filterIdentity, ids: new Set<number>() });
  const [pageSelection, setPageSelection] = useState({ filterIdentity, page: 0 });
  const requestedPage = pageSelection.filterIdentity === filterIdentity ? pageSelection.page : 0;
  const setPage = (page: number) => setPageSelection({ filterIdentity, page });
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [selectedActions, setSelectedActions] = useState<{ path: string; actions: AssetWorkspaceActions } | null>(null);
  const registerActions = useCallback((path: string, actions: AssetWorkspaceActions | null) => {
    setSelectedActions((current) => {
      if (actions) return { path, actions };
      return current?.path === path ? null : current;
    });
  }, []);

  const visibleGroups = useMemo(() => safeArray(groups).filter((group) => {
    if (treeSelection.libraryId !== null && group.libraryId !== treeSelection.libraryId) return false;
    return !treeSelection.path || assetPathWithin(group.path, treeSelection.path);
  }), [groups, treeSelection]);
  const visibleAssets = useMemo(() => {
    const seen = new Set<string>();
    return visibleGroups.flatMap((group) => safeArray(group.assets)
      .filter((asset) => {
        const key = asset.path.replace(/\\/g, '/').replace(/\/+$/g, '');
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      })
      .map((asset) => ({ asset, group })));
  }, [visibleGroups]);
  const selectableAssetIdsByGroup = useMemo(() => new Map(
    visibleGroups.map((group) => [
      normalizePath(group.path),
      new Set(librarySafeBulkSelectableAssets(group).map((asset) => asset.id).filter((id): id is number => id !== undefined)),
    ]),
  ), [visibleGroups]);
  const visibleSelectableAssetIds = useMemo(() => new Set(visibleAssets.flatMap(({ asset, group }) => (
    asset.id !== undefined && selectableAssetIdsByGroup.get(normalizePath(group.path))?.has(asset.id) ? [asset.id] : []
  ))), [selectableAssetIdsByGroup, visibleAssets]);
  const selectedAssetIds = useMemo(() => new Set(
    bulkSelectionState.filterIdentity === filterIdentity
      ? Array.from(bulkSelectionState.ids).filter((id) => visibleSelectableAssetIds.has(id))
      : [],
  ), [bulkSelectionState, filterIdentity, visibleSelectableAssetIds]);
  const updateLibraryAssetSelection = (assetIds: number[], selected: boolean) => {
    const next = new Set(selectedAssetIds);
    for (const id of assetIds) {
      if (selected && visibleSelectableAssetIds.has(id)) next.add(id);
      else next.delete(id);
    }
    setBulkSelectionState({ filterIdentity, ids: next });
  };
  const pageCount = Math.max(1, Math.ceil(visibleAssets.length / rowsPerPage));
  const page = Math.min(requestedPage, pageCount - 1);
  const pageRows = visibleAssets.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);
  const pageAssetIds = pageRows.filter(({ asset }) => !asset.missing).map(({ asset }) => asset.id).filter((id): id is number => typeof id === 'number' && id > 0);
  const effectiveConfigurations = useQuery({
    queryKey: ['effectiveAssetConfiguration', 'library-workspace', ...pageAssetIds],
    queryFn: () => api.effectiveAssetConfigurations(pageAssetIds),
    enabled: pageAssetIds.length > 0,
    staleTime: 15_000,
  });
  const selectablePageAssetIds = pageRows.flatMap(({ asset, group }) => (
    asset.id !== undefined && selectableAssetIdsByGroup.get(normalizePath(group.path))?.has(asset.id) ? [asset.id] : []
  ));
  const selectedSelectablePageAssetCount = selectablePageAssetIds.filter((id) => selectedAssetIds.has(id)).length;
  const allSelectablePageAssetsSelected = selectablePageAssetIds.length > 0 && selectedSelectablePageAssetCount === selectablePageAssetIds.length;
  const selectedEntry = pageRows.find(({ asset }) => asset.path === selectedAssetPath);
  const selectedAsset = selectedEntry?.asset;
  const selectedGroup = selectedEntry?.group;
  const selectedSnapshot = useQuery({
    queryKey: ['assetSnapshot', selectedAsset?.path ?? ''],
    queryFn: () => api.latestSnapshot(selectedAsset!.path),
    enabled: Boolean(selectedAsset && !selectedAsset.missing),
    staleTime: 15_000,
  });
  const selectedPathGroup = treeSelection.path
    ? visibleGroups.find((group) => normalizePath(group.path) === normalizePath(treeSelection.path))
    : undefined;
  const selectedPathAssetIds = selectedPathGroup
    ? librarySafeBulkSelectableAssets(selectedPathGroup).flatMap((asset) => asset.id === undefined ? [] : [asset.id])
    : [];
  const selectedPathBulkCount = selectedPathAssetIds.filter((id) => selectedAssetIds.has(id)).length;
  const selectedLibrary = libraries.find((library) => library.id === treeSelection.libraryId);
  const selectedLibraryName = selectedLibrary?.name ?? groups.find((group) => group.libraryId === treeSelection.libraryId)?.libraryName;
  const selectedPathParts = treeSelection.path.replace(/\\/g, '/').split('/').filter(Boolean);
  const libraryRootPartCount = selectedLibrary?.destinationPath.replace(/\\/g, '/').split('/').filter(Boolean).length ?? 0;
  const libraryBreadcrumbs = treeSelection.libraryId === null && !treeSelection.path
    ? ['Assets', 'Libraries', 'All Library assets']
    : ['Assets', 'Libraries', selectedLibraryName ?? 'Library', ...selectedPathParts.slice(libraryRootPartCount)];

  const selectedAssetRelativeParts = selectedAsset ? selectedAsset.relativePath.replace(/\\/g, '/').split('/').filter(Boolean) : [];
  const selectedAssetParentPath = selectedAssetRelativeParts.at(-1) === selectedAsset?.fileName
    ? selectedAssetRelativeParts.slice(0, -1).join('/')
    : selectedAssetRelativeParts.join('/');
  const selectedAssetPathContext = selectedAsset && selectedGroup
    ? selectedAssetParentPath
    : '';
  const selectedSnapshotData = selectedSnapshot.data?.snapshot ?? null;
  const selectedEffective = selectedAsset?.id ? effectiveConfigurations.data?.configurations[String(selectedAsset.id)] : undefined;
  const assignedProfileLabel = effectiveConfigurations.isLoading ? 'Loading…' : effectiveVideoProfileLabel(selectedEffective?.video, profiles);
  const selectedWorkspaceActions = selectedAsset && selectedActions?.path === selectedAsset.path ? selectedActions.actions : null;
  const selectedPipelineState = selectedAsset
    ? assetPipelineState(selectedAsset, associatedJobForCurrentPublication(selectedAsset, queueJobs), false)
    : undefined;

  return (
    <Box data-testid="assets-mode-workspace" sx={{ bgcolor: '#061525', p: { xs: 0.5, sm: 0.75 }, width: '100%', height: '100%', minWidth: 0, minHeight: 0, display: 'flex', overflow: 'hidden' }}>
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={{ xs: 0.75, md: 1 }} alignItems="stretch" sx={{ minWidth: 0, minHeight: 0, height: '100%', width: '100%', overflow: 'hidden' }}>
        {hierarchyPanel}
        <Box data-testid="assets-content-column" sx={{ flex: 1, minWidth: 0, minHeight: 0, height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <Box data-testid="assets-summary-in-workspace" sx={{ mb: 1.1, flexShrink: 0 }}>{summaryStrip}</Box>

          {selectedPathGroup ? (
            <AssetGroupRow
              group={selectedPathGroup}
              libraries={libraries}
              profiles={profiles}
              audioProfiles={audioProfiles}
              trackProfiles={trackProfiles}
              settings={settings}
              assetCategories={assetCategories}
              queueJobs={queueJobs}
              runningSnapshotPaths={runningSnapshotPaths}
              mode={mode}
              v2ToolbarOnly
              breadcrumbSegments={libraryBreadcrumbs}
              selectedCount={selectedPathBulkCount}
              selectedAssetIds={selectedAssetIds}
              onAssetSelectionChange={updateLibraryAssetSelection}
            />
          ) : (
            <AssetsV2PathToolbar
              path={treeSelection.path || selectedLibraryName || 'Library'}
              mode="library"
              breadcrumbs={<WorkspaceBreadcrumbs segments={libraryBreadcrumbs} label="Library location" />}
            />
          )}

          <AssetsV2InventoryControls count={visibleAssets.length} filterCount={activeFilterCount} syncDetails={syncDetails} syncPending={syncPending} onSync={onSync}>
            {filterContent}
          </AssetsV2InventoryControls>

          <Box data-testid="assets-list-scroll" role="region" aria-label="Library asset list" sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden', overscrollBehavior: 'contain', pb: 1.25 }}>
          {visibleAssets.length ? (
            <AssetsInventoryTable
              mode="library"
              selectable
              selectAll={{
                checked: allSelectablePageAssetsSelected,
                indeterminate: selectedSelectablePageAssetCount > 0 && !allSelectablePageAssetsSelected,
                disabled: selectablePageAssetIds.length === 0,
                onChange: (selected) => updateLibraryAssetSelection(selectablePageAssetIds, selected),
              }}
            >
                  {pageRows.map(({ asset, group }) => {
                    const effective = asset.id ? effectiveConfigurations.data?.configurations[String(asset.id)] : undefined;
                    return (
                      <AssetRow
                        key={asset.path}
                        asset={asset}
                        libraries={libraries}
                        profiles={profiles}
                        audioProfiles={audioProfiles}
                        trackProfiles={trackProfiles}
                        srtFallbackEnabled={subtitleSRTFallbackEnabled(settings)}
                        pathTrackProfile={trackProfiles.find((profile) => profile.key === effective?.tracks.profileKey)}
                        assetCategories={assetCategories}
                        groupCategory={firstCategory(group.pathMetadata?.categories) || group.libraryName || 'Unavailable'}
                        confidenceEnabled={!getDisabledConfidencePaths(settings).includes(group.path)}
                        groupProfileId={effective?.video.videoProfileId ?? 0}
                        effectiveConfiguration={effective}
                        groupAudioProfileKey={effective?.audio.profileKey ?? ''}
                        groupLibraryId={group.libraryId}
                        hasOpenJob={assetHasOpenJob(asset, queueJobs)}
                        queueJobs={queueJobs}
                        snapshotRunning={runningSnapshotPaths.has(asset.path)}
                        mode={mode}
                        bulkSelected={Boolean(asset.id && selectedAssetIds.has(asset.id))}
                        bulkSelectionEnabled
                        bulkSelectionDisabled={asset.id === undefined || !selectableAssetIdsByGroup.get(normalizePath(group.path))?.has(asset.id)}
                        onBulkSelectionChange={(_, selected) => { if (asset.id !== undefined) updateLibraryAssetSelection([asset.id], selected); }}
                        compactLibrary
                        selected={asset.path === selectedAssetPath}
                        onSelectAsset={() => setSelectedAssetPath(asset.path)}
                        onRegisterWorkspaceActions={registerActions}
                      />
                    );
                  })}
            </AssetsInventoryTable>
          ) : (
            <Alert severity="info" sx={{ mt: 1 }}>{emptyLabel}</Alert>
          )}
          </Box>

          {visibleAssets.length ? (
            <TablePagination
              component="div"
              count={visibleAssets.length}
              page={Math.min(page, pageCount - 1)}
              rowsPerPage={rowsPerPage}
              rowsPerPageOptions={[10, 25, 50, 100]}
              onPageChange={(_, nextPage) => { setPage(nextPage); setSelectedAssetPath(''); }}
              onRowsPerPageChange={(event) => { setRowsPerPage(Number(event.target.value)); setPage(0); setSelectedAssetPath(''); }}
              sx={{ flexShrink: 0, '& .MuiTablePagination-toolbar': { px: { xs: 0.5, sm: 1 }, minHeight: 42 }, '& .MuiTablePagination-selectLabel': { display: { xs: 'none', sm: 'block' } } }}
            />
          ) : null}

          {selectedAsset && selectedGroup ? (
            <SelectedAssetDetail
              asset={selectedAsset}
              pathContext={selectedAssetPathContext}
              categoryLabel={libraryEffectiveCategory(
                selectedEffective,
                firstCategory(selectedAsset.metadata.categories) || selectedAsset.libraryName || selectedGroup.libraryName || 'Unavailable',
                'Disabled',
              ) || 'Unavailable'}
              containerLabel={selectedSnapshotData?.container || 'Unavailable'}
              snapshot={selectedSnapshotData}
              snapshotLoading={selectedSnapshot.isLoading}
              statusLabel={selectedPipelineState?.label ?? statusLabel(selectedAsset.status)}
              statusColor={selectedPipelineState?.color ?? statusColor(selectedAsset.status)}
              assignedProfileLabel={assignedProfileLabel}
              modifiedLabel={formatDate(selectedAsset.modifiedAt)}
              sizeLabel={formatBytes(selectedAsset.sizeBytes)}
              onPlayPreview={() => selectedWorkspaceActions?.playPreview()}
              onAssignProfile={() => selectedWorkspaceActions?.assignProfile()}
              onTestEncode={() => selectedWorkspaceActions?.testEncode()}
              onMore={(anchor) => selectedWorkspaceActions?.openMore(anchor)}
              canAssignProfile={selectedWorkspaceActions?.canAssignProfile ?? false}
              canTestEncode={selectedWorkspaceActions?.canTestEncode ?? false}
              assignProfileUnavailableReason={selectedWorkspaceActions?.assignProfileUnavailableReason}
              testEncodeUnavailableReason={selectedWorkspaceActions?.testEncodeUnavailableReason}
            />
          ) : null}
        </Box>
      </Stack>
    </Box>
  );
}

type AssetWorkspaceActions = {
  playPreview: () => void;
  assignProfile: () => void;
  testEncode: () => void;
  openMore: (anchor: HTMLElement) => void;
  canAssignProfile: boolean;
  canTestEncode: boolean;
  assignProfileUnavailableReason: string;
  testEncodeUnavailableReason: string;
};

function assetGroupLifecycleFlags(group: AssetGroup, groupAssets = safeArray(group.assets)) {
  const memberProvenances = new Set(groupAssets.map((asset) => asset.lifecycle?.provenance ?? asset.status));
  const isMixedGroup = memberProvenances.size > 1
    || group.status === 'mixed'
    || (group.lifecycleSummary?.lifecycle.length ?? 0) > 1
    || (group.lifecycleSummary?.validation.length ?? 0) > 1
    || (group.lifecycleSummary?.publication.length ?? 0) > 1;
  const isConvertedGroup = !isMixedGroup && groupAssets.length > 0 && groupAssets.every((asset) => asset.lifecycle?.provenance === 'converted' || asset.status === 'converted');
  const isPublishedAsIsGroup = !isMixedGroup && groupAssets.length > 0 && groupAssets.every((asset) => asset.lifecycle?.provenance === 'published_as_is' || asset.publicationMode === 'as_is');
  const isAcceptedGroup = group.status === 'accepted' || (!isMixedGroup && groupAssets.length > 0 && groupAssets.every((asset) => asset.lifecycle?.provenance === 'imported' || asset.status === 'accepted'));
  return { isMixedGroup, isConvertedGroup, isPublishedAsIsGroup, isAcceptedGroup };
}

function librarySafeBulkSelectableAssets(group: AssetGroup) {
  const lifecycle = assetGroupLifecycleFlags(group);
  if (lifecycle.isMixedGroup || lifecycle.isAcceptedGroup) return [];
  const action = lifecycle.isPublishedAsIsGroup
    ? 'restore'
    : lifecycle.isConvertedGroup
      ? 'deleteArchivedOriginal'
      : undefined;
  if (!action) return [];
  return safeArray(group.assets).filter((asset) => (
    !asset.missing && asset.id !== undefined && assetInfoCapability(asset, action).allowed
  ));
}

type AssetCollection = {
  id: string;
  title: string;
  libraryName: string;
  groups: AssetGroup[];
  fileCount: number;
  sizeBytes: number;
};

function RawAssetsView({ summaryStrip, filterContent, activeFilterCount, syncDetails, syncPending, onSync, sourceGroups, libraries, profiles, audioProfiles, trackProfiles, settings, assetCategories, queueJobs, runningSnapshotPaths, query, legacyAssetPath, selection, hierarchyPanel }: AssetsWorkspaceChromeProps & {
  sourceGroups: AssetSourceGroup[];
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  trackProfiles: TrackProfile[];
  settings: AppSetting[];
  assetCategories: string[];
  queueJobs: QueueJob[];
  runningSnapshotPaths: Set<string>;
  query: string;
  legacyAssetPath: string;
  selection: RawHierarchySelection;
  hierarchyPanel: ReactNode;
}) {
  const selectionIdentity = `${query}\u0000${selection.kind}\u0000${selection.kind === 'all' ? '' : selection.sourceGroupId}\u0000${selection.kind === 'logical' || selection.kind === 'path' ? selection.logicalGroupId : ''}\u0000${selection.kind === 'path' ? selection.pathId : ''}`;
  const [selectedAssetIdsState, setSelectedAssetIdsState] = useState({ identity: selectionIdentity, ids: new Set<number>() });
  const selectedAssetIds = selectedAssetIdsState.identity === selectionIdentity ? selectedAssetIdsState.ids : new Set<number>();
  const [selectedAssetPathState, setSelectedAssetPathState] = useState({ identity: '', path: '' });
  const [pageState, setPageState] = useState({ identity: '', page: 0 });
  const [rowsPerPage, setRowsPerPage] = useState(25);
  const [selectedActions, setSelectedActions] = useState<{ path: string; actions: AssetWorkspaceActions } | null>(null);
  const registerActions = useCallback((path: string, actions: AssetWorkspaceActions | null) => {
    setSelectedActions((current) => {
      if (actions) return { path, actions };
      return current?.path === path ? null : current;
    });
  }, []);
  const cleanQuery = query.trim().toLowerCase();
  const filteredSourceGroups = useMemo(() => sourceGroups.map((sourceGroup) => ({
    ...sourceGroup,
    logicalGroups: safeArray(sourceGroup.logicalGroups).filter((logicalGroup) => sourceLogicalGroupMatches(logicalGroup, cleanQuery)),
  })).filter((sourceGroup) => sourceGroup.logicalGroups.length > 0), [sourceGroups, cleanQuery]);
  const selectionKind = selection.kind;
  const selectionSourceGroupId = selection.kind === 'all' ? null : selection.sourceGroupId;
  const selectionLogicalGroupId = selection.kind === 'logical' || selection.kind === 'path' ? selection.logicalGroupId : '';
  const selectionPathId = selection.kind === 'path' ? selection.pathId : '';
  const rawContext = useMemo(() => {
    const sourceGroup = selectionSourceGroupId === null ? undefined : sourceGroups.find((group) => group.id === selectionSourceGroupId);
    const visibleSourceGroup = selectionSourceGroupId === null ? undefined : filteredSourceGroups.find((group) => group.id === selectionSourceGroupId);
    const logicalGroup = selectionKind === 'logical' || selectionKind === 'path'
      ? visibleSourceGroup?.logicalGroups.find((group) => group.id === selectionLogicalGroupId)
      : undefined;
    const selectedPath = selectionKind === 'path'
      ? logicalGroup?.assetPaths.find((path) => path.id === selectionPathId)
      : undefined;
    const contextLogicalGroups = selectionKind === 'all'
      ? filteredSourceGroups.flatMap((group) => group.logicalGroups)
      : selectionKind === 'source'
        ? visibleSourceGroup?.logicalGroups ?? []
        : logicalGroup ? [logicalGroup] : [];
    const pathEntries = contextLogicalGroups.flatMap((group) => safeArray(group.assetPaths)
      .filter((path) => !selectedPath || path.id === selectedPath.id)
      .map((path) => ({ path, logicalGroup: group, sourceGroup: filteredSourceGroups.find((candidate) => candidate.logicalGroups.some((logical) => logical.id === group.id)) ?? sourceGroup }))
      .filter((entry): entry is { path: import('../api/types').AssetPath; logicalGroup: AssetLogicalGroup; sourceGroup: AssetSourceGroup } => Boolean(entry.sourceGroup)));
    const seen = new Set<string>();
    const visibleAssets = pathEntries.flatMap(({ path, logicalGroup, sourceGroup }) => safeArray(path.assets).flatMap((asset) => {
      const identity = normalizePath(asset.path);
      if (seen.has(identity)) return [];
      seen.add(identity);
      return [{ asset, group: assetGroupFromTreePath(path), path, logicalGroup, sourceGroup }];
    }));
    return { sourceGroup, logicalGroup, selectedPath, contextLogicalGroups, visibleAssets };
  }, [filteredSourceGroups, selectionKind, selectionLogicalGroupId, selectionPathId, selectionSourceGroupId, sourceGroups]);
  const { sourceGroup, logicalGroup, selectedPath, contextLogicalGroups, visibleAssets } = rawContext;
  const contextIdentity = `${query}\u0000${selection.kind}\u0000${selection.kind === 'all' ? '' : selection.sourceGroupId}\u0000${selection.kind === 'logical' || selection.kind === 'path' ? selection.logicalGroupId : ''}\u0000${selection.kind === 'path' ? selection.pathId : ''}`;
  const page = pageState.identity === contextIdentity ? pageState.page : 0;
  const requestedSelectedAssetPath = selectedAssetPathState.identity === contextIdentity ? selectedAssetPathState.path : '';
  const pageCount = Math.max(1, Math.ceil(visibleAssets.length / rowsPerPage));
  const visiblePage = Math.min(page, pageCount - 1);
  const pageRows = visibleAssets.slice(visiblePage * rowsPerPage, visiblePage * rowsPerPage + rowsPerPage);
  const selectablePageAssetIds = pageRows.flatMap(({ asset }) => !asset.missing && asset.id !== undefined ? [asset.id] : []);
  const selectedSelectablePageAssetCount = selectablePageAssetIds.filter((id) => selectedAssetIds.has(id)).length;
  const allSelectablePageAssetsSelected = selectablePageAssetIds.length > 0 && selectedSelectablePageAssetCount === selectablePageAssetIds.length;
  const selectedAssetPath = requestedSelectedAssetPath || pageRows[0]?.asset.path || '';
  const selectedEntry = pageRows.find(({ asset }) => normalizePath(asset.path) === normalizePath(selectedAssetPath));
  const selectedAsset = selectedEntry?.asset;
  const selectedGroup = selectedEntry?.group;
  const selectedSnapshot = useQuery({
    queryKey: ['assetSnapshot', selectedAsset?.path ?? ''],
    queryFn: () => api.latestSnapshot(selectedAsset!.path),
    enabled: Boolean(selectedAsset && !selectedAsset.missing),
    staleTime: 15_000,
  });
  const pageAssetIds = pageRows.filter(({ asset }) => !asset.missing).map(({ asset }) => asset.id).filter((id): id is number => typeof id === 'number' && id > 0);
  const effectiveConfigurations = useQuery({
    queryKey: ['effectiveAssetConfiguration', 'raw-workspace', ...pageAssetIds],
    queryFn: () => api.effectiveAssetConfigurations(pageAssetIds),
    enabled: pageAssetIds.length > 0,
    staleTime: 15_000,
  });
  const selectedSnapshotData = selectedSnapshot.data?.snapshot ?? null;
  const selectedEffective = selectedAsset?.id ? effectiveConfigurations.data?.configurations[String(selectedAsset.id)] : undefined;
  const assignedProfileLabel = effectiveConfigurations.isLoading ? 'Loading…' : effectiveVideoProfileLabel(selectedEffective?.video, profiles);
  const selectedWorkspaceActions = selectedAsset && selectedActions?.path === selectedAsset.path ? selectedActions.actions : null;
  const selectedPipelineState = selectedAsset
    ? assetPipelineState(selectedAsset, associatedJobForCurrentPublication(selectedAsset, queueJobs), false)
    : undefined;
  const selectedPathGroup = selectedPath ? assetGroupFromTreePath(selectedPath) : undefined;
  const rawBreadcrumbs = selection.kind === 'all'
    ? ['Assets', 'RAW', 'All RAW assets']
    : [
      'Assets',
      'RAW',
      ...(sourceGroup ? [sourceGroup.name] : []),
      ...(logicalGroup ? [logicalGroup.name] : []),
      ...(selectedPath ? [selectedPath.isLogicalGroupRoot ? 'Root' : selectedPath.displayPath || selectedPath.name] : []),
    ];
  const setSelectedAssetPath = (path: string) => setSelectedAssetPathState({ identity: contextIdentity, path });
  const setPage = (nextPage: number) => setPageState({ identity: contextIdentity, page: nextPage });
  const scopedSelectedAssets = selectedSelectableAssetsForLogicalGroups(contextLogicalGroups, selectedAssetIds);
  const updateSelectedAssets = (assetIds: number[], selected: boolean) => {
    const current = selectedAssetIdsState.identity === selectionIdentity ? selectedAssetIdsState.ids : new Set<number>();
    const next = new Set(current);
    for (const assetId of assetIds) {
      if (selected) next.add(assetId);
      else next.delete(assetId);
    }
    setSelectedAssetIdsState({ identity: selectionIdentity, ids: next });
  };

  return (
    <Box data-testid="assets-mode-workspace" sx={{ bgcolor: '#061525', p: { xs: 0.5, sm: 0.75 }, width: '100%', height: '100%', minWidth: 0, minHeight: 0, display: 'flex', overflow: 'hidden' }}>
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={{ xs: 0.75, md: 1 }} alignItems="stretch" sx={{ minWidth: 0, minHeight: 0, height: '100%', width: '100%', overflow: 'hidden' }}>
        {hierarchyPanel}
        <Box data-testid="assets-content-column" sx={{ flex: 1, minWidth: 0, minHeight: 0, height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
          <Box data-testid="assets-summary-in-workspace" sx={{ mb: 1.1, flexShrink: 0 }}>{summaryStrip}</Box>
          {selectedPathGroup ? (
            <AssetGroupRow
              group={selectedPathGroup}
              libraries={libraries}
              profiles={profiles}
              audioProfiles={audioProfiles}
              trackProfiles={trackProfiles}
              settings={settings}
              assetCategories={assetCategories}
              queueJobs={queueJobs}
              runningSnapshotPaths={runningSnapshotPaths}
              mode="unprocessed"
              v2ToolbarOnly
              breadcrumbSegments={rawBreadcrumbs}
              selectedCount={scopedSelectedAssets.length}
              selectedAssetIds={selectedAssetIds}
              onAssetSelectionChange={updateSelectedAssets}
            />
          ) : selection.kind === 'logical' && logicalGroup ? (
            <UnprocessedLogicalGroup
              key={logicalGroup.id}
              logicalGroup={logicalGroup}
              legacyAssetPath=""
              selectedAssetIds={selectedAssetIds}
              onAssetSelectionChange={updateSelectedAssets}
              libraries={libraries}
              profiles={profiles}
              audioProfiles={audioProfiles}
              trackProfiles={trackProfiles}
              settings={settings}
              assetCategories={assetCategories}
              queueJobs={queueJobs}
              runningSnapshotPaths={runningSnapshotPaths}
              breadcrumbSegments={rawBreadcrumbs}
              headerOnly
            />
          ) : (
            <AssetsV2PathToolbar
              path={sourceGroup?.sourcePath ?? ''}
              mode="raw"
              breadcrumbs={<WorkspaceBreadcrumbs segments={rawBreadcrumbs} label="RAW source location" />}
              primaryActions={sourceGroup ? <SourceGroupConfigureButton sourceGroup={sourceGroup} profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} libraries={libraries} categories={assetCategories} /> : null}
            />
          )}
          {scopedSelectedAssets.length ? (
            <UnprocessedSelectionToolbar
              selectedAssetIds={selectedAssetIds}
              logicalGroups={contextLogicalGroups}
              profiles={profiles}
              audioProfiles={audioProfiles}
              trackProfiles={trackProfiles}
              libraries={libraries}
              categories={assetCategories}
              queueJobs={queueJobs}
              onClear={() => setSelectedAssetIdsState({ identity: selectionIdentity, ids: new Set() })}
            />
          ) : null}
          <AssetsV2InventoryControls count={visibleAssets.length} filterCount={activeFilterCount} syncDetails={syncDetails} syncPending={syncPending} onSync={onSync}>
            {filterContent}
          </AssetsV2InventoryControls>
          <Box data-testid="assets-list-scroll" role="region" aria-label="RAW asset list" sx={{ flex: 1, minHeight: 0, overflowY: 'auto', overflowX: 'hidden', overscrollBehavior: 'contain', pb: 1.25 }}>
          {visibleAssets.length ? (
            <AssetsInventoryTable
              mode="raw"
              selectable
              selectAll={{
                checked: allSelectablePageAssetsSelected,
                indeterminate: selectedSelectablePageAssetCount > 0 && !allSelectablePageAssetsSelected,
                disabled: selectablePageAssetIds.length === 0,
                onChange: (selected) => updateSelectedAssets(selectablePageAssetIds, selected),
              }}
            >
                  {pageRows.map(({ asset, path }) => {
                    const effective = asset.id ? effectiveConfigurations.data?.configurations[String(asset.id)] : undefined;
                    const track = trackProfiles.find((profile) => profile.key === effective?.tracks.profileKey);
                    return (
                      <AssetRow
                        key={asset.id ?? asset.path}
                        asset={asset}
                        autoOpenAssetInfo={Boolean(legacyAssetPath && normalizePath(asset.path) === normalizePath(legacyAssetPath))}
                        compactLibrary
                        compactRaw
                        selected={normalizePath(asset.path) === normalizePath(selectedAssetPath)}
                        onSelectAsset={() => setSelectedAssetPath(asset.path)}
                        onRegisterWorkspaceActions={registerActions}
                        libraries={libraries}
                        profiles={profiles}
                        audioProfiles={audioProfiles}
                        trackProfiles={trackProfiles}
                        pathTrackProfile={track}
                        assetCategories={assetCategories}
                        groupCategory={effective?.category.category ?? ''}
                        confidenceEnabled={!getDisabledConfidencePaths(settings).includes(path.path)}
                        groupProfileId={effective?.video.videoProfileId ?? 0}
                        effectiveConfiguration={effective}
                        groupAudioProfileKey={effective?.audio.profileKey ?? ''}
                        groupLibraryId={effective?.destination.destinationLibraryId ?? 0}
                        hasOpenJob={assetHasOpenJob(asset, queueJobs)}
                        queueJobs={queueJobs}
                        snapshotRunning={runningSnapshotPaths.has(asset.path)}
                        mode="unprocessed"
                        srtFallbackEnabled={subtitleSRTFallbackEnabled(settings)}
                        bulkSelected={Boolean(asset.id && selectedAssetIds.has(asset.id))}
                        bulkSelectionEnabled
                        bulkSelectionDisabled={asset.missing || !asset.id}
                        onBulkSelectionChange={(_, selected) => { if (asset.id) updateSelectedAssets([asset.id], selected); }}
                      />
                    );
                  })}
            </AssetsInventoryTable>
          ) : (
            <Alert severity="info" sx={{ mt: 1 }}>{sourceGroups.length ? 'No RAW assets match this context or search.' : 'No RAW Source Groups found.'}</Alert>
          )}
          </Box>
          {visibleAssets.length ? (
            <TablePagination
              component="div"
              count={visibleAssets.length}
              page={visiblePage}
              rowsPerPage={rowsPerPage}
              rowsPerPageOptions={[10, 25, 50, 100]}
              onPageChange={(_, nextPage) => { setPage(nextPage); setSelectedAssetPath(''); }}
              onRowsPerPageChange={(event) => { setRowsPerPage(Number(event.target.value)); setPage(0); setSelectedAssetPath(''); }}
              sx={{ flexShrink: 0, '& .MuiTablePagination-toolbar': { px: { xs: 0.5, sm: 1 }, minHeight: 42 }, '& .MuiTablePagination-selectLabel': { display: { xs: 'none', sm: 'block' } } }}
            />
          ) : null}
          {selectedAsset && selectedGroup && selectedEntry ? (
            <SelectedAssetDetail
              asset={selectedAsset}
              pathContext={`${selectedEntry.sourceGroup.name} · ${selectedEntry.logicalGroup.name} · ${selectedEntry.path.isLogicalGroupRoot ? 'Root' : selectedEntry.path.displayPath || selectedEntry.path.name}`}
              categoryLabel={selectedAsset.metadata.categories?.[0] || selectedAsset.libraryName || selectedEntry.sourceGroup.name || 'Unavailable'}
              containerLabel={selectedSnapshotData?.container || 'Unavailable'}
              snapshot={selectedSnapshotData}
              snapshotLoading={selectedSnapshot.isLoading}
              statusLabel={selectedPipelineState?.label ?? statusLabel(selectedAsset.status)}
              statusColor={selectedPipelineState?.color ?? statusColor(selectedAsset.status)}
              assignedProfileLabel={assignedProfileLabel}
              modifiedLabel={formatDate(selectedAsset.modifiedAt)}
              sizeLabel={formatBytes(selectedAsset.sizeBytes)}
              onPlayPreview={() => selectedWorkspaceActions?.playPreview()}
              onAssignProfile={() => selectedWorkspaceActions?.assignProfile()}
              onTestEncode={() => selectedWorkspaceActions?.testEncode()}
              onMore={(anchor) => selectedWorkspaceActions?.openMore(anchor)}
              canAssignProfile={selectedWorkspaceActions?.canAssignProfile ?? false}
              canTestEncode={selectedWorkspaceActions?.canTestEncode ?? false}
              assignProfileUnavailableReason={selectedWorkspaceActions?.assignProfileUnavailableReason}
              testEncodeUnavailableReason={selectedWorkspaceActions?.testEncodeUnavailableReason}
            />
          ) : null}
        </Box>
      </Stack>
    </Box>
  );
}

type PreparedSelectionQueue = QueueSelectedAssetsResponse;

function UnprocessedLogicalGroup({ logicalGroup, legacyAssetPath, selectedAssetIds, onAssetSelectionChange, libraries, profiles, audioProfiles, trackProfiles, settings, assetCategories, queueJobs, runningSnapshotPaths, breadcrumbSegments, headerOnly = false }: {
	logicalGroup: AssetLogicalGroup; legacyAssetPath: string; selectedAssetIds: Set<number>; onAssetSelectionChange: (ids: number[], selected: boolean) => void; libraries: Library[]; profiles: Profile[]; audioProfiles: AudioEnhancementProfile[]; trackProfiles: TrackProfile[]; settings: AppSetting[]; assetCategories: string[]; queueJobs: QueueJob[]; runningSnapshotPaths: Set<string>; breadcrumbSegments?: string[]; headerOnly?: boolean;
}) {
	const queryClient = useQueryClient();
	const targetPath = safeArray(logicalGroup.assetPaths).find((path) => safeArray(path.assets).some((asset) => normalizePath(asset.path) === normalizePath(legacyAssetPath)));
	const [expanded, setExpanded] = useState(!headerOnly && Boolean(targetPath));
	const [expandedPaths, setExpandedPaths] = useState<Set<string>>(() => new Set(targetPath && !targetPath.isLogicalGroupRoot ? [targetPath.id] : []));
	const [titleSnapshotsOpen, setTitleSnapshotsOpen] = useState(false);
	const [titleActionsAnchor, setTitleActionsAnchor] = useState<HTMLElement | null>(null);
	const paths = safeArray(logicalGroup.assetPaths);
	const assets = selectableAssetsForPaths(paths);
	const titleHasOpenJob = paths.some((path) => safeArray(path.assets).some((asset) => assetHasOpenJob(asset, queueJobs)));
	const titleSnapshotGroup = paths.length ? {
		...assetGroupFromTreePath(paths[0]),
		id: logicalGroup.id,
		path: logicalGroup.path,
		relativePath: logicalGroup.relativePath,
		fileCount: logicalGroup.fileCount,
		sizeBytes: logicalGroup.totalSizeBytes,
		assets: paths.flatMap((path) => safeArray(path.assets)),
	} : null;
	const snapshotPathLabels = Object.fromEntries(paths.flatMap((path) => safeArray(path.assets).map((asset) => [normalizePath(asset.path), path.isLogicalGroupRoot ? 'Root' : path.displayPath || path.name])));
	const assetIds = assets.map((asset) => asset.id as number);
	const groupSelection = hierarchicalSelectionState(assets.map((asset) => asset.id as number), selectedAssetIds);
	const effectiveConfigurations = useQuery({
		queryKey: ['effectiveAssetConfiguration', 'batch', ...assetIds],
		queryFn: () => api.effectiveAssetConfigurations(assetIds),
		enabled: expanded && !headerOnly && assetIds.length > 0,
		staleTime: 15_000,
	});
	const [titleAdvisorResults, setTitleAdvisorResults] = useState<Array<{ asset: Asset; response?: AdvisorResponse; error?: string }>>([]);
	const runTitleAdvisor = useMutation({
		mutationFn: async () => {
			const effective = await api.effectiveAssetConfigurations(assetIds);
			const results: Array<{ asset: Asset; response?: AdvisorResponse; error?: string }> = [];
			for (const asset of assets) {
				const profileId = effective.configurations[String(asset.id)]?.video.videoProfileId ?? 0;
				if (!profileId) {
					results.push({ asset, error: 'No effective Video Profile is available for Advisor.' });
					continue;
				}
				try {
					const response = await api.evaluateAdvisor({ mediaPath: asset.path, profileId });
					queryClient.setQueryData(['advisor', 'asset-row', asset.path, profileId], response);
					results.push({ asset, response });
				} catch (error) {
					results.push({ asset, error: error instanceof Error ? error.message : 'Advisor evaluation failed.' });
				}
			}
			return results;
		},
		onSuccess: setTitleAdvisorResults,
	});
	const publishTitleAsIs = useMutation({
		mutationFn: async () => {
			const effective = await api.effectiveAssetConfigurations(assetIds);
			const resolvedDestinationIDs = assets.map((asset) => effective.configurations[String(asset.id)]?.destination.destinationLibraryId ?? 0);
			const destinationIDs = [...new Set(resolvedDestinationIDs)];
			if (resolvedDestinationIDs.some((id) => id <= 0) || destinationIDs.length !== 1) throw new Error('Configure the same effective Destination for every asset in this title before publishing as-is.');
			const destination = libraries.find((library) => library.id === destinationIDs[0]);
			if (!destination) throw new Error('The effective Destination Library is unavailable.');
			if (!window.confirm(`Publish ${assets.length} original asset(s) in ${logicalGroup.name} as-is to ${destination.name}? FFmpeg will not run and the title directory will move from Raw to Library.`)) throw new Error('Publish as-is canceled.');
			return api.publishAssetsAsIs({ sourcePath: logicalGroup.path, destinationLibraryId: destination.id });
		},
		onSuccess: async () => {
			await Promise.all([queryClient.invalidateQueries({ queryKey: ['assets'] }), queryClient.invalidateQueries({ queryKey: ['queueJobs'] })]);
		},
	});
	const rootPaths = paths.filter((path) => path.isLogicalGroupRoot);
	const childPaths = paths.filter((path) => !path.isLogicalGroupRoot);
	const renderAssets = (path: import('../api/types').AssetPath) => {
		const visibleAssets = safeArray(path.assets).filter((asset) => asset.id);
		if (!visibleAssets.length) return <Alert severity="info">No available assets in this path.</Alert>;
		return <Box sx={{ overflowX: 'auto' }}><Table size="small" sx={{ minWidth: 720, tableLayout: 'fixed' }}><TableHead><TableRow><TableCell padding="checkbox" sx={{ width: 48 }} /><TableCell>Asset</TableCell><TableCell sx={{ width: 180 }}>Status</TableCell><TableCell sx={{ width: 100 }}>Size</TableCell><TableCell align="center" sx={{ width: 220 }}>Actions</TableCell></TableRow></TableHead><TableBody>{visibleAssets.map((asset) => {
			const effective = asset.id ? effectiveConfigurations.data?.configurations[String(asset.id)] : undefined;
			const track = trackProfiles.find((profile) => profile.key === effective?.tracks.profileKey);
			return <AssetRow key={`${asset.id}-${effective?.video.videoProfileId ?? 0}-${effective?.destination.destinationLibraryId ?? 0}`} asset={asset} autoOpenAssetInfo={normalizePath(asset.path) === normalizePath(legacyAssetPath)} compactUnprocessed libraries={libraries} profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} pathTrackProfile={track} assetCategories={assetCategories} groupCategory={effective?.category.category ?? ''} confidenceEnabled={!getDisabledConfidencePaths(settings).includes(path.path)} groupProfileId={effective?.video.videoProfileId ?? 0} groupAudioProfileKey={effective?.audio.profileKey ?? ''} groupLibraryId={effective?.destination.destinationLibraryId ?? 0} hasOpenJob={assetHasOpenJob(asset, queueJobs)} queueJobs={queueJobs} snapshotRunning={runningSnapshotPaths.has(asset.path)} mode="unprocessed" srtFallbackEnabled={subtitleSRTFallbackEnabled(settings)} bulkSelected={selectedAssetIds.has(asset.id as number)} bulkSelectionEnabled bulkSelectionDisabled={asset.missing} onBulkSelectionChange={(_, selected) => onAssetSelectionChange([asset.id as number], selected)} />;
		})}</TableBody></Table></Box>;
	};
	if (headerOnly) {
		return (
			<>
				<AssetsV2PathToolbar
					path={logicalGroup.path}
					mode="raw"
					selectedCount={assets.filter((asset) => selectedAssetIds.has(asset.id ?? 0)).length}
					breadcrumbs={(
						<Stack direction="row" alignItems="center" spacing={0.45} sx={{ minWidth: 0 }}>
							<Checkbox
								size="small"
								checked={groupSelection.checked}
								indeterminate={groupSelection.indeterminate}
								disabled={!assets.length}
								onChange={(event) => onAssetSelectionChange(assets.map((asset) => asset.id as number), event.target.checked)}
								inputProps={{ 'aria-label': 'Select title ' + logicalGroup.name }}
							/>
							<WorkspaceBreadcrumbs segments={breadcrumbSegments ?? ['Assets', 'RAW', logicalGroup.name]} label="RAW title location" />
						</Stack>
					)}
					primaryActions={(
						<>
							<Button size="small" variant="outlined" startIcon={<RefreshIcon />} onClick={() => setTitleSnapshotsOpen(true)} disabled={!titleSnapshotGroup || assets.every((asset) => asset.missing)}>
								Scan
							</Button>
							<ScopeConfigureButton targetType="logical_group" scopeKey={logicalGroup.path} label="Configure Path" profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} libraries={libraries} categories={assetCategories} compact readOnly={titleHasOpenJob} />
							<Button size="small" variant="outlined" startIcon={<InfoOutlinedIcon />} onClick={() => runTitleAdvisor.mutate()} disabled={!assets.length || runTitleAdvisor.isPending}>
								{runTitleAdvisor.isPending ? 'Running Advisor…' : 'Advisor'}
							</Button>
							<Button
								size="small"
								variant="contained"
								startIcon={<DriveFileMoveIcon />}
								onClick={() => publishTitleAsIs.mutate()}
								disabled={!assets.length || titleHasOpenJob || assets.some((asset) => asset.review?.requiresReview) || publishTitleAsIs.isPending}
							>
								{publishTitleAsIs.isPending ? 'Moving…' : 'Move to library'}
							</Button>
						</>
					)}
				/>
				{titleAdvisorResults.length ? <Alert severity={titleAdvisorResults.some((result) => result.error) ? 'warning' : 'success'} sx={{ mt: 0.6 }}>{titleAdvisorResults.length} asset{titleAdvisorResults.length === 1 ? '' : 's'} evaluated · {titleAdvisorResults.filter((result) => result.response?.recommendation === 'worth_it').length} comply · {titleAdvisorResults.filter((result) => result.response?.recommendation === 'maybe').length} review · {titleAdvisorResults.filter((result) => result.error).length} failed</Alert> : null}
				{publishTitleAsIs.isSuccess ? <Alert severity="success" sx={{ mt: 0.6 }}>{publishTitleAsIs.data.message}</Alert> : null}
				{publishTitleAsIs.isError && publishTitleAsIs.error.message !== 'Publish as-is canceled.' ? <Alert severity="warning" sx={{ mt: 0.6 }}>{publishTitleAsIs.error.message}</Alert> : null}
				{titleSnapshotGroup ? <PathSnapshotsDialog open={titleSnapshotsOpen} group={titleSnapshotGroup} title={logicalGroup.name} pathLabels={snapshotPathLabels} runningSnapshotPaths={runningSnapshotPaths} onClose={() => setTitleSnapshotsOpen(false)} /> : null}
			</>
		);
	}
	return <Box data-testid={headerOnly ? 'assets-path-toolbar' : undefined} aria-label={headerOnly ? 'RAW title path actions' : undefined} sx={{ border: headerOnly ? 1 : 1, borderColor: '#15304a', borderRadius: headerOnly ? 1 : 1, overflow: 'hidden', mb: headerOnly ? 0.7 : 0, bgcolor: headerOnly ? '#091a2c' : 'rgba(255,255,255,0.035)' }}>
		<Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={0.6} sx={{ px: headerOnly ? 1.25 : 1, py: headerOnly ? 0.55 : 1 }}>
			<Stack direction="row" alignItems="center" spacing={0.45} sx={{ minWidth: 0, flex: 1 }}><Checkbox size="small" checked={groupSelection.checked} indeterminate={groupSelection.indeterminate} disabled={!assets.length} onChange={(event) => onAssetSelectionChange(assets.map((asset) => asset.id as number), event.target.checked)} inputProps={{ 'aria-label': `Select title ${logicalGroup.name}` }} />{headerOnly ? <WorkspaceBreadcrumbs segments={breadcrumbSegments ?? ['Assets', 'RAW', logicalGroup.name]} label="RAW title location" /> : <><IconButton size="small" onClick={() => setExpanded((value) => !value)} aria-label={`${expanded ? 'Collapse' : 'Expand'} ${logicalGroup.name}`}><ExpandMoreIcon sx={{ transform: expanded ? 'rotate(180deg)' : 'none' }} /></IconButton><Box><Typography variant="h3">{logicalGroup.name}</Typography><Typography variant="body2" color="text.secondary">{countLabel(logicalGroup.assetCount, 'asset')} · {countLabel(logicalGroup.pathCount, 'path')} · {formatBytes(logicalGroup.totalSizeBytes)}</Typography></Box></>}</Stack>
			<Stack direction="row" spacing={0.45} flexWrap="wrap" useFlexGap>
				<ScopeConfigureButton targetType="logical_group" scopeKey={logicalGroup.path} label={headerOnly ? 'Configure Path' : 'Configure title'} profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} libraries={libraries} categories={assetCategories} compact={headerOnly} readOnly={titleHasOpenJob} />
				<Button size="small" variant="outlined" startIcon={<InfoOutlinedIcon />} onClick={() => runTitleAdvisor.mutate()} disabled={!assets.length || runTitleAdvisor.isPending} sx={headerOnly ? { minHeight: 30, px: 0.9, fontSize: '0.8rem', '& .MuiButton-startIcon': { mr: 0.35 }, '& svg': { fontSize: 15 } } : undefined}>{runTitleAdvisor.isPending ? 'Running Advisor…' : headerOnly ? 'Advisor' : 'Run Advisor'}</Button>
				{headerOnly ? (
					<>
						<Tooltip title="More title actions">
							<IconButton size="small" aria-label={`More title actions for ${logicalGroup.name}`} onClick={(event) => setTitleActionsAnchor(event.currentTarget)}>
								<MoreVertIcon fontSize="small" />
							</IconButton>
						</Tooltip>
						<Menu anchorEl={titleActionsAnchor} open={Boolean(titleActionsAnchor)} onClose={() => setTitleActionsAnchor(null)}>
							<MenuItem onClick={() => { setTitleActionsAnchor(null); setTitleSnapshotsOpen(true); }} disabled={!titleSnapshotGroup}>Snapshots</MenuItem>
							<MenuItem onClick={() => { setTitleActionsAnchor(null); publishTitleAsIs.mutate(); }} disabled={!assets.length || titleHasOpenJob || assets.some((asset) => asset.review?.requiresReview) || publishTitleAsIs.isPending}>
								{publishTitleAsIs.isPending ? 'Publishing…' : 'Publish as-is'}
							</MenuItem>
						</Menu>
					</>
				) : (
					<>
						<Button size="small" variant="outlined" startIcon={<ManageSearchIcon />} onClick={() => setTitleSnapshotsOpen(true)} disabled={!titleSnapshotGroup}>Snapshots</Button>
						<Button size="small" variant="outlined" color="success" onClick={() => publishTitleAsIs.mutate()} disabled={!assets.length || titleHasOpenJob || assets.some((asset) => asset.review?.requiresReview) || publishTitleAsIs.isPending}>{publishTitleAsIs.isPending ? 'Publishing…' : 'Publish as-is'}</Button>
					</>
				)}
			</Stack>
		</Stack>
		{titleAdvisorResults.length ? <Alert severity={titleAdvisorResults.some((result) => result.error) ? 'warning' : 'success'} sx={{ m: 1 }}>{titleAdvisorResults.length} asset{titleAdvisorResults.length === 1 ? '' : 's'} evaluated · {titleAdvisorResults.filter((result) => result.response?.recommendation === 'worth_it').length} comply · {titleAdvisorResults.filter((result) => result.response?.recommendation === 'maybe').length} review · {titleAdvisorResults.filter((result) => result.error).length} failed</Alert> : null}
		{publishTitleAsIs.isSuccess ? <Alert severity="success" sx={{ m: 1 }}>{publishTitleAsIs.data.message}</Alert> : null}
		{publishTitleAsIs.isError && publishTitleAsIs.error.message !== 'Publish as-is canceled.' ? <Alert severity="warning" sx={{ m: 1 }}>{publishTitleAsIs.error.message}</Alert> : null}
		{headerOnly ? null : <Collapse in={expanded} unmountOnExit>
      <Stack spacing={1} sx={{ p: 1.25 }}>
          {rootPaths.map((path) => 
            <Box key={path.id}>{renderAssets(path)}</Box>
          )}
          {childPaths.map((path) => { 
              const pathAssets = selectableAssetsForPaths([path]); 
              const checked = pathAssets.length > 0 && pathAssets.every((asset) => selectedAssetIds.has(asset.id as number)); 
              const some = pathAssets.some((asset) => selectedAssetIds.has(asset.id as number)); 
              const open = expandedPaths.has(path.id); 
              const pathHasOpenJob = safeArray(path.assets).some((asset) => assetHasOpenJob(asset, queueJobs)); 
              return <Box key={path.id} sx={{ border: 1, borderColor: 'divider', borderRadius: 1 }}>
                <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ px: 0.75, py: 0.5 }}><Stack direction="row" alignItems="center">
                  <Checkbox size="small" checked={checked} indeterminate={some && !checked} disabled={!pathAssets.length} onChange={(event) => onAssetSelectionChange(pathAssets.map((asset) => asset.id as number), event.target.checked)} inputProps={{ 'aria-label': `Select path ${path.displayPath || path.name}` }} />
                  <IconButton size="small" aria-label={`${open ? 'Collapse' : 'Expand'} path ${path.displayPath || path.name}`} onClick={() => setExpandedPaths((current) => { const next = new Set(current); if (next.has(path.id)) next.delete(path.id); else next.add(path.id); return next; })}>
                    <ExpandMoreIcon sx={{ transform: open ? 'rotate(180deg)' : 'none' }} />
                  </IconButton>
                    <Box>
                      <Typography fontWeight={700}>
                        {path.displayPath || path.name}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {countLabel(path.assetCount, 'asset')} · {formatBytes(path.totalSizeBytes)}
                      </Typography>
                    </Box>
                </Stack>
              <ScopeConfigureButton targetType="path" scopeKey={path.path} label="Configure" profiles={profiles} audioProfiles={audioProfiles} trackProfiles={trackProfiles} libraries={libraries} categories={assetCategories} compact readOnly={pathHasOpenJob} /></Stack><Collapse in={open} unmountOnExit><Box sx={{ p: 1 }}>{renderAssets(path)}</Box></Collapse></Box>; 
            })}
        </Stack>
		</Collapse>}
		{titleSnapshotGroup ? <PathSnapshotsDialog open={titleSnapshotsOpen} group={titleSnapshotGroup} title={logicalGroup.name} pathLabels={snapshotPathLabels} runningSnapshotPaths={runningSnapshotPaths} onClose={() => setTitleSnapshotsOpen(false)} /> : null}
	</Box>;
}

function assetGroupFromTreePath(path: import('../api/types').AssetPath): AssetGroup {
	const assets = safeArray(path.assets);
	return { id: path.id, libraryId: assets[0]?.libraryId ?? 0, libraryName: assets[0]?.libraryName ?? 'Originals', path: path.path, relativePath: path.relativePath, status: 'unprocessed', fileCount: path.fileCount, sizeBytes: path.totalSizeBytes, modifiedAt: assets[0]?.modifiedAt ?? '', assets, review: assets[0]?.review, pathReview: assets[0]?.review, metadata: assets[0]?.metadata, pathMetadata: assets[0]?.metadata } as AssetGroup;
}

function ScopeConfigureButton({ targetType, scopeKey, label, profiles, audioProfiles, trackProfiles, libraries, categories, compact = false, readOnly = false, destinationLocked = false }: { targetType: 'logical_group' | 'path' | 'asset'; scopeKey: string; label: string; profiles: Profile[]; audioProfiles: AudioEnhancementProfile[]; trackProfiles: TrackProfile[]; libraries: Library[]; categories: string[]; compact?: boolean; readOnly?: boolean; destinationLocked?: boolean }) {
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
			if (destinationLocked && fields.has('destination')) throw new Error('Library replacement keeps its current destination. Use migration to move libraries.');
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

  const profileScope = targetType === 'asset' ? 'asset' : 'path';

	return <><Button size="small" variant={compact ? 'text' : 'outlined'} startIcon={<EditIcon />} onClick={openConfiguration}>{label}</Button><Dialog open={open} onClose={() => !save.isPending && setOpen(false)} maxWidth="md" fullWidth><DialogTitle>{label}</DialogTitle><DialogContent dividers>{loading ? <Stack spacing={1}><LinearProgress /><Typography variant="body2" color="text.secondary">Loading persisted scope configuration…</Typography></Stack> : loadError ? <Alert severity="warning" action={<Button color="inherit" size="small" onClick={() => void loadPersistedConfiguration()}>Retry</Button>}>{loadError}</Alert> : <Stack spacing={1.5}>{readOnly ? <Alert severity="warning">This scope has an active Queue job. Configuration is read-only until the job finishes.</Alert> : <Alert severity="info">Choose exactly which dimensions to change. Unchecked dimensions remain untouched.</Alert>}<FormControlLabel control={<Checkbox checked={fields.has('video')} onChange={() => toggle('video')} disabled={readOnly} />} label="Change video" /><ProfileAutocomplete profiles={profiles.filter((profile) => profile.scope === profileScope && !profile.disabled && !profile.deletedAt)} value={video} onChange={setVideo} label="Video profile" allowNone allowInherit disabled={readOnly || !fields.has('video')} /><FormControlLabel control={<Checkbox checked={fields.has('audio')} onChange={() => toggle('audio')} disabled={readOnly} />} label="Change audio" /><AudioProfileAutocomplete profiles={audioProfiles.filter((profile) => profile.scope === profileScope && !profile.disabled && !profile.deletedAt)} value={audio} onChange={setAudio} label="Audio profile" allowInherit disabled={readOnly || !fields.has('audio')} /><FormControlLabel control={<Checkbox checked={fields.has('tracks')} onChange={() => toggle('tracks')} disabled={readOnly} />} label="Change tracks" /><TrackProfileAutocomplete profiles={trackProfiles.filter((profile) => profile.scope === profileScope && !profile.disabled && !profile.deletedAt)} value={tracks} onChange={setTracks} label="Tracks profile" allowInherit disabled={readOnly || !fields.has('tracks')} /><Divider />{destinationLocked ? <Typography variant="caption" color="text.secondary">Library replacement preserves this Library destination. Move to another Library through migration.</Typography> : null}<FormControlLabel control={<Checkbox checked={fields.has('category')} onChange={() => toggle('category')} disabled={readOnly} />} label="Change category" /><Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}><TextField select fullWidth label="Category mode" value={categoryMode} onChange={(event) => setCategoryMode(event.target.value as typeof categoryMode)} disabled={readOnly || !fields.has('category')}><MenuItem value="inherit">Inherit</MenuItem><MenuItem value="value">Override</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField><AssetCategorySelect value={category} options={categories} onChange={setCategory} label="Category" disabled={readOnly || !fields.has('category') || categoryMode !== 'value'} /></Stack><FormControlLabel control={<Checkbox checked={fields.has('destination')} onChange={() => toggle('destination')} disabled={readOnly || destinationLocked} />} label="Change destination" /><Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}><TextField select fullWidth label="Destination mode" value={destinationMode} onChange={(event) => setDestinationMode(event.target.value as typeof destinationMode)} disabled={readOnly || destinationLocked || !fields.has('destination')}><MenuItem value="inherit">Inherit</MenuItem><MenuItem value="value">Override</MenuItem><MenuItem value="disabled">Disabled</MenuItem></TextField><LibraryAutocomplete libraries={libraries} value={destination} onChange={setDestination} label="Destination" disabled={readOnly || destinationLocked || !fields.has('destination') || destinationMode !== 'value'} /></Stack>{save.isError ? <Alert severity="warning">{save.error instanceof Error ? save.error.message : 'Could not save configuration.'}</Alert> : null}</Stack>}</DialogContent><DialogActions><Button onClick={() => setOpen(false)} disabled={save.isPending}>Cancel</Button><Button variant="contained" onClick={() => save.mutate()} disabled={readOnly || loading || Boolean(loadError) || save.isPending || !fields.size || (fields.has('category') && categoryMode === 'value' && !category) || (fields.has('destination') && destinationMode === 'value' && !destination)}>Apply</Button></DialogActions></Dialog></>;
}

function UnprocessedSelectionToolbar({ selectedAssetIds, logicalGroups, profiles, audioProfiles, trackProfiles, libraries, categories, queueJobs, onClear }: {
	selectedAssetIds: Set<number>;
	logicalGroups: AssetLogicalGroup[];
	profiles: Profile[];
	audioProfiles: AudioEnhancementProfile[];
	trackProfiles: TrackProfile[];
	libraries: Library[];
	categories: string[];
	queueJobs: QueueJob[];
	onClear: () => void;
}) {
	const queryClient = useQueryClient();
	const [configureOpen, setConfigureOpen] = useState(false);
	const [queuePlan, setQueuePlan] = useState<PreparedSelectionQueue | null>(null);
	const [queueCommitResult, setQueueCommitResult] = useState<QueueSelectedAssetsResponse | null>(null);
	const [queueAssetIds, setQueueAssetIds] = useState<number[]>([]);
	const [queueMode, setQueueMode] = useState<'selected' | 'remaining'>('selected');
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
	const selectedAssetsWithOpenJobs = selectedAssets.filter((asset) => assetHasOpenJob(asset, queueJobs));
	const allSelectedAssetsHaveOpenJobs = selectedAssets.length > 0 && selectedAssetsWithOpenJobs.length === selectedAssets.length;
	const selectedGroupHasOpenJob = fullySelectedGroups.some((group) => group.assetPaths.some((path) => path.assets.some((asset) => !asset.missing && assetHasOpenJob(asset, queueJobs))));
	const remainingAssetIds = Array.from(new Set(fullySelectedGroups.flatMap((group) => group.assetPaths.flatMap((path) => path.assets).filter((asset) => !asset.missing && asset.id).map((asset) => asset.id as number))));

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
		mutationFn: async (input: { assetIds: number[]; remaining: boolean }) => {
			setQueueAssetIds(input.assetIds);
			setQueueMode(input.remaining ? 'remaining' : 'selected');
			return input.remaining
				? api.queueRemainingPreview(input.assetIds)
				: api.queueSelectedAssets({ assetIds: input.assetIds, commit: false });
		},
		onSuccess: (response) => { setQueueCommitResult(null); setQueuePlan(response); },
	});

		const createQueue = useMutation({
		mutationFn: async () => {
			if (!queuePlan?.summary.eligible) throw new Error('No selected assets are eligible for Queue.');
			return api.preflightSelectedAssets(queueAssetIds);
		},
		onSuccess: async (candidate) => {
			await Promise.all([
				queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
				queryClient.invalidateQueries({ queryKey: ['assets'] }),
			]);
			setQueueCommitResult(null);
			setQueuePlan(null);
			setQueueAssetIds([]);
			openQueueBatchPreflight(candidate);
		},
	});
	const displayedQueueResult = queueCommitResult ?? queuePlan;
	const queueReasonCount = (reason: string) => displayedQueueResult?.results.filter((result) => result.reason === reason).length ?? 0;
	const closeQueueDialog = () => { setQueuePlan(null); setQueueCommitResult(null); setQueueAssetIds([]); };

	return <>
		<Box sx={{
			mt: 1,
			px: 0.85,
			py: 0.65,
			border: 1,
			borderColor: '#15304a',
			borderRadius: '6px',
			bgcolor: '#091a2c',
			position: 'sticky',
			top: 8,
			zIndex: 2,
			'& .MuiButton-root': {
				minHeight: 28,
				px: 1,
				borderRadius: '5px',
				fontSize: '0.8rem',
				fontWeight: 600,
				lineHeight: 1,
				textTransform: 'none',
				boxShadow: 'none',
				'& .MuiButton-startIcon': { mr: 0.4 },
				'& .MuiButton-startIcon svg': { fontSize: 14 },
				'&.Mui-disabled': { color: '#63758a', borderColor: 'rgba(21, 48, 74, 0.56)', bgcolor: 'rgba(12, 32, 53, 0.38)', boxShadow: 'none' },
			},
			'& .MuiButton-contained': { bgcolor: '#31238f', border: '1px solid rgba(108, 76, 246, 0.62)', color: '#e7e0ff' },
			'& .MuiButton-contained:hover': { bgcolor: '#3a2aa0', boxShadow: 'none' },
			'& .MuiButton-outlined': { bgcolor: '#0c2035', borderColor: '#1f4467', color: '#9cb0c8' },
			'& .MuiButton-outlined:hover': { bgcolor: '#112a44', borderColor: '#2b5a81', color: '#e7eef8' },
			'& .MuiButton-text:last-child': { minWidth: 0, px: 0.7, color: '#59718b', '&:hover': { bgcolor: 'transparent', color: '#9cb0c8' } },
		}}>
			<Stack direction={{ xs: 'column', md: 'row' }} alignItems={{ xs: 'stretch', md: 'center' }} justifyContent="space-between" spacing={0.65}>
				<Typography variant="body2" sx={{ color: '#7890a8', fontSize: '0.75rem', fontWeight: 500 }}>{countLabel(fullySelectedGroups.length, 'title')} selected · {countLabel(selectedAssets.length, 'asset')} · {formatBytes(selectedSize)}</Typography>
				<Stack direction="row" spacing={0.45} flexWrap="wrap" useFlexGap><Button size="small" variant="contained" startIcon={<PlayArrowIcon sx={{ color: '#b9adff' }} />} onClick={() => prepareQueue.mutate({ assetIds: selectedAssets.map((asset) => asset.id as number), remaining: false })} disabled={prepareQueue.isPending || allSelectedAssetsHaveOpenJobs}>{prepareQueue.isPending ? 'Resolving…' : 'Queue selected'}</Button><Button size="small" variant="outlined" startIcon={<PlayArrowIcon />} onClick={() => prepareQueue.mutate({ assetIds: remainingAssetIds, remaining: true })} disabled={prepareQueue.isPending || !remainingAssetIds.length}>{prepareQueue.isPending ? 'Resolving…' : 'Queue Remaining'}</Button><Button size="small" variant="outlined" startIcon={<EditIcon />} disabled={!fullySelectedGroups.length || selectedGroupHasOpenJob} onClick={() => { setConfigurationFields(new Set()); setConfigureOpen(true); }}>Configure selected</Button><Button size="small" variant="text" onClick={onClear}>Clear</Button></Stack>
			</Stack>
			{selectedAssetsWithOpenJobs.length ? <Alert severity="info" sx={{ mt: 1 }}>{countLabel(selectedAssetsWithOpenJobs.length, 'asset')} already locked by an active Queue job. Read-only details remain available.</Alert> : null}
			{prepareQueue.isError ? <Alert severity="warning" sx={{ mt: 1 }}>{prepareQueue.error instanceof Error ? prepareQueue.error.message : 'Could not resolve the selected assets.'}</Alert> : null}
		</Box>
		<Dialog open={Boolean(queuePlan)} onClose={() => !createQueue.isPending && closeQueueDialog()} maxWidth="sm" fullWidth>
			<DialogTitle>{queueMode === 'remaining' ? 'Queue Remaining titles' : 'Queue selected titles'}</DialogTitle>
			<DialogContent dividers>
				{displayedQueueResult ? <Stack spacing={1}>
					<Typography>Titles: {displayedQueueResult.summary.titleCount}</Typography>
					<Typography>Assets selected: {displayedQueueResult.summary.selected}</Typography>
					<Typography>Selectable: {displayedQueueResult.summary.selected} · Eligible: {displayedQueueResult.summary.eligible} · Skipped: {displayedQueueResult.summary.skipped} · Blocked: {displayedQueueResult.summary.failed}</Typography>
					<Typography>{queueCommitResult ? 'Queued' : 'Will be queued'}: {queueCommitResult ? displayedQueueResult.summary.queued : displayedQueueResult.summary.eligible}</Typography>
					<Typography>Estimated input size: {formatBytes(displayedQueueResult.summary.sizeBytes)}</Typography>
					<Typography>Already queued: {queueReasonCount('already_queued')}</Typography>
					<Typography>Needs review: {queueReasonCount('needs_review')}</Typography>
					<Typography>Missing: {queueReasonCount('missing')}</Typography>
					<Typography>Blocked or incomplete configuration: {queueReasonCount('invalid_configuration') + queueReasonCount('not_found') + queueReasonCount('active_maintenance')}</Typography>
					<Divider />
					<Typography variant="body2" color="text.secondary">Queue is resolved independently for every asset. Commit resubmits the same complete Asset ID set and revalidates current eligibility.</Typography>
					<Box sx={{ maxHeight: 220, overflowY: 'auto' }}>
						<Typography variant="caption" color="text.secondary">Persisted analysis evidence · read-only preview</Typography>
						{displayedQueueResult.results.map((result) => <Stack key={result.assetId} direction="row" spacing={1} alignItems="center">
							<Typography variant="body2">Asset {result.assetId}</Typography>
							<AnalysisEvidenceChip status={result.analysisEvidence?.status ?? 'unavailable'} />
							{result.analysisEvidence?.sourceIdentity?.snapshotId ? <Typography variant="caption" color="text.secondary">Snapshot #{result.analysisEvidence.sourceIdentity.snapshotId}</Typography> : null}
							{result.analysisEvidence?.staleComponents?.length ? <Typography variant="caption" color="text.secondary">{result.analysisEvidence.staleComponents.join(', ')}</Typography> : null}
						</Stack>)}
					</Box>
					{queueCommitResult ? <Alert severity={queueCommitResult.summary.skipped || queueCommitResult.summary.failed ? (queueCommitResult.summary.queued ? 'warning' : 'error') : 'success'}>{queueCommitResult.summary.queued} queued · {queueCommitResult.summary.skipped} skipped · {queueCommitResult.summary.failed} failed{queueCommitResult.results.filter((result) => result.outcome === 'skipped' || result.outcome === 'failed').map((result) => <Typography key={result.assetId} component="div" variant="body2">Asset {result.assetId}: {result.message ?? result.reason}</Typography>)}</Alert> : null}
					{createQueue.isError ? <Alert severity="warning">{createQueue.error instanceof Error ? createQueue.error.message : 'Could not queue the selection.'}</Alert> : null}
				</Stack> : null}
			</DialogContent>
			<DialogActions><Button onClick={closeQueueDialog} disabled={createQueue.isPending}>{queueCommitResult ? 'Close' : 'Cancel'}</Button><Button variant="contained" onClick={() => createQueue.mutate()} disabled={createQueue.isPending || Boolean(queueCommitResult) || !queuePlan?.summary.eligible}>{queueMode === 'remaining' ? 'Queue Remaining' : 'Queue'} {queuePlan?.summary.eligible ?? 0} assets</Button></DialogActions>
		</Dialog>
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
  mode: 'unprocessed' | 'library' | 'converted';
  columnCount: number;
}) {
  const queryClient = useQueryClient();
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
		const selectedAssets = selectedGroups.flatMap((group) => safeArray(group.assets));
		if (selectedAssets.some((asset) => typeof asset.id !== 'number' || asset.id <= 0)) {
			throw new Error('Every asset in the selected paths needs a persisted identity before Batch Preflight. Refresh Assets and try again.');
		}
		const assetIds = Array.from(new Set(selectedAssets.map((asset) => asset.id as number)));
		if (!assetIds.length) throw new Error('The selected paths have no available assets to review.');
		return api.preflightSelectedAssets(assetIds);
    },
		onSuccess: async (candidate) => {
		await Promise.all([queryClient.invalidateQueries({ queryKey: ['queueJobs'] }), queryClient.invalidateQueries({ queryKey: ['assets'] })]);
		openQueueBatchPreflight(candidate);
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
  title,
  pathLabels,
  runningSnapshotPaths,
  onClose,
}: {
  open: boolean;
  group: AssetGroup;
  title?: string;
  pathLabels?: Record<string, string>;
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
      <DialogTitle>Snapshots · {title ?? pathLabelForCollection(group)}</DialogTitle>
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
                      inputProps={{ 'aria-label': `Select all assets in ${title ?? group.path} for snapshot generation` }}
                    />
                  </TableCell>
                  <TableCell>Asset</TableCell>
                  {pathLabels ? <TableCell>Path</TableCell> : null}
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
                      {pathLabels ? <TableCell>{pathLabels[normalizePath(asset.path)] ?? 'Root'}</TableCell> : null}
                      <TableCell>{state?.snapshot?.createdAt ? formatDate(state.snapshot.createdAt) : '—'}</TableCell>
                      <TableCell>{query.isLoading ? <Chip size="small" label="Checking…" /> : <SnapshotStateChip status={running ? 'running' : state?.status ?? 'unavailable'} />}</TableCell>
                      <TableCell align="right"><Button size="small" variant="outlined" disabled={running || generateSnapshots.isPending} onClick={() => generateSnapshots.mutate([asset.path])}>{running ? 'Generating…' : state?.snapshot ? 'Regenerate' : 'Generate'}</Button></TableCell>
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

function analysisEvidenceLabel(status?: string) {
  switch (status) {
    case 'missing': return 'Missing';
    case 'ready': return 'Ready';
    case 'stale': return 'Stale';
    case 'unavailable': return 'Unavailable';
    default: return 'Unavailable';
  }
}

function latestSnapshotAnalysisStatus(snapshot?: Awaited<ReturnType<typeof api.latestSnapshot>>) {
  if (snapshot?.analysisEvidence?.status) return snapshot.analysisEvidence.status;
  // Older server responses already classify persisted evidence through these
  // fields. Keep the UI compatible without inferring identity from its path.
  if (!snapshot?.snapshot) return snapshot?.status === 'unavailable' ? 'unavailable' : 'missing';
  if (snapshot.requiresAnalysis || snapshot.status === 'stale' || snapshot.status === 'changed' || snapshot.status === 'legacy') return 'stale';
  return snapshot.status === 'unavailable' ? 'unavailable' : 'ready';
}

function AnalysisEvidenceChip({ status }: { status: 'missing' | 'ready' | 'stale' | 'unavailable' }) {
  const color = status === 'ready' ? 'success' : status === 'stale' ? 'warning' : 'default';
  return <Chip size="small" label={analysisEvidenceLabel(status)} color={color} />;
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
  pathSelected = false,
  onPathSelectionChange,
  onOpenPathSnapshots,
	collectionManaged = false,
	forceExpanded = false,
  hidePathHeader = false,
  compactPathHeader = false,
	v2ToolbarOnly = false,
	selectedCount = 0,
  breadcrumbSegments,
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
  mode: 'unprocessed' | 'library' | 'converted';
  pathSelected?: boolean;
  onPathSelectionChange?: (selected: boolean) => void;
  onOpenPathSnapshots?: () => void;
	collectionManaged?: boolean;
	forceExpanded?: boolean;
	hidePathHeader?: boolean;
	compactPathHeader?: boolean;
	v2ToolbarOnly?: boolean;
	selectedCount?: number;
  breadcrumbSegments?: string[];
	selectedAssetIds?: Set<number>;
	onAssetSelectionChange?: (assetIds: number[], selected: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const profileAssignments = useQuery({ queryKey: ['profileAssignments'], queryFn: api.profileAssignments });
  const [expanded, setExpanded] = useState(false);
  const [pathActionsAnchor, setPathActionsAnchor] = useState<HTMLElement | null>(null);
  const pathVideoProfiles = profiles.filter((profile) => profile.scope === 'path');
  const pathAudioProfiles = audioProfiles.filter((profile) => profile.scope === 'path');
  const pathTrackProfiles = trackProfiles.filter((profile) => profile.scope === 'path');
  const [selectedProfileId, setSelectedProfileId] = useState<number>(0);
  const [selectedAudioProfileKey, setSelectedAudioProfileKey] = useState<string>('');
  const pathTrackAssignments = getTrackProfilePathAssignments(settings);
  const [selectedTrackProfileKey, setSelectedTrackProfileKey] = useState<string>(pathTrackAssignments[normalizePath(group.path)] ?? '');
  const [selectedAssetPaths, setSelectedAssetPaths] = useState<string[]>([]);
  const groupAssets = safeArray(group.assets);
  const pathConfigurationAssetIds = Array.from(new Set(groupAssets.filter((asset) => !asset.missing && typeof asset.id === 'number' && asset.id > 0).map((asset) => asset.id as number)));
  const pathEffectiveConfigurations = useQuery({
    queryKey: ['effectiveAssetConfiguration', 'path-toolbar', normalizePath(group.path), pathConfigurationAssetIds],
    queryFn: () => api.effectiveAssetConfigurations(pathConfigurationAssetIds),
    enabled: v2ToolbarOnly && pathConfigurationAssetIds.length > 0,
  });
  const effectiveConfigurationsByPath = useMemo(() => new Map(
    Object.values(pathEffectiveConfigurations.data?.configurations ?? {}).map((configuration) => [normalizePath(configuration.assetPath), configuration]),
  ), [pathEffectiveConfigurations.data]);
  const pathMetadata = group.pathMetadata ?? { categories: [], tags: [], updatedAt: '' };
  const inheritedPathCategory = firstCategory(pathMetadata.categories);
  const groupReview = group.review ?? { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' };
	const effectiveDestinationIds = Array.from(new Set(groupAssets.filter((asset) => !asset.missing).map((asset) => {
		const destination = effectiveConfigurationsByPath.get(normalizePath(asset.path))?.destination;
		return destination?.selection === 'value' ? destination.destinationLibraryId ?? 0 : 0;
	}).filter((id) => id > 0)));
	const availablePathAssetCount = groupAssets.filter((asset) => !asset.missing).length;
	const pathHasCanonicalDestinations = availablePathAssetCount > 0 && groupAssets.filter((asset) => !asset.missing).every((asset) => {
		const destination = effectiveConfigurationsByPath.get(normalizePath(asset.path))?.destination;
		return destination?.selection === 'value' && (destination.destinationLibraryId ?? 0) > 0;
	});
	const configuredPathDestination = mode === 'library'
		? group.libraryId ?? 0
		: pathEffectiveConfigurations.data && effectiveDestinationIds.length === 1 &&
			groupAssets.filter((asset) => !asset.missing).every((asset) => effectiveConfigurationsByPath.get(normalizePath(asset.path))?.destination.selection === 'value')
			? effectiveDestinationIds[0]
			: 0;
  const [selectedLibraryId, setSelectedLibraryId] = useState<number>(configuredPathDestination);
  const [migrationLibraryId, setMigrationLibraryId] = useState<number>(0);
  const [renamePathOpen, setRenamePathOpen] = useState(false);
  const [pathSnapshotsOpen, setPathSnapshotsOpen] = useState(false);
  const [groupCategory, setGroupCategory] = useState<string>(inheritedPathCategory);
  const [pathAdvisorOpen, setPathAdvisorOpen] = useState(false);
  const [pathAdvisorCurrent, setPathAdvisorCurrent] = useState('');
  const [pathAdvisorResults, setPathAdvisorResults] = useState<Array<{
    asset: Asset;
    response?: AdvisorResponse;
    error?: string;
  }>>([]);
  const effectiveProfileId = v2ToolbarOnly
    ? (groupAssets.map((asset) => effectiveConfigurationsByPath.get(normalizePath(asset.path))?.video)
      .find((video) => video?.selection === 'profile' && (video.videoProfileId ?? 0) > 0)?.videoProfileId ?? 0)
    : selectedProfileId > 0 ? selectedProfileId : 0;
  const representativeAsset = firstAssetForGroup(groupAssets.filter((asset) => !asset.missing));
  const lifecycleSummary = group.lifecycleSummary;
  const { isMixedGroup, isConvertedGroup, isPublishedAsIsGroup, isAcceptedGroup } = assetGroupLifecycleFlags(group, groupAssets);
  const isLibraryGroup = mode === 'library' || group.status === 'unverified' || group.status === 'accepted' || group.status === 'library' || group.status === 'published_as_is';
  const pathQueueDestinationUnavailable = v2ToolbarOnly && !isLibraryGroup
    ? !pathHasCanonicalDestinations
    : !selectedLibraryId;
  const isReadOnlyGroup = isMixedGroup || isConvertedGroup || isPublishedAsIsGroup || isAcceptedGroup;
  const showConfidenceColumn = mode !== 'converted';
  const bulkSelectableAssets = mode === 'library'
    ? librarySafeBulkSelectableAssets(group)
    : isReadOnlyGroup && !isAcceptedGroup ? groupAssets.filter((asset) => !asset.missing) : [];
  const hasMultipleSelectableAssets = bulkSelectableAssets.length > 1;
	const hierarchicalSelectableAssets = selectedAssetIds
    ? mode === 'library' ? bulkSelectableAssets : groupAssets.filter((asset) => !asset.missing && asset.id)
    : bulkSelectableAssets;
	const allBulkAssetsSelected = hierarchicalSelectableAssets.length > 0 && hierarchicalSelectableAssets.every((asset) => selectedAssetIds ? selectedAssetIds.has(asset.id ?? 0) : selectedAssetPaths.includes(asset.path));
	const someHierarchicalAssetsSelected = hierarchicalSelectableAssets.some((asset) => selectedAssetIds ? selectedAssetIds.has(asset.id ?? 0) : selectedAssetPaths.includes(asset.path));
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
	}, [profileAssignments.data, group.path]);
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
  }, [configuredPathDestination, group.path, pathEffectiveConfigurations.data]);
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
  const canPublishPathAsIs = Boolean(selectedLibraryId && !publishAsIs.isPending && groupAssets.length && !groupAssets.some((asset) => asset.review?.requiresReview || assetHasOpenJob(asset, queueJobs)));
  const publishPathAsIs = () => {
    const destination = libraries.find((library) => library.id === selectedLibraryId);
    if (!destination || !canPublishPathAsIs) return;
    if (!window.confirm(`Publish ${groupAssets.length} original asset(s) as-is to ${destination.name}? FFmpeg will not run and the files will be moved from Raw to Library.`)) return;
    publishAsIs.mutate({ sourcePath: group.path, destinationLibraryId: destination.id });
  };
  const runPathAdvisor = useMutation({
    mutationFn: async () => {
      const candidates = groupAssets.filter((asset) => !asset.missing);
      const effectiveConfigurations = v2ToolbarOnly && pathConfigurationAssetIds.length
        ? new Map(Object.values((await api.effectiveAssetConfigurations(pathConfigurationAssetIds)).configurations)
          .map((configuration) => [normalizePath(configuration.assetPath), configuration]))
        : undefined;
      setPathAdvisorResults([]);
      for (const asset of candidates) {
        setPathAdvisorCurrent(asset.path);
        const effectiveVideo = effectiveConfigurations
          ? effectiveConfigurations.get(normalizePath(asset.path))?.video
          : undefined;
        const profileId = effectiveConfigurations
          ? effectiveVideo?.selection === 'profile' ? effectiveVideo.videoProfileId ?? 0 : 0
          : effectiveProfileId;
        if (profileId <= 0) {
          setPathAdvisorResults((current) => [...current, {
            asset,
            error: 'Advisor needs an effective video profile for this asset. Configure a profile before evaluating.',
          }]);
          continue;
        }
        try {
          const response = await api.evaluateAdvisor({ mediaPath: asset.path, profileId });
          queryClient.setQueryData(['advisor', 'asset-row', asset.path, profileId], response);
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
      const batchId = createBatchId(group);
      const batchName = groupDisplayPath(group);
      const queueAssets = groupAssets.filter((asset) => !asset.missing);
      const freshEffectiveConfigurations = v2ToolbarOnly && queueAssets.length
        ? new Map(Object.values((await api.effectiveAssetConfigurations(queueAssets.map((asset) => asset.id).filter((id): id is number => typeof id === 'number' && id > 0))).configurations)
          .map((configuration) => [normalizePath(configuration.assetPath), configuration]))
        : undefined;
      if (v2ToolbarOnly && !freshEffectiveConfigurations) {
        throw new Error('Effective path configuration is unavailable. Refresh Assets and try again.');
      }
      const batchJobs: QueueJobInput[] = queueAssets.map((asset) => {
        const effective = freshEffectiveConfigurations
          ? freshEffectiveConfigurations.get(normalizePath(asset.path))
          : undefined;
        if (v2ToolbarOnly && !effective) {
          throw new Error(`Effective configuration is unavailable for ${asset.fileName}. Refresh Assets and try again.`);
        }
        const destinationLibraryId = isLibraryGroup
          ? asset.libraryId
          : effective?.destination.selection === 'value'
            ? effective.destination.destinationLibraryId ?? 0
            : v2ToolbarOnly ? 0 : selectedLibraryId;
        if (destinationLibraryId <= 0) {
          throw new Error(`No effective destination library is configured for ${asset.fileName}. Configure a destination before Queue / Preflight.`);
        }
        const video = effective?.video;
        const audio = effective?.audio;
        const tracks = effective?.tracks;
        return {
        mediaPath: asset.path,
        publishMode: isLibraryGroup
          ? 'replace_library_asset'
          : 'standard',

        libraryId: destinationLibraryId,

        profileId: video?.selection === 'profile' ? video.videoProfileId ?? 0 : v2ToolbarOnly ? 0 : effectiveProfileId,
        audioProfileKey: audio?.selection === 'profile' ? audio.profileKey ?? '' : v2ToolbarOnly ? '' : selectedAudioProfileKey,
        trackProfileKey: tracks?.selection === 'profile' ? tracks.profileKey ?? '' : v2ToolbarOnly ? '' : selectedTrackProfileKey,
        resolveProfileAssignments: true,
        processingMode: video?.selection === 'audio_only' || video?.selection === 'disabled' || (!v2ToolbarOnly && selectedProfileId < 0)
          ? 'audio_only'
          : 'full_encode',
        priority: 5,

        notes: queueNotes(
          `Queued from folder: ${batchName}`,
          selectedAudioProfileKey,
        ),
      };
      },
    );
    if (!batchJobs.length) throw new Error('This path has no available assets to queue.');

    return api.createQueueBatchPreflight({
      batchId,
      batchName,
      jobs: batchJobs,
    });
    },
    onSuccess: async (candidate) => {
	  await Promise.all([
		queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
		queryClient.invalidateQueries({ queryKey: ['assets'] }),
	  ]);
	  openQueueBatchPreflight(candidate);
    },
  });
  const bulkAssetAction = useMutation({
    mutationFn: async (paths: string[]) => {
      const failures: string[] = [];
      let completed = 0;
      for (const assetPath of paths) {
        try {
          if (isPublishedAsIsGroup) {
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
    setExpanded((current) => !current);
  }

  function startPathAdvisor() {
    setPathAdvisorOpen(true);
    runPathAdvisor.mutate();
  }

  function queuePath() {
    if (isLibraryGroup && !window.confirm(`Convert and safely replace ${groupAssets.length} Library asset(s) after validation? Every original will be archived.`)) return;
    queueGroup.mutate();
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
    const confirmed = window.confirm(
      `${isPublishedAsIsGroup ? 'Return' : 'Safely delete'} ${paths.length} asset(s) from this path? ${
        isPublishedAsIsGroup
            ? 'Each Published as-is asset and its external subtitles will return to the original Raw path.'
            : 'Each converted file will be deleted only if its archived original can be restored to Raw.'
      }`,
    );
    if (confirmed) bulkAssetAction.mutate(paths);
  }

  const migrationControls = (isLibraryGroup || isConvertedGroup) ? (
    <Stack direction="row" spacing={0.45} alignItems="center" flexWrap="wrap" useFlexGap>
      <Box
        data-testid="destination-library-control"
        sx={{
          width: { xs: 132, sm: 156 },
          '& .MuiInputBase-root': { height: 32, fontSize: '0.75rem', borderRadius: '5px' },
          '& .MuiOutlinedInput-input': { py: 0.45, px: 0.8 },
          '& .MuiInputLabel-root': { fontSize: '0.75rem', transform: 'translate(12px, 8px) scale(1)' },
          '& .MuiInputLabel-root.MuiInputLabel-shrink': { transform: 'translate(12px, -7px) scale(0.75)' },
          '& .MuiAutocomplete-endAdornment': { right: 2 },
          '& .MuiAutocomplete-popupIndicator': { p: 0.25 },
          '& .MuiAutocomplete-popupIndicator svg': { fontSize: 16 },
        }}
      >
        <LibraryAutocomplete
          libraries={libraries.filter((library) => library.id !== group.libraryId)}
          value={migrationLibraryId}
          onChange={setMigrationLibraryId}
          label="Destination library"
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
        sx={{ minHeight: 30, px: 0.9, fontSize: '0.8rem', whiteSpace: 'nowrap' }}
      >
        {migratePath.isPending ? 'Moving...' : 'Move path'}
      </Button>
      <Button
        variant="outlined"
        size="small"
        startIcon={<DriveFileRenameOutlineIcon />}
        disabled={!group.libraryId || groupAssets.some((asset) => assetHasOpenJob(asset, queueJobs))}
        onClick={() => setRenamePathOpen(true)}
        sx={{ minHeight: 30, px: 0.9, fontSize: '0.8rem', whiteSpace: 'nowrap' }}
      >
        Rename path
      </Button>
    </Stack>
  ) : null;
  const v2PathToolbar = v2ToolbarOnly ? (
    <>
      <AssetsV2PathToolbar
        path={group.path}
        breadcrumbs={<WorkspaceBreadcrumbs segments={breadcrumbSegments ?? ['Assets', group.libraryName, pathLabelForCollection(group)]} label={`${mode === 'unprocessed' ? 'RAW' : 'Library'} location`} />}
        selectedCount={selectedAssetIds
          ? groupAssets.filter((asset) => selectedAssetIds.has(asset.id ?? 0)).length
          : selectedCount}
        mode={mode === 'unprocessed' ? 'raw' : 'library'}
        primaryActions={(
          <>
            {mode !== 'converted' ? (
              <Button size="small" variant="outlined" startIcon={<RefreshIcon sx={{ color: '#8ca7ff' }} />} onClick={() => setPathSnapshotsOpen(true)} disabled={groupAssets.every((asset) => asset.missing)} sx={{ minHeight: 30, px: 0.9, fontSize: '0.8rem', '& .MuiButton-startIcon': { mr: 0.35 }, '& svg': { fontSize: 14 } }}>
                Scan
              </Button>
            ) : null}
            {!isReadOnlyGroup && mode !== 'converted' ? (
              <Button
                size="small"
                variant="outlined"
                startIcon={<InfoOutlinedIcon />}
                onClick={startPathAdvisor}
                disabled={!isConfidenceEnabled || effectiveProfileId <= 0 || (v2ToolbarOnly && (pathEffectiveConfigurations.isLoading || pathEffectiveConfigurations.isError)) || groupAssets.every((asset) => asset.missing) || runPathAdvisor.isPending}
                sx={{ minHeight: 30, px: 0.9, fontSize: '0.8rem', '& .MuiButton-startIcon': { mr: 0.35 }, '& svg': { fontSize: 15 } }}
              >
                Advisor
              </Button>
            ) : null}
            {(mode === 'unprocessed' || isLibraryGroup) && !isReadOnlyGroup ? (
              <Button
                size="small"
                variant="contained"
                startIcon={<PlayArrowIcon sx={{ color: '#8d7aff' }} />}
                onClick={queuePath}
                disabled={profileAssignments.isLoading || queueGroup.isPending || updateProfileAssignment.isPending || (v2ToolbarOnly && (pathEffectiveConfigurations.isLoading || pathEffectiveConfigurations.isError)) || pathQueueDestinationUnavailable || !groupAssets.length || groupAssets.some((asset) => asset.review?.requiresReview || assetHasOpenJob(asset, queueJobs))}
                sx={{ minHeight: 30, px: 0.9, fontSize: '0.8rem', '& .MuiButton-startIcon': { mr: 0.35 }, '& svg': { fontSize: 15 } }}
              >
                Queue / Preflight
              </Button>
            ) : null}
            {mode !== 'converted' && !isReadOnlyGroup ? (
              <Tooltip title="Assign profiles and configure path-level settings">
                <Box>
                  <ScopeConfigureButton
                    targetType="path"
                    scopeKey={group.path}
                    label="Configure Path"
                    profiles={profiles}
                    audioProfiles={audioProfiles}
                    trackProfiles={trackProfiles}
                    libraries={libraries}
                    categories={assetCategories}
                    compact
                    destinationLocked={isLibraryGroup}
                    readOnly={groupAssets.some((asset) => assetHasOpenJob(asset, queueJobs))}
                  />
                </Box>
              </Tooltip>
            ) : null}
            {mode !== 'converted' && !isReadOnlyGroup ? (
              <Tooltip title="Confidence controls explicit Advisor evaluation for this path. Turning it off skips Advisor; manual Queue remains available." describeChild>
                <FormControlLabel sx={{ m: 0, '& .MuiFormControlLabel-label': { fontSize: '0.75rem' } }}
                  label={`Confidence ${isConfidenceEnabled ? 'On' : 'Off'}`}
                  control={<Switch size="small" checked={isConfidenceEnabled} onChange={(event) => toggleConfidence(event.target.checked)} disabled={updateSetting.isPending || groupAssets.some((asset) => assetHasOpenJob(asset, queueJobs))} />} />
              </Tooltip>
            ) : null}
            {mode === 'unprocessed' && !isReadOnlyGroup ? (
              <Tooltip title="Publish these originals as-is to the configured Library">
                <span>
                  <Button size="small" variant="outlined" onClick={publishPathAsIs} disabled={!canPublishPathAsIs} sx={{ minHeight: 30, px: 0.9, fontSize: '0.8rem', '& .MuiButton-startIcon': { mr: 0.35 }, '& svg': { fontSize: 14 } }}>
                    Move to library
                  </Button>
                </span>
              </Tooltip>
            ) : null}
          </>
        )}
        pathOperations={migrationControls}
        destructiveActions={bulkSelectableAssets.length ? (close) => {
          const selectedPaths = selectedAssetIds
            ? bulkSelectableAssets.filter((asset) => selectedAssetIds.has(asset.id ?? 0)).map((asset) => asset.path)
            : selectedAssetPaths;
          const pathTargets = bulkSelectableAssets.map((asset) => asset.path);
          return (
            <>
              {selectedPaths.length ? (
                <Button size="small" color="error" variant="contained" onClick={() => { close(); runBulkAssetAction(selectedPaths); }} disabled={bulkAssetAction.isPending} sx={{ minHeight: 30, px: 0.9, fontSize: '0.8rem', whiteSpace: 'nowrap', bgcolor: '#4b1826', color: '#ff92a3', border: 1, borderColor: '#982841', boxShadow: 'none', '&:hover': { bgcolor: 'rgba(255, 78, 106, 0.2)', boxShadow: 'none' }, '& svg': { fontSize: 14 } }}>
                  {isPublishedAsIsGroup ? 'Return selected' : 'Delete selected'}
                </Button>
              ) : null}
              <Button size="small" color="error" variant="text" onClick={() => { close(); runBulkAssetAction(pathTargets); }} disabled={bulkAssetAction.isPending || bulkSelectableAssets.some((asset) => assetHasOpenJob(asset, queueJobs))} sx={{ minHeight: 30, px: 0.8, fontSize: '0.8rem', color: '#ff778d', whiteSpace: 'nowrap' }}>
                {isPublishedAsIsGroup ? 'Return path to Raw' : 'Delete path'}
              </Button>
            </>
          );
        } : undefined}
      />
      <Stack spacing={0.45} sx={{ px: 0.5, pb: 0.35 }} aria-live="polite" data-testid="assets-v2-path-feedback">
        {updateSetting.isError ? <Alert severity="error">{updateSetting.error instanceof Error ? updateSetting.error.message : 'Could not save Confidence setting.'}</Alert> : null}
        {queueGroup.isPending ? <Alert severity="info" sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>Preparing Queue / Preflight for this path…</Alert> : null}
        {queueGroup.isSuccess ? <Alert severity="success" sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>Preflight is ready. Review and confirm the candidate in Queue.</Alert> : null}
        {queueGroup.isError ? <Alert severity="error" sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>{queueGroup.error instanceof Error ? queueGroup.error.message : 'Queue / Preflight failed.'}</Alert> : null}
        {publishAsIs.isPending ? <Alert severity="info" sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>Moving originals to the Library…</Alert> : null}
        {publishAsIs.isSuccess ? <Alert severity="success" sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>{publishAsIs.data.message || `${publishAsIs.data.assetsPublished} asset(s) published as-is.`}</Alert> : null}
        {publishAsIs.isError ? <Alert severity="error" sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>{publishAsIs.error instanceof Error ? publishAsIs.error.message : 'Publish as-is failed.'}</Alert> : null}
        {migratePath.isPending ? <Alert severity="info" sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>Migrating this path…</Alert> : null}
        {migratePath.isSuccess ? <Alert severity="success" sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>{migratePath.data.assetsMoved} asset(s) moved to the destination Library.</Alert> : null}
        {migratePath.isError ? <Alert severity="error" sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>{migratePath.error instanceof Error ? migratePath.error.message : 'Path migration failed.'}</Alert> : null}
        {bulkAssetAction.isPending ? <Alert severity="info" sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>Applying the selected path action…</Alert> : null}
        {bulkAssetAction.isSuccess ? <Alert severity={bulkAssetAction.data.failures.length ? 'warning' : 'success'} sx={{ py: 0, '& .MuiAlert-message': { py: 0.35, fontSize: '0.75rem' } }}>{bulkAssetAction.data.completed} asset(s) completed.{bulkAssetAction.data.failures.length ? ` ${bulkAssetAction.data.failures.length} failed: ${bulkAssetAction.data.failures.join(' · ')}` : ''}</Alert> : null}
      </Stack>
      <PathSnapshotsDialog
        open={pathSnapshotsOpen}
        group={group}
        title={pathLabelForCollection(group)}
        runningSnapshotPaths={runningSnapshotPaths}
        onClose={() => setPathSnapshotsOpen(false)}
      />
    </>
  ) : null;

  return (
    <>
      {v2ToolbarOnly ? v2PathToolbar : !hidePathHeader && compactPathHeader ? (
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
          sx={{
            cursor: 'pointer',
            '&.Mui-selected, &[aria-selected="true"]': { bgcolor: 'rgba(101, 86, 255, 0.13)' },
            '&.Mui-selected:hover, &[aria-selected="true"]:hover': { bgcolor: 'rgba(101, 86, 255, 0.2)' },
            '&[aria-selected="true"] td': { borderColor: 'rgba(139, 129, 255, 0.28)' },
            '&[aria-selected="true"] td:first-of-type': { borderLeft: '2px solid', borderLeftColor: 'primary.main' },
            '&:focus-visible': { outline: '2px solid', outlineColor: 'primary.main', outlineOffset: -2 },
          }}
        >
          <TableCell sx={{ minWidth: 0 }}>
            <Stack spacing={0.15} sx={{ minWidth: 0 }}>
              <Typography fontWeight={700} variant="body2" noWrap>{group.libraryName} · {pathLabelForCollection(group)}</Typography>
              <Typography color="text.secondary" variant="caption" noWrap title={group.path}>{cleanPathLabel(group.path)} · {countLabel(group.fileCount, 'asset')}</Typography>
            </Stack>
          </TableCell>
          <TableCell align="right" sx={{ width: 190 }} onClick={(event) => event.stopPropagation()}>
            <Stack direction="row" alignItems="center" justifyContent="flex-end" spacing={0.5}>
              {compactPathHeader && mode === 'unprocessed' ? (
                <ScopeConfigureButton
                  targetType="path"
                  scopeKey={group.path}
                  label="Configure"
                  profiles={profiles}
                  audioProfiles={audioProfiles}
                  trackProfiles={trackProfiles}
                  libraries={libraries}
                  categories={assetCategories}
                  compact
                  readOnly={groupAssets.some((asset) => assetHasOpenJob(asset, queueJobs))}
                />
              ) : null}
              {!isReadOnlyGroup && mode !== 'converted' ? (
                <Button
                  size="small"
                  variant="outlined"
                  onClick={startPathAdvisor}
                  disabled={!isConfidenceEnabled || effectiveProfileId <= 0 || groupAssets.every((asset) => asset.missing) || runPathAdvisor.isPending}
                >
                  Advisor
                </Button>
              ) : null}
              {mode === 'unprocessed' && !isReadOnlyGroup ? (
                <Button
                  size="small"
                  variant="contained"
                  onClick={queuePath}
                  disabled={profileAssignments.isLoading || queueGroup.isPending || updateProfileAssignment.isPending || (!effectiveProfileId && !selectedAudioProfileKey && !selectedTrackProfileKey) || !selectedLibraryId || !groupAssets.length || groupAssets.some((asset) => asset.review?.requiresReview || assetHasOpenJob(asset, queueJobs))}
                >
                  Queue
                </Button>
              ) : null}
              {isLibraryGroup && !isReadOnlyGroup ? (
                <Button
                  size="small"
                  variant="contained"
                  onClick={queuePath}
                  disabled={profileAssignments.isLoading || queueGroup.isPending || updateProfileAssignment.isPending || (!effectiveProfileId && !selectedAudioProfileKey && !selectedTrackProfileKey) || !selectedLibraryId || !groupAssets.length || groupAssets.some((asset) => asset.review?.requiresReview || assetHasOpenJob(asset, queueJobs))}
                >
                  Queue
                </Button>
              ) : null}
              <Tooltip title="More path actions">
                <IconButton size="small" aria-label={`More actions for ${group.path}`} onClick={(event) => setPathActionsAnchor(event.currentTarget)}>
                  <MoreVertIcon fontSize="small" />
                </IconButton>
              </Tooltip>
              <ExpandMoreIcon
                aria-hidden
                sx={{ color: 'text.secondary', transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)', transition: 'transform 120ms ease' }}
              />
            </Stack>
            <Menu anchorEl={pathActionsAnchor} open={Boolean(pathActionsAnchor)} onClose={() => setPathActionsAnchor(null)}>
              <MenuItem onClick={() => { setExpanded(true); setPathActionsAnchor(null); }}>
                Path configuration and more actions
              </MenuItem>
            </Menu>
          </TableCell>
        </TableRow>
      ) : !hidePathHeader ? <TableRow
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
            <Chip label={isMixedGroup ? 'Mixed lifecycle' : isPublishedAsIsGroup ? 'Published as-is' : statusLabel(group.status)} color={isMixedGroup ? 'warning' : isPublishedAsIsGroup ? 'success' : statusColor(group.status)} size="small" />
            {isMixedGroup ? <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>{(lifecycleSummary?.provenance ?? []).map((item) => <Chip key={`provenance-${item.value}`} label={`${item.count} ${lifecycleLabel(item.value)}`} size="small" variant="outlined" />)}{(lifecycleSummary?.lifecycle ?? []).map((item) => <Chip key={`lifecycle-${item.value}`} label={`${item.count} ${lifecycleLabel(item.value)}`} size="small" variant="outlined" />)}{(lifecycleSummary?.validation ?? []).map((item) => <Chip key={`validation-${item.value}`} label={`${item.count} ${lifecycleLabel(item.value)}`} size="small" variant="outlined" />)}{(lifecycleSummary?.publication ?? []).map((item) => <Chip key={`publication-${item.value}`} label={`${item.count} ${lifecycleLabel(item.value)}`} size="small" variant="outlined" />)}</Stack> : null}
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
      {!v2ToolbarOnly ? (
      <TableRow>
        <TableCell colSpan={compactPathHeader ? 2 : showConfidenceColumn ? 8 : 7} sx={{ p: 0, borderBottom: (forceExpanded || expanded) ? 1 : 0, borderColor: 'divider', maxWidth: 0 }}>
		  <Collapse in={forceExpanded || expanded} timeout="auto" unmountOnExit>
            <Box sx={{ bgcolor: 'rgba(255,255,255,0.02)', px: { xs: 1.5, md: 2 }, py: 2, width: '100%', maxWidth: '100%', overflow: 'hidden' }}>
              <Stack spacing={2}>
                {collectionManaged ? null : isConvertedGroup ? null : isPublishedAsIsGroup ? (
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
                        onClick={queuePath}
                        disabled={
                          profileAssignments.isLoading || queueGroup.isPending || updateProfileAssignment.isPending ||
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
                {!representativeAsset && groupAssets.length > 0 ? <Alert severity="warning">This path has no physically available asset to evaluate. Run Sync Assets after restoring or removing stale records.</Alert> : null}
                {queueGroup.isSuccess ? (
                  <Alert severity="success">
                    Batch Preflight opened for {queueGroup.data.assets.length} files from this folder.
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
                        disabled={!canPublishPathAsIs}
                        onClick={publishPathAsIs}
                        sx={{ minHeight: 40, whiteSpace: 'nowrap' }}
                      >
                        {publishAsIs.isPending ? 'Publishing...' : 'Publish as-is'}
                      </Button>
                    </Stack>
                  </Stack>
                ) : null}
                {publishAsIs.isSuccess ? <Alert severity="success">{publishAsIs.data.message}</Alert> : null}
                {publishAsIs.isError ? <Alert severity="warning">{publishAsIs.error instanceof Error ? publishAsIs.error.message : 'Direct publication failed.'}</Alert> : null}
                {isLibraryGroup || isConvertedGroup ? (
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
                      <Button
                        color="error"
                        variant="outlined"
                        size="small"
                        startIcon={<DeleteForeverIcon />}
                        disabled={!selectedAssetPaths.length || bulkAssetAction.isPending}
                        onClick={() => runBulkAssetAction()}
                        sx={{ minHeight: 40, whiteSpace: 'nowrap' }}
                      >
                        {bulkAssetAction.isPending
                          ? 'Processing...'
                          : isPublishedAsIsGroup
                              ? 'Return selected'
                              : 'Delete selected'}
                      </Button>
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
							srtFallbackEnabled={subtitleSRTFallbackEnabled(settings)}
                          pathTrackProfile={trackProfiles.find((profile) => profile.key === selectedTrackProfileKey)}
                          assetCategories={assetCategories}
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
						  bulkSelectionDisabled={asset.missing || bulkAssetAction.isPending || !bulkSelectableAssets.some((candidate) => candidate.path === asset.path)}
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
      ) : null}
      <LibraryPathRenameDialog
        open={renamePathOpen}
        libraryId={group.libraryId}
        path={group.path}
        onClose={() => setRenamePathOpen(false)}
        onApplied={async () => {
          await Promise.all([
            queryClient.invalidateQueries({ queryKey: ['assets'] }),
            queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
            queryClient.invalidateQueries({ queryKey: ['profileAssignments'] }),
            queryClient.invalidateQueries({ queryKey: ['settings'] }),
          ]);
        }}
      />
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

type AssetInfoActionKey =
  | 'rename'
  | 'move'
  | 'queue'
  | 'reconvert'
  | 'lab'
  | 'refreshAnalysis'
  | 'manageTracks'
  | 'manageSubtitles'
  | 'manageAttachments'
  | 'cancelWork'
  | 'reviewValidation'
  | 'retryValidation'
  | 'publish'
  | 'retryPublication'
  | 'archive'
  | 'restore'
  | 'deleteArchivedOriginal'
  | 'viewReports'
  | 'viewHistory';

function assetInfoCapability(asset: Asset, key: AssetInfoActionKey) {
  const capability = asset.lifecycle?.actions?.[key];
  return capability ?? { allowed: true };
}

function assetActionReasonMessage(capability: { reason?: string }, fallback = '') {
  const messages: Record<string, string> = {
    unsupported: 'This action is unavailable for this asset.',
    identity_unresolved: 'Asset identity is unavailable for this action.',
    active_work: 'An active operation must finish before this action is available.',
    missing: 'The asset file is missing.',
    unsafe_root: 'This asset is outside a supported Library location.',
    unknown_provenance: 'The asset provenance is unknown.',
    publication_conflict: 'Resolve the publication conflict before continuing.',
    needs_review: 'Review this asset before continuing.',
    not_raw: 'This action is available for RAW source assets.',
    not_library: 'This action is available for Library assets.',
    original_unavailable: 'No retained original is available.',
    already_archived: 'The original is already archived.',
    original_archive_in_progress: 'Original archival is still in progress. Try Rename again once it finishes.',
  };
  return capability.reason ? messages[capability.reason] ?? fallback : fallback;
}

function AssetMetadataSidecars({ values, loading }: { values: AssetMetadataSidecar[]; loading: boolean }) {
  return (
    <Stack spacing={0.75} data-testid="asset-metadata-sidecars">
      <Typography variant="h5">Metadata sidecars</Typography>
      {loading ? <LinearProgress /> : values.length ? values.map((sidecar) => (
        <Typography key={sidecar.path} data-testid="asset-nfo-sidecar" variant="body2">
          {sidecar.fileName} · {formatBytes(sidecar.sizeBytes)} · {formatDate(sidecar.modifiedAt)} · Read only
        </Typography>
      )) : <Typography variant="body2" color="text.secondary">No metadata sidecars found.</Typography>}
    </Stack>
  );
}

function lifecyclePresentationValue(value: string | undefined, fallback = 'unavailable') {
  if (!value) return fallback;
  return value.replaceAll('_', ' ').replace(/\b\w/g, (character) => character.toUpperCase());
}

function validationOutcomeLabel(outcome?: QueueJob['validationOutcome']) {
  switch (outcome) {
    case 'PASS': return 'Passed';
    case 'PASS_WITH_WARNINGS': return 'Passed with warnings';
    case 'REVIEW_REQUIRED': return 'Review required';
    case 'FAIL': return 'Failed';
    default: return outcome ? lifecyclePresentationValue(outcome) : 'Outcome unavailable';
  }
}

function validationEvidencePresentation(job: QueueJob | undefined, lifecycleStatus?: string) {
  const score = job && job.validationScore > 0 ? job.validationScore : undefined;
  if (job?.validationRunId) {
    return {
      kind: 'canonical' as const,
      runId: job.validationRunId,
      outcomeLabel: validationOutcomeLabel(job.validationOutcome),
      overridePresent: job.validationOverridePresent,
      hasActionableWarnings: job.validationHasActionableWarnings,
      score,
    };
  }

  const status = job?.validationStatus || lifecycleStatus;
  return status
    ? { kind: 'legacy' as const, statusLabel: lifecyclePresentationValue(status), score }
    : { kind: 'unavailable' as const, score };
}

export function AssetInfoOverview({ asset, libraries, status }: { asset: Asset; libraries: Library[]; status: string }) {
  const publication = asset.lifecycle?.currentPublication;
  const destination = libraries.find((library) => library.id === asset.libraryId);
  const provenance = asset.lifecycle?.provenance ?? asset.status;
  const source = provenance === 'converted'
    ? 'Converted by MVForge'
    : provenance === 'published_as_is'
      ? 'Published as-is by MVForge'
      : provenance === 'imported' || provenance === 'accepted'
        ? 'Imported'
        : 'Original asset';

  return (
    <Stack spacing={1.5} data-testid="asset-info-overview">
      <Typography variant="h3">Overview</Typography>
      <Grid container spacing={1.5}>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <Typography variant="caption" color="text.secondary">Status</Typography>
          <Typography>{status}</Typography>
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <Typography variant="caption" color="text.secondary">Library</Typography>
          <Typography>{destination?.name || asset.libraryName || 'RAW source'}</Typography>
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <Typography variant="caption" color="text.secondary">Published</Typography>
          <Typography>{publication?.publishedAt ? formatDate(publication.publishedAt) : '—'}</Typography>
        </Grid>
        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
          <Typography variant="caption" color="text.secondary">Source</Typography>
          <Typography>{source}</Typography>
        </Grid>
      </Grid>
    </Stack>
  );
}

export function ArchivedOriginalDetails({
  original,
  onRecover,
  onDelete,
  recovering = false,
  deleting = false,
  canMutate = false,
}: {
  original: AssetArchivedOriginalProjection;
  onRecover?: (event: MouseEvent<HTMLButtonElement>, original: AssetArchivedOriginalProjection) => void;
  onDelete?: (event: MouseEvent<HTMLButtonElement>, original: AssetArchivedOriginalProjection) => void;
  recovering?: boolean;
  deleting?: boolean;
  canMutate?: boolean;
}) {
  return <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
    <Stack spacing={1}>
      <Typography variant="body2" sx={{ wordBreak: 'break-all' }}>{original.path}</Typography>
      <Typography variant="body2">Size · {formatBytes(original.sizeBytes)}</Typography>
      <Typography variant="body2">Archive time · {original.archivedAt ? formatDate(original.archivedAt) : 'Unknown'}</Typography>
      <Typography variant="body2">Provenance · Queue job #{original.queueJobId} · Source AssetRecord #{original.provenance.sourceAssetRecordId ?? 'unknown'}</Typography>
      <Typography variant="body2">Relationship · {lifecyclePresentationValue(original.relationship)} · Availability · {lifecyclePresentationValue(original.availability)}</Typography>
      <Typography variant="body2">Retention · {lifecyclePresentationValue(original.retentionState)}{original.retentionDeadline ? ` · until ${formatDate(original.retentionDeadline)}` : ' · no recorded deadline'}</Typography>
      <Typography variant="caption" color="text.secondary">Restore · {original.restore?.allowed ? 'Available' : assetActionReasonMessage(original.restore ?? {}, 'Unavailable')}</Typography>
      <Typography variant="caption" color="text.secondary">Delete · {original.deleteArchivedOriginal?.allowed ? 'Available' : assetActionReasonMessage(original.deleteArchivedOriginal ?? {}, 'Unavailable')}</Typography>
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
        <Button variant="outlined" onClick={(event) => onRecover?.(event, original)} disabled={!canMutate || !onRecover || !original.restore?.allowed || recovering}>
          Restore original
        </Button>
        <Button color="error" variant="outlined" onClick={(event) => onDelete?.(event, original)} disabled={!canMutate || !onDelete || !original.deleteArchivedOriginal?.allowed || deleting}>
          Delete archived original
        </Button>
      </Stack>
    </Stack>
  </Box>;
}

function associatedJobForCurrentPublication(asset: Asset, jobs: QueueJob[]) {
  // Phase A is authoritative whenever the projection is present. Keep the
  // path-based lookup only for pre-projection compatibility responses.
  if (!asset.lifecycle) return associatedJobForAsset(asset, jobs);
  const current = asset.lifecycle?.currentPublication;
  if (current) {
    if (current.authority !== 'queue_job' || !current.id) return undefined;
    return jobs.find((job) => job.id === current.id);
  }
  return undefined;
}

function AssetRow({
  asset,
	autoOpenAssetInfo = false,
	compactUnprocessed = false,
  compactLibrary = false,
  compactRaw = false,
  selected = false,
  onSelectAsset,
  onRegisterWorkspaceActions,
  libraries,
  profiles,
  audioProfiles,
	trackProfiles,
	srtFallbackEnabled,
  pathTrackProfile,
  assetCategories,
  groupCategory,
  confidenceEnabled,
  groupProfileId,
  effectiveConfiguration,
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
	autoOpenAssetInfo?: boolean;
  compactUnprocessed?: boolean;
  compactLibrary?: boolean;
  compactRaw?: boolean;
  selected?: boolean;
  onSelectAsset?: () => void;
  onRegisterWorkspaceActions?: (path: string, actions: AssetWorkspaceActions | null) => void;
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
	trackProfiles: TrackProfile[];
	srtFallbackEnabled?: boolean;
  pathTrackProfile?: TrackProfile;
  assetCategories: string[];
  groupCategory: string;
  confidenceEnabled: boolean;
  groupProfileId: number;
  effectiveConfiguration?: EffectiveAssetConfiguration;
  groupAudioProfileKey: string;
  groupLibraryId: number;
  hasOpenJob: boolean;
  queueJobs: QueueJob[];
  snapshotRunning: boolean;
  mode: 'unprocessed' | 'library' | 'converted';
  bulkSelected: boolean;
  bulkSelectionEnabled: boolean;
  bulkSelectionDisabled: boolean;
  onBulkSelectionChange: (path: string, selected: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const profileAssignments = useQuery({ queryKey: ['profileAssignments'], queryFn: api.profileAssignments });
  const assetVideoProfiles = profiles.filter((profile) => profile.scope === 'asset');
  const assetAudioProfiles = audioProfiles.filter((profile) => profile.scope === 'asset');
  const assetTrackProfiles = trackProfiles.filter((profile) => profile.scope === 'asset');
  const [selectedProfileId, setSelectedProfileId] = useState<number>(0);
  const [selectedAudioProfileKey, setSelectedAudioProfileKey] = useState<string>('__inherit__');
  const [selectedTrackProfileKey, setSelectedTrackProfileKey] = useState<string>('__inherit__');
  const [assetDestinationMode, setAssetDestinationMode] = useState<'inherit' | 'value' | 'disabled'>('inherit');
  const [assetDestinationLibraryId, setAssetDestinationLibraryId] = useState(0);
  const effectiveProfileId = effectiveConfiguration ? effectiveConfiguration.video.videoProfileId ?? 0 :
    selectedProfileId === 0
      ? groupProfileId
      : selectedProfileId > 0
        ? selectedProfileId
        : 0;
  const effectiveAudioProfileKey = effectiveConfiguration ? effectiveConfiguration.audio.profileKey ?? '' : selectedAudioProfileKey === '__inherit__' ? groupAudioProfileKey : selectedAudioProfileKey;
  const effectiveTrackProfileKey = effectiveConfiguration ? effectiveConfiguration.tracks.profileKey ?? '' : selectedTrackProfileKey === '__inherit__' ? pathTrackProfile?.key ?? '' : selectedTrackProfileKey;
  const selectedTrackProfile = trackProfiles.find((profile) => profile.key === effectiveTrackProfileKey);
  const [selectedLibraryId, setSelectedLibraryId] = useState<number>(groupLibraryId);
  useEffect(() => {
    setSelectedLibraryId(groupLibraryId);
  }, [asset.path, groupLibraryId]);
  const [showSnapshotDialog, setShowSnapshotDialog] = useState(false);
  type AssetInfoSection = 'overview' | 'analysis' | 'tracks' | 'execution' | 'publication' | 'validation' | 'original' | 'evidence' | 'history' | 'integration' | 'advisor' | 'conversion' | 'test-encode';
  const [snapshotTab, setSnapshotTab] = useState<AssetInfoSection>('overview');
  const hidePreConversionActions = hidePreConversionActionsForAsset(asset);
  const [selectedEvidenceJobId, setSelectedEvidenceJobId] = useState<number | null>(null);
  const assetScopeConfigurations = useQuery({
    queryKey: ['assetScopeConfigurations'],
    queryFn: api.assetScopeConfigurations,
    enabled: showSnapshotDialog && mode === 'unprocessed' && snapshotTab === 'overview',
  });
  const [renameFileName, setRenameFileName] = useState(asset.fileName);
  const [snapshotOperation, setSnapshotOperation] = useState<SnapshotOperation | null>(null);
  const [editingSubtitle, setEditingSubtitle] = useState<ExternalSubtitle | null>(null);
  const [renamingSubtitle, setRenamingSubtitle] = useState<ExternalSubtitle | null>(null);
  const [subtitleFileName, setSubtitleFileName] = useState('');
  const [subtitleContent, setSubtitleContent] = useState('');
  const [subtitleGenerations, setSubtitleGenerations] = useState<Record<string, SubtitleGenerationState>>({});
  const [subtitleQualityReports, setSubtitleQualityReports] = useState<Record<string, SubtitleOCRQualityReport>>({});
  const [subtitleQualityMessages, setSubtitleQualityMessages] = useState<Record<string, string>>({});
  const [subtitleRestyleMessages, setSubtitleRestyleMessages] = useState<Record<string, string>>({});
  const [subtitleRestyleOperations, setSubtitleRestyleOperations] = useState<Record<string, SubtitleRestyleOperationState>>({});
  const [subtitleCompositionOperation, setSubtitleCompositionOperation] = useState<SubtitleExtractionOperation | null>(null);
  const [subtitleCompositionError, setSubtitleCompositionError] = useState('');
  const [translationSource, setTranslationSource] = useState<SubtitleTranslationSourceSelector | null>(null);
  const [translationDialogOpen, setTranslationDialogOpen] = useState(false);
  const [translationReviewTarget, setTranslationReviewTarget] = useState<CanonicalOCRReviewTarget | null>(null);
  const [translationReviewDialogOpen, setTranslationReviewDialogOpen] = useState(false);
  const subtitleOperationPollers = useRef<Set<string>>(new Set());
  const [showPreviewDialog, setShowPreviewDialog] = useState(false);
  const [assignProfileOpen, setAssignProfileOpen] = useState(false);
  const [assetMenuAnchor, setAssetMenuAnchor] = useState<HTMLElement | null>(null);
  const [showAdvisorDialog, setShowAdvisorDialog] = useState(false);
  const [showTestEncodeDialog, setShowTestEncodeDialog] = useState(false);
  const assetInfoOpenedRef = useRef(false);
  const [assetEditQueueMessage, setAssetEditQueueMessage] = useState('');
  const [previewMode, setPreviewMode] = useState<'compatible' | 'original'>('compatible');
  const assetReview = asset.review ?? { requiresReview: false, reason: '', source: '', tags: [], updatedAt: '' };
  const reconciliationJobId = reconciliationJobIdFromReview(assetReview);
  const assetMetadata = asset.metadata ?? { categories: [], tags: [], updatedAt: '' };
  const [reviewReason, setReviewReason] = useState(assetReview.reason || '');
  const [reviewTags, setReviewTags] = useState<string[]>(safeArray(assetReview.tags));
  const initialAssetEdit = assetInfoEditDocument(assetMetadata, normalizeAssetConversionOverride(asset.conversion));
  const [persistedAssetEdit, setPersistedAssetEdit] = useState<AssetInfoEditDocument>(initialAssetEdit);
  const [assetEditDraft, setAssetEditDraft] = useState<AssetInfoEditDocument>(initialAssetEdit);
  const assetEditDraftRef = useRef(assetEditDraft);
  const assetEditPathRef = useRef(asset.path);
  const assetEditAuthoritativeRef = useRef(initialAssetEdit);
  const pendingAssetEditRefreshRef = useRef(false);
  const assetEditDirty = !assetInfoDocumentsEqual(persistedAssetEdit, assetEditDraft);

  useEffect(() => {
    if (autoOpenAssetInfo) {
      assetInfoOpenedRef.current = true;
      setSnapshotTab('overview');
      setShowSnapshotDialog(true);
    }
  }, [autoOpenAssetInfo]);
  const conversionDraft = assetEditDraft.conversion;
  const pendingTracksPatch = assetTracksPatch(persistedAssetEdit.conversion, conversionDraft);
  const tracksDirty = Object.keys(pendingTracksPatch).length > 0;
  // Asset Info edits persisted metadata. Effective scope configuration is
  // projected separately for the read-only Selected Asset category label.
  const metadataCategory = firstCategory(assetEditDraft.metadata.categories) || groupCategory;
  const authoritativeAssetEdit = assetInfoEditDocument(assetMetadata, normalizeAssetConversionOverride(asset.conversion));

  useEffect(() => {
    assetEditDraftRef.current = assetEditDraft;
  }, [assetEditDraft]);

  function setConversionDraft(next: AssetConversionOverrideState | ((current: AssetConversionOverrideState) => AssetConversionOverrideState)) {
    setAssetEditDraft((current) => {
      const conversion = typeof next === 'function' ? next(current.conversion) : next;
      return {
        ...current,
        conversion: normalizeAssetConversionOverride(conversion),
      };
    });
  }

  function hydrateAssetEditDefaults(patch: Partial<AssetConversionOverrideState>) {
    const currentDraft = assetEditDraftRef.current;
    const hydratedDraft = assetInfoEditDocument(
      currentDraft.metadata,
      normalizeAssetConversionOverride({ ...currentDraft.conversion, ...patch }),
    );
    const reconciled = assetInfoHydrationReconciliation(persistedAssetEdit, currentDraft, hydratedDraft);
    setPersistedAssetEdit(reconciled.baseline);
    setAssetEditDraft(reconciled.draft);
    assetEditDraftRef.current = reconciled.draft;
  }

  function setCategory(nextCategory: string) {
    setAssetEditDraft((current) => ({
      ...current,
      metadata: {
        ...current.metadata,
        categories: nextCategory ? [nextCategory] : [],
      },
    }));
  }
  const storedSnapshot = useQuery({
    queryKey: ['assetSnapshot', asset.path],
    queryFn: () => api.latestSnapshot(asset.path),
    enabled: showSnapshotDialog && ['analysis', 'tracks', 'advisor', 'conversion', 'test-encode', 'history'].includes(snapshotTab) && !asset.missing,
    staleTime: 15_000,
  });
  const legacyAnalysisSettings = useQuery({
    queryKey: ['settings'],
    queryFn: api.settings,
    enabled: showSnapshotDialog && snapshotTab === 'history',
  });
  const legacyAnalysisRecords = (() => {
    const raw = legacyAnalysisSettings.data?.find((setting) => setting.key === 'analysisRecords')?.value.records;
    if (!Array.isArray(raw)) return [];
    const snapshot = storedSnapshot.data?.snapshot;
    const identity = storedSnapshot.data?.analysisEvidence?.sourceIdentity;
    if (!snapshot || !identity?.matchesCurrentSourceRevision || !identity.snapshotUpdatedAt) return [];
    return raw.filter((value): value is Record<string, unknown> => {
      if (!value || typeof value !== 'object') return false;
      const record = value as Record<string, unknown>;
      const scan = record.scan;
      if (!scan || typeof scan !== 'object') return false;
      const legacyScan = scan as Partial<ScanResult>;
      const cache = legacyScan.rawProbe?.snapshotCache;
      const fingerprint = cache && typeof cache === 'object' ? (cache as Record<string, unknown>).fingerprint : undefined;
      const legacyIdentity = fingerprint && typeof fingerprint === 'object' ? fingerprint as Record<string, unknown> : undefined;
      return record.assetPath === asset.path && legacyScan.path === snapshot.path &&
        legacyScan.id === identity.snapshotId && legacyScan.sizeBytes === identity.sizeBytes &&
        legacyScan.updatedAt === identity.snapshotUpdatedAt &&
        legacyIdentity?.sizeBytes === identity.sizeBytes && legacyIdentity?.mtimeNs === identity.modifiedAtUnixNano;
    });
  })();
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
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['assetSnapshot', asset.path] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
        queryClient.invalidateQueries({ queryKey: ['snapshotOperations'] }),
      ]);
    },
  });
  const snapshotData = storedSnapshot.data?.snapshot ?? snapshot.data ?? null;
  const profileSuggestion = useQuery({
    queryKey: ['profileSuggestion', asset.path, snapshotData?.updatedAt ?? ''],
    queryFn: () => api.suggestProfile(asset.path),
    staleTime: 15_000,
    enabled:
      showSnapshotDialog &&
      snapshotTab === 'advisor' &&
      Boolean(snapshotData) &&
      !hidePreConversionActions &&
      !asset.missing &&
      asset.status !== 'converted' &&
      asset.status !== 'accepted',
  });
  const storedSnapshotExists = snapshotData !== null;
  const refreshRequired = storedSnapshot.data?.snapshot != null && storedSnapshot.data.requiresAnalysis;
  const operationPending = snapshot.isPending;
  const externalSubtitles = useQuery({
    queryKey: ['externalSubtitles', asset.path],
    queryFn: () => api.externalAssetSubtitles(asset.path),
    enabled: showSnapshotDialog && snapshotTab === 'tracks' && !asset.missing,
  });
  const metadataSidecars = useQuery({
    queryKey: ['assetMetadataSidecars', asset.path],
    queryFn: () => api.assetMetadataSidecars(asset.path),
    enabled: showSnapshotDialog && snapshotTab === 'tracks' && !asset.missing,
  });
  const subtitleEligibility = useQuery({
    queryKey: ['subtitleEligibility', asset.path],
    queryFn: () => api.subtitleEligibility(asset.path),
    enabled: showSnapshotDialog && snapshotTab === 'tracks' && !asset.missing,
  });
  const evidenceArtifacts = useQuery({
    queryKey: ['assetEvidenceArtifacts', selectedEvidenceJobId],
    queryFn: () => api.jobArtifacts(selectedEvidenceJobId as number),
    enabled: showSnapshotDialog && snapshotTab === 'evidence' && selectedEvidenceJobId !== null,
  });
  const evidencePlans = useQuery({
    queryKey: ['assetEvidencePlans', selectedEvidenceJobId],
    queryFn: () => api.executionPlans(selectedEvidenceJobId as number),
    enabled: showSnapshotDialog && snapshotTab === 'evidence' && selectedEvidenceJobId !== null,
  });
  const advisor = useQuery({
    queryKey: ['advisor', 'asset-row', asset.path, effectiveProfileId],
    queryFn: () => api.evaluateAdvisor({ mediaPath: asset.path, profileId: effectiveProfileId }),
    enabled: showAdvisorDialog && mode !== 'converted' && asset.status !== 'converted' && asset.status !== 'published_as_is' && asset.status !== 'accepted' && confidenceEnabled && effectiveProfileId > 0,
  });
  async function refreshSubtitleEvidence() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['externalSubtitles', asset.path] }),
      queryClient.invalidateQueries({ queryKey: ['subtitleEligibility', asset.path] }),
    ]);
  }
  const saveAssetEdit = useMutation({
    mutationFn: (revision: AssetInfoEditDocument) => api.updateAssetEdit({
      path: asset.path,
      metadata: revision.metadata,
      conversion: cleanConversionOverride(revision.conversion),
    }),
    onSuccess: (response, revision) => {
      const normalizedResponse = {
        ...response,
        conversion: normalizeAssetConversionOverride(response.conversion),
      };
      const reconciled = assetInfoSaveReconciliation(assetEditDraftRef.current, revision, normalizedResponse);
      setPersistedAssetEdit(reconciled.baseline);
      setAssetEditDraft(reconciled.draft);
      queryClient.setQueryData<AssetInventory | undefined>(['assets'], (current) => current
        ? patchAssetInventoryAsset(current, response.path, response)
        : current);
    },
    onError: () => {
      const retained = assetInfoSaveFailureReconciliation(persistedAssetEdit, assetEditDraftRef.current);
      setPersistedAssetEdit(retained.baseline);
      setAssetEditDraft(retained.draft);
    },
  });
  const saveAssetTracks = useMutation({
    mutationFn: (patch: AssetTracksUpdateInput) => api.updateAssetTracks(patch),
    onSuccess: async (response) => {
      const savedConversion = normalizeAssetConversionOverride(response.conversion);
      const reconciled = assetInfoTracksSaveReconciliation(persistedAssetEdit, assetEditDraftRef.current, savedConversion);
      setPersistedAssetEdit(reconciled.baseline);
      setAssetEditDraft(reconciled.draft);
      assetEditDraftRef.current = reconciled.draft;
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
        queryClient.invalidateQueries({ queryKey: ['effectiveAssetConfiguration'] }),
      ]);
    },
  });
  const resetAssetEdit = saveAssetEdit.reset;
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
      }
      return api.createQueueJob(input);
    },
    onSuccess: async () => {
	  await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
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
  const renameAsset = useMutation({
    mutationFn: api.renameAsset,
    onSuccess: async () => {
      closeSnapshotDialog();
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const updateProfileAssignment = useMutation({
    mutationFn: api.updateProfileAssignment,
    onSuccess: async () => {
      setAssetEditQueueMessage('');
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['profileAssignments'] }),
        queryClient.invalidateQueries({ queryKey: ['effectiveAssetConfiguration'] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
      ]);
    },
    onError: (error) => setAssetEditQueueMessage(error instanceof Error ? error.message : 'Profile assignment could not be saved.'),
  });
  const updateAssetDestination = useMutation({
    mutationFn: async ({ selection, libraryId }: { selection: 'inherit' | 'value' | 'disabled'; libraryId: number }) => {
      const configurations = await api.assetScopeConfigurations();
      const current = configurations.find(
        (item) => item.scopeType === 'asset' && normalizePath(item.scopeKey) === normalizePath(asset.path),
      );
      const values = scopeConfigurationEditorValues(
        'asset',
        asset.path,
        profileAssignments.data ?? [],
        configurations,
      );
      return api.updateAssetScopeConfiguration(mergedScopeConfigurationInput(
        'asset',
        asset.path,
        current,
        new Set<ScopeConfigurationField>(['destination']),
        {
          ...values,
          destinationSelection: selection,
          destinationLibraryId: selection === 'value' ? libraryId : 0,
        },
      ));
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['assetScopeConfigurations'] }),
        queryClient.invalidateQueries({ queryKey: ['effectiveAssetConfiguration'] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
      ]);
    },
  });
  const recoverAsset = useMutation({
    mutationFn: api.recoverAsset,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const deleteArchiveAsset = useMutation({
    mutationFn: api.deleteArchiveAsset,
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
      await refreshSubtitleEvidence();
    },
  });
  const renameExternalSubtitle = useMutation({
    mutationFn: api.renameExternalAssetSubtitle,
    onSuccess: async () => {
      setRenamingSubtitle(null);
      setSubtitleFileName('');
      await refreshSubtitleEvidence();
    },
  });
  const deleteExternalSubtitle = useMutation({
    mutationFn: api.deleteExternalAssetSubtitle,
    onSuccess: async () => {
      await refreshSubtitleEvidence();
    },
  });
  const isBlockedByReview = assetReview.requiresReview;
  const assetProvenance = asset.lifecycle?.provenance;
  const isConverted = assetProvenance === 'converted' || (!assetProvenance && asset.status === 'converted');
  const isPublishedAsIs = assetProvenance === 'published_as_is' || (!assetProvenance && asset.status === 'published_as_is');
  const isAccepted = assetProvenance === 'imported' || (!assetProvenance && asset.status === 'accepted');
  const isLibraryReplacement = asset.status === 'unverified' || asset.status === 'library' || asset.status === 'published_as_is';
  const compactAssetsRow = compactLibrary || compactRaw;
  const canGenerateTestEncode = testEncodeEligibleAsset(asset, mode);
  const labCapability = assetInfoCapability(asset, 'lab');
  const renameCapability = assetInfoCapability(asset, 'rename');
  const rowColumnCount = compactAssetsRow ? compactRaw ? (bulkSelectionEnabled ? 11 : 10) : 12 : compactUnprocessed ? 5 : isConverted
    ? bulkSelectionEnabled ? 8 : 7
    : isAccepted ? 5 : 10;
  const associatedJob = associatedJobForCurrentPublication(asset, queueJobs);
  const currentPublication = asset.lifecycle?.currentPublication;
  const validationEvidence = validationEvidencePresentation(associatedJob, asset.lifecycle?.validation);
  const validationIntegrationState = validationEvidence.kind === 'canonical'
    ? `${validationEvidence.outcomeLabel} · run ${validationEvidence.runId}`
    : validationEvidence.kind === 'legacy'
      ? `Legacy/unbound · ${validationEvidence.statusLabel}`
      : 'Unavailable';
  const publicationIntegrationState = currentPublication
    ? lifecyclePresentationValue(currentPublication.state, 'Unavailable')
    : 'No current publication';
  const historyJobIds = new Set([
    ...(asset.lifecycle?.evidenceSummary?.queueJobIds ?? []),
    ...(associatedJob ? [associatedJob.id] : []),
  ]);
  const historyJobs = queueJobs.filter((job) => historyJobIds.has(job.id)).sort((left, right) => right.id - left.id);
  const jobInformationTabs: AssetInfoSection[] = ['execution', 'validation', 'original', 'evidence', 'history', 'publication', 'integration'];
  const isJobInformationTab = jobInformationTabs.includes(snapshotTab);
  const assetInfoMainTab = isJobInformationTab ? 'job-information' : snapshotTab === 'tracks' ? 'tracks' : 'overview';
  const rowLocked = hasOpenJob || createJob.isPending || (isConverted && !isLibraryReplacement) || isPublishedAsIs || isAccepted;
  const renameLocked = hasOpenJob || createJob.isPending || isPublishedAsIs || isAccepted;
  const pipelineState = assetPipelineState(asset, associatedJob, createJob.isPending);
  const pipelineTone = pipelineState.label.toLowerCase().includes('queue') || pipelineState.color === 'warning'
    ? { background: '#41320d', color: '#ffc43d' }
    : pipelineState.color === 'success'
      ? { background: '#063b35', color: '#00d99b' }
      : pipelineState.color === 'primary'
        ? { background: '#142d67', color: '#55a0ff' }
        : pipelineState.color === 'error'
          ? { background: '#4b1826', color: '#ff92a3' }
          : { background: '#0c2035', color: '#9cb0c8' };
  const cachedAdvisor = confidenceEnabled
    ? advisor.data ?? queryClient.getQueryData<AdvisorResponse>(['advisor', 'asset-row', asset.path, effectiveProfileId])
    : undefined;
  const technical = asset.technical;
  const technicalResolution = technical?.width && technical.height
    ? `${technical.width} × ${technical.height}`
    : 'Unavailable';
  const technicalBitrate = technical ? formatMediaBitrate(technical.bitrate) : 'Unavailable';
  const technicalRange = technical ? technical.hdr ? 'HDR' : 'SDR' : 'Unavailable';
  const rawSourceCodec = [asset.extension.replace(/^\./, '').toUpperCase(), technical?.videoCodec]
    .filter(Boolean)
    .join(' · ') || 'Unavailable';
  const openWorkspacePreview = useCallback(() => {
    setPreviewMode('compatible');
    setShowPreviewDialog(true);
  }, []);
  const openWorkspaceAssignProfile = useCallback(() => setAssignProfileOpen(true), []);
  const openWorkspaceTestEncode = useCallback(() => setShowTestEncodeDialog(true), []);
  const openWorkspaceMore = useCallback((anchor: HTMLElement) => setAssetMenuAnchor(anchor), []);
  const canAssignProfile = !hidePreConversionActions && (!isConverted || isLibraryReplacement) && !rowLocked && !updateProfileAssignment.isPending;
  const testConfigurationReason = compactAssetsRow
    ? testEncodeConfigurationReason(effectiveConfiguration?.video)
    : effectiveProfileId > 0 ? '' : testEncodeConfigurationReason(undefined);
  const canTestEncode = !testConfigurationReason && !updateProfileAssignment.isPending && !hidePreConversionActions && canGenerateTestEncode && labCapability.allowed && !showTestEncodeDialog;
  const assignProfileUnavailableReason = canAssignProfile
    ? ''
    : hidePreConversionActions || (isConverted && !isLibraryReplacement)
      ? 'Profile assignment is unavailable for this asset in its current Library lifecycle.'
      : rowLocked
        ? 'Profile assignment is unavailable while this asset has active work or a protected lifecycle state.'
        : 'Profile assignment is already in progress.';
  const testEncodeUnavailableReason = canTestEncode
    ? ''
    : hidePreConversionActions || !canGenerateTestEncode
      ? 'Test Encode is unavailable for this asset in its current lifecycle.'
      : !labCapability.allowed
        ? `Test Encode unavailable: ${assetActionReasonMessage(labCapability, 'The current Lab capability does not allow Test Encode for this asset.')}`
        : testConfigurationReason
          ? testConfigurationReason
          : updateProfileAssignment.isPending
            ? 'Wait for profile assignment to finish saving.'
        : showTestEncodeDialog
          ? 'Test Encode is already open for this asset.'
          : 'Test Encode is temporarily unavailable for this asset.';
  const workspaceActions = useMemo<AssetWorkspaceActions>(() => ({
    playPreview: openWorkspacePreview,
    assignProfile: openWorkspaceAssignProfile,
    testEncode: openWorkspaceTestEncode,
    openMore: openWorkspaceMore,
    canAssignProfile,
    canTestEncode,
    assignProfileUnavailableReason,
    testEncodeUnavailableReason,
  }), [assignProfileUnavailableReason, canAssignProfile, canTestEncode, openWorkspaceAssignProfile, openWorkspaceMore, openWorkspacePreview, openWorkspaceTestEncode, testEncodeUnavailableReason]);
  useEffect(() => {
    if (!onRegisterWorkspaceActions) return;
    if (compactAssetsRow && selected) onRegisterWorkspaceActions(asset.path, workspaceActions);
    else onRegisterWorkspaceActions(asset.path, null);
    return () => onRegisterWorkspaceActions(asset.path, null);
  }, [asset.path, compactAssetsRow, onRegisterWorkspaceActions, selected, workspaceActions]);
  const isAudioOnly = effectiveConfiguration ? effectiveConfiguration.video.selection === 'audio_only' || effectiveConfiguration.video.selection === 'disabled' :
    selectedProfileId === VIDEO_PROFILE_AUDIO_ONLY;

  useEffect(() => {
    if (!profileAssignments.data) return;
    const assignments = profileAssignments.data.filter((assignment) => assignment.targetType === 'asset' && normalizePath(assignment.targetPath) === normalizePath(asset.path));
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
        : 0,
    );
    setSelectedAudioProfileKey(audio ? audio.selection === 'disabled' ? '' : audio.profileKey || '__inherit__' : '__inherit__');
    setSelectedTrackProfileKey(tracks ? tracks.selection === 'disabled' ? '' : tracks.profileKey || '__inherit__' : '__inherit__');
  }, [profileAssignments.data, asset.path]);

  useEffect(() => {
    if (!showSnapshotDialog || !assetScopeConfigurations.data) return;
    const values = scopeConfigurationEditorValues(
      'asset',
      asset.path,
      profileAssignments.data ?? [],
      assetScopeConfigurations.data,
    );
    setAssetDestinationMode(values.destinationSelection);
    setAssetDestinationLibraryId(values.destinationSelection === 'value' ? values.destinationLibraryId : 0);
    setSelectedLibraryId(
      values.destinationSelection === 'value'
        ? values.destinationLibraryId
        : values.destinationSelection === 'inherit'
          ? groupLibraryId
          : 0,
    );
  }, [showSnapshotDialog, asset.path, assetScopeConfigurations.data, groupLibraryId, profileAssignments.data]);

  function selectAssetVideoProfile(id: number) {


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

    updateProfileAssignment.mutate({ targetType: 'asset', targetPath: asset.path, mediaType: 'audio', selection: key === '__inherit__' ? 'inherit' : key ? 'profile' : 'disabled', videoProfileId: 0, profileKey: key === '__inherit__' ? '' : key });
  }

  function selectAssetTrackProfile(key: string) {

    updateProfileAssignment.mutate({ targetType: 'asset', targetPath: asset.path, mediaType: 'tracks', selection: key === '__inherit__' ? 'inherit' : key ? 'profile' : 'disabled', videoProfileId: 0, profileKey: key === '__inherit__' ? '' : key });
  }

  function selectAssetDestinationMode(mode: 'inherit' | 'value' | 'disabled') {
    setAssetDestinationMode(mode);
    if (mode === 'value') return;
    setAssetDestinationLibraryId(0);
    setSelectedLibraryId(mode === 'inherit' ? groupLibraryId : 0);
    updateAssetDestination.mutate({ selection: mode, libraryId: 0 });
  }

  function selectAssetDestination(libraryId: number) {
    if (libraryId <= 0) return;
    setAssetDestinationMode('value');
    setAssetDestinationLibraryId(libraryId);
    setSelectedLibraryId(libraryId);
    updateAssetDestination.mutate({ selection: 'value', libraryId });
  }
  useEffect(() => {
    if (assetEditPathRef.current !== asset.path) {
      assetEditPathRef.current = asset.path;
      assetEditAuthoritativeRef.current = authoritativeAssetEdit;
      pendingAssetEditRefreshRef.current = false;
      setPersistedAssetEdit(authoritativeAssetEdit);
      setAssetEditDraft(authoritativeAssetEdit);
      assetEditDraftRef.current = authoritativeAssetEdit;
      resetAssetEdit();
      return;
    }

    if (!assetInfoDocumentsEqual(assetEditAuthoritativeRef.current, authoritativeAssetEdit)) {
      assetEditAuthoritativeRef.current = authoritativeAssetEdit;
      pendingAssetEditRefreshRef.current = true;
    }
    if (!pendingAssetEditRefreshRef.current || assetEditDirty) return;

    const refreshed = assetInfoRefreshReconciliation(persistedAssetEdit, assetEditDraft, authoritativeAssetEdit);
    if (!refreshed.dirty) {
      setPersistedAssetEdit(refreshed.baseline);
      setAssetEditDraft(refreshed.draft);
      assetEditDraftRef.current = refreshed.draft;
    }
    pendingAssetEditRefreshRef.current = false;
  }, [asset.path, authoritativeAssetEdit, persistedAssetEdit, assetEditDraft, assetEditDirty, resetAssetEdit]);

  function openSnapshotDialog(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    openAssetInfoDialog();
  }

  function openAssetInfoDialog() {
    if (assetInfoOpenedRef.current) {
      void queryClient.invalidateQueries({ queryKey: ['assetSnapshot', asset.path] });
    }
    assetInfoOpenedRef.current = true;
    setSnapshotTab('overview');
    setSelectedEvidenceJobId(null);
    setShowSnapshotDialog(true);
  }

  function closeSnapshotDialog() {
    if (assetEditDirty && !window.confirm('Discard unsaved Asset Info changes?')) {
      return;
    }
    if (assetEditDirty) {
      const discarded = assetInfoDiscardDraft(persistedAssetEdit);
      setAssetEditDraft(discarded);
      assetEditDraftRef.current = discarded;
      saveAssetEdit.reset();
    }
    setShowSnapshotDialog(false);
    snapshot.reset();
    setSnapshotOperation(null);
    setSelectedEvidenceJobId(null);
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
    setConversionDraft(next);
    setSnapshotTab('conversion');
    return 'Recommendations were applied to the draft. Save Asset Info to commit them.';
  }

  function toggleSnapshotStream(type: MediaStreamInfo['type'], index: number, keep: boolean) {
    const scan = snapshotData;
    if (!scan) {
      return;
    }
    const current = conversionStreamIndexes(conversionDraft, scan, type);
    const next = keep ? normalizeNumberList([...current, index]) : safeArray(current).filter((candidate) => candidate !== index);
    const indexes = normalizeNumberList(next);
    const nextDraft = withStreamSelection(conversionDraft, type, indexes);
    saveAssetTracks.reset();
    setConversionDraft(nextDraft);
  }

  function updateSubtitleSidecarFormats(index: number, value: string) {
    const formats = value === 'inherit' ? undefined : value.split('+').filter((format): format is 'original' | 'srt' => format === 'original' || format === 'srt');
    const current = { ...(conversionDraft.subtitleSidecarFormatsByStream ?? {}) };
    if (formats?.length) current[String(index)] = formats;
    else delete current[String(index)];
    const nextDraft = { ...conversionDraft, subtitleSidecarFormatsByStream: Object.keys(current).length ? current : undefined };
    saveAssetTracks.reset();
    setConversionDraft(nextDraft);
  }

  function updateSubtitleRole(index: number, value: SubtitleSemanticRole | 'auto') {
    const current = { ...(conversionDraft.subtitleRoleOverrides ?? {}) };
    if (value === 'auto') delete current[String(index)];
    else current[String(index)] = { role: value };
    const nextDraft = { ...conversionDraft, subtitleRoleOverrides: current };
    saveAssetTracks.reset();
    setConversionDraft(nextDraft);
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
    saveAssetTracks.reset();
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

  function resetTrackChanges() {
    if (saveAssetTracks.isPending || saveAssetEdit.isPending || renameAsset.isPending) return;
    const current = assetEditDraftRef.current;
    const reset = assetInfoEditDocument(
      current.metadata,
      mergeAssetTracks(current.conversion, persistedAssetEdit.conversion),
    );
    setAssetEditDraft(reset);
    assetEditDraftRef.current = reset;
    saveAssetTracks.reset();
  }

  function saveTrackChanges() {
    if (!tracksDirty || rowLocked || !assetInfoCapability(asset, 'manageTracks').allowed || saveAssetTracks.isPending || saveAssetEdit.isPending || renameAsset.isPending || asset.missing) return;
    saveAssetTracks.mutate({ path: asset.path, ...pendingTracksPatch });
  }

  function saveConversionOverrides() {
    if (!assetEditDirty || saveAssetEdit.isPending || saveAssetTracks.isPending || renameAsset.isPending) return;
    saveAssetEdit.mutate(assetEditDraft);
  }

  function openAssetTestEncode() {
    if (!canTestEncode) return;
    setShowTestEncodeDialog(true);
  }

  function openAssetInLab() {
    window.location.href = assetLabHref(asset.path);
  }

  function delay(ms: number) {
    return new Promise<void>((resolve) => {
      window.setTimeout(resolve, ms);
    });
  }

  const waitForSubtitleOperation = useCallback(async (
    operationId: string,
    key: string,
    streamIndex: number,
    format: 'original' | 'srt' | 'ass',
    onUpdate?: (operation: SubtitleExtractionOperation) => void,
    readOperation: (operationId: string) => Promise<SubtitleExtractionOperation> = api.subtitleExtractionOperation,
  ) => {
  if (subtitleOperationPollers.current.has(operationId)) {
    return;
  }

  subtitleOperationPollers.current.add(operationId);

  try {
    while (true) {
      const operation = await readOperation(operationId);
      onUpdate?.(operation);

      setSubtitleGenerations((current) => ({
        ...current,
        [key]: {
          status: operation.status === 'cancelled' ? 'cancelled' : 'running',
          streamIndex,
          format,
          operationId,
          cancelPending: current[key]?.cancelPending,
          progress: operation.progress,
          phase: operation.phase,
          processed: operation.processed,
          total: operation.total,
          message: operation.message,
        },
      }));

      if (subtitleOperationStopsPolling(operation.status)) {
        if (operation.status === 'completed') {
          return operation.result;
        }

        if (operation.status === 'cancelled') {
          setSubtitleGenerations((current) => ({
            ...current,
            [key]: {
              ...current[key],
              status: 'cancelled',
              streamIndex,
              format,
              operationId,
              cancelPending: false,
              message: operation.message || 'Subtitle generation cancelled.',
            },
          }));
          return undefined;
        }

        throw new Error(
          operation.error ||
            operation.message ||
            'Subtitle OCR failed.',
        );
      }

      await new Promise<void>((resolve) => window.setTimeout(resolve, 500));
    }
  } finally {
    subtitleOperationPollers.current.delete(operationId);
  }
  }, [setSubtitleGenerations]);

  const restoreSubtitleOperations = useCallback(async () => {
  try {
    const response = await api.subtitleExtractionOperations(asset.path);

    const runningOperations = response.operations.filter(
      (operation) => operation.status === 'running',
    );

    for (const operation of runningOperations) {
      if (operation.kind === 'compose') {
        setSubtitleCompositionOperation(operation);
        void waitForSubtitleOperation(
          operation.id,
          'composition',
          -1,
          'ass',
			(operationUpdate) => setSubtitleCompositionOperation(operationUpdate),
          api.subtitleOperation,
        ).catch((error) => {
          setSubtitleCompositionOperation((current) => current ? { ...current, status: 'error', error: error instanceof Error ? error.message : 'Subtitle composition failed.' } : current);
        });
        continue;
      }
      const format = operation.format;
      if (format === 'both') {
        continue;
      }
      const key = subtitleGenerationKey(operation.streamIndex, format);

      setSubtitleGenerations((current) => ({
        ...current,
        [key]: {
          status: 'running',
          streamIndex: operation.streamIndex,
          format,
          operationId: operation.id,
          cancelPending: false,
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
  }, [asset.path, setSubtitleCompositionOperation, setSubtitleGenerations, waitForSubtitleOperation]);

  useEffect(() => {
    if (!showSnapshotDialog || snapshotTab !== 'tracks' || asset.missing) return;
    void restoreSubtitleOperations();
  }, [showSnapshotDialog, snapshotTab, asset.missing, restoreSubtitleOperations]);

async function composeSubtitles(input: SubtitleCompositionInput) {
  if (subtitleCompositionOperation?.status === 'running') return;
  setSubtitleCompositionError('');
  try {
    const started = await api.composeSubtitles(input);
    const initial: SubtitleExtractionOperation = {
      id: started.operationId,
      assetPath: asset.path,
      status: 'running',
      phase: started.phase,
      progress: started.progress,
      processed: 0,
      total: 0,
      streamIndex: -1,
      format: 'ass',
      kind: 'compose',
      message: 'Starting subtitle composition…',
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };
    setSubtitleCompositionOperation(initial);
    await waitForSubtitleOperation(
      started.operationId,
      'composition',
      -1,
      'ass',
      (operation) => setSubtitleCompositionOperation(operation),
      api.subtitleOperation,
    );
    await refreshSubtitleEvidence();
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Subtitle composition failed.';
    setSubtitleCompositionError(message);
    setSubtitleCompositionOperation((current) => current ? { ...current, status: 'error', error: message } : current);
  }
}

async function cancelSubtitleComposition() {
  const operation = subtitleCompositionOperation;
  if (!operation || operation.status !== 'running') return;
  const operationId = operation.id;
  try {
    const cancelled = await api.cancelSubtitleOperation(operationId);
    setSubtitleCompositionOperation(cancelled);
  } catch (error) {
    setSubtitleCompositionError(error instanceof Error ? error.message : 'Unable to cancel subtitle composition.');
  }
}

async function retrySubtitleComposition() {
  const operation = subtitleCompositionOperation;
  if (!operation || !operation.retryable) return;
  setSubtitleCompositionError('');
  try {
    const retried = await api.retrySubtitleOperation(operation.id);
    setSubtitleCompositionOperation(retried.operation);
    await waitForSubtitleOperation(
      retried.operation.id,
      'composition',
      -1,
      'ass',
      (operationUpdate) => setSubtitleCompositionOperation(operationUpdate),
      api.subtitleOperation,
    );
    await refreshSubtitleEvidence();
  } catch (error) {
    const message = error instanceof Error ? error.message : 'Subtitle composition retry failed.';
    setSubtitleCompositionError(message);
    setSubtitleCompositionOperation((current) => current ? { ...current, status: 'error', error: message } : current);
  }
}

async function waitForSubtitleQualityOperation(operationId: string) {
  if (subtitleOperationPollers.current.has(operationId)) return undefined;
  subtitleOperationPollers.current.add(operationId);
  try {
    while (true) {
      const operation = await api.subtitleExtractionOperation(operationId);
      if (subtitleOperationStopsPolling(operation.status)) {
        if (operation.status === 'completed') return operation.qualityReport;
        throw new Error(operation.error || operation.message || 'OCR quality operation failed.');
      }
      await delay(500);
    }
  } finally {
    subtitleOperationPollers.current.delete(operationId);
  }
}

async function generateExternalSubtitle(
  streamIndex: number,
  format: 'original' | 'srt' | 'ass',
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

    if (!result) {
      return;
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

      await refreshSubtitleEvidence();
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

  async function cancelExternalSubtitle(
    streamIndex: number,
    format: 'original' | 'srt' | 'ass',
  ) {
    const key = subtitleGenerationKey(streamIndex, format);
    const generation = subtitleGenerations[key];
    if (generation?.status !== 'running' || !generation.operationId || generation.cancelPending) {
      return;
    }

    setSubtitleGenerations((current) => ({
      ...current,
      [key]: {
        ...current[key],
        status: 'running',
        streamIndex,
        format,
        operationId: generation.operationId,
        cancelPending: true,
        message: 'Cancelling subtitle generation…',
      },
    }));

    try {
      const operation = await api.cancelSubtitleExtractionOperation(generation.operationId);
      if (operation.status === 'cancelled') {
        setSubtitleGenerations((current) => ({
          ...current,
          [key]: {
            ...current[key],
            status: 'cancelled',
            streamIndex,
            format,
            operationId: generation.operationId,
            cancelPending: false,
            message: operation.message || 'Subtitle generation cancelled.',
          },
        }));
      }
    } catch (error) {
      setSubtitleGenerations((current) => ({
        ...current,
        [key]: {
          ...current[key],
          status: 'running',
          streamIndex,
          format,
          operationId: generation.operationId,
          cancelPending: false,
          message: error instanceof Error ? error.message : 'Unable to cancel subtitle generation.',
        },
      }));
    }
  }

  async function analyzeExternalSubtitle(sidecar: ExternalSubtitle) {
    const selection = subtitleOCRSidecarSelection(externalSubtitles.data ?? [], sidecar);
    const key = subtitleOCRReviewKey(sidecar);
    setSubtitleQualityMessages((current) => ({ ...current, [key]: 'Analyzing OCR quality…' }));
    try {
      const operation = await api.analyzeSubtitleOCR({
        mediaPath: asset.path,
        sidecarPath: selection.sidecarPaths[0],
        sidecarPaths: selection.sidecarPaths,
        format: selection.format,
        logicalTransformKey: sidecar.logicalTransformKey,
      });
      const report = await waitForSubtitleQualityOperation(operation.operationId);
      if (report) {
        setSubtitleQualityReports((current) => ({ ...current, [key]: report }));
        setSubtitleQualityMessages((current) => ({ ...current, [key]: '' }));
      }
    } catch (error) {
      setSubtitleQualityMessages((current) => ({ ...current, [key]: error instanceof Error ? error.message : 'OCR analysis failed.' }));
    }
  }

  function openTranslation(source: SubtitleTranslationSourceSelector) {
    setTranslationSource(source);
    setTranslationDialogOpen(true);
  }

  function reviewTranslationOCR(operation: SubtitleExtractionOperation) {
    const target = operation.translation?.ocrReviewTarget ?? {
      state: 'missing',
      message: 'This server did not return an exact OCR review target. Return to Translation and retry to refresh the request identity.',
    };
    setTranslationReviewTarget(target);
    setTranslationDialogOpen(false);
    setTranslationReviewDialogOpen(true);
  }

  async function restyleExternalSubtitle(sidecar: ExternalSubtitle, mode: 'restyle' | 'srt_to_ass', options: { replace?: boolean; overrideExistingASSStyles?: boolean } = {}) {
    const eligibility = sidecar.eligibility;
    const allowed = mode === 'restyle' ? eligibility?.restyle : eligibility?.srtToAss;
    if (eligibility && !allowed?.eligible) {
      setSubtitleRestyleMessages((current) => ({ ...current, [sidecar.path]: allowed?.reason || 'This subtitle operation is not eligible.' }));
      return;
    }
    if (!sidecar.sourceRevision) {
      setSubtitleRestyleMessages((current) => ({ ...current, [sidecar.path]: 'Source identity is unavailable; refresh Asset Info before restyling.' }));
      return;
    }
    const messageKey = sidecar.path;
    try {
      const started = await api.restyleSubtitle({
        path: asset.path,
        sourcePath: sidecar.path,
        sourceRevision: sidecar.sourceRevision,
        representation: sidecar.format,
        language: sidecar.language,
        mode,
        replace: options.replace,
        overrideExistingASSStyles: options.overrideExistingASSStyles,
      });
      setSubtitleRestyleOperations((current) => ({
        ...current,
        [messageKey]: { operationId: started.operationId, status: 'running', progress: started.progress, phase: started.phase, message: 'Preparing subtitle restyle…' },
      }));
      await waitForSubtitleOperation(started.operationId, `restyle:${messageKey}`, -1, 'ass', (operation) => {
        setSubtitleRestyleOperations((current) => ({
          ...current,
          [messageKey]: {
            operationId: started.operationId,
            status: operation.status === 'completed' ? 'completed' : operation.status === 'cancelled' ? 'cancelled' : operation.status === 'error' ? 'failed' : 'running',
            progress: operation.progress,
            phase: operation.phase,
            message: operation.message,
            error: operation.error,
            retryable: operation.retryable ?? Boolean(operation.status === 'error' || operation.status === 'cancelled'),
          },
        }));
      }, api.subtitleOperation);
      const final = await api.subtitleOperation(started.operationId);
      if (final.status === 'completed') {
        setSubtitleRestyleMessages((current) => ({ ...current, [messageKey]: 'Styled sibling published.' }));
        await refreshSubtitleEvidence();
      }
    } catch (error) {
      setSubtitleRestyleOperations((current) => ({ ...current, [messageKey]: { operationId: current[messageKey]?.operationId, status: 'failed', progress: current[messageKey]?.progress ?? 0, phase: 'error', error: error instanceof Error ? error.message : 'Subtitle restyle failed.', retryable: true } }));
    }
  }

  async function cancelExternalRestyle(sidecar: ExternalSubtitle) {
    const current = subtitleRestyleOperations[sidecar.path];
    if (!current?.operationId || current.status !== 'running') return;
    try {
      const operation = await api.cancelSubtitleOperation(current.operationId);
      setSubtitleRestyleOperations((state) => ({ ...state, [sidecar.path]: { ...state[sidecar.path], status: operation.status === 'cancelled' ? 'cancelled' : 'running', phase: operation.phase, progress: operation.progress, message: operation.message, retryable: operation.status === 'cancelled' } }));
    } catch (error) {
      setSubtitleRestyleOperations((state) => ({ ...state, [sidecar.path]: { ...state[sidecar.path], status: 'failed', error: error instanceof Error ? error.message : 'Unable to cancel subtitle restyle.', retryable: true } }));
    }
  }

  async function retryExternalRestyle(sidecar: ExternalSubtitle) {
    const current = subtitleRestyleOperations[sidecar.path];
    if (!current?.operationId || !current.retryable) return;
    try {
      const started = await api.retrySubtitleOperation(current.operationId);
      setSubtitleRestyleOperations((state) => ({ ...state, [sidecar.path]: { operationId: started.operation.id, status: 'running', progress: started.operation.progress, phase: started.operation.phase, message: 'Retrying subtitle operation…' } }));
      await waitForSubtitleOperation(started.operation.id, `restyle:${sidecar.path}:retry`, -1, 'ass', (operation) => {
        setSubtitleRestyleOperations((state) => ({ ...state, [sidecar.path]: { operationId: started.operation.id, status: operation.status === 'completed' ? 'completed' : operation.status === 'cancelled' ? 'cancelled' : operation.status === 'error' ? 'failed' : 'running', progress: operation.progress, phase: operation.phase, message: operation.message, error: operation.error, retryable: operation.retryable ?? (operation.status === 'error' || operation.status === 'cancelled') } }));
      }, api.subtitleOperation);
      await refreshSubtitleEvidence();
    } catch (error) {
      setSubtitleRestyleOperations((state) => ({ ...state, [sidecar.path]: { ...state[sidecar.path], status: 'failed', error: error instanceof Error ? error.message : 'Subtitle retry failed.', retryable: true } }));
    }
  }

  async function regenerateEmbeddedSubtitle(streamIndex: number) {
    const key = subtitleGenerationKey(streamIndex, 'ass');
    setSubtitleGenerations((current) => ({
      ...current,
      [key]: { status: 'running', streamIndex, format: 'ass', progress: 1, phase: 'preparing', processed: 0, total: 0, message: 'Preparing exact subtitle regeneration' },
    }));
    try {
      const started = await api.restyleSubtitle({
        path: asset.path,
        sourcePath: asset.path,
        representation: 'ass',
        mode: 'regenerate',
        streamIndex,
      });
      const result = await waitForSubtitleOperation(started.operationId, key, streamIndex, 'ass');
      setSubtitleGenerations((current) => ({
        ...current,
        [key]: { ...current[key], status: 'success', progress: 100, phase: 'completed', message: result?.created?.length ? 'Regenerated styled ASS sidecar.' : 'ASS sidecar already exists.' },
      }));
      await refreshSubtitleEvidence();
    } catch (error) {
      setSubtitleGenerations((current) => ({
        ...current,
        [key]: { ...current[key], status: 'error', message: error instanceof Error ? error.message : 'Subtitle regeneration failed.' },
      }));
    }
  }

  async function applyExternalSubtitleCorrection(
    sidecar: ExternalSubtitle,
    report: SubtitleOCRQualityReport,
    finding: SubtitleOCRQualityFinding,
    text: string,
  ) {
    await applyExternalSubtitleCorrections(sidecar, report, [{ cueId: finding.cueId, text }]);
  }

  async function applyExternalSubtitleCorrections(
    sidecar: ExternalSubtitle,
    report: SubtitleOCRQualityReport,
    corrections: Array<{ cueId: number; text?: string; suggestion?: string }>,
  ) {
    const selection = subtitleOCRSidecarSelection(externalSubtitles.data ?? [], sidecar);
    const key = subtitleOCRReviewKey(sidecar);
    setSubtitleQualityMessages((current) => ({ ...current, [key]: 'Applying validated correction…' }));
    try {
      const operation = await api.correctSubtitleOCR({
        mediaPath: asset.path,
        sidecarPath: selection.sidecarPaths[0],
        sidecarPaths: selection.sidecarPaths,
        format: selection.format,
        logicalTransformKey: sidecar.logicalTransformKey,
        baselineRevision: report.baselineRevision,
        dictionaryLanguage: report.dictionaryLanguage,
        dictionaryRevision: report.dictionaryRevision,
        revisions: report.revisionSet,
        corrections: corrections.map((correction) => correction.suggestion
          ? { cueId: correction.cueId, action: 'apply' as const, suggestion: correction.suggestion }
          : { cueId: correction.cueId, action: 'edit' as const, text: correction.text ?? '' }),
      });
      await waitForSubtitleQualityOperation(operation.operationId);
      setSubtitleQualityReports((current) => {
        const next = { ...current };
        delete next[key];
        return next;
      });
      setSubtitleQualityMessages((current) => ({ ...current, [key]: 'Correction published. Re-analyze to review the new baseline.' }));
      await refreshSubtitleEvidence();
    } catch (error) {
      setSubtitleQualityMessages((current) => ({ ...current, [key]: error instanceof Error ? error.message : 'OCR correction failed.' }));
    }
  }

  function resetConversionOverrides() {
    const empty: AssetConversionOverrideState = { subtitleRoleOverrides: {} };
    setConversionDraft(empty);
  }

  function queueAsset(event?: MouseEvent<HTMLButtonElement>) {
    event?.stopPropagation();
    if (assetEditDirty) {
      setAssetEditQueueMessage('Save Asset Info changes before Queue. Unsaved changes are not queued.');
      return;
    }
    setAssetEditQueueMessage('');
    const queueProfileId =
      effectiveProfileId > 0
        ? effectiveProfileId
        : 0;
    if (!selectedLibraryId) {
      setAssetEditQueueMessage('Queue is unavailable because this asset has no effective destination library. Configure a destination before queueing.');
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

  function toggleAssetReview(event?: MouseEvent<HTMLButtonElement>) {
    event?.stopPropagation();
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
  }

  function archivedOriginalMutationInput(original: AssetArchivedOriginalProjection) {
    if (original.relationship !== 'current' || !original.assetRecordId || !asset.id) return undefined;
    return { path: original.path, archiveAssetRecordId: original.assetRecordId, libraryAssetRecordId: asset.id };
  }

  function recoverArchivedAsset(event: MouseEvent<HTMLButtonElement>, original: AssetArchivedOriginalProjection) {
    event.stopPropagation();
    const input = archivedOriginalMutationInput(original);
    if (!input || !original.restore?.allowed) return;
    const confirmed = window.confirm('Recover this original from Archive? Converted files will not be deleted.');
    if (confirmed) {
      recoverAsset.mutate(input);
    }
  }

  function requestArchiveDeletion(event: MouseEvent<HTMLButtonElement>, original: AssetArchivedOriginalProjection) {
    event.stopPropagation();
    const input = archivedOriginalMutationInput(original);
    if (!input || !original.deleteArchivedOriginal?.allowed) return;
    const confirmed = window.confirm(`Permanently delete this archived original? The current Library publication and Queue provenance will remain.\n\n${original.path}`);
    if (confirmed) deleteArchiveAsset.mutate(input);
  }

  function safelyDeleteConvertedAsset(event?: MouseEvent<HTMLButtonElement>) {
    event?.stopPropagation();
    const confirmed = window.confirm('Delete this converted Library asset? MVForge will proceed only if its archived original exists and can be restored to the original Raw path. Reports, logs, and job history will be preserved.');
    if (confirmed) {
      deleteConvertedAsset.mutate(asset.path);
    }
  }

  function returnPublishedAsIsAsset(event?: MouseEvent<HTMLButtonElement>) {
    event?.stopPropagation();
    const confirmed = window.confirm('Remove this Published as-is asset from Library and return it to its original Raw path? Its original filename and external subtitle names will be restored.');
    if (confirmed) {
      returnPublishedAsIs.mutate(asset.path);
    }
  }

  return (
    <>
      {compactAssetsRow ? (
        <TableRow
          hover
          selected={selected}
          aria-selected={selected}
          data-testid="asset-list-row"
          data-row-surface="compact-asset"
          tabIndex={0}
          onClick={onSelectAsset}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault();
              onSelectAsset?.();
            }
          }}
          sx={{
            cursor: 'pointer',
            transition: 'background-color 120ms ease, box-shadow 120ms ease',
            '& > td': { borderTop: '1px solid rgba(21, 48, 74, 0.62)', borderBottomColor: 'rgba(21, 48, 74, 0.62)' },
            '&:hover > td': { backgroundColor: '#112a44' },
            '&.Mui-selected, &.Mui-selected:hover': { background: 'linear-gradient(90deg, rgba(48, 55, 169, 0.72), rgba(31, 43, 115, 0.5))' },
            '&.Mui-selected > td': { backgroundColor: 'transparent', borderTopColor: 'rgba(108, 76, 246, 0.56)', borderBottomColor: 'rgba(31, 68, 103, 0.8)' },
            '&.Mui-selected > td:first-of-type': { boxShadow: 'inset 2px 0 0 #6c4cf6' },
            '&:focus-visible': { outline: '2px solid #8265ff', outlineOffset: -2 },
          }}
        >
          {bulkSelectionEnabled ? (
            <TableCell padding="checkbox">
              <Checkbox
                size="small"
                checked={bulkSelected}
                disabled={bulkSelectionDisabled}
                onClick={(event) => event.stopPropagation()}
                onKeyDown={(event) => event.stopPropagation()}
                onChange={(event) => onBulkSelectionChange(asset.path, event.target.checked)}
                inputProps={{ 'aria-label': `Select asset ${asset.fileName}` }}
              />
            </TableCell>
          ) : null}
          <TableCell sx={{ minWidth: 0 }}>
            <Tooltip title={asset.fileName}>
              <Stack direction="row" alignItems="center" spacing={0.65} sx={{ minWidth: 0 }}>
                <InsertDriveFileOutlinedIcon sx={{ fontSize: 15, flexShrink: 0, color: selected ? '#b8a7ff' : '#8fb3d0' }} />
                <Typography variant="body2" fontWeight={600} noWrap sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', fontSize: '0.8rem', color: selected ? '#e7eef8' : '#afc1d5' }}>
                  {asset.fileName}
                </Typography>
              </Stack>
            </Tooltip>
          </TableCell>
          <TableCell>
            {cachedAdvisor && Number.isFinite(cachedAdvisor.score) ? (
              <Tooltip title={cachedAdvisor.summary || `Advisor score ${cachedAdvisor.score}`}>
                <Stack spacing={0.28} alignItems="flex-start" sx={{ width: 36 }} aria-label={`Advisor score ${cachedAdvisor.score}`}>
                  <Typography component="span" sx={{ fontSize: '0.8rem', lineHeight: 1, fontWeight: 700, color: selected ? '#c9d3ff' : '#aebdff', fontVariantNumeric: 'tabular-nums' }}>
                    {cachedAdvisor.score}
                  </Typography>
                  <Box aria-hidden="true" data-testid="advisor-score-indicator" sx={{ width: 35, height: 2, borderRadius: 2, bgcolor: '#1b3853', overflow: 'hidden' }}>
                    <Box sx={{ width: `${Math.max(0, Math.min(100, cachedAdvisor.score))}%`, height: '100%', background: 'linear-gradient(90deg, #555cff, #24c5cb)' }} />
                  </Box>
                </Stack>
              </Tooltip>
            ) : advisor.isError ? (
              <Typography variant="caption" aria-label="Advisor unavailable" noWrap sx={{ fontSize: '0.75rem', color: '#59718b' }}>Unavailable</Typography>
            ) : !confidenceEnabled ? (
              <Typography variant="caption" aria-label="Advisor off" noWrap sx={{ fontSize: '0.75rem', color: '#59718b' }}>Off</Typography>
            ) : (
              <Typography variant="caption" noWrap aria-label="Advisor not evaluated" sx={{ fontSize: '0.75rem', color: '#59718b' }}>
                Not evaluated
              </Typography>
            )}
          </TableCell>
          <TableCell>
            <Typography component="span" variant="caption" noWrap title={pipelineState.label} sx={{
              display: 'inline-flex',
              maxWidth: '100%',
              alignItems: 'center',
              gap: 0.45,
              px: 0.7,
              py: 0.14,
              borderRadius: '999px',
              bgcolor: pipelineTone.background,
              color: pipelineTone.color,
              fontSize: '0.75rem',
              fontWeight: 600,
              lineHeight: 1.35,
              '&::before': { content: '""', width: 4, height: 4, flexShrink: 0, borderRadius: '50%', bgcolor: 'currentColor', boxShadow: '0 0 6px currentColor' },
            }}>{pipelineState.label}</Typography>
            {isBlockedByReview ? <Typography variant="caption" color="warning.main" noWrap>Needs review</Typography> : null}
          </TableCell>
          {compactRaw ? (
            <>
              <TableCell><Typography variant="body2" noWrap title={rawSourceCodec} sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{rawSourceCodec}</Typography></TableCell>
              <TableCell><Typography variant="body2" noWrap sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{technicalResolution}</Typography></TableCell>
              <TableCell><Typography variant="body2" noWrap sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{formatDurationSummary(technical?.duration)}</Typography></TableCell>
              <TableCell><Typography variant="body2" noWrap sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{technicalBitrate}</Typography></TableCell>
            </>
          ) : (
            <>
              <TableCell><Typography variant="body2" noWrap sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{technical?.videoCodec || 'Unavailable'}</Typography></TableCell>
              <TableCell><Typography variant="body2" noWrap sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{technicalResolution}</Typography></TableCell>
              <TableCell><Typography variant="body2" noWrap title={technical?.encoder || 'Unavailable'} sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{technical?.encoder || 'Unavailable'}</Typography></TableCell>
              <TableCell><Typography variant="body2" noWrap sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{formatDurationSummary(technical?.duration)}</Typography></TableCell>
              <TableCell><Typography variant="body2" noWrap sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{technicalBitrate}</Typography></TableCell>
              <TableCell><Typography variant="body2" noWrap sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{technicalRange}</Typography></TableCell>
            </>
          )}
          <TableCell><Typography variant="body2" noWrap sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{formatBytes(asset.sizeBytes)}</Typography></TableCell>
          <TableCell>
            <Typography variant="body2" noWrap title={asset.modifiedAt} sx={{ fontSize: '0.8rem', fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace' }}>{formatDate(asset.modifiedAt)}</Typography>
          </TableCell>
          <TableCell padding="checkbox" align="right" onClick={(event) => event.stopPropagation()}>
            <Tooltip title="More asset actions">
              <IconButton size="small" aria-label={`More actions for ${asset.fileName}`} onClick={(event) => setAssetMenuAnchor(event.currentTarget)} sx={{ width: 26, height: 26, p: 0.25, color: '#59718b', '&:hover': { bgcolor: '#112a44', color: '#e7eef8' } }}>
                <MoreVertIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </TableCell>
        </TableRow>
      ) : <TableRow hover>
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
        {!compactUnprocessed && !isConverted && !isPublishedAsIs && !isAccepted ? (
          <TableCell>
            <Tooltip title="Click para evaluar">
              <span>
                <Button
                  size="small"
                  variant="outlined"
                  startIcon={<InfoOutlinedIcon />}
                  aria-label="Click para evaluar"
                  disabled={!confidenceEnabled || effectiveProfileId <= 0 || !assetInfoCapability(asset, 'refreshAnalysis').allowed}
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
            <AssetCategorySelect
              value={metadataCategory}
              options={assetCategories}
              onChange={saveAssetCategory}
              label="Category"
              size="small"
              disabled={rowLocked || asset.missing}
            />
          </TableCell>
        ) : null}
        {!compactUnprocessed && !isConverted && !isPublishedAsIs && !isAccepted ? (
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
            {reconciliationJobId > 0 ? (
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
                  disabled={rowLocked || updateReview.isPending}
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
                  <IconButton color="error" onClick={safelyDeleteConvertedAsset} disabled={!assetInfoCapability(asset, 'deleteArchivedOriginal').allowed || deleteConvertedAsset.isPending} aria-label={`Safely delete ${asset.fileName}`} sx={actionIconSx}>
                    <DeleteForeverIcon />
                  </IconButton>
                </span>
              </Tooltip>
            ) : isPublishedAsIs ? (
              <Tooltip title={returnPublishedAsIs.isPending ? 'Returning asset to Raw' : 'Remove from Library and return the original file to Raw'}>
                <span onClick={(event) => event.stopPropagation()}>
                  <IconButton color="error" onClick={returnPublishedAsIsAsset} disabled={!assetInfoCapability(asset, 'restore').allowed || returnPublishedAsIs.isPending} aria-label={`Return ${asset.fileName} to Raw`} sx={actionIconSx}>
                    <DeleteForeverIcon />
                  </IconButton>
                </span>
              </Tooltip>
            ) : isLibraryReplacement ? null : (
              <Tooltip title={hasOpenJob ? 'This asset already has an open job' : isBlockedByReview ? 'Resolve review before queueing' : !selectedLibraryId ? 'Queue unavailable: no effective destination library is configured.' : !assetInfoCapability(asset, 'queue').allowed ? assetActionReasonMessage(assetInfoCapability(asset, 'queue'), 'Queue is unavailable for this asset.') : 'Queue asset'}>
                <span>
                  <IconButton
                    color="primary"
                    onClick={queueAsset}
                    disabled={!assetInfoCapability(asset, 'queue').allowed || profileAssignments.isLoading || updateProfileAssignment.isPending || createJob.isPending || !selectedLibraryId || isBlockedByReview || hasOpenJob}
                    aria-label={`Queue ${asset.fileName}`}
                    sx={actionIconSx}
                  >
                    <PlaylistAddIcon />
                  </IconButton>
                </span>
              </Tooltip>
            )}
            {isLibraryReplacement && !isPublishedAsIs ? (
              <Tooltip title={hasOpenJob ? 'This asset already has an open job' : isBlockedByReview ? 'Resolve review before queueing' : !assetInfoCapability(asset, 'reconvert').allowed ? assetActionReasonMessage(assetInfoCapability(asset, 'reconvert'), 'Replacement is unavailable for this asset.') : 'Convert and safely replace this library asset'}>
                <span>
                  <IconButton
                    color="warning"
                    onClick={queueAsset}
                    disabled={!assetInfoCapability(asset, 'reconvert').allowed || profileAssignments.isLoading || updateProfileAssignment.isPending || createJob.isPending || isBlockedByReview || hasOpenJob}
                    aria-label={`Replace ${asset.fileName}`}
                    sx={actionIconSx}
                  >
                    <PlaylistAddIcon />
                  </IconButton>
                </span>
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
            open={deleteArchiveAsset.isSuccess || deleteArchiveAsset.isError}
            autoHideDuration={7000}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            onClose={(_, reason) => {
              if (reason !== 'clickaway') deleteArchiveAsset.reset();
            }}
          >
            <Alert severity={deleteArchiveAsset.isError ? 'warning' : 'success'} variant="filled" onClose={() => deleteArchiveAsset.reset()}>
              {deleteArchiveAsset.isError
                ? deleteArchiveAsset.error instanceof Error ? deleteArchiveAsset.error.message : 'Archived original deletion failed.'
                : 'Archived original permanently deleted. Library publication and job provenance were preserved.'}
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
      </TableRow>}
      <Snackbar
        open={Boolean(assetEditQueueMessage) || createJob.isSuccess || createJob.isError}
        autoHideDuration={assetEditQueueMessage ? 7000 : createJob.isSuccess ? 4000 : 7000}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        onClose={(_, reason) => {
          if (reason !== 'clickaway') {
            setAssetEditQueueMessage('');
            createJob.reset();
          }
        }}
      >
        <Alert
          severity={assetEditQueueMessage ? 'warning' : createJob.isError ? 'warning' : 'success'}
          variant="filled"
          onClose={() => { setAssetEditQueueMessage(''); createJob.reset(); }}
          sx={{ maxWidth: 520, overflowWrap: 'anywhere' }}
        >
          {assetEditQueueMessage || (createJob.isError
            ? createJob.error instanceof Error ? createJob.error.message : 'Could not queue this asset.'
            : 'Asset queued individually.')}
        </Alert>
      </Snackbar>
      <Menu anchorEl={assetMenuAnchor} open={Boolean(assetMenuAnchor)} onClose={() => setAssetMenuAnchor(null)}>
        <MenuItem onClick={() => { setAssetMenuAnchor(null); openAssetInfoDialog(); }}>Asset Info</MenuItem>
        {compactRaw ? (
          <MenuItem
            onClick={() => { setAssetMenuAnchor(null); refreshSnapshot(); }}
            disabled={asset.missing || snapshotRunning || snapshot.isPending || !assetInfoCapability(asset, 'refreshAnalysis').allowed}
          >
            Scan / Analyze Asset
          </MenuItem>
        ) : null}
        {compactAssetsRow && mode !== 'converted' ? (
          <>
            {!hidePreConversionActions ? (
              <MenuItem
                onClick={() => { setAssetMenuAnchor(null); setShowAdvisorDialog(true); }}
                disabled={!confidenceEnabled || effectiveProfileId <= 0 || !assetInfoCapability(asset, 'refreshAnalysis').allowed}
              >
                Advisor
              </MenuItem>
            ) : null}
            <MenuItem onClick={() => { setAssetMenuAnchor(null); openAssetInLab(); }} disabled={!labCapability.allowed}>
              Open in Lab
            </MenuItem>
            {!hidePreConversionActions && (!isConverted || isLibraryReplacement) ? (
              <MenuItem onClick={() => { setAssetMenuAnchor(null); setAssignProfileOpen(true); }} disabled={rowLocked || updateProfileAssignment.isPending}>
                Assign Profile
              </MenuItem>
            ) : null}
            {!hidePreConversionActions && canGenerateTestEncode ? (
              <MenuItem onClick={() => { setAssetMenuAnchor(null); openAssetTestEncode(); }} disabled={!canTestEncode} title={testEncodeUnavailableReason}>
                Test Encode
              </MenuItem>
            ) : null}
          </>
        ) : !hidePreConversionActions && mode !== 'converted' ? (
          <MenuItem onClick={() => { setAssetMenuAnchor(null); setShowAdvisorDialog(true); }} disabled={!confidenceEnabled || effectiveProfileId <= 0 || !assetInfoCapability(asset, 'refreshAnalysis').allowed}>
            Advisor
          </MenuItem>
        ) : null}
        <MenuItem
          onClick={() => { setAssetMenuAnchor(null); queueAsset(); }}
          disabled={!assetInfoCapability(asset, isLibraryReplacement ? 'reconvert' : 'queue').allowed || rowLocked || isBlockedByReview || hasOpenJob || !selectedLibraryId || profileAssignments.isLoading || updateProfileAssignment.isPending || createJob.isPending}
          title={!selectedLibraryId ? 'Queue is unavailable because no effective destination library is configured.' : hasOpenJob ? 'Queue is unavailable while this asset has active work.' : isBlockedByReview ? 'Resolve the review block before Queue.' : undefined}
        >
          {isLibraryReplacement ? 'Queue replacement' : 'Queue asset'}
        </MenuItem>
        {reconciliationJobId > 0 ? (
          <MenuItem onClick={() => { setAssetMenuAnchor(null); reconcilePublication.mutate({ jobId: reconciliationJobId, path: asset.path }); }} disabled={reconcilePublication.isPending}>
            Confirm relocated publication
          </MenuItem>
        ) : null}
        {asset.status === 'unverified' ? (
          <MenuItem onClick={() => { setAssetMenuAnchor(null); updateAssetDecision.mutate({ path: asset.path, action: 'accept' }); }} disabled={updateAssetDecision.isPending || hasOpenJob}>
            Accept as-is
          </MenuItem>
        ) : isAccepted ? (
          <MenuItem onClick={() => { setAssetMenuAnchor(null); updateAssetDecision.mutate({ path: asset.path, action: 'reconsider' }); }} disabled={updateAssetDecision.isPending}>
            Reconsider asset
          </MenuItem>
        ) : null}
        {!isConverted || isLibraryReplacement ? (
          <MenuItem onClick={() => { setAssetMenuAnchor(null); toggleAssetReview(); }} disabled={rowLocked || updateReview.isPending}>
            {assetReview.requiresReview ? 'Clear review flag' : 'Mark for review'}
          </MenuItem>
        ) : null}
        {isConverted || isPublishedAsIs ? <Divider component="li" /> : null}
        {isConverted ? (
          <MenuItem onClick={() => { setAssetMenuAnchor(null); safelyDeleteConvertedAsset(); }} disabled={!assetInfoCapability(asset, 'deleteArchivedOriginal').allowed || deleteConvertedAsset.isPending}>
            Safely delete and restore original
          </MenuItem>
        ) : isPublishedAsIs ? (
          <MenuItem onClick={() => { setAssetMenuAnchor(null); returnPublishedAsIsAsset(); }} disabled={!assetInfoCapability(asset, 'restore').allowed || returnPublishedAsIs.isPending}>
            Return to Raw
          </MenuItem>
        ) : null}
      </Menu>
      <Dialog open={assignProfileOpen} onClose={() => setAssignProfileOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Assign Profile · {asset.fileName}</DialogTitle>
        <DialogContent dividers>
          <Stack spacing={1.5} sx={{ pt: 0.5 }}>
            <Typography variant="caption" color="text.secondary">Selections save immediately and use the existing asset-scope assignment rules.</Typography>
            <ProfileAutocomplete
              profiles={assetVideoProfiles}
              value={selectedProfileId}
              onChange={selectAssetVideoProfile}
              label="Video profile"
              size="small"
              disabled={rowLocked || updateProfileAssignment.isPending}
              allowInherit
              allowOverrideOnly
              allowAudioOnly
            />
            <AudioProfileAutocomplete
              profiles={assetAudioProfiles}
              value={selectedAudioProfileKey}
              onChange={selectAssetAudioProfile}
              label="Audio profile"
              size="small"
              disabled={rowLocked || updateProfileAssignment.isPending}
              allowInherit
            />
            <TrackProfileAutocomplete
              profiles={assetTrackProfiles}
              value={selectedTrackProfileKey}
              onChange={selectAssetTrackProfile}
              label="Tracks profile"
              disabled={rowLocked || updateProfileAssignment.isPending}
              allowInherit
            />
          </Stack>
        </DialogContent>
        <DialogActions><Button onClick={() => setAssignProfileOpen(false)}>Done</Button></DialogActions>
      </Dialog>
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
                  disabled={rowLocked || updateReview.isPending}
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
              crossOrigin="anonymous"
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
      <Dialog
        open={showSnapshotDialog}
        onClose={closeSnapshotDialog}
        maxWidth="lg"
        fullWidth
        PaperProps={{
          sx: (theme) => ({
            height: `calc(100dvh - ${theme.spacing(4)})`,
            maxHeight: `calc(100dvh - ${theme.spacing(4)})`,
            margin: theme.spacing(2),
            display: 'flex',
            flexDirection: 'column',
            minHeight: 0,
            overflow: 'hidden',
          }),
        }}
      >
        <DialogTitle sx={{ pb: 0.75, flex: '0 0 auto' }}>
          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', sm: 'flex-start' }} spacing={1}>
            <Stack sx={{ minWidth: 0 }}>
              <Typography variant="h2">Asset Snapshot</Typography>
              <Typography color="text.secondary" variant="body2" noWrap>
                {assetTitle(asset)}
              </Typography>
            </Stack>
            {storedSnapshotExists && !refreshRequired ? (
              <Button startIcon={<RefreshIcon />} variant="outlined" size="small" onClick={refreshSnapshot} disabled={asset.missing || operationPending}>
                Refresh analysis
              </Button>
            ) : null}
          </Stack>
        </DialogTitle>
        <Box sx={{ px: 3, flex: '0 0 auto' }}>
          <Tabs
            aria-label="Asset Info sections"
            value={assetInfoMainTab}
            onChange={(_, value: 'overview' | 'tracks' | 'job-information') => {
              if (value === 'job-information') setSnapshotTab(isJobInformationTab ? snapshotTab : 'execution');
              else setSnapshotTab(value);
            }}
            variant="scrollable"
            allowScrollButtonsMobile
          >
            <Tab value="overview" label="Overview" onClick={() => setSnapshotTab('overview')} />
            <Tab value="tracks" label="Tracks" />
            <Tab value="job-information" label="Job Information" />
          </Tabs>
          {assetInfoMainTab === 'overview' ? (
            <Tabs
              aria-label="Overview sections"
              value={snapshotTab}
              onChange={(_, value: AssetInfoSection) => setSnapshotTab(value)}
              variant="scrollable"
              allowScrollButtonsMobile
            >
              <Tab value="overview" label="Summary" />
              <Tab value="analysis" label="Analysis" />
              {!hidePreConversionActions ? <Tab value="advisor" label="MVForge Suggestions" /> : null}
              <Tab value="conversion" label="Quick Asset Overrides" />
              {canGenerateTestEncode && !hidePreConversionActions ? <Tab value="test-encode" label="Test Encode" /> : null}
            </Tabs>
          ) : null}
          {assetInfoMainTab === 'job-information' ? (
            <Tabs
              aria-label="Job Information sections"
              value={snapshotTab}
              onChange={(_, value: AssetInfoSection) => setSnapshotTab(value)}
              variant="scrollable"
              allowScrollButtonsMobile
            >
              <Tab value="execution" label="Execution / Result" />
              <Tab value="validation" label="Validation" />
              <Tab value="original" label="Original" />
              <Tab value="evidence" label="Evidence" />
              <Tab value="history" label="History" />
              <Tab value="publication" label="Current publication" />
              <Tab value="integration" label="Integration evidence" />
            </Tabs>
          ) : null}
        </Box>
        <DialogContent sx={{ pt: 1, flex: '1 1 auto', minHeight: 0, overflowY: 'auto', overflowX: 'hidden' }}>
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
            {['tracks', 'advisor', 'conversion', 'test-encode'].includes(snapshotTab) && !asset.missing && !storedSnapshot.isLoading && !operationPending && !storedSnapshotExists ? (
              <Alert
                severity="info"
                action={<Button color="inherit" size="small" onClick={() => snapshot.mutate({ path: asset.path })} disabled={!assetInfoCapability(asset, 'refreshAnalysis').allowed}>{assetInfoCapability(asset, 'refreshAnalysis').allowed ? 'Analyze asset' : assetActionReasonMessage(assetInfoCapability(asset, 'refreshAnalysis'), 'Analysis unavailable')}</Button>}
              >
                No snapshot available
              </Alert>
            ) : null}
            <>
                {snapshotTab === 'analysis' ? <Box>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    <Stack direction="row" alignItems="center" spacing={1}>
                      <Typography variant="h3">Source evidence</Typography>
                      {storedSnapshot.isLoading ? <LinearProgress sx={{ flex: 1 }} /> : <AnalysisEvidenceChip status={latestSnapshotAnalysisStatus(storedSnapshot.data) ?? 'unavailable'} />}
                    </Stack>
                    {storedSnapshot.isError ? <Alert severity="warning">Analysis evidence is unavailable: {storedSnapshot.error instanceof Error ? storedSnapshot.error.message : 'the backend could not classify the current snapshot'}</Alert> : null}
                    {latestSnapshotAnalysisStatus(storedSnapshot.data) === 'stale' ? (
                      <Alert severity="warning">
                        Snapshot evidence is stale{storedSnapshot.data?.analysisEvidence?.staleComponents?.length ? ` · ${storedSnapshot.data.analysisEvidence.staleComponents.join(', ')}` : ''}. Refresh it explicitly to analyze the current source revision.
                      </Alert>
                    ) : null}
                    {latestSnapshotAnalysisStatus(storedSnapshot.data) === 'missing' ? <Alert severity="info">No snapshot available for the current source revision.</Alert> : null}
                    {latestSnapshotAnalysisStatus(storedSnapshot.data) === 'unavailable' ? <Alert severity="warning">Analysis evidence is unavailable{storedSnapshot.data?.analysisEvidence?.unavailableReason ? `: ${storedSnapshot.data.analysisEvidence.unavailableReason}` : ''}.</Alert> : null}
                    {(latestSnapshotAnalysisStatus(storedSnapshot.data) === 'missing' || latestSnapshotAnalysisStatus(storedSnapshot.data) === 'stale' || (latestSnapshotAnalysisStatus(storedSnapshot.data) === 'unavailable' && storedSnapshot.data?.requiresAnalysis)) ? (
                      <Button variant="contained" onClick={refreshSnapshot} disabled={operationPending || asset.missing || !assetInfoCapability(asset, 'refreshAnalysis').allowed} sx={{ alignSelf: 'flex-start' }}>
                        {latestSnapshotAnalysisStatus(storedSnapshot.data) === 'missing' ? 'Analyze asset' : 'Refresh analysis'}
                      </Button>
                    ) : null}
                    {snapshotData ? <><MediaSnapshotDetails scan={snapshotData} /><DirectPlaySnapshotComparison scan={snapshotData} job={associatedJob} /></> : null}
                  </Stack>
                </Box> : null}
                {snapshotTab === 'tracks' ? <Box>
                  <Stack spacing={2} sx={{ pt: 1.5 }}>
                    {snapshotTab === 'tracks' ? <Box>
                    {snapshotData ? <><TrackMaintenancePanel path={asset.path} active={showSnapshotDialog && snapshotTab === 'tracks'} hasOpenJob={hasOpenJob} canManageTracks={assetInfoCapability(asset, 'manageTracks').allowed} canManageAttachments={assetInfoCapability(asset, 'manageAttachments').allowed} />
                    {!isConverted ? <><Typography variant="h3">Conversion track selection</Typography><Alert severity="info">Saving a manual selection creates an asset-level Tracks override. It does not modify the Track Profile, and Queue continues to use the backend-resolved Track Plan.</Alert><MediaSnapshotDetails
                      scan={snapshotData}
                      section="tracks"
                      streamControls={
                        isConverted
                          ? undefined
                          : {
                              video: {
                                selected: conversionStreamIndexes(conversionDraft, snapshotData, 'video'),
                                disabled: rowLocked || !assetInfoCapability(asset, 'manageTracks').allowed || saveAssetTracks.isPending,
                                onToggle: (index, keep) => toggleSnapshotStream('video', index, keep),
                              },
                              audio: {
                                selected: conversionStreamIndexes(conversionDraft, snapshotData, 'audio'),
                                disabled: rowLocked || !assetInfoCapability(asset, 'manageTracks').allowed || saveAssetTracks.isPending,
                                onToggle: (index, keep) => toggleSnapshotStream('audio', index, keep),
                              },
                              subtitle: {
                                selected: conversionStreamIndexes(conversionDraft, snapshotData, 'subtitle'),
                                disabled: rowLocked || !assetInfoCapability(asset, 'manageTracks').allowed || saveAssetTracks.isPending,
                                onToggle: (index, keep) => toggleSnapshotStream('subtitle', index, keep),
                              },
                            }
                      }
                      metadataControls={
                        isConverted
                          ? undefined
                          : {
                              video: {
                                values: conversionDraft.videoMetadata ?? {},
                                disabled: rowLocked || !assetInfoCapability(asset, 'manageTracks').allowed || saveAssetTracks.isPending,
                                onChange: (index, patch) => updateStreamMetadata('video', index, patch),
                              },
                              audio: {
                                values: conversionDraft.audioMetadata ?? {},
                                disabled: rowLocked || !assetInfoCapability(asset, 'manageTracks').allowed || saveAssetTracks.isPending,
                                onChange: (index, patch) => updateStreamMetadata('audio', index, patch),
                              },
                              subtitle: {
                                values: conversionDraft.subtitleMetadata ?? {},
                                disabled: rowLocked || !assetInfoCapability(asset, 'manageTracks').allowed || saveAssetTracks.isPending,
                                onChange: (index, patch) => updateStreamMetadata('subtitle', index, patch),
                              },
                            }
                      }
                    />
                    {snapshotData.subtitleStreams.length ? <Stack spacing={1.25}>
                      <Typography fontWeight={700}>Subtitle compatibility sidecars</Typography>
                      <Typography variant="body2" color="text.secondary">Optional per-asset formats. Inherit uses the effective Track Profile; Queue freezes the backend-resolved artifact list.</Typography>
                      {snapshotData.subtitleStreams.map((stream) => {
                        const formats = conversionDraft.subtitleSidecarFormatsByStream?.[String(stream.index)];
                        const value = formats?.length ? formats.join('+') : 'inherit';
                        const textCompatible = ['ass', 'ssa'].includes(stream.codec.toLowerCase());
                        return <TextField key={stream.index} select fullWidth size="small" label={`#${stream.index} · ${(stream.language || 'und').toUpperCase()} · ${stream.codec.toUpperCase()}`} value={value} disabled={rowLocked || !assetInfoCapability(asset, 'manageTracks').allowed || saveAssetTracks.isPending} onChange={(event) => updateSubtitleSidecarFormats(stream.index, event.target.value)}>
                          <MenuItem value="inherit">Inherit Track Profile</MenuItem>
                          <MenuItem value="original">Original format only</MenuItem>
                          {textCompatible ? <MenuItem value="srt">SRT compatibility only</MenuItem> : null}
                          {textCompatible ? <MenuItem value="original+srt">Original + SRT compatibility</MenuItem> : null}
                        </TextField>;
                      })}
                    </Stack> : null}
                    {snapshotData.subtitleStreams.length ? <Stack spacing={1.25}>
                      <Typography fontWeight={700}>Semantic subtitle roles</Typography>
                      <Typography variant="body2" color="text.secondary">Roles are resolved from the current snapshot. A manual choice is bound to this stream signature; reset returns to automatic classification.</Typography>
                      {snapshotData.subtitleStreams.map((stream) => {
                        const override = conversionDraft.subtitleRoleOverrides?.[String(stream.index)];
                        const roleValue = override?.role ?? 'auto';
                        const automatic = stream.role ?? 'unknown';
                        return <TextField key={`role-${stream.index}`} select fullWidth size="small" label={`#${stream.index} · ${(stream.language || 'und').toUpperCase()} · ${stream.codec.toUpperCase()}`} value={roleValue} disabled={rowLocked || !assetInfoCapability(asset, 'manageTracks').allowed || saveAssetTracks.isPending} onChange={(event) => updateSubtitleRole(stream.index, event.target.value as SubtitleSemanticRole | 'auto')} helperText={`Automatic: ${automatic} · ${stream.roleProvenance ?? 'snapshot'}${stream.roleConfidence ? ` · ${stream.roleConfidence} confidence` : ''}`}>
                          <MenuItem value="auto">Automatic ({automatic})</MenuItem>
                          {(['full', 'forced', 'signs', 'commentary', 'sdh', 'unknown'] as SubtitleSemanticRole[]).map((role) => <MenuItem key={role} value={role}>{role}</MenuItem>)}
                        </TextField>;
                      })}
                    </Stack> : null}
                    {!assetInfoCapability(asset, 'manageTracks').allowed ? <Alert severity="warning">{assetActionReasonMessage(assetInfoCapability(asset, 'manageTracks'), 'Track changes are unavailable.')}</Alert> : null}
                    <Stack direction="row" spacing={1} justifyContent="flex-end">
                      <Button onClick={resetTrackChanges} disabled={!tracksDirty || saveAssetTracks.isPending || saveAssetEdit.isPending || renameAsset.isPending}>Reset</Button>
                      <Button variant="contained" onClick={saveTrackChanges} disabled={!tracksDirty || rowLocked || !assetInfoCapability(asset, 'manageTracks').allowed || saveAssetTracks.isPending || saveAssetEdit.isPending || renameAsset.isPending || asset.missing}>Save Tracks</Button>
                    </Stack>
                    {saveAssetTracks.isError ? <Alert severity="warning">Could not save Tracks: {saveAssetTracks.error instanceof Error ? saveAssetTracks.error.message : 'unknown backend error'}</Alert> : null}
                    {saveAssetTracks.isSuccess ? <Alert severity="success">Tracks saved.</Alert> : null}
                    </> : null}
                    </> : <Alert severity="info">Track evidence is unavailable until a persisted Analysis snapshot exists.</Alert>}
                    </Box> : null}
                    {snapshotTab === 'tracks' ? <Box>
                    <Typography variant="h4">Subtitles</Typography>
                    {snapshotData ? <>
                    <EmbeddedSubtitleActions
                      streams={snapshotData.subtitleStreams}
                      eligibility={subtitleEligibility.data?.embedded}
                      generations={subtitleGenerations}
			 srtFallbackEnabled={srtFallbackEnabled}
                      onGenerate={generateExternalSubtitle}
                      onRegenerate={regenerateEmbeddedSubtitle}
                      onCancel={cancelExternalSubtitle}
                      disabled={hasOpenJob || !assetInfoCapability(asset, 'manageSubtitles').allowed}
                      translationDisabled={rowLocked || !assetInfoCapability(asset, 'manageSubtitles').allowed}
                      onTranslate={(streamIndex) => openTranslation({ type: 'embedded', embedded: { streamIndex } })}
                    />
                    <Typography variant="h4">Sidecars</Typography>
                    <>
                    <Typography variant="h5">Subtitle sidecars</Typography>
                    <ExternalSubtitleList
                        values={subtitleEligibility.data?.external ?? externalSubtitles.data ?? []}
                        loading={externalSubtitles.isLoading}
                        deleting={deleteExternalSubtitle.isPending}
                        renaming={renameExternalSubtitle.isPending}
                        disabled={hasOpenJob || !assetInfoCapability(asset, 'manageSubtitles').allowed}
                        translationDisabled={rowLocked || !assetInfoCapability(asset, 'manageSubtitles').allowed}
                        onTranslate={(subtitle) => openTranslation({ type: 'external', external: { subtitlePath: subtitle.path } })}
                        onAnalyze={analyzeExternalSubtitle}
                        qualityReports={subtitleQualityReports}
                        qualityMessages={subtitleQualityMessages}
                        restyleMessages={subtitleRestyleMessages}
                        restyleOperations={subtitleRestyleOperations}
                        onCancelRestyle={cancelExternalRestyle}
                        onRetryRestyle={retryExternalRestyle}
                        onApplyCorrection={applyExternalSubtitleCorrection}
                        onApplyCorrections={applyExternalSubtitleCorrections}
                        onRestyle={restyleExternalSubtitle}
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
                    <SubtitleCompositionEligibilityPanel
                      assetPath={asset.path}
                      eligibility={subtitleEligibility.data?.composition}
                      loading={subtitleEligibility.isLoading}
                      disabled={hasOpenJob || !assetInfoCapability(asset, 'manageSubtitles').allowed}
                      operation={subtitleCompositionOperation}
                      error={subtitleCompositionError}
                      onCompose={composeSubtitles}
                      onCancel={cancelSubtitleComposition}
                      onRetry={retrySubtitleComposition}
                    />
                    </>
                    {externalSubtitles.isError ? (
                      <Alert severity="warning">{externalSubtitles.error instanceof Error ? externalSubtitles.error.message : 'Could not list external subtitles.'}</Alert>
                    ) : null}
                    {deleteExternalSubtitle.isError ? <Alert severity="warning">{deleteExternalSubtitle.error instanceof Error ? deleteExternalSubtitle.error.message : 'Subtitle deletion failed.'}</Alert> : null}
                    </> : <Alert severity="info">Subtitle controls are unavailable until a persisted Analysis snapshot exists.</Alert>}
                    {!snapshotData ? <Typography variant="h4">Sidecars</Typography> : null}
                    <AssetMetadataSidecars values={metadataSidecars.data ?? []} loading={metadataSidecars.isLoading} />
                    {metadataSidecars.isError ? <Alert severity="warning">Could not list metadata sidecars: {metadataSidecars.error instanceof Error ? metadataSidecars.error.message : 'unknown backend error'}</Alert> : null}
                    </Box> : null}
                  </Stack>
                </Box> : null}
                <Box hidden={snapshotTab !== 'overview'}>
                  <Stack spacing={2} sx={{ pt: 1.5 }}>
                    <AssetInfoOverview asset={asset} libraries={libraries} status={pipelineState.label} />
                    <Grid container spacing={1.5} alignItems="flex-start">
                      <Grid size={{ xs: 12, md: 6 }}>
                        <Stack direction="row" spacing={1} alignItems="flex-start">
                          <TextField size="small" label="Rename file" value={renameFileName} onChange={(event) => setRenameFileName(event.target.value)} disabled={renameLocked || !renameCapability.allowed || asset.missing || renameAsset.isPending || saveAssetEdit.isPending} fullWidth helperText={renameCapability.allowed ? 'File name only; the asset remains in its current folder.' : assetActionReasonMessage(renameCapability, 'Rename unavailable.')} />
                          <Button size="small" variant="contained" onClick={() => renameAsset.mutate({ path: asset.path, fileName: renameFileName })} disabled={ renameLocked || asset.missing || renameAsset.isPending || !renameFileName.trim() || renameFileName.trim() === asset.fileName
                            || saveAssetEdit.isPending || !renameCapability.allowed
                          }>Rename</Button>
                        </Stack>
                      </Grid>
                      <Grid size={{ xs: 12, md: 6 }}>
                        <AssetCategorySelect value={metadataCategory} options={assetCategories} onChange={saveAssetCategory} label="Category" size="small" disabled={rowLocked || asset.missing} />
                      </Grid>
                    </Grid>
                    {renameAsset.isError ? <Alert severity="warning">{renameAsset.error.message}</Alert> : null}
                    {mode === 'unprocessed' ? (
                      <Stack spacing={1.5}>
                        <Typography variant="h4">Asset configuration</Typography>
                        <Grid container spacing={1.5}>
                          <Grid size={{ xs: 12, sm: 6 }}>
                            <ProfileAutocomplete
                              profiles={assetVideoProfiles}
                              value={selectedProfileId}
                              onChange={selectAssetVideoProfile}
                              label="Video profile"
                              size="small"
                              allowInherit
                              allowOverrideOnly
                              allowAudioOnly
                              disabled={rowLocked || asset.missing || updateProfileAssignment.isPending || updateAssetDestination.isPending}
                            />
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6 }}>
                            <AudioProfileAutocomplete
                              profiles={assetAudioProfiles}
                              value={selectedAudioProfileKey}
                              onChange={selectAssetAudioProfile}
                              label="Audio profile"
                              size="small"
                              allowInherit
                              disabled={rowLocked || asset.missing || updateProfileAssignment.isPending || updateAssetDestination.isPending}
                            />
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6 }}>
                            <TrackProfileAutocomplete
                              profiles={assetTrackProfiles}
                              value={selectedTrackProfileKey}
                              onChange={selectAssetTrackProfile}
                              label="Tracks profile"
                              allowInherit
                              disabled={rowLocked || asset.missing || updateProfileAssignment.isPending || updateAssetDestination.isPending}
                            />
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6 }}>
                            <TextField
                              select
                              fullWidth
                              size="small"
                              label="Destination mode"
                              value={assetDestinationMode}
                              onChange={(event) => selectAssetDestinationMode(event.target.value as 'inherit' | 'value' | 'disabled')}
                              disabled={rowLocked || asset.missing || assetScopeConfigurations.isLoading || assetScopeConfigurations.isError || updateProfileAssignment.isPending || updateAssetDestination.isPending}
                            >
                              <MenuItem value="inherit">Inherit</MenuItem>
                              <MenuItem value="value">Override</MenuItem>
                              <MenuItem value="disabled">Disabled</MenuItem>
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6 }}>
                            <LibraryAutocomplete
                              libraries={libraries}
                              value={assetDestinationLibraryId}
                              onChange={selectAssetDestination}
                              label="Destination"
                              size="small"
                              disabled={rowLocked || asset.missing || assetDestinationMode !== 'value' || assetScopeConfigurations.isLoading || assetScopeConfigurations.isError || updateProfileAssignment.isPending || updateAssetDestination.isPending}
                            />
                          </Grid>
                        </Grid>
                        {assetScopeConfigurations.isError ? <Alert severity="warning">Asset Destination configuration could not be loaded. Retry by reopening Asset Info.</Alert> : null}
                        {updateAssetDestination.isError ? <Alert severity="warning">{updateAssetDestination.error instanceof Error ? updateAssetDestination.error.message : 'Could not save the asset Destination.'}</Alert> : null}
                      </Stack>
                    ) : null}
                    {mode === 'unprocessed' ? <Box>
                      <Typography variant="caption" color="text.secondary">Destination applied{assetDestinationMode === 'inherit' ? ' · inherited' : assetDestinationMode === 'value' ? ' · asset override' : ''}</Typography>
                      <Typography>{assetDestinationMode === 'disabled' ? 'Disabled' : libraries.find((library) => library.id === selectedLibraryId)?.name || 'None'}</Typography>
                    </Box> : null}
                  </Stack>
                </Box>
                {snapshotTab === 'execution' ? <Box>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    <Typography variant="h3">Execution / Result</Typography>
                    {associatedJob ? (
                      <FinalDetailsSummary asset={asset} job={associatedJob} />
                    ) : historyJobs.length ? (
                      <Alert severity="info">No current execution is linked. See History for linked executions.</Alert>
                    ) : (
                      <Alert severity="info">No MVForge processing history is linked to this asset.</Alert>
                    )}
                    {associatedJob ? <Grid container spacing={1.5}>
                      <Grid size={{ xs: 12, sm: 6 }}>
                        <Typography variant="caption" color="text.secondary">Video profile</Typography>
                        <Typography>{profiles.find((profile) => profile.id === associatedJob.profileId)?.name || 'Not recorded'}</Typography>
                      </Grid>
                      <Grid size={{ xs: 12, sm: 6 }}>
                        <Typography variant="caption" color="text.secondary">Audio profile</Typography>
                        <Typography>{associatedJob.audioProfileKey || 'Not recorded'}</Typography>
                      </Grid>
                      <Grid size={{ xs: 12, sm: 6 }}>
                        <Typography variant="caption" color="text.secondary">Tracks profile</Typography>
                        <Typography>{associatedJob.trackProfileKey || 'Not recorded'}</Typography>
                      </Grid>
                    </Grid> : null}
                  </Stack>
                </Box> : null}
                {snapshotTab === 'publication' ? <Box>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    <Typography variant="h3">Current publication</Typography>
                    {currentPublication?.state === 'conflict' || currentPublication?.state === 'unresolved' || currentPublication?.state === 'unresolved_conflict' ? (
                      <Alert severity="warning">Current publication is unresolved because canonical publication authorities conflict. Recovery and destructive actions are blocked.</Alert>
                    ) : (
                      <Typography>{lifecyclePresentationValue(currentPublication?.state || asset.lifecycle?.publication, 'No current publication')}</Typography>
                    )}
                    {currentPublication?.authority ? <Typography variant="body2" color="text.secondary">Authority · {currentPublication.authority}</Typography> : null}
                    {currentPublication?.id !== undefined ? <Typography variant="body2" color="text.secondary">Identity · {currentPublication.authority === 'queue_job' ? `Queue job #${currentPublication.id}` : `Publication #${currentPublication.id}`}</Typography> : null}
                    {currentPublication?.publishedPath ? <Typography variant="body2" color="text.secondary" sx={{ wordBreak: 'break-all' }}>{currentPublication.publishedPath}</Typography> : null}
                    {currentPublication?.publishedAt ? <Typography variant="body2" color="text.secondary">Published {formatDate(currentPublication.publishedAt)}</Typography> : null}
                  </Stack>
                </Box> : null}
                {snapshotTab === 'validation' ? <Box>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    <Typography variant="h3">Validation</Typography>
                    {validationEvidence.kind === 'canonical' ? <>
                      <Typography>Outcome · {validationEvidence.outcomeLabel}</Typography>
                      <Typography variant="body2" color="text.secondary">Validation run · {validationEvidence.runId}</Typography>
                      <Typography variant="body2" color="text.secondary">Override present · {validationEvidence.overridePresent === undefined ? 'Not recorded' : validationEvidence.overridePresent ? 'Yes' : 'No'}</Typography>
                      <Typography variant="body2" color="text.secondary">Actionable warnings · {validationEvidence.hasActionableWarnings === undefined ? 'Not recorded' : validationEvidence.hasActionableWarnings ? 'Yes' : 'No'}</Typography>
                    </> : validationEvidence.kind === 'legacy' ? (
                      <Typography>Legacy/unbound validation status · {validationEvidence.statusLabel}</Typography>
                    ) : <Typography>No validation evidence</Typography>}
                    {validationEvidence.score ? <Typography variant="body2" color="text.secondary">Latest score · {validationEvidence.score}/100</Typography> : null}
                    {associatedJob ? <FinalValidationEvidence job={associatedJob} /> : null}
                  </Stack>
                </Box> : null}
                {snapshotTab === 'original' ? <Box>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    <Typography variant="h3">Original retention</Typography>
                    <Typography>{lifecyclePresentationValue(asset.lifecycle?.original, 'unknown')}</Typography>
                    {associatedJob ? <>
                      <Typography>Original archive · {originalArchiveStatusLabel(associatedJob.originalArchiveStatus)}</Typography>
                      {associatedJob.originalArchiveStatus === 'failed' && associatedJob.publicationSummary ? <Alert severity="error">{associatedJob.publicationSummary}</Alert> : null}
                    </> : null}
                    {asset.lifecycle?.archivedOriginals?.length ? asset.lifecycle.archivedOriginals.map((original) => (
                      <ArchivedOriginalDetails
                        key={`${original.queueJobId}-${original.assetRecordId ?? original.path}`}
                        original={original}
                        onRecover={recoverArchivedAsset}
                        onDelete={requestArchiveDeletion}
                        recovering={recoverAsset.isPending}
                        deleting={deleteArchiveAsset.isPending}
                        canMutate={Boolean(archivedOriginalMutationInput(original))}
                      />
                    )) : <Alert severity="info">No archived originals linked to this asset.</Alert>}
                    <Alert severity="info">Restore leaves the current Library publication intact. Deletion preserves Queue and publication provenance.</Alert>
                  </Stack>
                </Box> : null}
                {snapshotTab === 'evidence' ? <Box>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    <Typography variant="h3">Evidence</Typography>
                    <Typography color="text.secondary">Reports, frozen plans, validation evidence, logs, snapshots, and maintenance records remain in their existing stores and are loaded on demand.</Typography>
                    {historyJobs.length ? historyJobs.map((job) => <Stack key={job.id} direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={1}>
                      <Typography>Queue job #{job.id} · {job.status}{job.publicationRetiredAt ? ' · historical' : ''}</Typography>
                      <Button size="small" variant="outlined" onClick={() => setSelectedEvidenceJobId(job.id)}>Load report summary</Button>
                    </Stack>) : <Typography color="text.secondary">No linked job evidence.</Typography>}
                    {asset.lifecycle?.evidenceSummary?.directPublicationIds?.map((id) => <Typography key={id}>AS-IS publication #{id} · summary available from canonical publication evidence</Typography>)}
                    {selectedEvidenceJobId !== null ? <Box>
                      {evidenceArtifacts.isLoading ? <LinearProgress /> : null}
                      {evidenceArtifacts.isError ? <Alert severity="warning">Could not load evidence for Queue job #{selectedEvidenceJobId}.</Alert> : null}
                      {evidenceArtifacts.data ? <Typography variant="body2">Queue job #{selectedEvidenceJobId} evidence loaded: {evidenceArtifacts.data.resultPath || evidenceArtifacts.data.asIsPath || 'body available on demand'}.</Typography> : null}
                      {evidencePlans.isLoading ? <LinearProgress /> : null}
                      {evidencePlans.data?.map((plan) => <Typography key={plan.id} variant="body2">Frozen Queue plan v{plan.version} · {plan.status} · {plan.selectedEncoder || 'encoder pending'}</Typography>)}
                    </Box> : null}
                  </Stack>
                </Box> : null}
                {snapshotTab === 'history' ? <Box>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    <Typography variant="h3">History</Typography>
                    {historyJobs.length ? historyJobs.map((job) => {
                      const frozenEvidence = job.profileSnapshot.sourceAnalysisEvidence as {
                        status?: string;
                        sourceAssetRecordId?: number;
                        sourceIdentity?: { path?: string; snapshotId?: number; matchesCurrentSourceRevision?: boolean };
                      } | undefined;
                      const evidenceIsLinked = job.sourceAssetRecordId === asset.id && frozenEvidence?.sourceAssetRecordId === asset.id;
                      return <Box key={job.id}>
                        <Typography>Queue result #{job.id} · {job.publishedPath || job.outputPath || job.mediaPath}{asset.lifecycle?.currentPublication?.authority === 'queue_job' && job.id === asset.lifecycle.currentPublication.id ? ' · current' : ' · historical'}</Typography>
                        {evidenceIsLinked ? <Typography variant="body2" color="text.secondary">
                          Source evidence {frozenEvidence?.sourceIdentity?.snapshotId ? `snapshot #${frozenEvidence.sourceIdentity.snapshotId}` : 'unavailable'} · {analysisEvidenceLabel(frozenEvidence?.status)} · {frozenEvidence?.sourceIdentity?.path || 'source identity unavailable'} → frozen plan {job.activeExecutionPlanId ? `#${job.activeExecutionPlanId}` : 'not available'} → Queue result #{job.id}
                        </Typography> : <Typography variant="body2" color="text.secondary">Source evidence not linked.</Typography>}
                      </Box>;
                    }) : null}
                    {asset.lifecycle?.evidenceSummary?.directPublicationIds?.map((id) => <Typography key={id}>Direct publication #{id} · historical/current distinction remains backend-resolved</Typography>)}
                    {legacyAnalysisRecords.map((record, index) => <Alert key={String(record.id ?? index)} severity="info"><strong>Legacy Analysis decision:</strong> {String(record.decision ?? 'No decision')} · {String(record.createdAt ?? '')}{record.notes ? ` · ${String(record.notes)}` : ''}</Alert>)}
                    {!historyJobs.length && !asset.lifecycle?.evidenceSummary?.directPublicationIds?.length && !legacyAnalysisRecords.length ? <Alert severity="info">No MVForge processing history is linked to this asset.</Alert> : null}
                  </Stack>
                </Box> : null}
                {snapshotTab === 'integration' ? <Box>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    <Typography variant="h3">Integration evidence</Typography>
                    <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                      <Chip size="small" label={`Analysis · ${analysisEvidenceLabel(asset.lifecycle?.evidenceSummary?.analysis?.status)}`} />
                      <Chip size="small" label={`Validation · ${validationIntegrationState}`} />
                      <Chip size="small" label={`Publication · ${publicationIntegrationState}`} />
                      {(asset.lifecycle?.evidenceSummary?.queueJobIds?.length ?? 0) > 0 ? <Chip size="small" label={`${asset.lifecycle?.evidenceSummary?.queueJobIds?.length} Queue jobs`} /> : null}
                      {(asset.lifecycle?.evidenceSummary?.directPublicationIds?.length ?? 0) > 0 ? <Chip size="small" label={`${asset.lifecycle?.evidenceSummary?.directPublicationIds?.length} direct publications`} /> : null}
                      {(asset.lifecycle?.archivedOriginals?.length ?? 0) > 0 ? <Chip size="small" label={`${asset.lifecycle?.archivedOriginals?.length} archived originals`} /> : null}
                    </Stack>
                    {!asset.lifecycle?.evidenceSummary?.analysis && validationEvidence.kind === 'unavailable' && !currentPublication && !asset.lifecycle?.evidenceSummary?.queueJobIds?.length && !asset.lifecycle?.evidenceSummary?.directPublicationIds?.length && !asset.lifecycle?.archivedOriginals?.length ? <Typography color="text.secondary">No linked integration evidence.</Typography> : null}
                  </Stack>
                </Box> : null}
                {snapshotTab === 'advisor' && !hidePreConversionActions ? <Box>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    {!isConverted && profileSuggestion.isPending ? <Alert severity="info">Comparing the snapshot with available profiles…</Alert> : null}
                    {!isConverted && profileSuggestion.isError ? (
                      <Alert severity="warning">The snapshot was created, but a profile could not be suggested: {profileSuggestion.error instanceof Error ? profileSuggestion.error.message : 'unknown backend error'}</Alert>
                    ) : null}
                    {!isConverted && profileSuggestion.data ? (
                      <ProfileSuggestionCard
                        suggestion={profileSuggestion.data}
                        selectionPending={updateProfileAssignment.isPending}
                        onSelect={canAssignProfile ? (profile) => {
                          if (profile.scope !== 'asset' || profile.disabled || profile.deletedAt) {
                            setAssetEditQueueMessage('This suggested profile cannot be assigned at Asset scope. Choose an enabled Asset profile, or configure a Path profile from Configure Path.');
                            return;
                          }
                          setAssetEditQueueMessage('');
                          selectAssetVideoProfile(profile.id);
                        } : undefined}
                        onApplyRecommendations={
                          hasOpenJob ? undefined : applySnapshotRecommendations
                        }
                        onReviewInLab={openAssetInLab}
                      />
                    ) : null}
                    {isConverted ? <Alert severity="info">This is already a converted asset. Re-processing recommendations should be evaluated from its archived original.</Alert> : null}
                  </Stack>
                </Box> : null}
                {snapshotTab === 'conversion' ? <Box>
                  <Stack spacing={1.5} sx={{ pt: 1.5 }}>
                    {
                      <AssetConversionOverridePanel
                        assetPath={asset.path}
                        draft={conversionDraft}
                        profile={profiles.find((profile) => profile.id === effectiveProfileId)}
                        scan={snapshotData ?? undefined}
                        onChange={updateConversionDraft}
                        onChangeMany={(patch) => setConversionDraft((current) => ({ ...current, ...patch }))}
                        onHydrateMany={hydrateAssetEditDefaults}
                        onSave={saveConversionOverrides}
                        onReset={resetConversionOverrides}
                        saving={saveAssetEdit.isPending || saveAssetTracks.isPending || renameAsset.isPending}
                        readOnly={isConverted || hasOpenJob}
                      />
                    }
                  </Stack>
                </Box> : null}
                {snapshotTab === 'test-encode' && canGenerateTestEncode && !hidePreConversionActions ? <Box>
                  <Stack spacing={2} sx={{ pt: 1.5 }}>
                    <Stack spacing={0.5}>
                      <Typography variant="h3">Test Encode</Typography>
                      <Typography color="text.secondary" variant="body2">
                        Generate a short real encode with the persisted effective pipeline that this asset would use in Queue. Unsaved Asset Override changes are not included. The original is not archived or modified.
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
                          disabled={asset.missing || hasOpenJob || updateProfileAssignment.isPending || !labCapability.allowed}
                          allowInherit
                        />
                      </Grid>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Audio profile</Typography><Typography>{effectiveAudioProfileKey || 'None'}</Typography></Grid>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Tracks profile</Typography><Typography>{selectedTrackProfile?.name || selectedTrackProfile?.key || conversionDraft.trackProfileKey || 'None'}</Typography></Grid>
                      <Grid size={{ xs: 12, sm: 6 }}><Typography variant="caption" color="text.secondary">Destination Library</Typography><Typography>{libraries.find((library) => library.id === selectedLibraryId)?.name || 'Choose when generating'}</Typography></Grid>
                    </Grid>
                    {!canGenerateTestEncode ? (
                      <Alert severity="info">{labCapability.allowed ? 'Generate Test Encode is unavailable for this asset.' : `Test Encode unavailable: ${assetActionReasonMessage(labCapability, 'Test Encode is unavailable for this asset.')}`}</Alert>
                    ) : (
                      <Tooltip title={testConfigurationReason || (hasOpenJob ? 'Asset has an active Queue job' : updateProfileAssignment.isPending ? 'Saving the current asset configuration' : 'Generate a short real encode from the persisted effective asset configuration')}>
                        <span style={{ alignSelf: 'flex-start' }}>
                          <Button
                            startIcon={<ScienceIcon />}
                            variant="contained"
                            color="secondary"
                            onClick={openAssetTestEncode}
                            disabled={asset.missing || hasOpenJob || Boolean(testConfigurationReason) || updateProfileAssignment.isPending || !labCapability.allowed}
                          >
                            Generate Test Encode
                          </Button>
                        </span>
                      </Tooltip>
                    )}
                  </Stack>
                </Box> : null}
              </>
            {saveAssetEdit.isSuccess ? <Alert severity="success">Asset Info saved.</Alert> : null}
            {assetEditQueueMessage ? <Alert severity="warning">{assetEditQueueMessage}</Alert> : null}
            {saveAssetEdit.isError ? (
              <Alert severity="warning">Could not save Asset Info: {saveAssetEdit.error instanceof Error ? saveAssetEdit.error.message : 'Unknown error'}</Alert>
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
             disabled={
                hasOpenJob ||
                loadSubtitleContent.isPending ||
                loadSubtitleContent.isError
              }
              inputProps={{ spellCheck: false }}
            />
            <Stack direction="row" justifyContent="flex-end" spacing={1}>
              <Button onClick={() => setEditingSubtitle(null)}>Cancel</Button>
              <Button
                variant="contained"
                disabled={
                  hasOpenJob ||
                  !editingSubtitle ||
                  !subtitleContent.trim() ||
                  saveSubtitleContent.isPending
                }
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
              disabled={hasOpenJob || !assetInfoCapability(asset, 'manageSubtitles').allowed}
            />
            {renameExternalSubtitle.isError ? <Alert severity="warning">{renameExternalSubtitle.error instanceof Error ? renameExternalSubtitle.error.message : 'Subtitle rename failed.'}</Alert> : null}
            <Stack direction="row" spacing={1} justifyContent="flex-end">
              <Button onClick={() => setRenamingSubtitle(null)}>Cancel</Button>
              <Button
                variant="contained"
                disabled={
                  hasOpenJob ||
                  !renamingSubtitle ||
                  renameExternalSubtitle.isPending ||
                  !subtitleFileName.trim() ||
                  subtitleFileName.trim() === renamingSubtitle.fileName
                }
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
      {translationSource ? (
        <SubtitleTranslationDialog
          key={`${asset.path}:${translationSource.type === 'embedded' ? `embedded:${translationSource.embedded.streamIndex}` : `external:${translationSource.external.subtitlePath}`}`}
          open={translationDialogOpen}
          path={asset.path}
          source={translationSource}
          onClose={() => setTranslationDialogOpen(false)}
          onReviewOCR={reviewTranslationOCR}
        />
      ) : null}
      <SubtitleOCRReviewDialog
        open={translationReviewDialogOpen}
        mediaPath={asset.path}
        target={translationReviewTarget}
        onClose={() => {
          setTranslationReviewDialogOpen(false);
          if (translationSource) setTranslationDialogOpen(true);
        }}
        onSaved={() => {
          setTranslationReviewDialogOpen(false);
          void (async () => {
            await refreshSubtitleEvidence();
            if (translationSource) setTranslationDialogOpen(true);
          })();
        }}
      />
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
        <SourceBitrateQualityEvidence source={advisor.sourceBitrateEvidence ?? advisor.scan.sourceBitrateEvidence} assessment={advisor.bpppfAssessment} />
      </Stack>
    </Box>
  );
}

export function EmbeddedSubtitleActions({
  streams,
  eligibility,
  generations,
  onGenerate,
  onRegenerate,
  onCancel,
  onTranslate,
  srtFallbackEnabled = false,
  disabled = false,
  translationDisabled = false,
}: {
  streams: MediaStreamInfo[];
  eligibility?: SubtitleSourceEligibility[];
  generations: Record<string, SubtitleGenerationState>;
  onGenerate: (
    streamIndex: number,
    format: 'original' | 'srt' | 'ass',
    ocrLanguage?: string,
    ocrMode?: 'raw' | 'clean' | 'accurate',
  ) => void;
  onRegenerate?: (streamIndex: number) => void;
  onCancel: (
    streamIndex: number,
    format: 'original' | 'srt' | 'ass',
  ) => void;
  onTranslate?: (streamIndex: number) => void;
  srtFallbackEnabled?: boolean;
  disabled?: boolean;
  translationDisabled?: boolean;
}) {
  const [ocrLanguages, setOcrLanguages] = useState<Record<number, string>>({});
  const [ocrModes, setOcrModes] = useState<Record<number, 'raw' | 'clean' | 'accurate'>>({});
  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
      <Stack spacing={1.25}>
        <Stack>
          <Typography variant="h3">Generate external subtitles</Typography>
          <Typography color="text.secondary" variant="body2">Create an SRT or ASS sidecar through the canonical subtitle pipeline. Bitmap tracks use OCR and produce neutral recognized text rather than preserving the source image styling; the embedded track is not removed.</Typography>
          {srtFallbackEnabled ? <Typography color="info.main" variant="caption">ASS + SRT compatibility fallback is enabled; selecting ASS also produces converted SRT.</Typography> : null}
        </Stack>
        {streams.length === 0 ? <Alert severity="info">This asset has no embedded subtitle tracks.</Alert> : null}
        {streams.map((stream) => {
          const bitmap = isBitmapSubtitleCodec(stream.codec);
          const sourceEligibility = eligibility?.find((value) => value.streamIndex === stream.index);
          const regenerateAllowed = sourceEligibility ? sourceEligibility.regenerate.eligible : true;
           const originalStyledFormat =
            stream.codec.toLowerCase() === 'ssa'
              ? 'SSA'
              : stream.codec.toLowerCase() === 'ass'
                ? 'ASS'
                : null;
          const streamGenerations = (['original', 'srt', 'ass'] as const)
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
                  {sourceEligibility ? <Typography color="text.secondary" variant="caption" sx={{ overflowWrap: 'anywhere' }}>Identity: {sourceEligibility.sourceIdentity} · Revision: {sourceEligibility.sourceRevision || 'unavailable'} · Role: {sourceEligibility.role} ({sourceEligibility.roleProvenance})</Typography> : null}
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
                      disabled={disabled}
                      sx={{ minWidth: 130 }}
                    >
                      <MenuItem value="eng">English</MenuItem>
                      <MenuItem value="spa">Spanish</MenuItem>
                      <MenuItem value="jpn">Japanese</MenuItem>
                      <MenuItem value="jpn_vert">Japanese vertical</MenuItem>
                      <MenuItem value="auto">Automatic</MenuItem>
                    </TextField>
                  ) : null}
                  {bitmap ? (
                    <TextField
                      select
                      size="small"
                      label="OCR quality"
                      value={ocrModes[stream.index] || 'accurate'}
                      onChange={(event) =>
                        setOcrModes((current) => ({
                          ...current,
                          [stream.index]: event.target.value as 'raw' | 'clean' | 'accurate',
                        }))
                      }
                      disabled={disabled}
                      sx={{ minWidth: 155 }}
                    >
                      <MenuItem value="raw">Raw</MenuItem>
                      <MenuItem value="clean">Clean</MenuItem>
                      <MenuItem value="accurate">Accurate</MenuItem>
                    </TextField>
                  ) : null}
                  <Button size="small" variant="outlined" disabled={ disabled || generations[subtitleGenerationKey(stream.index, 'srt')]?.status === 'running' } onClick={() => onGenerate(stream.index, 'srt', bitmap ? ocrLanguages[stream.index] : undefined, bitmap ? ocrModes[stream.index] : undefined)}>Generate SRT</Button>
                  <Button size="small" variant="outlined" disabled={disabled || generations[subtitleGenerationKey(stream.index, 'ass')]?.status === 'running'} onClick={() => onGenerate(stream.index, 'ass', bitmap ? ocrLanguages[stream.index] : undefined, bitmap ? ocrModes[stream.index] : undefined)}>Generate ASS</Button>
                  {onRegenerate && regenerateAllowed ? <Button size="small" variant="outlined" disabled={disabled || generations[subtitleGenerationKey(stream.index, 'ass')]?.status === 'running'} onClick={() => onRegenerate(stream.index)}>Regenerate styled ASS</Button> : null}
                  {onTranslate ? <Button size="small" variant="outlined" aria-label={`Translate subtitle stream ${stream.index}`} disabled={translationDisabled} onClick={() => onTranslate(stream.index)}>Translate...</Button> : null}
                  {originalStyledFormat ? (
                    <Button
                      size="small"
                      variant="outlined"
                      disabled={
                        disabled ||
                        generations[subtitleGenerationKey(stream.index, 'original')]?.status === 'running'
                      }
                      onClick={() => onGenerate(stream.index, 'original')}
                    >
                      Extract {originalStyledFormat}
                    </Button>
                  ) : null}
                </Stack>
              </Stack>
              {sourceEligibility && !sourceEligibility.regenerate.eligible ? <Typography variant="caption" color="text.secondary">{sourceEligibility.regenerate.reason || 'Regeneration is unavailable for this source.'}</Typography> : null}
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
                                  : generation.phase === 'analyzing'
                                    ? 'Analyzing OCR quality…'
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
                      <Button
                        size="small"
                        color="warning"
                        variant="outlined"
                        disabled={disabled || generation.cancelPending}
                        onClick={() => onCancel(generation.streamIndex, generation.format)}
                      >
                        {generation.cancelPending ? 'Cancelling…' : 'Cancel'}
                      </Button>
                    </Stack>
                  ) : (
                    <Alert severity={generation.status === 'success' ? 'success' : generation.status === 'cancelled' ? 'info' : 'warning'} sx={{ py: 0 }}>
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
  status: 'running' | 'success' | 'cancelled' | 'error';
  streamIndex: number;
  format: 'original' | 'srt' | 'ass';
  operationId?: string;
  cancelPending?: boolean;
  message?: string;

  progress?: number;
  phase?:
    | 'preparing'
    | 'translating'
    | 'validating'
    | 'analyzing'
    | 'extracting'
    | 'ocr'
    | 'ocr_review'
    | 'reconciliation'
    | 'cleanup'
    | 'publishing'
    | 'completed'
    | 'cancelled'
    | 'error';
  processed?: number;
  total?: number;
};

type SubtitleRestyleOperationState = {
  operationId?: string;
  status: 'running' | 'completed' | 'failed' | 'cancelled';
  progress: number;
  phase?: string;
  message?: string;
  error?: string;
  retryable?: boolean;
};

function subtitleGenerationKey(streamIndex: number, format:  'original' | 'srt' | 'ass') {
  return `${streamIndex}:${format}`;
}

function defaultOCRLanguage(language: string) {
  const value = (language || '').trim().toLowerCase();
  if (value === 'spa' || value === 'es' || value === 'esp') return 'spa';
  if (value === 'jpn' || value === 'ja' || value === 'jp') return 'jpn';
  return 'eng';
}

export function ExternalSubtitleList({
  values,
  loading,
  deleting,
  renaming,
  disabled,
  translationDisabled = false,
  onRename,
  onEdit,
  onDelete,
  onTranslate,
  onAnalyze,
  qualityReports = {},
  qualityMessages = {},
  restyleMessages = {},
  restyleOperations = {},
  onCancelRestyle,
  onRetryRestyle,
  onApplyCorrection,
  onApplyCorrections,
  onRestyle,
}: {
  values: ExternalSubtitle[];
  loading: boolean;
  deleting: boolean;
  renaming: boolean;
  disabled?: boolean;
  translationDisabled?: boolean;
  onRename: (subtitle: ExternalSubtitle) => void;
  onEdit: (subtitle: ExternalSubtitle) => void;
  onDelete: (subtitle: ExternalSubtitle) => void;
  onTranslate?: (subtitle: ExternalSubtitle) => void;
  onAnalyze?: (subtitle: ExternalSubtitle) => void;
  qualityReports?: Record<string, SubtitleOCRQualityReport>;
  qualityMessages?: Record<string, string>;
  restyleMessages?: Record<string, string>;
  restyleOperations?: Record<string, SubtitleRestyleOperationState>;
  onCancelRestyle?: (subtitle: ExternalSubtitle) => void;
  onRetryRestyle?: (subtitle: ExternalSubtitle) => void;
  onApplyCorrection?: (subtitle: ExternalSubtitle, report: SubtitleOCRQualityReport, finding: SubtitleOCRQualityFinding, text: string) => void;
  onApplyCorrections?: (subtitle: ExternalSubtitle, report: SubtitleOCRQualityReport, corrections: Array<{ cueId: number; suggestion: string }>) => void;
  onRestyle?: (subtitle: ExternalSubtitle, mode: 'restyle' | 'srt_to_ass', options?: { replace?: boolean; overrideExistingASSStyles?: boolean }) => void;
}) {
  const [correctionText, setCorrectionText] = useState<Record<string, string>>({});
  const [restyleTarget, setRestyleTarget] = useState<{ subtitle: ExternalSubtitle; mode: 'restyle' | 'srt_to_ass' } | null>(null);
  const [replace, setReplace] = useState(false);
  const [overrideExistingASSStyles, setOverrideExistingASSStyles] = useState(false);
  const openRestyleDialog = (subtitle: ExternalSubtitle, mode: 'restyle' | 'srt_to_ass') => {
    setReplace(false);
    setOverrideExistingASSStyles(false);
    setRestyleTarget({ subtitle, mode });
  };
  return (
    <>
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
      <Stack spacing={1.25}>
        <Typography variant="h3">External SRT / ASS files</Typography>
        {loading ? <Alert severity="info">Reading external subtitles…</Alert> : null}
        {!loading && values.length === 0 ? <Alert severity="info">No external SRT or ASS sidecars were found beside this asset.</Alert> : null}
        {values.map((subtitle) => {
          const reviewKey = subtitleOCRReviewKey(subtitle);
          const report = qualityReports[reviewKey] ?? qualityReports[subtitle.path];
          const message = qualityMessages[reviewKey] ?? qualityMessages[subtitle.path];
          const eligibility = subtitle.eligibility;
          const restyleAllowed = eligibility ? eligibility.restyle.eligible : true;
          const srtToAssAllowed = eligibility ? eligibility.srtToAss.eligible : true;
          const restyleOperation = restyleOperations[subtitle.path];
          const logicalSiblings = subtitle.logicalTransformKey
            ? values.filter((candidate) => candidate.logicalTransformKey === subtitle.logicalTransformKey)
            : [];
          const reportOwner = logicalSiblings.length > 1
            ? [...logicalSiblings].sort((left, right) => left.path.localeCompare(right.path))[0]?.path
            : subtitle.path;
          const showReport = Boolean(report) && subtitle.path === reportOwner;
          return (
          <Stack
            key={subtitle.path}
            spacing={1}
            sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1 }}
          >
            <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={1}>
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
                {onRestyle && restyleAllowed ? <Button size="small" variant="outlined" disabled={disabled || restyleOperation?.status === 'running'} onClick={() => openRestyleDialog(subtitle, 'restyle')}>Restyle</Button> : null}
                {onRestyle && srtToAssAllowed ? <Button size="small" variant="outlined" disabled={disabled || restyleOperation?.status === 'running'} onClick={() => openRestyleDialog(subtitle, 'srt_to_ass')}>Generate ASS</Button> : null}
                {onAnalyze && subtitle.ocrAnalysisAvailable ? <Button size="small" variant="outlined" disabled={disabled} onClick={() => onAnalyze(subtitle)}>Analyze OCR</Button> : null}
                {onTranslate ? <Button size="small" variant="outlined" aria-label={`Translate ${subtitle.fileName}`} disabled={translationDisabled} onClick={() => onTranslate(subtitle)}>Translate...</Button> : null}
                <Tooltip title="Rename subtitle file">
                  <IconButton size="small" color="primary" disabled={disabled || renaming} onClick={() => onRename(subtitle)} ><DriveFileRenameOutlineIcon /></IconButton>
                </Tooltip>
                <Tooltip title="Edit subtitle text">
                  <IconButton size="small" color="primary" disabled={disabled} onClick={() => onEdit(subtitle)} ><EditIcon /></IconButton>
                </Tooltip>
                <Tooltip title="Delete external subtitle">
                  <IconButton size="small" color="error" disabled={disabled || deleting} onClick={() => onDelete(subtitle)}><DeleteForeverIcon /></IconButton>
                </Tooltip>
              </Stack>
            </Stack>
            {message && !showReport ? <Alert severity="info" sx={{ py: 0 }}>{message}</Alert> : null}
            {restyleMessages[subtitle.path] ? <Alert severity="info" sx={{ py: 0 }}>{restyleMessages[subtitle.path]}</Alert> : null}
            {eligibility && !restyleAllowed && !srtToAssAllowed ? <Typography variant="caption" color="text.secondary">{eligibility.restyle.reason || eligibility.srtToAss.reason || eligibility.regenerate.reason || 'No subtitle styling operation is available for this source.'}</Typography> : null}
            {restyleOperation ? <Box sx={{ borderTop: 1, borderColor: 'divider', pt: 1 }}>
              {restyleOperation.status === 'running' ? <Stack spacing={0.5}>
                <Stack direction="row" justifyContent="space-between"><Typography variant="caption">{restyleOperation.message || restyleOperation.phase || 'Subtitle operation running'}</Typography><Typography variant="caption">{Math.round(restyleOperation.progress)}%</Typography></Stack>
                <LinearProgress variant="determinate" value={restyleOperation.progress} aria-label={`Subtitle operation for ${subtitle.fileName}`} />
                {onCancelRestyle ? <Button size="small" color="warning" variant="outlined" disabled={disabled} onClick={() => onCancelRestyle(subtitle)}>Cancel</Button> : null}
              </Stack> : <Stack direction="row" spacing={1} alignItems="center"><Alert severity={restyleOperation.status === 'completed' ? 'success' : restyleOperation.status === 'cancelled' ? 'info' : 'warning'} sx={{ py: 0, flex: 1 }}>{restyleOperation.error || restyleOperation.message || (restyleOperation.status === 'completed' ? 'Subtitle operation completed.' : 'Subtitle operation failed.')}</Alert>{restyleOperation.retryable && onRetryRestyle ? <Button size="small" variant="outlined" onClick={() => onRetryRestyle(subtitle)}>Retry</Button> : null}</Stack>}
            </Box> : null}
            {showReport && report ? <Box sx={{ borderTop: 1, borderColor: 'divider', pt: 1 }}>
              <Typography variant="subtitle2">OCR quality</Typography>
              <Typography variant="caption" color="text.secondary">{report.cleanCount} clean · {report.suspiciousCount} suspicious · {report.reviewCount} review</Typography>
              {report.findings.filter((finding) => finding.status !== 'clean').map((finding) => {
                const editKey = `${subtitle.path}:${finding.cueId}`;
                const value = correctionText[editKey] ?? finding.suggestion ?? finding.originalText;
                return <Stack key={editKey} direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }} sx={{ mt: 0.75 }}>
                  <Typography variant="caption" sx={{ minWidth: 60 }}>Cue {finding.cueId}</Typography>
                  <TextField size="small" fullWidth value={value} onChange={(event) => setCorrectionText((current) => ({ ...current, [editKey]: event.target.value }))} helperText={finding.reasons?.join('; ')} />
                  {onApplyCorrection ? <Button size="small" variant="outlined" disabled={disabled || !report.canCorrect} onClick={() => onApplyCorrection(subtitle, report, finding, value)}>Apply</Button> : null}
                </Stack>;
              })}
              {onApplyCorrections ? <Button size="small" variant="outlined" disabled={disabled || !report.canCorrect} onClick={() => onApplyCorrections(subtitle, report, report.findings.filter((finding) => finding.status === 'suspicious' && finding.candidateSource === 'deterministic-confusion-repair' && Boolean(finding.suggestion)).map((finding) => ({ cueId: finding.cueId, suggestion: finding.suggestion! })))}>Apply reviewed suggestions</Button> : null}
              {message ? <Typography variant="caption" color="text.secondary">{message}</Typography> : null}
            </Box> : null}
          </Stack>
          );
        })}
      </Stack>
    </Box>
    <Dialog open={Boolean(restyleTarget)} onClose={() => setRestyleTarget(null)} maxWidth="sm" fullWidth>
      <DialogTitle>{restyleTarget?.mode === 'srt_to_ass' ? 'Generate styled ASS sibling' : 'Restyle ASS / SSA sidecar'}</DialogTitle>
      <DialogContent dividers>
        {restyleTarget ? <Stack spacing={1.5}>
          <Typography variant="body2">The backend will use this exact source identity; it will not guess another same-language sidecar.</Typography>
          <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>Source: {restyleTarget.subtitle.path}</Typography>
          <Typography variant="body2">Identity: {restyleTarget.subtitle.eligibility?.sourceIdentity || `external:${restyleTarget.subtitle.path}`}</Typography>
          <Typography variant="body2">Revision: {restyleTarget.subtitle.eligibility?.sourceRevision || restyleTarget.subtitle.sourceRevision || 'unavailable'}</Typography>
          <Typography variant="body2">Representation: {(restyleTarget.subtitle.eligibility?.representation || restyleTarget.subtitle.format).toUpperCase()} · Role: {restyleTarget.subtitle.eligibility?.role || 'unknown'} · Provenance: {restyleTarget.subtitle.eligibility?.roleProvenance || 'backend-resolved'} · Policy: {restyleTarget.subtitle.eligibility?.policyVersion || 'current snapshot'}</Typography>
          {!restyleTarget.subtitle.sourceRevision ? <Alert severity="warning">Refresh Asset Info before starting; the exact source revision is unavailable.</Alert> : null}
          <FormControlLabel control={<Switch checked={replace} disabled={restyleTarget.subtitle.eligibility ? !restyleTarget.subtitle.eligibility.replaceEligible : false} onChange={(event) => setReplace(event.target.checked)} />} label="Replace the selected sidecar (explicit opt-in)" />
          {restyleTarget.mode === 'restyle' ? <FormControlLabel control={<Switch checked={overrideExistingASSStyles} disabled={restyleTarget.subtitle.eligibility ? !restyleTarget.subtitle.eligibility.nativeOverrideEligible : false} onChange={(event) => setOverrideExistingASSStyles(event.target.checked)} />} label="Override eligible native ASS styles for this operation" /> : null}
          {overrideExistingASSStyles ? <Alert severity="warning">Only eligible base Style assignments may change. Text, timing, karaoke, positioning, layers, margins, and source definitions remain protected.</Alert> : null}
        </Stack> : null}
      </DialogContent>
      <DialogActions>
        <Button onClick={() => setRestyleTarget(null)}>Cancel</Button>
        <Button variant="contained" disabled={!restyleTarget?.subtitle.sourceRevision || !onRestyle} onClick={() => {
          if (!restyleTarget || !onRestyle) return;
          onRestyle(restyleTarget.subtitle, restyleTarget.mode, { replace, overrideExistingASSStyles });
          setRestyleTarget(null);
        }}>{restyleTarget?.mode === 'srt_to_ass' ? 'Generate sibling' : 'Start restyle'}</Button>
      </DialogActions>
    </Dialog>
    </>
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
  onHydrateMany,
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
  onHydrateMany: (patch: Partial<AssetConversionOverrideState>) => void;
  onSave: () => void;
  onReset: () => void;
  saving: boolean;
  readOnly?: boolean;
}) {
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const runtimeSnapshot = useQuery({ queryKey: ['runtime-state'], queryFn: api.runtimeState });
  const workerNodes = useQuery({ queryKey: ['worker-nodes'], queryFn: api.workerNodes });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const [lastEncoderRecommendation, setLastEncoderRecommendation] =
    useState<{
      path: string;
      data: QualityRecommendationResponse;
    }>();

  const { mutate: requestEncoderQuality, isError: encoderQualityRecommendationError, error: encoderQualityRecommendationFailure } = useMutation({
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
        onHydrateMany(missingPreferences);
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
      onHydrateMany,
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
  const effectiveEvaluationProfileRef = useRef(effectiveEvaluationProfile);
  useEffect(() => {
    effectiveEvaluationProfileRef.current = effectiveEvaluationProfile;
  }, [effectiveEvaluationProfile]);
  const upscaleHeightError = customUpscaleHeightError(draft.upscaleMode, draft.upscaleCustomHeight);
  const displayedEncoderRecommendation =
    lastEncoderRecommendation?.path === assetPath
      ? lastEncoderRecommendation.data
      : undefined;

  useEffect(() => {
    const timer = window.setTimeout(() => requestEncoderQuality({ path: assetPath, profile: effectiveEvaluationProfileRef.current }), 250);
    return () => window.clearTimeout(timer);
  }, [assetPath, effectiveEvaluationSignature, requestEncoderQuality]);

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

    requestEncoderQuality(
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
    requestEncoderQuality({ path: assetPath, profile: requested });
  }

  function updateFrameStructurePolicy(patch: Record<string, unknown>) {
    const retainedQualityPreset = String(draft.hardwareQualityPreset ?? profile?.workerConfig?.hardwareQualityPreset ?? 'custom');
    Object.entries(patch).filter(([name]) => name !== 'frameStructureMode').forEach(([name, setting]) => onChange(name as keyof AssetConversionOverrideState, setting as never));
    if ('frameStructureMode' in patch) onChange('frameStructureMode', patch.frameStructureMode as AssetConversionOverrideState['frameStructureMode']);
    onChange('hardwareQualityPreset', retainedQualityPreset);
    if (effectiveVideoEncoder === 'hevc_qsv' || effectiveVideoEncoder === 'hevc_videotoolbox') {
      const requested = assetQualityProfile(profile, { ...draft, ...patch }, effectiveVideoEncoder, String(draft.hardwareQualityPreset ?? profile?.workerConfig?.hardwareQualityPreset ?? 'custom'));
      requestEncoderQuality({ path: assetPath, profile: requested });
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
            <Button variant="contained" onClick={onSave} disabled={saving || readOnly || Boolean(upscaleHeightError)}>
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
          <Grid size={{ xs: 12 }}>
            <SmartUpscaleControls
              allowInherit
              disabled={smartUpscaleControlsDisabled(effectiveVideoCodec, readOnly)}
              value={{ mode: draft.upscaleMode, sharpen: draft.upscaleSharpen, customHeight: draft.upscaleCustomHeight, customSharpenStrength: draft.upscaleSharpenCustomStrength }}
              onChange={(patch) => {
                if ('mode' in patch) {
                  onChange('upscaleMode', patch.mode as UpscaleMode | undefined);
                  if (patch.mode !== 'custom') onChange('upscaleCustomHeight', undefined);
                }
                if ('sharpen' in patch) onChange('upscaleSharpen', patch.sharpen as UpscaleSharpen | undefined);
                if ('customSharpenStrength' in patch) onChange('upscaleSharpenCustomStrength', patch.customSharpenStrength);
                if ('customHeight' in patch) onChange('upscaleCustomHeight', patch.customHeight);
              }}
            />
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
            disabled={readOnly}
            {...assetMotionModes}
            scan={scan}
            allowProfileDefault
            onFieldStructureChange={(value) => onChange('fieldStructureMode', value || undefined)}
            onCadenceChange={(value) => onChange('cadenceMode', value || undefined)}
            onCadenceFieldOrderChange={(value) => onChange('cadenceFieldOrder', value || undefined)}
            onDeinterlaceFieldOrderChange={(value) => onChange('deinterlaceFieldOrder', value || undefined)}
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
                    autoStrategy={assetFrameRecommendation?.autoStrategy}
                    analysisDriven={assetFrameRecommendation?.analysisDriven}
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
              {encoderQualityRecommendationError ? <Alert severity="warning">Quality recommendation failed: {encoderQualityRecommendationFailure instanceof Error ? encoderQualityRecommendationFailure.message : 'unknown error'}</Alert> : null}
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
                  <SourceBitrateQualityEvidence source={displayedEncoderRecommendation.sourceBitrateEvidence} assessment={displayedEncoderRecommendation.recommendation.bpppfAssessment} />
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

function FinalDetailsSummary({ asset, job }: { asset: Asset; job: QueueJob }) {
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
  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, bgcolor: 'rgba(79,179,255,0.035)' }}>
      <Stack spacing={1.5}>
        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
          <Chip label={status} color={job?.status === 'failed' ? 'error' : 'success'} size="small" />
          {job ? <Chip label={job.executionNumber ? `Job #${job.executionNumber}` : 'Pending'} size="small" /> : null}
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
          {job.outputPath ? (
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
            ) : <Typography variant="caption" color="text.secondary">Conversion command unavailable.</Typography>}
          </>
        ) : null}
        {job.notes ? (
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

function FinalValidationEvidence({ job }: { job: QueueJob }) {
  const report = job.validationReport ?? {};
  const colorPolicy = recordFromRecord(report, 'colorPolicy');
  const sourceColor = recordFromRecord(colorPolicy, 'source');
  const outputColor = recordFromRecord(colorPolicy, 'output');
  const frameFidelity = recordFromRecord(report, 'frameFidelity');
  const frameFidelityFields = recordFromRecord(frameFidelity, 'fields');

  return <Stack spacing={1.5}>
    {Object.keys(colorPolicy).length > 0 ? (
      <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
        <Stack spacing={1}>
          <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
            <Typography fontWeight={700}>Color validation</Typography>
            <Chip size="small" label={stringFromRecord(colorPolicy, 'status') || 'unverified'} color={stringFromRecord(colorPolicy, 'status') === 'passed' ? 'success' : 'warning'} />
            <Chip size="small" label={`Requested: ${stringFromRecord(colorPolicy, 'requestedPolicy') || 'automatic'}`} />
            <Chip size="small" label={`Effective: ${stringFromRecord(colorPolicy, 'effectivePolicy') || 'unknown'}`} />
          </Stack>
          <Typography variant="body2" color="text.secondary">Source: {colorCharacteristicsLabel(sourceColor)}</Typography>
          <Typography variant="body2" color="text.secondary">Output: {colorCharacteristicsLabel(outputColor)}</Typography>
        </Stack>
      </Box>
    ) : null}
    {Object.keys(frameFidelityFields).length > 0 ? (
      <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
        <Stack spacing={1}>
          <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
            <Typography fontWeight={700}>Frame fidelity</Typography>
            <Chip size="small" label={stringFromRecord(frameFidelity, 'status') || 'unverified'} color={stringFromRecord(frameFidelity, 'status') === 'passed' ? 'success' : 'warning'} />
          </Stack>
          <Grid container spacing={1}>
            {Object.entries(frameFidelityFields).map(([key, raw]) => {
              const field = raw && typeof raw === 'object' && !Array.isArray(raw) ? raw as Record<string, unknown> : {};
              const status = stringFromRecord(field, 'status') || 'unverified';
              return (
                <Grid key={key} size={{ xs: 12, sm: 6, md: 4 }}>
                  <Typography variant="caption" color="text.secondary">{frameFidelityLabel(key)}</Typography>
                  <Typography variant="body2">Source: {fidelityValueLabel(field.source)}</Typography>
                  <Typography variant="body2">Expected: {fidelityValueLabel(field.expected)}</Typography>
                  <Typography variant="body2">Actual: {fidelityValueLabel(field.actual ?? field.output)}</Typography>
                  <Stack direction="row" spacing={0.75} alignItems="center" flexWrap="wrap" useFlexGap>
                    <Chip size="small" label={status.replaceAll('_', ' ')} color={status === 'mismatch' ? 'error' : status === 'passed' ? 'success' : status === 'unverified' ? 'warning' : 'default'} />
                    {stringFromRecord(field, 'authority') ? <Chip size="small" variant="outlined" label={stringFromRecord(field, 'authority')} /> : null}
                  </Stack>
                  {stringFromRecord(field, 'reason') ? <Typography variant="caption" color="text.secondary">{stringFromRecord(field, 'reason')}</Typography> : null}
                </Grid>
              );
            })}
          </Grid>
        </Stack>
      </Box>
    ) : null}
  </Stack>;
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
    defaultSubtitleLanguage: '', validationMode: 'warn', subtitleDisposition: 'keep', subtitleRules: [],
    attachmentPolicy: 'auto', chapterPolicy: 'keep', notes: '',
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

function lifecycleLabel(value: string) {
  return value.replaceAll('_', ' ').replace(/\b\w/g, (character) => character.toUpperCase());
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

function effectivePathDestination(group: AssetGroup, destinations: Record<string, number>, mode: 'unprocessed' | 'library' | 'converted') {
  return destinations[normalizePath(group.path)] ?? (mode === 'library' ? group.libraryId : 0);
}

function pathConfigurationComplete(group: AssetGroup, assignments: ProfileAssignment[], destinations: Record<string, number>, mode: 'unprocessed' | 'library' | 'converted') {
  const pathAssignments = pathAssignmentsFor(group.path, assignments);
  const hasProfile = [...pathAssignments.values()].some((assignment) => assignment.selection === 'profile');
  return hasProfile && effectivePathDestination(group, destinations, mode) > 0;
}

function pathCanQueue(group: AssetGroup, mode: 'unprocessed' | 'library' | 'converted') {
  if (mode === 'unprocessed') return group.status === 'unprocessed';
  if (mode !== 'library') return false;
  if (group.status === 'accepted' || group.status === 'published_as_is') return false;
  return group.status === 'library' || group.status === 'unverified';
}

function configurationForPathSelection(groups: AssetGroup[], assignments: ProfileAssignment[], destinations: Record<string, number>, mode: 'unprocessed' | 'library' | 'converted') {
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

function countLabel(count: number, singular: string) {
	return `${count} ${singular}${count === 1 ? '' : 's'}`;
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
    default:
      return 'warning';
  }
}

function firstCategory(categories?: string[] | null) {
  return safeArray(categories).find((category) => category.trim()) ?? '';
}

function libraryEffectiveCategory(effective: EffectiveAssetConfiguration | undefined, inheritedFallback: string, disabledLabel = '') {
  if (effective?.category.selection === 'value') return effective.category.category ?? '';
  if (effective?.category.selection === 'disabled') return disabledLabel;
  return inheritedFallback;
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
    .filter(queueJobIsActive)
    .filter((job) => asset.status !== 'unprocessed' || job.status === 'queued' || job.status === 'running')
    .find((job) => {
      const candidates = [job.publishedPath, job.outputPath, job.mediaPath].map(normalizePath).filter(Boolean);
      return candidates.includes(normalizedAssetPath);
    });
}

function assetPipelineState(asset: Asset, job: QueueJob | undefined, pendingQueue: boolean): { label: string; color: 'default' | 'primary' | 'success' | 'warning' | 'error' } {
  if (asset.lifecycle) {
    const state = asset.lifecycle.lifecycle;
    if (state === 'published') return { label: 'Published', color: 'success' };
    if (state === 'queued') return { label: 'QUEUED', color: 'primary' };
    if (state === 'preparing') return { label: 'PREPARING', color: 'primary' };
    if (state === 'processing') return { label: 'PROCESSING', color: 'primary' };
    if (state === 'validating') return { label: 'VALIDATING', color: 'primary' };
    if (state === 'publishing') return { label: 'PUBLISHING', color: 'primary' };
    if (state === 'failed') return { label: 'Failed', color: 'error' };
    if (state === 'needs_review') return { label: 'Needs review', color: 'error' };
    if (asset.lifecycle.provenance === 'converted') return { label: 'Converted', color: 'success' };
    if (asset.lifecycle.provenance === 'published_as_is') return { label: 'Published as-is', color: 'success' };
    if (asset.lifecycle.provenance === 'imported') return { label: 'Imported', color: 'success' };
    return { label: 'Available', color: 'default' };
  }
  if (pendingQueue) {
    return { label: 'Queueing', color: 'primary' };
  }
  if (jobPublishesAsset(asset, job)) {
    return { label: 'Published', color: 'success' };
  }
  const activeJob = job?.publishedAt && !job.publicationRetiredAt ? undefined : job;
  if (activeJob?.status === 'running') {
    return activeJob.stage === 'claimed'
      ? { label: 'PREPARING', color: 'primary' }
      : { label: 'PROCESSING', color: 'primary' };
  }
  if (activeJob?.status === 'queued') {
    return { label: 'QUEUED', color: 'primary' };
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
  const recommendation = scan?.frameStructureRecommendation;
  if (!recommendation) return undefined;

  const autoStrategy = recommendation.autoStrategy;
  const autoGop = autoStrategy
    ? recommendation.byMode?.[autoStrategy]?.targetGopFrames
    : undefined;

  return {
    fps: recommendation.fps,
    gop: autoGop,
    bFrames: recommendation.recommendedMaxBFrames,
    autoStrategy,
    analysisDriven: recommendation.analysisDriven,
    gopByMode: {
      compatible: recommendation.byMode?.compatible?.targetGopFrames,
      balanced: recommendation.byMode?.balanced?.targetGopFrames,
      maximum_compression: recommendation.byMode?.maximum_compression?.targetGopFrames,
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
  return ({ width: 'Width', height: 'Height', sampleAspectRatio: 'SAR', displayAspectRatio: 'DAR', frameRate: 'Frame rate', fieldStructure: 'Field structure', fieldOrder: 'Field order', pixelFormat: 'Pixel format', bitDepth: 'Bit depth', chromaLocation: 'Chroma location', hevcLevel: 'HEVC Level', hevcTier: 'HEVC Tier', frameSize: 'Frame size', colorPolicy: 'Color policy' } as Record<string, string>)[key] || key;
}

function fidelityValueLabel(value: unknown): string {
  if (value === null || value === undefined || value === '') return 'Unknown';
  if (typeof value !== 'object') return String(value);
  const record = value as Record<string, unknown>;
  if (record.raw || record.bitDepth || record.chroma || record.profile) {
    return [record.raw, record.bitDepth, record.chroma, record.profile].filter(Boolean).join(' / ');
  }
  try {
    return JSON.stringify(value);
  } catch {
    return 'Unknown';
  }
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
  if (value.subtitleRoleOverrides) {
    const roles: SubtitleSemanticRole[] = ['full', 'forced', 'signs', 'commentary', 'sdh', 'unknown'];
    const overrides = Object.fromEntries(Object.entries(value.subtitleRoleOverrides).flatMap(([index, override]) => {
      const numericIndex = Number(index);
      return Number.isInteger(numericIndex) && numericIndex >= 0 && roles.includes(override.role)
        ? [[String(numericIndex), { ...override, role: override.role }]]
        : [];
    }));
    if (Object.keys(overrides).length || value.subtitleRoleOverrides !== undefined) clean.subtitleRoleOverrides = overrides;
  }
  if (Array.isArray(value.subtitleTransforms) && value.subtitleTransforms.length) {
    clean.subtitleTransforms = value.subtitleTransforms;
  }
  if (value.subtitleSidecarFormatsByStream) {
    const formats = Object.fromEntries(Object.entries(value.subtitleSidecarFormatsByStream).flatMap(([index, values]) => {
      const numericIndex = Number(index);
      const normalized = Array.from(new Set(values.filter((format) => format === 'original' || format === 'srt')));
      return Number.isInteger(numericIndex) && numericIndex >= 0 && normalized.length ? [[String(numericIndex), normalized]] : [];
    }));
    if (Object.keys(formats).length) clean.subtitleSidecarFormatsByStream = formats;
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
  if (value.upscaleMode === 'disabled' || value.upscaleMode === 'auto' || value.upscaleMode === '720p' || value.upscaleMode === '1080p' || value.upscaleMode === 'custom') {
    clean.upscaleMode = value.upscaleMode;
  }
  if (value.upscaleSharpen === 'off' || value.upscaleSharpen === 'light' || value.upscaleSharpen === 'medium' || value.upscaleSharpen === 'custom') {
    clean.upscaleSharpen = value.upscaleSharpen;
  }
  if (value.upscaleSharpen === 'custom' && typeof value.upscaleSharpenCustomStrength === 'number' && Number.isFinite(value.upscaleSharpenCustomStrength) && value.upscaleSharpenCustomStrength >= 0 && value.upscaleSharpenCustomStrength <= 0.4) clean.upscaleSharpenCustomStrength = value.upscaleSharpenCustomStrength;
  if (typeof value.upscaleCustomHeight === 'number' && Number.isFinite(value.upscaleCustomHeight) && value.upscaleCustomHeight >= 2 && value.upscaleCustomHeight % 2 === 0 && value.upscaleMode === 'custom') {
    clean.upscaleCustomHeight = value.upscaleCustomHeight;
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
  if (value.deinterlaceFieldOrder === 'auto' || value.deinterlaceFieldOrder === 'tff' || value.deinterlaceFieldOrder === 'bff') clean.deinterlaceFieldOrder = value.deinterlaceFieldOrder;
  if (value.frameStructureMode === 'auto' || value.frameStructureMode === 'off' || value.frameStructureMode === 'compatible' || value.frameStructureMode === 'balanced' || value.frameStructureMode === 'maximum_compression' || value.frameStructureMode === 'custom') clean.frameStructureMode = value.frameStructureMode;
  if (value.frameStructureGopMode === 'auto' || value.frameStructureGopMode === 'recommended' || value.frameStructureGopMode === 'custom') clean.frameStructureGopMode = value.frameStructureGopMode;
  if (typeof value.frameStructureGopFrames === 'number' && Number.isFinite(value.frameStructureGopFrames) && value.frameStructureGopFrames > 0) clean.frameStructureGopFrames = Math.min(1000, Math.round(value.frameStructureGopFrames));
  if (value.frameStructureBFrameMode === 'auto' || value.frameStructureBFrameMode === 'recommended' || value.frameStructureBFrameMode === 'custom' || value.frameStructureBFrameMode === 'off') clean.frameStructureBFrameMode = value.frameStructureBFrameMode;
  if (value.hevcLevelMode === 'auto' || value.hevcLevelMode === 'recommended' || value.hevcLevelMode === 'custom') clean.hevcLevelMode = value.hevcLevelMode;
  if (typeof value.frameStructureMaxBFrames === 'number' && Number.isFinite(value.frameStructureMaxBFrames) && value.frameStructureMaxBFrames >= 0) clean.frameStructureMaxBFrames = Math.min(16, Math.round(value.frameStructureMaxBFrames));
  if (value.qsvRateControl === 'icq' || value.qsvRateControl === 'la_icq') {
    clean.qsvRateControl = value.qsvRateControl;
  }
  if (value.qsvMBBRCMode === 'auto' || value.qsvMBBRCMode === 'enabled' || value.qsvMBBRCMode === 'disabled') {
    clean.qsvMBBRCMode = value.qsvMBBRCMode;
  }
  if (value.qsvRDOMode === 'auto' || value.qsvRDOMode === 'enabled' || value.qsvRDOMode === 'disabled') {
    clean.qsvRDOMode = value.qsvRDOMode;
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
  if (typeof value.videoToolboxQualityProfile === 'number' && Number.isFinite(value.videoToolboxQualityProfile) && value.videoToolboxQualityProfile > 0) clean.videoToolboxQualityProfile = Math.round(value.videoToolboxQualityProfile);
  if (value.videoToolboxProfile?.trim()) clean.videoToolboxProfile = value.videoToolboxProfile.trim();
  if (typeof value.videoToolboxGop === 'number' && Number.isFinite(value.videoToolboxGop) && value.videoToolboxGop > 0) clean.videoToolboxGop = Math.round(value.videoToolboxGop);
  if (typeof value.videoToolboxRealtime === 'boolean') clean.videoToolboxRealtime = value.videoToolboxRealtime;
  if (value.videoToolboxBFramePolicy === 'auto' || value.videoToolboxBFramePolicy === 'enabled' || value.videoToolboxBFramePolicy === 'disabled') clean.videoToolboxBFramePolicy = value.videoToolboxBFramePolicy;
  if (typeof value.videoToolboxBFrames === 'number' && Number.isFinite(value.videoToolboxBFrames) && value.videoToolboxBFrames >= 0) clean.videoToolboxBFrames = Math.round(value.videoToolboxBFrames);
  if (typeof value.videoToolboxAutoAdjustBitrate === 'boolean') clean.videoToolboxAutoAdjustBitrate = value.videoToolboxAutoAdjustBitrate;
  if (typeof value.videoToolboxAllowFrameReordering === 'boolean') clean.videoToolboxAllowFrameReordering = value.videoToolboxAllowFrameReordering;
  if (typeof value.videoToolboxPowerEfficiency === 'boolean') clean.videoToolboxPowerEfficiency = value.videoToolboxPowerEfficiency;
  if (value.hardwareQualityPreset?.trim()) clean.hardwareQualityPreset = value.hardwareQualityPreset.trim();
  return clean;
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
