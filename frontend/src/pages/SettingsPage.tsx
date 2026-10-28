import {
  Alert,
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Grid,
  MenuItem,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import AddIcon from '@mui/icons-material/Add';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import ArticleIcon from '@mui/icons-material/Article';
import InfoIcon from '@mui/icons-material/Info';
import SaveIcon from '@mui/icons-material/Save';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import { Link as RouterLink, useLocation, useNavigate, useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import { ValidationPolicySettingsCard } from '../components/ValidationPolicySettingsCard';
import { OCRDictionaryManager } from '../components/OCRDictionaryManager';
import { TestEncodeSettingsDomain } from './TestEncodeSettingsDomain';
import { AdvancedSettings } from './settings/AdvancedSettings';
import { GeneralSettingsDomain } from './settings/GeneralSettings';
import { ProvidersIntegrationsSettingsDomain } from './settings/ProvidersIntegrationsSettings';
import { AssetDefaultsSettingsDomain } from './settings/AssetDefaultsSettings';
import { ArchiveSettingsDomain } from './settings/ArchiveSettings';
import { AnalysisSettingsDomain } from './settings/AnalysisSettings';
import type { AnalysisPolicyValue, ProfileSampleEstimatePolicyValue } from './settings/AnalysisSettings';
import { OperationsSettingsDomain } from './settings/OperationsSettings';
import { AssetsStorageSettingsDomain, rebuildAssetTypes, summarizeLibrariesByType } from './settings/AssetsStorageSettings';
import { PipelineSettingsDomain } from './settings/PipelineSettings';
import { OCRAISettingsCard, ocrAISettingsValue } from './settings/OCRAISettingsCard';
import { AIModelRuntimeSettingsCard, AICopilotSettingsCard, AITranslationSettingsCard, aiCopilotSettingsValue } from './settings/OllamaSettings';
export { AITranslationSettingsCard as SubtitleTranslationSettingsCard } from './settings/OllamaSettings';
import { useSettingsDraftValue } from '../hooks/useSettingsDraftValue';
export { HousekeepingCard } from './settings/OperationsSettings';
import { encoderNamesForWorker } from '../utils/workerEncoders';
import { DirectPlayCard, directPlayValue, RemoteExecutorsCard, remoteExecutorsValue, RuntimeProfilesCard, runtimePolicyValue, RuntimeSnapshotCard, WorkersCard } from './settings/RuntimeSettings';
export { WorkersCard } from './settings/RuntimeSettings';
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
import type { AppSetting, MVForgePreferences } from '../api/types';
import {
  archivePathValue,
  arrayValue,
  booleanValue,
  buildSettingsForm,
  initialSettings,
  normalizeExtension,
  normalizeLibraryTypes,
  normalizeStringArray,
  numberValue,
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
  subtitleTranslationSettingsDraftValue,
  subtitleTranslationSettingsPayload,
  subtitleTranslationSettingsValue,
  workerJobStartPacingModeValue,
  workerProgressPercentError,
  workerProgressPercentValue,
  workerSettingsPayload,
} from '../utils/settingsPageHelpers';

function mergeSavedSettings(current: AppSetting[] | undefined, savedSettings: AppSetting[]): AppSetting[] | undefined {
  if (!current) return current;
  const savedByKey = new Map(savedSettings.map((setting) => [setting.key, setting]));
  const merged = current.map((setting) => {
    const saved = savedByKey.get(setting.key);
    if (saved) savedByKey.delete(setting.key);
    return saved ?? setting;
  });
  return [...merged, ...savedByKey.values()];
}

const assetCatalogSettingKeys = new Set(['assetTypes', 'assetCategories', 'assetInventory', 'paths', 'storageRoles', 'workspace']);

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

function identitySettingsValue<T>(value: unknown): T {
  return value as T;
}
export type WorkingHoursValue = {
  enabled: boolean;
  timezone: string;
  preset: string;
  windows: WorkingWindow[];
  outsideWindowPolicy: { startNewHeavyJobs: boolean; continueRunningJobs: boolean; allowAnalysisJobs: boolean; allowValidationJobs: boolean; allowPublisherJobs: boolean; allowCleanupJobs: boolean; allowLabPreviews: boolean };
};
export type WorkspaceValue = { preferredMode: 'copy_to_work_disk' | 'direct_mode'; fallbackMode: 'wait' | 'direct_mode'; allowDirectMode: boolean; estimateRequiredSpace: boolean };
export type HousekeepingValue = {
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
  ordinarySettings: Record<string, Record<string, unknown>>;
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






const emptyDraft: LibraryTypeDraft = {
  key: '',
  label: '',
  description: '',
  extensions: [],
  extensionInput: '',
};

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
  const ollamaProviderSetting = settings.data?.find((item) => item.key === 'ollamaProviderSettings');
  const ollamaProviderConfigurationRevision = ollamaProviderSetting?.updatedAt ?? 'unconfigured';
  const ollamaProviderConfigured = ollamaProviderSetting?.value.state === 'configured';
  const subtitleTranslationSetting = settings.data?.find((item) => item.key === 'subtitleTranslationSettings');
  const autoPublisherSetting = settings.data?.find((item) => item.key === 'pipelineAutomation');
  const autoPublisherEnabled = settings.data
    ? autoPublisherSetting?.value.autoPublisherEnabled === true
    : undefined;
  const legacyAnalysisRecords = settings.data?.find((item) => item.key === 'analysisRecords')?.value.records;
  const hasLegacyAnalysisRecords = Array.isArray(legacyAnalysisRecords) && legacyAnalysisRecords.length > 0;
  const requestedSection = searchParams.get('section');
  const [restoredDraft] = useState(() => readStoredSettingsDraft());
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
    onSuccess: (savedSetting) => {
      if (savedSetting.key === 'housekeeping') {
        previewHousekeeping.reset();
        runHousekeeping.reset();
      }
      queryClient.setQueryData<AppSetting[]>(['settings'], (current) => mergeSavedSettings(current, [savedSetting]));
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
      if (assetCatalogSettingKeys.has(savedSetting.key)) {
        void Promise.all([
          queryClient.invalidateQueries({ queryKey: ['libraries'] }),
          queryClient.invalidateQueries({ queryKey: ['assets'] }),
        ]);
      }
      if (savedSetting.key === 'remoteExecutors') {
        void Promise.all([
          queryClient.invalidateQueries({ queryKey: ['runtime-profiles'] }),
          queryClient.invalidateQueries({ queryKey: ['runtime-state'] }),
        ]);
      }
    },
  });

  const updateSettingsBatch = useMutation({
    mutationFn: api.updateSettingsBatch,
    onSuccess: (savedSettings, variables) => {
      queryClient.setQueryData<AppSetting[]>(['settings'], (current) => mergeSavedSettings(current, savedSettings));
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
      if (variables.settings.some(({ key }) => assetCatalogSettingKeys.has(key))) {
        void Promise.all([
          queryClient.invalidateQueries({ queryKey: ['libraries'] }),
          queryClient.invalidateQueries({ queryKey: ['assets'] }),
        ]);
      }
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
        batch.push(
          { key: 'pipelineAutomation', value: nextForm.pipelineAutomation as unknown as Record<string, unknown> },
          { key: 'workingHours', value: nextForm.ordinarySettings.workingHours },
        );
      }
      if (draftSection === 'analysis') {
        batch.push(
          { key: 'analysisPolicy', value: nextForm.analysisPolicy as unknown as Record<string, unknown> },
          { key: 'profileSampleEstimatePolicy', value: nextForm.profileSampleEstimatePolicy as unknown as Record<string, unknown> },
        );
      }
      if (draftSection === 'runtime') {
        batch.push(
          { key: 'workers', value: workerSettingsPayload(nextForm.workers) as unknown as Record<string, unknown> },
          { key: 'runtimePolicy', value: nextForm.ordinarySettings.runtimePolicy },
          { key: 'directPlay', value: nextForm.ordinarySettings.directPlay },
          { key: 'aiResourceProtectionSettings', value: nextForm.ordinarySettings.aiResourceProtectionSettings },
        );
      }
      if (draftSection === 'assets') {
        const persistedStorageRoles = storageRolesValue(settings.data?.find((item) => item.key === 'storageRoles')?.value);
        const draftStorageRoles = storageRolesValue(nextForm.ordinarySettings.storageRoles);
        const storageRoles = {
          ...persistedStorageRoles,
          ...draftStorageRoles,
          originals_archive: persistedStorageRoles.originals_archive,
        };
        batch.push(
          { key: 'paths', value: pathSettings as unknown as Record<string, unknown> },
          { key: 'assetInventory', value: nextForm.assetInventory as unknown as Record<string, unknown> },
          { key: 'assetTypes', value: { types: nextForm.assetTypes } },
          { key: 'assetCategories', value: { categories: nextForm.assetCategories } },
          { key: 'storageRoles', value: storageRoles as unknown as Record<string, unknown> },
          { key: 'workspace', value: nextForm.ordinarySettings.workspace },
        );
      }
      if (draftSection === 'asset-defaults') {
        batch.push({ key: 'assetDefaults', value: nextForm.ordinarySettings.assetDefaults });
      }
      if (draftSection === 'validation') {
        batch.push({ key: 'validation', value: nextForm.ordinarySettings.validation });
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
        batch.push(
          { key: 'cancellationPolicy', value: nextForm.cancellationPolicy as unknown as Record<string, unknown> },
          { key: 'housekeeping', value: nextForm.ordinarySettings.housekeeping },
        );
      }
      if (draftSection === 'providers-integrations' && !equalSettingsDrafts(nextForm.ordinarySettings.ollamaProviderSettings, baseline.ordinarySettings.ollamaProviderSettings)) {
        batch.push({ key: 'ollamaProviderSettings', value: { endpoint: stringValue(nextForm.ordinarySettings.ollamaProviderSettings.endpoint, '') } });
      }
      if (draftSection === 'ai-copilot' && !equalSettingsDrafts(nextForm.ordinarySettings.aiCopilotSettings, baseline.ordinarySettings.aiCopilotSettings)) {
        batch.push({ key: 'aiCopilotSettings', value: nextForm.ordinarySettings.aiCopilotSettings });
      }
      if (draftSection === 'subtitles') {
        if (!equalSettingsDrafts(nextForm.ordinarySettings.subtitleSettings, baseline.ordinarySettings.subtitleSettings)) {
          batch.push({ key: 'subtitleSettings', value: nextForm.ordinarySettings.subtitleSettings });
        }
        if (!equalSettingsDrafts(nextForm.ordinarySettings.subtitleTranslationSettings, baseline.ordinarySettings.subtitleTranslationSettings)) {
          batch.push({ key: 'subtitleTranslationSettings', value: subtitleTranslationSettingsPayload(nextForm.ordinarySettings.subtitleTranslationSettings) });
        }
        if (!equalSettingsDrafts(nextForm.ordinarySettings.ocrAISettings, baseline.ordinarySettings.ocrAISettings)) {
          batch.push({ key: 'ocrAISettings', value: nextForm.ordinarySettings.ocrAISettings });
        }
      }
      if (draftSection === 'test-encode') {
        batch.push({ key: 'testEncodePolicy', value: nextForm.ordinarySettings.testEncodePolicy });
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
    onSuccess: (savedSettings, variables) => {
      queryClient.setQueryData<AppSetting[]>(['settings'], (current) => mergeSavedSettings(current, savedSettings));
      void queryClient.invalidateQueries({ queryKey: ['settings'] });
      if (['assets', 'archive', 'advanced'].includes(variables.section)) {
        void Promise.all([
          queryClient.invalidateQueries({ queryKey: ['libraries'] }),
          queryClient.invalidateQueries({ queryKey: ['assets'] }),
        ]);
      }
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

  function updateOrdinarySettingDraft(key: keyof SettingsForm['ordinarySettings'], value: Record<string, unknown>) {
    setForm((current) => ({
      ...current,
      ordinarySettings: { ...current.ordinarySettings, [key]: value },
    }));
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
      ordinarySettings: form.ordinarySettings,
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
    runtime: ['runtimePolicy', 'remoteExecutors', 'directPlay', 'aiResourceProtectionSettings'],
    assets: ['storageRoles', 'workspace', 'assetTypes', 'assetCategories', 'paths', 'assetInventory'],
    'asset-defaults': ['assetDefaults'],
    validation: ['validation'],
    'providers-integrations': ['jellyfin', 'ollamaProviderSettings'],
    'ai-copilot': ['aiCopilotSettings'],
    archive: ['originalRetentionPolicy', 'storageRoles'],
    operations: ['housekeeping', 'cancellationPolicy'],
    subtitles: ['subtitleSettings', 'subtitleTranslationSettings', 'ocrAISettings'],
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
                value={form.ordinarySettings.testEncodePolicy}
                saving={coreSavePending}
                draftOnly
                onDraftChange={(value) => updateOrdinarySettingDraft('testEncodePolicy', value)}
                onSave={() => undefined}
              />
            ) : null}

            {section === 'providers-integrations' ? (
              <ProvidersIntegrationsSettingsDomain
                value={settings.data?.find((item) => item.key === 'jellyfin')?.value}
                ollamaValue={form.ordinarySettings.ollamaProviderSettings}
                ollamaSavedValue={ollamaProviderSetting?.value}
                ollamaConfigured={ollamaProviderConfigured}
                providerConfigurationRevision={ollamaProviderConfigurationRevision}
                libraries={libraries.data ?? []}
                saving={updateSetting.isPending}
                saveError={updateSetting.isError && updateSetting.variables?.key === 'jellyfin'
                  ? updateSetting.error instanceof Error ? updateSetting.error.message : 'Please try again.'
                  : undefined}
                onOllamaDraftChange={(value) => updateOrdinarySettingDraft('ollamaProviderSettings', value)}
                onSave={async (value) => { await updateSetting.mutateAsync({ key: 'jellyfin', value }); }}
              />
            ) : null}

            {section === 'ai-copilot' ? (
              <AICopilotSettingsCard
                value={aiCopilotSettingsValue(form.ordinarySettings.aiCopilotSettings)}
                providerConfigured={ollamaProviderConfigured}
                providerConfigurationRevision={ollamaProviderConfigurationRevision}
                onDraftChange={(value) => updateOrdinarySettingDraft('aiCopilotSettings', value)}
              />
            ) : null}

            {section === 'assets' ? (
              <AssetsStorageSettingsDomain
                form={form}
                setForm={setForm}
                librariesByType={librariesByType}
                settings={settings.data ?? []}
                saving={coreSavePending}
                hasLegacyAnalysisRecords={hasLegacyAnalysisRecords}
                onAddType={openAddType}
                onEditType={openEditType}
                onDeleteType={openDeleteType}
                onStorageDraftChange={(roles, workspace) => setForm((current) => ({
                  ...current,
                  ordinarySettings: {
                    ...current.ordinarySettings,
                    storageRoles: roles as unknown as Record<string, unknown>,
                    workspace: workspace as unknown as Record<string, unknown>,
                  },
                }))}
              />
            ) : null}

            {section === 'asset-defaults' ? (
              <AssetDefaultsSettingsDomain
                value={form.ordinarySettings.assetDefaults}
                settings={settings.data ?? []}
                profiles={profilesQuery.data ?? []}
                runtimeEncoders={runtimeSnapshot.data?.encoders}
                saving={coreSavePending}
                draftOnly
                onDraftChange={(value) => updateOrdinarySettingDraft('assetDefaults', value as unknown as Record<string, unknown>)}
                onSave={() => undefined}
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
                value={subtitleSRTFallbackValue(form.ordinarySettings.subtitleSettings)}
                saving={coreSavePending}
                draftOnly
                onDraftChange={(value) => updateOrdinarySettingDraft('subtitleSettings', { ...form.ordinarySettings.subtitleSettings, ...value })}
              />
            ) : null}

            {section === 'subtitles' ? (
              <SubtitleStylingCard
                value={subtitleStylingValue(form.ordinarySettings.subtitleSettings)}
                saving={coreSavePending}
                draftOnly
                onDraftChange={(value) => updateOrdinarySettingDraft('subtitleSettings', { ...form.ordinarySettings.subtitleSettings, subtitleStyling: value as unknown as Record<string, unknown> })}
              />
            ) : null}

            {section === 'subtitles' ? (
              <AITranslationSettingsCard
                value={subtitleTranslationSettingsValue({ ...(subtitleTranslationSetting?.value ?? {}), ...form.ordinarySettings.subtitleTranslationSettings })}
                configDirty={!equalSettingsDrafts(form.ordinarySettings.subtitleTranslationSettings, savedForm.ordinarySettings.subtitleTranslationSettings)}
                providerConfigured={ollamaProviderConfigured}
                providerConfigurationRevision={ollamaProviderConfigurationRevision}
                disableSaving={updateSetting.isPending && updateSetting.variables?.key === 'subtitleTranslationSettings'}
                disableError={updateSetting.isError && updateSetting.variables?.key === 'subtitleTranslationSettings'
                  ? updateSetting.error instanceof Error ? updateSetting.error.message : 'Translation could not be disabled.'
                  : undefined}
                onDraftChange={(value) => updateOrdinarySettingDraft('subtitleTranslationSettings', value)}
                onDisable={() => updateSetting.mutate({
                  key: 'subtitleTranslationSettings',
                  value: { ...subtitleTranslationSettingsPayload(subtitleTranslationSettingsDraftValue(subtitleTranslationSetting?.value)), enabled: false },
                })}
              />
            ) : null}

            {section === 'subtitles' ? (
              <OCRAISettingsCard
                value={ocrAISettingsValue(form.ordinarySettings.ocrAISettings)}
                providerConfigured={ollamaProviderConfigured}
                providerConfigurationRevision={ollamaProviderConfigurationRevision}
                onDraftChange={(value) => updateOrdinarySettingDraft('ocrAISettings', value)}
              />
            ) : null}

            {section === 'subtitles' ? (
              <Accordion>
                <AccordionSummary expandIcon={<ExpandMoreIcon />} aria-controls="ocr-dictionary-panel-content" id="ocr-dictionary-panel-header">
                  <Stack>
                    <Typography variant="h3">OCR Dictionary</Typography>
                    <Typography color="text.secondary" variant="body2">Search and manage contextual OCR correction rules.</Typography>
                  </Stack>
                </AccordionSummary>
                <AccordionDetails>
                  <OCRDictionaryManager />
                </AccordionDetails>
              </Accordion>
            ) : null}

            {section === 'pipeline' ? (
              <PipelineSettingsDomain
                form={form}
                setForm={setForm}
                saving={coreSavePending}
                onWorkingHoursDraftChange={(value) => updateOrdinarySettingDraft('workingHours', value as unknown as Record<string, unknown>)}
              />
            ) : null}

            {section === 'validation' ? (
              <ValidationPolicySettingsCard
                value={form.ordinarySettings.validation}
                saving={coreSavePending}
                autoPublisherEnabled={autoPublisherEnabled}
                draftOnly
                onDraftChange={(value) => updateOrdinarySettingDraft('validation', value)}
                onSave={() => undefined}
              />
            ) : null}

            {section === 'runtime' ? (
              <WorkersCard form={form} setForm={setForm} />
            ) : null}

            {section === 'runtime' ? (
              <AIModelRuntimeSettingsCard
                value={form.ordinarySettings.aiResourceProtectionSettings}
                configured={ollamaProviderConfigured}
                providerConfigurationRevision={ollamaProviderConfigurationRevision}
                onDraftChange={(value) => updateOrdinarySettingDraft('aiResourceProtectionSettings', value)}
              />
            ) : null}

            {section === 'runtime' ? (
              <RuntimeProfilesCard
                catalog={runtimeProfilesQuery.data}
                value={runtimePolicyValue(form.ordinarySettings.runtimePolicy)}
                detectedProfile={runtimeSnapshot.data?.recommendedProfile ?? 'desktop_safe'}
                saving={coreSavePending}
                draftOnly
                onDraftChange={(value) => updateOrdinarySettingDraft('runtimePolicy', value as unknown as Record<string, unknown>)}
                onSave={() => undefined}
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
                value={directPlayValue(form.ordinarySettings.directPlay)}
                saving={coreSavePending}
                draftOnly
                onDraftChange={(value) => updateOrdinarySettingDraft('directPlay', value as unknown as Record<string, unknown>)}
                onSave={() => undefined}
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



            {section === 'operations' ? (
              <OperationsSettingsDomain
                form={form}
                setForm={setForm}
                recovery={{ report: schedulerRecovery.data, loading: schedulerRecovery.isLoading, error: schedulerRecovery.isError, running: runRecovery.isPending, runError: runRecovery.error, onRun: () => runRecovery.mutate() }}
                housekeeping={{
                  value: housekeepingValue(form.ordinarySettings.housekeeping),
                  savedValue: housekeepingValue(settings.data?.find((item) => item.key === 'housekeeping')?.value),
                  saving: coreSavePending,
                  preview: previewHousekeeping.data,
                  result: runHousekeeping.data,
                  busy: previewHousekeeping.isPending || runHousekeeping.isPending,
                  onDraftChange: (value) => updateOrdinarySettingDraft('housekeeping', value as unknown as Record<string, unknown>),
                  onPreview: () => { previewHousekeeping.reset(); runHousekeeping.reset(); previewHousekeeping.mutate(); },
                  onRun: (previewId) => runHousekeeping.mutate({ previewId }),
                }}
              />
            ) : null}

            {section === 'runtime' || section === 'operations' ? (
              <Grid container spacing={2}>
                <Grid size={{ xs: 12 }} sx={{ display: section === 'runtime' ? 'block' : 'none' }}>
                  <RuntimeSnapshotCard snapshot={runtimeSnapshot.data} loading={runtimeSnapshot.isLoading} error={runtimeSnapshot.isError} errorMessage={runtimeSnapshot.error instanceof Error ? runtimeSnapshot.error.message : 'unknown error'} refreshPending={refreshRuntime.isPending} refreshSucceeded={refreshRuntime.isSuccess} refreshErrorMessage={refreshRuntime.isError ? (refreshRuntime.error instanceof Error ? refreshRuntime.error.message : 'unknown error') : undefined} onRefresh={() => refreshRuntime.mutate()} />
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

export type StorageRoleName = typeof storageRoleNames[number];
export type StorageRolesValue = Record<StorageRoleName, { path: string }>;

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

export function SubtitleStylingCard({ value, saving, onSave, onDraftChange, draftOnly = false }: { value: SubtitleStylingSettings; saving: boolean; onSave?: (value: SubtitleStylingSettings) => void; onDraftChange?: (value: SubtitleStylingSettings) => void; draftOnly?: boolean }) {
  const [draft, setDraft] = useSettingsDraftValue(value, identitySettingsValue<SubtitleStylingSettings>, onDraftChange);
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
            {!draftOnly ? <Button startIcon={<SaveIcon />} variant="contained" onClick={() => onSave?.(draft)} disabled={saving || !dirty || hasInvalidColor} sx={{ alignSelf: 'flex-start' }}>Save Subtitle Styling</Button> : null}
            <Button variant="outlined" onClick={resetDraft} disabled={saving || !canReset} sx={{ alignSelf: 'flex-start' }}>Reset to defaults</Button>
          </Stack>
        </Stack>
      </CardContent>
    </Card>
  );
}

export function SubtitleCompatibilityCard({ value, saving, onSave, onDraftChange, draftOnly = false }: { value: SubtitleSRTFallbackSettings; saving: boolean; onSave?: (value: SubtitleSRTFallbackSettings) => void; onDraftChange?: (value: SubtitleSRTFallbackSettings) => void; draftOnly?: boolean }) {
  const [draft, setDraft] = useSettingsDraftValue(value, identitySettingsValue<SubtitleSRTFallbackSettings>, onDraftChange);

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
          {!draftOnly ? <Button
            startIcon={<SaveIcon />}
            variant="contained"
            onClick={() => onSave?.(draft)}
            disabled={saving}
            sx={{ alignSelf: 'flex-start' }}
          >
            Save Subtitle Settings
          </Button> : null}
        </Stack>
      </CardContent>
    </Card>
  );
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
