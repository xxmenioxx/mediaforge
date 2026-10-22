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

const draftSections: SettingsDraftSection[] = ['general', 'pipeline', 'analysis', 'runtime', 'assets', 'archive', 'operations', 'advanced'];

type LegacySettingsForm = Omit<SettingsForm, 'mvforgePreferences' | 'analysisPolicy' | 'profileSampleEstimatePolicy'>;

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
    && isRecord(record.profileSampleEstimatePolicy);
}

function upgradeStoredForm(value: unknown): SettingsForm | undefined {
  if (isSettingsForm(value)) return value;
  if (!isLegacySettingsForm(value)) return undefined;
  return {
    ...value,
    mvforgePreferences: initialSettings.mvforgePreferences,
    analysisPolicy: initialSettings.analysisPolicy,
    profileSampleEstimatePolicy: initialSettings.profileSampleEstimatePolicy,
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
    const form = value.version === 1 ? upgradeStoredForm(value.form) : isSettingsForm(value.form) ? value.form : undefined;
    const baseline = value.version === 1 ? upgradeStoredForm(value.baseline) : isSettingsForm(value.baseline) ? value.baseline : undefined;
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
      return { pipelineAutomation: form.pipelineAutomation };
    case 'analysis':
      return { analysisPolicy: form.analysisPolicy, profileSampleEstimatePolicy: form.profileSampleEstimatePolicy };
    case 'runtime':
      return { workers: form.workers };
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
      };
    }
    case 'archive':
      return {
        originalsArchivePath: form.paths.originalsArchivePath,
        originalRetentionPolicy: form.originalRetentionPolicy,
      };
    case 'operations':
      return form.cancellationPolicy;
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
      return { ...target, pipelineAutomation: source.pipelineAutomation };
    case 'analysis':
      return { ...target, analysisPolicy: source.analysisPolicy, profileSampleEstimatePolicy: source.profileSampleEstimatePolicy };
    case 'runtime':
      return { ...target, workers: source.workers };
    case 'assets':
      return {
        ...target,
        paths: { ...target.paths, ...source.paths, originalsArchivePath: target.paths.originalsArchivePath },
        assetInventory: source.assetInventory,
        assetTypes: source.assetTypes,
        assetCategories: source.assetCategories,
      };
    case 'archive':
      return {
        ...target,
        paths: { ...target.paths, originalsArchivePath: source.paths.originalsArchivePath },
        originalRetentionPolicy: source.originalRetentionPolicy,
      };
    case 'operations':
      return { ...target, cancellationPolicy: source.cancellationPolicy };
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
