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

export const subtitlePreferredActionOptions = [
  { value: 'keep', label: 'Keep' },
  { value: 'extract', label: 'Extract' },
  { value: 'keep_and_extract', label: 'Keep and extract' },
] as const;
