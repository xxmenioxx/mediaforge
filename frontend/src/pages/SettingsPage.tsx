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
  FormControl,
  FormControlLabel,
  FormLabel,
  Grid,
  IconButton,
  InputAdornment,
  MenuItem,
  Radio,
  RadioGroup,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
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
import { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import { Link as RouterLink, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import type { Dispatch, SetStateAction } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import { ValidationPolicySettingsCard } from '../components/ValidationPolicySettingsCard';
import { TestEncodeSettingsDomain } from './TestEncodeSettingsDomain';
import { LegacyAnalysisRecords } from '../components/LegacyAnalysisRecords';
import { AdvancedSettings } from './settings/AdvancedSettings';
import { GeneralSettingsDomain } from './settings/GeneralSettings';
import { ProvidersIntegrationsSettingsDomain } from './settings/ProvidersIntegrationsSettings';
import { AssetDefaultsSettingsDomain } from './settings/AssetDefaultsSettings';
import { ArchiveSettingsDomain } from './settings/ArchiveSettings';
import { AnalysisSettingsDomain } from './settings/AnalysisSettings';
import type { AnalysisMode, AnalysisPolicyValue, ProfileSampleEstimatePolicyValue } from './settings/AnalysisSettings';
import {
  resolveSettingsSection,
  SettingsSectionHeader,
  SettingsSidebar,
  settingsSectionDefinitions,
  type SettingsDraftSection,
  type SettingsSection,
} from './settings/SettingsNavigation';
import { SettingsSaveBar, type SettingsSaveState } from './settings/SettingsSaveBar';
import {
  dirtySettingsDraftSections,
  equalSettingsDrafts,
  mergeSettingsDraftSection,
  mergeSettingsDraftSections,
  readStoredSettingsDraft,
  settingsDraftSectionForRoute,
  settingsDraftSnapshot,
  settingsDraftStorageKey,
} from './settings/settingsDrafts';
import type { AppSetting, HousekeepingReport, Library, MVForgePreferences, RuntimeProfileOverride, RuntimeProfilesResponse, RuntimeProfileValues, SchedulerRecoveryReport, RemoteExecutorProbe, SubtitleTranslationReadiness, SubtitleTranslationReadinessState, SubtitleTranslationSettingsValue } from '../api/types';
import { encoderNamesForWorker } from '../utils/workerEncoders';
import { liveRuntimeStateLabel, runtimeRefreshSuccessMessage } from '../utils/runtimeStatePresentation';
import {
  archivePathValue,
  analysisPolicyValue,
  arrayValue,
  booleanValue,
  buildSettingsForm,
  defaultAssetCategories,
  initialSettings,
  normalizeExtension,
  normalizeLibraryTypes,
  normalizeStringArray,
  numberValue,
  profileSampleEstimatePolicyValue,
  reconciliationModeValue,
  settingsToForm,
  slugify,
  storageRoleNames,
  storageRolesValue,
  stringValue,
  subtitleHexError,
  subtitleNormalizeHexColor,
  subtitleSRTFallbackValue,
  subtitleStylingValue,
  subtitleTranslationReadinessForSettings,
  subtitleTranslationSettingsValue,
  workerJobStartPacingModeValue,
  workerProgressPercentError,
  workerProgressPercentValue,
  workerSettingsPayload,
} from '../utils/settingsPageHelpers';

export type LibraryType = {
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
type HousekeepingValue = {
  autoEnabled: boolean;
  intervalHours: number;
  failedRetentionDays: number;
  canceledRetentionDays: number;
  orphanRetentionDays: number;
  publishedWorkspaceRetentionDays: number;
  publishedJobLogRetentionDays: number;
  failedJobLogRetentionDays: number;
  canceledJobLogRetentionDays: number;
  orphanJobLogRetentionDays: number;
  emptyRawDirectoriesEnabled: boolean;
  orphanStageArtifactsEnabled: boolean;
};
type RuntimePolicyValue = { schemaVersion: number; mode: 'automatic' | 'manual'; preferredProfile: string; fallbackProfile: string; overrides: Record<string, RuntimeProfileOverride> };

export type WorkerJobStartPacingMode = 'time' | 'progress';

export type SettingsForm = {
  paths: {
    rawRoot: string;
    libraryRoot: string;
    stagingPath: string;
    originalsArchivePath: string;
    asIsReportsPath: string;
    resultsReportsPath: string;
    logsPath: string;
  };
  mvforgePreferences: MVForgePreferences;
  analysisPolicy: AnalysisPolicyValue;
  profileSampleEstimatePolicy: ProfileSampleEstimatePolicyValue;
  workers: {
    defaultWorkerName: string;
    autoWorkerEnabled: boolean;
    maxConcurrentJobs: number;
    maxJobsPerBatch: number;
    delaySecondsBetweenJobs: number;
    batchCooldownSeconds: number;
    jobStartPacingMode: WorkerJobStartPacingMode;
    startNextJobAtProgressPercent: number;
    dryRunOnly: boolean;
  };
  pipelineAutomation: {
    autoAnalysisEnabled: boolean;
    reviewMode: 'manual' | 'automatic' | 'conditional';
    autoExecutionEnabled: boolean;
    autoValidationEnabled: boolean;
    autoPublisherEnabled: boolean;
    publisherOverwriteEnabled: boolean;
    publishedJobReconciliationEnabled: boolean;
    forceFreshSnapshotBeforeExecution: boolean;
  };
  assetInventory: {
    autoSyncEnabled: boolean;
    syncIntervalMinutes: number;
    reconciliationMode: 'off' | 'review' | 'exact';
  };
  cancellationPolicy: {
    keepLogsAndDiagnostics: boolean;
    deleteGeneratedFiles: boolean;
    deletePartialOutputFromStaging: boolean;
    controlledRoots: string[];
  };
  originalRetentionPolicy: {
    keepOriginalsDays: number;
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
    cancellationPolicy: string;
    originalRetentionPolicy: string;
    assetTypes: string;
    assetCategories: string;
  };
};

type RemoteStorageMapping = {
  localRoot: string;
  remoteRoot: string;
};

type RemoteExecutorConfig = {
  name: string;
  enabled: boolean;
  host: string;
  user: string;
  sshKeyPath: string;
  knownHostsPath: string;
  ffmpegPath: string;
  ffprobePath: string;
  storageMappings: RemoteStorageMapping[];
};

type RemoteExecutorsValue = {
  executors: RemoteExecutorConfig[];
};






const emptyDraft: LibraryTypeDraft = {
  key: '',
  label: '',
  description: '',
  extensions: [],
  extensionInput: '',
};

function remoteExecutorsValue(value: unknown): RemoteExecutorsValue {
  const record =
    value && typeof value === 'object'
      ? (value as Record<string, unknown>)
      : {};

  const executors = Array.isArray(record.executors)
    ? record.executors
    : [];

  return {
    executors: executors
      .filter(
        (item): item is Record<string, unknown> =>
          Boolean(item) && typeof item === 'object',
      )
      .map((item) => ({
        name: typeof item.name === 'string' ? item.name : '',
        enabled: item.enabled !== false,
        host: typeof item.host === 'string' ? item.host : '',
        user: typeof item.user === 'string' ? item.user : '',
        sshKeyPath:
          typeof item.sshKeyPath === 'string'
            ? item.sshKeyPath
            : '/etc/mvforge/ssh/id_ed25519',
        knownHostsPath:
          typeof item.knownHostsPath === 'string'
            ? item.knownHostsPath
            : '/etc/mvforge/ssh/known_hosts',
        ffmpegPath:
          typeof item.ffmpegPath === 'string'
            ? item.ffmpegPath
            : '/usr/local/bin/ffmpeg',
        ffprobePath:
          typeof item.ffprobePath === 'string'
            ? item.ffprobePath
            : '/usr/local/bin/ffprobe',
        storageMappings:
          Array.isArray(item.storageMappings)
            ? item.storageMappings
                .filter(
                  (
                    mapping,
                  ): mapping is {
                    localRoot: string;
                    remoteRoot: string;
                  } =>
                    typeof mapping === 'object' &&
                    mapping !== null &&
                    typeof mapping.localRoot === 'string' &&
                    typeof mapping.remoteRoot === 'string',
                )
                .map((mapping) => ({
                  localRoot: mapping.localRoot,
                  remoteRoot: mapping.remoteRoot,
                }))
            : typeof item.storageRoot === 'string'
              ? [
                  {
                    localRoot: '/media/raw',
                    remoteRoot: `${item.storageRoot}/raw`,
                  },
                  {
                    localRoot: '/media/staging',
                    remoteRoot: `${item.storageRoot}/staging`,
                  },
                ]
              : [],
      })),
  };
}

export function SettingsPage() {
  const [searchParams] = useSearchParams();
  const location = useLocation();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const runtimeSnapshot = useQuery({ queryKey: ['runtime-state'], queryFn: api.runtimeState });
  const workerNodes = useQuery({ queryKey: ['worker-nodes'], queryFn: api.workerNodes });
  const runtimeProfilesQuery = useQuery({ queryKey: ['runtime-profiles'], queryFn: api.runtimeProfiles });
  const profilesQuery = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const schedulerRecovery = useQuery({ queryKey: ['scheduler-recovery'], queryFn: api.schedulerRecovery });
  const autoPublisherSetting = settings.data?.find((item) => item.key === 'pipelineAutomation');
  const autoPublisherEnabled = settings.data
    ? autoPublisherSetting?.value.autoPublisherEnabled === true
    : undefined;
  const legacyAnalysisRecords = settings.data?.find((item) => item.key === 'analysisRecords')?.value.records;
  const hasLegacyAnalysisRecords = Array.isArray(legacyAnalysisRecords) && legacyAnalysisRecords.length > 0;
  const requestedSection = searchParams.get('section');
  const storedDraftRef = useRef(readStoredSettingsDraft());
  const restoredDraft = storedDraftRef.current;
  const requestedSectionOrDraft = requestedSection ?? restoredDraft?.section ?? null;
  const section: SettingsSection = requestedSectionOrDraft === 'pipeline' && location.hash === '#validation'
    ? 'validation'
    : resolveSettingsSection(requestedSectionOrDraft);
  const [form, setForm] = useState<SettingsForm>(() => restoredDraft?.form ?? initialSettings);
  const [savedForm, setSavedForm] = useState<SettingsForm>(() => restoredDraft?.baseline ?? initialSettings);
  const [externalSettingsChanged, setExternalSettingsChanged] = useState(false);
  const [advancedJsonError, setAdvancedJsonError] = useState<string>();
  const formRef = useRef(form);
  const savedFormRef = useRef(savedForm);
  const dirtySections = dirtySettingsDraftSections(form, savedForm);
  const activeDraftSection = settingsDraftSectionForRoute(section);
  const activeDraftIsDirty = activeDraftSection ? dirtySections.includes(activeDraftSection) : false;
  const activeDraftCanSave = activeDraftSection === 'general'
    ? form.mvforgePreferences.preferredLanguages.length > 0
    : activeDraftSection === 'analysis'
      ? form.analysisPolicy.mode !== 'custom' || (form.analysisPolicy.maximumWindows >= form.analysisPolicy.initialWindows && form.analysisPolicy.positions.length >= form.analysisPolicy.maximumWindows)
      : activeDraftSection === 'runtime'
        ? !workerProgressPercentError(form.workers.startNextJobAtProgressPercent)
        : true;
  const changeSection = (next: SettingsSection) => {
    navigate({ pathname: location.pathname, search: `?section=${next}`, hash: '' });
  };
  const [showTypeForm, setShowTypeForm] = useState(false);
  const [editingTypeKey, setEditingTypeKey] = useState<string | null>(null);
  const [typeDraft, setTypeDraft] = useState<LibraryTypeDraft>(emptyDraft);
  const [deleteTarget, setDeleteTarget] = useState<LibraryType | null>(null);
  const [reassignTo, setReassignTo] = useState('');

  const librariesByType = useMemo(() => summarizeLibrariesByType(libraries.data ?? []), [libraries.data]);

  const updateSetting = useMutation({
    mutationFn: api.updateSetting,
    onSuccess: async (savedSetting) => {
      if (savedSetting.key === 'housekeeping') {
        previewHousekeeping.reset();
        runHousekeeping.reset();
      }
      queryClient.setQueryData<AppSetting[]>(['settings'], (current) => current?.map((setting) => (
        setting.key === savedSetting.key ? savedSetting : setting
      )));
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
      await queryClient.invalidateQueries({ queryKey: ['libraries'] });
      await queryClient.invalidateQueries({ queryKey: ['runtime-profiles'] });
      await queryClient.invalidateQueries({ queryKey: ['runtime-state'] });
    },
  });

  const updateSettingsBatch = useMutation({
    mutationFn: api.updateSettingsBatch,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
      await queryClient.invalidateQueries({ queryKey: ['libraries'] });
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });

  const saveCoreSettings = useMutation({
    mutationFn: async ({
      section: draftSection,
      nextForm,
      baseline,
    }: {
      section: SettingsDraftSection;
      nextForm: SettingsForm;
      baseline: SettingsForm;
    }) => {
      const storageRoles = storageRolesValue(settings.data?.find((item) => item.key === 'storageRoles')?.value);
      const persistedPaths = { ...baseline.paths };
      const pathSettings = draftSection === 'advanced'
        ? nextForm.paths
        : draftSection === 'archive'
          ? { ...persistedPaths, originalsArchivePath: nextForm.paths.originalsArchivePath }
          : draftSection === 'assets'
            ? {
                ...persistedPaths,
                rawRoot: nextForm.paths.rawRoot,
                libraryRoot: nextForm.paths.libraryRoot,
                stagingPath: nextForm.paths.stagingPath,
                asIsReportsPath: nextForm.paths.asIsReportsPath,
                resultsReportsPath: nextForm.paths.resultsReportsPath,
                logsPath: nextForm.paths.logsPath,
              }
            : undefined;
      const batch: Array<{ key: string; value: Record<string, unknown> }> = [];

      if (draftSection === 'general') {
        batch.push({ key: 'mvforgePreferences', value: nextForm.mvforgePreferences as unknown as Record<string, unknown> });
      }
      if (draftSection === 'pipeline') {
        batch.push({ key: 'pipelineAutomation', value: nextForm.pipelineAutomation as unknown as Record<string, unknown> });
      }
      if (draftSection === 'analysis') {
        batch.push(
          { key: 'analysisPolicy', value: nextForm.analysisPolicy as unknown as Record<string, unknown> },
          { key: 'profileSampleEstimatePolicy', value: nextForm.profileSampleEstimatePolicy as unknown as Record<string, unknown> },
        );
      }
      if (draftSection === 'runtime') {
        batch.push({ key: 'workers', value: workerSettingsPayload(nextForm.workers) as unknown as Record<string, unknown> });
      }
      if (draftSection === 'assets') {
        batch.push(
          { key: 'paths', value: pathSettings as unknown as Record<string, unknown> },
          { key: 'assetInventory', value: nextForm.assetInventory as unknown as Record<string, unknown> },
          { key: 'assetTypes', value: { types: nextForm.assetTypes } },
          { key: 'assetCategories', value: { categories: nextForm.assetCategories } },
        );
      }
      if (draftSection === 'archive') {
        storageRoles.originals_archive = { path: nextForm.paths.originalsArchivePath };
        batch.push(
          { key: 'paths', value: pathSettings as unknown as Record<string, unknown> },
          { key: 'storageRoles', value: storageRoles as unknown as Record<string, unknown> },
          { key: 'originalRetentionPolicy', value: nextForm.originalRetentionPolicy as unknown as Record<string, unknown> },
        );
      }
      if (draftSection === 'operations') {
        batch.push({ key: 'cancellationPolicy', value: nextForm.cancellationPolicy as unknown as Record<string, unknown> });
      }
      if (draftSection === 'advanced') {
        storageRoles.originals_archive = { path: nextForm.paths.originalsArchivePath };
        batch.push(
          { key: 'paths', value: nextForm.paths as unknown as Record<string, unknown> },
          { key: 'storageRoles', value: storageRoles as unknown as Record<string, unknown> },
          { key: 'workers', value: workerSettingsPayload(nextForm.workers) as unknown as Record<string, unknown> },
          { key: 'pipelineAutomation', value: nextForm.pipelineAutomation as unknown as Record<string, unknown> },
          { key: 'assetInventory', value: nextForm.assetInventory as unknown as Record<string, unknown> },
          { key: 'cancellationPolicy', value: nextForm.cancellationPolicy as unknown as Record<string, unknown> },
          { key: 'originalRetentionPolicy', value: nextForm.originalRetentionPolicy as unknown as Record<string, unknown> },
          { key: 'assetTypes', value: { types: nextForm.assetTypes } },
          { key: 'assetCategories', value: { categories: nextForm.assetCategories } },
        );
      }

      return api.updateSettingsBatch({ settings: batch });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
      await queryClient.invalidateQueries({ queryKey: ['libraries'] });
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
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
    mutationFn: api.refreshRuntime,
    onSuccess: async (snapshot) => {
      queryClient.setQueryData(['runtime-state'], snapshot);
      await queryClient.invalidateQueries({ queryKey: ['runtime-state'] });
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
    formRef.current = form;
  }, [form]);

  useEffect(() => {
    savedFormRef.current = savedForm;
  }, [savedForm]);

  useEffect(() => {
    if (!settings.data) {
      return;
    }

    const persistedForm = settingsToForm(settings.data);
    const currentForm = formRef.current;
    const currentBaseline = savedFormRef.current;
    const dirty = dirtySettingsDraftSections(currentForm, currentBaseline);

    if (dirty.length === 0) {
      formRef.current = persistedForm;
      savedFormRef.current = persistedForm;
      setForm(persistedForm);
      setSavedForm(persistedForm);
      setExternalSettingsChanged(false);
      return;
    }

    const changedDraft = dirty.some((draftSection) => !equalSettingsDrafts(
      settingsDraftSnapshot(currentBaseline, draftSection),
      settingsDraftSnapshot(persistedForm, draftSection),
    ));
    const mergedForm = mergeSettingsDraftSections(persistedForm, currentForm, dirty);
    formRef.current = mergedForm;
    savedFormRef.current = persistedForm;
    setForm(mergedForm);
    setSavedForm(persistedForm);
    setExternalSettingsChanged(changedDraft);
  }, [settings.data]);

  useEffect(() => {
    if (requestedSection !== section) {
      navigate({ pathname: location.pathname, search: `?section=${section}`, hash: location.hash }, { replace: true });
    }
    if (section !== 'validation' || location.hash !== '#validation') {
      return;
    }
    const timer = window.setTimeout(() => {
      document.getElementById('validation-settings')?.scrollIntoView({ block: 'start', behavior: 'smooth' });
    }, 50);
    return () => window.clearTimeout(timer);
  }, [location.hash, location.pathname, navigate, requestedSection, section]);

  useEffect(() => {
    const dirty = dirtySettingsDraftSections(form, savedForm);
    if (dirty.length === 0) {
      window.sessionStorage.removeItem(settingsDraftStorageKey);
      return;
    }

    const activeDraftSection = settingsDraftSectionForRoute(section);
    const draftSection = activeDraftSection && dirty.includes(activeDraftSection) ? activeDraftSection : dirty[0];
    window.sessionStorage.setItem(settingsDraftStorageKey, JSON.stringify({
      version: 2,
      form,
      baseline: savedForm,
      section: draftSection,
    }));
  }, [form, savedForm, section]);

  useEffect(() => {
    if (dirtySections.length === 0) return;
    const warnBeforeLeaving = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', warnBeforeLeaving);
    return () => window.removeEventListener('beforeunload', warnBeforeLeaving);
  }, [dirtySections.length]);

  function discardDraftSection(draftSection: SettingsDraftSection) {
    setForm((current) => mergeSettingsDraftSection(current, savedFormRef.current, draftSection));
    setExternalSettingsChanged(false);
  }

  async function reloadPersistedSettings() {
    if (!settings.data) return;
    const persistedForm = settingsToForm(settings.data);
    formRef.current = persistedForm;
    savedFormRef.current = persistedForm;
    setForm(persistedForm);
    setSavedForm(persistedForm);
    setExternalSettingsChanged(false);
    window.sessionStorage.removeItem(settingsDraftStorageKey);
  }

  async function saveDraftSection(draftSection: SettingsDraftSection, nextForm = form) {
    if ((draftSection === 'runtime' || draftSection === 'advanced') && workerProgressPercentError(nextForm.workers.startNextJobAtProgressPercent)) {
      return false;
    }

    try {
      await saveCoreSettings.mutateAsync({ section: draftSection, nextForm, baseline: savedFormRef.current });
      const nextBaseline = draftSection === 'advanced'
        ? {
            ...nextForm,
            mvforgePreferences: savedFormRef.current.mvforgePreferences,
            analysisPolicy: savedFormRef.current.analysisPolicy,
            profileSampleEstimatePolicy: savedFormRef.current.profileSampleEstimatePolicy,
          }
        : mergeSettingsDraftSection(savedFormRef.current, nextForm, draftSection);
      savedFormRef.current = nextBaseline;
      setSavedForm(nextBaseline);
      setForm((current) => draftSection === 'advanced' ? nextForm : mergeSettingsDraftSection(current, nextForm, draftSection));
      setExternalSettingsChanged(false);
      return true;
    } catch {
      return false;
    }
  }

  async function saveLibraryTypes(nextForm: SettingsForm) {
    setForm(nextForm);
    try {
      await updateSettingsBatch.mutateAsync({ settings: [
        { key: 'assetTypes', value: { types: nextForm.assetTypes } },
      ] });
      const nextBaseline: SettingsForm = {
        ...savedFormRef.current,
        assetTypes: nextForm.assetTypes,
        advancedJson: { ...savedFormRef.current.advancedJson, assetTypes: nextForm.advancedJson.assetTypes },
      };
      savedFormRef.current = nextBaseline;
      setSavedForm(nextBaseline);
      setExternalSettingsChanged(false);
      return true;
    } catch {
      return false;
    }
  }

  async function saveAdvancedSettings() {
    const invalidJsonKey = (Object.keys(form.advancedJson) as Array<keyof SettingsForm['advancedJson']>)
      .find((key) => !isJsonObject(form.advancedJson[key]));
    if (invalidJsonKey) {
      setAdvancedJsonError(`${invalidJsonKey} must contain a valid JSON object.`);
      return false;
    }
    setAdvancedJsonError(undefined);

    const paths = parseJsonObject(form.advancedJson.paths, form.paths);
    const workers = parseJsonObject(form.advancedJson.workers, form.workers);
    const pipelineAutomation = parseJsonObject(form.advancedJson.pipelineAutomation, form.pipelineAutomation);
    const assetInventory = parseJsonObject(form.advancedJson.assetInventory, form.assetInventory);
    const cancellationPolicy = parseJsonObject(form.advancedJson.cancellationPolicy, form.cancellationPolicy);
    const originalRetentionPolicy = parseJsonObject(
      form.advancedJson.originalRetentionPolicy,
      form.originalRetentionPolicy,
    );
    const assetTypesValue = parseJsonObject(form.advancedJson.assetTypes, { types: form.assetTypes });
    const assetCategoriesValue = parseJsonObject(form.advancedJson.assetCategories, { categories: form.assetCategories });
    const next = buildSettingsForm({
      mvforgePreferences: form.mvforgePreferences,
      analysisPolicy: form.analysisPolicy,
      profileSampleEstimatePolicy: form.profileSampleEstimatePolicy,
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
        jobStartPacingMode: workerJobStartPacingModeValue(workers.jobStartPacingMode),
        startNextJobAtProgressPercent: workerProgressPercentValue(workers.startNextJobAtProgressPercent),
        dryRunOnly: booleanValue(workers.dryRunOnly, initialSettings.workers.dryRunOnly),
      },
      pipelineAutomation: {
        autoAnalysisEnabled: booleanValue(
          pipelineAutomation.autoAnalysisEnabled,
          initialSettings.pipelineAutomation.autoAnalysisEnabled,
        ),
        forceFreshSnapshotBeforeExecution: booleanValue(
          pipelineAutomation.forceFreshSnapshotBeforeExecution,
          initialSettings.pipelineAutomation.forceFreshSnapshotBeforeExecution,
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
        publisherOverwriteEnabled: booleanValue(
          pipelineAutomation.publisherOverwriteEnabled,
          initialSettings.pipelineAutomation.publisherOverwriteEnabled,
        ),
        publishedJobReconciliationEnabled: booleanValue(
          pipelineAutomation.publishedJobReconciliationEnabled,
          initialSettings.pipelineAutomation.publishedJobReconciliationEnabled,
        ),
      },
      assetInventory: {
        autoSyncEnabled: booleanValue(assetInventory.autoSyncEnabled, initialSettings.assetInventory.autoSyncEnabled),
        syncIntervalMinutes: numberValue(assetInventory.syncIntervalMinutes, initialSettings.assetInventory.syncIntervalMinutes),
        reconciliationMode: reconciliationModeValue(assetInventory.reconciliationMode),
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
    return saveDraftSection('advanced', next);
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

  async function submitType(event: FormEvent<HTMLFormElement>) {
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
    const saved = await saveLibraryTypes(next);
    if (!saved) return;
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

  async function confirmDeleteType() {
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
    const saved = await saveLibraryTypes(next);
    if (!saved) return;
    setDeleteTarget(null);
    setReassignTo('');
  }

  const settingKeysBySection: Partial<Record<SettingsSection, string[]>> = {
    general: ['mvforgePreferences'],
    pipeline: ['workingHours', 'pipelineAutomation'],
    analysis: ['analysisPolicy', 'profileSampleEstimatePolicy'],
    runtime: ['runtimePolicy', 'remoteExecutors', 'directPlay'],
    assets: ['storageRoles', 'workspace', 'assetTypes', 'assetCategories', 'paths', 'assetInventory'],
    'asset-defaults': ['assetDefaults'],
    validation: ['validation'],
    'providers-integrations': ['jellyfin'],
    archive: ['originalRetentionPolicy', 'storageRoles'],
    operations: ['housekeeping', 'cancellationPolicy'],
    subtitles: ['subtitleSettings', 'subtitleTranslationSettings'],
    'test-encode': ['testEncodePolicy'],
  };
  const currentSectionKeys = settingKeysBySection[section] ?? [];
  const singleSettingMatches = currentSectionKeys.includes(updateSetting.variables?.key ?? '');
  const batchSettingMatches = (updateSettingsBatch.variables?.settings ?? []).some((setting) => currentSectionKeys.includes(setting.key));
  const directSavePending = (updateSetting.isPending && singleSettingMatches) || (updateSettingsBatch.isPending && batchSettingMatches);
  const directSaveSucceeded = (updateSetting.isSuccess && singleSettingMatches) || (updateSettingsBatch.isSuccess && batchSettingMatches) || (section === 'assets' && updateLibrary.isSuccess);
  const directSaveFailed = (updateSetting.isError && singleSettingMatches) || (updateSettingsBatch.isError && batchSettingMatches) || (section === 'assets' && updateLibrary.isError);
  const coreSavePending = Boolean(activeDraftSection && saveCoreSettings.isPending && saveCoreSettings.variables?.section === activeDraftSection);
  const coreSaveFailed = Boolean(activeDraftSection && saveCoreSettings.isError && saveCoreSettings.variables?.section === activeDraftSection);
  const currentSaveState: SettingsSaveState | undefined = coreSavePending || directSavePending
    ? 'saving'
    : coreSaveFailed || directSaveFailed || (activeDraftSection === 'advanced' && Boolean(advancedJsonError))
      ? 'failed'
      : activeDraftIsDirty
        ? 'dirty'
        : activeDraftSection || directSaveSucceeded
          ? 'saved'
          : undefined;
  const settingsSectionLabel = (value: SettingsSection) => settingsSectionDefinitions.find((definition) => definition.key === value)?.title ?? value;
  const headerSaveState = currentSaveState
    ? { label: currentSaveState === 'saving' ? 'Saving…' : currentSaveState === 'failed' ? 'Save failed' : currentSaveState === 'dirty' ? 'Unsaved changes' : 'Saved', color: currentSaveState === 'failed' ? 'error' as const : currentSaveState === 'dirty' ? 'warning' as const : currentSaveState === 'saved' ? 'success' as const : 'default' as const }
    : undefined;

  return (
    <>
      <PageHeader title="Settings" eyebrow="System configuration">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 780 }}>
          Browse configuration by domain. Draft changes remain available if you switch sections or visit another page in this tab.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {settings.isError ? <Alert severity="warning">Unable to load settings.</Alert> : null}
        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr)', md: '260px minmax(0, 1fr)' }, alignItems: 'start', gap: { xs: 2, md: 3 } }}>
          <SettingsSidebar section={section} dirtySections={dirtySections} onChange={changeSection} />
          <Box sx={{ minWidth: 0 }}>
            <SettingsSectionHeader section={section} saveState={headerSaveState} />
            {externalSettingsChanged ? (
              <Alert
                severity="warning"
                sx={{ mb: 2 }}
                action={<Button color="inherit" size="small" onClick={reloadPersistedSettings}>Reload persisted settings</Button>}
              >
                Settings changed elsewhere. Your draft is preserved; reload the persisted values or save your draft over them.
              </Alert>
            ) : null}
            {restoredDraft && dirtySections.length > 0 ? (
              <Alert severity="info" sx={{ mb: 2 }}>
                An unsaved settings draft was restored from this tab session.
              </Alert>
            ) : null}
            {directSaveSucceeded ? <Alert severity="success" sx={{ mb: 2 }}>{settingsSectionLabel(section)} settings saved.</Alert> : null}
            {directSaveFailed ? <Alert severity="warning" sx={{ mb: 2 }}>{settingsSectionLabel(section)} settings could not be saved.</Alert> : null}
            <Stack spacing={2}>

            {section === 'general' ? (
              <GeneralSettingsDomain
                value={form.mvforgePreferences}
                availableEncoders={[...new Set((workerNodes.data ?? []).flatMap((worker) => [...encoderNamesForWorker(worker)]))]}
                onChange={(value) => setForm((current) => ({ ...current, mvforgePreferences: value }))}
              />
            ) : null}

            {section === 'test-encode' ? (
              <TestEncodeSettingsDomain
                key={JSON.stringify(settings.data?.find((item) => item.key === 'testEncodePolicy')?.value ?? null)}
                value={settings.data?.find((item) => item.key === 'testEncodePolicy')?.value}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'testEncodePolicy', value })}
              />
            ) : null}

            {section === 'providers-integrations' ? (
              <ProvidersIntegrationsSettingsDomain
                value={settings.data?.find((item) => item.key === 'jellyfin')?.value}
                libraries={libraries.data ?? []}
                saving={updateSetting.isPending}
                saveError={updateSetting.isError && updateSetting.variables?.key === 'jellyfin'
                  ? updateSetting.error instanceof Error ? updateSetting.error.message : 'Please try again.'
                  : undefined}
                onSave={async (value) => { await updateSetting.mutateAsync({ key: 'jellyfin', value }); }}
              />
            ) : null}

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
                  <Grid size={{ xs: 12, md: 6 }}>
                    <AssetInventoryCard form={form} setForm={setForm} />
                  </Grid>
                </Grid>
                <StorageWorkspaceCard
                  key={JSON.stringify({
                    roles: storageRolesValue(settings.data?.find((item) => item.key === 'storageRoles')?.value),
                    workspace: workspaceValue(settings.data?.find((item) => item.key === 'workspace')?.value),
                  })}
                  roles={storageRolesValue(settings.data?.find((item) => item.key === 'storageRoles')?.value)}
                  workspace={workspaceValue(settings.data?.find((item) => item.key === 'workspace')?.value)}
                  saving={updateSetting.isPending || updateSettingsBatch.isPending}
                  onSave={(roles, workspace) => updateSettingsBatch.mutate({ settings: [
                    { key: 'storageRoles', value: roles },
                    { key: 'workspace', value: workspace as unknown as Record<string, unknown> },
                  ] })}
                />
                {hasLegacyAnalysisRecords ? (
                  <Card variant="outlined" component="section" aria-label="Assets Settings" data-testid="assets-settings-section">
                    <CardContent>
                      <Typography variant="h3" sx={{ mb: 1 }}>Assets Settings</Typography>
                      <LegacyAnalysisRecords settings={settings.data ?? []} />
                    </CardContent>
                  </Card>
                ) : null}
              </Stack>
            ) : null}

            {section === 'asset-defaults' ? (
              <AssetDefaultsSettingsDomain
                value={settings.data?.find((item) => item.key === 'assetDefaults')?.value}
                settings={settings.data ?? []}
                profiles={profilesQuery.data ?? []}
                runtimeEncoders={runtimeSnapshot.data?.encoders}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'assetDefaults', value: value as unknown as Record<string, unknown> })}
              />
            ) : null}

            {section === 'archive' ? (
              <ArchiveSettingsDomain form={form} setForm={setForm} />
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

            {section === 'subtitles' ? (
              <SubtitleCompatibilityCard
                key={JSON.stringify(subtitleSRTFallbackValue(settings.data?.find((item) => item.key === 'subtitleSettings')?.value))}
                value={subtitleSRTFallbackValue(settings.data?.find((item) => item.key === 'subtitleSettings')?.value)}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'subtitleSettings', value: { ...(settings.data?.find((item) => item.key === 'subtitleSettings')?.value ?? {}), ...value } })}
              />
            ) : null}

            {section === 'subtitles' ? (
              <SubtitleStylingCard
                key={JSON.stringify(subtitleStylingValue(settings.data?.find((item) => item.key === 'subtitleSettings')?.value))}
                value={subtitleStylingValue(settings.data?.find((item) => item.key === 'subtitleSettings')?.value)}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'subtitleSettings', value: { ...(settings.data?.find((item) => item.key === 'subtitleSettings')?.value ?? {}), subtitleStyling: value as unknown as Record<string, unknown> } })}
              />
            ) : null}

            {section === 'subtitles' ? (
              <SubtitleTranslationSettingsCard
                key={JSON.stringify(subtitleTranslationSettingsValue(settings.data?.find((item) => item.key === 'subtitleTranslationSettings')?.value))}
                value={subtitleTranslationSettingsValue(settings.data?.find((item) => item.key === 'subtitleTranslationSettings')?.value)}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'subtitleTranslationSettings', value })}
              />
            ) : null}

            {section === 'pipeline' ? (
              <PipelineAutomationCard form={form} setForm={setForm} />
            ) : null}

            {section === 'validation' ? (
              <ValidationPolicySettingsCard
                value={settings.data?.find((item) => item.key === 'validation')?.value}
                saving={updateSetting.isPending}
                autoPublisherEnabled={autoPublisherEnabled}
                onSave={(value) => updateSetting.mutate({ key: 'validation', value })}
              />
            ) : null}

            {section === 'runtime' ? (
              <WorkersCard form={form} setForm={setForm} />
            ) : null}

            {section === 'runtime' ? (
              <RuntimeProfilesCard
                key={JSON.stringify(runtimePolicyValue(settings.data?.find((item) => item.key === 'runtimePolicy')?.value))}
                catalog={runtimeProfilesQuery.data}
                value={runtimePolicyValue(settings.data?.find((item) => item.key === 'runtimePolicy')?.value)}
                detectedProfile={runtimeSnapshot.data?.recommendedProfile ?? 'desktop_safe'}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'runtimePolicy', value: value as unknown as Record<string, unknown> })}
              />
            ) : null}

            {section === 'runtime' ? (
              <RemoteExecutorsCard
                value={remoteExecutorsValue(
                  settings.data?.find(
                    (item) => item.key === 'remoteExecutors',
                  )?.value,
                )}
                saving={updateSetting.isPending}
                onSave={(value) =>
                  updateSetting.mutate({
                    key: 'remoteExecutors',
                    value: value as unknown as Record<string, unknown>,
                  })
                }
              />
            ) : null}
            
            {section === 'runtime' ? (
              <DirectPlayCard
                key={JSON.stringify(directPlayValue(settings.data?.find((item) => item.key === 'directPlay')?.value))}
                value={directPlayValue(settings.data?.find((item) => item.key === 'directPlay')?.value)}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'directPlay', value: value as unknown as Record<string, unknown> })}
              />
            ) : null}

            {section === 'analysis' ? (
              <AnalysisSettingsDomain
                analysisPolicy={form.analysisPolicy}
                profileSamplePolicy={form.profileSampleEstimatePolicy}
                onChangeAnalysisPolicy={(value) => setForm((current) => ({ ...current, analysisPolicy: value }))}
                onChangeProfileSamplePolicy={(value) => setForm((current) => ({ ...current, profileSampleEstimatePolicy: value }))}
              />
            ) : null}

            {section === 'pipeline' ? (
              <WorkingHoursCard
                key={JSON.stringify(workingHoursValue(settings.data?.find((item) => item.key === 'workingHours')?.value))}
                value={workingHoursValue(settings.data?.find((item) => item.key === 'workingHours')?.value)}
                saving={updateSetting.isPending}
                onSave={(value) => updateSetting.mutate({ key: 'workingHours', value: value as unknown as Record<string, unknown> })}
              />
            ) : null}

            {section === 'operations' ? (
              <Grid container spacing={2}>
                <Grid size={{ xs: 12 }}>
                  <SchedulerRecoveryCard report={schedulerRecovery.data} loading={schedulerRecovery.isLoading} error={schedulerRecovery.isError} running={runRecovery.isPending} runError={runRecovery.error} onRun={() => runRecovery.mutate()} />
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <CancellationPolicyCard form={form} setForm={setForm} />
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <HousekeepingCard
                    key={JSON.stringify(housekeepingValue(settings.data?.find((item) => item.key === 'housekeeping')?.value))}
                    value={housekeepingValue(settings.data?.find((item) => item.key === 'housekeeping')?.value)}
                    saving={updateSetting.isPending}
                    preview={previewHousekeeping.data}
                    result={runHousekeeping.data}
                    busy={previewHousekeeping.isPending || runHousekeeping.isPending}
                    onSave={(value) => updateSetting.mutate({ key: 'housekeeping', value: value as unknown as Record<string, unknown> })}
                    onPreview={() => { previewHousekeeping.reset(); runHousekeeping.reset(); previewHousekeeping.mutate(); }}
                    onRun={(previewId) => runHousekeeping.mutate({ previewId })}
                  />
                </Grid>
              </Grid>
            ) : null}

            {section === 'runtime' || section === 'operations' ? (
              <Grid container spacing={2}>
                <Grid size={{ xs: 12 }} sx={{ display: section === 'runtime' ? 'block' : 'none' }}>
                  <Card>
                    <CardContent>
                      <Stack spacing={1.5}>
                        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                          <Stack>
                            <Typography variant="h3">Scheduler runtime</Typography>
                            <Typography color="text.secondary" variant="body2">
                              Live host state used to select the execution policy.
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
                            <Typography color="text.secondary" variant="caption">{liveRuntimeStateLabel(runtimeSnapshot.data.detectedAt, (value) => new Date(value).toLocaleString())}</Typography>
                          </Stack>
                        ) : (
                          <Typography color="text.secondary">No current runtime state has been detected yet.</Typography>
                        )}
                        {refreshRuntime.isSuccess ? <Alert severity="success">{runtimeRefreshSuccessMessage}</Alert> : null}
                        {refreshRuntime.isError ? <Alert severity="error">Refresh failed: {refreshRuntime.error instanceof Error ? refreshRuntime.error.message : 'unknown error'}</Alert> : null}
                      </Stack>
                    </CardContent>
                  </Card>
                </Grid>
                <Grid size={{ xs: 12, md: 6 }} sx={{ display: section === 'operations' ? 'block' : 'none' }}>
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
                <Grid size={{ xs: 12, md: 6 }} sx={{ display: section === 'operations' ? 'block' : 'none' }}>
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

            {section === 'advanced' ? <AdvancedSettings form={form} setForm={setForm} error={advancedJsonError} /> : null}
            {activeDraftSection ? (
              <SettingsSaveBar
                state={currentSaveState ?? 'saved'}
                canSave={activeDraftCanSave}
                onDiscard={() => discardDraftSection(activeDraftSection)}
                onSave={() => {
                  if (activeDraftSection === 'advanced') {
                    void saveAdvancedSettings();
                  } else {
                    void saveDraftSection(activeDraftSection);
                  }
                }}
              />
            ) : null}
            </Stack>
          </Box>
        </Box>
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
    publishedWorkspaceRetentionDays: numberValue(value?.publishedWorkspaceRetentionDays, 0),
    publishedJobLogRetentionDays: numberValue(value?.publishedJobLogRetentionDays, 14),
    failedJobLogRetentionDays: numberValue(value?.failedJobLogRetentionDays, 14),
    canceledJobLogRetentionDays: numberValue(value?.canceledJobLogRetentionDays, 7),
    orphanJobLogRetentionDays: numberValue(value?.orphanJobLogRetentionDays, 7),
    emptyRawDirectoriesEnabled: booleanValue(value?.emptyRawDirectoriesEnabled, true),
    orphanStageArtifactsEnabled: booleanValue(value?.orphanStageArtifactsEnabled, true),
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

const housekeepingKindLabels: Record<HousekeepingReport['candidates'][number]['kind'], string> = {
  job_workspace: 'Job workspace', job_log: 'Job log', stage_artifact: 'Stage artifact',
  empty_directory: 'Empty directory', test_encode: 'Test Encode', runtime_snapshot: 'Runtime snapshot',
};

export function HousekeepingCard({ value, saving, preview, result, busy, onSave, onPreview, onRun }: { value: HousekeepingValue; saving: boolean; preview?: HousekeepingReport; result?: HousekeepingReport; busy: boolean; onSave: (value: HousekeepingValue) => void; onPreview: () => void; onRun: (previewId: string) => void }) {
  const [draft, setDraft] = useState(value);
  const [category, setCategory] = useState<'all' | HousekeepingReport['candidates'][number]['kind']>('all');
  const report = result ?? preview;
  const activePreview = preview?.dryRun ? preview : undefined;
  const validPreview = Boolean(activePreview?.previewId && activePreview.previewExpiresAt && Date.parse(activePreview.previewExpiresAt) > Date.now());
  const safeCandidates = activePreview?.candidates.filter((item) => item.safeToDelete) ?? [];
  const policyDirty = JSON.stringify(draft) !== JSON.stringify(value);
  const recoverableBytes = report?.summaries.reduce((total, summary) => total + summary.recoverableBytes, 0) ?? 0;
  const daysField = (key: keyof HousekeepingValue, label: string) => <TextField type="number" label={label} value={draft[key] as number} inputProps={{ min: 0 }} onChange={(event) => setDraft({ ...draft, [key]: Math.max(0, Number(event.target.value)) })} fullWidth />;
  const candidateAge = (modifiedAt: string) => {
    const ageDays = Math.max(0, Math.floor((Date.now() - Date.parse(modifiedAt)) / 86_400_000));
    return Number.isFinite(ageDays) ? `${ageDays} days` : 'Unknown';
  };
  const rows = (report?.candidates ?? []).filter((item) => category === 'all' || item.kind === category);
  return <Card><CardContent><Stack spacing={2}>
    <Stack><Typography variant="h3">Workspace Housekeeping</Typography><Typography color="text.secondary" variant="body2">Review cleanup candidates before removal. Only items marked eligible can be removed.</Typography></Stack>
    <FormControlLabel control={<Switch checked={draft.autoEnabled} onChange={(event) => setDraft({ ...draft, autoEnabled: event.target.checked })} />} label="Enable automatic housekeeping" />
    <Grid container spacing={2}>
      <Grid size={{ xs: 12, md: 3 }}><TextField type="number" label="Interval (hours)" value={draft.intervalHours} inputProps={{ min: 1 }} onChange={(event) => setDraft({ ...draft, intervalHours: Math.max(1, Number(event.target.value)) })} fullWidth /></Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('failedRetentionDays', 'Failed workspace retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('canceledRetentionDays', 'Canceled workspace retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('orphanRetentionDays', 'Orphan workspace retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('publishedWorkspaceRetentionDays', 'Published workspace retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('publishedJobLogRetentionDays', 'Published job log retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('failedJobLogRetentionDays', 'Failed job log retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('canceledJobLogRetentionDays', 'Canceled job log retention (days)')}</Grid>
      <Grid size={{ xs: 12, md: 3 }}>{daysField('orphanJobLogRetentionDays', 'Orphan job log retention (days)')}</Grid>
    </Grid>
    <FormControlLabel control={<Switch checked={draft.emptyRawDirectoriesEnabled} onChange={(event) => setDraft({ ...draft, emptyRawDirectoriesEnabled: event.target.checked })} />} label="Remove eligible empty RAW directories" />
    <FormControlLabel control={<Switch checked={draft.orphanStageArtifactsEnabled} onChange={(event) => setDraft({ ...draft, orphanStageArtifactsEnabled: event.target.checked })} />} label="Remove eligible orphan stage artifacts" />
    <Alert severity="info">Preview authorizes one short-lived cleanup run. Blocked candidates stay visible with their reason.</Alert>
    <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
      <Button startIcon={<SaveIcon />} variant="contained" disabled={saving || busy} onClick={() => onSave(draft)}>Save policy</Button>
      <Button variant="outlined" disabled={busy || saving || policyDirty} onClick={onPreview}>Preview cleanup</Button>
      <Button color="error" variant="outlined" disabled={busy || saving || policyDirty || !validPreview || safeCandidates.length === 0} onClick={() => { if (activePreview?.previewId) onRun(activePreview.previewId); }}>Remove previewed candidates</Button>
    </Stack>
    {policyDirty ? <Alert severity="info">Save policy changes before previewing cleanup.</Alert> : null}
    {preview?.previewId && !validPreview ? <Alert severity="warning">This preview has expired. Run Preview cleanup again before removing candidates.</Alert> : null}
    {report ? <Stack spacing={1}>
      <Typography variant="h4">{report.dryRun ? 'Cleanup preview' : 'Cleanup result'}</Typography>
      {report.previewExpiresAt ? <Typography variant="body2" color="text.secondary">Preview expires {new Date(report.previewExpiresAt).toLocaleString()}</Typography> : null}
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>{report.summaries.map((summary) => <Chip key={summary.kind} label={`${housekeepingKindLabels[summary.kind]}: ${summary.candidateCount} · ${formatBytes(summary.recoverableBytes)}`} />)}{!report.dryRun && report.outcome ? <Chip label={`Outcome: ${report.outcome.replaceAll('_', ' ')}`} color={report.outcome === 'cleaned' || report.outcome === 'already_absent' ? 'success' : report.outcome === 'failed' ? 'error' : report.outcome === 'partial' ? 'warning' : 'default'} /> : null}<Chip label={`${report.dryRun ? 'Recoverable' : 'Recovered'}: ${formatBytes(report.dryRun ? recoverableBytes : report.recoveredBytes)}`} /><Chip label={`${report.removedPaths.length} removed`} color={report.removedPaths.length ? 'success' : 'default'} /></Stack>
      <TextField select label="Category" value={category} onChange={(event) => setCategory(event.target.value as typeof category)} fullWidth>
        <MenuItem value="all">All categories ({report.candidates.length})</MenuItem>
        {report.summaries.map((summary) => <MenuItem key={summary.kind} value={summary.kind}>{housekeepingKindLabels[summary.kind]} ({summary.candidateCount})</MenuItem>)}
      </TextField>
      {rows.length ? <Box sx={{ overflowX: 'auto' }}><Table size="small" aria-label="Housekeeping candidates"><TableHead><TableRow>
        <TableCell>Category</TableCell><TableCell>Path</TableCell><TableCell>Owner / job</TableCell><TableCell>Reason</TableCell><TableCell>Age</TableCell><TableCell>Size</TableCell><TableCell>Eligibility</TableCell>
      </TableRow></TableHead><TableBody>{rows.map((item) => <TableRow key={item.candidateId}>
        <TableCell>{housekeepingKindLabels[item.kind]}</TableCell><TableCell sx={{ overflowWrap: 'anywhere' }}>{item.path}</TableCell>
        <TableCell>{[item.ownership, item.jobId !== undefined ? `Job ${item.jobId}` : undefined, item.operationId, item.testEncodeId !== undefined ? `Test Encode ${item.testEncodeId}` : undefined].filter(Boolean).join(' · ') || '—'}</TableCell>
        <TableCell sx={{ overflowWrap: 'anywhere' }}><Stack spacing={0.5}>
          <Typography variant="body2">{item.reason}</Typography>
          {item.maintenance ? <Stack spacing={0.25}>
            <Typography variant="caption">Asset: {item.maintenance.assetPath}</Typography>
            <Typography variant="caption">{item.maintenance.assetRecordId ? `AssetRecord #${item.maintenance.assetRecordId} · ${item.maintenance.assetRecordStatus || 'status unknown'} · ${item.maintenance.assetRecordMissing ? 'marked missing' : 'not marked missing'}` : 'AssetRecord identity unavailable'}</Typography>
            <Typography variant="caption">Operation: {item.maintenance.status} / {item.maintenance.phase}{item.maintenance.finishedAt ? ` · finished ${new Date(item.maintenance.finishedAt).toLocaleString()}` : ' · finished time unknown'}</Typography>
            <Typography variant="caption">Temporary file: {item.maintenance.temporaryPresence}{item.maintenance.temporarySizeKnown ? ` · ${formatBytes(item.sizeBytes)}` : ' · size unknown'}</Typography>
            <Typography variant="caption">Backup: {item.maintenance.backupPath || 'path not recorded'} · {item.maintenance.backupPresence} · {item.maintenance.recoveryState}</Typography>
            {item.maintenance.resultFingerprint ? <Typography variant="caption" sx={{ overflowWrap: 'anywhere' }}>Result fingerprint: {item.maintenance.resultFingerprint}</Typography> : null}
            <Typography variant="caption" sx={{ overflowWrap: 'anywhere' }}>Current asset fingerprint: {item.maintenance.assetFingerprintState}{item.maintenance.currentAssetFingerprint ? ` · ${item.maintenance.currentAssetFingerprint}` : ''}</Typography>
            {item.maintenance.warning ? <Typography variant="caption" color="warning.main">Operation warning: {item.maintenance.warning}</Typography> : null}
          </Stack> : null}
          {item.protectionReferences?.map((reference) => <Typography key={`${reference.source}-${reference.recordId}-${reference.field}-${reference.path}`} variant="caption">
            {reference.source} #{reference.recordId} · {reference.field}: {reference.path} · {reference.state} — {reference.reason}
          </Typography>)}
          {item.protectionReferenceCount && item.protectionReferences && item.protectionReferenceCount > item.protectionReferences.length ? <Typography variant="caption">{item.protectionReferenceCount - item.protectionReferences.length} additional references</Typography> : null}
        </Stack></TableCell>
        <TableCell>{item.maintenance?.finishedAt ? candidateAge(item.maintenance.finishedAt) : candidateAge(item.modifiedAt)}</TableCell>
        <TableCell>{item.maintenance?.temporaryPresence === 'unknown' ? 'Unknown' : item.kind === 'empty_directory' ? '0 bytes (directory entry only)' : formatBytes(item.sizeBytes)}</TableCell>
        <TableCell>{item.outcome === 'already_absent' ? 'Already absent · 0 recoverable bytes' : item.safeToDelete ? (item.manualOnly ? 'Eligible · manual cleanup' : 'Eligible') : `Blocked: ${item.blockedReason || 'Not eligible'}`}</TableCell>
      </TableRow>)}</TableBody></Table></Box> : <Typography color="text.secondary">No candidates in this category.</Typography>}
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

export type StorageRoleName = typeof storageRoleNames[number];
export type StorageRolesValue = Record<StorageRoleName, { path: string }>;

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
  return <Grid container spacing={2}>
    <Grid size={{ xs: 12, md: 7 }}><Card><CardContent><Stack spacing={2}><Stack><Typography variant="h3">Storage Roles</Typography><Typography color="text.secondary" variant="body2">Scheduler paths are referenced by role instead of host-specific locations.</Typography></Stack>
      {storageRoleNames.filter((role) => role !== 'originals_archive').map((role) => <TextField key={role} label={role.replaceAll('_', ' ')} value={roleDraft[role].path} onChange={(event) => setRoleDraft((current) => ({ ...current, [role]: { path: event.target.value } }))} fullWidth />)}
      <Alert severity="info">The originals_archive role is managed in the Original Archive settings section.</Alert>
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

export function WorkersCard({ form, setForm }: SettingsCardProps) {
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
          <FormControl>
            <FormLabel id="worker-start-pacing-label">Start next job by</FormLabel>
            <RadioGroup
              row
              aria-labelledby="worker-start-pacing-label"
              value={form.workers.jobStartPacingMode}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  workers: {
                    ...current.workers,
                    jobStartPacingMode: event.target.value as WorkerJobStartPacingMode,
                  },
                }))
              }
            >
              <FormControlLabel value="time" control={<Radio />} label="Time delay" />
              <FormControlLabel value="progress" control={<Radio />} label="Job progress" />
            </RadioGroup>
          </FormControl>
          {form.workers.jobStartPacingMode === 'progress' ? (
            <TextField
              label="Progress trigger"
              type="number"
              value={form.workers.startNextJobAtProgressPercent}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  workers: {
                    ...current.workers,
                    startNextJobAtProgressPercent: Number(event.target.value),
                  },
                }))
              }
              error={Boolean(workerProgressPercentError(form.workers.startNextJobAtProgressPercent))}
              helperText={workerProgressPercentError(form.workers.startNextJobAtProgressPercent) ?? 'Start the next job when the latest worker job reaches this percentage.'}
              inputProps={{ min: 1, max: 100, step: 1 }}
              InputProps={{ endAdornment: <InputAdornment position="end">%</InputAdornment> }}
              fullWidth
            />
          ) : (
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
          )}
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
          <FormControlLabel
            control={
              <Switch
                checked={
                  form.pipelineAutomation.forceFreshSnapshotBeforeExecution
                }
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      forceFreshSnapshotBeforeExecution:
                        event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Force fresh snapshot before execution"
          />

          <Typography color="text.secondary" variant="caption">
            Re-analyzes the physical asset immediately before a real conversion.
            Existing snapshots are ignored for that analysis. This adds processing
            time but ensures AUTO decisions use current media metadata and frame
            structure.
          </Typography>
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
                checked={form.pipelineAutomation.publisherOverwriteEnabled}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    pipelineAutomation: {
                      ...current.pipelineAutomation,
                      publisherOverwriteEnabled: event.target.checked,
                    },
                  }))
                }
              />
            }
            label="Allow publisher overwrite"
          />

          <Typography color="text.secondary" variant="caption">
            Allows Publisher to replace existing media files and subtitle sidecars at
            their destination. Disabled by default.
          </Typography>
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
        </Stack>
      </CardContent>
    </Card>
  );
}

export type SubtitleSRTFallbackSettings = {
  generateSRTFallbackForASS: boolean;
};

export type SubtitleStylePresetSettings = {
  id: string;
  displayName: string;
  primaryColor: string;
  outlineColor: string;
  shadowBackColor: string;
  opacity: number;
  fontFamily: string;
  fontSize: number;
  autoFontSize: boolean;
  bold: boolean;
  outlineWidth: number;
  shadowDepth: number;
  alignmentBehavior: 'bottom' | 'top' | 'center' | 'auto';
  provenance: 'builtIn' | 'custom';
};

export type SubtitleStylingSettings = {
  version: number;
  enabled: boolean;
  applyToGeneratedASS: boolean;
  preserveSourceStyles: boolean;
  overrideExistingASSStyles: boolean;
  builtInOverrides: Record<string, Partial<SubtitleStylePresetSettings>>;
  customPresets: SubtitleStylePresetSettings[];
  savedColors: string[];
  recentColors: string[];
};

const subtitleStyleDefaults: SubtitleStylePresetSettings[] = [
  { id: 'MVF_Default', displayName: 'Default', primaryColor: '#FFFFFF', outlineColor: '#000000', shadowBackColor: '#000000', opacity: 100, fontFamily: 'Arial', fontSize: 20, autoFontSize: false, bold: false, outlineWidth: 2, shadowDepth: 0, alignmentBehavior: 'bottom', provenance: 'builtIn' },
  { id: 'MVF_Top', displayName: 'Top', primaryColor: '#FFFF00', outlineColor: '#000000', shadowBackColor: '#000000', opacity: 100, fontFamily: 'Arial', fontSize: 20, autoFontSize: false, bold: false, outlineWidth: 2, shadowDepth: 0, alignmentBehavior: 'top', provenance: 'builtIn' },
  { id: 'MVF_Lyrics', displayName: 'Lyrics', primaryColor: '#00FFFF', outlineColor: '#000000', shadowBackColor: '#000000', opacity: 100, fontFamily: 'Arial', fontSize: 20, autoFontSize: false, bold: false, outlineWidth: 2, shadowDepth: 0, alignmentBehavior: 'bottom', provenance: 'builtIn' },
  { id: 'MVF_Sign', displayName: 'Sign', primaryColor: '#90EE90', outlineColor: '#000000', shadowBackColor: '#000000', opacity: 100, fontFamily: 'Arial', fontSize: 20, autoFontSize: false, bold: false, outlineWidth: 2, shadowDepth: 0, alignmentBehavior: 'center', provenance: 'builtIn' },
];

const subtitleStylePalette = ['#FFFFFF', '#FFF4D6', '#FFFF00', '#FFD700', '#00FFFF', '#7FDBFF', '#90EE90', '#FF9ED2', '#FFA500', '#C4A7E7'];

function subtitlePresetValue(settings: SubtitleStylingSettings, id: string): SubtitleStylePresetSettings {
  const base = subtitleStyleDefaults.find((preset) => preset.id === id) ?? settings.customPresets.find((preset) => preset.id === id) ?? subtitleStyleDefaults[0];
  const override = settings.builtInOverrides[id] ?? {};
  return { ...base, ...override, id: base.id, provenance: base.provenance };
}

export function SubtitleStylingCard({ value, saving, onSave }: { value: SubtitleStylingSettings; saving: boolean; onSave: (value: SubtitleStylingSettings) => void }) {
  const [draft, setDraft] = useState(value);
  const [selectedID, setSelectedID] = useState('MVF_Default');
  const valueSnapshot = JSON.stringify(value);
  const [previousValueSnapshot, setPreviousValueSnapshot] = useState(valueSnapshot);
  if (previousValueSnapshot !== valueSnapshot) {
    setPreviousValueSnapshot(valueSnapshot);
    setDraft(value);
  }
  const preset = subtitlePresetValue(draft, selectedID);
  const savedPreset = subtitlePresetValue(value, selectedID);
  const effectiveFontSize = preset.autoFontSize || preset.fontSize === 0 ? 20 : preset.fontSize;
  const savedEffectiveFontSize = savedPreset.autoFontSize || savedPreset.fontSize === 0 ? 20 : savedPreset.fontSize;
  const dirty = JSON.stringify(draft) !== valueSnapshot;
  const defaultValue = subtitleStylingValue(undefined);
  const canReset = JSON.stringify(draft) !== JSON.stringify(defaultValue);
  const updatePreset = (key: keyof SubtitleStylePresetSettings, next: string | number | boolean) => {
    setDraft((current) => {
      const recentColors = typeof next === 'string' && /^#[0-9a-f]{6}$/i.test(next)
        ? [next.toUpperCase(), ...current.recentColors.filter((color) => color.toUpperCase() !== next.toUpperCase())].slice(0, 12)
        : current.recentColors;
      if (selectedID.startsWith('custom_')) {
        return {
          ...current,
          customPresets: current.customPresets.map((custom) => custom.id === selectedID ? { ...custom, [key]: next } : custom),
          recentColors,
        };
      }
      return {
        ...current,
        builtInOverrides: {
          ...current.builtInOverrides,
          [selectedID]: { ...current.builtInOverrides[selectedID], [key]: next },
        },
        recentColors,
      };
    });
  };
  const updateBoundedNumber = (key: 'fontSize' | 'outlineWidth' | 'shadowDepth', raw: string, min: number, max: number) => {
    const parsed = Number(raw);
    if (!Number.isFinite(parsed)) return;
    updatePreset(key, Math.max(min, Math.min(max, Math.round(parsed))));
  };
  const addCustomPreset = () => {
    const id = `custom_${Date.now().toString(36)}`;
    const custom: SubtitleStylePresetSettings = { ...subtitleStyleDefaults[0], id, displayName: 'Custom style', provenance: 'custom' };
    setDraft((current) => ({ ...current, customPresets: [...current.customPresets, custom] }));
    setSelectedID(id);
  };
  const selectPaletteColor = (color: string) => updatePreset('primaryColor', color);
  const saveSelectedColor = () => {
    const color = subtitleNormalizeHexColor(preset.primaryColor);
    if (!color) return;
    setDraft((current) => ({ ...current, savedColors: [color, ...current.savedColors.filter((item) => item !== color)].slice(0, 32) }));
  };
  const resetDraft = () => {
    setDraft(defaultValue);
    setSelectedID('MVF_Default');
  };
  const colorFields = (['primaryColor', 'outlineColor', 'shadowBackColor'] as const);
  const hasInvalidColor = colorFields.some((key) => Boolean(subtitleHexError(preset[key])));
  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">ASS/SSA Styling</Typography>
            <Typography color="text.secondary" variant="body2">Configure reusable MVForge styles for generated or explicitly restyled ASS/SSA. Saving a preset does not change existing subtitle files.</Typography>
          </Stack>
          <FormControlLabel control={<Switch checked={draft.enabled} onChange={(event) => setDraft((current) => ({ ...current, enabled: event.target.checked }))} />} label="Enable position-aware styling" />
          <FormControlLabel control={<Switch checked={draft.applyToGeneratedASS} disabled={!draft.enabled} onChange={(event) => setDraft((current) => ({ ...current, applyToGeneratedASS: event.target.checked }))} />} label="Style generated subtitles" />
          <FormControlLabel control={<Switch checked={draft.preserveSourceStyles} disabled={!draft.enabled} onChange={(event) => setDraft((current) => ({ ...current, preserveSourceStyles: event.target.checked }))} />} label="Preserve source styles" />
          <FormControlLabel control={<Switch checked={draft.overrideExistingASSStyles} disabled={!draft.enabled} onChange={(event) => setDraft((current) => ({ ...current, overrideExistingASSStyles: event.target.checked }))} />} label="Override existing ASS styles" />
          {draft.enabled && draft.overrideExistingASSStyles ? <Alert severity="warning">Native ASS/SSA appearance and eligible event style assignments may change. Text, timing, positioning, karaoke, and source definitions remain protected.</Alert> : null}
          <TextField select label="Style preset" value={selectedID} onChange={(event) => setSelectedID(event.target.value)}>
            {subtitleStyleDefaults.map((option) => <MenuItem key={option.id} value={option.id}>{option.displayName} · {option.id}</MenuItem>)}
            {draft.customPresets.map((option) => <MenuItem key={option.id} value={option.id}>{option.displayName} · custom</MenuItem>)}
          </TextField>
          <Button variant="outlined" onClick={addCustomPreset}>Add custom style</Button>
          <Grid container spacing={1.5}>
            {selectedID.startsWith('custom_') ? <Grid size={{ xs: 12, md: 4 }}><TextField label="Display name" value={preset.displayName} onChange={(event) => updatePreset('displayName', event.target.value)} fullWidth /></Grid> : null}
            {colorFields.map((key) => (
              <Grid key={key} size={{ xs: 12, md: 4 }}>
                <Stack spacing={0.75}>
                  <TextField type="color" label={key === 'primaryColor' ? 'Primary color' : key === 'outlineColor' ? 'Outline color' : 'Shadow/back color'} value={subtitleNormalizeHexColor(preset[key]) || '#000000'} onChange={(event) => updatePreset(key, event.target.value)} fullWidth InputLabelProps={{ shrink: true }} />
                  <TextField label="HEX" value={preset[key]} onChange={(event) => updatePreset(key, event.target.value)} error={Boolean(subtitleHexError(preset[key]))} helperText={subtitleHexError(preset[key]) || 'Normalized as #RRGGBB'} fullWidth />
                </Stack>
              </Grid>
            ))}
            <Grid size={{ xs: 12, md: 4 }}><TextField label="Font size (ASS units)" type="number" inputProps={{ min: 0, max: 200, step: 1 }} value={preset.fontSize} onChange={(event) => updateBoundedNumber('fontSize', event.target.value, 0, 200)} helperText={`Saved: ${savedPreset.fontSize} ASS units (effective ${savedEffectiveFontSize}) · draft output: ${effectiveFontSize} ASS units`} fullWidth /></Grid>
            <Grid size={{ xs: 12, md: 4 }}><TextField label="Outline width (ASS units)" type="number" inputProps={{ min: 0, max: 20, step: 1 }} value={preset.outlineWidth} onChange={(event) => updateBoundedNumber('outlineWidth', event.target.value, 0, 20)} helperText={`Saved: ${savedPreset.outlineWidth} · draft: ${preset.outlineWidth} (0–20)`} fullWidth /></Grid>
            <Grid size={{ xs: 12, md: 4 }}><TextField label="Shadow depth (ASS units)" type="number" inputProps={{ min: 0, max: 20, step: 1 }} value={preset.shadowDepth} onChange={(event) => updateBoundedNumber('shadowDepth', event.target.value, 0, 20)} helperText={`Saved: ${savedPreset.shadowDepth} · draft: ${preset.shadowDepth} (0–20)`} fullWidth /></Grid>
            <Grid size={{ xs: 12, md: 4 }}><TextField label="Opacity" type="number" inputProps={{ min: 0, max: 100 }} value={preset.opacity} onChange={(event) => updatePreset('opacity', Math.max(0, Math.min(100, Number(event.target.value))))} fullWidth /></Grid>
            <Grid size={{ xs: 12, md: 4 }}><TextField label="Font family" value={preset.fontFamily} onChange={(event) => updatePreset('fontFamily', event.target.value)} fullWidth /></Grid>
            <Grid size={{ xs: 12, md: 4 }}><TextField select label="Alignment fallback" value={preset.alignmentBehavior} onChange={(event) => updatePreset('alignmentBehavior', event.target.value)} fullWidth><MenuItem value="bottom">Bottom</MenuItem><MenuItem value="top">Top</MenuItem><MenuItem value="center">Center</MenuItem><MenuItem value="auto">Automatic geometry</MenuItem></TextField></Grid>
          </Grid>
          <Stack spacing={0.5}>
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
              <FormControlLabel control={<Switch checked={preset.autoFontSize} onChange={(event) => updatePreset('autoFontSize', event.target.checked)} />} label="Auto font size (fixed 20-unit fallback)" />
              <FormControlLabel control={<Switch checked={preset.bold} onChange={(event) => updatePreset('bold', event.target.checked)} />} label="Bold" />
            </Stack>
            <Typography variant="caption" color="text.secondary">Auto font size is a legacy fixed fallback: generated and restyled ASS uses 20 ASS units. It does not adapt to PlayResX/PlayResY. A font size of 0 also resolves to 20.</Typography>
            <Typography variant="caption" color="text.secondary">These values are authored ASS style units. Their rendered size depends on the subtitle file's PlayResX/PlayResY and the player's renderer; they are not screen pixels.</Typography>
          </Stack>
          <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
            {subtitleStylePalette.map((color) => <Button key={color} size="small" variant="outlined" onClick={() => selectPaletteColor(color)} sx={{ minWidth: 42, bgcolor: color, color: '#111' }} aria-label={`Use ${color}`}>{color.slice(1)}</Button>)}
          </Stack>
          <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
            <Button size="small" variant="outlined" onClick={saveSelectedColor} disabled={hasInvalidColor}>Save current color</Button>
            <Typography variant="caption" color="text.secondary">Saved colors ({draft.savedColors.length}/32)</Typography>
            {draft.savedColors.map((color) => <Button key={`saved-${color}`} size="small" aria-label={`Use saved ${color}`} onClick={() => selectPaletteColor(color)} sx={{ minWidth: 42, bgcolor: color, color: '#111' }}>{color.slice(1)}</Button>)}
          </Stack>
          <Typography variant="caption" color="text.secondary">Recent colors ({draft.recentColors.length}/12): {draft.recentColors.join(', ') || 'none'}</Typography>
          <Box sx={{ p: 1.5, borderRadius: 1, bgcolor: '#16181d', color: '#fff' }}>
            <Typography variant="caption" color="inherit">Preview · 384×288 PlayRes reference. Browser approximation; font availability and libass/Jellyfin/TV rendering can differ.</Typography>
            <Grid container spacing={1} sx={{ mt: 0.5 }}>
              {[
                { id: 'dark', label: 'Dark background', color: '#16181d' },
                { id: 'light', label: 'Light background', color: '#f0f0f0' },
              ].map((sample) => (
                <Grid key={sample.id} size={{ xs: 12, md: 6 }}>
                  <Box sx={{ borderRadius: 1, overflow: 'hidden', bgcolor: sample.color }}>
                    <svg viewBox="0 0 384 288" role="img" aria-label={`${sample.label} dialogue preview`} style={{ display: 'block', width: '100%', height: 'auto' }}>
                      {preset.shadowDepth > 0 ? (
                        <defs>
                          <filter id={`subtitle-shadow-${sample.id}`} x="-50%" y="-50%" width="200%" height="200%">
                            <feDropShadow dx={preset.shadowDepth} dy={preset.shadowDepth} stdDeviation="0" floodColor={preset.shadowBackColor} floodOpacity={preset.opacity / 100} />
                          </filter>
                        </defs>
                      ) : null}
                      <rect width="384" height="288" fill={sample.color} />
                      <text x="192" y="226" textAnchor="middle" fill={preset.primaryColor} fillOpacity={preset.opacity / 100} stroke={preset.outlineColor} strokeOpacity={preset.opacity / 100} strokeWidth={preset.outlineWidth * 2} strokeLinejoin="round" paintOrder="stroke" fontFamily={preset.fontFamily || 'Arial'} fontSize={effectiveFontSize} fontWeight={preset.bold ? 700 : 400} filter={preset.shadowDepth > 0 ? `url(#subtitle-shadow-${sample.id})` : undefined}>We need to leave before sunrise.</text>
                      <text x="192" y="256" textAnchor="middle" fill={preset.primaryColor} fillOpacity={preset.opacity / 100} stroke={preset.outlineColor} strokeOpacity={preset.opacity / 100} strokeWidth={preset.outlineWidth * 2} strokeLinejoin="round" paintOrder="stroke" fontFamily={preset.fontFamily || 'Arial'} fontSize={effectiveFontSize} fontWeight={preset.bold ? 700 : 400} filter={preset.shadowDepth > 0 ? `url(#subtitle-shadow-${sample.id})` : undefined}>Can you keep up?</text>
                    </svg>
                    <Typography variant="caption" color={sample.id === 'light' ? '#333' : '#ddd'} sx={{ display: 'block', px: 1, pb: 0.5 }}>{sample.label}</Typography>
                  </Box>
                </Grid>
              ))}
            </Grid>
          </Box>
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
            <Button startIcon={<SaveIcon />} variant="contained" onClick={() => onSave(draft)} disabled={saving || !dirty || hasInvalidColor} sx={{ alignSelf: 'flex-start' }}>Save Subtitle Styling</Button>
            <Button variant="outlined" onClick={resetDraft} disabled={saving || !canReset} sx={{ alignSelf: 'flex-start' }}>Reset to defaults</Button>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}

export function SubtitleCompatibilityCard({ value, saving, onSave }: { value: SubtitleSRTFallbackSettings; saving: boolean; onSave: (value: SubtitleSRTFallbackSettings) => void }) {
  const [draft, setDraft] = useState(value);

  useEffect(() => {
    setDraft(value);
  }, [value]);

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Subtitle compatibility</Typography>
            <Typography color="text.secondary" variant="body2">
              Optional compatibility sidecars resolved by the canonical subtitle planner.
            </Typography>
          </Stack>
          <FormControlLabel
            control={
              <Switch
                checked={draft.generateSRTFallbackForASS}
                onChange={(event) => setDraft({ generateSRTFallbackForASS: event.target.checked })}
              />
            }
            label="Generate SRT fallback for ASS compatibility"
          />
          <Typography color="text.secondary" variant="caption">
            Disabled by default. When enabled, eligible ASS requests can also produce an SRT compatibility sidecar.
          </Typography>
          <Button
            startIcon={<SaveIcon />}
            variant="contained"
            onClick={() => onSave(draft)}
            disabled={saving}
            sx={{ alignSelf: 'flex-start' }}
          >
            Save Subtitle Settings
          </Button>
        </Stack>
      </CardContent>
    </Card>
  );
}

const subtitleTranslationReadinessLabels: Record<SubtitleTranslationReadinessState, string> = {
  disabled: 'Disabled',
  not_configured: 'Not configured',
  untested: 'Untested',
  ready: 'Ready',
  unavailable: 'Unavailable',
  model_not_found: 'Model not found',
  invalid_configuration: 'Invalid configuration',
};


function subtitleTranslationConfigurationPayload(value: SubtitleTranslationSettingsValue, enabled?: boolean): Record<string, unknown> {
  const projectionFields = new Set([
    'provider',
    'ollamaEndpoint',
    'ollamaModel',
    'qualityReviewerModel',
    'maxCuesPerRequest',
    'maxRequestBytes',
    'maxContextCues',
    'maxContextChars',
    'enabled',
    'readiness',
    'readinessConfigIdentity',
    'readinessCheckedAt',
    'readinessDiagnostic',
    'ollamaProviderState',
  ]);
  const payload: Record<string, unknown> = Object.fromEntries(
    Object.entries(value).filter(([key]) => !projectionFields.has(key)),
  );
  Object.assign(payload, {
    provider: value.provider,
    ollamaEndpoint: value.ollamaEndpoint,
    ollamaModel: value.ollamaModel,
    qualityReviewerModel: value.qualityReviewerModel,
    maxCuesPerRequest: value.maxCuesPerRequest,
    maxRequestBytes: value.maxRequestBytes,
    maxContextCues: value.maxContextCues,
    maxContextChars: value.maxContextChars,
  });
  if (enabled !== undefined) {
    payload.enabled = enabled;
  }
  return payload;
}

function subtitleTranslationErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

function formatSubtitleTranslationCheckedAt(value?: string) {
  if (!value) {
    return '';
  }
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

export function SubtitleTranslationSettingsCard({
  value,
  saving,
  onSave,
}: {
  value: SubtitleTranslationSettingsValue;
  saving: boolean;
  onSave: (value: Record<string, unknown>) => void;
}) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState(value);
  const [readinessOverride, setReadinessOverride] = useState<SubtitleTranslationReadiness | null>(null);
  const [previousValue, setPreviousValue] = useState(value);
  const testConnection = useMutation({
    mutationFn: api.testSubtitleTranslationConnection,
    onSuccess: async (result) => {
      setReadinessOverride(result);
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
    onError: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });
  const enable = useMutation({
    mutationFn: api.enableSubtitleTranslation,
    onSuccess: async (result) => {
      setReadinessOverride(result);
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
    onError: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });

  if (previousValue !== value) {
    setPreviousValue(value);
    setDraft(value);
    setReadinessOverride(null);
  }

  const persistedReadiness = subtitleTranslationReadinessForSettings(value);
  const readiness = readinessOverride ?? persistedReadiness;
  const dirty = draft.provider !== value.provider || draft.ollamaEndpoint !== value.ollamaEndpoint || draft.ollamaModel !== value.ollamaModel || draft.qualityReviewerModel !== value.qualityReviewerModel;
  const structurallyConfigured = draft.provider === 'ollama' && Boolean(draft.ollamaEndpoint.trim()) && Boolean(draft.ollamaModel.trim());
  const canTest = structurallyConfigured && !dirty && !testConnection.isPending;
  const canEnable = structurallyConfigured && !dirty && !readiness.enabled && !enable.isPending;

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Subtitle translation</Typography>
            <Typography color="text.secondary" variant="body2">
              Configure the backend-authorized Ollama provider. Readiness is checked by MVForge and is not inferred from these fields.
            </Typography>
          </Stack>
          <TextField
            select
            label="Provider"
            value={draft.provider}
            onChange={(event) => setDraft((current) => ({ ...current, provider: event.target.value }))}
            fullWidth
          >
            <MenuItem value="">Not configured</MenuItem>
            <MenuItem value="ollama">Ollama</MenuItem>
          </TextField>
          {draft.provider === 'ollama' ? (
            <>
              <TextField
                label="Ollama endpoint"
                placeholder="http://192.168.1.100:11434"
                value={draft.ollamaEndpoint}
                onChange={(event) => setDraft((current) => ({ ...current, ollamaEndpoint: event.target.value }))}
                fullWidth
              />
              <TextField
                label="Ollama model"
                placeholder="qwen2.5:7b"
                value={draft.ollamaModel}
                onChange={(event) => setDraft((current) => ({ ...current, ollamaModel: event.target.value }))}
                fullWidth
              />
              <TextField
                label="Independent quality reviewer model (optional)"
                helperText="Uses a separate installed Ollama model. No model is downloaded automatically, and reviewer suggestions always require your decision."
                value={draft.qualityReviewerModel}
                onChange={(event) => setDraft((current) => ({ ...current, qualityReviewerModel: event.target.value }))}
                fullWidth
              />
            </>
          ) : null}
          <Stack spacing={0.5}>
            <Typography variant="body2">Configured: {readiness.configured ? 'Yes' : 'No'}</Typography>
            <Typography variant="body2">Enabled: {readiness.enabled ? 'Yes' : 'No'}</Typography>
            <Typography variant="body2">Readiness: {subtitleTranslationReadinessLabels[readiness.readiness]}</Typography>
            {readiness.diagnostic ? <Typography color="text.secondary" variant="body2">{readiness.diagnostic}</Typography> : null}
            {readiness.lastCheckedAt ? <Typography color="text.secondary" variant="caption">Last checked: {formatSubtitleTranslationCheckedAt(readiness.lastCheckedAt)}</Typography> : null}
          </Stack>
          {dirty ? <Alert severity="info">Save configuration before testing or enabling.</Alert> : null}
          {testConnection.isError ? <Alert severity="warning">{subtitleTranslationErrorMessage(testConnection.error, 'Connection test failed.')}</Alert> : null}
          {enable.isError ? <Alert severity="warning">{subtitleTranslationErrorMessage(enable.error, 'Translation could not be enabled.')}</Alert> : null}
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} flexWrap="wrap" useFlexGap>
            <Button
              startIcon={<SaveIcon />}
              variant="contained"
              onClick={() => onSave(subtitleTranslationConfigurationPayload(draft))}
              disabled={saving}
            >
              Save translation configuration
            </Button>
            <Button
              variant="outlined"
              onClick={() => testConnection.mutate()}
              disabled={!canTest}
            >
              {testConnection.isPending ? 'Testing…' : 'Test Connection'}
            </Button>
            {readiness.enabled ? (
              <Button
                variant="outlined"
                color="warning"
                onClick={() => onSave(subtitleTranslationConfigurationPayload(value, false))}
                disabled={saving}
              >
                Disable translation
              </Button>
            ) : (
              <Button
                variant="outlined"
                onClick={() => enable.mutate()}
                disabled={!canEnable}
              >
                {enable.isPending ? 'Enabling…' : 'Enable translation'}
              </Button>
            )}
          </Stack>
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

function AssetInventoryCard({ form, setForm }: SettingsCardProps) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Asset Inventory</Typography>
            <Typography color="text.secondary" variant="body2">
              Discovery and reconciliation of Raw, Library, Converted, and Archive records.
            </Typography>
          </Stack>
          <FormControlLabel
            control={
              <Switch
                checked={form.assetInventory.autoSyncEnabled}
                onChange={(event) => setForm((current) => ({
                  ...current,
                  assetInventory: { ...current.assetInventory, autoSyncEnabled: event.target.checked },
                }))}
              />
            }
            label="Automatic inventory sync"
          />
          <TextField
            label="Sync interval minutes"
            type="number"
            value={form.assetInventory.syncIntervalMinutes}
            onChange={(event) => setForm((current) => ({
              ...current,
              assetInventory: { ...current.assetInventory, syncIntervalMinutes: Number(event.target.value) },
            }))}
            helperText="Runs after backend startup and then at this interval."
            inputProps={{ min: 5, max: 10080 }}
            disabled={!form.assetInventory.autoSyncEnabled}
            fullWidth
          />
          <TextField
            select
            label="Renamed or relocated assets"
            value={form.assetInventory.reconciliationMode}
            onChange={(event) => setForm((current) => ({
              ...current,
              assetInventory: {
                ...current.assetInventory,
                reconciliationMode: event.target.value as 'off' | 'review' | 'exact',
              },
            }))}
            helperText="Exact fingerprints can be reconciled automatically; uncertain legacy matches can require review."
            fullWidth
          >
            <MenuItem value="exact">Automatically reconcile exact fingerprints</MenuItem>
            <MenuItem value="review">Review all possible matches</MenuItem>
            <MenuItem value="off">Off</MenuItem>
          </TextField>
          <Alert severity="info">
            Inventory sync is read-only for media unless automatic Original Archive deletion is explicitly enabled in its own section.
          </Alert>
        </Stack>
      </CardContent>
    </Card>
  );
}

function RemoteExecutorsCard({
  value,
  saving,
  onSave,
}: {
  value: RemoteExecutorsValue;
  saving: boolean;
  onSave: (value: RemoteExecutorsValue) => void;
}) {
  const [draft, setDraft] = useState<RemoteExecutorsValue>(value);
  const [probeResult, setProbeResult] = useState<RemoteExecutorProbe | null>(
    null,
  );
  const [probing, setProbing] = useState(false);

  useEffect(() => {
    setDraft(value);
  }, [value]);

  const executor = draft.executors[0] ?? {
    name: 'macbook',
    enabled: true,
    host: '',
    user: 'anuelvs',
    sshKeyPath: '/etc/mvforge/ssh/id_ed25519',
    knownHostsPath: '/etc/mvforge/ssh/known_hosts',
    ffmpegPath: '/usr/local/bin/ffmpeg',
    ffprobePath: '/usr/local/bin/ffprobe',
    storageMappings: [
      {
        localRoot: '/media/raw',
        remoteRoot: '/Volumes/docker/nas-media-stack/work/mediaforge/raw',
      },
      {
        localRoot: '/media/staging',
        remoteRoot: '/Volumes/docker/nas-media-stack/work/mediaforge/staging',
      },
    ],
  };

  function updateExecutor(
    key: keyof RemoteExecutorConfig,
    nextValue: string | boolean,
  ) {
    setDraft({
      executors: [
        {
          ...executor,
          [key]: nextValue,
        },
      ],
    });
  }

  function updateStorageMapping(
    index: number,
    key: keyof RemoteStorageMapping,
    nextValue: string,
  ) {
    setDraft({
      executors: [
        {
          ...executor,
          storageMappings: executor.storageMappings.map(
            (mapping, mappingIndex) =>
              mappingIndex === index
                ? {
                    ...mapping,
                    [key]: nextValue,
                  }
                : mapping,
          ),
        },
      ],
    });
  }
  async function probe() {
    setProbing(true);
    setProbeResult(null);

    try {
      const result = await api.probeRemoteExecutor(executor);
      setProbeResult(result);
    } catch (error) {
      setProbeResult({
        name: executor.name,
        reachable: false,
        ffmpegAvailable: false,
        ffprobeAvailable: false,
        videoToolbox: false,
        storageAccessible: false,
        error:
          error instanceof Error
            ? error.message
            : 'Remote executor probe failed.',
      });
    } finally {
      setProbing(false);
    }
  }

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Remote Executors</Typography>
            <Typography color="text.secondary" variant="body2">
              Optional machines that can provide additional media processing
              capacity.
            </Typography>
          </Stack>

          <FormControlLabel
            control={
              <Switch
                checked={executor.enabled}
                onChange={(event) =>
                  updateExecutor('enabled', event.target.checked)
                }
              />
            }
            label="Enabled"
          />

          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 4 }}>
              <TextField
                label="Name"
                value={executor.name}
                onChange={(event) =>
                  updateExecutor('name', event.target.value)
                }
                fullWidth
              />
            </Grid>

            <Grid size={{ xs: 12, md: 4 }}>
              <TextField
                label="Host"
                value={executor.host}
                onChange={(event) =>
                  updateExecutor('host', event.target.value)
                }
                fullWidth
              />
            </Grid>

            <Grid size={{ xs: 12, md: 4 }}>
              <TextField
                label="User"
                value={executor.user}
                onChange={(event) =>
                  updateExecutor('user', event.target.value)
                }
                fullWidth
              />
            </Grid>

            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="FFmpeg path"
                value={executor.ffmpegPath}
                onChange={(event) =>
                  updateExecutor('ffmpegPath', event.target.value)
                }
                fullWidth
              />
            </Grid>

            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="FFprobe path"
                value={executor.ffprobePath}
                onChange={(event) =>
                  updateExecutor('ffprobePath', event.target.value)
                }
                fullWidth
              />
            </Grid>

           <Grid size={{ xs: 12 }}>
            <Typography variant="h4">Storage mappings</Typography>
          </Grid>

          <Grid size={{ xs: 12, md: 6 }}>
            <TextField
              label="NAS Raw path"
              value={executor.storageMappings[0]?.localRoot ?? ''}
              onChange={(event) =>
                updateStorageMapping(0, 'localRoot', event.target.value)
              }
              helperText="Path visible to the MVForge backend."
              fullWidth
            />
          </Grid>

          <Grid size={{ xs: 12, md: 6 }}>
            <TextField
              label="Mac Raw path"
              value={executor.storageMappings[0]?.remoteRoot ?? ''}
              onChange={(event) =>
                updateStorageMapping(0, 'remoteRoot', event.target.value)
              }
              helperText="Equivalent path visible to the remote executor."
              fullWidth
            />
          </Grid>

          <Grid size={{ xs: 12, md: 6 }}>
            <TextField
              label="NAS Staging path"
              value={executor.storageMappings[1]?.localRoot ?? ''}
              onChange={(event) =>
                updateStorageMapping(1, 'localRoot', event.target.value)
              }
              helperText="Staging path visible to MVForge."
              fullWidth
            />
          </Grid>

          <Grid size={{ xs: 12, md: 6 }}>
            <TextField
              label="Mac Staging path"
              value={executor.storageMappings[1]?.remoteRoot ?? ''}
              onChange={(event) =>
                updateStorageMapping(1, 'remoteRoot', event.target.value)
              }
              helperText="Equivalent staging path visible to the remote executor."
              fullWidth
            />
          </Grid>

            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="SSH key path"
                value={executor.sshKeyPath}
                onChange={(event) =>
                  updateExecutor('sshKeyPath', event.target.value)
                }
                fullWidth
              />
            </Grid>

            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="Known hosts path"
                value={executor.knownHostsPath}
                onChange={(event) =>
                  updateExecutor('knownHostsPath', event.target.value)
                }
                fullWidth
              />
            </Grid>
          </Grid>

          <Stack direction="row" spacing={1}>
            <Button
              variant="contained"
              startIcon={<SaveIcon />}
              disabled={saving}
              onClick={() => onSave(draft)}
            >
              Save Executor
            </Button>

            <Button
              variant="outlined"
              disabled={probing || !executor.host}
              onClick={() => void probe()}
            >
              {probing ? 'Probing…' : 'Probe'}
            </Button>
          </Stack>

          {probeResult ? (
            <Alert
              severity={
                probeResult.reachable &&
                probeResult.ffmpegAvailable &&
                probeResult.ffprobeAvailable &&
                probeResult.storageAccessible
                  ? 'success'
                  : 'warning'
              }
            >
              {probeResult.reachable
                ? `${probeResult.hostname ?? executor.name} · FFmpeg ${
                    probeResult.ffmpegAvailable ? 'OK' : 'missing'
                  } · FFprobe ${
                    probeResult.ffprobeAvailable ? 'OK' : 'missing'
                  } · VideoToolbox ${
                    probeResult.videoToolbox ? 'available' : 'unavailable'
                  } · Storage ${
                    probeResult.storageAccessible ? 'OK' : 'unavailable'
                  }`
                : probeResult.error ?? 'Executor is unreachable.'}
            </Alert>
          ) : null}
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
	['qsvVbrExtBrcMain8', 'QSV VBR + ExtBRC Main'],
	['qsvVbrLookAheadMain8', 'QSV VBR + LookAhead Main'],
	['qsvVbrAdvancedMain8', 'QSV VBR Advanced Main'],
    ['qsvVbrMain10', 'QSV VBR Main10'],
    ['qsvVbrExtBrcMain10', 'QSV VBR + ExtBRC Main10'],
    ['qsvVbrLookAheadMain10', 'QSV VBR + LookAhead Main10'],
    ['qsvVbrAdvancedMain10', 'QSV VBR Advanced Main10'],

    ['qsvCbrMain8', 'QSV CBR Main'],
	['qsvCbrExtBrcMain8', 'QSV CBR + ExtBRC Main'],
	['qsvCbrLookAheadMain8', 'QSV CBR + LookAhead Main'],
    ['qsvCbrMain10', 'QSV CBR Main10'],
    ['qsvCbrExtBrcMain10', 'QSV CBR + ExtBRC Main10'],
    ['qsvCbrLookAheadMain10', 'QSV CBR + LookAhead Main10'],

    ['qsvAdaptiveIMain10', 'QSV Adaptive I Main10'],
    ['qsvAdaptiveBMain10', 'QSV Adaptive B Main10'],
    ['qsvAdaptiveIMain8', 'QSV Adaptive I Main8'],
    ['qsvAdaptiveBMain8', 'QSV Adaptive B Main8'],

    ['qsvLowPowerMain8', 'QSV Low Power Main'],
    ['qsvLowPowerMain10', 'QSV Low Power Main10'],

    ['qsvLaIcqMain10', 'QSV LA-ICQ Main10'],
	['qsvLaIcqMain8', 'QSV LA-ICQ Main'],

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

function rebuildAssetTypes(current: SettingsForm, assetTypes: LibraryType[]): SettingsForm {
  return buildSettingsForm({ ...current, assetTypes });
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

function isJsonObject(value: string): boolean {
  try {
    const parsed: unknown = JSON.parse(value);
    return typeof parsed === 'object' && parsed !== null && !Array.isArray(parsed);
  } catch {
    return false;
  }
}
