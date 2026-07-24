import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  Collapse,
  Divider,
  FormControlLabel,
  Grid,
  MenuItem,
  Slider,
  Stack,
  Tab,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import FactCheckIcon from '@mui/icons-material/FactCheck';
import GraphicEqIcon from '@mui/icons-material/GraphicEq';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import SaveIcon from '@mui/icons-material/Save';
import TuneIcon from '@mui/icons-material/Tune';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { Dispatch, ReactNode, SetStateAction } from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { api } from '../api/client';
import type {
  AppSetting,
  Asset,
  AssetConversionOverrideState,
  AudioEnhancementProfile,
  MediaStreamInfo,
  Profile,
  ProfileInput,
  ScanResult,
  StreamMetadataOverride,
} from '../api/types';
import { starterAudioProfiles } from '../audioProfiles';
import { MediaSnapshotDetails } from '../components/MediaSnapshotDetails';
import { PageHeader } from '../components/PageHeader';

const eqFrequencies = [60, 120, 250, 500, 1000, 2000, 4000, 8000, 12000] as const;

type LabSection = 'video' | 'audio' | 'tracks';

type TrackProfile = {
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

const languageOptions = [
  { value: 'jpn', label: 'Japanese' },
  { value: 'spa', label: 'Spanish' },
  { value: 'eng', label: 'English' },
  { value: 'lat', label: 'Latin Spanish' },
  { value: 'und', label: 'Undetermined' },
] as const;

const channelModes = [
  { value: 'preserve', label: 'Preserve', description: 'Keep the source channel layout unless other filters/codecs change it.' },
  { value: 'dual-mono', label: 'Mono to Dual Mono', description: 'Safely duplicate mono into left and right channels.' },
  { value: 'force-stereo', label: 'Force Stereo', description: 'Ask FFmpeg to output a stereo layout without pseudo-stereo widening.' },
  { value: 'downmix-mono', label: 'Downmix to Mono', description: 'Mix stereo or multichannel audio into one centered mono track.' },
  { value: 'light-stereo', label: 'Mono to Light Stereo', description: 'Experimental pseudo-stereo using small delay and widening.' },
] as const;

const forceStereoModes = [
  { value: 'auto', label: 'Auto mix', description: 'Recommended. Let FFmpeg convert the source layout into stereo.' },
  { value: 'first-two', label: 'First L/R', description: 'Keep the first two input channels as left and right.' },
  { value: 'duplicate-first', label: 'Duplicate first channel', description: 'Use the first input channel for both left and right.' },
] as const;

const videoCodecOptions = [
  { value: 'x265', label: 'HEVC / x265' },
  { value: 'x264', label: 'H.264 / x264' },
  { value: 'copy', label: 'Keep original video' },
] as const;

const audioCodecOptions = [
  { value: 'copy', label: 'Keep original audio' },
  { value: 'aac', label: 'AAC (best compatibility)' },
  { value: 'ac3', label: 'AC-3 (home theater)' },
  { value: 'opus', label: 'Opus (small files)' },
] as const;

const workModeOptions = [
  { value: '', label: 'Profile default' },
  { value: 'full_encode', label: 'Re-encode video' },
  { value: 'audio_only', label: 'Audio/subtitle fixes only' },
] as const;

const encoderPresetOptions = [
  { value: 'veryfast', label: 'Fast preview', description: 'Faster conversions, larger files. Useful for quick tests.' },
  { value: 'medium', label: 'Balanced', description: 'Recommended default for quality, size, and speed.' },
  { value: 'slow', label: 'Higher compression', description: 'Slower, usually smaller files at the same quality.' },
  { value: 'slower', label: 'Archive patience', description: 'Very slow. Use only when size matters and time is acceptable.' },
] as const;

const pixelFormatOptions = [
  { value: 'auto', label: 'Auto / codec default', description: 'Lets MVForge choose a compatible pixel format from the codec and encoder.' },
  { value: 'yuv420p10le', label: '10-bit Main10', description: 'Recommended for x265/anime/DVD. Helps reduce banding while staying widely playable.' },
  { value: 'yuv420p', label: '8-bit compatibility', description: 'Use for older devices or codecs that do not need 10-bit output.' },
] as const;

const videoEncoderOptions = [
  { value: 'auto', label: 'Auto', description: 'MVForge chooses software unless hardware fallback is enabled and available.' },
  { value: 'hevc_qsv', label: 'Intel Quick Sync', description: 'Fast HEVC hardware encoding for bulk conversion on Intel systems.' },
  { value: 'hevc_nvenc', label: 'NVIDIA NVENC', description: 'Fast HEVC hardware encoding on NVIDIA GPUs.' },
  { value: 'hevc_videotoolbox', label: 'Apple VideoToolbox', description: 'HEVC hardware encoding on supported Apple Silicon and Intel Macs.' },
  { value: 'hevc_amf', label: 'AMD AMF', description: 'Fast HEVC hardware encoding on supported AMD GPUs.' },
  { value: 'libx265', label: 'Software x265', description: 'Slower, usually better compression and quality per GB.' },
] as const;

const deinterlaceOptions = [
  { value: 'off', label: 'Off' },
  { value: 'auto', label: 'Auto at conversion (uses Analysis)' },
  { value: 'force', label: 'Force' },
  { value: 'ivtc_bff', label: 'Inverse telecine (BFF DVD)' },
] as const;

const denoiseOptions = [
  { value: 'off', label: 'Off' },
  { value: 'light', label: 'Light' },
  { value: 'medium', label: 'Medium' },
  { value: 'strong', label: 'Strong' },
] as const;

const debandOptions = [
  { value: 'off', label: 'Off' },
  { value: 'light', label: 'Light' },
  { value: 'medium', label: 'Medium' },
] as const;

const cropOptions = [
  { value: 'off', label: 'Off' },
  { value: 'manual', label: 'Manual' },
] as const;

const videoStarterPresets = [
  {
    key: 'anime-dvd',
    label: 'Anime DVD',
    description: 'DVDs comerciales y rips limpios. Balance ideal de calidad y tamaño.',
    draft: {
      videoCodec: 'x265_10bit',
      qualityValue: 20,
      workerConfig: {
        videoEncoder: 'libx265',
        preferredEncoder: 'software',
        useHardwareIfAvailable: false,
        videoPreset: 'medium',
        pixFmt: 'yuv420p10le',
        deinterlaceMode: 'off',
        denoise: 'off',
        deband: 'off',
        crop: 'off',
        videoFilters: '',
        x265Params: 'aq-mode=3:aq-strength=0.9:deblock=-1,-1',
      },
    },
  },
  {
    key: 'anime-tv-rip',
    label: 'Anime TV Rip',
    description: 'Capturas MPEG-2, fansubs antiguos o fuentes con entrelazado/ruido.',
    draft: {
      videoCodec: 'x265_10bit',
      qualityValue: 21,
      workerConfig: {
        videoEncoder: 'libx265',
        preferredEncoder: 'software',
        useHardwareIfAvailable: false,
        videoPreset: 'medium',
        pixFmt: 'yuv420p10le',
        deinterlaceMode: 'force',
        denoise: 'light',
        deband: 'off',
        crop: 'off',
        videoFilters: 'bwdif=mode=send_frame,hqdn3d=1.5:1.5:6:6',
        x265Params: 'aq-mode=3:aq-strength=0.9:deblock=-1,-1',
      },
    },
  },
  {
    key: 'anime-master',
    label: 'Archivo Maestro',
    description: 'Para animes favoritos o conversiones donde pesa más la fidelidad.',
    draft: {
      videoCodec: 'x265_10bit',
      qualityValue: 18,
      workerConfig: {
        videoEncoder: 'libx265',
        preferredEncoder: 'software',
        useHardwareIfAvailable: false,
        videoPreset: 'slow',
        pixFmt: 'yuv420p10le',
        deinterlaceMode: 'off',
        denoise: 'off',
        deband: 'light',
        crop: 'off',
        videoFilters: '',
        x265Params: 'aq-mode=3:aq-strength=0.9:deblock=-1,-1',
      },
    },
  },
  {
    key: 'anime-high-compression',
    label: 'Compresión Alta',
    description: 'Cuando el espacio importa más. Suele funcionar bien con anime SD.',
    draft: {
      videoCodec: 'x265_10bit',
      qualityValue: 23,
      workerConfig: {
        videoEncoder: 'libx265',
        preferredEncoder: 'software',
        useHardwareIfAvailable: false,
        videoPreset: 'medium',
        pixFmt: 'yuv420p10le',
        deinterlaceMode: 'off',
        denoise: 'off',
        deband: 'off',
        crop: 'off',
        videoFilters: '',
        x265Params: 'aq-mode=3:aq-strength=0.9:deblock=-1,-1',
      },
    },
  },
  {
    key: 'hevc-small-size',
    label: 'HEVC Small Size',
    description: 'Software x265 cuando importa más ahorrar espacio.',
    draft: {
      videoCodec: 'x265_10bit',
      qualityValue: 21,
      workerConfig: {
        videoEncoder: 'libx265',
        preferredEncoder: 'software',
        useHardwareIfAvailable: false,
        videoPreset: 'slow',
        pixFmt: 'yuv420p10le',
        deinterlaceMode: 'off',
        denoise: 'off',
        deband: 'off',
        crop: 'off',
        videoFilters: '',
        x265Params: 'aq-mode=3:aq-strength=0.8:deblock=-1,-1',
        addAacStereoTrack: true,
        aacStereoDefault: false,
        preserveOriginalAudio: true,
        warnSubtitleFormats: true,
        preferSrtSubtitles: false,
      },
    },
  },
  {
    key: 'hevc-archive-quality',
    label: 'HEVC Archive Quality',
    description: 'Software x265 para películas, conciertos y assets difíciles.',
    draft: {
      videoCodec: 'x265_10bit',
      qualityValue: 19,
      workerConfig: {
        videoEncoder: 'libx265',
        preferredEncoder: 'software',
        useHardwareIfAvailable: false,
        videoPreset: 'slow',
        pixFmt: 'yuv420p10le',
        deinterlaceMode: 'off',
        denoise: 'off',
        deband: 'light',
        crop: 'off',
        videoFilters: '',
        x265Params: 'aq-mode=3:aq-strength=0.9:deblock=-1,-1',
        addAacStereoTrack: true,
        aacStereoDefault: false,
        preserveOriginalAudio: true,
        warnSubtitleFormats: true,
        preferSrtSubtitles: false,
      },
    },
  },
  {
    key: 'hevc-balanced-fast',
    label: 'HEVC Balanced Fast',
    description: 'QSV/hardware cuando importa más velocidad y no saturar el NAS.',
    draft: {
      videoCodec: 'x265_10bit',
      qualityValue: 25,
      workerConfig: {
        videoEncoder: 'hevc_qsv',
        preferredEncoder: 'hardware',
        useHardwareIfAvailable: true,
        globalQuality: 25,
        videoPreset: 'medium',
        pixFmt: 'yuv420p10le',
        deinterlaceMode: 'off',
        denoise: 'off',
        deband: 'off',
        crop: 'off',
        videoFilters: '',
        x265Params: '',
        addAacStereoTrack: true,
        aacStereoDefault: false,
        preserveOriginalAudio: true,
        warnSubtitleFormats: true,
        preferSrtSubtitles: false,
      },
    },
  },
  {
    key: 'hevc-bulk-convert',
    label: 'HEVC Bulk Convert',
    description: 'Hardware HEVC para bibliotecas grandes y conversiones masivas.',
    draft: {
      videoCodec: 'x265_10bit',
      qualityValue: 27,
      workerConfig: {
        videoEncoder: 'hevc_qsv',
        preferredEncoder: 'hardware',
        useHardwareIfAvailable: true,
        globalQuality: 27,
        videoPreset: 'medium',
        pixFmt: 'yuv420p10le',
        deinterlaceMode: 'off',
        denoise: 'off',
        deband: 'off',
        crop: 'off',
        videoFilters: '',
        x265Params: '',
        addAacStereoTrack: true,
        aacStereoDefault: false,
        preserveOriginalAudio: true,
        warnSubtitleFormats: true,
        preferSrtSubtitles: false,
      },
    },
  },
] as const;

const emptyVideoDraft: ProfileInput = {
  name: '',
  description: '',
  container: 'mkv',
  videoCodec: 'x265',
  audioCodec: 'copy',
  qualityMode: 'crf',
  qualityValue: 22,
  preserveHdr: true,
  preserveSubtitles: true,
  preserveChapters: true,
  workerConfig: {
    encoder: 'ffmpeg',
    preset: 'profile-lab',
    videoEncoder: 'auto',
    preferredEncoder: 'software',
    useHardwareIfAvailable: false,
    globalQuality: 25,
    videoPreset: 'medium',
    pixFmt: 'yuv420p10le',
    deinterlaceMode: 'off',
    denoise: 'off',
    deband: 'off',
    crop: 'off',
    cropValue: '',
    videoFilters: '',
    x265Params: 'aq-mode=3:aq-strength=0.9:deblock=-1,-1',
    addAacStereoTrack: false,
    aacStereoBitrateKbps: 192,
    aacStereoDefault: false,
    preserveOriginalAudio: true,
    preferSrtSubtitles: false,
    warnSubtitleFormats: true,
  },
};

const emptyAudioDraft: AudioEnhancementProfile = {
  key: '',
  name: '',
  description: '',
  intent: 'Asset-specific restoration',
  filters: 'loudnorm=I=-18:TP=-2:LRA=11',
  rnnoiseModelPath: '',
  channelMode: 'preserve',
  forceStereoMode: 'auto',
  stereoDelayMs: 12,
  stereoWidth: 20,
  eqBands: defaultEqBands(),
  preserveOriginalTrack: true,
  outputCodec: 'aac',
  targetLoudness: -18,
  truePeak: -2,
  notes: '',
};

const emptyTrackDraft: TrackProfile = {
  key: '',
  name: '',
  description: '',
  videoMode: 'first',
  audioMode: 'languages',
  audioLanguages: ['jpn', 'spa'],
  audioRequired: true,
  dropCommentary: true,
  defaultAudioLanguage: 'jpn',
  subtitleMode: 'forced-or-languages',
  subtitleLanguages: ['spa', 'eng'],
  subtitlesRequired: false,
  defaultSubtitleLanguage: 'spa',
  validationMode: 'review',
  notes: '',
};

export function ProfileLabPage() {
  const queryClient = useQueryClient();
  const assets = useQuery({ queryKey: ['assets'], queryFn: api.assets });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const rawAssets = assets.data?.unprocessed ?? [];
  const audioProfiles = useMemo(() => getAudioProfiles(settings.data), [settings.data]);
  const trackProfiles = useMemo(() => getTrackProfiles(settings.data), [settings.data]);
  const [labSection, setLabSection] = useState<LabSection>('video');
  const [assetPath, setAssetPath] = useState('');
  const [start, setStart] = useState('00:00:00');
  const [seconds, setSeconds] = useState(20);
  const [previewMode, setPreviewMode] = useState<'quick' | 'quality'>('quick');
  const [originalPreviewMode, setOriginalPreviewMode] = useState<'direct' | 'compatible'>('direct');
  const [previewNonce, setPreviewNonce] = useState(0);
  const [videoPreviewNonce, setVideoPreviewNonce] = useState(0);
  const [processedVideoCodec, setProcessedVideoCodec] = useState(emptyVideoDraft.videoCodec);
  const [processedVideoQualityValue, setProcessedVideoQualityValue] = useState(emptyVideoDraft.qualityValue);
  const [processedVideoOptions, setProcessedVideoOptions] = useState(videoPreviewOptions(emptyVideoDraft));
  const [videoPreviewStatus, setVideoPreviewStatus] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
  const [audioPreviewNonce, setAudioPreviewNonce] = useState(0);
  const [audioPreviewStreamIndex, setAudioPreviewStreamIndex] = useState<number | null>(null);
  const [processedAudioFilters, setProcessedAudioFilters] = useState('');
  const [processedAudioChannelMode, setProcessedAudioChannelMode] = useState<AudioEnhancementProfile['channelMode']>('preserve');
  const [audioFilterChain, setAudioFilterChain] = useState(effectiveAudioFilters(emptyAudioDraft));
  const [audioFilterChainEdited, setAudioFilterChainEdited] = useState(false);
  const [audioPreviewStatus, setAudioPreviewStatus] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
  const [videoDraft, setVideoDraft] = useState<ProfileInput>(emptyVideoDraft);
  const [selectedVideoStarterPreset, setSelectedVideoStarterPreset] = useState('');
  const [videoAdvancedOpen, setVideoAdvancedOpen] = useState(false);
  const [audioDraft, setAudioDraft] = useState<AudioEnhancementProfile>(emptyAudioDraft);
  const [selectedAudioStarterPreset, setSelectedAudioStarterPreset] = useState('');
  const [trackDraft, setTrackDraft] = useState<TrackProfile>(emptyTrackDraft);
  const [trackConversionDraft, setTrackConversionDraft] = useState<AssetConversionOverrideState>({});
  const previewsRef = useRef<HTMLDivElement | null>(null);
  const selectedAsset = rawAssets.find((asset) => asset.path === assetPath) ?? null;
  const effectivePreviewSeconds = previewMode === 'quick' ? Math.min(seconds, 8) : seconds;
  const currentAudioFilters = effectiveAudioFilters(audioDraft);
  const previewAudioFilters = audioFilterChain.trim() || 'anull';

  useEffect(() => {
    if (!audioFilterChainEdited) {
      setAudioFilterChain(currentAudioFilters || 'anull');
    }
  }, [audioFilterChainEdited, currentAudioFilters]);

  const createVideoProfile = useMutation({
    mutationFn: api.createProfile,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['profiles'] });
    },
  });

  const updateSetting = useMutation({
    mutationFn: api.updateSetting,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });
  const updateAudioSourceStream = useMutation({
    mutationFn: api.updateAssetConversion,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });

  const trackSnapshot = useMutation({ mutationFn: api.scan });
  const availableAudioStreams = trackSnapshot.data?.audioStreams ?? [];
  const selectedAudioStreamIndex = availableAudioStreams.some((stream) => stream.index === audioPreviewStreamIndex)
    ? audioPreviewStreamIndex ?? undefined
    : availableAudioStreams.find((stream) => stream.default)?.index ?? availableAudioStreams[0]?.index;

  useEffect(() => {
    setTrackConversionDraft({});
    if ((labSection === 'audio' || labSection === 'tracks') && assetPath) {
      trackSnapshot.reset();
      trackSnapshot.mutate({ path: assetPath });
    }
  }, [assetPath, labSection]);

  function selectVideoProfile(profileId: number) {
    const profile = (profiles.data ?? []).find((candidate) => candidate.id === profileId);
    if (!profile) {
      return;
    }
    setSelectedVideoStarterPreset('');
    setVideoDraft({
      name: `${profile.name} - ${selectedAsset?.fileName ?? 'Asset'} Lab`,
      description: `Derived in Profile Lab from ${profile.name}${selectedAsset ? ` for ${selectedAsset.relativePath || selectedAsset.fileName}` : ''}.`,
      container: profile.container,
      videoCodec: profile.videoCodec,
      audioCodec: profile.audioCodec,
      qualityMode: profile.qualityMode,
      qualityValue: profile.qualityValue,
      preserveHdr: profile.preserveHdr,
      preserveSubtitles: profile.preserveSubtitles,
      preserveChapters: profile.preserveChapters,
      workerConfig: { ...profile.workerConfig, derivedFromProfileId: profile.id, derivedFromAsset: selectedAsset?.path ?? '' },
    });
  }

  function applyStarterVideoPreset(presetKey: string) {
    const preset = videoStarterPresets.find((candidate) => candidate.key === presetKey);
    if (!preset) {
      return;
    }
    setSelectedVideoStarterPreset(presetKey);
    setVideoDraft((current) => {
      const workerConfig = {
        ...current.workerConfig,
        ...preset.draft.workerConfig,
        starterPreset: preset.key,
      };
      return {
        ...current,
        name: selectedAsset?.fileName ? `${preset.label} - ${selectedAsset.fileName}` : preset.label,
        description: `${preset.description}${selectedAsset ? ` Asset base: ${selectedAsset.relativePath || selectedAsset.fileName}.` : ''}`,
        videoCodec: preset.draft.videoCodec,
        qualityValue: preset.draft.qualityValue,
        workerConfig: {
          ...workerConfig,
          videoFilters: buildVideoFilterChain(workerConfig),
        },
      };
    });
  }

  function selectAudioProfile(profileKey: string) {
    const profile = audioProfiles.find((candidate) => candidate.key === profileKey);
    if (!profile) {
      return;
    }
    setSelectedAudioStarterPreset('');
    const baseName = selectedAsset?.fileName ? `${profile.name} - ${selectedAsset.fileName}` : `${profile.name} Lab`;
    setAudioDraft({
      ...profile,
      key: uniqueKey(slugify(baseName), audioProfiles),
      name: baseName,
      description: `Derived in Profile Lab from ${profile.name}${selectedAsset ? ` for ${selectedAsset.relativePath || selectedAsset.fileName}` : ''}.`,
      notes: [profile.notes, selectedAsset ? `Lab asset: ${selectedAsset.path}` : ''].filter(Boolean).join('\n'),
    });
    setAudioFilterChainEdited(false);
  }

  function applyStarterAudioProfile(profileKey: string) {
    const profile = starterAudioProfiles.find((candidate) => candidate.key === profileKey);
    if (!profile) {
      return;
    }
    setSelectedAudioStarterPreset(profileKey);
    const baseName = selectedAsset?.fileName ? `${profile.name} - ${selectedAsset.fileName}` : `${profile.name} Lab`;
    setAudioDraft({
      ...profile,
      key: uniqueKey(slugify(baseName), audioProfiles),
      name: baseName,
      description: `Started from ${profile.name}${selectedAsset ? ` for ${selectedAsset.relativePath || selectedAsset.fileName}` : ''}.`,
      notes: [profile.notes, selectedAsset ? `Lab asset: ${selectedAsset.path}` : ''].filter(Boolean).join('\n'),
    });
    setAudioFilterChainEdited(false);
  }

  function saveVideoProfile() {
    createVideoProfile.mutate({
      ...videoDraft,
      workerConfig: {
        ...videoDraft.workerConfig,
        source: 'profile-lab',
        derivedFromAsset: selectedAsset?.path ?? '',
      },
    });
  }

  function saveAudioProfile() {
    const normalized = {
      ...audioDraft,
      key: slugify(audioDraft.key || audioDraft.name),
      filters: audioFilterChainEdited ? previewAudioFilters : audioDraft.filters,
      channelMode: audioFilterChainEdited ? 'preserve' as const : audioDraft.channelMode,
      eqBands: audioFilterChainEdited ? defaultEqBands() : normalizeEqBands(audioDraft.eqBands),
    };
    const existing = audioProfiles.filter((profile) => profile.key !== normalized.key);
    updateSetting.mutate({ key: 'audioEnhancementProfiles', value: { profiles: [...existing, normalized] } });
  }

  function saveTrackProfile() {
    const conversion = cleanTrackConversionOverride(trackConversionDraft);
    const normalized = {
      ...trackDraft,
      key: slugify(trackDraft.key || trackDraft.name),
      sourceAssetPath: selectedAsset?.path ?? trackDraft.sourceAssetPath ?? '',
      sourceAssetName: selectedAsset?.fileName ?? trackDraft.sourceAssetName ?? '',
      keepVideoStreams: conversion.keepVideoStreams,
      keepAudioStreams: conversion.keepAudioStreams,
      keepSubtitleStreams: conversion.keepSubtitleStreams,
      videoMetadata: conversion.videoMetadata,
      audioMetadata: conversion.audioMetadata,
      subtitleMetadata: conversion.subtitleMetadata,
      audioLanguages: normalizeStringList(trackDraft.audioLanguages),
      subtitleLanguages: normalizeStringList(trackDraft.subtitleLanguages),
      defaultAudioLanguage: trackDraft.defaultAudioLanguage.trim().toLowerCase(),
      defaultSubtitleLanguage: trackDraft.defaultSubtitleLanguage.trim().toLowerCase(),
    };
    const existing = trackProfiles.filter((profile) => profile.key !== normalized.key);
    updateSetting.mutate({ key: 'trackProfiles', value: { profiles: [...existing, normalized] } });
  }

  function scanTrackAsset(force = false) {
    if (!assetPath) {
      return;
    }
    trackSnapshot.mutate({ path: assetPath, force });
  }

  function selectTrackProfile(profile: TrackProfile | null) {
    if (!profile) {
      return;
    }
    const nextName = `${profile.name} Lab`;
    setTrackDraft({
      ...profile,
      key: uniqueTrackKey(`${profile.key}-lab`, trackProfiles),
      name: nextName,
      sourceAssetPath: selectedAsset?.path ?? profile.sourceAssetPath,
      sourceAssetName: selectedAsset?.fileName ?? profile.sourceAssetName,
    });
    setTrackConversionDraft({
      keepVideoStreams: profile.keepVideoStreams,
      keepAudioStreams: profile.keepAudioStreams,
      keepSubtitleStreams: profile.keepSubtitleStreams,
      videoMetadata: profile.videoMetadata,
      audioMetadata: profile.audioMetadata,
      subtitleMetadata: profile.subtitleMetadata,
    });
  }

  function toggleTrackStream(type: MediaStreamInfo['type'], index: number, keep: boolean) {
    const scan = trackSnapshot.data;
    if (!scan) {
      return;
    }
    const allIndexes = streamIndexesForType(scan, type);
    const current = conversionStreamIndexes(trackConversionDraft, scan, type);
    const next = keep ? normalizeNumberList([...current, index]) : current.filter((candidate) => candidate !== index);
    updateTrackDraftStream(type, selectedOrUndefined(next, allIndexes));
  }

  function updateTrackDraftStream(type: MediaStreamInfo['type'], indexes: number[] | undefined) {
    setTrackConversionDraft((current) => {
      if (type === 'video') {
        return { ...current, keepVideoStreams: indexes };
      }
      if (type === 'audio') {
        return { ...current, keepAudioStreams: indexes };
      }
      return { ...current, keepSubtitleStreams: indexes };
    });
  }

  function updateTrackMetadata(type: 'video' | 'audio' | 'subtitle', index: number, patch: StreamMetadataOverride) {
    setTrackConversionDraft((current) => {
      const key = type === 'video' ? 'videoMetadata' : type === 'audio' ? 'audioMetadata' : 'subtitleMetadata';
      const currentMap = current[key] ?? {};
      const nextItem = cleanStreamMetadataOverride({ ...(currentMap[String(index)] ?? {}), ...patch });
      const nextMap = { ...currentMap };
      if (streamMetadataOverrideEmpty(nextItem)) {
        delete nextMap[String(index)];
      } else {
        nextMap[String(index)] = nextItem;
      }
      return { ...current, [key]: Object.keys(nextMap).length ? nextMap : undefined };
    });
  }

  function processAudioPreview() {
    setProcessedAudioFilters(previewAudioFilters);
    setProcessedAudioChannelMode(audioDraft.channelMode);
    setAudioPreviewStatus('loading');
    setAudioPreviewNonce((current) => current + 1);
    scrollToPreviews();
  }

  function processVideoPreview() {
    setProcessedVideoCodec(videoDraft.videoCodec);
    setProcessedVideoQualityValue(videoDraft.qualityValue);
    setProcessedVideoOptions(videoPreviewOptions(videoDraft));
    setVideoPreviewStatus('loading');
    setVideoPreviewNonce((current) => current + 1);
    scrollToPreviews();
  }

  function resetProcessedAudioPreview() {
    setAudioPreviewNonce(0);
    setAudioPreviewStatus('idle');
    setProcessedAudioFilters('');
    setProcessedAudioChannelMode('preserve');
  }

  function resetProcessedVideoPreview() {
    setVideoPreviewNonce(0);
    setVideoPreviewStatus('idle');
  }

  function resetProcessedPreviews() {
    resetProcessedAudioPreview();
    resetProcessedVideoPreview();
  }

  function scrollToPreviews() {
    window.requestAnimationFrame(() => {
      previewsRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    });
  }

  return (
    <>
      <PageHeader title="Profile Lab" eyebrow="A/B Preview">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 900 }}>
          Compare original samples against video and audio profile drafts, then save asset-specific profiles for repeatable series, anime, or source families.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        <Card sx={{ mb: 2 }}>
          <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
            <Grid container spacing={1.5} alignItems="flex-start">
              <Grid size={{ xs: 12, lg: 4.75 }}>
                <AssetAutocomplete
                  assets={rawAssets}
                  value={selectedAsset}
                  onChange={(asset) => {
                    setAssetPath(asset?.path ?? '');
                    setAudioPreviewStreamIndex(asset?.conversion?.enhancedAudioSourceStreamIndex ?? null);
                    resetProcessedPreviews();
                  }}
                />
              </Grid>
              <Grid size={{ xs: 7, sm: 3, lg: 1.25 }}>
                <TextField
                  label="Start"
                  size="small"
                  value={start}
                  onChange={(event) => {
                    setStart(event.target.value);
                    resetProcessedPreviews();
                  }}
                  placeholder="HH:MM:SS"
                  fullWidth
                />
              </Grid>
              <Grid size={{ xs: 5, sm: 2, lg: 0.75 }}>
                <TextField
                  label="Seconds"
                  size="small"
                  value={seconds}
                  type="number"
                  inputProps={{ min: 5, max: 60 }}
                  onChange={(event) => {
                    setSeconds(Math.min(60, Number(event.target.value)));
                    resetProcessedPreviews();
                  }}
                  onBlur={() => setSeconds((current) => Math.max(5, Math.min(60, current || 5)))}
                  fullWidth
                />
              </Grid>
              <Grid size={{ xs: 12, sm: 4, lg: 2 }}>
                <TextField
                  label="Preview mode"
                  size="small"
                  value={previewMode}
                  onChange={(event) => {
                    setPreviewMode(event.target.value as 'quick' | 'quality');
                    resetProcessedPreviews();
                  }}
                  select
                  fullWidth
                >
                  <MenuItem value="quick">Quick iteration</MenuItem>
                  <MenuItem value="quality">Quality check</MenuItem>
                </TextField>
              </Grid>
              <Grid size={{ xs: 12, sm: 5, lg: 2.25 }}>
                <TextField
                  label="Original playback"
                  size="small"
                  value={originalPreviewMode}
                  onChange={(event) => {
                    setOriginalPreviewMode(event.target.value as 'direct' | 'compatible');
                    resetProcessedPreviews();
                  }}
                  select
                  fullWidth
                >
                  <MenuItem value="direct">Direct original</MenuItem>
                  <MenuItem value="compatible">Browser compatible (Jellyfin / Plex)</MenuItem>
                </TextField>
              </Grid>
              <Grid size={{ xs: 12, sm: 2, lg: 1 }}>
                <Button
                  startIcon={<PlayArrowIcon />}
                  variant="contained"
                  size="small"
                  disabled={!assetPath}
                  onClick={() => setPreviewNonce((current) => current + 1)}
                  fullWidth
                  sx={{ minHeight: 40 }}
                >
                  Preview
                </Button>
              </Grid>
            </Grid>
          </CardContent>
        </Card>
        <Box ref={previewsRef} sx={{ scrollMarginTop: 16, mb: 2 }}>
          <Stack spacing={2}>
              {assetPath && previewNonce > 0 ? (
                <Alert severity={originalPreviewMode === 'compatible' || previewMode === 'quick' ? 'warning' : 'success'}>
                  {originalPreviewMode === 'compatible'
                    ? 'Sample A is a temporary H.264/AAC browser-compatible proxy of the original. It does not change the source file, but it is not a bit-exact visual reference.'
                    : previewMode === 'quick'
                      ? 'Sample A is the untouched original. Sample B is an 8-second, reduced-resolution proxy for fast iteration; use Quality check for final visual comparisons.'
                      : 'Sample A is the untouched original. Sample B uses the requested duration, source resolution, and exact profile settings for a fidelity comparison.'}
                </Alert>
              ) : null}
              {assetPath && previewNonce > 0 ? (
                <Grid container spacing={2} alignItems="stretch">
                  <Grid size={{ xs: 12, lg: 6 }}>
                    <SampleCard
                      title="Sample A"
                      subtitle={originalPreviewMode === 'compatible' ? 'Original content transcoded temporarily for browser playback' : 'Original source stream, no conversion'}
                    >
                      <Stack spacing={2}>
                        {originalPreviewMode === 'compatible' ? (
                          <>
                            <VideoPreview
                              key={`original-compatible-${previewNonce}`}
                              label="Original · browser-compatible H.264 proxy"
                              src={api.compatibleAssetPreviewUrl({
                                path: assetPath,
                                start,
                                seconds,
                                mode: 'quality',
                                videoCodec: 'x264',
                                qualityValue: 20,
                                videoPreset: 'medium',
                                pixFmt: 'yuv420p',
                                videoEncoder: 'libx264',
                                useHardwareIfAvailable: false,
                                globalQuality: 25,
                              })}
                            />
                            <AudioPreview
                              label="Original · browser-compatible AAC stereo 192 kbps"
                              src={api.audioPreviewUrl({ path: assetPath, start, seconds, compatibility: true, streamIndex: selectedAudioStreamIndex })}
                            />
                          </>
                        ) : (
                          <>
                            <VideoPreview key={`original-${previewNonce}`} label="Original video" src={originalPreviewUrl(assetPath, start, seconds)} direct />
                            <AudioPreview label="Original audio" src={`${api.audioPreviewUrl({ path: assetPath, start, seconds, streamIndex: selectedAudioStreamIndex })}&nonce=${previewNonce}`} />
                          </>
                        )}
                      </Stack>
                    </SampleCard>
                  </Grid>
                  <Grid size={{ xs: 12, lg: 6 }}>
                    <SampleCard title="Sample B" subtitle="Current video and audio profile drafts">
                      <Stack spacing={2}>
                        {videoPreviewNonce > 0 ? (
                          <VideoPreview
                            key={`draft-${videoPreviewNonce}-${previewMode}`}
                            label="Video draft"
                            src={api.compatibleAssetPreviewUrl({
                              path: assetPath,
                              start,
                              seconds: effectivePreviewSeconds,
                              videoCodec: processedVideoCodec,
                              qualityValue: processedVideoQualityValue,
                              mode: previewMode,
                              ...processedVideoOptions,
                            })}
                            onStatusChange={setVideoPreviewStatus}
                          />
                        ) : (
                          <Alert severity="info">Process video after choosing the video profile settings to generate Sample B video.</Alert>
                        )}
                        {audioPreviewNonce > 0 ? (
                          <AudioPreview
                            label="Audio draft"
                            src={`${api.audioPreviewUrl({ path: assetPath, start, seconds: effectivePreviewSeconds, filters: processedAudioFilters, streamIndex: selectedAudioStreamIndex })}&nonce=${audioPreviewNonce}`}
                            onStatusChange={setAudioPreviewStatus}
                          />
                        ) : (
                          <Alert severity="info">Process audio after tuning the EQ and filters to generate Sample B audio.</Alert>
                        )}
                      </Stack>
                    </SampleCard>
                  </Grid>
                </Grid>
              ) : (
                <Card>
                  <CardContent>
                    <Stack spacing={1}>
                      <Typography variant="h3">Preview Workbench</Typography>
                      <Typography color="text.secondary">
                        Select an asset and generate a preview to compare Sample A against Sample B.
                      </Typography>
                    </Stack>
                  </CardContent>
                </Card>
              )}
            </Stack>
        </Box>

        <Card sx={{ mb: 2 }}>
          <CardContent sx={{ py: 0, '&:last-child': { pb: 0 } }}>
            <Tabs value={labSection} onChange={(_, value) => setLabSection(value)} variant="scrollable" scrollButtons="auto">
              <Tab value="video" icon={<TuneIcon />} iconPosition="start" label="Video" />
              <Tab value="audio" icon={<GraphicEqIcon />} iconPosition="start" label="Audio" />
              <Tab value="tracks" icon={<FactCheckIcon />} iconPosition="start" label="Tracks" />
            </Tabs>
          </CardContent>
        </Card>

        <Stack spacing={2}>
          {labSection === 'video' ? (
              <Card>
              <CardContent>
                <Stack spacing={2}>
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between">
                    <Stack direction="row" spacing={1} alignItems="center">
                      <TuneIcon color="primary" />
                      <Typography variant="h3">Video Profile Draft</Typography>
                    </Stack>
                    <Stack direction="row" spacing={1} justifyContent={{ xs: 'flex-start', sm: 'flex-end' }}>
                      <Button
                        startIcon={<PlayArrowIcon />}
                        variant="contained"
                        size="small"
                        disabled={!assetPath || previewNonce === 0 || videoPreviewStatus === 'loading'}
                        onClick={processVideoPreview}
                        sx={{ minHeight: 32 }}
                      >
                        {videoPreviewStatus === 'loading' ? 'Processing...' : 'Process Video'}
                      </Button>
                      <Button
                        startIcon={<SaveIcon />}
                        variant="outlined"
                        size="small"
                        disabled={!videoDraft.name || createVideoProfile.isPending}
                        onClick={saveVideoProfile}
                        sx={{ minHeight: 32 }}
                      >
                        Save Profile
                      </Button>
                    </Stack>
                  </Stack>
                  <Alert severity="info">
                    <strong>copy</strong> keeps the original stream untouched. Use it to preserve quality; choose x264/x265 or another codec when
                    you need smaller files, compatibility, or filters.
                  </Alert>
                  <Grid container spacing={2}>
                    <Grid size={{ xs: 12, md: 4 }}>
                      <TextField label="Starter preset" value={selectedVideoStarterPreset} onChange={(event) => applyStarterVideoPreset(event.target.value)} select fullWidth>
                        <MenuItem value="" disabled>
                          Choose a video preset
                        </MenuItem>
                        {videoStarterPresets.map((preset) => (
                          <MenuItem key={preset.key} value={preset.key}>
                            {preset.label}
                          </MenuItem>
                        ))}
                      </TextField>
                    </Grid>
                    <Grid size={{ xs: 12, md: 8 }}>
                      <VideoProfileAutocomplete profiles={profiles.data ?? []} onChange={(profile) => profile ? selectVideoProfile(profile.id) : undefined} />
                    </Grid>
                    <Grid size={{ xs: 12, md: 7 }}>
                      <TextField label="New video profile name" value={videoDraft.name} onChange={(event) => setVideoDraft({ ...videoDraft, name: event.target.value })} fullWidth />
                    </Grid>
                    <Grid size={{ xs: 12, md: 5 }}>
                      <TextField label="Video codec" value={displayVideoCodec(videoDraft.videoCodec)} onChange={(event) => setVideoDraft({ ...videoDraft, videoCodec: event.target.value })} select fullWidth>
                        {videoCodecOptions.map((codec) => (
                          <MenuItem key={codec.value} value={codec.value}>
                            {codec.label}
                          </MenuItem>
                        ))}
                      </TextField>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 4 }}>
                      <TextField label="Container" value={videoDraft.container} onChange={(event) => setVideoDraft({ ...videoDraft, container: event.target.value })} select fullWidth>
                        {['mkv', 'mp4'].map((container) => (
                          <MenuItem key={container} value={container}>
                            {container}
                          </MenuItem>
                        ))}
                      </TextField>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 4 }}>
                      <TextField label="Audio codec" value={videoDraft.audioCodec} onChange={(event) => setVideoDraft({ ...videoDraft, audioCodec: event.target.value })} select fullWidth>
                        {audioCodecOptions.map((codec) => (
                          <MenuItem key={codec.value} value={codec.value}>
                            {codec.label}
                          </MenuItem>
                        ))}
                      </TextField>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 4 }}>
                      <TextField
                        label="Work mode"
                        value={videoWorkerValue(videoDraft, 'processingMode')}
                        onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'processingMode', event.target.value)}
                        select
                        fullWidth
                      >
                        {workModeOptions.map((mode) => (
                          <MenuItem key={mode.value || 'default'} value={mode.value}>
                            {mode.label}
                          </MenuItem>
                        ))}
                      </TextField>
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, bgcolor: 'rgba(255,255,255,0.018)' }}>
                        <Grid container spacing={2} alignItems="flex-start">
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="Encoder"
                              value={videoWorkerValue(videoDraft, 'videoEncoder', 'auto')}
                              onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoEncoder', event.target.value)}
                              helperText={videoEncoderDescription(videoWorkerValue(videoDraft, 'videoEncoder', 'auto'))}
                              select
                              size="small"
                              disabled={videoDraft.videoCodec === 'copy'}
                              fullWidth
                            >
                              {videoEncoderOptions.map((encoder) => (
                                <MenuItem key={encoder.value} value={encoder.value} disabled={isHardwareEncoderOption(encoder.value) && !videoWorkerBool(videoDraft, 'useHardwareIfAvailable')}>
                                  {encoder.label}
                                </MenuItem>
                              ))}
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="Hardware quality"
                              value={numberWorkerValue(videoDraft, 'globalQuality', videoDraft.qualityValue || 25)}
                              onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'globalQuality', Number(event.target.value))}
                              type="number"
                              inputProps={{ min: 15, max: 35 }}
                              helperText={hardwareQualityHelper(videoDraft.qualityValue)}
                              size="small"
                              disabled={videoDraft.videoCodec === 'copy' || !videoWorkerBool(videoDraft, 'useHardwareIfAvailable')}
                              fullWidth
                            />
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <FormControlLabel
                              control={
                                <Checkbox
                                  checked={videoWorkerBool(videoDraft, 'useHardwareIfAvailable')}
                                  onChange={(event) => setVideoDraft((current) => ({ ...current, workerConfig: { ...current.workerConfig, useHardwareIfAvailable: event.target.checked, ...(event.target.checked ? {} : { videoEncoder: 'libx265', preferredEncoder: 'software' }) } }))}
                                />
                              }
                              label="Use hardware if available"
                            />
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <FormControlLabel
                              control={
                                <Checkbox
                                  checked={videoAACTrackEnabled(videoDraft)}
                                  onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'addAacStereoTrack', event.target.checked)}
                                />
                              }
                              label="Add AAC stereo track"
                            />
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="AAC compatibility quality"
                              value={numberWorkerValue(videoDraft, 'aacStereoBitrateKbps', 192)}
                              onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'aacStereoBitrateKbps', Number(event.target.value))}
                              disabled={!videoAACTrackEnabled(videoDraft)}
                              helperText="192 kb/s recommended"
                              select
                              fullWidth
                            >
                              {[128, 160, 192, 224, 256].map((value) => (
                                <MenuItem key={value} value={value}>{value} kb/s</MenuItem>
                              ))}
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <FormControlLabel
                              control={
                                <Checkbox
                                  checked={videoAACTrackDefault(videoDraft)}
                                  disabled={!videoAACTrackEnabled(videoDraft)}
                                  onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'aacStereoDefault', event.target.checked)}
                                />
                              }
                              label="Make AAC stereo default"
                            />
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <FormControlLabel
                              control={
                                <Checkbox
                                  checked={videoWorkerBool(videoDraft, 'preserveOriginalAudio', true)}
                                  onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'preserveOriginalAudio', event.target.checked)}
                                />
                              }
                              label="Original audio secondary"
                            />
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              select
                              fullWidth
                              label="Subtitle output format"
                              value={videoSubtitleOutputFormat(videoDraft)}
                              onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'subtitleOutputFormat', event.target.value)}
                              helperText="Bitmap tracks remain unchanged"
                            >
                              <MenuItem value="source">Preserve source format</MenuItem>
                              <MenuItem value="srt">Convert text to SRT</MenuItem>
                              <MenuItem value="ass">Convert text to ASS</MenuItem>
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <FormControlLabel
                              control={
                                <Checkbox
                                  checked={videoWorkerBool(videoDraft, 'warnSubtitleFormats', true)}
                                  onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'warnSubtitleFormats', event.target.checked)}
                                />
                              }
                              label="Warn ASS/PGS/VobSub"
                            />
                          </Grid>
                        </Grid>
                      </Box>
                    </Grid>
                    <Grid size={{ xs: 12, md: 5 }}>
                      <Box sx={{ maxWidth: 440, pb: 1.5 }}>
                        <Stack spacing={0.5}>
                          <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                            <Typography fontWeight={700}>Quality</Typography>
                            <Tooltip title="Lower CRF keeps more detail. Higher CRF creates smaller files.">
                              <Chip label={videoDraft.videoCodec === 'copy' ? 'Original' : `CRF ${videoDraft.qualityValue}`} size="small" />
                            </Tooltip>
                          </Stack>
                          <Slider
                            value={videoDraft.qualityValue}
                            min={14}
                            max={30}
                            step={1}
                            marks={[
                              { value: 14, label: '14' },
                              { value: 22, label: '22' },
                              { value: 30, label: '30' },
                            ]}
                            disabled={videoDraft.videoCodec === 'copy'}
                            onChange={(_, value) => setVideoDraft({ ...videoDraft, qualityValue: Array.isArray(value) ? value[0] : value })}
                            valueLabelDisplay="auto"
                            size="small"
                            sx={{
                              mx: 1,
                              mt: 0.25,
                              mb: 2,
                              width: 'calc(100% - 16px)',
                              '& .MuiSlider-markLabel': { fontSize: 12, color: 'text.secondary', top: 28 },
                            }}
                          />
                        </Stack>
                      </Box>
                    </Grid>
                    <Grid size={{ xs: 12, md: 7 }}>
                      <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap sx={{ alignItems: 'center', minHeight: 48 }}>
                        <PreserveButton
                          active={videoDraft.preserveHdr}
                          label="HDR"
                          tooltip="Keep HDR metadata and source color information when possible."
                          onClick={() => setVideoDraft((current) => ({ ...current, preserveHdr: !current.preserveHdr }))}
                        />
                        <PreserveButton
                          active={videoDraft.preserveSubtitles}
                          label="Subtitles"
                          tooltip="Keep subtitle streams unless a tracks profile removes specific ones."
                          onClick={() => setVideoDraft((current) => ({ ...current, preserveSubtitles: !current.preserveSubtitles }))}
                        />
                        <PreserveButton
                          active={videoDraft.preserveChapters}
                          label="Chapters"
                          tooltip="Keep chapter markers from the source."
                          onClick={() => setVideoDraft((current) => ({ ...current, preserveChapters: !current.preserveChapters }))}
                        />
                      </Stack>
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <Grid container spacing={2}>
                        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                          <TextField
                            label="Encoding speed"
                            value={videoWorkerValue(videoDraft, 'videoPreset', 'medium')}
                            onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoPreset', event.target.value)}
                            helperText={encoderPresetDescription(videoWorkerValue(videoDraft, 'videoPreset', 'medium'))}
                            select
                            size="small"
                            fullWidth
                          >
                            {encoderPresetOptions.map((preset) => (
                              <MenuItem key={preset.value} value={preset.value}>
                                {preset.label}
                              </MenuItem>
                            ))}
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                          <TextField
                            label="Color depth"
                            value={videoWorkerValue(videoDraft, 'pixFmt', 'yuv420p10le')}
                            onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'pixFmt', event.target.value)}
                            helperText={pixelFormatDescription(videoWorkerValue(videoDraft, 'pixFmt', 'yuv420p10le'))}
                            select
                            size="small"
                            fullWidth
                          >
                            {pixelFormatOptions.map((pixFmt) => (
                              <MenuItem key={pixFmt.value} value={pixFmt.value}>
                                {pixFmt.label}
                              </MenuItem>
                            ))}
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 12 }}>
                          <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, bgcolor: 'rgba(255,255,255,0.018)' }}>
                            <Stack spacing={2}>
                              <Stack spacing={0.4}>
                                <Typography fontWeight={700}>Image cleanup</Typography>
                                <Typography color="text.secondary" variant="body2">
                                  These controls build the FFmpeg video filter chain for Sample B and saved profiles.
                                </Typography>
                              </Stack>
                              <Grid container spacing={2}>
                                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                  <TextField
                                    label="Deinterlace"
                                    value={videoFilterControlValue(videoDraft, 'deinterlaceMode', 'auto')}
                                    onChange={(event) => updateVideoFilterControl(setVideoDraft, 'deinterlaceMode', event.target.value)}
                                    select
                                    fullWidth
                                  >
                                    {deinterlaceOptions.map((option) => (
                                      <MenuItem key={option.value} value={option.value}>
                                        {option.label}
                                      </MenuItem>
                                    ))}
                                  </TextField>
                                </Grid>
                                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                  <TextField
                                    label="Denoise"
                                    value={videoFilterControlValue(videoDraft, 'denoise', 'off')}
                                    onChange={(event) => updateVideoFilterControl(setVideoDraft, 'denoise', event.target.value)}
                                    select
                                    fullWidth
                                  >
                                    {denoiseOptions.map((option) => (
                                      <MenuItem key={option.value} value={option.value}>
                                        {option.label}
                                      </MenuItem>
                                    ))}
                                  </TextField>
                                </Grid>
                                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                  <TextField
                                    label="Deband"
                                    value={videoFilterControlValue(videoDraft, 'deband', 'off')}
                                    onChange={(event) => updateVideoFilterControl(setVideoDraft, 'deband', event.target.value)}
                                    select
                                    fullWidth
                                  >
                                    {debandOptions.map((option) => (
                                      <MenuItem key={option.value} value={option.value}>
                                        {option.label}
                                      </MenuItem>
                                    ))}
                                  </TextField>
                                </Grid>
                                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                  <TextField
                                    label="Crop"
                                    value={videoFilterControlValue(videoDraft, 'crop', 'off')}
                                    onChange={(event) => updateVideoFilterControl(setVideoDraft, 'crop', event.target.value)}
                                    select
                                    fullWidth
                                  >
                                    {cropOptions.map((option) => (
                                      <MenuItem key={option.value} value={option.value}>
                                        {option.label}
                                      </MenuItem>
                                    ))}
                                  </TextField>
                                </Grid>
                                {videoFilterControlValue(videoDraft, 'crop', 'off') === 'manual' ? (
                                  <Grid size={{ xs: 12, md: 6 }}>
                                    <TextField
                                      label="Manual crop"
                                      value={videoWorkerValue(videoDraft, 'cropValue')}
                                      onChange={(event) => updateVideoFilterControl(setVideoDraft, 'cropValue', event.target.value)}
                                      placeholder="iw:ih-80:0:40"
                                      helperText="FFmpeg crop expression: width:height:x:y."
                                      fullWidth
                                    />
                                  </Grid>
                                ) : null}
                                <Grid size={{ xs: 12, md: 4 }}>
                                  <ImageAdjustmentSlider
                                    label="Brightness"
                                    tooltip="Small luma correction for sources that are too dark or washed out."
                                    value={numberWorkerValue(videoDraft, 'brightness', 0)}
                                    min={-100}
                                    max={100}
                                    valueLabel={(value) => (value === 0 ? 'Neutral' : `${value > 0 ? '+' : ''}${value}%`)}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'brightness', String(value))}
                                  />
                                </Grid>
                                <Grid size={{ xs: 12, md: 4 }}>
                                  <ImageAdjustmentSlider
                                    label="Contrast"
                                    tooltip="Adjust image separation. Keep changes subtle for animation."
                                    value={numberWorkerValue(videoDraft, 'contrast', 100)}
                                    min={50}
                                    max={150}
                                    valueLabel={(value) => `${value}%`}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'contrast', String(value))}
                                  />
                                </Grid>
                                <Grid size={{ xs: 12, md: 4 }}>
                                  <ImageAdjustmentSlider
                                    label="Saturation"
                                    tooltip="Color intensity correction. Useful for faded sources."
                                    value={numberWorkerValue(videoDraft, 'saturation', 100)}
                                    min={50}
                                    max={150}
                                    valueLabel={(value) => `${value}%`}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'saturation', String(value))}
                                  />
                                </Grid>
                                <Grid size={{ xs: 12, md: 4 }}>
                                  <ImageAdjustmentSlider
                                    label="Gamma"
                                    tooltip="Midtone correction. Helps sources that look flat, too dark, or too bright without moving black/white as much."
                                    value={numberWorkerValue(videoDraft, 'gamma', 100)}
                                    min={70}
                                    max={130}
                                    valueLabel={(value) => `${value}%`}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'gamma', String(value))}
                                  />
                                </Grid>
                                <Grid size={{ xs: 12, md: 4 }}>
                                  <ImageAdjustmentSlider
                                    label="Temperature"
                                    tooltip="Warmer/cooler correction. Positive warms, negative cools."
                                    value={numberWorkerValue(videoDraft, 'temperature', 0)}
                                    min={-100}
                                    max={100}
                                    valueLabel={(value) => (value === 0 ? 'Neutral' : `${value > 0 ? '+' : ''}${value}%`)}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'temperature', String(value))}
                                  />
                                </Grid>
                                <Grid size={{ xs: 12, md: 4 }}>
                                  <ImageAdjustmentSlider
                                    label="Tint"
                                    tooltip="Green/magenta correction. Useful for color casts after old transfers."
                                    value={numberWorkerValue(videoDraft, 'tint', 0)}
                                    min={-100}
                                    max={100}
                                    valueLabel={(value) => (value === 0 ? 'Neutral' : `${value > 0 ? '+' : ''}${value}%`)}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'tint', String(value))}
                                  />
                                </Grid>
                              </Grid>
                            </Stack>
                          </Box>
                        </Grid>
                      </Grid>
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <Button
                        variant="text"
                        onClick={() => setVideoAdvancedOpen((current) => !current)}
                        endIcon={
                          <ExpandMoreIcon
                            sx={{
                              transform: videoAdvancedOpen ? 'rotate(180deg)' : 'rotate(0deg)',
                              transition: (theme) => theme.transitions.create('transform', { duration: theme.transitions.duration.shortest }),
                            }}
                          />
                        }
                        sx={{ px: 0 }}
                      >
                        Advanced
                      </Button>
                      <Collapse in={videoAdvancedOpen} timeout="auto" unmountOnExit>
                        <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, mt: 1, bgcolor: 'rgba(255,255,255,0.018)' }}>
                          <Stack spacing={2}>
                            <Stack spacing={0.4}>
                              <Typography fontWeight={700}>Technical encoder controls</Typography>
                              <Typography color="text.secondary" variant="body2">
                                These values are saved exactly into the profile and used by FFmpeg previews and workers.
                              </Typography>
                            </Stack>
                            <Divider />
                            <Grid container spacing={2}>
                              <Grid size={{ xs: 12, md: 6 }}>
                                <TextField
                                  label="Custom FFmpeg video filters"
                                  value={videoWorkerValue(videoDraft, 'videoFilters')}
                                  onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoFilters', event.target.value)}
                                  placeholder="bwdif=mode=send_frame,hqdn3d=1.5:1.5:6:6"
                                  helperText="Normally generated by Image cleanup. Edit only when you need custom FFmpeg filters."
                                  fullWidth
                                />
                              </Grid>
                              <Grid size={{ xs: 12, md: 6 }}>
                                <TextField
                                  label="Custom x265 params"
                                  value={videoWorkerValue(videoDraft, 'x265Params')}
                                  onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'x265Params', event.target.value)}
                                  placeholder="aq-mode=3:aq-strength=0.9:deblock=-1,-1"
                                  fullWidth
                                />
                              </Grid>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                <TextField label="AQ mode" value={x265ParamValue(videoDraft, 'aq-mode')} onChange={(event) => updateX265Param(setVideoDraft, videoDraft, 'aq-mode', event.target.value)} fullWidth />
                              </Grid>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                <TextField label="psy-rd" value={x265ParamValue(videoDraft, 'psy-rd')} onChange={(event) => updateX265Param(setVideoDraft, videoDraft, 'psy-rd', event.target.value)} fullWidth />
                              </Grid>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                <TextField label="deblock" value={x265ParamValue(videoDraft, 'deblock')} onChange={(event) => updateX265Param(setVideoDraft, videoDraft, 'deblock', event.target.value)} placeholder="-1,-1" fullWidth />
                              </Grid>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                <TextField label="cutree" value={x265ParamValue(videoDraft, 'cutree')} onChange={(event) => updateX265Param(setVideoDraft, videoDraft, 'cutree', event.target.value)} placeholder="1" fullWidth />
                              </Grid>
                            </Grid>
                          </Stack>
                        </Box>
                      </Collapse>
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <TextField label="Description" value={videoDraft.description} onChange={(event) => setVideoDraft({ ...videoDraft, description: event.target.value })} multiline minRows={2} fullWidth />
                    </Grid>
                  </Grid>
                  {videoPreviewStatus === 'ready' ? (
                    <Alert severity="success">Video sample ready in Sample B.</Alert>
                  ) : null}
                  {videoPreviewNonce > 0 &&
                  videoPreviewStatus !== 'loading' &&
                  (processedVideoCodec !== videoDraft.videoCodec || processedVideoQualityValue !== videoDraft.qualityValue || JSON.stringify(processedVideoOptions) !== JSON.stringify(videoPreviewOptions(videoDraft))) ? (
                    <Alert severity="info">Video settings changed. Process video again to refresh Sample B.</Alert>
                  ) : null}
                  {videoPreviewStatus === 'error' ? (
                    <Alert severity="warning">Video sample could not be processed.</Alert>
                  ) : null}
                  {createVideoProfile.isSuccess ? <Alert severity="success">Video profile saved.</Alert> : null}
                </Stack>
              </CardContent>
            </Card>

          ) : null}
          {labSection === 'audio' ? (
              <Card>
              <CardContent>
                <Stack spacing={2}>
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between">
                    <Stack direction="row" spacing={1} alignItems="center">
                      <GraphicEqIcon color="primary" />
                      <Typography variant="h3">Audio Profile Draft</Typography>
                    </Stack>
                    <Stack direction="row" spacing={1} justifyContent={{ xs: 'flex-start', sm: 'flex-end' }}>
                      <Button
                        startIcon={<PlayArrowIcon />}
                        variant="contained"
                        size="small"
                        disabled={!assetPath || previewNonce === 0 || audioPreviewStatus === 'loading'}
                        onClick={processAudioPreview}
                        sx={{ minHeight: 32 }}
                      >
                        {audioPreviewStatus === 'loading' ? 'Processing...' : 'Process Audio'}
                      </Button>
                      <Button
                        startIcon={<SaveIcon />}
                        variant="outlined"
                        size="small"
                        disabled={!audioDraft.name || updateSetting.isPending}
                        onClick={saveAudioProfile}
                        sx={{ minHeight: 32 }}
                      >
                        Save Profile
                      </Button>
                    </Stack>
                  </Stack>
                  <Alert severity="info">
                    <strong>copy</strong> cannot apply loudness, EQ, denoise, or filters. For restored audio, keep the original track and output the
                    enhanced copy as AAC, Opus, AC3, or FLAC.
                  </Alert>
                  <Box
                    sx={{
                      display: 'grid',
                      gridTemplateColumns: {
                        xs: 'minmax(0, 1fr)',
                        lg: 'minmax(0, 5fr) minmax(0, 7fr)',
                        xl: 'minmax(360px, 1fr) 760px',
                      },
                      gap: 2,
                      alignItems: 'start',
                    }}
                  >
                    <Box sx={{ minWidth: 0 }}>
                      <Grid container spacing={1.5}>
                    <Grid size={{ xs: 12, sm: 6 }}>
                      <TextField
                        label="Starter preset"
                        value={selectedAudioStarterPreset}
                        onChange={(event) => applyStarterAudioProfile(event.target.value)}
                        size="small"
                        select
                        fullWidth
                      >
                        <MenuItem value="" disabled>
                          Choose a starter preset
                        </MenuItem>
                        {starterAudioProfiles.map((profile) => (
                          <MenuItem key={profile.key} value={profile.key}>
                            {profile.name} - {profile.intent}
                          </MenuItem>
                        ))}
                      </TextField>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 6 }}>
                      <AudioProfileAutocomplete profiles={audioProfiles} onChange={(profile) => profile ? selectAudioProfile(profile.key) : undefined} />
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <TextField label="New audio profile name" value={audioDraft.name} onChange={(event) => setAudioDraft({ ...audioDraft, name: event.target.value, key: slugify(event.target.value) })} size="small" fullWidth />
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <TextField
                        label="Audio track for A/B preview"
                        value={selectedAudioStreamIndex ?? ''}
                        onChange={(event) => {
                          const streamIndex = Number(event.target.value);
                          setAudioPreviewStreamIndex(streamIndex);
                          resetProcessedAudioPreview();
                          if (selectedAsset) {
                            updateAudioSourceStream.mutate({
                              path: selectedAsset.path,
                              ...selectedAsset.conversion,
                              enhancedAudioSourceStreamIndex: streamIndex,
                            });
                          }
                        }}
                        disabled={!assetPath || trackSnapshot.isPending || updateAudioSourceStream.isPending || availableAudioStreams.length === 0}
                        helperText={trackSnapshot.isPending
                          ? 'Scanning audio tracks…'
                          : availableAudioStreams.length === 0
                            ? 'No audio tracks are available for this asset.'
                            : updateAudioSourceStream.isError
                              ? 'Could not save the selected source track for final conversion.'
                              : 'Used by Sample A, Sample B, and the final enhanced-audio conversion for this asset.'}
                        size="small"
                        select
                        fullWidth
                      >
                        {availableAudioStreams.map((stream) => (
                          <MenuItem key={stream.index} value={stream.index}>
                            #{stream.index} · {stream.language || 'und'} · {stream.title || 'Untitled'} · {stream.codec || 'unknown'}
                            {stream.channels ? ` · ${stream.channels} ch` : ''}{stream.default ? ' · Default' : ''}
                          </MenuItem>
                        ))}
                      </TextField>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 6 }}>
                      <TextField label="Output codec" value={audioDraft.outputCodec} onChange={(event) => setAudioDraft({ ...audioDraft, outputCodec: event.target.value })} size="small" select fullWidth>
                        {['aac', 'copy', 'flac', 'opus', 'ac3'].map((codec) => (
                          <MenuItem key={codec} value={codec}>
                            {codec}
                          </MenuItem>
                        ))}
                      </TextField>
                    </Grid>
                    <Grid size={{ xs: 12, sm: 6 }}>
                      <TextField
                        label="Channel mode"
                        value={audioDraft.channelMode}
                        onChange={(event) =>
                          setAudioDraft({
                            ...audioDraft,
                            channelMode: event.target.value as AudioEnhancementProfile['channelMode'],
                          })
                        }
                        size="small"
                        select
                        fullWidth
                      >
                        {channelModes.map((mode) => (
                          <MenuItem key={mode.value} value={mode.value}>
                            {mode.label}
                          </MenuItem>
                        ))}
                      </TextField>
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <Alert severity={audioDraft.channelMode === 'light-stereo' ? 'warning' : 'info'}>
                        {channelModes.find((mode) => mode.value === audioDraft.channelMode)?.description}
                      </Alert>
                    </Grid>
                    {audioDraft.channelMode === 'force-stereo' ? (
                      <Grid size={{ xs: 12 }}>
                        <TextField
                            label="Stereo method"
                            value={audioDraft.forceStereoMode}
                            size="small"
                            onChange={(event) =>
                              setAudioDraft({
                                ...audioDraft,
                                forceStereoMode: event.target.value as AudioEnhancementProfile['forceStereoMode'],
                              })
                            }
                            select
                            fullWidth
                        >
                            {forceStereoModes.map((mode) => (
                              <MenuItem key={mode.value} value={mode.value}>
                                {mode.label}
                              </MenuItem>
                            ))}
                        </TextField>
                      </Grid>
                    ) : null}
                    {audioDraft.channelMode === 'light-stereo' ? (
                      <>
                            <Grid size={{ xs: 12, sm: 6 }}>
                              <TextField
                                label="Stereo delay ms"
                                value={audioDraft.stereoDelayMs}
                                type="number"
                                onChange={(event) => setAudioDraft({ ...audioDraft, stereoDelayMs: Number(event.target.value) })}
                                inputProps={{ min: 1, max: 40 }}
                                helperText="Small values are safer. Try 8-16 ms first."
                                size="small"
                                fullWidth
                              />
                            </Grid>
                            <Grid size={{ xs: 12, sm: 6 }}>
                              <TextField
                                label="Stereo width"
                                value={audioDraft.stereoWidth}
                                type="number"
                                onChange={(event) => setAudioDraft({ ...audioDraft, stereoWidth: Number(event.target.value) })}
                                inputProps={{ min: 0, max: 100 }}
                                helperText="Higher values widen more, but can sound phasey."
                                size="small"
                                fullWidth
                              />
                            </Grid>
                      </>
                    ) : null}
                    <Grid size={{ xs: 12, sm: 6 }}>
                      <TextField label="Target loudness LUFS" value={audioDraft.targetLoudness} type="number" onChange={(event) => setAudioDraft({ ...audioDraft, targetLoudness: Number(event.target.value) })} size="small" fullWidth />
                    </Grid>
                    <Grid size={{ xs: 12, sm: 6 }}>
                      <TextField label="True peak dB" value={audioDraft.truePeak} type="number" onChange={(event) => setAudioDraft({ ...audioDraft, truePeak: Number(event.target.value) })} size="small" fullWidth />
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <Stack direction="row" spacing={1} alignItems="center" sx={{ minHeight: 40 }}>
                        <Checkbox
                          checked={audioDraft.preserveOriginalTrack}
                          onChange={(event) => setAudioDraft({ ...audioDraft, preserveOriginalTrack: event.target.checked })}
                        />
                        <Typography>Preserve original audio track</Typography>
                      </Stack>
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <TextField
                        label="FFmpeg audio filter chain"
                        value={audioFilterChain}
                        onChange={(event) => {
                          setAudioFilterChainEdited(true);
                          setAudioFilterChain(event.target.value);
                        }}
                        onBlur={() => {
                          if (!audioFilterChain.trim()) {
                            setAudioFilterChainEdited(false);
                          }
                        }}
                        multiline
                        minRows={5}
                        helperText="Auto-generated from the controls until edited. Process Audio uses this exact chain for Sample B."
                        size="small"
                        fullWidth
                      />
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <TextField label="Notes" value={audioDraft.notes} onChange={(event) => setAudioDraft({ ...audioDraft, notes: event.target.value })} multiline minRows={2} size="small" fullWidth />
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                        <Chip label={audioFilterChainEdited ? 'Edited chain' : 'Auto chain'} size="small" />
                        <Chip label={`Preview: ${previewAudioFilters || 'anull'}`} size="small" />
                        <Chip label={audioDraft.preserveOriginalTrack ? 'Preserve original' : 'Replace audio'} size="small" />
                      </Stack>
                    </Grid>
                      </Grid>
                    </Box>
                    <Box sx={{ minWidth: 0, position: { lg: 'sticky' }, top: { lg: 88 }, alignSelf: 'flex-start' }}>
                      <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2 }}>
                        <Stack spacing={2}>
                          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                            <Stack>
                              <Typography fontWeight={700}>Graphic EQ</Typography>
                              <Typography color="text.secondary" variant="body2">
                                Tune this draft before generating the A/B audio preview.
                              </Typography>
                            </Stack>
                            <Button
                              variant="outlined"
                              onClick={() => setAudioDraft((current) => ({ ...current, eqBands: defaultEqBands() }))}
                            >
                              Reset EQ
                            </Button>
                          </Stack>
                          <Box sx={{ overflowX: 'auto', pb: 1 }}>
                            <Stack direction="row" spacing={1.25} sx={{ minWidth: 720 }}>
                              {eqFrequencies.map((frequency) => {
                                const key = String(frequency);
                                const value = audioDraft.eqBands?.[key] ?? 0;
                                return (
                                  <Stack
                                    key={key}
                                    spacing={1}
                                    alignItems="center"
                                    sx={{
                                      width: 72,
                                      border: 1,
                                      borderColor: 'divider',
                                      borderRadius: 1,
                                      p: 1,
                                      bgcolor: 'rgba(255,255,255,0.02)',
                                      flexShrink: 0,
                                    }}
                                  >
                                    <Chip label={eqBandLabel(frequency)} size="small" />
                                    <Typography fontWeight={700} variant="body2" noWrap>
                                      {formatFrequency(frequency)}
                                    </Typography>
                                    <Typography color={value === 0 ? 'text.secondary' : 'primary.main'} fontWeight={700} variant="caption">
                                      {value > 0 ? '+' : ''}{value} dB
                                    </Typography>
                                    <Stack direction="row" spacing={0.5} alignItems="center" sx={{ height: 190 }}>
                                      <Stack justifyContent="space-between" sx={{ height: '100%' }}>
                                        <Typography color="text.secondary" variant="caption">+12</Typography>
                                        <Typography color="text.secondary" variant="caption">0</Typography>
                                        <Typography color="text.secondary" variant="caption">-12</Typography>
                                      </Stack>
                                      <Slider
                                        value={value}
                                        min={-12}
                                        max={12}
                                        step={0.5}
                                        orientation="vertical"
                                        sx={{ height: 170 }}
                                        onChange={(_, nextValue) =>
                                          setAudioDraft((current) => ({
                                            ...current,
                                            eqBands: {
                                              ...(current.eqBands ?? defaultEqBands()),
                                              [key]: Array.isArray(nextValue) ? nextValue[0] : nextValue,
                                            },
                                          }))
                                        }
                                      />
                                    </Stack>
                                  </Stack>
                                );
                              })}
                            </Stack>
                          </Box>
                          {audioPreviewStatus === 'ready' ? (
                            <Alert severity="success">Audio sample ready in Sample B.</Alert>
                          ) : null}
                          {audioPreviewNonce > 0 && audioPreviewStatus !== 'loading' && processedAudioFilters !== previewAudioFilters ? (
                            <Alert severity="info">Audio settings changed. Process audio again to refresh Sample B.</Alert>
                          ) : null}
                          {audioPreviewNonce > 0 && audioPreviewStatus !== 'loading' && processedAudioChannelMode !== audioDraft.channelMode ? (
                            <Alert severity="info">Channel mode changed. Process audio again to refresh Sample B.</Alert>
                          ) : null}
                          {audioPreviewStatus === 'error' ? (
                            <Alert severity="warning">Audio sample could not be processed.</Alert>
                          ) : null}
                        </Stack>
                      </Box>
                    </Box>
                  </Box>
                  {updateSetting.isSuccess ? <Alert severity="success">Audio profile saved.</Alert> : null}
                </Stack>
              </CardContent>
            </Card>
          ) : null}
          {labSection === 'tracks' ? (
            <Card>
              <CardContent>
                <Stack spacing={2}>
                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between">
                    <Stack direction="row" spacing={1} alignItems="center">
                      <FactCheckIcon color="primary" />
                      <Typography variant="h3">Track Profile Draft</Typography>
                    </Stack>
                    <Stack direction="row" spacing={1} justifyContent={{ xs: 'flex-start', sm: 'flex-end' }}>
                      <Button size="small" variant="outlined" disabled={!assetPath || trackSnapshot.isPending} onClick={() => scanTrackAsset(false)} sx={{ minHeight: 32 }}>
                        Scan
                      </Button>
                      <Button size="small" variant="outlined" disabled={!assetPath || trackSnapshot.isPending} onClick={() => scanTrackAsset(true)} sx={{ minHeight: 32 }}>
                        Rescan
                      </Button>
                      <Button
                        startIcon={<SaveIcon />}
                        variant="contained"
                        size="small"
                        disabled={!trackDraft.name || updateSetting.isPending}
                        onClick={saveTrackProfile}
                        sx={{ minHeight: 32 }}
                      >
                        Save Profile
                      </Button>
                    </Stack>
                  </Stack>
                  <Alert severity="info">
                    Start from a real asset snapshot, choose the tracks to keep, edit audio/subtitle metadata, then save those choices as a reusable track profile.
                  </Alert>
                  <Grid container spacing={2}>
                    <Grid size={{ xs: 12, md: 4 }}>
                      <TrackProfileAutocomplete
                        profiles={trackProfiles}
                        onChange={selectTrackProfile}
                      />
                    </Grid>
                    <Grid size={{ xs: 12, md: 4 }}>
                      <TextField
                        label="New track profile name"
                        value={trackDraft.name}
                        onChange={(event) => setTrackDraft({ ...trackDraft, name: event.target.value, key: slugify(event.target.value) })}
                        fullWidth
                      />
                    </Grid>
                    <Grid size={{ xs: 12, md: 4 }}>
                      <TextField
                        label="Validation"
                        value={trackDraft.validationMode}
                        onChange={(event) => setTrackDraft({ ...trackDraft, validationMode: event.target.value as TrackProfile['validationMode'] })}
                        select
                        fullWidth
                      >
                        <MenuItem value="review">Mark asset for review</MenuItem>
                        <MenuItem value="block">Block queue</MenuItem>
                        <MenuItem value="warn">Warn but allow</MenuItem>
                      </TextField>
                    </Grid>

                    <Grid size={{ xs: 12 }}>
                      <Stack spacing={1.5}>
                        <Stack sx={{ minWidth: 0 }}>
                          <Typography fontWeight={700}>Current asset tracks</Typography>
                          <Typography color="text.secondary" variant="body2" sx={{ wordBreak: 'break-all' }}>
                            {selectedAsset?.relativePath || selectedAsset?.path || 'Select an asset in the Lab header to inspect its tracks.'}
                          </Typography>
                        </Stack>
                        {!assetPath ? <Alert severity="warning">Choose an asset first so the Lab can show real video, audio, and subtitle tracks.</Alert> : null}
                        {trackSnapshot.isPending ? <Alert severity="info">Reading track snapshot...</Alert> : null}
                        {trackSnapshot.isError ? <Alert severity="warning">Could not scan this asset. It may not be readable from the backend.</Alert> : null}
                        {trackSnapshot.data ? (
                          <MediaSnapshotDetails
                            scan={trackSnapshot.data}
                            streamControls={{
                              video: {
                                selected: conversionStreamIndexes(trackConversionDraft, trackSnapshot.data, 'video'),
                                disabled: updateSetting.isPending,
                                onToggle: (index, keep) => toggleTrackStream('video', index, keep),
                              },
                              audio: {
                                selected: conversionStreamIndexes(trackConversionDraft, trackSnapshot.data, 'audio'),
                                disabled: updateSetting.isPending,
                                onToggle: (index, keep) => toggleTrackStream('audio', index, keep),
                              },
                              subtitle: {
                                selected: conversionStreamIndexes(trackConversionDraft, trackSnapshot.data, 'subtitle'),
                                disabled: updateSetting.isPending,
                                onToggle: (index, keep) => toggleTrackStream('subtitle', index, keep),
                              },
                            }}
                            metadataControls={{
                              video: {
                                values: trackConversionDraft.videoMetadata ?? {},
                                disabled: updateSetting.isPending,
                                onChange: (index, patch) => updateTrackMetadata('video', index, patch),
                              },
                              audio: {
                                values: trackConversionDraft.audioMetadata ?? {},
                                disabled: updateSetting.isPending,
                                onChange: (index, patch) => updateTrackMetadata('audio', index, patch),
                              },
                              subtitle: {
                                values: trackConversionDraft.subtitleMetadata ?? {},
                                disabled: updateSetting.isPending,
                                onChange: (index, patch) => updateTrackMetadata('subtitle', index, patch),
                              },
                            }}
                          />
                        ) : null}
                      </Stack>
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <TextField
                        label="Description"
                        value={trackDraft.description}
                        onChange={(event) => setTrackDraft({ ...trackDraft, description: event.target.value })}
                        multiline
                        minRows={2}
                        fullWidth
                      />
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <TextField
                        label="Notes"
                        value={trackDraft.notes}
                        onChange={(event) => setTrackDraft({ ...trackDraft, notes: event.target.value })}
                        multiline
                        minRows={2}
                        fullWidth
                      />
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                        <Chip label={`Video kept: ${trackSelectionLabel(trackConversionDraft, trackSnapshot.data, 'video')}`} size="small" />
                        <Chip label={`Audio kept: ${trackSelectionLabel(trackConversionDraft, trackSnapshot.data, 'audio')}`} size="small" />
                        <Chip label={`Subs kept: ${trackSelectionLabel(trackConversionDraft, trackSnapshot.data, 'subtitle')}`} size="small" />
                        <Chip label={`Validation: ${trackValidationLabel(trackDraft.validationMode)}`} size="small" />
                      </Stack>
                    </Grid>
                  </Grid>
                  {updateSetting.isSuccess ? <Alert severity="success">Track profile saved.</Alert> : null}
                </Stack>
              </CardContent>
            </Card>
          ) : null}
        </Stack>
      </Box>
    </>
  );
}

function SampleCard({ title, subtitle, children }: { title: string; subtitle: string; children: ReactNode }) {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent>
        <Stack spacing={2}>
          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
            <Typography variant="h3">{title}</Typography>
            <Typography color="text.secondary" variant="body2">
              {subtitle}
            </Typography>
          </Stack>
          {children}
        </Stack>
      </CardContent>
    </Card>
  );
}

function PreserveButton({
  active,
  label,
  tooltip,
  onClick,
}: {
  active: boolean;
  label: string;
  tooltip: string;
  onClick: () => void;
}) {
  return (
    <Tooltip title={tooltip}>
      <Button
        type="button"
        size="small"
        variant={active ? 'contained' : 'outlined'}
        onClick={onClick}
        sx={{ minHeight: 32, borderRadius: 1, px: 1.25, whiteSpace: 'nowrap' }}
      >
        {active ? 'Preserve ' : 'Remove '}{label}
      </Button>
    </Tooltip>
  );
}

function ImageAdjustmentSlider({
  label,
  tooltip,
  value,
  min,
  max,
  valueLabel,
  onChange,
}: {
  label: string;
  tooltip: string;
  value: number;
  min: number;
  max: number;
  valueLabel: (value: number) => string;
  onChange: (value: number) => void;
}) {
  return (
    <Tooltip title={tooltip}>
      <Stack spacing={0.5}>
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          <Typography fontWeight={700}>{label}</Typography>
          <Chip label={valueLabel(value)} size="small" />
        </Stack>
        <Slider
          value={value}
          min={min}
          max={max}
          step={5}
          size="small"
          onChange={(_, nextValue) => onChange(Array.isArray(nextValue) ? nextValue[0] : nextValue)}
        />
      </Stack>
    </Tooltip>
  );
}

function VideoPreview({
  label,
  src,
  direct = false,
  onStatusChange,
}: {
  label: string;
  src: string;
  direct?: boolean;
  onStatusChange?: (status: 'idle' | 'loading' | 'ready' | 'error') => void;
}) {
  const [videoSrc, setVideoSrc] = useState('');
  const [metrics, setMetrics] = useState<{ cache: string; mode: string; generationMs: number; bytes: number } | null>(null);

  useEffect(() => {
    const controller = new AbortController();
    let objectUrl = '';
    setVideoSrc('');
    setMetrics(null);
    onStatusChange?.('loading');
    if (direct) {
      setVideoSrc(src);
      return () => controller.abort();
    }
    fetch(src, { signal: controller.signal })
      .then(async (response) => {
        if (!response.ok) throw new Error(`Preview failed with ${response.status}`);
        const blob = await response.blob();
        objectUrl = URL.createObjectURL(blob);
        setMetrics({
          cache: response.headers.get('X-MVForge-Preview-Cache') ?? 'unknown',
          mode: response.headers.get('X-MVForge-Preview-Mode') ?? 'unknown',
          generationMs: Number(response.headers.get('X-MVForge-Preview-Generation-Ms') ?? 0),
          bytes: blob.size,
        });
        setVideoSrc(objectUrl);
      })
      .catch((error) => {
        if (error instanceof DOMException && error.name === 'AbortError') return;
        onStatusChange?.('error');
      });
    return () => {
      controller.abort();
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [direct, src]);

  return (
    <Stack spacing={1}>
      <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap" useFlexGap>
        <Typography fontWeight={700}>{label}</Typography>
        {direct ? <Chip size="small" label="Original stream · no conversion" color="success" /> : null}
        {metrics ? <Chip size="small" label={`${metrics.mode} · ${metrics.cache}`} /> : null}
        {metrics ? <Chip size="small" label={`${(metrics.generationMs / 1000).toFixed(2)}s · ${formatPreviewBytes(metrics.bytes)}`} /> : null}
      </Stack>
      <Box
        component="video"
        controls
        src={videoSrc}
        onCanPlay={() => onStatusChange?.('ready')}
        onError={() => onStatusChange?.('error')}
        sx={{ width: '100%', maxHeight: 420, aspectRatio: '16 / 9', bgcolor: 'black', borderRadius: 1 }}
      />
    </Stack>
  );
}

function originalPreviewUrl(path: string, start: string, seconds: number) {
  const startSeconds = previewStartSeconds(start);
  const endSeconds = startSeconds + Math.max(1, seconds);
  return `${api.assetPreviewUrl(path)}#t=${startSeconds},${endSeconds}`;
}

function previewStartSeconds(value: string) {
  if (/^\d+$/.test(value.trim())) return Number(value);
  const parts = value.split(':').map((part) => Number(part));
  if (parts.length !== 3 || parts.some((part) => !Number.isFinite(part))) return 0;
  return Math.max(0, parts[0] * 3600 + parts[1] * 60 + parts[2]);
}

function formatPreviewBytes(bytes: number) {
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
}

function AudioPreview({
  label,
  src,
  onStatusChange,
}: {
  label: string;
  src: string;
  onStatusChange?: (status: 'idle' | 'loading' | 'ready' | 'error') => void;
}) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [status, setStatus] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
  const [waveformError, setWaveformError] = useState('');
  const [audioSrc, setAudioSrc] = useState('');

  useEffect(() => {
    let canceled = false;
    let objectUrl = '';
    const canvas = canvasRef.current;
    const context = canvas?.getContext('2d');
    if (!canvas || !context || !src) {
      return;
    }
    const waveformCanvas = canvas;
    const waveformContext = context;

    setStatus('loading');
    setWaveformError('');
    onStatusChange?.('loading');
    setAudioSrc('');
    clearWaveform(waveformCanvas, waveformContext);

    async function draw() {
      try {
        const response = await fetch(src);
        if (!response.ok) {
          const errorText = await response.text();
          throw new Error(errorText || `Preview failed with ${response.status}`);
        }
        const blob = await response.blob();
        objectUrl = URL.createObjectURL(blob);
        setAudioSrc(objectUrl);
        setStatus('ready');
        onStatusChange?.('ready');

        const data = await blob.arrayBuffer();
        const AudioContextClass =
          window.AudioContext ||
          (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
        if (!AudioContextClass) {
          throw new Error('AudioContext is unavailable');
        }
        const audioContext = new AudioContextClass();
        const buffer = await audioContext.decodeAudioData(data.slice(0));
        await audioContext.close();
        if (canceled) {
          return;
        }
        drawWaveform(waveformCanvas, waveformContext, buffer);
      } catch (error) {
        if (!canceled) {
          if (objectUrl) {
            setWaveformError('Waveform unavailable, but the audio preview can still be played.');
            drawWaveformError(waveformCanvas, waveformContext);
            return;
          }
          const message = error instanceof Error ? error.message : 'Audio preview unavailable.';
          setWaveformError(message);
          setStatus('error');
          onStatusChange?.('error');
          drawWaveformError(waveformCanvas, waveformContext, message);
        }
      }
    }

    void draw();
    return () => {
      canceled = true;
      if (objectUrl) {
        URL.revokeObjectURL(objectUrl);
      }
    };
  }, [onStatusChange, src]);

  return (
    <Stack spacing={1}>
      <Typography fontWeight={700}>{label}</Typography>
      <Box
        component="canvas"
        ref={canvasRef}
        height={132}
        sx={{
          width: '100%',
          height: 132,
          border: 1,
          borderColor: 'divider',
          borderRadius: 1,
          bgcolor: 'rgba(255,255,255,0.03)',
        }}
      />
      {status === 'error' ? (
        <Typography color="warning.main" variant="body2">
          {waveformError || 'Audio preview unavailable.'}
        </Typography>
      ) : waveformError ? (
        <Typography color="warning.main" variant="body2">
          {waveformError}
        </Typography>
      ) : null}
      <Box component="audio" controls src={audioSrc} sx={{ width: '100%' }} />
    </Stack>
  );
}

function clearWaveform(canvas: HTMLCanvasElement, context: CanvasRenderingContext2D) {
  const width = canvas.clientWidth || 480;
  const height = canvas.clientHeight || 132;
  const ratio = window.devicePixelRatio || 1;
  canvas.width = width * ratio;
  canvas.height = height * ratio;
  context.setTransform(ratio, 0, 0, ratio, 0, 0);
  context.clearRect(0, 0, width, height);
  context.fillStyle = 'rgba(255,255,255,0.03)';
  context.fillRect(0, 0, width, height);
}

function drawWaveform(canvas: HTMLCanvasElement, context: CanvasRenderingContext2D, buffer: AudioBuffer) {
  clearWaveform(canvas, context);
  const width = canvas.clientWidth || 480;
  const height = canvas.clientHeight || 132;
  const channels = Math.min(buffer.numberOfChannels, 2);
  const laneHeight = height / channels;

  for (let channel = 0; channel < channels; channel += 1) {
    drawWaveformChannel(context, buffer.getChannelData(channel), channel, laneHeight, width);
  }
}

function drawWaveformChannel(
  context: CanvasRenderingContext2D,
  data: Float32Array,
  channel: number,
  laneHeight: number,
  width: number,
) {
  const step = Math.max(1, Math.floor(data.length / width));
  const laneTop = channel * laneHeight;
  const center = laneTop + laneHeight / 2;
  const amplitude = laneHeight * 0.38;
  const label = channel === 0 ? 'L' : 'R';

  context.strokeStyle = channel === 0 ? '#4fb3ff' : '#75d36b';
  context.lineWidth = 1;
  context.beginPath();
  for (let x = 0; x < width; x += 1) {
    let min = 1;
    let max = -1;
    const start = x * step;
    for (let index = 0; index < step && start + index < data.length; index += 1) {
      const value = data[start + index];
      min = Math.min(min, value);
      max = Math.max(max, value);
    }
    context.moveTo(x, center + min * amplitude);
    context.lineTo(x, center + max * amplitude);
  }
  context.stroke();

  context.strokeStyle = 'rgba(255,255,255,0.18)';
  context.beginPath();
  context.moveTo(0, center);
  context.lineTo(width, center);
  context.stroke();

  context.fillStyle = 'rgba(255,255,255,0.72)';
  context.font = '12px sans-serif';
  context.fillText(label, 10, laneTop + 18);
}

function drawWaveformError(canvas: HTMLCanvasElement, context: CanvasRenderingContext2D, message = 'Waveform unavailable') {
  clearWaveform(canvas, context);
  const height = canvas.clientHeight || 132;
  context.fillStyle = 'rgba(246,180,75,0.85)';
  context.font = '14px sans-serif';
  context.fillText(message.slice(0, 88), 16, height / 2);
}

function videoPreviewOptions(draft: ProfileInput) {
  return {
    videoEncoder: videoWorkerValue(draft, 'videoEncoder', 'auto'),
    useHardwareIfAvailable: videoWorkerBool(draft, 'useHardwareIfAvailable'),
    globalQuality: numberWorkerValue(draft, 'globalQuality', draft.qualityValue || 25),
    videoPreset: videoWorkerValue(draft, 'videoPreset', 'medium'),
    pixFmt: videoWorkerValue(draft, 'pixFmt', isTenBitDraft(draft) ? 'yuv420p10le' : 'yuv420p'),
    videoFilters: videoWorkerValue(draft, 'videoFilters'),
    x265Params: videoWorkerValue(draft, 'x265Params'),
    addAacStereoTrack: videoAACTrackEnabled(draft),
    aacStereoBitrateKbps: numberWorkerValue(draft, 'aacStereoBitrateKbps', 192),
    aacStereoDefault: videoAACTrackDefault(draft),
    preserveOriginalAudio: videoWorkerBool(draft, 'preserveOriginalAudio', true),
    subtitleOutputFormat: videoSubtitleOutputFormat(draft),
    preferSrtSubtitles: videoWorkerBool(draft, 'preferSrtSubtitles'),
    warnSubtitleFormats: videoWorkerBool(draft, 'warnSubtitleFormats', true),
  };
}

function isTenBitDraft(draft: ProfileInput) {
  return draft.videoCodec.includes('10bit') || videoWorkerValue(draft, 'pixFmt').includes('10');
}

function videoWorkerValue(draft: ProfileInput, key: string, fallback = '') {
  const value = draft.workerConfig?.[key];
  return typeof value === 'string' ? value : fallback;
}

function videoWorkerBool(draft: ProfileInput, key: string, fallback = false) {
  const value = draft.workerConfig?.[key];
  if (typeof value === 'boolean') {
    return value;
  }
  if (typeof value === 'string') {
    return ['true', '1', 'yes', 'enabled', 'on'].includes(value.toLowerCase());
  }
  return fallback;
}

function videoSubtitleOutputFormat(draft: ProfileInput) {
  const configured = videoWorkerValue(draft, 'subtitleOutputFormat').toLowerCase();
  if (configured === 'srt' || configured === 'ass' || configured === 'source') {
    return configured;
  }
  return videoWorkerBool(draft, 'preferSrtSubtitles') ? 'srt' : 'source';
}

function videoAACTrackEnabled(draft: ProfileInput) {
  return draft.workerConfig && 'addAacStereoTrack' in draft.workerConfig
    ? videoWorkerBool(draft, 'addAacStereoTrack')
    : videoWorkerBool(draft, 'addAacStereoDefault');
}

function videoAACTrackDefault(draft: ProfileInput) {
  return videoWorkerBool(draft, 'aacStereoDefault');
}

function numberWorkerValue(draft: ProfileInput, key: string, fallback: number) {
  const value = draft.workerConfig?.[key];
  const parsed = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : NaN;
  return Number.isFinite(parsed) ? parsed : fallback;
}

function updateVideoWorkerConfig(
  setVideoDraft: Dispatch<SetStateAction<ProfileInput>>,
  key: string,
  value: unknown,
) {
  setVideoDraft((current) => ({
    ...current,
    ...(key === 'pixFmt' ? { videoCodec: displayVideoCodec(current.videoCodec) } : {}),
    workerConfig: {
      ...current.workerConfig,
      [key]: value,
    },
  }));
}

function encoderPresetDescription(value: string) {
  return encoderPresetOptions.find((option) => option.value === value)?.description ?? 'Controls how much time FFmpeg spends compressing video.';
}

function pixelFormatDescription(value: string) {
  return pixelFormatOptions.find((option) => option.value === value)?.description ?? 'Controls output color depth and playback compatibility.';
}

function videoEncoderDescription(value: string) {
  return videoEncoderOptions.find((option) => option.value === value)?.description ?? 'Controls whether the worker uses hardware HEVC or software x265.';
}

function displayVideoCodec(value: string) {
  return value.toLowerCase().includes('x265') || value.toLowerCase().includes('hevc') || value.toLowerCase().includes('h265') ? 'x265' : value;
}

function isHardwareEncoderOption(value: string) {
  return ['hevc_qsv', 'hevc_nvenc', 'hevc_videotoolbox', 'hevc_amf'].includes(value);
}

function hardwareQualityHelper(softwareCRF: number) {
  const suggested = Math.min(35, Math.max(15, softwareCRF + 5));
  return `Approximate starting point: software CRF ${softwareCRF} ≈ hardware quality ${suggested}. Encoder-dependent; lower means higher quality.`;
}

function videoFilterControlValue(draft: ProfileInput, key: string, fallback = '') {
  return videoWorkerValue(draft, key, fallback);
}

function updateVideoFilterControl(
  setVideoDraft: Dispatch<SetStateAction<ProfileInput>>,
  key: string,
  value: string,
) {
  setVideoDraft((current) => {
    const workerConfig = {
      ...current.workerConfig,
      [key]: value,
    };
    return {
      ...current,
      workerConfig: {
        ...workerConfig,
        videoFilters: buildVideoFilterChain(workerConfig),
      },
    };
  });
}

function buildVideoFilterChain(workerConfig: Record<string, unknown>) {
  const filters: string[] = [];
  const deinterlaceMode = stringValue(workerConfig.deinterlaceMode, 'auto');
  const denoise = stringValue(workerConfig.denoise, 'off');
  const deband = stringValue(workerConfig.deband, 'off');
  const crop = stringValue(workerConfig.crop, 'off');
  const cropValue = stringValue(workerConfig.cropValue, '');
  const brightness = numericWorkerConfigValue(workerConfig, 'brightness', 0);
  const contrast = numericWorkerConfigValue(workerConfig, 'contrast', 100);
  const saturation = numericWorkerConfigValue(workerConfig, 'saturation', 100);
  const gamma = numericWorkerConfigValue(workerConfig, 'gamma', 100);
  const temperature = numericWorkerConfigValue(workerConfig, 'temperature', 0);
  const tint = numericWorkerConfigValue(workerConfig, 'tint', 0);

  if (deinterlaceMode === 'force') {
    filters.push('bwdif=mode=send_frame:parity=auto:deint=all');
  } else if (deinterlaceMode === 'ivtc_bff') {
    filters.push('fieldmatch=order=bff,decimate');
  }
  if (denoise === 'light') {
    filters.push('hqdn3d=1.5:1.5:6:6');
  } else if (denoise === 'medium') {
    filters.push('hqdn3d=2:2:7:7');
  } else if (denoise === 'strong') {
    filters.push('nlmeans=s=2:p=7:r=15');
  }
  if (deband === 'light') {
    filters.push('deband=1thr=0.018:2thr=0.018:3thr=0.018:4thr=0.018');
  } else if (deband === 'medium') {
    filters.push('deband=1thr=0.028:2thr=0.028:3thr=0.028:4thr=0.028');
  }
  if (crop === 'manual' && cropValue.trim()) {
    filters.push(`crop=${cropValue.trim()}`);
  }
  if (brightness !== 0 || contrast !== 100 || saturation !== 100 || gamma !== 100) {
    const eqParts = [
      `brightness=${trimGain((Math.max(-100, Math.min(100, brightness)) / 100) * 0.12)}`,
      `contrast=${trimGain(Math.max(50, Math.min(150, contrast)) / 100)}`,
      `saturation=${trimGain(Math.max(50, Math.min(150, saturation)) / 100)}`,
      `gamma=${trimGain(Math.max(70, Math.min(130, gamma)) / 100)}`,
    ];
    filters.push(`eq=${eqParts.join(':')}`);
  }
  if (temperature !== 0 || tint !== 0) {
    const warm = Math.max(-100, Math.min(100, temperature)) / 100;
    const magenta = Math.max(-100, Math.min(100, tint)) / 100;
    const red = trimGain(warm * 0.08);
    const blue = trimGain(-warm * 0.08);
    const green = trimGain(-magenta * 0.06);
    filters.push(`colorbalance=rs=${red}:bs=${blue}:gs=${green}:rm=${red}:bm=${blue}:gm=${green}`);
  }
  return filters.join(',');
}

function numericWorkerConfigValue(workerConfig: Record<string, unknown>, key: string, fallback: number) {
  const value = workerConfig[key];
  const parsed = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : NaN;
  return Number.isFinite(parsed) ? parsed : fallback;
}

function x265ParamValue(draft: ProfileInput, key: string) {
  const params = parseX265Params(videoWorkerValue(draft, 'x265Params'));
  return params[key] ?? '';
}

function updateX265Param(
  setVideoDraft: Dispatch<SetStateAction<ProfileInput>>,
  draft: ProfileInput,
  key: string,
  value: string,
) {
  const params = parseX265Params(videoWorkerValue(draft, 'x265Params'));
  const cleanValue = value.trim();
  if (cleanValue) {
    params[key] = cleanValue;
  } else {
    delete params[key];
  }
  updateVideoWorkerConfig(setVideoDraft, 'x265Params', serializeX265Params(params));
}

function parseX265Params(value: string) {
  return value
    .split(':')
    .map((part) => part.trim())
    .filter(Boolean)
    .reduce<Record<string, string>>((params, part) => {
      const separator = part.indexOf('=');
      if (separator === -1) {
        params[part] = '1';
        return params;
      }
      params[part.slice(0, separator)] = part.slice(separator + 1);
      return params;
    }, {});
}

function serializeX265Params(params: Record<string, string>) {
  return Object.entries(params)
    .filter(([, value]) => value.trim() !== '')
    .map(([key, value]) => `${key}=${value}`)
    .join(':');
}

function VideoProfileAutocomplete({ profiles, onChange }: { profiles: Profile[]; onChange: (profile: Profile | null) => void }) {
  return (
    <Autocomplete
      options={profiles}
      value={null}
      onChange={(_, profile) => onChange(profile)}
      getOptionLabel={(profile) => `${profile.name} · ${profile.videoCodec} CRF ${profile.qualityValue}`}
      isOptionEqualToValue={(option, selected) => option.id === selected.id}
      filterOptions={(options, state) => {
        const query = state.inputValue.trim().toLowerCase();
        if (!query) {
          return options.slice(0, 50);
        }
        return options
          .filter((profile) =>
            [profile.name, profile.description, profile.videoCodec, profile.audioCodec, profile.container].some((value) =>
              value.toLowerCase().includes(query),
            ),
          )
          .slice(0, 50);
      }}
      renderInput={(params) => <TextField {...params} label="Search video profile" />}
      renderOption={(props, profile) => (
        <Box component="li" {...props} key={profile.id}>
          <Stack sx={{ minWidth: 0 }}>
            <Typography fontWeight={700} noWrap>
              {profile.name}
            </Typography>
            <Typography color="text.secondary" variant="body2" noWrap>
              {profile.container.toUpperCase()} · {profile.videoCodec} · {profile.audioCodec} · CRF {profile.qualityValue}
            </Typography>
          </Stack>
        </Box>
      )}
      fullWidth
    />
  );
}

function AudioProfileAutocomplete({
  profiles,
  onChange,
}: {
  profiles: AudioEnhancementProfile[];
  onChange: (profile: AudioEnhancementProfile | null) => void;
}) {
  return (
    <Autocomplete
      options={profiles}
      value={null}
      onChange={(_, profile) => onChange(profile)}
      getOptionLabel={(profile) => `${profile.name} · ${profile.outputCodec}`}
      isOptionEqualToValue={(option, selected) => option.key === selected.key}
      filterOptions={(options, state) => {
        const query = state.inputValue.trim().toLowerCase();
        if (!query) {
          return options.slice(0, 50);
        }
        return options
          .filter((profile) =>
            [profile.name, profile.description, profile.intent, profile.outputCodec, profile.filters].some((value) =>
              value.toLowerCase().includes(query),
            ),
          )
          .slice(0, 50);
      }}
      renderInput={(params) => <TextField {...params} label="Search audio profile" size="small" />}
      renderOption={(props, profile) => (
        <Box component="li" {...props} key={profile.key}>
          <Stack sx={{ minWidth: 0 }}>
            <Typography fontWeight={700} noWrap>
              {profile.name}
            </Typography>
            <Typography color="text.secondary" variant="body2" noWrap>
              {profile.outputCodec} · {profile.intent || 'Audio enhancement'}
            </Typography>
          </Stack>
        </Box>
      )}
      fullWidth
    />
  );
}

function AssetAutocomplete({ assets, value, onChange }: { assets: Asset[]; value: Asset | null; onChange: (asset: Asset | null) => void }) {
  return (
    <Autocomplete
      options={assets}
      value={value}
      onChange={(_, asset) => onChange(asset)}
      getOptionLabel={(asset) => asset.relativePath || asset.fileName}
      isOptionEqualToValue={(option, selected) => option.path === selected.path}
      filterOptions={(options, state) => {
        const query = state.inputValue.trim().toLowerCase();
        if (!query) {
          return options.slice(0, 50);
        }
        return options
          .filter((asset) =>
            [asset.fileName, asset.relativePath, asset.path].some((value) => value.toLowerCase().includes(query)),
          )
          .slice(0, 50);
      }}
      renderInput={(params) => <TextField {...params} label="Raw asset" size="small" />}
      renderOption={(props, asset) => (
        <Box component="li" {...props} key={asset.path}>
          <Stack sx={{ minWidth: 0 }}>
            <Typography fontWeight={700} noWrap>
              {asset.fileName}
            </Typography>
            <Typography color="text.secondary" variant="body2" noWrap>
              {asset.relativePath || asset.path}
            </Typography>
          </Stack>
        </Box>
      )}
      fullWidth
    />
  );
}

function TrackProfileAutocomplete({
  profiles,
  onChange,
}: {
  profiles: TrackProfile[];
  onChange: (profile: TrackProfile | null) => void;
}) {
  return (
    <Autocomplete
      options={profiles}
      value={null}
      onChange={(_, profile) => onChange(profile)}
      getOptionLabel={(profile) => `${profile.name} · ${profileTrackSummary(profile)}`}
      isOptionEqualToValue={(option, selected) => option.key === selected.key}
      renderInput={(params) => <TextField {...params} label="Start from track profile" />}
      fullWidth
    />
  );
}

function LanguageMultiSelect({
  label,
  value,
  onChange,
  disabled,
}: {
  label: string;
  value: string[];
  onChange: (value: string[]) => void;
  disabled?: boolean;
}) {
  return (
    <Autocomplete
      multiple
      options={languageOptions}
      value={languageOptions.filter((option) => value.includes(option.value))}
      onChange={(_, options) => onChange(options.map((option) => option.value))}
      getOptionLabel={(option) => option.label}
      disabled={disabled}
      renderInput={(params) => <TextField {...params} label={label} />}
      fullWidth
    />
  );
}

function LanguageSelect({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <TextField label={label} value={value} onChange={(event) => onChange(event.target.value)} select fullWidth>
      <MenuItem value="">Do not change</MenuItem>
      {languageOptions.map((option) => (
        <MenuItem key={option.value} value={option.value}>
          {option.label}
        </MenuItem>
      ))}
    </TextField>
  );
}

function getTrackProfiles(settings?: AppSetting[]) {
  const value = settings?.find((setting) => setting.key === 'trackProfiles')?.value.profiles;
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((profile) => normalizeTrackProfile(profile))
    .filter((profile): profile is TrackProfile => Boolean(profile))
    .filter((profile) => !profile.disabled && !profile.deletedAt);
}

function normalizeTrackProfile(value: unknown): TrackProfile | null {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const candidate = value as Record<string, unknown>;
  if (typeof candidate.key !== 'string' || typeof candidate.name !== 'string') {
    return null;
  }
  return {
    key: candidate.key,
    name: candidate.name,
    description: stringValue(candidate.description),
    sourceAssetPath: stringValue(candidate.sourceAssetPath),
    sourceAssetName: stringValue(candidate.sourceAssetName),
    keepVideoStreams: optionalNumberList(candidate.keepVideoStreams),
    keepAudioStreams: optionalNumberList(candidate.keepAudioStreams),
    keepSubtitleStreams: optionalNumberList(candidate.keepSubtitleStreams),
    videoMetadata: streamMetadataMapValue(candidate.videoMetadata),
    audioMetadata: streamMetadataMapValue(candidate.audioMetadata),
    subtitleMetadata: streamMetadataMapValue(candidate.subtitleMetadata),
    videoMode: trackVideoMode(candidate.videoMode),
    audioMode: trackAudioMode(candidate.audioMode),
    audioLanguages: stringArrayValue(candidate.audioLanguages),
    audioRequired: booleanValue(candidate.audioRequired, false),
    dropCommentary: booleanValue(candidate.dropCommentary, true),
    defaultAudioLanguage: stringValue(candidate.defaultAudioLanguage),
    subtitleMode: trackSubtitleMode(candidate.subtitleMode),
    subtitleLanguages: stringArrayValue(candidate.subtitleLanguages),
    subtitlesRequired: booleanValue(candidate.subtitlesRequired, false),
    defaultSubtitleLanguage: stringValue(candidate.defaultSubtitleLanguage),
    validationMode: trackValidationMode(candidate.validationMode),
    notes: stringValue(candidate.notes),
    disabled: booleanValue(candidate.disabled, false),
    deletedAt: stringValue(candidate.deletedAt),
  };
}

function trackVideoMode(value: unknown): TrackProfile['videoMode'] {
  if (value === 'all' || value === 'require-one') {
    return value;
  }
  return 'first';
}

function trackAudioMode(value: unknown): TrackProfile['audioMode'] {
  if (value === 'all' || value === 'default' || value === 'none') {
    return value;
  }
  return 'languages';
}

function trackSubtitleMode(value: unknown): TrackProfile['subtitleMode'] {
  if (value === 'all' || value === 'none' || value === 'forced' || value === 'languages') {
    return value;
  }
  return 'forced-or-languages';
}

function trackValidationMode(value: unknown): TrackProfile['validationMode'] {
  if (value === 'block' || value === 'warn') {
    return value;
  }
  return 'review';
}

function trackRuleLabel(value: string) {
  return value.replace(/-/g, ' ');
}

function profileTrackSummary(profile: TrackProfile) {
  const video = Array.isArray(profile.keepVideoStreams) ? `${profile.keepVideoStreams.length} video` : 'all video';
  const audio = Array.isArray(profile.keepAudioStreams) ? `${profile.keepAudioStreams.length} audio` : 'all audio';
  const subs = Array.isArray(profile.keepSubtitleStreams) ? `${profile.keepSubtitleStreams.length} subs` : 'all subs';
  return `${video} / ${audio} / ${subs}`;
}

function trackSelectionLabel(value: AssetConversionOverrideState, scan: ScanResult | undefined, type: MediaStreamInfo['type']) {
  if (!scan) {
    return 'scan asset';
  }
  const selected = conversionStreamIndexes(value, scan, type);
  const all = streamIndexesForType(scan, type);
  return `${selected.length}/${all.length}`;
}

function trackValidationLabel(value: TrackProfile['validationMode']) {
  switch (value) {
    case 'block':
      return 'block queue';
    case 'warn':
      return 'warn only';
    default:
      return 'mark review';
  }
}

function uniqueTrackKey(baseKey: string, profiles: TrackProfile[]) {
  const existing = new Set(profiles.map((profile) => profile.key));
  let nextKey = slugify(baseKey || 'track-profile');
  let suffix = 2;
  while (existing.has(nextKey)) {
    nextKey = `${slugify(baseKey || 'track-profile')}-${suffix}`;
    suffix += 1;
  }
  return nextKey;
}

function cleanTrackConversionOverride(value: AssetConversionOverrideState): AssetConversionOverrideState {
  const clean: AssetConversionOverrideState = {};
  if (Array.isArray(value.keepVideoStreams)) {
    clean.keepVideoStreams = normalizeNumberList(value.keepVideoStreams);
  }
  if (Array.isArray(value.keepAudioStreams)) {
    clean.keepAudioStreams = normalizeNumberList(value.keepAudioStreams);
  }
  if (Array.isArray(value.keepSubtitleStreams)) {
    clean.keepSubtitleStreams = normalizeNumberList(value.keepSubtitleStreams);
  }
  const audioMetadata = cleanStreamMetadataMap(value.audioMetadata);
  const subtitleMetadata = cleanStreamMetadataMap(value.subtitleMetadata);
  const videoMetadata = cleanStreamMetadataMap(value.videoMetadata);
  if (videoMetadata) {
    clean.videoMetadata = videoMetadata;
  }
  if (audioMetadata) {
    clean.audioMetadata = audioMetadata;
  }
  if (subtitleMetadata) {
    clean.subtitleMetadata = subtitleMetadata;
  }
  return clean;
}

function cleanStreamMetadataMap(value?: Record<string, StreamMetadataOverride>) {
  if (!value) {
    return undefined;
  }
  const clean: Record<string, StreamMetadataOverride> = {};
  Object.entries(value).forEach(([index, metadata]) => {
    const numericIndex = Number(index);
    if (!Number.isInteger(numericIndex) || numericIndex < 0) {
      return;
    }
    const item = cleanStreamMetadataOverride(metadata);
    if (!streamMetadataOverrideEmpty(item)) {
      clean[String(numericIndex)] = item;
    }
  });
  return Object.keys(clean).length ? clean : undefined;
}

function cleanStreamMetadataOverride(value: StreamMetadataOverride): StreamMetadataOverride {
  const clean: StreamMetadataOverride = {};
  const title = value.title?.trim();
  const language = value.language?.trim().toLowerCase();
  if (title) {
    clean.title = title;
  }
  if (language) {
    clean.language = language;
  }
  if (typeof value.default === 'boolean') {
    clean.default = value.default;
  }
  if (typeof value.forced === 'boolean') {
    clean.forced = value.forced;
  }
  return clean;
}

function streamMetadataOverrideEmpty(value: StreamMetadataOverride) {
  return !value.title && !value.language && value.default === undefined && value.forced === undefined;
}

function streamMetadataMapValue(value: unknown) {
  if (!value || typeof value !== 'object') {
    return undefined;
  }
  return cleanStreamMetadataMap(value as Record<string, StreamMetadataOverride>);
}

function optionalNumberList(value: unknown) {
  if (!Array.isArray(value)) {
    return undefined;
  }
  return normalizeNumberList(value.filter((candidate): candidate is number => typeof candidate === 'number'));
}

function conversionStreamIndexes(value: AssetConversionOverrideState, scan: ScanResult, type: MediaStreamInfo['type']) {
  const allIndexes = streamIndexesForType(scan, type);
  const selected =
    type === 'video'
      ? value.keepVideoStreams
      : type === 'audio'
        ? value.keepAudioStreams
        : value.keepSubtitleStreams;
  return Array.isArray(selected) ? selected : allIndexes;
}

function streamIndexesForType(scan: ScanResult, type: MediaStreamInfo['type']) {
  const streams = type === 'video' ? scan.videoStreams : type === 'audio' ? scan.audioStreams : scan.subtitleStreams;
  return normalizeNumberList(Array.isArray(streams) ? streams.map((stream) => stream.index) : []);
}

function selectedOrUndefined(selected: number[], allIndexes: number[]) {
  const normalizedSelected = normalizeNumberList(selected);
  const normalizedAll = normalizeNumberList(allIndexes);
  if (normalizedSelected.length === normalizedAll.length && normalizedSelected.every((value, index) => value === normalizedAll[index])) {
    return undefined;
  }
  return normalizedSelected;
}

function normalizeNumberList(values: number[]) {
  return Array.from(new Set(values.filter((value) => Number.isInteger(value) && value >= 0))).sort((left, right) => left - right);
}

function getAudioProfiles(settings?: AppSetting[]) {
  const value = settings?.find((setting) => setting.key === 'audioEnhancementProfiles')?.value.profiles;
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((profile) => normalizeAudioProfile(profile))
    .filter((profile): profile is AudioEnhancementProfile => Boolean(profile))
    .filter((profile) => !profile.disabled && !profile.deletedAt);
}

function stringArrayValue(value: unknown) {
  if (!Array.isArray(value)) {
    return [];
  }
  return normalizeStringList(value.filter((item): item is string => typeof item === 'string'));
}

function normalizeStringList(values: string[]) {
  const seen = new Set<string>();
  const normalized: string[] = [];
  values.forEach((value) => {
    const clean = value.trim().toLowerCase();
    if (!clean || seen.has(clean)) {
      return;
    }
    seen.add(clean);
    normalized.push(clean);
  });
  return normalized;
}

function normalizeAudioProfile(value: unknown): AudioEnhancementProfile | null {
  if (!value || typeof value !== 'object') {
    return null;
  }
  const candidate = value as Record<string, unknown>;
  if (typeof candidate.key !== 'string' || typeof candidate.name !== 'string' || typeof candidate.filters !== 'string') {
    return null;
  }
  return {
    key: candidate.key,
    name: candidate.name,
    description: stringValue(candidate.description),
    intent: stringValue(candidate.intent),
    filters: sanitizeAudioFilterChain(candidate.filters),
    rnnoiseModelPath: stringValue(candidate.rnnoiseModelPath),
    channelMode: channelModeValue(candidate.channelMode),
    forceStereoMode: forceStereoModeValue(candidate.forceStereoMode),
    stereoDelayMs: numberValue(candidate.stereoDelayMs, 12),
    stereoWidth: numberValue(candidate.stereoWidth, 20),
    eqBands: normalizeEqBands(candidate.eqBands),
    preserveOriginalTrack: booleanValue(candidate.preserveOriginalTrack, true),
    outputCodec: stringValue(candidate.outputCodec, 'aac'),
    targetLoudness: numberValue(candidate.targetLoudness, -18),
    truePeak: numberValue(candidate.truePeak, -2),
    notes: stringValue(candidate.notes),
    disabled: booleanValue(candidate.disabled, false),
    deletedAt: stringValue(candidate.deletedAt),
  };
}

function effectiveAudioFilters(profile: AudioEnhancementProfile) {
  return sanitizeAudioFilterChain([rnnoiseFilter(profile.rnnoiseModelPath), channelFilter(profile), normalizedBaseFilters(profile), eqFilterChain(profile.eqBands)]
    .filter(Boolean)
    .join(','));
}

function channelFilter(profile: AudioEnhancementProfile) {
  switch (profile.channelMode) {
    case 'dual-mono':
      return 'pan=stereo|c0=c0|c1=c0';
    case 'force-stereo':
      return forceStereoFilter(profile.forceStereoMode);
    case 'downmix-mono':
      return 'aresample=ocl=mono';
    case 'light-stereo': {
      const delay = Math.max(1, Math.min(40, Math.round(profile.stereoDelayMs || 12)));
      const width = Math.max(0, Math.min(100, profile.stereoWidth || 20));
      const feedback = trimGain((width / 100) * 0.12);
      const crossfeed = trimGain(Math.max(0.05, 0.22 - width / 600));
      return `pan=stereo|c0=c0|c1=c0,adelay=0|${delay},stereowiden=delay=${delay}:feedback=${feedback}:crossfeed=${crossfeed}:drymix=0.9`;
    }
    default:
      return '';
  }
}

function forceStereoFilter(mode: AudioEnhancementProfile['forceStereoMode']) {
  switch (mode) {
    case 'first-two':
      return 'pan=stereo|c0=c0|c1=c1';
    case 'duplicate-first':
      return 'pan=stereo|c0=c0|c1=c0';
    default:
      return 'aresample=ocl=stereo';
  }
}

function normalizedBaseFilters(profile: AudioEnhancementProfile) {
  const filters = profile.filters
    .split(',')
    .map((filter) => filter.trim())
    .filter(Boolean);
  const loudnorm = loudnormFilter(profile);
  let foundLoudnorm = false;
  const normalized = filters.map((filter) => {
    if (!filter.startsWith('loudnorm=')) {
      return filter;
    }
    foundLoudnorm = true;
    return loudnorm;
  });
  if (!foundLoudnorm) {
    normalized.push(loudnorm);
  }
  return normalized.join(',');
}

function loudnormFilter(profile: AudioEnhancementProfile) {
  const target = Number.isFinite(profile.targetLoudness) ? profile.targetLoudness : -18;
  const peak = Number.isFinite(profile.truePeak) ? profile.truePeak : -2;
  const existing = profile.filters
    .split(',')
    .map((filter) => filter.trim())
    .find((filter) => filter.startsWith('loudnorm='));
  const lraMatch = existing?.match(/(?:^|:)LRA=([^:]+)/);
  const lra = lraMatch?.[1] ?? '11';
  return `loudnorm=I=${trimGain(target)}:TP=${trimGain(peak)}:LRA=${lra}`;
}

function rnnoiseFilter(modelPath: string) {
  const path = modelPath.trim();
  return path ? `arnndn=m=${path}` : '';
}

function eqFilterChain(bands: Record<string, number>) {
  return eqFrequencies
    .map((frequency) => ({ frequency, gain: bands[String(frequency)] ?? 0 }))
    .filter(({ gain }) => Math.abs(gain) > 0)
    .map(({ frequency, gain }) => `equalizer=f=${frequency}:t=q:w=1:g=${trimGain(gain)}`)
    .join(',');
}

function sanitizeAudioFilterChain(filterChain: string) {
  return filterChain.replace(/afftdn=([^,]*\bnf=)(-?\d+(?:\.\d+)?)/g, (_match, prefix: string, rawValue: string) => {
    const parsed = Number(rawValue);
    if (!Number.isFinite(parsed)) {
      return `afftdn=${prefix}${rawValue}`;
    }
    return `afftdn=${prefix}${trimGain(Math.max(-80, Math.min(-20, parsed)))}`;
  });
}

function defaultEqBands() {
  return eqFrequencies.reduce<Record<string, number>>((bands, frequency) => {
    bands[String(frequency)] = 0;
    return bands;
  }, {});
}

function normalizeEqBands(value: unknown) {
  const bands = defaultEqBands();
  if (!value || typeof value !== 'object') {
    return bands;
  }
  const candidate = value as Record<string, unknown>;
  eqFrequencies.forEach((frequency) => {
    const key = String(frequency);
    bands[key] = Math.max(-12, Math.min(12, numberValue(candidate[key], 0)));
  });
  return bands;
}

function uniqueKey(baseKey: string, profiles: AudioEnhancementProfile[]) {
  const existing = new Set(profiles.map((profile) => profile.key));
  let nextKey = slugify(baseKey || 'audio-profile');
  let suffix = 2;
  while (existing.has(nextKey)) {
    nextKey = `${slugify(baseKey || 'audio-profile')}-${suffix}`;
    suffix += 1;
  }
  return nextKey;
}

function slugify(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

function trimGain(value: number) {
  return Number.isInteger(value) ? String(value) : value.toFixed(1);
}

function formatFrequency(frequency: number) {
  return frequency >= 1000 ? `${frequency / 1000} kHz` : `${frequency} Hz`;
}

function eqBandLabel(frequency: number) {
  if (frequency < 250) {
    return 'Bajos';
  }
  if (frequency < 4000) {
    return 'Medios';
  }
  return 'Agudos';
}

function stringValue(value: unknown, fallback = '') {
  return typeof value === 'string' ? value : fallback;
}

function channelModeValue(value: unknown): AudioEnhancementProfile['channelMode'] {
  if (value === 'dual-mono' || value === 'force-stereo' || value === 'downmix-mono' || value === 'light-stereo') {
    return value;
  }
  return 'preserve';
}

function forceStereoModeValue(value: unknown): AudioEnhancementProfile['forceStereoMode'] {
  if (value === 'first-two' || value === 'duplicate-first') {
    return value;
  }
  return 'auto';
}

function numberValue(value: unknown, fallback: number) {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback;
}

function booleanValue(value: unknown, fallback: boolean) {
  return typeof value === 'boolean' ? value : fallback;
}
