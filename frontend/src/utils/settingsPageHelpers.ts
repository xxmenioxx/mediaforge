import type { SubtitleTranslationReadiness, SubtitleTranslationReadinessState, SubtitleTranslationSettingsValue } from '../api/types';
import { defaultMVForgePreferences, getMVForgePreferences } from '../mvforgePreferences';
import type { AnalysisMode, AnalysisPolicyValue, ProfileSampleEstimatePolicyValue } from '../pages/settings/AnalysisSettings';
import type {
  LibraryType,
  SettingsForm,
  StorageRoleName,
  StorageRolesValue,
  SubtitleSRTFallbackSettings,
  SubtitleStylePresetSettings,
  SubtitleStylingSettings,
  WorkerJobStartPacingMode,
} from '../pages/SettingsPage';

export const defaultLibraryTypes: LibraryType[] = [
  { key: 'movies', label: 'Movies', description: 'Feature films and movie collections.', extensions: ['.mkv', '.mp4', '.avi', '.mov'] },
  { key: 'tv', label: 'TV Shows', description: 'Episode-based libraries.', extensions: ['.mkv', '.mp4'] },
  { key: 'anime', label: 'Anime', description: 'Anime movies and series.', extensions: ['.mkv', '.mp4'] },
  { key: 'music-videos', label: 'Music Videos', description: 'Music clips and short-form video.', extensions: ['.mkv', '.mp4', '.mov'] },
  { key: 'concerts', label: 'Concerts', description: 'Live performances and concert films.', extensions: ['.mkv', '.mp4'] },
  { key: 'home-videos', label: 'Home Videos', description: 'Personal and family media.', extensions: ['.mp4', '.mov'] },
];

export const defaultAssetCategories = ['movie', 'anime', 'series', 'episode', 'season', 'extras', 'special', 'concert', 'music-video', 'documentary'];
const ordinarySettingsKeys = [
  'testEncodePolicy', 'assetDefaults', 'validation', 'runtimePolicy', 'directPlay',
  'workingHours', 'storageRoles', 'workspace', 'housekeeping', 'ocrAISettings', 'ocrCorrectionMemorySettings',
  'subtitleSettings', 'subtitleTranslationSettings', 'ollamaProviderSettings',
  'aiCopilotSettings', 'aiResourceProtectionSettings',
] as const;

function defaultOrdinarySettings(): Record<string, Record<string, unknown>> {
  return {
    testEncodePolicy: {}, assetDefaults: {}, validation: {}, runtimePolicy: {}, directPlay: {},
    workingHours: {}, storageRoles: {}, workspace: {}, housekeeping: {},
    ocrAISettings: { enabled: false, model: '' },
    ocrCorrectionMemorySettings: { enabled: false, retentionDays: 180, includeInAIPrompts: false },
    subtitleSettings: {},
    subtitleTranslationSettings: {
      provider: '', ollamaModel: '', qualityReviewerModel: '',
      maxCuesPerRequest: 32, maxRequestBytes: 64 * 1024, maxContextCues: 8, maxContextChars: 4096,
    },
    ollamaProviderSettings: { endpoint: '' },
    aiCopilotSettings: { enabled: false, model: '', preference: 'balanced' },
    aiResourceProtectionSettings: {
      enabled: true, maxResidentModels: 1, maxConcurrentInferences: 1, maxConcurrentModelLoads: 1,
      minimumAvailableMemoryBytes: 6 * 1024 * 1024 * 1024, maximumAdmissionWaitSeconds: 300,
      telemetryMaximumAgeSeconds: 60, leaseCleanupGraceSeconds: 30, pressureThresholds: {}, requiredMetrics: [], modelEstimates: [],
    },
  };
}

export function buildSettingsForm(base: Omit<SettingsForm, 'advancedJson'>): SettingsForm {
  return {
    ...base,
    ordinarySettings: Object.fromEntries(ordinarySettingsKeys.map((key) => [
      key,
      { ...(defaultOrdinarySettings()[key] ?? {}), ...(base.ordinarySettings[key] ?? {}) },
    ])),
    advancedJson: {
      paths: JSON.stringify(base.paths, null, 2),
      workers: JSON.stringify(base.workers, null, 2),
      pipelineAutomation: JSON.stringify(base.pipelineAutomation, null, 2),
      assetInventory: JSON.stringify(base.assetInventory, null, 2),
      cancellationPolicy: JSON.stringify(base.cancellationPolicy, null, 2),
      originalRetentionPolicy: JSON.stringify(base.originalRetentionPolicy, null, 2),
      assetTypes: JSON.stringify({ types: base.assetTypes }, null, 2),
      assetCategories: JSON.stringify({ categories: base.assetCategories }, null, 2),
    },
  };
}

export function workerJobStartPacingModeValue(value: unknown): WorkerJobStartPacingMode {
  return value === 'progress' ? 'progress' : 'time';
}

export function workerProgressPercentValue(value: unknown): number {
  return typeof value === 'number' ? value : 75;
}

export function workerProgressPercentError(value: unknown): string | undefined {
  return typeof value === 'number' && Number.isInteger(value) && value >= 1 && value <= 100
    ? undefined
    : 'Enter a whole percentage from 1 to 100.';
}

export function workerSettingsPayload(workers: SettingsForm['workers']): SettingsForm['workers'] {
  return { ...workers };
}

export function subtitleStylingValue(value: Record<string, unknown> | undefined): SubtitleStylingSettings {
  const raw = value?.subtitleStyling as Record<string, unknown> | undefined;
  const builtInOverrides = raw?.builtInOverrides && typeof raw.builtInOverrides === 'object'
    ? raw.builtInOverrides as Record<string, Partial<SubtitleStylePresetSettings>>
    : {};
  const customPresets = Array.isArray(raw?.customPresets) ? raw.customPresets.filter((preset): preset is SubtitleStylePresetSettings => Boolean(preset) && typeof preset === 'object' && typeof (preset as { id?: unknown }).id === 'string') : [];
  const colors = (key: 'savedColors' | 'recentColors', limit: number) => Array.isArray(raw?.[key])
    ? [...new Set(raw[key].filter((item): item is string => typeof item === 'string' && /^#[0-9a-f]{6}$/i.test(item)).map((item) => item.toUpperCase()))].slice(0, limit)
    : [];
  return {
    version: 1,
    enabled: raw?.enabled === true,
    applyToGeneratedASS: raw?.applyToGeneratedASS !== false,
    preserveSourceStyles: raw?.preserveSourceStyles !== false,
    overrideExistingASSStyles: raw?.overrideExistingASSStyles === true,
    builtInOverrides,
    customPresets,
    savedColors: colors('savedColors', 32),
    recentColors: colors('recentColors', 12),
  };
}

export function subtitleNormalizeHexColor(value: string): string | null {
  const trimmed = value.trim().toUpperCase();
  if (/^#[0-9A-F]{3}$/.test(trimmed)) {
    return `#${trimmed.slice(1).split('').map((digit) => `${digit}${digit}`).join('')}`;
  }
  return /^#[0-9A-F]{6}$/.test(trimmed) ? trimmed : null;
}

export function subtitleHexError(value: string): string {
  return subtitleNormalizeHexColor(value) ? '' : 'Use a valid HEX color (#RRGGBB or #RGB).';
}

export function subtitleSRTFallbackValue(value: Record<string, unknown> | undefined): SubtitleSRTFallbackSettings {
  return { generateSRTFallbackForASS: value?.generateSRTFallbackForASS === true };
}

export function subtitleTranslationReadinessState(value: unknown): SubtitleTranslationReadinessState {
  return value === 'disabled' || value === 'not_configured' || value === 'untested' || value === 'ready' ||
    value === 'unavailable' || value === 'model_not_found' || value === 'invalid_configuration'
    ? value
    : 'not_configured';
}

export const defaultSubtitleTranslationSettings: SubtitleTranslationSettingsValue = {
  provider: '',
  ollamaEndpoint: '',
  ollamaModel: '',
  qualityReviewerModel: '',
  maxCuesPerRequest: 32,
  maxRequestBytes: 64 * 1024,
  maxContextCues: 8,
  maxContextChars: 4096,
  enabled: false,
  readiness: 'not_configured',
};

export function subtitleTranslationSettingsValue(value: Record<string, unknown> | undefined): SubtitleTranslationSettingsValue {
  const source = value ?? {};
  return {
    ...source,
    provider: stringValue(source.provider, ''),
    ollamaEndpoint: stringValue(source.ollamaEndpoint, ''),
    ollamaModel: stringValue(source.ollamaModel, ''),
    qualityReviewerModel: stringValue(source.qualityReviewerModel, ''),
    maxCuesPerRequest: numberValue(source.maxCuesPerRequest, defaultSubtitleTranslationSettings.maxCuesPerRequest),
    maxRequestBytes: numberValue(source.maxRequestBytes, defaultSubtitleTranslationSettings.maxRequestBytes),
    maxContextCues: numberValue(source.maxContextCues, defaultSubtitleTranslationSettings.maxContextCues),
    maxContextChars: numberValue(source.maxContextChars, defaultSubtitleTranslationSettings.maxContextChars),
    enabled: booleanValue(source.enabled, false),
    readiness: subtitleTranslationReadinessState(source.readiness),
    readinessConfigIdentity: stringValue(source.readinessConfigIdentity, '') || undefined,
    readinessCheckedAt: stringValue(source.readinessCheckedAt, '') || undefined,
    readinessDiagnostic: stringValue(source.readinessDiagnostic, '') || undefined,
  };
}

const subtitleTranslationDraftProjectionFields = new Set([
  'ollamaEndpoint', 'enabled', 'readiness', 'readinessConfigIdentity',
  'readinessCheckedAt', 'readinessDiagnostic', 'ollamaProviderState',
]);

export function subtitleTranslationSettingsDraftValue(value: Record<string, unknown> | undefined): Record<string, unknown> {
  const source = value ?? {};
  const editable = Object.fromEntries(Object.entries(source).filter(([key]) => !subtitleTranslationDraftProjectionFields.has(key)));
  const normalized = subtitleTranslationSettingsValue(source);
  return {
    ...editable,
    provider: normalized.provider,
    ollamaModel: normalized.ollamaModel,
    qualityReviewerModel: normalized.qualityReviewerModel,
    maxCuesPerRequest: normalized.maxCuesPerRequest,
    maxRequestBytes: normalized.maxRequestBytes,
    maxContextCues: normalized.maxContextCues,
    maxContextChars: normalized.maxContextChars,
  };
}

export function subtitleTranslationSettingsPayload(value: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(Object.entries(value).filter(([key]) => !subtitleTranslationDraftProjectionFields.has(key)));
}

export function subtitleTranslationReadinessForSettings(value: SubtitleTranslationSettingsValue): SubtitleTranslationReadiness {
  return {
    configured: value.readiness !== 'not_configured',
    enabled: value.enabled,
    readiness: value.readiness,
    lastCheckedAt: value.readinessCheckedAt,
    diagnostic: value.readinessDiagnostic,
  };
}

export const storageRoleNames = ['raw', 'library', 'originals_archive', 'work', 'cache', 'reports', 'logs'] as const;

export function storageRolesValue(value: Record<string, unknown> | undefined): StorageRolesValue {
  const defaults: Record<StorageRoleName, string> = { raw: '/media/raw', library: '/media/library', originals_archive: '/media/originals_archive', work: '/media/staging', cache: '/mvforge/cache', reports: '/media/reports', logs: '/media/reports/logs' };
  return Object.fromEntries(storageRoleNames.map((role) => {
    const entry = value?.[role] && typeof value[role] === 'object' ? value[role] as Record<string, unknown> : {};
    return [role, { path: stringValue(entry.path, defaults[role]) }];
  })) as StorageRolesValue;
}

export function settingsToForm(settings: Array<{ key: string; value: Record<string, unknown> }>): SettingsForm {
  const byKey = Object.fromEntries(settings.map((setting) => [setting.key, setting.value]));
  const paths = byKey.paths ?? {};
  const workers = byKey.workers ?? {};
  const pipelineAutomation = byKey.pipelineAutomation ?? {};
  const assetInventory = byKey.assetInventory ?? {};
  const cancellationPolicy = byKey.cancellationPolicy ?? {};
  const originalRetentionPolicy = byKey.originalRetentionPolicy ?? {};
  const analysisPolicy = byKey.analysisPolicy;
  const profileSampleEstimatePolicy = byKey.profileSampleEstimatePolicy;
  const storageRoles = storageRolesValue(byKey.storageRoles);
  const hasOriginalArchiveRole = Boolean(byKey.storageRoles && typeof byKey.storageRoles.originals_archive === 'object');
  const assetTypes = byKey.assetTypes ?? {};
  const assetCategories = byKey.assetCategories ?? {};

  return buildSettingsForm({
    mvforgePreferences: getMVForgePreferences(settings),
    analysisPolicy: analysisPolicyValue(analysisPolicy),
    profileSampleEstimatePolicy: profileSampleEstimatePolicyValue(profileSampleEstimatePolicy),
    paths: {
      rawRoot: stringValue(paths.rawRoot, initialSettings.paths.rawRoot),
      libraryRoot: stringValue(paths.libraryRoot, initialSettings.paths.libraryRoot),
      stagingPath: stringValue(paths.stagingPath, initialSettings.paths.stagingPath),
      originalsArchivePath: archivePathValue(hasOriginalArchiveRole ? storageRoles.originals_archive.path : paths.originalsArchivePath ?? paths.trashPath, initialSettings.paths.originalsArchivePath),
      asIsReportsPath: stringValue(paths.asIsReportsPath, initialSettings.paths.asIsReportsPath),
      resultsReportsPath: stringValue(paths.resultsReportsPath, initialSettings.paths.resultsReportsPath),
      logsPath: stringValue(paths.logsPath, initialSettings.paths.logsPath),
    },
    workers: {
      defaultWorkerName: stringValue(workers.defaultWorkerName, initialSettings.workers.defaultWorkerName),
      autoWorkerEnabled: booleanValue(workers.autoWorkerEnabled, initialSettings.workers.autoWorkerEnabled),
      maxConcurrentJobs: numberValue(workers.maxConcurrentJobs, initialSettings.workers.maxConcurrentJobs),
      maxJobsPerBatch: numberValue(workers.maxJobsPerBatch, initialSettings.workers.maxJobsPerBatch),
      delaySecondsBetweenJobs: numberValue(workers.delaySecondsBetweenJobs, initialSettings.workers.delaySecondsBetweenJobs),
      batchCooldownSeconds: numberValue(workers.batchCooldownSeconds, initialSettings.workers.batchCooldownSeconds),
      jobStartPacingMode: workerJobStartPacingModeValue(workers.jobStartPacingMode),
      startNextJobAtProgressPercent: workerProgressPercentValue(workers.startNextJobAtProgressPercent),
      dryRunOnly: booleanValue(workers.dryRunOnly, initialSettings.workers.dryRunOnly),
    },
    pipelineAutomation: {
      autoAnalysisEnabled: booleanValue(pipelineAutomation.autoAnalysisEnabled, initialSettings.pipelineAutomation.autoAnalysisEnabled),
      forceFreshSnapshotBeforeExecution: booleanValue(pipelineAutomation.forceFreshSnapshotBeforeExecution, initialSettings.pipelineAutomation.forceFreshSnapshotBeforeExecution),
      reviewMode: stringValue(pipelineAutomation.reviewMode, initialSettings.pipelineAutomation.reviewMode) as SettingsForm['pipelineAutomation']['reviewMode'],
      autoExecutionEnabled: booleanValue(pipelineAutomation.autoExecutionEnabled, initialSettings.pipelineAutomation.autoExecutionEnabled),
      autoValidationEnabled: booleanValue(pipelineAutomation.autoValidationEnabled, initialSettings.pipelineAutomation.autoValidationEnabled),
      autoPublisherEnabled: booleanValue(pipelineAutomation.autoPublisherEnabled, initialSettings.pipelineAutomation.autoPublisherEnabled),
      publisherOverwriteEnabled: booleanValue(pipelineAutomation.publisherOverwriteEnabled, initialSettings.pipelineAutomation.publisherOverwriteEnabled),
      publishedJobReconciliationEnabled: booleanValue(pipelineAutomation.publishedJobReconciliationEnabled, initialSettings.pipelineAutomation.publishedJobReconciliationEnabled),
    },
    assetInventory: {
      autoSyncEnabled: booleanValue(assetInventory.autoSyncEnabled, initialSettings.assetInventory.autoSyncEnabled),
      syncIntervalMinutes: numberValue(assetInventory.syncIntervalMinutes, initialSettings.assetInventory.syncIntervalMinutes),
      reconciliationMode: reconciliationModeValue(assetInventory.reconciliationMode),
    },
    cancellationPolicy: {
      keepLogsAndDiagnostics: booleanValue(cancellationPolicy.keepLogsAndDiagnostics, initialSettings.cancellationPolicy.keepLogsAndDiagnostics),
      deleteGeneratedFiles: booleanValue(cancellationPolicy.deleteGeneratedFiles, initialSettings.cancellationPolicy.deleteGeneratedFiles),
      deletePartialOutputFromStaging: booleanValue(cancellationPolicy.deletePartialOutputFromStaging, initialSettings.cancellationPolicy.deletePartialOutputFromStaging),
      controlledRoots: normalizeStringArray(arrayValue(cancellationPolicy.controlledRoots, initialSettings.cancellationPolicy.controlledRoots), initialSettings.cancellationPolicy.controlledRoots),
    },
    originalRetentionPolicy: {
      keepOriginalsDays: numberValue(originalRetentionPolicy.keepOriginalsDays, initialSettings.originalRetentionPolicy.keepOriginalsDays),
      processedOriginalsPath: archivePathValue(originalRetentionPolicy.processedOriginalsPath, initialSettings.originalRetentionPolicy.processedOriginalsPath),
      autoDeleteEnabled: booleanValue(originalRetentionPolicy.autoDeleteEnabled, initialSettings.originalRetentionPolicy.autoDeleteEnabled),
    },
    assetTypes: normalizeLibraryTypes(arrayValue(assetTypes.types, defaultLibraryTypes)),
    assetCategories: normalizeStringArray(arrayValue(assetCategories.categories, defaultAssetCategories), defaultAssetCategories),
    ordinarySettings: Object.fromEntries(ordinarySettingsKeys.map((key) => {
      if (key === 'subtitleTranslationSettings') return [key, subtitleTranslationSettingsDraftValue(byKey[key])];
      if (key === 'ollamaProviderSettings') return [key, { endpoint: stringValue(byKey[key]?.endpoint, '') }];
      return [key, byKey[key] ?? {}];
    })),
  });
}

export const initialSettings: SettingsForm = buildSettingsForm({
  mvforgePreferences: defaultMVForgePreferences,
  analysisPolicy: analysisPolicyValue(undefined),
  profileSampleEstimatePolicy: profileSampleEstimatePolicyValue(undefined),
  paths: {
    rawRoot: '/media/raw', libraryRoot: '/media/library', stagingPath: '/media/staging',
    originalsArchivePath: '/media/originals_archive', asIsReportsPath: '/media/reports/as-is',
    resultsReportsPath: '/media/reports/results', logsPath: '/media/reports/logs',
  },
  workers: {
    defaultWorkerName: 'local-worker', autoWorkerEnabled: true, maxConcurrentJobs: 1, maxJobsPerBatch: 10,
    delaySecondsBetweenJobs: 30, batchCooldownSeconds: 600, jobStartPacingMode: 'time',
    startNextJobAtProgressPercent: 75, dryRunOnly: true,
  },
  pipelineAutomation: {
    autoAnalysisEnabled: false, reviewMode: 'conditional', autoExecutionEnabled: true, autoValidationEnabled: false,
    autoPublisherEnabled: false, publisherOverwriteEnabled: false, publishedJobReconciliationEnabled: false,
    forceFreshSnapshotBeforeExecution: false,
  },
  assetInventory: { autoSyncEnabled: true, syncIntervalMinutes: 60, reconciliationMode: 'exact' },
  cancellationPolicy: {
    keepLogsAndDiagnostics: true, deleteGeneratedFiles: false, deletePartialOutputFromStaging: false,
    controlledRoots: ['/media/staging', '/mwp/work', '/mwp/work/temp', '/tmp/mvforge'],
  },
  originalRetentionPolicy: {
    keepOriginalsDays: 30, autoDeleteEnabled: false,
    processedOriginalsPath: '/media/originals_archive/processed-originals',
  },
  assetTypes: defaultLibraryTypes,
  assetCategories: defaultAssetCategories,
  ordinarySettings: {},
});

export function normalizeLibraryTypes(value: unknown[]): LibraryType[] {
  return value.map((item) => {
    if (!item || typeof item !== 'object') return null;
    const candidate = item as Record<string, unknown>;
    const label = stringValue(candidate.label, '');
    const key = stringValue(candidate.key, slugify(label));
    if (!label || !key) return null;
    return {
      key, label, description: stringValue(candidate.description, ''),
      extensions: arrayValue(candidate.extensions, []).map((extension) => normalizeExtension(String(extension))).filter(Boolean),
    };
  }).filter((type): type is LibraryType => Boolean(type));
}

export function normalizeStringArray(value: unknown[], fallback: string[]) {
  const normalized = value.map((item) => String(item).trim()).filter(Boolean);
  return normalized.length ? normalized : fallback;
}

export function slugify(value: string) {
  return value.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
}

export function normalizeExtension(value: string) {
  const trimmed = value.trim().toLowerCase();
  if (!trimmed) return '';
  return trimmed.startsWith('.') ? trimmed : `.${trimmed}`;
}

export function stringValue(value: unknown, fallback: string) {
  return typeof value === 'string' ? value : fallback;
}

export function numberValue(value: unknown, fallback: number) {
  return typeof value === 'number' ? value : fallback;
}

export function booleanValue(value: unknown, fallback: boolean) {
  return typeof value === 'boolean' ? value : fallback;
}

export function analysisPolicyValue(value: Record<string, unknown> | undefined): AnalysisPolicyValue {
  const mode: AnalysisMode = ['fast', 'balanced', 'thorough', 'custom'].includes(String(value?.mode)) ? value?.mode as AnalysisMode : 'balanced';
  const positions = Array.isArray(value?.positions) ? value.positions.map(Number).filter((item) => Number.isFinite(item) && item > 0 && item < 1) : [];
  return {
    mode,
    adaptiveAnalysis: booleanValue(value?.adaptiveAnalysis, true),
    earlyConfidenceEnabled: booleanValue(value?.earlyConfidenceEnabled, true),
    earlyConfidenceThreshold: Math.max(0.9, Math.min(1, numberValue(value?.earlyConfidenceThreshold, 0.98))),
    initialWindows: Math.max(3, Math.min(5, numberValue(value?.initialWindows, 3))),
    maximumWindows: Math.max(3, Math.min(5, numberValue(value?.maximumWindows, 5))),
    windowSeconds: Math.max(5, Math.min(60, numberValue(value?.windowSeconds, 20))),
    positions: positions.length ? positions : [0.08, 0.27, 0.5, 0.73, 0.92],
    interlaceValidation: value?.interlaceValidation === 'always' ? 'always' : 'automatic',
    cropDepth: value?.cropDepth === 'reduced' || value?.cropDepth === 'full' ? value.cropDepth : 'normal',
    reuseSnapshots: booleanValue(value?.reuseSnapshots, true),
    incrementalRefresh: booleanValue(value?.incrementalRefresh, true),
    concurrentAssets: Math.max(1, Math.min(4, numberValue(value?.concurrentAssets, 1))),
  };
}

export function profileSampleEstimatePolicyValue(value: Record<string, unknown> | undefined): ProfileSampleEstimatePolicyValue {
  return {
    hardwareWindows: Math.max(1, Math.min(10, numberValue(value?.hardwareWindows, 5))),
    softwareWindows: Math.max(1, Math.min(10, numberValue(value?.softwareWindows, 3))),
    operationTimeoutMinutes: Math.max(5, Math.min(120, numberValue(value?.operationTimeoutMinutes, 30))),
  };
}

export function arrayValue(value: unknown, fallback: unknown[]) {
  return Array.isArray(value) ? value : fallback;
}

export function archivePathValue(value: unknown, fallback: string) {
  const path = stringValue(value, fallback);
  return path.startsWith('/media/trash') ? path.replace('/media/trash', '/media/originals_archive') : path;
}

export function reconciliationModeValue(value: unknown): SettingsForm['assetInventory']['reconciliationMode'] {
  return value === 'off' || value === 'review' || value === 'exact' ? value : 'exact';
}
