import { describe, expect, it } from 'vitest';
import { testEncodeEligibleAsset } from '../utils/testEncodeEligibility';

describe('testEncodeEligibleAsset', () => {
  it('allows Unverified and other unpublished Library assets', () => {
    expect(testEncodeEligibleAsset({ status: 'unverified' }, 'library')).toBe(true);
    expect(testEncodeEligibleAsset({ status: 'library' }, 'library')).toBe(true);
    expect(testEncodeEligibleAsset({ status: 'accepted' }, 'library')).toBe(true);
  });

  it('blocks Published as-is provenance even when the inventory status is still Unverified', () => {
    expect(testEncodeEligibleAsset({ status: 'published_as_is' }, 'library')).toBe(false);
    expect(testEncodeEligibleAsset({ status: 'unverified', publicationMode: 'as_is' }, 'library')).toBe(false);
  });

  it('does not expose Test Encode for Converted or Archive assets', () => {
    expect(testEncodeEligibleAsset({ status: 'converted' }, 'converted')).toBe(false);
    expect(testEncodeEligibleAsset({ status: 'archive' }, 'archive')).toBe(false);
  });
});
