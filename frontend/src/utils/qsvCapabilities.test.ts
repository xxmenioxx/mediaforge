import { describe, expect, it } from 'vitest';
import { qsvSelectionWarnings, resolveQSVFeatures } from './qsvCapabilities';

describe('QSV MBBRC capabilities', () => {
  it('keeps ICQ Main8 and Main10 capability evidence independent', () => {
    const capability = {
      qsvIcqMain8: true,
      qsvIcqMain10: true,
      testedModes: { qsvMbbrcIcqMain8: true, qsvMbbrcIcqMain10: false },
    };
    expect(resolveQSVFeatures(capability, { main10: false, rateControl: 'icq' }).mbbrc).toBe(true);
    expect(resolveQSVFeatures(capability, { main10: true, rateControl: 'icq' }).mbbrc).toBe(false);
  });

  it('does not reuse ICQ MBBRC evidence for VBR', () => {
    const features = resolveQSVFeatures({
      qsvVbrMain10: true,
      testedModes: { qsvMbbrcIcqMain10: true, qsvMbbrcVbrMain10: false },
    }, { main10: true, rateControl: 'vbr' });
    expect(features.mbbrc).toBe(false);
  });

  it('does not warn for Auto but warns for an unsupported explicit request', () => {
    const features = resolveQSVFeatures({
      qsvIcqMain10: true,
      testedModes: { qsvMbbrcIcqMain10: false },
    }, { main10: true, rateControl: 'icq' });
    expect(qsvSelectionWarnings(features, { mbbrcMode: 'auto' })).toEqual([]);
    expect(qsvSelectionWarnings(features, { mbbrcMode: 'enabled' })).toContainEqual(expect.stringContaining('MBBRC was explicitly requested'));
  });
});
