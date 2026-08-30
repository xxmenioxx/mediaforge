import type { AppSetting, AssetConversionOverrideState, StreamMetadataOverride } from './api/types';

export type SubtitleTransform = {
  streamIndex: number;
  format: 'srt' | 'ass';
  removeEmbedded: boolean;
  makeDefault: boolean;
  language: string;
  ocrLanguage?: string;
  ocrMode?: 'raw' | 'clean' | 'accurate';
  title?: string;
};

export type SubtitleDisposition = 'keep' | 'remove' | 'extract' | 'keep_and_extract';
export type SubtitleSidecarFormat = 'original' | 'srt';
export type SubtitleRule = { language?: string; streamIndex?: number; action: SubtitleDisposition; sidecarFormats?: SubtitleSidecarFormat[] };

export type TrackProfile = {
	trackDispositionVersion?: number;
  key: string;
  name: string;
  description: string;
  scope?: 'asset' | 'path';
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
  subtitleTransforms?: SubtitleTransform[];
	  subtitleDisposition: SubtitleDisposition;
	  subtitleRules: SubtitleRule[];
	  subtitleSidecarFormats?: SubtitleSidecarFormat[];
	  attachmentPolicy: 'auto' | 'keep' | 'remove';
	  chapterPolicy: 'keep' | 'remove';
  notes: string;
  disabled?: boolean;
  deletedAt?: string;
};

export const emptyTrackProfile: TrackProfile = {
	trackDispositionVersion: 1,
  key: '', name: '', description: '', videoMode: 'first', audioMode: 'languages', audioLanguages: [],
  audioRequired: true, dropCommentary: true, defaultAudioLanguage: '', subtitleMode: 'forced-or-languages',
  subtitleLanguages: [], subtitlesRequired: false, defaultSubtitleLanguage: '', validationMode: 'review', notes: '',
	  subtitleDisposition: 'keep', subtitleRules: [], subtitleSidecarFormats: ['original'], attachmentPolicy: 'auto', chapterPolicy: 'keep',
};

export function migrateTrackDisposition(profile: TrackProfile, patch: Partial<Pick<TrackProfile, 'subtitleDisposition' | 'subtitleRules' | 'subtitleSidecarFormats' | 'attachmentPolicy' | 'chapterPolicy'>>): TrackProfile {
  return { ...profile, ...patch, trackDispositionVersion: 1 };
}

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
    subtitleTransforms: profile.subtitleTransforms,
  };
}

export function trackProfileWithConversion(profile: TrackProfile, conversion: AssetConversionOverrideState): TrackProfile {
  const assetScope = (profile.scope ?? 'asset') === 'asset';
  return {
    ...profile,
    scope: profile.scope ?? 'asset',
    keepVideoStreams: assetScope ? normalizedOptionalIndexes(conversion.keepVideoStreams) : undefined,
    keepAudioStreams: assetScope ? normalizedOptionalIndexes(conversion.keepAudioStreams) : undefined,
    keepSubtitleStreams: assetScope ? normalizedOptionalIndexes(conversion.keepSubtitleStreams) : undefined,
    videoMetadata: assetScope ? conversion.videoMetadata : undefined,
    audioMetadata: assetScope ? conversion.audioMetadata : undefined,
    subtitleMetadata: assetScope ? conversion.subtitleMetadata : undefined,
    subtitleTransforms: assetScope ? conversion.subtitleTransforms : undefined,
  };
}

export function materializeAssetTrackSelection(conversion: AssetConversionOverrideState, scan: { videoStreams: Array<{ index: number }>; audioStreams: Array<{ index: number }>; subtitleStreams: Array<{ index: number }> }): AssetConversionOverrideState {
  const exact = (selected: number[] | null | undefined, streams: Array<{ index: number }>) =>
    normalizedOptionalIndexes(Array.isArray(selected) ? selected : streams.map((stream) => stream.index)) ?? [];
  return {
    ...conversion,
    keepVideoStreams: exact(conversion.keepVideoStreams, scan.videoStreams),
    keepAudioStreams: exact(conversion.keepAudioStreams, scan.audioStreams),
    keepSubtitleStreams: exact(conversion.keepSubtitleStreams, scan.subtitleStreams),
  };
}

function normalizedOptionalIndexes(value?: number[] | null) {
  if (!Array.isArray(value)) return undefined;
  return Array.from(new Set(value.filter((index) => Number.isInteger(index) && index >= 0))).sort((left, right) => left - right);
}

function normalizeTrackProfile(value: unknown): TrackProfile | null {
  if (!value || typeof value !== 'object') return null;
  const item = value as Record<string, unknown>;
  if (typeof item.key !== 'string' || typeof item.name !== 'string') return null;
  const strings = (entry: unknown) => Array.isArray(entry) ? entry.filter((part): part is string => typeof part === 'string') : [];
  const numbers = (entry: unknown) => Array.isArray(entry) ? entry.filter((part): part is number => Number.isInteger(part)) : undefined;
  const metadata = (entry: unknown) => entry && typeof entry === 'object' ? entry as Record<string, StreamMetadataOverride> : undefined;
  const transforms = (entry: unknown): SubtitleTransform[] | undefined => {
    if (!Array.isArray(entry)) return undefined;
    const values = entry.flatMap((raw) => {
      if (!raw || typeof raw !== 'object') return [];
      const value = raw as Record<string, unknown>;
      if (!Number.isInteger(value.streamIndex) || (value.format !== 'srt' && value.format !== 'ass')) return [];
      return [{
        streamIndex: value.streamIndex as number,
        format: value.format as SubtitleTransform['format'],
        removeEmbedded: value.removeEmbedded !== false,
        makeDefault: value.makeDefault === true,
        language: typeof value.language === 'string' && value.language.trim() ? value.language.trim().toLowerCase() : 'und',
        ocrLanguage: typeof value.ocrLanguage === 'string' && value.ocrLanguage.trim() ? value.ocrLanguage.trim().toLowerCase() : undefined,
        ocrMode: (value.ocrMode === 'raw' || value.ocrMode === 'clean' ? value.ocrMode : 'accurate') as SubtitleTransform['ocrMode'],
        title: typeof value.title === 'string' ? value.title : undefined,
      }];
    });
    return values.length ? values : undefined;
  };
	const disposition = (entry: unknown): SubtitleDisposition => entry === 'remove' || entry === 'extract' || entry === 'keep_and_extract' ? entry : 'keep';
	const sidecarFormats = (entry: unknown): SubtitleSidecarFormat[] => Array.isArray(entry)
		? Array.from(new Set(entry.filter((value): value is SubtitleSidecarFormat => value === 'original' || value === 'srt')))
		: [];
	const rules = (entry: unknown): SubtitleRule[] => Array.isArray(entry) ? entry.flatMap((raw) => {
		if (!raw || typeof raw !== 'object') return [];
		const value = raw as Record<string, unknown>;
		const action = disposition(value.action ?? value.disposition);
		const language = typeof value.language === 'string' && value.language.trim() ? value.language.trim().toLowerCase() : undefined;
		const streamIndex = Number.isInteger(value.streamIndex) && Number(value.streamIndex) >= 0 ? Number(value.streamIndex) : undefined;
		return [{ language, streamIndex, action, sidecarFormats: sidecarFormats(value.sidecarFormats).length ? sidecarFormats(value.sidecarFormats) : undefined }];
	}) : [];
  return {
    ...emptyTrackProfile,
    key: item.key, name: item.name, description: typeof item.description === 'string' ? item.description : '',
	  trackDispositionVersion: typeof item.trackDispositionVersion === 'number' ? item.trackDispositionVersion : undefined,
	  scope: item.scope === 'path' ? 'path' : 'asset',
    sourceAssetPath: typeof item.sourceAssetPath === 'string' ? item.sourceAssetPath : undefined,
    sourceAssetName: typeof item.sourceAssetName === 'string' ? item.sourceAssetName : undefined,
    keepVideoStreams: numbers(item.keepVideoStreams), keepAudioStreams: numbers(item.keepAudioStreams), keepSubtitleStreams: numbers(item.keepSubtitleStreams),
    videoMetadata: metadata(item.videoMetadata), audioMetadata: metadata(item.audioMetadata), subtitleMetadata: metadata(item.subtitleMetadata),
    subtitleTransforms: transforms(item.subtitleTransforms),
	  subtitleDisposition: disposition(item.subtitleDisposition), subtitleRules: rules(item.subtitleRules),
	  subtitleSidecarFormats: sidecarFormats(item.subtitleSidecarFormats).length ? sidecarFormats(item.subtitleSidecarFormats) : undefined,
	  attachmentPolicy: item.attachmentPolicy === 'keep' || item.attachmentPolicy === 'remove' ? item.attachmentPolicy : 'auto',
	  chapterPolicy: item.chapterPolicy === 'remove' ? 'remove' : 'keep',
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
