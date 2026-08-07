import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  Grid,
  IconButton,
  InputAdornment,
  MenuItem,
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
import AddIcon from '@mui/icons-material/Add';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import ArticleIcon from '@mui/icons-material/Article';
import InfoIcon from '@mui/icons-material/Info';
import SaveIcon from '@mui/icons-material/Save';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import { Link as RouterLink, useSearchParams } from 'react-router-dom';
import type { Dispatch, SetStateAction } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import type { HousekeepingReport, Library, RuntimeProfileOverride, RuntimeProfilesResponse, RuntimeProfileValues, SchedulerRecoveryReport } from '../api/types';

type LibraryType = {
  key: string;
  label: string;
  description: string;
  extensions: string[];
};

type LibraryTypeDraft = LibraryType & {
  extensionInput: string;
};

type WorkingWindow = { name: string; days: string[]; start: string; end: string };
type WorkingHoursValue = {
  enabled: boolean;
  timezone: string;
  preset: string;
  windows: WorkingWindow[];
  outsideWindowPolicy: { startNewHeavyJobs: boolean; continueRunningJobs: boolean; allowAnalysisJobs: boolean; allowValidationJobs: boolean; allowPublisherJobs: boolean; allowCleanupJobs: boolean; allowLabPreviews: boolean };
};
type WorkspaceValue = { preferredMode: 'copy_to_work_disk' | 'direct_mode'; fallbackMode: 'wait' | 'direct_mode'; allowDirectMode: boolean; estimateRequiredSpace: boolean };
type DirectPlayValue = { enabled: boolean; strategy: string; targetClients: string[]; minimumScore: number; enforcement: 'warn' | 'block' };
type HousekeepingValue = { autoEnabled: boolean; intervalHours: number; failedRetentionDays: number; canceledRetentionDays: number; orphanRetentionDays: number };
type RuntimePolicyValue = { schemaVersion: number; mode: 'automatic' | 'manual'; preferredProfile: string; fallbackProfile: string; overrides: Record<string, RuntimeProfileOverride> };

type SettingsForm = {
  paths: {
    rawRoot: string;
    libraryRoot: string;
    stagingPath: string;
    originalsArchivePath: string;
    asIsReportsPath: string;
    resultsReportsPath: string;
    logsPath: string;
  };
  workers: {
    defaultWorkerName: string;
    autoWorkerEnabled: boolean;
    maxConcurrentJobs: number;
    maxJobsPerBatch: number;
    delaySecondsBetweenJobs: number;
    batchCooldownSeconds: number;
    dryRunOnly: boolean;
  };
  pipelineAutomation: {
    autoAnalysisEnabled: boolean;
    reviewMode: 'manual' | 'automatic' | 'conditional';
    autoExecutionEnabled: boolean;
    autoValidationEnabled: boolean;
    autoPublisherEnabled: boolean;
    publishedJobReconciliationEnabled: boolean;
  };
  assetInventory: {
    autoSyncEnabled: boolean;
    syncIntervalMinutes: number;
    expireArchiveFiles: boolean;
    reconciliationMode: 'off' | 'review' | 'exact';
  };
  validation: {
    minimumScore: number;
    requireDurationMatch: boolean;
  };
  cancellationPolicy: {
    keepLogsAndDiagnostics: boolean;
    deleteGeneratedFiles: boolean;
    deletePartialOutputFromStaging: boolean;
    controlledRoots: string[];
  };
  originalRetentionPolicy: {
    keepOriginalsDays: number;
    enabledForSuccessfulConversionsOnly: boolean;
    autoDeleteEnabled: boolean;
    processedOriginalsPath: string;
  };
  assetTypes: LibraryType[];
  assetCategories: string[];
  advancedJson: {
    paths: string;
    workers: string;
    pipelineAutomation: string;
    assetInventory: string;
    validation: string;
    cancellationPolicy: string;
    originalRetentionPolicy: string;
    assetTypes: string;
    assetCategories: string;
  };
};

const defaultLibraryTypes: LibraryType[] = [
  { key: 'movies', label: 'Movies', description: 'Feature films and movie collections.', extensions: ['.mkv', '.mp4', '.avi', '.mov'] },
  { key: 'tv', label: 'TV Shows', description: 'Episode-based libraries.', extensions: ['.mkv', '.mp4'] },
  { key: 'anime', label: 'Anime', description: 'Anime movies and series.', extensions: ['.mkv', '.mp4'] },
  { key: 'music-videos', label: 'Music Videos', description: 'Music clips and short-form video.', extensions: ['.mkv', '.mp4', '.mov'] },
  { key: 'concerts', label: 'Concerts', description: 'Live performances and concert films.', extensions: ['.mkv', '.mp4'] },
  { key: 'home-videos', label: 'Home Videos', description: 'Personal and family media.', extensions: ['.mp4', '.mov'] },
];

const defaultAssetCategories = ['movie', 'anime', 'series', 'episode', 'season', 'extras', 'special', 'concert', 'music-video', 'documentary'];

const initialSettings: SettingsForm = buildSettingsForm({
  paths: {
    rawRoot: '/media/raw',
    libraryRoot: '/media/library',
    stagingPath: '/media/staging',
    originalsArchivePath: '/media/originals_archive',
    asIsReportsPath: '/media/reports/as-is',
    resultsReportsPath: '/media/reports/results',
    logsPath: '/media/reports/logs',
  },
  workers: {
    defaultWorkerName: 'local-worker',
    autoWorkerEnabled: true,
    maxConcurrentJobs: 1,
    maxJobsPerBatch: 10,
    delaySecondsBetweenJobs: 30,
    batchCooldownSeconds: 600,
    dryRunOnly: true,
  },
  pipelineAutomation: {
    autoAnalysisEnabled: false,
    reviewMode: 'conditional',
    autoExecutionEnabled: true,
    autoValidationEnabled: false,
    autoPublisherEnabled: false,
    publishedJobReconciliationEnabled: false,
  },
  assetInventory: {
    autoSyncEnabled: true,
    syncIntervalMinutes: 60,
    expireArchiveFiles: true,
    reconciliationMode: 'exact',
  },
  validation: { minimumScore: 90, requireDurationMatch: true },
  cancellationPolicy: {
    keepLogsAndDiagnostics: true,
    deleteGeneratedFiles: false,
    deletePartialOutputFromStaging: false,
    controlledRoots: ['/media/staging', '/mwp/work', '/mwp/work/temp', '/tmp/mvforge'],
  },
  originalRetentionPolicy: {
    keepOriginalsDays: 30,
    enabledForSuccessfulConversionsOnly: true,
    autoDeleteEnabled: false,
    processedOriginalsPath: '/media/originals_archive/processed-originals',
  },
  assetTypes: defaultLibraryTypes,
  assetCategories: defaultAssetCategories,
});

const emptyDraft: LibraryTypeDraft = {
  key: '',
  label: '',
  description: '',
  extensions: [],
  extensionInput: '',
};

export function SettingsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const runtimeSnapshot = useQuery({ queryKey: ['runtime-snapshot'], queryFn: api.runtimeSnapshot });
  const runtimeProfilesQuery = useQuery({ queryKey: ['runtime-profiles'], queryFn: api.runtimeProfiles });
  const schedulerRecovery = useQuery({ queryKey: ['scheduler-recovery'], queryFn: api.schedulerRecovery });
  const [tab, setTab] = useState<'general' | 'advanced'>('general');
  const requestedSection = searchParams.get('section');
  const [section, setSection] = useState<'overview' | 'pipeline' | 'runtime' | 'assets' | 'operations'>(() => requestedSection && ['pipeline', 'runtime', 'assets', 'operations'].includes(requestedSection) ? requestedSection as 'pipeline' | 'runtime' | 'assets' | 'operations' : 'overview');
  const changeSection = (next: 'overview' | 'pipeline' | 'runtime' | 'assets' | 'operations') => { setSection(next); setSearchParams(next === 'overview' ? {} : { section: next }); };
  const [form, setForm] = useState<SettingsForm>(initialSettings);
  const [showTypeForm, setShowTypeForm] = useState(false);
  const [editingTypeKey, setEditingTypeKey] = useState<string | null>(null);
  const [typeDraft, setTypeDraft] = useState<LibraryTypeDraft>(emptyDraft);
  const [deleteTarget, setDeleteTarget] = useState<LibraryType | null>(null);
  const [reassignTo, setReassignTo] = useState('');

  const librariesByType = useMemo(() => summarizeLibrariesByType(libraries.data ?? []), [libraries.data]);

  const updateSetting = useMutation({
    mutationFn: api.updateSetting,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
      await queryClient.invalidateQueries({ queryKey: ['libraries'] });
      await queryClient.invalidateQueries({ queryKey: ['runtime-profiles'] });
      await queryClient.invalidateQueries({ queryKey: ['runtime-snapshot'] });
    },
  });

  const updateLibrary = useMutation({
    mutationFn: api.updateLibrary,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['libraries'] });
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });

  const refreshRuntime = useMutation({
    mutationFn: api.refreshRuntimeSnapshot,
    onSuccess: async (snapshot) => {
      queryClient.setQueryData(['runtime-snapshot'], snapshot);
      await queryClient.invalidateQueries({ queryKey: ['runtime-snapshot'] });
    },
  });
  const runRecovery = useMutation({
    mutationFn: api.runSchedulerRecovery,
    onSuccess: async (report) => {
      queryClient.setQueryData(['scheduler-recovery'], report);
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
      await queryClient.invalidateQueries({ queryKey: ['workerNodes'] });
    },
  });
  const previewHousekeeping = useMutation({ mutationFn: api.previewHousekeeping });
  const runHousekeeping = useMutation({ mutationFn: api.runHousekeeping, onSuccess: () => previewHousekeeping.reset() });

  useEffect(() => {
    if (!settings.data) {
      return;
    }

    const timer = window.setTimeout(() => {
      setForm(settingsToForm(settings.data));
    }, 0);

    return () => window.clearTimeout(timer);
  }, [settings.data]);

  function saveSettings(nextForm = form) {
    updateSetting.mutate({ key: 'paths', value: nextForm.paths });
    updateSetting.mutate({ key: 'workers', value: nextForm.workers });
    updateSetting.mutate({ key: 'pipelineAutomation', value: nextForm.pipelineAutomation });
    updateSetting.mutate({ key: 'assetInventory', value: nextForm.assetInventory });
    updateSetting.mutate({ key: 'validation', value: nextForm.validation });
    updateSetting.mutate({ key: 'cancellationPolicy', value: nextForm.cancellationPolicy });
    updateSetting.mutate({ key: 'originalRetentionPolicy', value: nextForm.originalRetentionPolicy });
    updateSetting.mutate({ key: 'assetTypes', value: { types: nextForm.assetTypes } });
    updateSetting.mutate({ key: 'assetCategories', value: { categories: nextForm.assetCategories } });
  }

  function saveAdvancedSettings() {
    const paths = parseJsonObject(form.advancedJson.paths, form.paths);
    const workers = parseJsonObject(form.advancedJson.workers, form.workers);
    const pipelineAutomation = parseJsonObject(form.advancedJson.pipelineAutomation, form.pipelineAutomation);
    const assetInventory = parseJsonObject(form.advancedJson.assetInventory, form.assetInventory);
    const validation = parseJsonObject(form.advancedJson.validation, form.validation);
    const cancellationPolicy = parseJsonObject(form.advancedJson.cancellationPolicy, form.cancellationPolicy);
    const originalRetentionPolicy = parseJsonObject(
      form.advancedJson.originalRetentionPolicy,
      form.originalRetentionPolicy,
    );
    const assetTypesValue = parseJsonObject(form.advancedJson.assetTypes, { types: form.assetTypes });
    const assetCategoriesValue = parseJsonObject(form.advancedJson.assetCategories, { categories: form.assetCategories });
    const next = buildSettingsForm({
      paths: {
        rawRoot: stringValue(paths.rawRoot, initialSettings.paths.rawRoot),
        libraryRoot: stringValue(paths.libraryRoot, initialSettings.paths.libraryRoot),
        stagingPath: stringValue(paths.stagingPath, initialSettings.paths.stagingPath),
        originalsArchivePath: archivePathValue(
          paths.originalsArchivePath ?? paths.trashPath,
          initialSettings.paths.originalsArchivePath,
        ),
        asIsReportsPath: stringValue(paths.asIsReportsPath, initialSettings.paths.asIsReportsPath),
        resultsReportsPath: stringValue(paths.resultsReportsPath, initialSettings.paths.resultsReportsPath),
        logsPath: stringValue(paths.logsPath, initialSettings.paths.logsPath),
      },
      workers: {
        defaultWorkerName: stringValue(workers.defaultWorkerName, initialSettings.workers.defaultWorkerName),
        autoWorkerEnabled: booleanValue(workers.autoWorkerEnabled, initialSettings.workers.autoWorkerEnabled),
        maxConcurrentJobs: numberValue(workers.maxConcurrentJobs, initialSettings.workers.maxConcurrentJobs),
        maxJobsPerBatch: numberValue(workers.maxJobsPerBatch, initialSettings.workers.maxJobsPerBatch),
        delaySecondsBetweenJobs: numberValue(
          workers.delaySecondsBetweenJobs,
          initialSettings.workers.delaySecondsBetweenJobs,
        ),
        batchCooldownSeconds: numberValue(workers.batchCooldownSeconds, initialSettings.workers.batchCooldownSeconds),
        dryRunOnly: booleanValue(workers.dryRunOnly, initialSettings.workers.dryRunOnly),
      },
      pipelineAutomation: {
        autoAnalysisEnabled: booleanValue(
          pipelineAutomation.autoAnalysisEnabled,
          initialSettings.pipelineAutomation.autoAnalysisEnabled,
        ),
        reviewMode: stringValue(pipelineAutomation.reviewMode, initialSettings.pipelineAutomation.reviewMode) as SettingsForm['pipelineAutomation']['reviewMode'],
        autoExecutionEnabled: booleanValue(
          pipelineAutomation.autoExecutionEnabled,
          initialSettings.pipelineAutomation.autoExecutionEnabled,
        ),
        autoValidationEnabled: booleanValue(
          pipelineAutomation.autoValidationEnabled,
          initialSettings.pipelineAutomation.autoValidationEnabled,
        ),
        autoPublisherEnabled: booleanValue(
          pipelineAutomation.autoPublisherEnabled,
          initialSettings.pipelineAutomation.autoPublisherEnabled,
        ),
        publishedJobReconciliationEnabled: booleanValue(
          pipelineAutomation.publishedJobReconciliationEnabled,
          initialSettings.pipelineAutomation.publishedJobReconciliationEnabled,
        ),
      },
      assetInventory: {
        autoSyncEnabled: booleanValue(assetInventory.autoSyncEnabled, initialSettings.assetInventory.autoSyncEnabled),
        syncIntervalMinutes: numberValue(assetInventory.syncIntervalMinutes, initialSettings.assetInventory.syncIntervalMinutes),
        expireArchiveFiles: booleanValue(assetInventory.expireArchiveFiles, initialSettings.assetInventory.expireArchiveFiles),
        reconciliationMode: reconciliationModeValue(assetInventory.reconciliationMode),
      },
      validation: {
        minimumScore: numberValue(validation.minimumScore, initialSettings.validation.minimumScore),
        requireDurationMatch: booleanValue(
          validation.requireDurationMatch,
          initialSettings.validation.requireDurationMatch,
        ),
      },
      cancellationPolicy: {
        keepLogsAndDiagnostics: booleanValue(
          cancellationPolicy.keepLogsAndDiagnostics,
          initialSettings.cancellationPolicy.keepLogsAndDiagnostics,
        ),
        deleteGeneratedFiles: booleanValue(
          cancellationPolicy.deleteGeneratedFiles,
          initialSettings.cancellationPolicy.deleteGeneratedFiles,
        ),
        deletePartialOutputFromStaging: booleanValue(
          cancellationPolicy.deletePartialOutputFromStaging,
          initialSettings.cancellationPolicy.deletePartialOutputFromStaging,
        ),
        controlledRoots: normalizeStringArray(
          arrayValue(cancellationPolicy.controlledRoots, initialSettings.cancellationPolicy.controlledRoots),
          initialSettings.cancellationPolicy.controlledRoots,
        ),
      },
      originalRetentionPolicy: {
        keepOriginalsDays: numberValue(
          originalRetentionPolicy.keepOriginalsDays,
          initialSettings.originalRetentionPolicy.keepOriginalsDays,
        ),
        processedOriginalsPath: archivePathValue(
          originalRetentionPolicy.processedOriginalsPath,
          initialSettings.originalRetentionPolicy.processedOriginalsPath,
        ),
        enabledForSuccessfulConversionsOnly: booleanValue(
          originalRetentionPolicy.enabledForSuccessfulConversionsOnly,
          initialSettings.originalRetentionPolicy.enabledForSuccessfulConversionsOnly,
        ),
        autoDeleteEnabled: booleanValue(
          originalRetentionPolicy.autoDeleteEnabled,
          initialSettings.originalRetentionPolicy.autoDeleteEnabled,
        ),
      },
      assetTypes: normalizeLibraryTypes(arrayValue(assetTypesValue.types, form.assetTypes)),
      assetCategories: normalizeStringArray(
        arrayValue(assetCategoriesValue.categories, form.assetCategories),
        form.assetCategories,
      ),
    });

    setForm(next);
    saveSettings(next);
  }

  function openAddType() {
    setEditingTypeKey(null);
    setTypeDraft(emptyDraft);
    setShowTypeForm(true);
  }

  function openEditType(type: LibraryType) {
    setEditingTypeKey(type.key);
    setTypeDraft({ ...type, extensionInput: '' });
    setShowTypeForm(true);
  }

  function submitType(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const key = editingTypeKey ?? slugify(typeDraft.key || typeDraft.label);
    const nextType = {
      key,
      label: typeDraft.label.trim(),
      description: typeDraft.description.trim(),
      extensions: typeDraft.extensions,
    };

    if (!nextType.label || !nextType.key) {
      return;
    }

    const nextTypes = editingTypeKey
      ? form.assetTypes.map((type) => (type.key === editingTypeKey ? nextType : type))
      : [...form.assetTypes, nextType];

    const next = rebuildAssetTypes(form, nextTypes);
    setForm(next);
    saveSettings(next);
    setShowTypeForm(false);
    setEditingTypeKey(null);
    setTypeDraft(emptyDraft);
  }

  function addExtension() {
    const extension = normalizeExtension(typeDraft.extensionInput);
    if (!extension || typeDraft.extensions.includes(extension)) {
      setTypeDraft((current) => ({ ...current, extensionInput: '' }));
      return;
    }

    setTypeDraft((current) => ({
      ...current,
      extensionInput: '',
      extensions: [...current.extensions, extension],
    }));
  }

  function removeExtension(extension: string) {
    setTypeDraft((current) => ({
      ...current,
      extensions: current.extensions.filter((candidate) => candidate !== extension),
    }));
  }

  function openDeleteType(type: LibraryType) {
    setDeleteTarget(type);
    setReassignTo(form.assetTypes.find((candidate) => candidate.key !== type.key)?.key ?? '');
  }

  function confirmDeleteType() {
    if (!deleteTarget) {
      return;
    }

    const affectedLibraries = (libraries.data ?? []).filter((library) => library.type === deleteTarget.key);
    if (affectedLibraries.length > 0 && !reassignTo) {
      return;
    }

    affectedLibraries.forEach((library) => {
      updateLibrary.mutate({ ...library, type: reassignTo });
    });

    const next = rebuildAssetTypes(
      form,
      form.assetTypes.filter((type) => type.key !== deleteTarget.key),
    );
    setForm(next);
    saveSettings(next);
    setDeleteTarget(null);
    setReassignTo('');
  }

  return (
    <>
      <PageHeader title="Settings" eyebrow="System configuration">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 780 }}>
          Simple defaults stay up front; technical JSON and runtime details stay available when needed.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {settings.isError ? <Alert severity="warning">Unable to load settings.</Alert> : null}
        {updateSetting.isSuccess ? (
          <Alert severity="success" sx={{ mb: 2 }}>
            Settings saved.
          </Alert>
        ) : null}
        {updateSetting.isError || updateLibrary.isError ? (
          <Alert severity="warning" sx={{ mb: 2 }}>
            Settings could not be fully saved.
          </Alert>
        ) : null}

        <Card sx={{ mb: 2 }}>
          <CardContent sx={{ pb: 0 }}>
            <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={1}>
              <Tabs value={tab} onChange={(_, value) => setTab(value)} sx={{ minHeight: 44 }}>
                <Tab label="General" value="general" />
                <Tab label="Advanced" value="advanced" />
              </Tabs>
            </Stack>
          </CardContent>
        </Card>

        {tab === 'general' ? (
          <Stack spacing={2}>
            <SettingsDomainNavigation section={section} onChange={changeSection} />

            {section === 'assets' ? (
              <Stack spacing={2}>
                <AssetCategoriesPanel
                  categories={form.assetCategories}
                  onChange={(categories) =>
                    setForm((current) => buildSettingsForm({ ...current, assetCategories: normalizeStringArray(categories, current.assetCategories) }))
                  }
                />
                <LibraryTypesPanel
                  assetTypes={form.assetTypes}
                  librariesByType={librariesByType}
                  onAdd={openAddType}
                  onEdit={openEditType}
                  onDelete={openDeleteType}
                />
                <Grid container spacing={2}>
                  <Grid size={{ xs: 12, md: 6 }}>
                    <PathsCard form={form} setForm={setForm} />
                  </Grid>
                </Grid>
                <StorageWorkspaceCard
                  roles={storageRolesValue(settings.data?.find((item) => item.key === 'storageRoles')?.value)}
                  workspace={workspaceValue(settings.data?.find((item) => item.key === 'workspace')?.value)}
                  saving={updateSetting.isPending}
                  onSave={(roles, workspace) => {
                    updateSetting.mutate({ key: 'storageRoles', value: roles });
                    updateSetting.mutate({ key: 'workspace', value: workspace as unknown as Record<string, unknown> });
                  }}
                />
              </Stack>
            ) : null}

            <Dialog open={showTypeForm} onClose={() => setShowTypeForm(false)} maxWidth="md" fullWidth>
              <DialogTitle>{editingTypeKey ? 'Edit Library Type' : 'New Library Type'}</DialogTitle>
              <DialogContent>
                  <Box component="form" onSubmit={submitType}>
                    <Stack spacing={2}>
                      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                        <Stack>
                          <Typography color="text.secondary" variant="body2">
                            Use a friendly name. The internal key remains stable once the type is created.
                          </Typography>
                        </Stack>
                        <Button startIcon={<CloseIcon />} onClick={() => setShowTypeForm(false)}>
                          Close
                        </Button>
                      </Stack>
                      <Grid container spacing={2}>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Name"
                            value={typeDraft.label}
                            onChange={(event) =>
                              setTypeDraft((current) => ({
                                ...current,
                                label: event.target.value,
                                key: editingTypeKey ? current.key : slugify(event.target.value),
                              }))
                            }
                            required
                            fullWidth
                          />
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Key"
                            value={editingTypeKey ?? typeDraft.key}
                            disabled={Boolean(editingTypeKey)}
                            onChange={(event) =>
                              setTypeDraft((current) => ({ ...current, key: slugify(event.target.value) }))
                            }
                            required
                            fullWidth
                          />
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <Stack direction="row" spacing={1}>
                            <TextField
                              label="Extension"
                              value={typeDraft.extensionInput}
                              onChange={(event) =>
                                setTypeDraft((current) => ({ ...current, extensionInput: event.target.value }))
                              }
                              onKeyDown={(event) => {
                                if (event.key === 'Enter') {
                                  event.preventDefault();
                                  addExtension();
                                }
                              }}
                              placeholder=".mkv"
                              fullWidth
                            />
                            <Button variant="outlined" onClick={addExtension} sx={{ minWidth: 52 }}>
                              <AddIcon />
                            </Button>
                          </Stack>
                        </Grid>
                        <Grid size={{ xs: 12 }}>
                          <TextField
                            label="Description"
                            value={typeDraft.description}
                            onChange={(event) =>
                              setTypeDraft((current) => ({ ...current, description: event.target.value }))
                            }
                            fullWidth
                          />
                        </Grid>
                        <Grid size={{ xs: 12 }}>
                          <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                            {typeDraft.extensions.map((extension) => (
                              <Chip key={extension} label={extension} onDelete={() => removeExtension(extension)} />
                            ))}
                            {typeDraft.extensions.length === 0 ? (
                              <Typography color="text.secondary" variant="body2">
                                Add one or more extensions used by this library type.
                              </Typography>
                            ) : null}
                          </Stack>
                        </Grid>
                        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                          <Button
                            type="submit"
                            startIcon={<SaveIcon />}
                            variant="contained"
                            disabled={updateSetting.isPending}
                            fullWidth
                          >
                            Save Type
                          </Button>
                        </Grid>
                      </Grid>
                    </Stack>
                  </Box>
              </DialogContent>
            </Dialog>

            {section === 'pipeline' ? (
              <Grid container spacing={2}>
                <Grid size={{ xs: 12, md: 6 }}>
                  <WorkersCard form={form} setForm={setForm} />
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <PipelineAutomationCard form={form} setForm={setForm} />
                </Grid>
              </Grid>
            ) : null}

            {section === 'pipeline' ? (
              <Grid container spacing={2}>
                <Grid size={{ xs: 12, md: 6 }}>
                  <ValidationCard form={form} setForm={setForm} />
                </Grid>
              </Grid>
            ) : null}

            {section === 'runtime' ? (
              <RuntimeProfilesCard
                catalog={runtimeProfilesQuery.data}
                value={runtimePolicyValue(settings.data?.find((item) => item.key === 'runtimePolicy')?.value)}
                detectedProfile={runtimeSnapshot.data?.recommendedProfile ?? 'desktop_safe'}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'runtimePolicy', value: value as unknown as Record<string, unknown> })}
              />
            ) : null}

            {section === 'pipeline' ? (
              <DirectPlayCard
                value={directPlayValue(settings.data?.find((item) => item.key === 'directPlay')?.value)}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'directPlay', value: value as unknown as Record<string, unknown> })}
              />
            ) : null}

            {section === 'pipeline' ? (
              <WorkingHoursCard
                value={workingHoursValue(settings.data?.find((item) => item.key === 'workingHours')?.value)}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'workingHours', value: value as unknown as Record<string, unknown> })}
              />
            ) : null}

            {section === 'runtime' ? (
              <Grid container spacing={2}>
                <Grid size={{ xs: 12 }}>
                  <SchedulerRecoveryCard report={schedulerRecovery.data} loading={schedulerRecovery.isLoading} error={schedulerRecovery.isError} running={runRecovery.isPending} runError={runRecovery.error} onRun={() => runRecovery.mutate()} />
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <CancellationPolicyCard form={form} setForm={setForm} />
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <OriginalRetentionCard form={form} setForm={setForm} />
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <HousekeepingCard
                    value={housekeepingValue(settings.data?.find((item) => item.key === 'housekeeping')?.value)}
                    saving={updateSetting.isPending}
                    preview={previewHousekeeping.data}
                    result={runHousekeeping.data}
                    busy={previewHousekeeping.isPending || runHousekeeping.isPending}
                    onSave={(value) => updateSetting.mutate({ key: 'housekeeping', value: value as unknown as Record<string, unknown> })}
                    onPreview={() => { runHousekeeping.reset(); previewHousekeeping.mutate(); }}
                    onRun={() => runHousekeeping.mutate()}
                  />
                </Grid>
              </Grid>
            ) : null}

            {section === 'operations' ? (
              <Grid container spacing={2}>
                <Grid size={{ xs: 12 }}>
                  <Card>
                    <CardContent>
                      <Stack spacing={1.5}>
                        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                          <Stack>
                            <Typography variant="h3">Scheduler runtime</Typography>
                            <Typography color="text.secondary" variant="body2">
                              Persisted machine snapshot used to select the execution policy.
                            </Typography>
                          </Stack>
                          <Button onClick={() => refreshRuntime.mutate()} disabled={refreshRuntime.isPending}>
                            {refreshRuntime.isPending ? 'Detecting…' : 'Refresh detection'}
                          </Button>
                        </Stack>
                        {runtimeSnapshot.isError ? (
                          <Alert severity="warning">
                            Unable to load runtime detection: {runtimeSnapshot.error instanceof Error ? runtimeSnapshot.error.message : 'unknown error'}
                          </Alert>
                        ) : runtimeSnapshot.isLoading ? (
                          <Typography color="text.secondary">Detecting runtime capabilities…</Typography>
                        ) : runtimeSnapshot.data ? (
                          <Stack spacing={2}>
                            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                              <Chip label={`Effective: ${runtimeSnapshot.data.selectedProfile}`} color="primary" />
                              <Chip label={`Detected: ${runtimeSnapshot.data.recommendedProfile}`} />
                              <Chip label={`Preferred: ${runtimeSnapshot.data.preferredProfile || 'auto'}`} />
                              {runtimeSnapshot.data.appliedOverrides.length ? <Chip label={`${runtimeSnapshot.data.appliedOverrides.length} overrides`} color="warning" /> : null}
                              <Chip label={`${runtimeSnapshot.data.os}/${runtimeSnapshot.data.architecture}`} />
                              <Chip label={`${runtimeSnapshot.data.cpuCores} CPU cores`} />
                              <Chip label={`Load ${runtimeSnapshot.data.cpuLoad1.toFixed(2)}`} />
                              <Chip label={`${formatBytes(runtimeSnapshot.data.totalMemoryBytes)} RAM total`} />
                              <Chip label={`${formatBytes(runtimeSnapshot.data.availableMemoryBytes)} RAM available`} />
                              <Chip label={runtimeSnapshot.data.container ? 'Container' : 'Native host'} />
                              <Chip label={runtimeSnapshot.data.batteryPresent ? `${runtimeSnapshot.data.onBattery ? 'Battery' : 'AC'} · ${runtimeSnapshot.data.batteryPercent}%` : `Power: ${runtimeSnapshot.data.powerSource}`} color={runtimeSnapshot.data.onBattery ? 'warning' : 'default'} />
                            </Stack>

                            <Divider />
                            <Typography variant="h4">Storage</Typography>
                            <Table size="small">
                              <TableHead><TableRow><TableCell>Role</TableCell><TableCell>Path</TableCell><TableCell>Type</TableCell><TableCell>Total</TableCell><TableCell>Available</TableCell></TableRow></TableHead>
                              <TableBody>
                                {Object.entries(safeRuntimeRecord(runtimeSnapshot.data.disks)).map(([role, rawDisk]) => {
                                  const disk = runtimeDisk(rawDisk);
                                  return <TableRow key={role}><TableCell>{role}</TableCell><TableCell sx={{ wordBreak: 'break-all' }}>{disk.path}</TableCell><TableCell>{disk.type}</TableCell><TableCell>{formatBytes(disk.totalBytes)}</TableCell><TableCell>{formatBytes(disk.availableBytes)}</TableCell></TableRow>;
                                })}
                              </TableBody>
                            </Table>

                            <Typography variant="h4">FFmpeg encoders</Typography>
                            <Table size="small">
                              <TableHead><TableRow><TableCell>Encoder</TableCell><TableCell>Listed</TableCell><TableCell>Usable</TableCell><TableCell>Verified combinations</TableCell><TableCell>Diagnostic</TableCell></TableRow></TableHead>
                              <TableBody>
                                {Object.entries(safeRuntimeRecord(runtimeSnapshot.data.encoders)).map(([name, rawEncoder]) => {
                                  const encoder = runtimeEncoder(rawEncoder);
                                  const modes = runtimeEncoderCapabilities(name, rawEncoder);
                                  const failedModes = modes.filter((mode) => !mode.passed && mode.reason);
                                  return <TableRow key={name}><TableCell>{name}</TableCell><TableCell>{encoder.listed ? 'Yes' : 'No'}</TableCell><TableCell><Chip size="small" color={encoder.usable ? 'success' : 'default'} label={encoder.usable ? 'Usable' : 'Unavailable'} /></TableCell><TableCell><Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>{modes.map((mode) => <Chip key={mode.key} size="small" variant="outlined" color={mode.passed ? 'success' : 'warning'} label={`${mode.label} · ${mode.passed ? 'passed' : 'failed'}`} title={mode.reason || undefined} />)}{modes.length === 0 ? '—' : null}</Stack></TableCell>
                                  <TableCell>
                                    <Stack spacing={0.5}>
                                      {encoder.reason ? (
                                        <Typography variant="body2">{encoder.reason}</Typography>
                                      ) : failedModes.length ? (
                                          failedModes.map((mode) => (
                                          <Tooltip key={mode.key} title={mode.reason || ''} arrow>
                                            <Typography
                                              variant="caption"
                                              color="text.secondary"
                                              sx={{ display: 'block', cursor: mode.reason ? 'help' : 'default' }}
                                            >
                                              <strong>{mode.label}:</strong>{' '}
                                              {summarizeModeReason(mode.reason)}
                                            </Typography>
                                          </Tooltip>
                                        ))
                                      ) : (
                                        <Typography variant="body2">Passed capability check</Typography>
                                      )}
                                    </Stack>
                                  </TableCell>
                                  </TableRow>;
                                })}
                              </TableBody>
                            </Table>

                            {safeRuntimeList(runtimeSnapshot.data.selectionReasons).length ? <Alert severity="info">{safeRuntimeList(runtimeSnapshot.data.selectionReasons).map(String).join(' · ')}</Alert> : null}
                            {safeRuntimeList(runtimeSnapshot.data.warnings).map((warning, index) => <Alert key={index} severity="warning">{String(warning)}</Alert>)}
                            <Typography color="text.secondary" variant="caption">Detected {new Date(runtimeSnapshot.data.detectedAt).toLocaleString()}</Typography>
                          </Stack>
                        ) : (
                          <Typography color="text.secondary">No runtime snapshot has been recorded yet.</Typography>
                        )}
                        {refreshRuntime.isSuccess ? <Alert severity="success">Runtime refreshed successfully. Snapshot #{refreshRuntime.data.id} was recorded.</Alert> : null}
                        {refreshRuntime.isError ? <Alert severity="error">Refresh failed: {refreshRuntime.error instanceof Error ? refreshRuntime.error.message : 'unknown error'}</Alert> : null}
                      </Stack>
                    </CardContent>
                  </Card>
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <Card>
                    <CardContent>
                      <Stack spacing={1.5}>
                        <ArticleIcon color="primary" />
                        <Typography variant="h3">Logs</Typography>
                        <Typography color="text.secondary">
                          Open generated worker, validation, publishing, and diagnostics log files.
                        </Typography>
                        <Button component={RouterLink} to="/logs" variant="contained" sx={{ alignSelf: 'flex-start' }}>
                          Open Logs
                        </Button>
                      </Stack>
                    </CardContent>
                  </Card>
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <Card>
                    <CardContent>
                      <Stack spacing={1.5}>
                        <InfoIcon color="primary" />
                        <Typography variant="h3">Versions</Typography>
                        <Typography color="text.secondary">
                          Review MVForge, frontend, backend, FFmpeg, and dependency versions.
                        </Typography>
                        <Button component={RouterLink} to="/versions" variant="contained" sx={{ alignSelf: 'flex-start' }}>
                          Open Versions
                        </Button>
                      </Stack>
                    </CardContent>
                  </Card>
                </Grid>
              </Grid>
            ) : null}

            <Button
              startIcon={<SaveIcon />}
              variant="contained"
              onClick={() => saveSettings()}
              disabled={updateSetting.isPending}
              sx={{ alignSelf: 'flex-start' }}
            >
              Save Settings
            </Button>
          </Stack>
        ) : (
          <AdvancedSettings form={form} setForm={setForm} onSave={saveAdvancedSettings} isSaving={updateSetting.isPending} />
        )}
      </Box>

      <Dialog open={Boolean(deleteTarget)} onClose={() => setDeleteTarget(null)} maxWidth="sm" fullWidth>
        <DialogTitle>Delete Library Type</DialogTitle>
        <DialogContent>
          <Stack spacing={2} sx={{ mt: 1 }}>
            <Alert severity={librariesByType[deleteTarget?.key ?? ''] ? 'warning' : 'info'}>
              {librariesByType[deleteTarget?.key ?? '']
                ? `${librariesByType[deleteTarget?.key ?? '']} libraries use this type. Reassign them before deleting.`
                : 'No libraries currently use this type.'}
            </Alert>
            {librariesByType[deleteTarget?.key ?? ''] ? (
              <TextField
                label="Reassign libraries to"
                value={reassignTo}
                onChange={(event) => setReassignTo(event.target.value)}
                select
                fullWidth
              >
                {form.assetTypes
                  .filter((type) => type.key !== deleteTarget?.key)
                  .map((type) => (
                    <MenuItem key={type.key} value={type.key}>
                      {type.label}
                    </MenuItem>
                  ))}
              </TextField>
            ) : null}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteTarget(null)}>Cancel</Button>
          <Button
            color="error"
            startIcon={<DeleteIcon />}
            onClick={confirmDeleteType}
            disabled={Boolean(librariesByType[deleteTarget?.key ?? '']) && !reassignTo}
          >
            Delete Type
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}

function SettingsDomainNavigation({ section, onChange }: { section: 'overview' | 'pipeline' | 'runtime' | 'assets' | 'operations'; onChange: (section: 'overview' | 'pipeline' | 'runtime' | 'assets' | 'operations') => void }) {
  const domains = [
    { key: 'pipeline' as const, title: 'Pipeline', description: 'Scheduler workflow, workers, automation, working hours, DirectPlay and validation.', color: 'primary.main' },
    { key: 'runtime' as const, title: 'Runtime', description: 'Effective runtime profiles, host detection, resources, disks, power and FFmpeg encoders.', color: 'info.main' },
    { key: 'assets' as const, title: 'Assets & Storage', description: 'Asset types, categories, controlled paths, storage roles and workspace strategy.', color: 'success.main' },
    { key: 'operations' as const, title: 'Operations', description: 'Recovery, cleanup, retention, runtime diagnostics and system-wide logs.', color: 'warning.main' },
  ];
  return <Stack spacing={2}>
    {section !== 'overview' ? <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={1}><Stack><Typography variant="h2">{domains.find((domain) => domain.key === section)?.title}</Typography><Typography color="text.secondary">{domains.find((domain) => domain.key === section)?.description}</Typography></Stack><Button onClick={() => onChange('overview')}>Back to settings dashboard</Button></Stack> : <Stack><Typography variant="h2">Configuration dashboard</Typography><Typography color="text.secondary">Choose an operational area. Related controls stay together on one page.</Typography></Stack>}
    {section === 'overview' ? <Grid container spacing={2}>{domains.map((domain) => <Grid size={{ xs: 12, sm: 6, lg: 3 }} key={domain.key}><Card sx={{ height: '100%', borderTop: 3, borderTopColor: domain.color }}><CardContent><Stack spacing={1.5} sx={{ height: '100%' }}><Typography variant="h3">{domain.title}</Typography><Typography color="text.secondary" sx={{ flex: 1 }}>{domain.description}</Typography><Button variant="contained" onClick={() => onChange(domain.key)}>Open {domain.title}</Button>{domain.key === 'operations' ? <Button component={RouterLink} to="/logs">Open logs directly</Button> : null}</Stack></CardContent></Card></Grid>)}</Grid> : null}
  </Stack>;
}

function SchedulerRecoveryCard({ report, loading, error, running, runError, onRun }: { report?: SchedulerRecoveryReport; loading: boolean; error: boolean; running: boolean; runError: Error | null; onRun: () => void }) {
  return <Card><CardContent><Stack spacing={1.5}>
    <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
      <Stack><Typography variant="h3">Scheduler recovery</Typography><Typography color="text.secondary" variant="body2">Conservative reconciliation for jobs, reservations, workers and workspaces after a restart.</Typography></Stack>
      <Button onClick={onRun} disabled={running}>{running ? 'Reconciling…' : 'Run reconciliation'}</Button>
    </Stack>
    {report ? <><Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap><Chip label={`${report.interruptedJobs} interrupted jobs`} /><Chip label={`${report.reservationsReleased} reservations released`} /><Chip label={`${report.workersMarkedOffline} workers offline`} /><Chip label={`${report.partialOutputsPreserved} partial outputs preserved`} /><Chip label={`${report.orphanWorkspacePaths.length} orphan workspaces`} color={report.orphanWorkspacePaths.length ? 'warning' : 'success'} /><Chip label={`${report.missingCompletedOutputs} missing outputs`} color={report.missingCompletedOutputs ? 'error' : 'default'} /></Stack>{report.orphanWorkspacePaths.map((path) => <Alert severity="warning" key={path}>{path}</Alert>)}<Typography color="text.secondary" variant="caption">Last reconciliation: {new Date(report.ranAt).toLocaleString()}</Typography></> : error ? <Alert severity="warning">No recovery report is available.</Alert> : loading ? <Typography color="text.secondary">Loading recovery report…</Typography> : null}
    {runError ? <Alert severity="error">Recovery failed: {runError.message}</Alert> : null}
  </Stack></CardContent></Card>;
}

function AssetCategoriesPanel({ categories, onChange }: { categories: string[]; onChange: (categories: string[]) => void }) {
  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Asset Categories</Typography>
            <Typography color="text.secondary" variant="body2">
              Define controlled labels for classifying assets and groups. These are separate from library destination types.
            </Typography>
          </Stack>
          <Autocomplete
            multiple
            freeSolo
            options={defaultAssetCategories}
            value={categories}
            onChange={(_, value) => onChange(value.map((category) => category.trim()).filter(Boolean))}
            renderTags={(value, getTagProps) =>
              value.map((option, index) => {
                const { key, ...tagProps } = getTagProps({ index });
                return <Chip key={key} label={option} size="small" {...tagProps} />;
              })
            }
            renderInput={(params) => (
              <TextField {...params} label="Controlled asset categories" placeholder="Add category" helperText="Examples: anime, movie, extras, episode, season." />
            )}
            fullWidth
          />
        </Stack>
      </CardContent>
    </Card>
  );
}

function LibraryTypesPanel({
  assetTypes,
  librariesByType,
  onAdd,
  onEdit,
  onDelete,
}: {
  assetTypes: LibraryType[];
  librariesByType: Record<string, number>;
  onAdd: () => void;
  onEdit: (type: LibraryType) => void;
  onDelete: (type: LibraryType) => void;
}) {
  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
            <Stack>
              <Typography variant="h3">Library Types</Typography>
              <Typography color="text.secondary" variant="body2">
                Define the friendly categories used when creating libraries.
              </Typography>
            </Stack>
            <Button startIcon={<AddIcon />} variant="contained" onClick={onAdd}>
              Add Type
            </Button>
          </Stack>
          <Table sx={{ tableLayout: 'fixed' }}>
            <TableHead>
              <TableRow>
                <TableCell>Type</TableCell>
                <TableCell>Extensions</TableCell>
                <TableCell>Libraries</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {assetTypes.map((type) => (
                <TableRow key={type.key} hover>
                  <TableCell>
                    <Stack spacing={0.4}>
                      <Typography fontWeight={700}>{type.label}</Typography>
                      <Typography color="text.secondary" variant="body2">
                        {type.key}
                      </Typography>
                      {type.description ? (
                        <Typography color="text.secondary" variant="body2">
                          {type.description}
                        </Typography>
                      ) : null}
                    </Stack>
                  </TableCell>
                  <TableCell>
                    <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                      {type.extensions.map((extension) => (
                        <Chip key={extension} label={extension} size="small" />
                      ))}
                    </Stack>
                  </TableCell>
                  <TableCell>{librariesByType[type.key] ?? 0}</TableCell>
                  <TableCell align="right">
                    <Stack direction="row" spacing={1} justifyContent="flex-end">
                      <Tooltip title="Edit type">
                        <IconButton onClick={() => onEdit(type)} aria-label={`Edit ${type.label}`}>
                          <EditIcon />
                        </IconButton>
                      </Tooltip>
                      <Tooltip title="Delete type">
                        <IconButton color="error" onClick={() => onDelete(type)} aria-label={`Delete ${type.label}`}>
                          <DeleteIcon />
                        </IconButton>
                      </Tooltip>
                    </Stack>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Stack>
      </CardContent>
    </Card>
  );
}

const directPlayClients = ['jellyfin_web', 'jellyfin_android_tv', 'jellyfin_roku', 'jellyfin_webos', 'apple_tv'];

function housekeepingValue(value: Record<string, unknown> | undefined): HousekeepingValue {
  return {
    autoEnabled: booleanValue(value?.autoEnabled, true), intervalHours: numberValue(value?.intervalHours, 24),
    failedRetentionDays: numberValue(value?.failedRetentionDays, 7), canceledRetentionDays: numberValue(value?.canceledRetentionDays, 3),
    orphanRetentionDays: numberValue(value?.orphanRetentionDays, 7),
  };
}

function runtimePolicyValue(value: Record<string, unknown> | undefined): RuntimePolicyValue {
  const overrides = value?.overrides && typeof value.overrides === 'object' ? value.overrides as Record<string, RuntimeProfileOverride> : {};
  const legacySelected = stringValue(value?.selectedProfile, 'desktop_balanced');
  return { schemaVersion: 2, mode: value?.mode === 'manual' ? 'manual' : 'automatic', preferredProfile: stringValue(value?.preferredProfile, value?.mode === 'manual' ? legacySelected : 'auto'), fallbackProfile: stringValue(value?.fallbackProfile, 'desktop_safe'), overrides };
}

const runtimeNumericFields: Array<{ key: keyof RuntimeProfileValues; label: string; suffix?: string }> = [
  { key: 'maxRunningJobs', label: 'Max running jobs' }, { key: 'maxVideoJobs', label: 'Max video jobs' },
  { key: 'maxSoftwareX265Jobs', label: 'Max software x265 jobs' }, { key: 'maxHardwareEncodeJobs', label: 'Max hardware encode jobs' },
  { key: 'maxAudioJobs', label: 'Max audio jobs' }, { key: 'maxLabJobs', label: 'Max Lab jobs' },
  { key: 'minFreeRamGb', label: 'Minimum free RAM', suffix: 'GB' }, { key: 'minFreeWorkGb', label: 'Work disk reserve', suffix: 'GB' },
  { key: 'minFreeLibraryGb', label: 'Library reserve', suffix: 'GB' }, { key: 'maxWorkspaceGb', label: 'Maximum workspace', suffix: 'GB' },
];

function RuntimeProfilesCard({ catalog, value, detectedProfile, saving, onSave }: { catalog?: RuntimeProfilesResponse; value: RuntimePolicyValue; detectedProfile: string; saving: boolean; onSave: (value: RuntimePolicyValue) => void }) {
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [JSON.stringify(value)]);
  if (!catalog) return <Card><CardContent><Typography color="text.secondary">Loading runtime profiles…</Typography></CardContent></Card>;
  const selectedKey = draft.preferredProfile === 'auto' ? detectedProfile : draft.preferredProfile;
  const selected = catalog.profiles.find((profile) => profile.key === selectedKey) ?? catalog.profiles[0];
  if (!selected) return <Alert severity="error">No default runtime profiles are available.</Alert>;
  const override = draft.overrides[selected.key] ?? {};
  const effective = { ...selected.values, ...override };
  const setOverride = (key: keyof RuntimeProfileValues, next: number | boolean) => setDraft((current) => ({ ...current, overrides: { ...current.overrides, [selected.key]: { ...(current.overrides[selected.key] ?? {}), [key]: next } } }));
  const overridden = (key: keyof RuntimeProfileValues) => Object.prototype.hasOwnProperty.call(override, key);
  const numericField = ({ key, label, suffix }: { key: keyof RuntimeProfileValues; label: string; suffix?: string }) => { const changed = overridden(key); return <Stack spacing={1} sx={{ border: 1, borderColor: changed ? 'warning.main' : 'divider', bgcolor: changed ? 'rgba(255, 167, 38, 0.05)' : 'transparent', borderRadius: 1, p: 1.25, height: '100%' }}><Stack direction="row" spacing={0.5} alignItems="center"><Typography variant="body2" fontWeight={700}>{label}</Typography>{changed ? <Tooltip title="Changed from the profile default"><EditIcon color="warning" sx={{ fontSize: 16 }} /></Tooltip> : null}</Stack><TextField type="number" size="small" value={Number(effective[key])} inputProps={{ min: 1 }} InputProps={suffix ? { endAdornment: <InputAdornment position="end">{suffix}</InputAdornment> } : undefined} onChange={(event) => setOverride(key, Math.max(1, Number(event.target.value)))} fullWidth /></Stack>; };
  const booleanField = (key: 'allowDirectMode' | 'pauseWhenOnBattery' | 'preventSleepDuringJobs', label: string) => { const changed = overridden(key); return <Stack spacing={0.75} sx={{ border: 1, borderColor: changed ? 'warning.main' : 'divider', bgcolor: changed ? 'rgba(255, 167, 38, 0.05)' : 'transparent', borderRadius: 1, px: 1.25, py: 1, height: '100%' }}><Stack direction="row" spacing={0.5} alignItems="center"><Typography variant="body2" fontWeight={700}>{label}</Typography>{changed ? <Tooltip title="Changed from the profile default"><EditIcon color="warning" sx={{ fontSize: 16 }} /></Tooltip> : null}</Stack><Switch size="small" checked={Boolean(effective[key])} onChange={(event) => setOverride(key, event.target.checked)} sx={{ alignSelf: 'flex-start' }} /></Stack>; };
  return <Card><CardContent><Stack spacing={2.5}>
    <Stack><Typography variant="h3">Effective Runtime Profiles</Typography><Typography color="text.secondary" variant="body2">Profile defaults remain unchanged. Editing a field creates a custom value only for the selected base profile.</Typography></Stack>
    <Grid container spacing={2}>
      <Grid size={{ xs: 12, md: 4 }}><TextField select label="Detection mode" value={draft.mode} onChange={(event) => { const mode = event.target.value as RuntimePolicyValue['mode']; setDraft({ ...draft, mode, preferredProfile: mode === 'manual' && draft.preferredProfile === 'auto' ? detectedProfile : draft.preferredProfile }); }} fullWidth><MenuItem value="automatic">Automatic</MenuItem><MenuItem value="manual">Manual</MenuItem></TextField></Grid>
      <Grid size={{ xs: 12, md: 4 }}><TextField select label="Preferred base profile" value={draft.preferredProfile} onChange={(event) => setDraft({ ...draft, preferredProfile: event.target.value })} fullWidth><MenuItem value="auto">Auto recommended ({detectedProfile.replaceAll('_', ' ')})</MenuItem>{catalog.profiles.map((profile) => <MenuItem value={profile.key} key={profile.key}>{profile.name}</MenuItem>)}</TextField></Grid>
      <Grid size={{ xs: 12, md: 4 }}><TextField select label="Safe fallback" value={draft.fallbackProfile} onChange={(event) => setDraft({ ...draft, fallbackProfile: event.target.value })} fullWidth>{catalog.profiles.map((profile) => <MenuItem value={profile.key} key={profile.key}>{profile.name}</MenuItem>)}</TextField></Grid>
    </Grid>
    <Alert severity="info">Detected: {detectedProfile.replaceAll('_', ' ')} · Base: {selected.name} · Effective overrides: {Object.keys(override).length}</Alert>
    <Stack><Typography variant="h4">{selected.name}</Typography><Typography color="text.secondary" variant="body2">{selected.description}</Typography></Stack>
    <Grid container spacing={2} alignItems="flex-start">
      <Grid size={{ xs: 12, lg: 6 }}><Stack spacing={1}><Typography variant="h4">Concurrency</Typography><Grid container spacing={1}>{runtimeNumericFields.slice(0, 6).map((field) => <Grid size={{ xs: 12, sm: 6 }} key={field.key}>{numericField(field)}</Grid>)}</Grid></Stack></Grid>
      <Grid size={{ xs: 12, lg: 6 }}><Stack spacing={1}><Typography variant="h4">Resource reserves</Typography><Grid container spacing={1}>{runtimeNumericFields.slice(6).map((field) => <Grid size={{ xs: 12, sm: 6 }} key={field.key}>{numericField(field)}</Grid>)}</Grid></Stack></Grid>
    </Grid>
    <Stack spacing={1}><Typography variant="h4">Runtime behavior</Typography><Grid container spacing={1}><Grid size={{ xs: 12, sm: 6 }}>{booleanField('allowDirectMode', 'Allow direct workspace mode')}</Grid><Grid size={{ xs: 12, sm: 6 }}>{booleanField('pauseWhenOnBattery', 'Pause new jobs while on battery')}</Grid><Grid size={{ xs: 12, sm: 6 }}>{booleanField('preventSleepDuringJobs', 'Prevent sleep during active jobs (macOS)')}</Grid></Grid></Stack>
    <Stack direction="row" spacing={1}><Button startIcon={<SaveIcon />} variant="contained" disabled={saving} onClick={() => onSave(draft)}>Save effective runtime policy</Button><Button disabled={!Object.keys(override).length} onClick={() => setDraft((current) => { const overrides = { ...current.overrides }; delete overrides[selected.key]; return { ...current, overrides }; })}>Restore profile defaults</Button></Stack>
  </Stack></CardContent></Card>;
}

function HousekeepingCard({ value, saving, preview, result, busy, onSave, onPreview, onRun }: { value: HousekeepingValue; saving: boolean; preview?: HousekeepingReport; result?: HousekeepingReport; busy: boolean; onSave: (value: HousekeepingValue) => void; onPreview: () => void; onRun: () => void }) {
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [JSON.stringify(value)]);
  const report = result ?? preview;
  const daysField = (key: 'failedRetentionDays' | 'canceledRetentionDays' | 'orphanRetentionDays', label: string) => <TextField type="number" label={label} value={draft[key]} inputProps={{ min: 0 }} onChange={(event) => setDraft({ ...draft, [key]: Math.max(0, Number(event.target.value)) })} fullWidth />;
  return <Card><CardContent><Stack spacing={2}>
    <Stack><Typography variant="h3">Workspace Housekeeping</Typography><Typography color="text.secondary" variant="body2">Only direct job-N directories inside the configured work storage role can be removed.</Typography></Stack>
    <FormControlLabel control={<Switch checked={draft.autoEnabled} onChange={(event) => setDraft({ ...draft, autoEnabled: event.target.checked })} />} label="Enable automatic housekeeping" />
    <Grid container spacing={2}>
      <Grid size={{ xs: 12, md: 3 }}><TextField type="number" label="Interval (hours)" value={draft.intervalHours} inputProps={{ min: 1 }} onChange={(event) => setDraft({ ...draft, intervalHours: Math.max(1, Number(event.target.value)) })} fullWidth /></Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('failedRetentionDays', 'Failed retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('canceledRetentionDays', 'Canceled retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('orphanRetentionDays', 'Orphan retention (days)')}</Grid>
    </Grid>
    <Alert severity="info">Completed but unpublished outputs and running/queued jobs are never housekeeping candidates. Use Preview before manual cleanup.</Alert>
    <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
      <Button startIcon={<SaveIcon />} variant="contained" disabled={saving} onClick={() => onSave(draft)}>Save policy</Button>
      <Button variant="outlined" disabled={busy} onClick={onPreview}>Preview cleanup</Button>
      <Button color="error" variant="outlined" disabled={busy || !preview?.candidates.length} onClick={onRun}>Remove previewed candidates</Button>
    </Stack>
    {report ? <Stack spacing={1}>
      <Typography variant="h4">{report.dryRun ? 'Cleanup preview' : 'Cleanup result'}</Typography>
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap><Chip label={`${report.candidates.length} candidates`} /><Chip label={`${formatBytes(report.dryRun ? report.candidates.reduce((sum, item) => sum + item.sizeBytes, 0) : report.recoveredBytes)} recoverable`} /><Chip label={`${report.removedPaths.length} removed`} color={report.removedPaths.length ? 'success' : 'default'} /></Stack>
      {report.candidates.map((item) => <Alert severity={report.dryRun ? 'warning' : 'success'} key={item.path}>{item.reason}: {item.path} ({formatBytes(item.sizeBytes)})</Alert>)}
      {report.errors.map((error) => <Alert severity="error" key={error}>{error}</Alert>)}
    </Stack> : null}
  </Stack></CardContent></Card>;
}

function directPlayValue(value: Record<string, unknown> | undefined): DirectPlayValue {
  return {
    enabled: booleanValue(value?.enabled, true), strategy: stringValue(value?.strategy, 'balanced'),
    targetClients: arrayValue(value?.targetClients, directPlayClients).filter((client): client is string => typeof client === 'string'),
    minimumScore: numberValue(value?.minimumScore, 70), enforcement: value?.enforcement === 'block' ? 'block' : 'warn',
  };
}

function DirectPlayCard({ value, saving, onSave }: { value: DirectPlayValue; saving: boolean; onSave: (value: DirectPlayValue) => void }) {
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [JSON.stringify(value)]);
  const toggleClient = (client: string) => setDraft((current) => ({ ...current, targetClients: current.targetClients.includes(client) ? current.targetClients.filter((item) => item !== client) : [...current.targetClients, client] }));
  return <Card><CardContent><Stack spacing={2}>
    <Stack><Typography variant="h3">DirectPlay Policy</Typography><Typography color="text.secondary" variant="body2">Estimate playback compatibility from the planned output profile for each target client.</Typography></Stack>
    <FormControlLabel control={<Switch checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked })} />} label="Enable DirectPlay preflight" />
    <Grid container spacing={2}>
      <Grid size={{ xs: 12, md: 4 }}><TextField select label="Strategy" value={draft.strategy} disabled={!draft.enabled} onChange={(event) => setDraft({ ...draft, strategy: event.target.value })} fullWidth><MenuItem value="balanced">Balanced</MenuItem><MenuItem value="maximum_compatibility">Maximum compatibility</MenuItem><MenuItem value="modern_clients">Modern clients</MenuItem></TextField></Grid>
      <Grid size={{ xs: 12, md: 4 }}><TextField type="number" label="Minimum client score" value={draft.minimumScore} disabled={!draft.enabled} inputProps={{ min: 1, max: 100 }} onChange={(event) => setDraft({ ...draft, minimumScore: Math.min(100, Math.max(1, Number(event.target.value))) })} fullWidth /></Grid>
      <Grid size={{ xs: 12, md: 4 }}><TextField select label="Below threshold" value={draft.enforcement} disabled={!draft.enabled} onChange={(event) => setDraft({ ...draft, enforcement: event.target.value as DirectPlayValue['enforcement'] })} fullWidth><MenuItem value="warn">Warn only</MenuItem><MenuItem value="block">Require manual review</MenuItem></TextField></Grid>
    </Grid>
    <Stack><Typography variant="h4">Target clients</Typography><Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>{directPlayClients.map((client) => <FormControlLabel key={client} control={<Checkbox checked={draft.targetClients.includes(client)} disabled={!draft.enabled} onChange={() => toggleClient(client)} />} label={client.replaceAll('_', ' ')} />)}</Stack></Stack>
    <Alert severity="info">This is a preflight estimate. MVForge records it in the Execution Plan; definitive compatibility requires analyzing the final file.</Alert>
    <Button startIcon={<SaveIcon />} variant="contained" disabled={saving || (draft.enabled && draft.targetClients.length === 0)} onClick={() => onSave(draft)}>Save DirectPlay policy</Button>
  </Stack></CardContent></Card>;
}

function workingHoursValue(value: Record<string, unknown> | undefined): WorkingHoursValue {
  const policy = value?.outsideWindowPolicy && typeof value.outsideWindowPolicy === 'object' ? value.outsideWindowPolicy as Record<string, unknown> : {};
  const windows = Array.isArray(value?.windows) ? value.windows.flatMap((entry) => {
    if (!entry || typeof entry !== 'object') return [];
    const item = entry as Record<string, unknown>;
    return [{ name: stringValue(item.name, ''), days: arrayValue(item.days, []).filter((day): day is string => typeof day === 'string'), start: stringValue(item.start, '23:00'), end: stringValue(item.end, '07:00') }];
  }) : [];
  return {
    enabled: booleanValue(value?.enabled, false), timezone: stringValue(value?.timezone, 'America/Mexico_City'), preset: stringValue(value?.preset, 'custom'), windows,
    outsideWindowPolicy: {
      startNewHeavyJobs: booleanValue(policy.startNewHeavyJobs, true), continueRunningJobs: booleanValue(policy.continueRunningJobs, true),
      allowAnalysisJobs: booleanValue(policy.allowAnalysisJobs, true), allowValidationJobs: booleanValue(policy.allowValidationJobs, true),
      allowPublisherJobs: booleanValue(policy.allowPublisherJobs, true), allowCleanupJobs: booleanValue(policy.allowCleanupJobs, true), allowLabPreviews: booleanValue(policy.allowLabPreviews, true),
    },
  };
}

function WorkingHoursCard({ value, saving, onSave }: { value: WorkingHoursValue; saving: boolean; onSave: (value: WorkingHoursValue) => void }) {
  const [draft, setDraft] = useState(value);
  useEffect(() => setDraft(value), [JSON.stringify(value)]);
  const updateWindow = (index: number, patch: Partial<WorkingWindow>) => setDraft((current) => ({ ...current, windows: current.windows.map((window, position) => position === index ? { ...window, ...patch } : window) }));
  return <Card><CardContent><Stack spacing={2}>
    <Stack><Typography variant="h3">Working Hours</Typography><Typography color="text.secondary" variant="body2">Heavy conversions only start inside allowed windows; running jobs continue.</Typography></Stack>
    <FormControlLabel control={<Switch checked={draft.enabled} onChange={(event) => setDraft({ ...draft, enabled: event.target.checked, preset: event.target.checked ? 'custom' : 'disabled' })} />} label={draft.enabled ? 'Schedule enabled' : 'Schedule disabled'} />
    <TextField label="Timezone" value={draft.timezone} onChange={(event) => setDraft({ ...draft, timezone: event.target.value })} helperText="IANA timezone, for example America/Mexico_City" />
    <FormControlLabel control={<Switch checked={draft.outsideWindowPolicy.startNewHeavyJobs} onChange={(event) => setDraft({ ...draft, outsideWindowPolicy: { ...draft.outsideWindowPolicy, startNewHeavyJobs: event.target.checked } })} />} label="Allow heavy jobs outside configured windows" />
    <Divider />
    {draft.windows.map((window, index) => <Grid container spacing={1.5} key={index} alignItems="center">
      <Grid size={{ xs: 12, md: 3 }}><TextField label="Window name" value={window.name} onChange={(event) => updateWindow(index, { name: event.target.value })} fullWidth /></Grid>
      <Grid size={{ xs: 12, md: 4 }}><TextField label="Days" value={window.days.join(', ')} onChange={(event) => updateWindow(index, { days: event.target.value.split(',').map((day) => day.trim().toLowerCase()).filter(Boolean) })} helperText="mon, tue, wed, thu, fri, sat, sun" fullWidth /></Grid>
      <Grid size={{ xs: 6, md: 2 }}><TextField label="Start" type="time" value={window.start} onChange={(event) => updateWindow(index, { start: event.target.value })} fullWidth /></Grid>
      <Grid size={{ xs: 6, md: 2 }}><TextField label="End" type="time" value={window.end} onChange={(event) => updateWindow(index, { end: event.target.value })} fullWidth /></Grid>
      <Grid size={{ xs: 12, md: 1 }}><IconButton color="error" onClick={() => setDraft((current) => ({ ...current, windows: current.windows.filter((_, position) => position !== index) }))}><DeleteIcon /></IconButton></Grid>
    </Grid>)}
    {!draft.windows.length ? <Alert severity="info">No conversion windows configured. With heavy jobs disabled outside windows, conversions will wait.</Alert> : null}
    <Stack direction="row" spacing={1}><Button startIcon={<AddIcon />} onClick={() => setDraft((current) => ({ ...current, windows: [...current.windows, { name: 'New window', days: ['mon', 'tue', 'wed', 'thu', 'fri'], start: '23:00', end: '07:00' }] }))}>Add window</Button><Button startIcon={<SaveIcon />} variant="contained" disabled={saving} onClick={() => onSave(draft)}>Save working hours</Button></Stack>
  </Stack></CardContent></Card>;
}

const storageRoleNames = ['raw', 'library', 'originals_archive', 'work', 'cache', 'reports', 'logs'] as const;
type StorageRoleName = typeof storageRoleNames[number];
type StorageRolesValue = Record<StorageRoleName, { path: string }>;

function storageRolesValue(value: Record<string, unknown> | undefined): StorageRolesValue {
  const defaults: Record<StorageRoleName, string> = { raw: '/media/raw', library: '/media/library', originals_archive: '/media/originals_archive', work: '/media/staging', cache: '/mvforge/cache', reports: '/media/reports', logs: '/media/reports/logs' };
  return Object.fromEntries(storageRoleNames.map((role) => {
    const entry = value?.[role] && typeof value[role] === 'object' ? value[role] as Record<string, unknown> : {};
    return [role, { path: stringValue(entry.path, defaults[role]) }];
  })) as StorageRolesValue;
}

function workspaceValue(value: Record<string, unknown> | undefined): WorkspaceValue {
  return {
    preferredMode: value?.preferredMode === 'direct_mode' ? 'direct_mode' : 'copy_to_work_disk',
    fallbackMode: value?.fallbackMode === 'direct_mode' ? 'direct_mode' : 'wait',
    allowDirectMode: booleanValue(value?.allowDirectMode, false), estimateRequiredSpace: booleanValue(value?.estimateRequiredSpace, true),
  };
}

function StorageWorkspaceCard({ roles, workspace, saving, onSave }: { roles: StorageRolesValue; workspace: WorkspaceValue; saving: boolean; onSave: (roles: Record<string, unknown>, workspace: WorkspaceValue) => void }) {
  const [roleDraft, setRoleDraft] = useState(roles);
  const [workspaceDraft, setWorkspaceDraft] = useState(workspace);
  useEffect(() => setRoleDraft(roles), [JSON.stringify(roles)]);
  useEffect(() => setWorkspaceDraft(workspace), [JSON.stringify(workspace)]);
  return <Grid container spacing={2}>
    <Grid size={{ xs: 12, md: 7 }}><Card><CardContent><Stack spacing={2}><Stack><Typography variant="h3">Storage Roles</Typography><Typography color="text.secondary" variant="body2">Scheduler paths are referenced by role instead of host-specific locations.</Typography></Stack>
      {storageRoleNames.map((role) => <TextField key={role} label={role.replaceAll('_', ' ')} value={roleDraft[role].path} onChange={(event) => setRoleDraft((current) => ({ ...current, [role]: { path: event.target.value } }))} fullWidth />)}
    </Stack></CardContent></Card></Grid>
    <Grid size={{ xs: 12, md: 5 }}><Card><CardContent><Stack spacing={2}><Stack><Typography variant="h3">Workspace Strategy</Typography><Typography color="text.secondary" variant="body2">Choose whether FFmpeg reads a work-disk copy or reads raw directly.</Typography></Stack>
      <TextField select label="Preferred mode" value={workspaceDraft.preferredMode} onChange={(event) => setWorkspaceDraft({ ...workspaceDraft, preferredMode: event.target.value as WorkspaceValue['preferredMode'] })}><MenuItem value="copy_to_work_disk">Copy input to work disk</MenuItem><MenuItem value="direct_mode">Read raw directly</MenuItem></TextField>
      <TextField select label="Insufficient workspace fallback" value={workspaceDraft.fallbackMode} onChange={(event) => setWorkspaceDraft({ ...workspaceDraft, fallbackMode: event.target.value as WorkspaceValue['fallbackMode'] })}><MenuItem value="wait">Wait for workspace</MenuItem><MenuItem value="direct_mode">Use direct mode</MenuItem></TextField>
      <FormControlLabel control={<Switch checked={workspaceDraft.allowDirectMode} onChange={(event) => setWorkspaceDraft({ ...workspaceDraft, allowDirectMode: event.target.checked })} />} label="Allow direct-mode fallback" />
      <FormControlLabel control={<Switch checked={workspaceDraft.estimateRequiredSpace} onChange={(event) => setWorkspaceDraft({ ...workspaceDraft, estimateRequiredSpace: event.target.checked })} />} label="Estimate required workspace" />
      <Alert severity="info">Copy mode reserves input + estimated output + temporary overhead. Insufficient space produces WAITING_SSD_SPACE.</Alert>
      <Button startIcon={<SaveIcon />} variant="contained" disabled={saving} onClick={() => onSave(roleDraft as unknown as Record<string, unknown>, workspaceDraft)}>Save storage settings</Button>
    </Stack></CardContent></Card></Grid>
  </Grid>;
}

function PathsCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Paths</Typography>
            <Typography color="text.secondary" variant="body2">
              Global source root and controlled output locations.
            </Typography>
          </Stack>
          <TextField
            label="Raw root"
            value={form.paths.rawRoot}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, rawRoot: event.target.value } }))
            }
            fullWidth
          />
          <TextField
            select
            label="Renamed or relocated assets"
            value={form.assetInventory.reconciliationMode}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                assetInventory: {
                  ...current.assetInventory,
                  reconciliationMode: event.target.value as 'off' | 'review' | 'exact',
                },
              }))
            }
            helperText="Exact fingerprints can be reconciled automatically; uncertain legacy matches are sent to Review."
            fullWidth
          >
            <MenuItem value="exact">Automatically reconcile exact fingerprints</MenuItem>
            <MenuItem value="review">Review all possible matches</MenuItem>
            <MenuItem value="off">Off</MenuItem>
          </TextField>
          <TextField
            label="Library root"
            value={form.paths.libraryRoot}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, libraryRoot: event.target.value } }))
            }
            fullWidth
          />
          <TextField
            label="Staging path"
            value={form.paths.stagingPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, stagingPath: event.target.value } }))
            }
            fullWidth
          />
          <TextField
            label="Originals archive path"
            value={form.paths.originalsArchivePath}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, originalsArchivePath: event.target.value } }))
            }
            helperText="Controlled root for originals that have already been converted and published."
            fullWidth
          />
          <TextField
            label="AS-IS reports path"
            value={form.paths.asIsReportsPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, asIsReportsPath: event.target.value } }))
            }
            helperText="Persistent host/NAS path for pre-conversion analysis snapshots."
            fullWidth
          />
          <TextField
            label="Results reports path"
            value={form.paths.resultsReportsPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, resultsReportsPath: event.target.value } }))
            }
            helperText="Persistent host/NAS path for final conversion result reports."
            fullWidth
          />
          <TextField
            label="Logs path"
            value={form.paths.logsPath}
            onChange={(event) =>
              setForm((current) => ({ ...current, paths: { ...current.paths, logsPath: event.target.value } }))
            }
            helperText="Persistent host/NAS path for job logs that can be used later by IA analysis."
            fullWidth
          />
        </Stack>
      </CardContent>
    </Card>
  );
}

function WorkersCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Workers</Typography>
            <Typography color="text.secondary" variant="body2">
              Defaults used by local and future distributed workers.
            </Typography>
          </Stack>
          <TextField
            label="Default worker name"
            value={form.workers.defaultWorkerName}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                workers: { ...current.workers, defaultWorkerName: event.target.value },
              }))
            }
            fullWidth
          />
          <FormControlLabel
            control={
              <Switch
                checked={form.workers.autoWorkerEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    workers: { ...current.workers, autoWorkerEnabled: event.target.checked },
                  }))
                }
              />
            }
            label={form.workers.autoWorkerEnabled ? 'Auto worker enabled' : 'Auto worker disabled'}
          />
          <TextField
            label="Max concurrent jobs"
            type="number"
            value={form.workers.maxConcurrentJobs}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                workers: { ...current.workers, maxConcurrentJobs: Number(event.target.value) },
              }))
            }
            inputProps={{ min: 1, max: 8 }}
            fullWidth
          />
          <TextField
            label="Jobs per worker batch"
            type="number"
            value={form.workers.maxJobsPerBatch}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                workers: { ...current.workers, maxJobsPerBatch: Number(event.target.value) },
              }))
            }
            helperText="General limit of jobs the worker may start before resting. This is not tied to a folder batch."
            inputProps={{ min: 0, max: 1000 }}
            fullWidth
          />
          <TextField
            label="Batch rest time"
            type="number"
            value={form.workers.batchCooldownSeconds}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                workers: { ...current.workers, batchCooldownSeconds: Number(event.target.value) },
              }))
            }
            helperText="Seconds to wait after starting the configured number of jobs. 600 = 10 minutes."
            inputProps={{ min: 0, max: 86400 }}
            fullWidth
          />
          <TextField
            label="Delay between job starts"
            type="number"
            value={form.workers.delaySecondsBetweenJobs}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                workers: { ...current.workers, delaySecondsBetweenJobs: Number(event.target.value) },
              }))
            }
            helperText="Seconds to wait between claiming jobs."
            inputProps={{ min: 0, max: 86400 }}
            fullWidth
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={form.workers.dryRunOnly}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    workers: { ...current.workers, dryRunOnly: event.target.checked },
                  }))
                }
              />
            }
            label="Dry-run only"
          />
        </Stack>
      </CardContent>
    </Card>
  );
}

function PipelineAutomationCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Pipeline Automation</Typography>
            <Typography color="text.secondary" variant="body2">
              Automatic handoff between worker, validation, publishing, and originals archive.
            </Typography>
          </Stack>
          <Alert severity="info">
            Disabled stages stop the pipeline and wait for human action in that section.
          </Alert>
          <FormControlLabel
            control={
              <Switch
                checked={form.pipelineAutomation.autoAnalysisEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      autoAnalysisEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Automatic analysis"
          />
          <TextField
            label="Review Plan approval"
            value={form.pipelineAutomation.reviewMode}
            onChange={(event) => setForm((current) => ({ ...current, pipelineAutomation: { ...current.pipelineAutomation, reviewMode: event.target.value as SettingsForm['pipelineAutomation']['reviewMode'] } }))}
            helperText="Manual waits for approval; conditional approves safe estimates; automatic approves every evaluated plan."
            select
            fullWidth
          >
            <MenuItem value="manual">Manual</MenuItem>
            <MenuItem value="conditional">Conditional</MenuItem>
            <MenuItem value="automatic">Automatic</MenuItem>
          </TextField>
          <FormControlLabel
            control={<Switch checked={form.pipelineAutomation.autoExecutionEnabled} onChange={(event) => setForm((current) => ({ ...current, pipelineAutomation: { ...current.pipelineAutomation, autoExecutionEnabled: event.target.checked } }))} />}
            label="Automatic execution of approved plans"
          />
          <FormControlLabel
            control={
              <Switch
                checked={form.pipelineAutomation.autoValidationEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      autoValidationEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Automatic validation"
          />
          <FormControlLabel
            control={
              <Switch
                checked={form.pipelineAutomation.autoPublisherEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      autoPublisherEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Automatic publisher and originals archive"
          />
          <FormControlLabel
            control={
              <Switch
                checked={form.pipelineAutomation.publishedJobReconciliationEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      publishedJobReconciliationEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Published job reconciliation"
          />
          <Typography color="text.secondary" variant="caption">
            Periodically cleans staging for published jobs. It does not move recovered originals back to Archive.
          </Typography>
          <Divider />
          <Typography variant="h3">Asset Inventory Sync</Typography>
          <FormControlLabel
            control={
              <Switch
                checked={form.assetInventory.autoSyncEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    assetInventory: {
                      ...current.assetInventory,
                      autoSyncEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Automatic asset inventory sync"
          />
          <TextField
            label="Sync interval minutes"
            type="number"
            value={form.assetInventory.syncIntervalMinutes}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                assetInventory: {
                  ...current.assetInventory,
                  syncIntervalMinutes: Number(event.target.value),
                },
              }))
            }
            helperText="How often MVForge refreshes raw/library/archive inventory in the DB."
            inputProps={{ min: 5, max: 10080 }}
            fullWidth
          />
          <Typography color="text.secondary" variant="caption">
            Runs automatically after backend startup and at this interval. Each run also reconciles renamed or moved Library assets according to the reconciliation mode above.
          </Typography>
          <FormControlLabel
            control={
              <Checkbox
                checked={form.assetInventory.expireArchiveFiles}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    assetInventory: {
                      ...current.assetInventory,
                      expireArchiveFiles: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Physically delete expired archive files during sync"
          />
        </Stack>
      </CardContent>
    </Card>
  );
}

function ValidationCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Validation</Typography>
            <Typography color="text.secondary" variant="body2">
              Global defaults for post-conversion acceptance.
            </Typography>
          </Stack>
          <TextField
            label="Minimum score"
            type="number"
            value={form.validation.minimumScore}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                validation: { ...current.validation, minimumScore: Number(event.target.value) },
              }))
            }
            inputProps={{ min: 0, max: 100 }}
            fullWidth
          />
          <TextField
            label="Processed originals archive path"
            value={form.originalRetentionPolicy.processedOriginalsPath}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                originalRetentionPolicy: {
                  ...current.originalRetentionPolicy,
                  processedOriginalsPath: event.target.value,
                },
              }))
            }
            helperText="Where MVForge should move converted originals before retention cleanup or reuse."
            fullWidth
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={form.validation.requireDurationMatch}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    validation: { ...current.validation, requireDurationMatch: event.target.checked },
                  }))
                }
              />
            }
            label="Require duration match"
          />
        </Stack>
      </CardContent>
    </Card>
  );
}

function CancellationPolicyCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Cancel Job</Typography>
            <Typography color="text.secondary" variant="body2">
              Cleanup defaults for canceled jobs.
            </Typography>
          </Stack>
          <FormControlLabel
            control={
              <Checkbox
                checked={form.cancellationPolicy.keepLogsAndDiagnostics}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    cancellationPolicy: {
                      ...current.cancellationPolicy,
                      keepLogsAndDiagnostics: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Keep logs and diagnostics"
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={form.cancellationPolicy.deleteGeneratedFiles}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    cancellationPolicy: {
                      ...current.cancellationPolicy,
                      deleteGeneratedFiles: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Delete generated files and temporary artifacts"
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={form.cancellationPolicy.deletePartialOutputFromStaging}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    cancellationPolicy: {
                      ...current.cancellationPolicy,
                      deletePartialOutputFromStaging: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Delete partial output from staging"
          />
          <Alert severity="info">
            Cleanup can only remove artifacts inside MVForge-controlled paths.
          </Alert>
          <Stack spacing={0.75}>
            <Typography color="text.secondary" variant="body2">
              Controlled paths
            </Typography>
            <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
              {form.cancellationPolicy.controlledRoots.map((root) => (
                <Chip key={root} label={root} size="small" />
              ))}
            </Stack>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}

function OriginalRetentionCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Originals Retention</Typography>
            <Typography color="text.secondary" variant="body2">
              How long converted originals should be preserved before cleanup.
            </Typography>
          </Stack>
          <TextField
            label="Keep originals for days"
            type="number"
            value={form.originalRetentionPolicy.keepOriginalsDays}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                originalRetentionPolicy: {
                  ...current.originalRetentionPolicy,
                  keepOriginalsDays: Number(event.target.value),
                },
              }))
            }
            inputProps={{ min: 0, max: 3650 }}
            fullWidth
          />
          <TextField
            label="Processed originals archive path"
            value={form.originalRetentionPolicy.processedOriginalsPath}
            onChange={(event) =>
              setForm((current) => ({
                ...current,
                originalRetentionPolicy: {
                  ...current.originalRetentionPolicy,
                  processedOriginalsPath: event.target.value,
                },
              }))
            }
            helperText="Where MVForge should move converted originals before retention cleanup or reuse."
            fullWidth
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={form.originalRetentionPolicy.enabledForSuccessfulConversionsOnly}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    originalRetentionPolicy: {
                      ...current.originalRetentionPolicy,
                      enabledForSuccessfulConversionsOnly: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Only after successful conversions"
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={form.originalRetentionPolicy.autoDeleteEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    originalRetentionPolicy: {
                      ...current.originalRetentionPolicy,
                      autoDeleteEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Enable automatic deletion later"
          />
          <Alert severity="info">
            Cleanup will only apply to originals tracked by MVForge after a successful conversion.
          </Alert>
        </Stack>
      </CardContent>
    </Card>
  );
}

function AdvancedSettings({
  form,
  setForm,
  onSave,
  isSaving,
}: {
  form: SettingsForm;
  setForm: Dispatch<SetStateAction<SettingsForm>>;
  onSave: () => void;
  isSaving: boolean;
}) {
  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Advanced JSON</Typography>
            <Typography color="text.secondary" variant="body2">
              Direct configuration editing for technical workflows.
            </Typography>
          </Stack>
          <Divider />
          {(['paths', 'workers', 'pipelineAutomation', 'assetInventory', 'validation', 'cancellationPolicy', 'originalRetentionPolicy', 'assetTypes', 'assetCategories'] as const).map((key) => (
            <TextField
              key={key}
              label={key}
              value={form.advancedJson[key]}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  advancedJson: { ...current.advancedJson, [key]: event.target.value },
                }))
              }
              multiline
              minRows={key === 'assetTypes' ? 10 : 5}
              fullWidth
            />
          ))}
          <Button startIcon={<SaveIcon />} variant="contained" onClick={onSave} disabled={isSaving} sx={{ alignSelf: 'flex-start' }}>
            Save Advanced Settings
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}

type SettingsCardProps = {
  form: SettingsForm;
  setForm: Dispatch<SetStateAction<SettingsForm>>;
};

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return 'Unknown';
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  const unit = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  return `${(value / 1024 ** unit).toFixed(unit >= 3 ? 1 : 0)} ${units[unit]}`;
}

function safeRuntimeList(value: unknown): unknown[] {
  return Array.isArray(value) ? value : [];
}

function safeRuntimeRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === 'object' ? value as Record<string, unknown> : {};
}

function runtimeEncoder(value: unknown) {
  const encoder = safeRuntimeRecord(value);
  return {
    listed: encoder.listed === true,
    usable: encoder.usable === true,
    reason: typeof encoder.reason === 'string' ? encoder.reason : '',
  };
}

function summarizeModeReason(reason: string) {
  const lower = reason.toLowerCase();

  if (lower.includes('invalid video parameters')) {
    return 'Unsupported parameter combination';
  }

  if (lower.includes('requested qsv rate control la_icq but encoder used icq')) {
    return 'LA-ICQ is not supported by this runtime';
  }

  if (lower.includes('encoder is not listed')) {
    return 'Encoder is not available';
  }

  if (lower.includes('unsupported')) {
    return 'Unsupported by this encoder/runtime';
  }

  if (lower.includes('skipped because')) {
    return reason;
  }

  return 'Capability probe failed';
}

function runtimeEncoderCapabilities(name: string, value: unknown) {
  const encoder = safeRuntimeRecord(value);
  const tested = safeRuntimeRecord(encoder.testedModes);
  const reasons = safeRuntimeRecord(encoder.modeReasons);
  const labels: Array<[string, string]> = [
    ['qsvIcqMain8', 'QSV ICQ Main'],
    ['qsvIcqMain10', 'QSV ICQ Main10'],

    ['qsvCqpMain8', 'QSV CQP Main'],
    ['qsvCqpMain10', 'QSV CQP Main10'],

    ['qsvVbrMain8', 'QSV VBR Main'],
    ['qsvVbrMain10', 'QSV VBR Main10'],
    ['qsvVbrExtBrcMain10', 'QSV VBR + ExtBRC Main10'],
    ['qsvVbrLookAheadMain10', 'QSV VBR + LookAhead Main10'],
    ['qsvVbrAdvancedMain10', 'QSV VBR Advanced Main10'],

    ['qsvCbrMain8', 'QSV CBR Main'],
    ['qsvCbrMain10', 'QSV CBR Main10'],
    ['qsvCbrExtBrcMain10', 'QSV CBR + ExtBRC Main10'],
    ['qsvCbrLookAheadMain10', 'QSV CBR + LookAhead Main10'],

    ['qsvAdaptiveIMain10', 'QSV Adaptive I Main10'],
    ['qsvAdaptiveBMain10', 'QSV Adaptive B Main10'],

    ['qsvLowPowerMain8', 'QSV Low Power Main'],
    ['qsvLowPowerMain10', 'QSV Low Power Main10'],

    ['qsvLaIcqMain10', 'QSV LA-ICQ Main10'],

    ['videoToolboxMain', 'VT Main'],
    ['videoToolboxMain10', 'VT Main10'],
    ['videoToolboxBFramesMain', 'VT B-frames Main'],
    ['videoToolboxBFramesMain10', 'VT B-frames Main10'],
    ['videoToolboxPowerEfficientMain', 'VT power efficient Main'],
    ['videoToolboxPowerEfficientMain10', 'VT power efficient Main10'],
  ];
  const isQSV = name.endsWith('_qsv');
  const isVideoToolbox = name.endsWith('_videotoolbox');

  const relevantLabels = labels.filter(([key]) => {
    if (key.startsWith('qsv')) {
      return isQSV;
    }

    if (key.startsWith('videoToolbox')) {
      return isVideoToolbox;
    }

    return true;
  });
  return relevantLabels
  .filter(([key]) =>
    typeof tested[key] === 'boolean' ||
    typeof encoder[key] === 'boolean'
  )
  .map(([key, label]) => ({
    key,
    label,
    passed: tested[key] === true || encoder[key] === true,
    reason: typeof reasons[key] === 'string' ? reasons[key] as string : '',
  }));
}

function runtimeDisk(value: unknown): { path: string; type: string; totalBytes: number; availableBytes: number } {
  const disk = value && typeof value === 'object' ? value as Record<string, unknown> : {};
  return {
    path: typeof disk.path === 'string' ? disk.path : 'Unknown',
    type: typeof disk.type === 'string' ? disk.type : 'unknown',
    totalBytes: typeof disk.totalBytes === 'number' ? disk.totalBytes : 0,
    availableBytes: typeof disk.availableBytes === 'number' ? disk.availableBytes : 0,
  };
}

function settingsToForm(settings: Array<{ key: string; value: Record<string, unknown> }>): SettingsForm {
  const byKey = Object.fromEntries(settings.map((setting) => [setting.key, setting.value]));
  const paths = byKey.paths ?? {};
  const workers = byKey.workers ?? {};
  const pipelineAutomation = byKey.pipelineAutomation ?? {};
  const assetInventory = byKey.assetInventory ?? {};
  const validation = byKey.validation ?? {};
  const cancellationPolicy = byKey.cancellationPolicy ?? {};
  const originalRetentionPolicy = byKey.originalRetentionPolicy ?? {};
  const assetTypes = byKey.assetTypes ?? {};
  const assetCategories = byKey.assetCategories ?? {};

  return buildSettingsForm({
    paths: {
      rawRoot: stringValue(paths.rawRoot, initialSettings.paths.rawRoot),
      libraryRoot: stringValue(paths.libraryRoot, initialSettings.paths.libraryRoot),
      stagingPath: stringValue(paths.stagingPath, initialSettings.paths.stagingPath),
      originalsArchivePath: archivePathValue(
        paths.originalsArchivePath ?? paths.trashPath,
        initialSettings.paths.originalsArchivePath,
      ),
      asIsReportsPath: stringValue(paths.asIsReportsPath, initialSettings.paths.asIsReportsPath),
      resultsReportsPath: stringValue(paths.resultsReportsPath, initialSettings.paths.resultsReportsPath),
      logsPath: stringValue(paths.logsPath, initialSettings.paths.logsPath),
    },
    workers: {
      defaultWorkerName: stringValue(workers.defaultWorkerName, initialSettings.workers.defaultWorkerName),
      autoWorkerEnabled: booleanValue(workers.autoWorkerEnabled, initialSettings.workers.autoWorkerEnabled),
      maxConcurrentJobs: numberValue(workers.maxConcurrentJobs, initialSettings.workers.maxConcurrentJobs),
      maxJobsPerBatch: numberValue(workers.maxJobsPerBatch, initialSettings.workers.maxJobsPerBatch),
      delaySecondsBetweenJobs: numberValue(
        workers.delaySecondsBetweenJobs,
        initialSettings.workers.delaySecondsBetweenJobs,
      ),
      batchCooldownSeconds: numberValue(workers.batchCooldownSeconds, initialSettings.workers.batchCooldownSeconds),
      dryRunOnly: booleanValue(workers.dryRunOnly, initialSettings.workers.dryRunOnly),
    },
    pipelineAutomation: {
      autoAnalysisEnabled: booleanValue(
        pipelineAutomation.autoAnalysisEnabled,
        initialSettings.pipelineAutomation.autoAnalysisEnabled,
      ),
      reviewMode: stringValue(pipelineAutomation.reviewMode, initialSettings.pipelineAutomation.reviewMode) as SettingsForm['pipelineAutomation']['reviewMode'],
      autoExecutionEnabled: booleanValue(
        pipelineAutomation.autoExecutionEnabled,
        initialSettings.pipelineAutomation.autoExecutionEnabled,
      ),
      autoValidationEnabled: booleanValue(
        pipelineAutomation.autoValidationEnabled,
        initialSettings.pipelineAutomation.autoValidationEnabled,
      ),
      autoPublisherEnabled: booleanValue(
        pipelineAutomation.autoPublisherEnabled,
        initialSettings.pipelineAutomation.autoPublisherEnabled,
      ),
      publishedJobReconciliationEnabled: booleanValue(
        pipelineAutomation.publishedJobReconciliationEnabled,
        initialSettings.pipelineAutomation.publishedJobReconciliationEnabled,
      ),
    },
    assetInventory: {
      autoSyncEnabled: booleanValue(assetInventory.autoSyncEnabled, initialSettings.assetInventory.autoSyncEnabled),
      syncIntervalMinutes: numberValue(assetInventory.syncIntervalMinutes, initialSettings.assetInventory.syncIntervalMinutes),
      expireArchiveFiles: booleanValue(assetInventory.expireArchiveFiles, initialSettings.assetInventory.expireArchiveFiles),
      reconciliationMode: reconciliationModeValue(assetInventory.reconciliationMode),
    },
    validation: {
      minimumScore: numberValue(validation.minimumScore, initialSettings.validation.minimumScore),
      requireDurationMatch: booleanValue(
        validation.requireDurationMatch,
        initialSettings.validation.requireDurationMatch,
      ),
    },
    cancellationPolicy: {
      keepLogsAndDiagnostics: booleanValue(
        cancellationPolicy.keepLogsAndDiagnostics,
        initialSettings.cancellationPolicy.keepLogsAndDiagnostics,
      ),
      deleteGeneratedFiles: booleanValue(
        cancellationPolicy.deleteGeneratedFiles,
        initialSettings.cancellationPolicy.deleteGeneratedFiles,
      ),
      deletePartialOutputFromStaging: booleanValue(
        cancellationPolicy.deletePartialOutputFromStaging,
        initialSettings.cancellationPolicy.deletePartialOutputFromStaging,
      ),
      controlledRoots: normalizeStringArray(
        arrayValue(cancellationPolicy.controlledRoots, initialSettings.cancellationPolicy.controlledRoots),
        initialSettings.cancellationPolicy.controlledRoots,
      ),
    },
    originalRetentionPolicy: {
      keepOriginalsDays: numberValue(
        originalRetentionPolicy.keepOriginalsDays,
        initialSettings.originalRetentionPolicy.keepOriginalsDays,
      ),
      processedOriginalsPath: archivePathValue(
        originalRetentionPolicy.processedOriginalsPath,
        initialSettings.originalRetentionPolicy.processedOriginalsPath,
      ),
      enabledForSuccessfulConversionsOnly: booleanValue(
        originalRetentionPolicy.enabledForSuccessfulConversionsOnly,
        initialSettings.originalRetentionPolicy.enabledForSuccessfulConversionsOnly,
      ),
      autoDeleteEnabled: booleanValue(
        originalRetentionPolicy.autoDeleteEnabled,
        initialSettings.originalRetentionPolicy.autoDeleteEnabled,
      ),
    },
    assetTypes: normalizeLibraryTypes(arrayValue(assetTypes.types, defaultLibraryTypes)),
    assetCategories: normalizeStringArray(
      arrayValue(assetCategories.categories, defaultAssetCategories),
      defaultAssetCategories,
    ),
  });
}

function buildSettingsForm(base: Omit<SettingsForm, 'advancedJson'>): SettingsForm {
  return {
    ...base,
    advancedJson: {
      paths: JSON.stringify(base.paths, null, 2),
      workers: JSON.stringify(base.workers, null, 2),
      pipelineAutomation: JSON.stringify(base.pipelineAutomation, null, 2),
      assetInventory: JSON.stringify(base.assetInventory, null, 2),
      validation: JSON.stringify(base.validation, null, 2),
      cancellationPolicy: JSON.stringify(base.cancellationPolicy, null, 2),
      originalRetentionPolicy: JSON.stringify(base.originalRetentionPolicy, null, 2),
      assetTypes: JSON.stringify({ types: base.assetTypes }, null, 2),
      assetCategories: JSON.stringify({ categories: base.assetCategories }, null, 2),
    },
  };
}

function rebuildAssetTypes(current: SettingsForm, assetTypes: LibraryType[]): SettingsForm {
  return buildSettingsForm({ ...current, assetTypes });
}

function normalizeLibraryTypes(value: unknown[]): LibraryType[] {
  return value
    .map((item) => {
      if (!item || typeof item !== 'object') {
        return null;
      }

      const candidate = item as Record<string, unknown>;
      const label = stringValue(candidate.label, '');
      const key = stringValue(candidate.key, slugify(label));
      if (!label || !key) {
        return null;
      }

      return {
        key,
        label,
        description: stringValue(candidate.description, ''),
        extensions: arrayValue(candidate.extensions, []).map((extension) => normalizeExtension(String(extension))).filter(Boolean),
      };
    })
    .filter((type): type is LibraryType => Boolean(type));
}

function normalizeStringArray(value: unknown[], fallback: string[]) {
  const normalized = value.map((item) => String(item).trim()).filter(Boolean);
  return normalized.length ? normalized : fallback;
}

function summarizeLibrariesByType(libraries: Library[]) {
  return libraries.reduce<Record<string, number>>((summary, library) => {
    summary[library.type] = (summary[library.type] ?? 0) + 1;
    return summary;
  }, {});
}

function parseJsonObject(value: string, fallback: Record<string, unknown>) {
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : fallback;
  } catch {
    return fallback;
  }
}

function slugify(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

function normalizeExtension(value: string) {
  const trimmed = value.trim().toLowerCase();
  if (!trimmed) {
    return '';
  }

  return trimmed.startsWith('.') ? trimmed : `.${trimmed}`;
}

function stringValue(value: unknown, fallback: string) {
  return typeof value === 'string' ? value : fallback;
}

function reconciliationModeValue(value: unknown): SettingsForm['assetInventory']['reconciliationMode'] {
  return value === 'off' || value === 'review' || value === 'exact' ? value : 'exact';
}

function archivePathValue(value: unknown, fallback: string) {
  const path = stringValue(value, fallback);
  return path.startsWith('/media/trash') ? path.replace('/media/trash', '/media/originals_archive') : path;
}

function numberValue(value: unknown, fallback: number) {
  return typeof value === 'number' ? value : fallback;
}

function booleanValue(value: unknown, fallback: boolean) {
  return typeof value === 'boolean' ? value : fallback;
}

function arrayValue(value: unknown, fallback: unknown[]) {
  return Array.isArray(value) ? value : fallback;
}
