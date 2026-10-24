export const subtitleLanguageOptions = [
  { value: 'spa', label: 'Spanish' },
  { value: 'eng', label: 'English' },
  { value: 'jpn', label: 'Japanese' },
  { value: 'fra', label: 'French' },
  { value: 'deu', label: 'German' },
  { value: 'ita', label: 'Italian' },
  { value: 'por', label: 'Portuguese' },
  { value: 'kor', label: 'Korean' },
  { value: 'zho', label: 'Chinese' },
] as const;

export const trackLanguageOptions = [
  ...subtitleLanguageOptions,
  { value: 'lat', label: 'Latin Spanish' },
  { value: 'und', label: 'Undetermined' },
] as const;

export type CanonicalSubtitleLanguage = typeof subtitleLanguageOptions[number]['value'];

const trackLanguageAliases: Record<string, string> = {
  es: 'spa', esp: 'spa',
  en: 'eng',
  ja: 'jpn',
  fr: 'fra', fre: 'fra', 'fr-fr': 'fra', fr_fr: 'fra',
  de: 'deu', ger: 'deu',
  it: 'ita',
  pt: 'por',
  ko: 'kor', 'ko-kr': 'kor', ko_kr: 'kor',
  zh: 'zho', chi: 'zho',
};

export function normalizeTrackLanguage(value: unknown): string {
  if (typeof value !== 'string') return '';
  const normalized = value.trim().toLowerCase();
  if (!normalized) return 'und';
  return trackLanguageAliases[normalized] ?? normalized;
}

export function normalizeTrackLanguageList(values: readonly unknown[]): string[] {
  const result: string[] = [];
  const seen = new Set<string>();
  for (const value of values) {
    if (typeof value !== 'string' || !value.trim()) continue;
    const language = normalizeTrackLanguage(value);
    if (!seen.has(language)) {
      seen.add(language);
      result.push(language);
    }
  }
  return result;
}

export function trackLanguageLabel(value: unknown): string {
  const language = normalizeTrackLanguage(value);
  const option = trackLanguageOptions.find((item) => item.value === language);
  return option ? `${option.label} (${language})` : language;
}

export function canonicalSubtitleLanguage(value: unknown): CanonicalSubtitleLanguage | '' {
  const normalized = normalizeTrackLanguage(value);
  if (subtitleLanguageOptions.some((option) => option.value === normalized)) return normalized as CanonicalSubtitleLanguage;
  return '';
}

export const subtitlePreferredActionOptions = [
  { value: 'keep', label: 'Keep' },
  { value: 'extract', label: 'Extract' },
  { value: 'keep_and_extract', label: 'Keep and extract' },
] as const;
