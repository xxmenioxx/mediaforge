import type { LabResolutionAction, ProfileInput } from '../api/types';

export function labResolutionActionDraft(
  draft: ProfileInput,
  action: LabResolutionAction,
  fieldOrder?: 'tff' | 'bff',
): ProfileInput {
  const workerConfig = { ...(draft.workerConfig ?? {}) };
  if (action.id === 'preserve_source') {
    workerConfig.cadenceMode = 'preserve';
    workerConfig.fieldStructureMode = 'preserve';
    workerConfig.cadenceFieldOrder = 'auto';
  } else if (action.id === 'compare_field_order' && fieldOrder) {
    workerConfig.cadenceFieldOrder = fieldOrder;
  }
  return { ...draft, scope: 'asset', workerConfig };
}
