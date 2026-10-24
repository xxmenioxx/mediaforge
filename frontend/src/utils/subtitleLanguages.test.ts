import { describe, expect, it } from 'vitest';
import { canonicalSubtitleLanguage, normalizeTrackLanguage, normalizeTrackLanguageList, trackLanguageLabel } from './subtitleLanguages';

describe('track language normalization', () => {
  it.each([
    [' KO ', 'kor'],
    ['ko-KR', 'kor'],
    ['ko_KR', 'kor'],
    ['KOR', 'kor'],
    ['Fr', 'fra'],
    ['fr-FR', 'fra'],
    ['fre', 'fra'],
    ['FRA', 'fra'],
    ['ES', 'spa'],
    ['ESP', 'spa'],
    ['EN', 'eng'],
    ['JA', 'jpn'],
    ['GER', 'deu'],
    ['CHI', 'zho'],
    ['FR-CA', 'fr-ca'],
    ['und', 'und'],
    ['', 'und'],
  ])('normalizes %s to %s', (input, expected) => {
    expect(normalizeTrackLanguage(input)).toBe(expected);
  });

  it('deduplicates equivalent preference aliases without reordering values', () => {
    expect(normalizeTrackLanguageList(['KO', 'kor', 'fr-FR', 'fre', 'en', 'fr-CA'])).toEqual(['kor', 'fra', 'eng', 'fr-ca']);
  });

  it('keeps subtitle validation within the existing supported catalog', () => {
    expect(canonicalSubtitleLanguage('fr-FR')).toBe('fra');
    expect(canonicalSubtitleLanguage('ko-KR')).toBe('kor');
    expect(canonicalSubtitleLanguage('fr-CA')).toBe('');
  });

  it('shows compact canonical labels for supported languages and preserves unknown codes', () => {
    expect(trackLanguageLabel('FR-FR')).toBe('French (fra)');
    expect(trackLanguageLabel('fr-CA')).toBe('fr-ca');
    expect(trackLanguageLabel('und')).toBe('Undetermined (und)');
  });
});
