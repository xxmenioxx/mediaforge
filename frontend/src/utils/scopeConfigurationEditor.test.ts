import { describe, expect, it } from 'vitest';
import type { AssetScopeConfiguration, ProfileAssignment } from '../api/types';
import {
  mergedScopeConfigurationInput,
  profileAssignmentForScopeChange,
  scopeConfigurationEditorValues,
  type ConfigurableAssetScope,
} from './scopeConfigurationEditor';

function assignment(
  targetType: ConfigurableAssetScope,
  targetPath: string,
  mediaType: ProfileAssignment['mediaType'],
  selection: ProfileAssignment['selection'],
  videoProfileId = 0,
  profileKey = '',
): ProfileAssignment {
  return { id: 1, targetType, targetPath, mediaType, selection, videoProfileId, profileKey, createdAt: '', updatedAt: '' };
}

function configuration(scopeType: ConfigurableAssetScope, scopeKey: string): AssetScopeConfiguration {
  return {
    id: 1,
    scopeType,
    scopeKey,
    categorySelection: 'disabled',
    category: '',
    destinationSelection: 'value',
    destinationLibraryId: 7,
    createdAt: '',
    updatedAt: '',
  };
}

describe.each<ConfigurableAssetScope>(['logical_group', 'path'])('%s scope configuration editor', (targetType) => {
  const scopeKey = targetType === 'logical_group' ? '/media/raw/movies/Akira' : '/media/raw/movies/Akira/extras';

  it('loads persisted profile, disabled, inherit, category, and destination states', () => {
    const values = scopeConfigurationEditorValues(targetType, `${scopeKey}/`, [
      assignment(targetType, scopeKey, 'video', 'profile', 11),
      assignment(targetType, scopeKey, 'audio', 'disabled'),
      assignment(targetType, scopeKey, 'tracks', 'profile', 0, 'latam-tracks'),
    ], [configuration(targetType, scopeKey)]);

    expect(values).toEqual({
      videoProfileId: 11,
      audioProfileKey: '',
      trackProfileKey: 'latam-tracks',
      categorySelection: 'disabled',
      category: '',
      destinationSelection: 'value',
      destinationLibraryId: 7,
    });
  });

  it('defaults missing persisted dimensions to inherit', () => {
    expect(scopeConfigurationEditorValues(targetType, scopeKey, [], [])).toEqual({
      videoProfileId: 0,
      audioProfileKey: '__inherit__',
      trackProfileKey: '__inherit__',
      categorySelection: 'inherit',
      category: '',
      destinationSelection: 'inherit',
      destinationLibraryId: 0,
    });
  });

  it('changes only category while preserving the persisted destination', () => {
    const current = configuration(targetType, scopeKey);
    const values = scopeConfigurationEditorValues(targetType, scopeKey, [], [current]);
    const input = mergedScopeConfigurationInput(targetType, scopeKey, current, new Set(['category']), {
      ...values,
      categorySelection: 'inherit',
      category: 'must-not-survive',
    });

    expect(input).toEqual({
      scopeType: targetType,
      scopeKey,
      categorySelection: 'inherit',
      category: '',
      destinationSelection: 'value',
      destinationLibraryId: 7,
    });
  });

  it('changes only destination while preserving the persisted category state', () => {
    const current = configuration(targetType, scopeKey);
    const values = scopeConfigurationEditorValues(targetType, scopeKey, [], [current]);
    const input = mergedScopeConfigurationInput(targetType, scopeKey, current, new Set(['destination']), {
      ...values,
      destinationSelection: 'disabled',
      destinationLibraryId: 99,
    });

    expect(input).toEqual({
      scopeType: targetType,
      scopeKey,
      categorySelection: 'disabled',
      category: '',
      destinationSelection: 'disabled',
      destinationLibraryId: 0,
    });
  });

  it('maps profile values without collapsing inherit and disabled', () => {
    expect(profileAssignmentForScopeChange(targetType, scopeKey, 'video', 0).selection).toBe('inherit');
    expect(profileAssignmentForScopeChange(targetType, scopeKey, 'video', -1).selection).toBe('disabled');
    expect(profileAssignmentForScopeChange(targetType, scopeKey, 'video', 11)).toMatchObject({ selection: 'profile', videoProfileId: 11 });
    expect(profileAssignmentForScopeChange(targetType, scopeKey, 'audio', '__inherit__').selection).toBe('inherit');
    expect(profileAssignmentForScopeChange(targetType, scopeKey, 'audio', '').selection).toBe('disabled');
    expect(profileAssignmentForScopeChange(targetType, scopeKey, 'tracks', 'latam-tracks')).toMatchObject({ selection: 'profile', profileKey: 'latam-tracks' });
  });
});
