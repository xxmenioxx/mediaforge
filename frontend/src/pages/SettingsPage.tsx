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
import { Link as RouterLink } from 'react-router-dom';
import type { Dispatch, SetStateAction } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import type { Library } from '../api/types';

type LibraryType = {
  key: string;
  label: string;
  description: string;
  extensions: string[];
};

type LibraryTypeDraft = LibraryType & {
  extensionInput: string;
};

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
  };
  assetInventory: {
    autoSyncEnabled: boolean;
    syncIntervalMinutes: number;
    expireArchiveFiles: boolean;
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
  },
  assetInventory: {
    autoSyncEnabled: true,
    syncIntervalMinutes: 60,
    expireArchiveFiles: true,
  },
  validation: { minimumScore: 90, requireDurationMatch: true },
  cancellationPolicy: {
    keepLogsAndDiagnostics: true,
    deleteGeneratedFiles: false,
    deletePartialOutputFromStaging: false,
    controlledRoots: ['/media/staging', '/mwp/work', '/mwp/work/temp', '/tmp/mediaforge'],
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
  const queryClient = useQueryClient();
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const runtimeSnapshot = useQuery({ queryKey: ['runtime-snapshot'], queryFn: api.runtimeSnapshot });
  const [tab, setTab] = useState<'general' | 'advanced'>('general');
  const [section, setSection] = useState<'assets' | 'workers' | 'validation' | 'cleanup' | 'diagnostics'>('assets');
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
      },
      assetInventory: {
        autoSyncEnabled: booleanValue(assetInventory.autoSyncEnabled, initialSettings.assetInventory.autoSyncEnabled),
        syncIntervalMinutes: numberValue(assetInventory.syncIntervalMinutes, initialSettings.assetInventory.syncIntervalMinutes),
        expireArchiveFiles: booleanValue(assetInventory.expireArchiveFiles, initialSettings.assetInventory.expireArchiveFiles),
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
            <Card>
              <CardContent sx={{ pb: 0 }}>
                <Tabs value={section} onChange={(_, value) => setSection(value)} sx={{ minHeight: 44 }}>
                  <Tab label="Assets" value="assets" />
                  <Tab label="Workers" value="workers" />
                  <Tab label="Validation" value="validation" />
                  <Tab label="Cleanup" value="cleanup" />
                  <Tab label="Diagnostics" value="diagnostics" />
                </Tabs>
              </CardContent>
            </Card>

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

            {section === 'workers' ? (
              <Grid container spacing={2}>
                <Grid size={{ xs: 12, md: 6 }}>
                  <WorkersCard form={form} setForm={setForm} />
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <PipelineAutomationCard form={form} setForm={setForm} />
                </Grid>
              </Grid>
            ) : null}

            {section === 'validation' ? (
              <Grid container spacing={2}>
                <Grid size={{ xs: 12, md: 6 }}>
                  <ValidationCard form={form} setForm={setForm} />
                </Grid>
              </Grid>
            ) : null}

            {section === 'cleanup' ? (
              <Grid container spacing={2}>
                <Grid size={{ xs: 12, md: 6 }}>
                  <CancellationPolicyCard form={form} setForm={setForm} />
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <OriginalRetentionCard form={form} setForm={setForm} />
                </Grid>
              </Grid>
            ) : null}

            {section === 'diagnostics' ? (
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
                              <Chip label={`Selected: ${runtimeSnapshot.data.selectedProfile}`} color="primary" />
                              <Chip label={`Recommended: ${runtimeSnapshot.data.recommendedProfile}`} />
                              <Chip label={`${runtimeSnapshot.data.os}/${runtimeSnapshot.data.architecture}`} />
                              <Chip label={`${runtimeSnapshot.data.cpuCores} CPU cores`} />
                              <Chip label={`${formatBytes(runtimeSnapshot.data.totalMemoryBytes)} RAM total`} />
                              <Chip label={`${formatBytes(runtimeSnapshot.data.availableMemoryBytes)} RAM available`} />
                              <Chip label={runtimeSnapshot.data.container ? 'Container' : 'Native host'} />
                              <Chip label={runtimeSnapshot.data.batteryPresent ? 'Battery present' : 'No battery'} />
                            </Stack>

                            <Divider />
                            <Typography variant="h4">Storage</Typography>
                            <Table size="small">
                              <TableHead><TableRow><TableCell>Role</TableCell><TableCell>Path</TableCell><TableCell>Total</TableCell><TableCell>Available</TableCell></TableRow></TableHead>
                              <TableBody>
                                {Object.entries(safeRuntimeRecord(runtimeSnapshot.data.disks)).map(([role, rawDisk]) => {
                                  const disk = runtimeDisk(rawDisk);
                                  return <TableRow key={role}><TableCell>{role}</TableCell><TableCell sx={{ wordBreak: 'break-all' }}>{disk.path}</TableCell><TableCell>{formatBytes(disk.totalBytes)}</TableCell><TableCell>{formatBytes(disk.availableBytes)}</TableCell></TableRow>;
                                })}
                              </TableBody>
                            </Table>

                            <Typography variant="h4">FFmpeg encoders</Typography>
                            <Table size="small">
                              <TableHead><TableRow><TableCell>Encoder</TableCell><TableCell>Listed</TableCell><TableCell>Usable</TableCell><TableCell>Diagnostic</TableCell></TableRow></TableHead>
                              <TableBody>
                                {Object.entries(safeRuntimeRecord(runtimeSnapshot.data.encoders)).map(([name, rawEncoder]) => {
                                  const encoder = runtimeEncoder(rawEncoder);
                                  return <TableRow key={name}><TableCell>{name}</TableCell><TableCell>{encoder.listed ? 'Yes' : 'No'}</TableCell><TableCell><Chip size="small" color={encoder.usable ? 'success' : 'default'} label={encoder.usable ? 'Usable' : 'Unavailable'} /></TableCell><TableCell>{encoder.reason || 'Passed capability check'}</TableCell></TableRow>;
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
                          Review MediaForge, frontend, backend, FFmpeg, and dependency versions.
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
            helperText="How often MediaForge refreshes raw/library/archive inventory in the DB."
            inputProps={{ min: 5, max: 10080 }}
            fullWidth
          />
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
            helperText="Where MediaForge should move converted originals before retention cleanup or reuse."
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
            Cleanup can only remove artifacts inside MediaForge-controlled paths.
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
            helperText="Where MediaForge should move converted originals before retention cleanup or reuse."
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
            Cleanup will only apply to originals tracked by MediaForge after a successful conversion.
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

function runtimeDisk(value: unknown): { path: string; totalBytes: number; availableBytes: number } {
  const disk = value && typeof value === 'object' ? value as Record<string, unknown> : {};
  return {
    path: typeof disk.path === 'string' ? disk.path : 'Unknown',
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
    },
    assetInventory: {
      autoSyncEnabled: booleanValue(assetInventory.autoSyncEnabled, initialSettings.assetInventory.autoSyncEnabled),
      syncIntervalMinutes: numberValue(assetInventory.syncIntervalMinutes, initialSettings.assetInventory.syncIntervalMinutes),
      expireArchiveFiles: booleanValue(assetInventory.expireArchiveFiles, initialSettings.assetInventory.expireArchiveFiles),
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
