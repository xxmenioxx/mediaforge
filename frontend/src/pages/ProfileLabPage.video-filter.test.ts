import { describe, expect, it } from 'vitest';
import { buildVideoFilterChain } from './ProfileLabPage';

describe('Profile Lab video filter chain', () => {
  it('places medium Regrain after final sharpen and before field metadata', () => {
    expect(buildVideoFilterChain({
      unsharp: 'light',
      regrain: 'medium',
      correctProgressiveFieldMetadata: true,
    })).toBe('unsharp=5:5:0.25:5:5:0.0,noise=c0s=2:c0f=t,setfield=prog');
  });
});
