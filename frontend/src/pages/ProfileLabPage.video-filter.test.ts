import { describe, expect, it } from 'vitest';
import { buildVideoFilterChain, combinedAutomaticMotionFilters } from './ProfileLabPage';
import type { ProfileInput, ScanResult } from '../api/types';

describe('Profile Lab video filter chain', () => {
  it('places medium Regrain after final sharpen and before field metadata', () => {
    expect(buildVideoFilterChain({
      unsharp: 'light',
      regrain: 'medium',
      correctProgressiveFieldMetadata: true,
    })).toBe('unsharp=5:5:0.25:5:5:0.0,noise=c0s=2:c0f=t,setfield=prog');
  });

  it('uses the backend-resolved selective IVTC chain without inferring its parts locally', () => {
    const profile = {
      workerConfig: {
        cadenceMode: 'inverse_telecine',
        deinterlaceMode: 'ivtc_bff',
      },
    } as unknown as ProfileInput;
    const scan = {
      interlaceAnalysis: {
        automaticFilter: 'fieldmatch=order=bff:combmatch=full,bwdif=mode=send_frame:parity=bff:deint=interlaced,decimate,setfield=prog',
      },
    } as ScanResult;

    expect(combinedAutomaticMotionFilters(profile, scan)).toEqual([
      'fieldmatch=order=bff:combmatch=full',
      'bwdif=mode=send_frame:parity=bff:deint=interlaced',
      'decimate',
      'setfield=prog',
    ]);
  });

  it('does not add an IVTC fallback for an unvalidated telecine scan', () => {
    const profile = { workerConfig: { cadenceMode: 'auto', deinterlaceMode: 'auto' } } as unknown as ProfileInput;
    const scan = { interlaceAnalysis: { automaticFilter: '' } } as ScanResult;
    expect(combinedAutomaticMotionFilters(profile, scan)).toEqual([]);
  });
});
