import type {
  AssetScopeConfiguration,
  AssetScopeConfigurationInput,
  ProfileAssignment,
  ProfileAssignmentInput,
} from '../api/types';

export type ConfigurableAssetScope = 'logical_group' | 'path' | 'asset';
export type ScopeConfigurationField = 'video' | 'audio' | 'tracks' | 'category' | 'destination';
export type ScopeValueSelection = 'inherit' | 'value' | 'disabled';

export type ScopeConfigurationEditorValues = {
  videoProfileId: number;
  audioProfileKey: string;
  trackProfileKey: string;
  categorySelection: ScopeValueSelection;
  category: string;
  destinationSelection: ScopeValueSelection;
  destinationLibraryId: number;
};

function normalizedPath(value: string) {
  return value.trim().replace(/\\/g, '/').replace(/\/{2,}/g, '/').replace(/\/$/, '') || '/';
}

export function scopeConfigurationEditorValues(
  targetType: ConfigurableAssetScope,
  scopeKey: string,
  assignments: ProfileAssignment[],
  configurations: AssetScopeConfiguration[],
): ScopeConfigurationEditorValues {
  const targetPath = normalizedPath(scopeKey);
  const scopedAssignments = assignments.filter(
    (assignment) => assignment.targetType === targetType && normalizedPath(assignment.targetPath) === targetPath,
  );
  const configuration = configurations.find(
    (item) => item.scopeType === targetType && normalizedPath(item.scopeKey) === targetPath,
  );
  const video = scopedAssignments.find((item) => item.mediaType === 'video');
  const audio = scopedAssignments.find((item) => item.mediaType === 'audio');
  const tracks = scopedAssignments.find((item) => item.mediaType === 'tracks');

  return {
    videoProfileId: video?.selection === 'profile' ? video.videoProfileId ?? 0 : video?.selection === 'disabled' ? -1 : 0,
    audioProfileKey: audio?.selection === 'profile' ? audio.profileKey ?? '' : audio?.selection === 'disabled' ? '' : '__inherit__',
    trackProfileKey: tracks?.selection === 'profile' ? tracks.profileKey ?? '' : tracks?.selection === 'disabled' ? '' : '__inherit__',
    categorySelection: configuration?.categorySelection ?? 'inherit',
    category: configuration?.category ?? '',
    destinationSelection: configuration?.destinationSelection ?? 'inherit',
    destinationLibraryId: configuration?.destinationLibraryId ?? 0,
  };
}

export function profileAssignmentForScopeChange(
  targetType: ConfigurableAssetScope,
  scopeKey: string,
  mediaType: 'video' | 'audio' | 'tracks',
  value: number | string,
): ProfileAssignmentInput {
  const inherit = value === 0 || value === '__inherit__';
  const disabled = value === '' || (mediaType === 'video' && Number(value) < 0);
  return {
    targetType,
    targetPath: scopeKey,
    mediaType,
    selection: inherit ? 'inherit' : disabled ? 'disabled' : 'profile',
    videoProfileId: mediaType === 'video' && !inherit && !disabled ? Number(value) : 0,
    profileKey: mediaType === 'video' || inherit || disabled ? '' : String(value),
  };
}

export function mergedScopeConfigurationInput(
  targetType: ConfigurableAssetScope,
  scopeKey: string,
  current: AssetScopeConfiguration | undefined,
  changedFields: ReadonlySet<ScopeConfigurationField>,
  values: ScopeConfigurationEditorValues,
): AssetScopeConfigurationInput {
  const categoryChanged = changedFields.has('category');
  const destinationChanged = changedFields.has('destination');
  const categorySelection = categoryChanged ? values.categorySelection : current?.categorySelection ?? 'inherit';
  const destinationSelection = destinationChanged ? values.destinationSelection : current?.destinationSelection ?? 'inherit';

  return {
    scopeType: targetType,
    scopeKey,
    categorySelection,
    category: categorySelection === 'value' ? (categoryChanged ? values.category : current?.category ?? '') : '',
    destinationSelection,
    destinationLibraryId: destinationSelection === 'value'
      ? (destinationChanged ? values.destinationLibraryId : current?.destinationLibraryId ?? 0)
      : 0,
  };
}
