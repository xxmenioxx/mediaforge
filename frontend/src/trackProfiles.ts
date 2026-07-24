import type { AppSetting, AssetConversionOverrideState, StreamMetadataOverride } from './api/types';

export type TrackProfile = {
  key: string;
  name: string;
  description: string;
  sourceAssetPath?: string;
  sourceAssetName?: string;
  keepVideoStreams?: number[];
  keepAudioStreams?: number[];
  keepSubtitleStreams?: number[];
  videoMetadata?: Record<string, StreamMetadataOverride>;
  audioMetadata?: Record<string, StreamMetadataOverride>;
  subtitleMetadata?: Record<string, StreamMetadataOverride>;
  videoMode: 'first' | 'all' | 'require-one';
  audioMode: 'all' | 'default' | 'languages' | 'none';
  audioLanguages: string[];
  audioRequired: boolean;
  dropCommentary: boolean;
  defaultAudioLanguage: string;
  subtitleMode: 'all' | 'none' | 'forced' | 'languages' | 'forced-or-languages';
  subtitleLanguages: string[];
  subtitlesRequired: boolean;
  defaultSubtitleLanguage: string;
  validationMode: 'block' | 'review' | 'warn';
  notes: string;
  disabled?: boolean;
  deletedAt?: string;
};

export const emptyTrackProfile: TrackProfile = {
  key: '', name: '', description: '', videoMode: 'first', audioMode: 'languages', audioLanguages: [],
  audioRequired: true, dropCommentary: true, defaultAudioLanguage: '', subtitleMode: 'forced-or-languages',
  subtitleLanguages: [], subtitlesRequired: false, defaultSubtitleLanguage: '', validationMode: 'review', notes: '',
};

export function getTrackProfiles(settings?: AppSetting[], includeDisabled = false): TrackProfile[] {
  const value = settings?.find((setting) => setting.key === 'trackProfiles')?.value.profiles;
  if (!Array.isArray(value)) return [];
  return value.map(normalizeTrackProfile).filter((profile): profile is TrackProfile => Boolean(profile))
    .filter((profile) => includeDisabled || (!profile.disabled && !profile.deletedAt));
}

export function trackProfileOverride(profile: TrackProfile): AssetConversionOverrideState {
  return {
    keepVideoStreams: profile.keepVideoStreams,
    keepAudioStreams: profile.keepAudioStreams,
    keepSubtitleStreams: profile.keepSubtitleStreams,
    videoMetadata: profile.videoMetadata,
    audioMetadata: profile.audioMetadata,
    subtitleMetadata: profile.subtitleMetadata,
  };
}

function normalizeTrackProfile(value: unknown): TrackProfile | null {
  if (!value || typeof value !== 'object') return null;
  const item = value as Record<string, unknown>;
  if (typeof item.key !== 'string' || typeof item.name !== 'string') return null;
  const strings = (entry: unknown) => Array.isArray(entry) ? entry.filter((part): part is string => typeof part === 'string') : [];
  const numbers = (entry: unknown) => Array.isArray(entry) ? entry.filter((part): part is number => Number.isInteger(part)) : undefined;
  const metadata = (entry: unknown) => entry && typeof entry === 'object' ? entry as Record<string, StreamMetadataOverride> : undefined;
  return {
    ...emptyTrackProfile,
    key: item.key, name: item.name, description: typeof item.description === 'string' ? item.description : '',
    sourceAssetPath: typeof item.sourceAssetPath === 'string' ? item.sourceAssetPath : undefined,
    sourceAssetName: typeof item.sourceAssetName === 'string' ? item.sourceAssetName : undefined,
    keepVideoStreams: numbers(item.keepVideoStreams), keepAudioStreams: numbers(item.keepAudioStreams), keepSubtitleStreams: numbers(item.keepSubtitleStreams),
    videoMetadata: metadata(item.videoMetadata), audioMetadata: metadata(item.audioMetadata), subtitleMetadata: metadata(item.subtitleMetadata),
    videoMode: item.videoMode === 'all' || item.videoMode === 'require-one' ? item.videoMode : 'first',
    audioMode: item.audioMode === 'all' || item.audioMode === 'default' || item.audioMode === 'none' ? item.audioMode : 'languages',
    audioLanguages: strings(item.audioLanguages), audioRequired: item.audioRequired !== false, dropCommentary: item.dropCommentary !== false,
    defaultAudioLanguage: typeof item.defaultAudioLanguage === 'string' ? item.defaultAudioLanguage : '',
    subtitleMode: item.subtitleMode === 'all' || item.subtitleMode === 'none' || item.subtitleMode === 'forced' || item.subtitleMode === 'languages' ? item.subtitleMode : 'forced-or-languages',
    subtitleLanguages: strings(item.subtitleLanguages), subtitlesRequired: item.subtitlesRequired === true,
    defaultSubtitleLanguage: typeof item.defaultSubtitleLanguage === 'string' ? item.defaultSubtitleLanguage : '',
    validationMode: item.validationMode === 'block' || item.validationMode === 'warn' ? item.validationMode : 'review',
    notes: typeof item.notes === 'string' ? item.notes : '', disabled: item.disabled === true,
    deletedAt: typeof item.deletedAt === 'string' ? item.deletedAt : undefined,
  };
}
