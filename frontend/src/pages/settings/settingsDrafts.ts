import type { SettingsForm } from '../SettingsPage';
import type { SettingsDraftSection, SettingsSection } from './SettingsNavigation';
import { initialSettings } from '../../utils/settingsPageHelpers';

export const settingsDraftStorageKey = 'mvforge.settings.core-draft.v1';

export type StoredSettingsDraft = {
  version: 2;
  form: SettingsForm;
  baseline: SettingsForm;
  section: SettingsSection;
};

const draftSections: SettingsDraftSection[] = ['general', 'pipeline', 'analysis', 'runtime', 'assets', 'asset-defaults', 'validation', 'archive', 'operations', 'subtitles', 'test-encode', 'advanced'];

type LegacySettingsForm = Omit<SettingsForm, 'mvforgePreferences' | 'analysisPolicy' | 'profileSampleEstimatePolicy' | 'ordinarySettings'>;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isLegacySettingsForm(value: unknown): value is LegacySettingsForm {
  if (!isRecord(value)) return false;
  return isRecord(value.paths)
    && isRecord(value.workers)
    && isRecord(value.pipelineAutomation)
    && isRecord(value.assetInventory)
    && isRecord(value.cancellationPolicy)
    && isRecord(value.originalRetentionPolicy)
    && Array.isArray(value.assetTypes)
    && Array.isArray(value.assetCategories)
    && isRecord(value.advancedJson);
}

function isSettingsForm(value: unknown): value is SettingsForm {
  if (!isLegacySettingsForm(value)) return false;
  const record = value as Record<string, unknown>;
  return isRecord(record.mvforgePreferences)
    && isRecord(record.analysisPolicy)
    && isRecord(record.profileSampleEstimatePolicy)
    && isRecord(record.ordinarySettings);
}

function upgradeStoredForm(value: unknown): SettingsForm | undefined {
  if (isSettingsForm(value)) return value;
  if (!isLegacySettingsForm(value)) return undefined;
  const record = value as unknown as Record<string, unknown>;
  return {
    ...value,
    mvforgePreferences: isRecord(record.mvforgePreferences) ? record.mvforgePreferences as SettingsForm['mvforgePreferences'] : initialSettings.mvforgePreferences,
    analysisPolicy: isRecord(record.analysisPolicy) ? record.analysisPolicy as SettingsForm['analysisPolicy'] : initialSettings.analysisPolicy,
    profileSampleEstimatePolicy: isRecord(record.profileSampleEstimatePolicy) ? record.profileSampleEstimatePolicy as SettingsForm['profileSampleEstimatePolicy'] : initialSettings.profileSampleEstimatePolicy,
    ordinarySettings: isRecord(record.ordinarySettings) ? record.ordinarySettings as SettingsForm['ordinarySettings'] : initialSettings.ordinarySettings,
  };
}

export function readStoredSettingsDraft(): StoredSettingsDraft | undefined {
  try {
    const raw = window.sessionStorage.getItem(settingsDraftStorageKey);
    if (!raw) return undefined;
    const value: unknown = JSON.parse(raw);
    if (!isRecord(value) || (value.version !== 1 && value.version !== 2)) {
      window.sessionStorage.removeItem(settingsDraftStorageKey);
      return undefined;
    }
    const form = upgradeStoredForm(value.form);
    const baseline = upgradeStoredForm(value.baseline);
    if (!form || !baseline) {
      window.sessionStorage.removeItem(settingsDraftStorageKey);
      return undefined;
    }
    return {
      version: 2,
      form,
      baseline,
      section: typeof value.section === 'string' ? value.section as SettingsSection : 'general',
    };
  } catch {
    return undefined;
  }
}

export function settingsDraftSnapshot(form: SettingsForm, section: SettingsDraftSection): unknown {
  switch (section) {
    case 'general':
      return form.mvforgePreferences;
    case 'pipeline':
      return { pipelineAutomation: form.pipelineAutomation, workingHours: form.ordinarySettings.workingHours };
    case 'analysis':
      return { analysisPolicy: form.analysisPolicy, profileSampleEstimatePolicy: form.profileSampleEstimatePolicy };
    case 'runtime':
      return {
        workers: form.workers,
        runtimePolicy: form.ordinarySettings.runtimePolicy,
        directPlay: form.ordinarySettings.directPlay,
      };
    case 'assets': {
      return {
        paths: {
          rawRoot: form.paths.rawRoot,
          libraryRoot: form.paths.libraryRoot,
          stagingPath: form.paths.stagingPath,
          asIsReportsPath: form.paths.asIsReportsPath,
          resultsReportsPath: form.paths.resultsReportsPath,
          logsPath: form.paths.logsPath,
        },
        assetInventory: form.assetInventory,
        assetTypes: form.assetTypes,
        assetCategories: form.assetCategories,
        storageRoles: form.ordinarySettings.storageRoles,
        workspace: form.ordinarySettings.workspace,
      };
    }
    case 'asset-defaults':
      return form.ordinarySettings.assetDefaults;
    case 'validation':
      return form.ordinarySettings.validation;
    case 'archive':
      return {
        originalsArchivePath: form.paths.originalsArchivePath,
        originalRetentionPolicy: form.originalRetentionPolicy,
      };
    case 'operations':
      return { cancellationPolicy: form.cancellationPolicy, housekeeping: form.ordinarySettings.housekeeping };
    case 'subtitles':
      return form.ordinarySettings.ocrAISettings;
    case 'test-encode':
      return form.ordinarySettings.testEncodePolicy;
    case 'advanced':
      return form.advancedJson;
  }
}

export function equalSettingsDrafts(left: unknown, right: unknown): boolean {
  return JSON.stringify(left) === JSON.stringify(right);
}

export function dirtySettingsDraftSections(form: SettingsForm, baseline: SettingsForm): SettingsDraftSection[] {
  return draftSections.filter((section) => !equalSettingsDrafts(
    settingsDraftSnapshot(form, section),
    settingsDraftSnapshot(baseline, section),
  ));
}

export function mergeSettingsDraftSection(
  target: SettingsForm,
  source: SettingsForm,
  section: SettingsDraftSection,
): SettingsForm {
  switch (section) {
    case 'general':
      return { ...target, mvforgePreferences: source.mvforgePreferences };
    case 'pipeline':
      return {
        ...target,
        pipelineAutomation: source.pipelineAutomation,
        ordinarySettings: { ...target.ordinarySettings, workingHours: source.ordinarySettings.workingHours },
      };
    case 'analysis':
      return { ...target, analysisPolicy: source.analysisPolicy, profileSampleEstimatePolicy: source.profileSampleEstimatePolicy };
    case 'runtime':
      return {
        ...target,
        workers: source.workers,
        ordinarySettings: {
          ...target.ordinarySettings,
          runtimePolicy: source.ordinarySettings.runtimePolicy,
          directPlay: source.ordinarySettings.directPlay,
        },
      };
    case 'assets':
      return {
        ...target,
        paths: { ...target.paths, ...source.paths, originalsArchivePath: target.paths.originalsArchivePath },
        assetInventory: source.assetInventory,
        assetTypes: source.assetTypes,
        assetCategories: source.assetCategories,
        ordinarySettings: {
          ...target.ordinarySettings,
          storageRoles: source.ordinarySettings.storageRoles,
          workspace: source.ordinarySettings.workspace,
        },
      };
    case 'asset-defaults':
      return { ...target, ordinarySettings: { ...target.ordinarySettings, assetDefaults: source.ordinarySettings.assetDefaults } };
    case 'validation':
      return { ...target, ordinarySettings: { ...target.ordinarySettings, validation: source.ordinarySettings.validation } };
    case 'archive':
      return {
        ...target,
        paths: { ...target.paths, originalsArchivePath: source.paths.originalsArchivePath },
        originalRetentionPolicy: source.originalRetentionPolicy,
      };
    case 'operations':
      return {
        ...target,
        cancellationPolicy: source.cancellationPolicy,
        ordinarySettings: { ...target.ordinarySettings, housekeeping: source.ordinarySettings.housekeeping },
      };
    case 'subtitles':
      return { ...target, ordinarySettings: { ...target.ordinarySettings, ocrAISettings: source.ordinarySettings.ocrAISettings } };
    case 'test-encode':
      return { ...target, ordinarySettings: { ...target.ordinarySettings, testEncodePolicy: source.ordinarySettings.testEncodePolicy } };
    case 'advanced':
      return { ...target, advancedJson: source.advancedJson };
  }
}

export function mergeSettingsDraftSections(
  persisted: SettingsForm,
  draft: SettingsForm,
  sections: SettingsDraftSection[],
): SettingsForm {
  return sections.reduce((merged, section) => mergeSettingsDraftSection(merged, draft, section), persisted);
}

export function settingsDraftSectionForRoute(section: SettingsSection): SettingsDraftSection | undefined {
  return draftSections.includes(section as SettingsDraftSection) ? section as SettingsDraftSection : undefined;
}

export function allSettingsDraftSections(): SettingsDraftSection[] {
  return [...draftSections];
}
