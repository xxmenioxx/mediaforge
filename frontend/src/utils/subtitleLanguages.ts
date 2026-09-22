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

export type CanonicalSubtitleLanguage = typeof subtitleLanguageOptions[number]['value'];

const subtitleLanguageAliases: Record<string, CanonicalSubtitleLanguage> = {
  es: 'spa',
  esp: 'spa',
  en: 'eng',
  ja: 'jpn',
};

export function canonicalSubtitleLanguage(value: unknown): CanonicalSubtitleLanguage | '' {
  if (typeof value !== 'string') return '';
  const normalized = value.trim().toLowerCase();
  if (subtitleLanguageOptions.some((option) => option.value === normalized)) return normalized as CanonicalSubtitleLanguage;
  return subtitleLanguageAliases[normalized] ?? '';
}

export const subtitlePreferredActionOptions = [
  { value: 'keep', label: 'Keep' },
  { value: 'extract', label: 'Extract' },
  { value: 'keep_and_extract', label: 'Keep and extract' },
] as const;
