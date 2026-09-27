import { describe, expect, it } from 'vitest';
import { hidePreConversionActionsForAsset, testEncodeEligibleAsset } from '../utils/testEncodeEligibility';

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

  it('does not expose Test Encode for Converted assets', () => {
    expect(testEncodeEligibleAsset({ status: 'converted' }, 'converted')).toBe(false);
  });
});

describe('hidePreConversionActionsForAsset', () => {
  const lifecycle = (provenance: string, publicationState = 'published') => ({
    location: 'library' as const,
    lifecycle: 'published',
    provenance,
    validation: 'passed',
    publication: publicationState,
    original: 'unknown',
    currentPublication: { state: publicationState },
  });

  it.each(['converted', 'published_as_is'])('hides pre-conversion actions for Library provenance %s', (provenance) => {
    expect(hidePreConversionActionsForAsset({ lifecycle: lifecycle(provenance) })).toBe(true);
  });

  it('uses the projected current publication to hide actions for a published Library asset', () => {
    expect(hidePreConversionActionsForAsset({ lifecycle: lifecycle('legacy_or_unknown') })).toBe(true);
  });

  it('keeps pre-conversion actions available for a RAW source asset', () => {
    expect(hidePreConversionActionsForAsset({
      lifecycle: { ...lifecycle('legacy_or_unknown', 'unpublished'), location: 'raw' },
    })).toBe(false);
  });
});
