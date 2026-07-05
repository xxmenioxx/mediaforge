import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Collapse,
  Dialog,
  DialogContent,
  DialogTitle,
  Grid,
  IconButton,
  Stack,
  Switch,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import FactCheckIcon from '@mui/icons-material/FactCheck';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import ManageSearchIcon from '@mui/icons-material/ManageSearch';
import PlayCircleIcon from '@mui/icons-material/PlayCircle';
import PlaylistAddIcon from '@mui/icons-material/PlaylistAdd';
import RefreshIcon from '@mui/icons-material/Refresh';
import ReportProblemIcon from '@mui/icons-material/ReportProblem';
import TaskAltIcon from '@mui/icons-material/TaskAlt';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import type { MouseEvent } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { JobDetailsDialog } from '../components/JobDetailsDialog';
import { MediaSnapshotDetails } from '../components/MediaSnapshotDetails';
import { PageHeader } from '../components/PageHeader';
import type { AdvisorResponse, AppSetting, Asset, AssetGroup, AudioEnhancementProfile, Library, Profile, QueueJob } from '../api/types';

export function AssetsPage() {
  const [tab, setTab] = useState<'unprocessed' | 'converted'>('unprocessed');
  const assets = useQuery({ queryKey: ['assets'], queryFn: api.assets });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const jobs = useQuery({ queryKey: ['queueJobs'], queryFn: api.queueJobs });
  const audioProfiles = getAudioProfiles(settings.data);
  const assetCategories = getAssetCategories(settings.data);
  const unprocessedCount = assets.data?.unprocessed.length ?? 0;
  const convertedCount = assets.data?.converted.length ?? 0;
  const currentGroups = tab === 'unprocessed' ? assets.data?.unprocessedGroups ?? [] : assets.data?.convertedGroups ?? [];

  return (
    <>
      <PageHeader title="Assets" eyebrow="Media inventory">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Originals are listed from the global source root. Converted assets are listed from each configured destination library.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {assets.isError ? <Alert severity="warning">Unable to read library paths from the backend container.</Alert> : null}

        <Card>
          <CardContent sx={{ pb: 0 }}>
            <Stack
              direction={{ xs: 'column', sm: 'row' }}
              alignItems={{ xs: 'stretch', sm: 'center' }}
              justifyContent="space-between"
              spacing={1}
              sx={{ borderBottom: 1, borderColor: 'divider' }}
            >
              <Tabs value={tab} onChange={(_, value) => setTab(value)}>
                <Tab label={<AssetTabLabel label="Unprocessed" count={unprocessedCount} color="warning" />} value="unprocessed" />
                <Tab label={<AssetTabLabel label="Converted" count={convertedCount} color="success" />} value="converted" />
              </Tabs>
              <Button startIcon={<RefreshIcon />} variant="outlined" onClick={() => assets.refetch()} sx={{ mb: 1 }}>
                Snapshot
              </Button>
            </Stack>
          </CardContent>
          <AssetTable
            key={tab}
            groups={currentGroups}
            libraries={libraries.data ?? []}
            profiles={profiles.data ?? []}
            audioProfiles={audioProfiles}
            settings={settings.data ?? []}
            assetCategories={assetCategories}
            queueJobs={jobs.data ?? []}
            emptyLabel={tab === 'unprocessed' ? 'No pending asset groups found.' : 'No converted asset groups found.'}
          />
        </Card>
      </Box>
    </>
  );
}

function AssetTabLabel({ label, count, color }: { label: string; count: number; color: 'warning' | 'success' }) {
  return (
    <Stack direction="row" alignItems="center" spacing={1}>
      <Typography fontWeight={700}>{label}</Typography>
      <Chip label={count} color={color} size="small" />
    </Stack>
  );
}

const actionIconSx = {
  border: 1,
  borderColor: 'divider',
  width: 38,
  height: 38,
};

function AssetTable({
  groups,
  libraries,
  profiles,
  audioProfiles,
  settings,
  assetCategories,
  queueJobs,
  emptyLabel,
}: {
  groups: AssetGroup[];
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  settings: AppSetting[];
  assetCategories: string[];
  queueJobs: QueueJob[];
  emptyLabel: string;
}) {
  if (groups.length === 0) {
    return (
      <CardContent>
        <Alert severity="info">{emptyLabel}</Alert>
      </CardContent>
    );
  }

  return (
    <Box sx={{ overflowX: 'auto' }}>
      <Table size="small" sx={{ minWidth: 980, tableLayout: 'fixed' }}>
        <TableHead>
          <TableRow>
            <TableCell sx={{ width: 360 }}>Path</TableCell>
            <TableCell sx={{ width: 130 }}>Library</TableCell>
            <TableCell sx={{ width: 150 }}>Status</TableCell>
            <TableCell sx={{ width: 150 }}>Confidence</TableCell>
            <TableCell sx={{ width: 80 }}>Files</TableCell>
            <TableCell sx={{ width: 140 }}>Total size</TableCell>
            <TableCell sx={{ width: 190 }}>Modified</TableCell>
            <TableCell padding="checkbox" sx={{ width: 52 }} />
          </TableRow>
        </TableHead>
        <TableBody>
          {groups.map((group) => (
            <AssetGroupRow
              key={group.id}
              group={group}
              libraries={libraries}
              profiles={profiles}
              audioProfiles={audioProfiles}
              settings={settings}
              assetCategories={assetCategories}
              queueJobs={queueJobs}
            />
          ))}
        </TableBody>
      </Table>
    </Box>
  );
}

function AssetGroupRow({
  group,
  libraries,
  profiles,
  audioProfiles,
  settings,
  assetCategories,
  queueJobs,
}: {
  group: AssetGroup;
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  settings: AppSetting[];
  assetCategories: string[];
  queueJobs: QueueJob[];
}) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [expanded, setExpanded] = useState(false);
  const [selectedProfileId, setSelectedProfileId] = useState<number>(profiles[0]?.id ?? 0);
  const [selectedAudioProfileKey, setSelectedAudioProfileKey] = useState<string>('');
  const [selectedLibraryId, setSelectedLibraryId] = useState<number>(group.libraryId);
  const [groupCategory, setGroupCategory] = useState<string>(firstCategory(group.pathMetadata.categories));
  const effectiveProfileId = selectedProfileId || profiles[0]?.id || 0;
  const representativeAsset = firstAssetForGroup(group);
  const isConvertedGroup = group.status === 'converted';
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
  const advisor = useQuery({
    queryKey: ['advisor', group.id, representativeAsset?.path, effectiveProfileId],
    queryFn: () =>
      api.evaluateAdvisor({
        mediaPath: representativeAsset?.path ?? '',
        profileId: effectiveProfileId,
      }),
    enabled: expanded && isConfidenceEnabled && Boolean(effectiveProfileId && representativeAsset),
  });
  const queueGroup = useMutation({
    mutationFn: async () => {
      if (!effectiveProfileId || !selectedLibraryId) {
        return [];
      }

      const batchId = createBatchId(group);
      const batchName = groupDisplayPath(group);

      return Promise.all(
        group.assets.map((asset) =>
          api.createQueueJob({
            mediaPath: asset.path,
            batchId,
            batchName,
            libraryId: selectedLibraryId,
            profileId: effectiveProfileId,
            audioProfileKey: selectedAudioProfileKey,
            priority: priorityForSize(asset.sizeBytes),
            notes: queueNotes(`Queued from folder: ${batchName}`, selectedAudioProfileKey),
          }),
        ),
      );
    },
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['queueJobs'] }),
        queryClient.invalidateQueries({ queryKey: ['assets'] }),
      ]);
      navigate(`/queue?path=${encodeURIComponent(group.path)}`);
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
          <Stack spacing={0.4}>
            <Typography fontWeight={700} sx={{ wordBreak: 'break-word' }}>
              {groupDisplayPath(group)}
            </Typography>
            <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
              {group.path}
            </Typography>
          </Stack>
        </TableCell>
        <TableCell>{group.libraryName}</TableCell>
        <TableCell>
          <Stack spacing={0.75} alignItems="flex-start">
            <Chip
              label={group.status === 'converted' ? 'Converted' : 'Unprocessed'}
              color={group.status === 'converted' ? 'success' : 'warning'}
              size="small"
            />
            {group.review.requiresReview ? <Chip label="Some need review" color="error" size="small" /> : null}
          </Stack>
        </TableCell>
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
        <TableCell colSpan={8} sx={{ p: 0, borderBottom: expanded ? 1 : 0, borderColor: 'divider', maxWidth: 0 }}>
          <Collapse in={expanded} timeout="auto" unmountOnExit>
            <Box sx={{ bgcolor: 'rgba(255,255,255,0.02)', px: { xs: 1.5, md: 2 }, py: 2, width: '100%', maxWidth: '100%', overflow: 'hidden' }}>
              <Stack spacing={2}>
                {isConvertedGroup ? (
                  <Alert severity="info">
                    Converted assets are read-only here. Use Preview or Final Details to inspect results; re-processing should start from Original Archive.
                  </Alert>
                ) : (
                  <Grid container spacing={2} alignItems="stretch">
                    <Grid size={{ xs: 12, md: 2 }}>
                      <AssetCategorySelect
                        value={groupCategory}
                        options={assetCategories}
                        onChange={(category) => {
                          setGroupCategory(category);
                          updateMetadata.mutate({ path: group.path, categories: category ? [category] : [], tags: group.pathMetadata.tags ?? [] });
                        }}
                        label="Category"
                      />
                    </Grid>
                    <Grid size={{ xs: 12, md: 3 }}>
                      <ProfileAutocomplete profiles={profiles} value={effectiveProfileId} onChange={setSelectedProfileId} label="Video profile" />
                    </Grid>
                    <Grid size={{ xs: 12, md: 3 }}>
                      <AudioProfileAutocomplete
                        profiles={audioProfiles}
                        value={selectedAudioProfileKey}
                        onChange={setSelectedAudioProfileKey}
                        label="Audio profile"
                      />
                    </Grid>
                    <Grid size={{ xs: 12, md: 2 }}>
                      <LibraryAutocomplete libraries={libraries} value={selectedLibraryId} onChange={setSelectedLibraryId} label="Destination library" />
                    </Grid>
                    <Grid size={{ xs: 12, md: 2 }}>
                      <Button
                        startIcon={<PlaylistAddIcon />}
                        variant="contained"
                        size="small"
                        onClick={() => queueGroup.mutate()}
                        disabled={
                          queueGroup.isPending ||
                          !effectiveProfileId ||
                          !selectedLibraryId ||
                          group.assets.length === 0 ||
                          group.assets.some((asset) => asset.review.requiresReview || assetHasOpenJob(asset, queueJobs))
                        }
                        fullWidth
                        sx={{ minHeight: 40, alignSelf: 'center' }}
                      >
                        Queue Folder
                      </Button>
                    </Grid>
                  </Grid>
                )}
                {!isConfidenceEnabled ? (
                  <Alert severity="warning">
                    Confidence is off for this path. Advisor checks and any future confidence-based automation will be skipped here; manual queueing still works.
                  </Alert>
                ) : null}
                {group.review.requiresReview ? (
                  <Alert severity="warning">
                    Folder queue is blocked because at least one asset in this path needs review. You can still queue approved assets individually.
                  </Alert>
                ) : null}
                {advisor.isError ? <Alert severity="warning">Could not evaluate this path.</Alert> : null}
                {queueGroup.isSuccess ? <Alert severity="success">{group.assets.length} files queued from this folder.</Alert> : null}
                {queueGroup.isError ? <Alert severity="warning">Could not queue this folder.</Alert> : null}
                <Box sx={{ width: '100%', maxWidth: '100%', overflowX: 'auto', pb: 0.5 }}>
                  <Table size="small" sx={{ minWidth: 1240, tableLayout: 'fixed' }}>
                    <TableHead>
                      <TableRow>
                        <TableCell sx={{ width: 230 }}>Asset file</TableCell>
                        <TableCell sx={{ width: 118 }}>Status</TableCell>
                        <TableCell sx={{ width: 95 }}>Score</TableCell>
                        <TableCell sx={{ width: 100 }}>Size</TableCell>
                        <TableCell sx={{ width: 128 }}>Modified</TableCell>
                        <TableCell sx={{ width: 145 }}>Category</TableCell>
                        <TableCell sx={{ width: 165 }}>Video profile</TableCell>
                        <TableCell sx={{ width: 165 }}>Audio profile</TableCell>
                        <TableCell sx={{ width: 160 }}>Destination</TableCell>
                        <TableCell align="center" sx={{ width: 180 }}>Actions</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {group.assets.map((asset) => (
                        <AssetRow
                          key={`${asset.status}-${asset.libraryId}-${asset.path}-${selectedProfileId}-${selectedLibraryId}`}
                          asset={asset}
                          libraries={libraries}
                          profiles={profiles}
                          audioProfiles={audioProfiles}
                          assetCategories={assetCategories}
                          groupRelativePath={group.relativePath}
                          groupCategory={groupCategory}
                          confidenceEnabled={isConfidenceEnabled}
                          groupProfileId={effectiveProfileId}
                          groupAudioProfileKey={selectedAudioProfileKey}
                          groupLibraryId={selectedLibraryId}
                          hasOpenJob={queueGroup.isPending || assetHasOpenJob(asset, queueJobs)}
                          queueJobs={queueJobs}
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
  assetCategories,
  groupRelativePath,
  groupCategory,
  confidenceEnabled,
  groupProfileId,
  groupAudioProfileKey,
  groupLibraryId,
  hasOpenJob,
  queueJobs,
}: {
  asset: Asset;
  libraries: Library[];
  profiles: Profile[];
  audioProfiles: AudioEnhancementProfile[];
  assetCategories: string[];
  groupRelativePath: string;
  groupCategory: string;
  confidenceEnabled: boolean;
  groupProfileId: number;
  groupAudioProfileKey: string;
  groupLibraryId: number;
  hasOpenJob: boolean;
  queueJobs: QueueJob[];
}) {
  const queryClient = useQueryClient();
  const [selectedProfileId, setSelectedProfileId] = useState<number>(groupProfileId);
  const [selectedAudioProfileKey, setSelectedAudioProfileKey] = useState<string>(groupAudioProfileKey);
  const [selectedLibraryId, setSelectedLibraryId] = useState<number>(groupLibraryId);
  const [showSnapshotDialog, setShowSnapshotDialog] = useState(false);
  const [showPreviewDialog, setShowPreviewDialog] = useState(false);
  const [showAdvisorDialog, setShowAdvisorDialog] = useState(false);
  const [showFinalDetailsDialog, setShowFinalDetailsDialog] = useState(false);
  const [previewMode, setPreviewMode] = useState<'compatible' | 'original'>('compatible');
  const [reviewReason, setReviewReason] = useState(asset.review.reason || '');
  const [reviewTags, setReviewTags] = useState<string[]>(asset.review.tags ?? []);
  const [category, setCategory] = useState<string>(firstCategory(asset.metadata.categories) || groupCategory);
  const snapshot = useMutation({ mutationFn: api.scan });
  const advisor = useQuery({
    queryKey: ['advisor', 'asset-row', asset.path, selectedProfileId],
    queryFn: () => api.evaluateAdvisor({ mediaPath: asset.path, profileId: selectedProfileId }),
    enabled: confidenceEnabled && Boolean(selectedProfileId),
  });
  const createJob = useMutation({
    mutationFn: api.createQueueJob,
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
  const updateMetadata = useMutation({
    mutationFn: api.updateAssetMetadata,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const isBlockedByReview = asset.review.requiresReview;
  const isConverted = asset.status === 'converted';
  const associatedJob = associatedJobForAsset(asset, queueJobs);
  const rowLocked = hasOpenJob || createJob.isPending || isConverted;
  const pipelineState = assetPipelineState(asset, associatedJob, createJob.isPending);

  useEffect(() => {
    if (!firstCategory(asset.metadata.categories)) {
      setCategory(groupCategory);
    }
  }, [asset.metadata.categories, groupCategory]);

  function openSnapshotDialog(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    setShowSnapshotDialog(true);
    if (!snapshot.data && !snapshot.isPending) {
      snapshot.mutate({ path: asset.path });
    }
  }

  function openPreviewDialog(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    setPreviewMode('compatible');
    setShowPreviewDialog(true);
  }

  function refreshSnapshot() {
    snapshot.mutate({ path: asset.path, force: true });
  }

  function queueAsset(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    if (!selectedProfileId || !selectedLibraryId) {
      return;
    }

    createJob.mutate({
      mediaPath: asset.path,
      libraryId: selectedLibraryId,
      profileId: selectedProfileId,
      audioProfileKey: selectedAudioProfileKey,
      priority: priorityForSize(asset.sizeBytes),
      notes: queueNotes(`Queued individually from folder view: ${relativeAssetPath(asset, libraries)}`, selectedAudioProfileKey),
    });
  }

  function toggleAssetReview(event: MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    const nextRequiresReview = !asset.review.requiresReview;
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
      requiresReview: asset.review.requiresReview,
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
      tags: asset.metadata.tags ?? [],
    });
  }

  return (
    <>
      <TableRow hover>
        <TableCell>
          <Stack spacing={0.4}>
            <Typography fontWeight={700} sx={{ wordBreak: 'break-word' }}>
              {assetDisplayPath(asset, groupRelativePath, libraries)}
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
          </Stack>
        </TableCell>
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
        <TableCell>{formatBytes(asset.sizeBytes)}</TableCell>
        <TableCell>{formatDate(asset.modifiedAt)}</TableCell>
        <TableCell sx={{ minWidth: 180 }}>
          <AssetCategorySelect value={category} options={assetCategories} onChange={saveAssetCategory} label="Category" size="small" disabled={rowLocked} />
        </TableCell>
        <TableCell sx={{ minWidth: 220 }}>
          <ProfileAutocomplete profiles={profiles} value={selectedProfileId} onChange={setSelectedProfileId} label="Video" size="small" disabled={rowLocked} />
        </TableCell>
        <TableCell sx={{ minWidth: 220 }}>
          <AudioProfileAutocomplete profiles={audioProfiles} value={selectedAudioProfileKey} onChange={setSelectedAudioProfileKey} label="Audio" size="small" disabled={rowLocked} />
        </TableCell>
        <TableCell sx={{ minWidth: 220 }}>
          <LibraryAutocomplete libraries={libraries} value={selectedLibraryId} onChange={setSelectedLibraryId} label="Destination" size="small" disabled={rowLocked} />
        </TableCell>
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
            {!isConverted ? (
              <Tooltip title={asset.review.requiresReview ? 'Disable review block' : 'Enable review block'}>
                <IconButton
                  color={asset.review.requiresReview ? 'warning' : 'primary'}
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
              <Tooltip title="Final details">
                <IconButton
                  color="primary"
                  onClick={() => setShowFinalDetailsDialog(true)}
                  aria-label={`Final details for ${asset.fileName}`}
                  sx={actionIconSx}
                >
                  <FactCheckIcon />
                </IconButton>
              </Tooltip>
            ) : (
              <Tooltip title={hasOpenJob ? 'This asset already has an open job' : isBlockedByReview ? 'Resolve review before queueing' : 'Queue asset'}>
                <IconButton
                  color="primary"
                  onClick={queueAsset}
                  disabled={createJob.isPending || !selectedProfileId || !selectedLibraryId || isBlockedByReview || hasOpenJob}
                  aria-label={`Queue ${asset.fileName}`}
                  sx={actionIconSx}
                >
                  <PlaylistAddIcon />
                </IconButton>
              </Tooltip>
            )}
          </Stack>
        </TableCell>
      </TableRow>
      {createJob.isSuccess ? (
        <TableRow>
          <TableCell colSpan={10} sx={{ bgcolor: 'rgba(102,217,168,0.05)' }}>
            <Alert severity="success">Asset queued individually.</Alert>
          </TableCell>
        </TableRow>
      ) : null}
      {createJob.isError ? (
        <TableRow>
          <TableCell colSpan={10} sx={{ bgcolor: 'rgba(246,180,75,0.05)' }}>
            <Alert severity="warning">Could not queue this asset.</Alert>
          </TableCell>
        </TableRow>
      ) : null}
      {isBlockedByReview || reviewReason || reviewTags.length > 0 ? (
        <TableRow>
          <TableCell colSpan={10} sx={{ bgcolor: 'rgba(246,180,75,0.05)' }}>
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
                {relativeAssetPath(asset, libraries)}
              </Typography>
              <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                {asset.path}
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
      <Dialog open={showSnapshotDialog} onClose={() => setShowSnapshotDialog(false)} maxWidth="md" fullWidth>
        <DialogTitle>Asset Snapshot</DialogTitle>
        <DialogContent>
          <Stack spacing={2}>
            <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
              <Stack sx={{ minWidth: 0 }}>
                <Typography fontWeight={700} sx={{ wordBreak: 'break-word' }}>
                  {relativeAssetPath(asset, libraries)}
                </Typography>
                <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                  {asset.path}
                </Typography>
              </Stack>
              <Button startIcon={<RefreshIcon />} variant="outlined" onClick={refreshSnapshot} disabled={snapshot.isPending}>
                Rescan
              </Button>
            </Stack>
            {snapshot.isPending ? <Alert severity="info">Reading asset snapshot...</Alert> : null}
            {snapshot.isError ? (
              <Alert severity="warning">Could not scan this asset. The file may not be readable from the backend container.</Alert>
            ) : null}
            {snapshot.data ? <MediaSnapshotDetails scan={snapshot.data} /> : null}
            {isConverted ? (
              <FinalDetailsSummary asset={asset} job={associatedJob} compact />
            ) : null}
          </Stack>
        </DialogContent>
      </Dialog>
      <Dialog open={showAdvisorDialog} onClose={() => setShowAdvisorDialog(false)} maxWidth="md" fullWidth>
        <DialogTitle>Advisor Score</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ pt: 1 }}>
            <Stack>
              <Typography fontWeight={700} sx={{ wordBreak: 'break-word' }}>
                {assetDisplayPath(asset, groupRelativePath, libraries)}
              </Typography>
              <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                {asset.path}
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
      {associatedJob ? (
        <JobDetailsDialog job={showFinalDetailsDialog ? associatedJob : null} onClose={() => setShowFinalDetailsDialog(false)} />
      ) : (
        <Dialog open={showFinalDetailsDialog} onClose={() => setShowFinalDetailsDialog(false)} maxWidth="md" fullWidth>
          <DialogTitle>Final Details</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ pt: 1 }}>
              <Stack>
                <Typography fontWeight={700} sx={{ wordBreak: 'break-word' }}>
                  {assetDisplayPath(asset, groupRelativePath, libraries)}
                </Typography>
                <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                  {asset.path}
                </Typography>
              </Stack>
              <FinalDetailsSummary asset={asset} job={associatedJob} />
            </Stack>
          </DialogContent>
        </Dialog>
      )}
    </>
  );
}

function AdvisorSummary({ advisor, audioProfile }: { advisor: AdvisorResponse; audioProfile?: AudioEnhancementProfile }) {
  const primaryAudio = advisor.scan.audioStreams[0];
  const outputCodec = audioProfile?.outputCodec || advisor.profile.audioCodec;
  const sourceCodec = primaryAudio?.codec || 'unknown';
  const audioCodecChanges = outputCodec && outputCodec !== 'copy' && outputCodec.toLowerCase() !== sourceCodec.toLowerCase();

  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, bgcolor: 'rgba(79,179,255,0.04)', p: 2 }}>
      <Stack spacing={2}>
        <Stack direction="row" spacing={2} alignItems="center" flexWrap="wrap" useFlexGap>
          <Chip label={recommendationLabel(advisor.recommendation)} color={recommendationColor(advisor.recommendation)} />
          <Typography variant="h2">{advisor.score}/100</Typography>
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
              {advisor.reasons.map((reason) => (
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
            {advisor.warnings.length ? (
              <Stack spacing={0.8}>
                {advisor.warnings.map((warning) => (
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

function FinalDetailsSummary({ asset, job, compact = false }: { asset: Asset; job?: QueueJob; compact?: boolean }) {
  const report = job?.validationReport ?? {};
  const mode = stringFromRecord(report, 'processingMode') || modeFromNotes(job?.notes ?? '');
  const audioProfile = job?.audioProfileKey || audioProfileFromNotes(job?.notes ?? '');
  const status = job?.publishedAt ? 'Published' : job?.status === 'completed' ? 'Converted' : job?.status || 'Converted';

  return (
    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, bgcolor: 'rgba(79,179,255,0.035)' }}>
      <Stack spacing={1.5}>
        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
          <Chip label={status} color={job?.status === 'failed' ? 'error' : 'success'} size="small" />
          {job ? <Chip label={`Job #${job.id}`} size="small" /> : null}
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
}: {
  profiles: Profile[];
  value: number;
  onChange: (id: number) => void;
  label: string;
  size?: 'small' | 'medium';
  disabled?: boolean;
}) {
  return (
    <Autocomplete
      options={profiles}
      value={profiles.find((profile) => profile.id === value) ?? null}
      onChange={(_, profile) => onChange(profile?.id ?? 0)}
      getOptionLabel={(profile) => `${profile.name} · ${profile.videoCodec}`}
      isOptionEqualToValue={(option, selected) => option.id === selected.id}
      filterOptions={(options, state) =>
        filterByText(options, state.inputValue, (profile) => [
          profile.name,
          profile.description,
          profile.container,
          profile.videoCodec,
          profile.audioCodec,
        ])
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
  return (
    <Autocomplete
      options={profiles}
      value={profiles.find((profile) => profile.key === value) ?? null}
      onChange={(_, profile) => onChange(profile?.key ?? '')}
      getOptionLabel={(profile) => `${profile.name} · ${profile.outputCodec || 'copy'}`}
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

function filterByText<T>(items: T[], inputValue: string, getValues: (item: T) => string[]) {
  const query = inputValue.trim().toLowerCase();
  if (!query) {
    return items.slice(0, 50);
  }

  return items
    .filter((item) => getValues(item).some((value) => value.toLowerCase().includes(query)))
    .slice(0, 50);
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

function firstCategory(categories?: string[]) {
  return categories?.find((category) => category.trim()) ?? '';
}

function assetDisplayPath(asset: Asset, groupRelativePath: string, libraries: Library[]) {
  const relativePath = relativeAssetPath(asset, libraries);
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

function relativeAssetPath(asset: Asset, libraries: Library[]) {
  if (asset.relativePath) {
    return asset.relativePath;
  }

  const library = libraries.find((candidate) => candidate.id === asset.libraryId);
  const basePath = asset.status === 'converted' ? library?.destinationPath : library?.sourcePath;

  if (!basePath) {
    return asset.fileName;
  }

  const normalizedBase = basePath.endsWith('/') ? basePath : `${basePath}/`;
  if (asset.path.startsWith(normalizedBase)) {
    return asset.path.slice(normalizedBase.length);
  }

  return asset.fileName;
}

function groupDisplayPath(group: AssetGroup) {
  if (group.relativePath) {
    return group.relativePath;
  }

  if (group.assets.length === 1) {
    return group.assets[0].fileName;
  }

  return 'Library root';
}

function firstAssetForGroup(group: AssetGroup) {
  return [...group.assets].sort((left, right) => left.path.localeCompare(right.path))[0];
}

function assetHasOpenJob(asset: Asset, jobs: QueueJob[]) {
  return jobs.some((job) => job.mediaPath === asset.path && job.status !== 'canceled' && !job.publishedAt);
}

function associatedJobForAsset(asset: Asset, jobs: QueueJob[]) {
  const normalizedAssetPath = normalizePath(asset.path);
  const assetFileName = asset.fileName.toLowerCase();
  return [...jobs]
    .sort((left, right) => right.id - left.id)
    .find((job) => {
      const candidates = [job.publishedPath, job.outputPath, job.mediaPath].map(normalizePath).filter(Boolean);
      if (candidates.includes(normalizedAssetPath)) {
        return true;
      }
      return candidates.some((candidate) => candidate.toLowerCase().endsWith(`/${assetFileName}`));
    });
}

function assetPipelineState(asset: Asset, job: QueueJob | undefined, pendingQueue: boolean): { label: string; color: 'default' | 'primary' | 'success' | 'warning' | 'error' } {
  if (pendingQueue) {
    return { label: 'Queueing', color: 'primary' };
  }
  if (job?.publishedAt) {
    return { label: 'Published', color: 'success' };
  }
  if (job?.status === 'running') {
    return { label: 'Worker', color: 'primary' };
  }
  if (job?.status === 'queued') {
    return { label: 'Queued', color: 'primary' };
  }
  if (job?.status === 'failed') {
    return { label: 'Failed', color: 'error' };
  }
  if (job?.status === 'canceled') {
    return { label: 'Canceled', color: 'default' };
  }
  if (job?.status === 'completed' && !job.validationStatus) {
    return { label: 'Analysis', color: 'warning' };
  }
  if (job?.validationStatus === 'passed' || job?.validationStatus === 'warning') {
    return { label: 'Publisher', color: 'primary' };
  }
  if (asset.status === 'converted') {
    return { label: 'Converted', color: 'success' };
  }
  return { label: 'Unprocessed', color: 'warning' };
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
