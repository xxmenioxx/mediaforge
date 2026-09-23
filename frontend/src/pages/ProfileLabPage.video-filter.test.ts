import { describe, expect, it } from 'vitest';
import { buildVideoFilterChain, combinedAutomaticMotionFilters, combinedVideoCommandArgs } from './ProfileLabPage';
import type { EffectiveVideoDecision, ProfileInput, ScanResult } from '../api/types';

describe('Profile Lab video filter chain', () => {
  it('places medium Regrain after final sharpen and before field metadata', () => {
    expect(buildVideoFilterChain({
      unsharp: 'light',
      regrain: 'medium',
      correctProgressiveFieldMetadata: true,
    })).toBe('unsharp=5:5:0.25:5:5:0.0,noise=c0s=2:c0f=t,setfield=prog');
  });

  it('uses backend TFF resolution when explicit profile order overrides scan BFF evidence', () => {
    const profile = {
      videoCodec: 'x265_10bit',
      qualityValue: 20,
      workerConfig: {
        cadenceMode: 'inverse_telecine',
        cadenceFieldOrder: 'tff',
      },
    } as unknown as ProfileInput;
    const scan = {
      interlaceAnalysis: {
        automaticFilter: 'fieldmatch=order=bff:combmatch=full,bwdif=mode=send_frame:parity=bff:deint=interlaced,decimate,setfield=prog',
      },
    } as ScanResult;
    const resolvedDecision: EffectiveVideoDecision = {
      cadenceOperation: 'inverse_telecine',
      cadenceFieldOrder: 'tff',
      residualCombingFallback: 'selective',
      effectiveMotionFilters: 'fieldmatch=order=tff:combmatch=full,bwdif=mode=send_frame:parity=tff:deint=interlaced,decimate,setfield=prog',
    };

    const args = combinedVideoCommandArgs(profile, scan, resolvedDecision);
    expect(args[args.indexOf('-vf') + 1]).toContain('fieldmatch=order=tff:combmatch=full');
    expect(args[args.indexOf('-vf') + 1]).not.toContain('fieldmatch=order=bff');
  });

  it('uses the backend IVTC chain when scan automaticFilter is empty', () => {
    const profile = {
      videoCodec: 'x265_10bit',
      qualityValue: 20,
      workerConfig: {
        cadenceMode: 'inverse_telecine',
        cadenceFieldOrder: 'tff',
      },
    } as unknown as ProfileInput;
    const scan = { interlaceAnalysis: { automaticFilter: '' } } as ScanResult;
    const resolvedDecision: EffectiveVideoDecision = {
      cadenceOperation: 'inverse_telecine',
      cadenceFieldOrder: 'tff',
      residualCombingFallback: 'selective',
      effectiveMotionFilters: 'fieldmatch=order=tff:combmatch=full,bwdif=mode=send_frame:parity=tff:deint=interlaced,decimate,setfield=prog',
    };

    const args = combinedVideoCommandArgs(profile, scan, resolvedDecision);
    expect(args[args.indexOf('-vf') + 1]).toContain('fieldmatch=order=tff:combmatch=full');
  });

  it('does not add automatic motion filters without a backend-resolved decision', () => {
    expect(combinedAutomaticMotionFilters()).toEqual([]);
  });
});
