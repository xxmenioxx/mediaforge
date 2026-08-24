import { describe, expect, it } from 'vitest';

import { labAudioProfileForTestEncode } from './testEncodeDraft';

describe('labAudioProfileForTestEncode', () => {
  const enhancement = { filters: 'anull', outputCodec: 'aac' };

  it.each(['video', 'tracks'] as const)('does not attach Enhanced Audio from %s', (section) => {
    expect(labAudioProfileForTestEncode(section, enhancement)).toBeUndefined();
  });

  it('keeps an explicitly tested Audio draft', () => {
    expect(labAudioProfileForTestEncode('audio', enhancement)).toEqual(enhancement);
  });
});
