import { describe, expect, it } from 'vitest';
import type { LabResolutionAction, ProfileInput } from '../api/types';
import { labResolutionActionDraft } from './labResolutionAction';

const draft: ProfileInput = {
  name: 'Current video', scope: 'path', description: '', container: 'mkv', videoCodec: 'hevc',
  audioCodec: 'copy', qualityMode: 'icq', qualityValue: 24, preserveHdr: true,
  preserveSubtitles: true, preserveChapters: true,
  workerConfig: { cadenceMode: 'auto', fieldStructureMode: 'auto', cadenceFieldOrder: 'auto', frameStructureGopMode: 'auto' },
};

function action(id: LabResolutionAction['id']): LabResolutionAction {
  return { id, label: id, eligible: true, reason: 'synthetic' };
}

describe('LAB resolution action drafts', () => {
  it('applies Preserve to an asset-scoped draft without mutating the source draft', () => {
    const next = labResolutionActionDraft(draft, action('preserve_source'));
    expect(next.scope).toBe('asset');
    expect(next.workerConfig).toMatchObject({ cadenceMode: 'preserve', fieldStructureMode: 'preserve', cadenceFieldOrder: 'auto' });
    expect(next.workerConfig).not.toBe(draft.workerConfig);
    expect(draft.scope).toBe('path');
    expect(draft.workerConfig).toMatchObject({ cadenceMode: 'auto', fieldStructureMode: 'auto', cadenceFieldOrder: 'auto' });
  });

  it.each(['tff', 'bff'] as const)('keeps explicit %s comparison asset-scoped and does not resolve it automatically', (fieldOrder) => {
    const next = labResolutionActionDraft(draft, action('compare_field_order'), fieldOrder);
    expect(next.scope).toBe('asset');
    expect(next.workerConfig?.cadenceFieldOrder).toBe(fieldOrder);
    expect(next.workerConfig?.cadenceMode).toBe('auto');
    expect(next.workerConfig?.fieldStructureMode).toBe('auto');
    expect(draft.workerConfig?.cadenceFieldOrder).toBe('auto');
  });
});
