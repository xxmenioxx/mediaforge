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
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Grid,
  MenuItem,
  Slider,
  Stack,
  Tab,
  Tabs,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
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
import AutoFixHighIcon from '@mui/icons-material/AutoFixHigh';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import type { Dispatch, ReactNode, SetStateAction } from 'react';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { api } from '../api/client';
import type {
  AppSetting,
  Asset,
  AssetConversionOverrideState,
  AudioEnhancementProfile,
  CompatiblePreviewOptions,
  MediaStreamInfo,
  Profile,
  ProfileInput,
  ProfileSuggestion,
  PreviewVideoCharacteristics,
  PreviewFrameMetrics,
  ScanResult,
  StreamMetadataOverride,
} from '../api/types';
import { starterAudioProfiles } from '../audioProfiles';
import { MediaSnapshotDetails } from '../components/MediaSnapshotDetails';
import { PageHeader } from '../components/PageHeader';
import { qsvQualityHelper, qsvQualityRangeForCrf } from '../utils/qsv';
import { adaptiveVideoToolboxPresetKbps as sharedAdaptiveVideoToolboxPresetKbps, applyHardwareQualityPreset as applySharedHardwareQualityPreset, hardwareQualityPresetOptions } from '../utils/hardwareQualityPresets';
import { getTrackProfiles, trackProfileOverride, type TrackProfile } from '../trackProfiles';

const eqFrequencies = [60, 120, 250, 500, 1000, 2000, 4000, 8000, 12000] as const;

type LabSection = 'video' | 'audio' | 'tracks';

type LabRecommendationReport = {
  summary: string;
  match: string;
  video: string[];
  audio: string[];
  tracks: string[];
  general: string[];
};

type RecommendationSectionState = Record<LabSection, boolean>;

type LabFidelityInspection = {
  assetPath: string;
  source: PreviewVideoCharacteristics;
  reference: PreviewVideoCharacteristics;
  conversion: PreviewVideoCharacteristics;
  requestedEncoder: string;
  effectiveEncoder: string;
  referenceEncoder: string;
  normalization: {
    mode: 'preserve' | 'normalize_bt709';
    applied: boolean;
    filter?: string;
    reason: string;
    sarPreserved: boolean;
    aliasWarnings?: string[];
  };
  metrics: PreviewFrameMetrics;
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

const encoderPresetOptions = [
  { value: 'veryfast', label: 'Fast preview', description: 'Faster conversions, larger files. Useful for quick tests.' },
  { value: 'medium', label: 'Balanced', description: 'Recommended default for quality, size, and speed.' },
  { value: 'slow', label: 'Higher compression', description: 'Slower, usually smaller files at the same quality.' },
  { value: 'slower', label: 'Archive patience', description: 'Very slow. Use only when size matters and time is acceptable.' },
] as const;

const pixelFormatOptions = [
  { value: 'auto', label: 'Auto / codec default', description: 'Lets MVForge choose a compatible pixel format from the codec and encoder.' },
  { value: 'yuv420p10le', label: '10-bit Main10', description: 'Recommended for x265/anime/DVD. Helps reduce banding while staying widely playable.' },
  { value: 'p010le', label: 'Hardware 10-bit Main10 (P010)', description: 'Native 10-bit format for hardware HEVC Main10, including Quick Sync and VideoToolbox.' },
  { value: 'yuv420p', label: '8-bit compatibility', description: 'Use for older devices or codecs that do not need 10-bit output.' },
  { value: 'nv12', label: 'QSV 8-bit Main (NV12)', description: 'Native 8-bit input format for Intel Quick Sync HEVC Main.' },
] as const;

const videoEncoderOptions = [
  { value: 'auto', label: 'Auto', description: 'MVForge chooses software unless hardware fallback is enabled and available.' },
  { value: 'hevc_qsv', label: 'Intel Quick Sync', description: 'Fast HEVC hardware encoding for bulk conversion on Intel systems.' },
  { value: 'hevc_vaapi', label: 'VAAPI', description: 'Linux hardware encoding through the active VA-API driver.' },
  { value: 'hevc_nvenc', label: 'NVIDIA NVENC', description: 'Fast HEVC hardware encoding on NVIDIA GPUs.' },
  { value: 'hevc_videotoolbox', label: 'Apple VideoToolbox', description: 'HEVC hardware encoding on supported Apple Silicon and Intel Macs.' },
  { value: 'hevc_amf', label: 'AMD AMF', description: 'Fast HEVC hardware encoding on supported AMD GPUs.' },
  { value: 'libx265', label: 'Software x265', description: 'Slower, usually better compression and quality per GB.' },
] as const;

const deinterlaceOptions = [
  { value: 'off', label: 'Off' },
  { value: 'auto', label: 'Auto at conversion (uses Analysis)' },
  { value: 'force', label: 'Force · bwdif (single-rate)' },
  { value: 'ivtc_tff', label: 'Inverse telecine · fieldmatch + decimate (TFF)' },
  { value: 'ivtc_bff', label: 'Inverse telecine · fieldmatch + decimate (BFF)' },
] as const;

const denoiseOptions = [
  { value: 'off', label: 'Off' },
  { value: 'film-grain', label: 'Preserve grain · very light' },
  { value: 'film-restore', label: 'Film restoration · gentle' },
  { value: 'light', label: 'Light' },
  { value: 'medium', label: 'Medium' },
  { value: 'strong', label: 'Strong' },
] as const;

const unsharpOptions = [
  { value: 'off', label: 'Off' },
  { value: 'subtle', label: 'Subtle · 0.10' },
  { value: 'film-restore', label: 'Film restore · 0.15' },
  { value: 'light', label: 'Light · 0.25' },
] as const;

const debandOptions = [
  { value: 'off', label: 'Off' },
  { value: 'light', label: 'Light' },
  { value: 'medium', label: 'Medium' },
] as const;

const deflickerOptions = [
  { value: 'off', label: 'Off' },
  { value: 'light', label: 'Light · 5 frames' },
  { value: 'medium', label: 'Medium · 9 frames' },
] as const;

const deblockOptions = [
  { value: 'off', label: 'Off' },
  { value: 'light', label: 'Light' },
  { value: 'medium', label: 'Medium' },
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
      qualityValue: 20,
      workerConfig: {
        videoEncoder: 'hevc_qsv',
        preferredEncoder: 'hardware',
        useHardwareIfAvailable: true,
        globalQuality: 18,
        qsvRateControl: 'icq',
        qsvLookAheadDepth: 40,
        qsvExtendedBRC: false,
        qsvAdaptiveI: true,
        qsvAdaptiveB: true,
        videoPreset: 'medium',
        pixFmt: 'p010le',
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
      qualityValue: 23,
      workerConfig: {
        videoEncoder: 'hevc_qsv',
        preferredEncoder: 'hardware',
        useHardwareIfAvailable: true,
        globalQuality: 21,
        qsvRateControl: 'icq',
        qsvLookAheadDepth: 40,
        qsvExtendedBRC: false,
        qsvAdaptiveI: true,
        qsvAdaptiveB: false,
        videoPreset: 'medium',
        pixFmt: 'p010le',
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
    globalQuality: 18,
    videoPreset: 'medium',
    pixFmt: 'yuv420p10le',
    deinterlaceMode: 'off',
    denoise: 'off',
    deflicker: 'off',
    deblockFilter: 'off',
    unsharp: 'off',
    deband: 'off',
    crop: 'off',
    cropValue: '',
    brightness: 0,
    contrast: 100,
    saturation: 100,
    gamma: 100,
    temperature: 0,
    tint: 0,
    exposure: 0,
    vibrance: 0,
    blackPoint: 0,
    whitePoint: 100,
    videoFilters: '',
    x265Params: 'aq-mode=3:aq-strength=0.9:deblock=-1,-1',
    addAacStereoTrack: false,
    aacStereoBitrateKbps: 192,
    aacStereoDefault: false,
    subtitleOCRMode: 'accurate',
    subtitleOCRLanguage: 'auto',
    preserveOriginalAudio: true,
    preferSrtSubtitles: false,
    warnSubtitleFormats: true,
  },
};

function profileInputFromSavedProfile(profile: Profile): ProfileInput {
  const workerConfig = { ...emptyVideoDraft.workerConfig, ...profile.workerConfig };
  delete workerConfig.processingMode;
  const requestedPreference = typeof workerConfig.preferredEncoder === 'string' ? workerConfig.preferredEncoder : '';
  const hardwareSupported = hardwareEncodingSupportedForCodec(profile.videoCodec);
  const preference = requestedPreference === 'hardware' && !hardwareSupported
    ? 'software'
    : requestedPreference === 'hardware' || requestedPreference === 'software'
      ? requestedPreference
      : workerConfig.useHardwareIfAvailable === true ? 'hardware' : 'software';
  workerConfig.preferredEncoder = preference;
  workerConfig.useHardwareIfAvailable = preference !== 'software' && hardwareSupported;
  if (preference === 'software') {
    workerConfig.videoEncoder = 'auto';
  } else if (!isHardwareEncoderOption(String(workerConfig.videoEncoder ?? ''))) {
    workerConfig.videoEncoder = 'auto';
  }
  return synchronizeLabAuthoritativeContract({
    name: profile.name,
    description: profile.description,
    container: profile.container,
    videoCodec: profile.videoCodec,
    codecFamily: profile.codecFamily,
    encoderPolicy: profile.encoderPolicy,
    preferredEncoder: profile.preferredEncoder,
    allowedEncoders: profile.allowedEncoders,
    fallbackPolicy: profile.fallbackPolicy,
    bitDepth: profile.bitDepth,
    pixelFormat: profile.pixelFormat,
    qualityStrategy: profile.qualityStrategy,
    audioCodec: 'copy',
    qualityMode: profile.qualityMode,
    qualityValue: profile.qualityValue,
    preserveHdr: profile.preserveHdr,
    preserveSubtitles: profile.preserveSubtitles,
    preserveChapters: profile.preserveChapters,
    workerConfig,
    disabled: profile.disabled,
  });
}

const emptyAudioDraft: AudioEnhancementProfile = {
  key: '',
  name: '',
  description: '',
  intent: 'Asset-specific restoration',
  filters: 'anull',
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
  const [searchParams] = useSearchParams();
  const assets = useQuery({ queryKey: ['assets'], queryFn: api.assets });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const adminProfiles = useQuery({ queryKey: ['profiles', 'admin'], queryFn: api.profilesAdmin });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const runtimeSnapshot = useQuery({ queryKey: ['runtime-snapshot'], queryFn: api.runtimeSnapshot });
  const labAssets = useMemo(
    () => uniqueAssets([
      ...(assets.data?.unprocessed ?? []),
      ...(assets.data?.library ?? []),
      ...(assets.data?.converted ?? []),
      ...(assets.data?.unverified ?? []),
    ]),
    [assets.data],
  );
  const audioProfiles = useMemo(() => getAudioProfiles(settings.data), [settings.data]);
  const trackProfiles = useMemo(() => getTrackProfiles(settings.data), [settings.data]);
  const allAudioProfiles = useMemo(() => getAudioProfiles(settings.data, true), [settings.data]);
  const allTrackProfiles = useMemo(() => getTrackProfiles(settings.data, true), [settings.data]);
  const [labSection, setLabSection] = useState<LabSection>('video');
  const [assetPath, setAssetPath] = useState('');
  const [start, setStart] = useState('00:00:00');
  const [seconds, setSeconds] = useState(20);
  const [previewNormalization, setPreviewNormalization] = useState<'preserve' | 'normalize_bt709'>('normalize_bt709');
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
  const [savedVideoProfileId, setSavedVideoProfileId] = useState<number | null>(null);
  const [videoProfileSaveMessage, setVideoProfileSaveMessage] = useState('');
  const [selectedVideoStarterPreset, setSelectedVideoStarterPreset] = useState('');
  const [videoAdvancedOpen, setVideoAdvancedOpen] = useState(false);
  const [audioDraft, setAudioDraft] = useState<AudioEnhancementProfile>(emptyAudioDraft);
  const [savedAudioProfileKey, setSavedAudioProfileKey] = useState<string | null>(null);
  const [selectedAudioStarterPreset, setSelectedAudioStarterPreset] = useState('');
  const [trackDraft, setTrackDraft] = useState<TrackProfile>(emptyTrackDraft);
  const [savedTrackProfileKey, setSavedTrackProfileKey] = useState<string | null>(null);
  const [trackConversionDraft, setTrackConversionDraft] = useState<AssetConversionOverrideState>({});
  const [terminalCommandOpen, setTerminalCommandOpen] = useState(false);
  const [terminalCommandCopied, setTerminalCommandCopied] = useState(false);
  const [fidelityOpen, setFidelityOpen] = useState(true);
  const [fidelityTab, setFidelityTab] = useState<'previews' | 'frames' | 'characteristics'>('previews');
  const [recommendationReport, setRecommendationReport] = useState<LabRecommendationReport | null>(null);
  const [recommendationSuggestion, setRecommendationSuggestion] = useState<ProfileSuggestion | null>(null);
  const [recommendationOpen, setRecommendationOpen] = useState(false);
  const [recommendationApplied, setRecommendationApplied] = useState<RecommendationSectionState>({ video: false, audio: false, tracks: false });
  const [recommendationSelected, setRecommendationSelected] = useState<RecommendationSectionState>({ video: true, audio: true, tracks: true });
  const [videoSaveReviewOpen, setVideoSaveReviewOpen] = useState(false);
  const [audioSaveReviewOpen, setAudioSaveReviewOpen] = useState(false);
  const [trackSaveReviewOpen, setTrackSaveReviewOpen] = useState(false);
  const [pendingTrackProfileSave, setPendingTrackProfileSave] = useState<TrackProfile | null>(null);
  const previewsRef = useRef<HTMLDivElement | null>(null);
  const loadedEditRequestRef = useRef('');
  const recommendationDefaultsRef = useRef<{
    video: ProfileInput;
    audio: AudioEnhancementProfile;
    tracks: TrackProfile;
    trackConversion: AssetConversionOverrideState;
    savedVideoProfileId: number | null;
    selectedVideoStarterPreset: string;
    selectedAudioStarterPreset: string;
  } | null>(null);
  const selectedAsset = labAssets.find((asset) => asset.path === assetPath) ?? null;
  const videoNameConflict = Boolean(videoDraft.name.trim()) && [...(profiles.data ?? []), ...(adminProfiles.data ?? [])].some(
    (profile) => profile.id !== savedVideoProfileId && sameProfileName(profile.name, videoDraft.name),
  );
  const audioNameConflict = Boolean(audioDraft.name.trim()) && allAudioProfiles.some(
    (profile) => profile.key !== savedAudioProfileKey && sameProfileName(profile.name, audioDraft.name),
  );
  const trackNameConflict = Boolean(trackDraft.name.trim()) && allTrackProfiles.some(
    (profile) => profile.key !== savedTrackProfileKey && sameProfileName(profile.name, trackDraft.name),
  );
  useEffect(() => {
    const requestedAssetPath = searchParams.get('assetPath') || '';
    if (requestedAssetPath && labAssets.some((asset) => asset.path === requestedAssetPath) && assetPath !== requestedAssetPath) {
      setAssetPath(requestedAssetPath);
      resetProcessedPreviews();
    }
  }, [assetPath, labAssets, searchParams]);
  const selectedHardwareEncoder = videoWorkerValue(videoDraft, 'videoEncoder', 'auto');
  const selectedHardwareCapability = selectedHardwareEncoder === 'auto'
    ? undefined
    : runtimeSnapshot.data?.encoders?.[selectedHardwareEncoder];
  const qsvMain10Selected = ['p010le', 'yuv420p10le'].includes(videoWorkerValue(videoDraft, 'pixFmt', '').toLowerCase());
  const qsvLookAheadAvailable = qsvMain10Selected && selectedHardwareCapability?.qsvLaIcqMain10 === true;
  const qsvAdvancedAvailable = qsvMain10Selected && selectedHardwareCapability?.qsvFullCombination === true;
  const videoToolboxMain10Selected = videoWorkerValue(videoDraft, 'videoToolboxProfile', '').toLowerCase() === 'main10'
    || ['p010le', 'yuv420p10le'].includes(videoWorkerValue(videoDraft, 'pixFmt', '').toLowerCase());
  const videoToolboxBFramesAvailable = selectedHardwareCapability?.videoToolboxBFrames === true
    && (!videoToolboxMain10Selected || selectedHardwareCapability.testedModes?.videoToolboxBFramesMain10 === true);
  const videoToolboxPowerAvailable = selectedHardwareCapability?.videoToolboxPowerEfficient === true
    && (!videoToolboxMain10Selected || selectedHardwareCapability.testedModes?.videoToolboxPowerEfficientMain10 === true);
  const hasUsableHardwareEncoder = videoEncoderOptions.some(
    (encoder) => isHardwareEncoderOption(encoder.value) && runtimeSnapshot.data?.encoders?.[encoder.value]?.usable,
  );
  const defaultHardwareEncoder = videoEncoderOptions.find(
    (encoder) => isHardwareEncoderOption(encoder.value) && runtimeSnapshot.data?.encoders?.[encoder.value]?.usable,
  )?.value ?? '';
  const currentAudioFilters = effectiveAudioFilters(audioDraft);
  const previewAudioFilters = audioFilterChain.trim() || 'anull';
  const videoPreviewStale =
    videoPreviewNonce > 0 &&
    videoPreviewStatus !== 'loading' &&
    (processedVideoCodec !== videoDraft.videoCodec ||
      processedVideoQualityValue !== videoDraft.qualityValue ||
      JSON.stringify(processedVideoOptions) !== JSON.stringify(videoPreviewOptions(videoDraft)));

  useEffect(() => {
    if (
      videoWorkerValue(videoDraft, 'preferredEncoder', 'software') === 'hardware' &&
      !isHardwareEncoderOption(selectedHardwareEncoder) &&
      defaultHardwareEncoder
    ) {
      updateVideoHardwareEncoder(setVideoDraft, defaultHardwareEncoder);
    }
  }, [defaultHardwareEncoder, selectedHardwareEncoder, videoDraft]);

  useEffect(() => {
    const videoProfileId = Number(searchParams.get('videoProfileId') || 0);
    const audioProfileKey = searchParams.get('audioProfileKey') || '';
    const trackProfileKey = searchParams.get('trackProfileKey') || '';
    const requestKey = videoProfileId > 0
      ? `video:${videoProfileId}`
      : audioProfileKey
        ? `audio:${audioProfileKey}`
        : trackProfileKey
          ? `tracks:${trackProfileKey}`
          : '';
    if (!requestKey || loadedEditRequestRef.current === requestKey) {
      return;
    }

    if (videoProfileId > 0) {
      const profile = [...(profiles.data ?? []), ...(adminProfiles.data ?? [])].find((candidate) => candidate.id === videoProfileId);
      if (!profile) {
        return;
      }
      setLabSection('video');
      setVideoDraft(profileInputFromSavedProfile(profile));
      setSavedVideoProfileId(profile.id);
      setVideoProfileSaveMessage('');
      setSelectedVideoStarterPreset('');
    } else if (audioProfileKey) {
      const profile = audioProfiles.find((candidate) => candidate.key === audioProfileKey);
      if (!profile) {
        return;
      }
      setLabSection('audio');
      setAudioDraft({ ...profile, eqBands: normalizeEqBands(profile.eqBands) });
      setSavedAudioProfileKey(profile.key);
      setAudioFilterChainEdited(false);
      setSelectedAudioStarterPreset('');
    } else {
      const profile = allTrackProfiles.find((candidate) => candidate.key === trackProfileKey);
      if (!profile) {
        return;
      }
      setLabSection('tracks');
      setTrackDraft({ ...profile });
      setSavedTrackProfileKey(profile.key);
      setTrackConversionDraft(trackProfileOverride(profile));
      const sourceAsset = labAssets.find((asset) => asset.path === profile.sourceAssetPath)
        ?? labAssets.find((asset) => Boolean(profile.sourceAssetName) && asset.fileName === profile.sourceAssetName);
      if (sourceAsset) {
        setAssetPath(sourceAsset.path);
      }
    }
    loadedEditRequestRef.current = requestKey;
  }, [adminProfiles.data, allTrackProfiles, audioProfiles, labAssets, profiles.data, searchParams]);

  useEffect(() => {
    if (!audioFilterChainEdited) {
      setAudioFilterChain(currentAudioFilters || 'anull');
    }
  }, [audioFilterChainEdited, currentAudioFilters]);

  const saveVideoProfileMutation = useMutation({
    mutationFn: async (profile: ProfileInput) => {
      if (savedVideoProfileId) {
        return { profile: await api.updateProfile({ id: savedVideoProfileId, ...profile }), action: 'updated' as const };
      }
      return { profile: await api.createProfile(profile), action: 'created' as const };
    },
    onSuccess: async ({ profile, action }) => {
      setSavedVideoProfileId(profile.id);
      setVideoProfileSaveMessage(action === 'updated' ? 'Video profile updated.' : 'Video profile saved.');
      setVideoSaveReviewOpen(false);
      await queryClient.invalidateQueries({ queryKey: ['profiles'] });
    },
  });

  const updateSetting = useMutation({
    mutationFn: api.updateSetting,
    onSuccess: async (_setting, variables) => {
      if (variables.key === 'audioEnhancementProfiles') {
        setSavedAudioProfileKey(slugify(audioDraft.key || audioDraft.name));
        setAudioSaveReviewOpen(false);
      } else if (variables.key === 'trackProfiles') {
        setSavedTrackProfileKey(pendingTrackProfileSave?.key ?? slugify(trackDraft.key || trackDraft.name));
        setPendingTrackProfileSave(null);
        setTrackSaveReviewOpen(false);
      }
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });
  const updateAudioSourceStream = useMutation({
    mutationFn: api.updateAssetConversion,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });
  const autoRecommendation = useMutation({
    mutationFn: api.suggestProfile,
    onSuccess: (suggestion) => {
      recommendationDefaultsRef.current = {
        video: videoDraft,
        audio: audioDraft,
        tracks: trackDraft,
        trackConversion: trackConversionDraft,
        savedVideoProfileId,
        selectedVideoStarterPreset,
        selectedAudioStarterPreset,
      };
      setRecommendationSuggestion(suggestion);
      setRecommendationReport(previewRecommendationReport(suggestion));
      setRecommendationApplied({ video: false, audio: false, tracks: false });
      setRecommendationSelected({ video: true, audio: true, tracks: true });
      setRecommendationOpen(true);
    },
  });

  const trackSnapshot = useMutation({ mutationFn: api.scan, onSuccess: (scan) => {
    setVideoDraft((current) => {
      const encoder = videoWorkerValue(current, 'videoEncoder', 'auto');
      const preset = videoWorkerValue(current, 'hardwareQualityPreset', '');
      if (!isHardwareEncoderOption(encoder) || !preset || preset === 'custom') return current;
      const workerConfig = applySharedHardwareQualityPreset(current.workerConfig ?? {}, encoder, preset, scan);
      if (JSON.stringify(workerConfig) === JSON.stringify(current.workerConfig)) return current;
      return synchronizeLabAuthoritativeContract({ ...current, workerConfig });
    });
  } });
  const fidelityInspection = useMutation({
    mutationFn: async ({
      reference,
      conversion,
    }: {
      reference: Parameters<typeof api.inspectCompatibleAssetPreview>[0];
      conversion: Parameters<typeof api.inspectCompatibleAssetPreview>[0];
    }): Promise<LabFidelityInspection> => {
      const [referenceInspection, conversionInspection, metrics] = await Promise.all([
        api.inspectCompatibleAssetPreview(reference),
        api.inspectCompatibleAssetPreview(conversion),
        api.compatibleAssetFrameMetrics(conversion),
      ]);
      return {
        assetPath: conversion.path,
        source: referenceInspection.source,
        reference: referenceInspection.output,
        conversion: conversionInspection.output,
        requestedEncoder: conversionInspection.requestedEncoder,
        effectiveEncoder: conversionInspection.effectiveEncoder,
        referenceEncoder: referenceInspection.effectiveEncoder,
        normalization: referenceInspection.normalization,
        metrics,
      };
    },
    retry: (failureCount, error) => failureCount < 1 && isCanceledFidelityRequest(error),
    retryDelay: 250,
  });
  const profileSampleEstimate = useMutation({ mutationFn: api.estimateCompatibleAssetProfile });
  const currentProfileSampleEstimate = profileSampleEstimate.data?.assetPath === assetPath
    && profileSampleEstimate.variables?.path === assetPath
    && JSON.stringify(profileSampleEstimate.variables.profile) === JSON.stringify(videoDraft)
    ? profileSampleEstimate.data
    : undefined;
  const currentFidelityInspection = fidelityInspection.data?.assetPath === assetPath
    ? fidelityInspection.data
    : undefined;
  const availableAudioStreams = trackSnapshot.data?.audioStreams ?? [];
  const selectedAudioStreamIndex = availableAudioStreams.some((stream) => stream.index === audioPreviewStreamIndex)
    ? audioPreviewStreamIndex ?? undefined
    : availableAudioStreams.find((stream) => stream.default)?.index ?? availableAudioStreams[0]?.index;

  useEffect(() => {
    // Sample A is generated automatically when an asset is selected. The old
    // Preview button used to initialize this nonce; without it the entire
    // Fidelity workbench remained hidden and Sample B actions stayed disabled.
    setPreviewNonce((current) => assetPath ? current + 1 : 0);
    resetProcessedPreviews();
    fidelityInspection.reset();
    if (assetPath) {
      trackSnapshot.reset();
      trackSnapshot.mutate({ path: assetPath });
    }
  }, [assetPath]);

  function selectVideoProfile(profileId: number) {
    const profile = (profiles.data ?? []).find((candidate) => candidate.id === profileId);
    if (!profile) {
      return;
    }
    setSavedVideoProfileId(null);
    setVideoProfileSaveMessage('');
    setSelectedVideoStarterPreset('');
    const draft = profileInputFromSavedProfile(profile);
    const encoder = videoWorkerValue(draft, 'videoEncoder', 'auto');
    const qualityPreset = videoWorkerValue(draft, 'hardwareQualityPreset', '');
    const workerConfig = isHardwareEncoderOption(encoder) && qualityPreset && qualityPreset !== 'custom'
      ? applySharedHardwareQualityPreset(draft.workerConfig ?? {}, encoder, qualityPreset, trackSnapshot.data)
      : draft.workerConfig;
    setVideoDraft({
      ...draft,
      name: `${profile.name} - ${selectedAsset?.fileName ?? 'Asset'} Lab`,
      description: `Derived in Profile Lab from ${profile.name}${selectedAsset ? ` for ${selectedAsset.relativePath || selectedAsset.fileName}` : ''}.`,
      audioCodec: 'copy',
      workerConfig: { ...workerConfig, derivedFromProfileId: profile.id, derivedFromAsset: selectedAsset?.path ?? '' },
    });
  }

  function applyAutoRecommendation(suggestion: ProfileSuggestion, section: LabSection) {
    const scan = suggestion.scan;
    const proposed = suggestion.proposedProfile;
    const sourceName = selectedAsset?.fileName ?? scan.fileName;
    if (section === 'video') {
      setSavedVideoProfileId(null);
      setVideoProfileSaveMessage('');
    } else if (section === 'audio') {
      setSavedAudioProfileKey(null);
    } else if (section === 'tracks') {
      setSavedTrackProfileKey(null);
    }
    const interlaceStatus = scan.interlaceAnalysis.status ?? 'unknown';
    const cropStatus = scan.cropAnalysis?.status ?? 'unknown';
    const recommendedCrop = scan.cropAnalysis?.recommendedCrop?.trim() ?? '';
    const bitmapSubtitleStreams = scan.subtitleStreams.filter(isBitmapSubtitleStream);
    const cropSuggestionAvailable = ['detected', 'variable'].includes(cropStatus) && Boolean(recommendedCrop);
    const cropSuggestionSafe = cropSuggestionAvailable && bitmapSubtitleStreams.length === 0;
    const sourceIsHEVC = ['hevc', 'h265', 'x265'].some((codec) => scan.videoCodec.toLowerCase().includes(codec));
    const needsVideoCorrection = interlaceStatus === 'interlaced' ||
      interlaceStatus === 'telecine_suspected' ||
      scan.interlaceAnalysis.fieldOrderMismatch === true;
    const targetVideoCodec = sourceIsHEVC && !needsVideoCorrection ? 'copy' : proposed.videoCodec;
    const videoReasons = targetVideoCodec === 'copy'
      ? ['The source is already HEVC and no definite filter correction was detected, so video copy avoids generational loss.']
      : [`CRF ${suggestion.insights.recommendedCrf} was selected from the detected ${scan.width}×${scan.height} source.`];
    let deinterlaceMode = 'auto';
    if (interlaceStatus === 'progressive') {
      deinterlaceMode = 'off';
      videoReasons.push('The analyzed motion window is progressive, so deinterlacing was disabled.');
      if (scan.interlaceAnalysis.fieldOrderMismatch) {
        videoReasons.push(
          `The container declares ${(scan.interlaceAnalysis.containerFieldOrder || 'interlaced').toUpperCase()}, but the sampled frames are progressive; output field metadata will be corrected without deinterlacing.`,
        );
      }
    } else if (interlaceStatus === 'interlaced') {
      deinterlaceMode = 'force';
      videoReasons.push('Interlacing was detected, so bwdif was enabled for the video draft.');
    } else if (interlaceStatus === 'telecine_suspected') {
      const detectedOrder = scan.interlaceAnalysis.detectedFieldOrder?.toLowerCase()
        || scan.interlaceAnalysis.fieldOrder?.toLowerCase()
        || '';
      deinterlaceMode = scan.interlaceAnalysis.recommendedMode
        || (detectedOrder.startsWith('b') ? 'ivtc_bff' : 'ivtc_tff');
      videoReasons.push(
        `Telecine is suspected; inverse telecine ${deinterlaceMode === 'ivtc_bff' ? 'BFF' : 'TFF'} was enabled with fieldmatch and decimate.`,
      );
      if (scan.interlaceAnalysis.fieldOrderMismatch) {
        videoReasons.push(
          `The container reports ${(scan.interlaceAnalysis.containerFieldOrder || 'unknown').toUpperCase()}, but distributed samples detected ${detectedOrder.toUpperCase()}; the measured content order was used.`,
        );
      }
    } else {
      videoReasons.push('Motion classification is not definitive, so conversion-time automatic analysis remains enabled.');
    }
    if (scan.hdr) {
      videoReasons.push('HDR and 10-bit output are preserved because HDR metadata was detected.');
    }
    if (cropSuggestionSafe) {
      videoReasons.push(
        `Stable black bars were detected in ${scan.cropAnalysis.matchingWindows ?? 0}/${scan.cropAnalysis.windows ?? 0} samples; crop=${recommendedCrop} was added to the draft but left disabled for visual confirmation.`,
      );
    } else if (cropStatus === 'detected' && bitmapSubtitleStreams.length > 0) {
      videoReasons.push(
        `Stable black bars were detected, but crop remains disabled because ${bitmapSubtitleStreams.length} bitmap subtitle track(s) may be positioned inside those bars.`,
      );
    } else if (cropStatus === 'variable') {
      videoReasons.push(recommendedCrop
        ? `Black bars varied slightly between samples; conservative candidate crop=${recommendedCrop} was added but remains disabled for manual review.`
        : 'Black bars varied between samples, so crop remains disabled and requires manual review.');
    }
    const workerConfig = {
      ...proposed.workerConfig,
      deinterlaceMode,
      correctProgressiveFieldMetadata: scan.interlaceAnalysis.fieldOrderMismatch === true,
      denoise: 'off',
      deflicker: 'off',
      deblockFilter: 'off',
      unsharp: 'off',
      deband: 'off',
      crop: 'off',
      cropValue: cropSuggestionAvailable ? recommendedCrop : '',
      brightness: 0,
      contrast: 100,
      saturation: 100,
      gamma: 100,
      temperature: 0,
      tint: 0,
      exposure: 0,
      vibrance: 0,
      blackPoint: 0,
      whitePoint: 100,
    };
    videoReasons.push('Image adjustment controls remain neutral because snapshot metadata alone cannot justify exposure, levels, color, or detail corrections.');
    const recommendedVideoDraft: ProfileInput = synchronizeLabAuthoritativeContract({
      ...proposed,
      name: `Auto recommended - ${sourceName}`,
      description: `Generated in Profile Lab from the current snapshot of ${sourceName}.`,
      videoCodec: targetVideoCodec,
      qualityValue: suggestion.insights.recommendedCrf,
      workerConfig: {
        ...workerConfig,
        source: 'profile-lab-auto-recommendation',
        derivedFromAsset: selectedAsset?.path ?? scan.path,
        videoFilters: buildVideoFilterChain(workerConfig),
      },
    });
    if (section === 'video') {
      setSelectedVideoStarterPreset('');
      setVideoDraft(recommendedVideoDraft);
      setPreviewNonce((current) => current + 1);
      setProcessedVideoCodec(recommendedVideoDraft.videoCodec);
      setProcessedVideoQualityValue(recommendedVideoDraft.qualityValue);
      setProcessedVideoOptions(videoPreviewOptions(recommendedVideoDraft));
      setVideoPreviewStatus('loading');
      setVideoPreviewNonce((current) => current + 1);
      scrollToPreviews();
    }

    const incompatibleAudio = scan.audioStreams.filter((stream) => !['aac', 'ac3', 'eac3', 'opus', 'flac'].includes(stream.codec.toLowerCase()));
    const multichannelAudio = scan.audioStreams.filter((stream) => (stream.channels ?? 0) > 2);
    const audioReasons: string[] = [];
    const recommendedAudioDraft: AudioEnhancementProfile = {
      ...emptyAudioDraft,
      key: uniqueKey(slugify(`auto-compatible-${sourceName}`), audioProfiles),
      name: `Compatible audio - ${sourceName}`,
      description: `Conservative compatibility copy derived from the snapshot of ${sourceName}.`,
      filters: 'anull',
      channelMode: multichannelAudio.length > 0 ? 'force-stereo' : 'preserve',
      preserveOriginalTrack: true,
      outputCodec: 'aac',
      notes: 'Auto recommendation preserves the original and does not apply EQ, denoise, or loudness changes without an audio measurement.',
    };
    if (section === 'audio') {
      setSelectedAudioStarterPreset('');
      setSavedAudioProfileKey(null);
      setAudioDraft(recommendedAudioDraft);
      setAudioFilterChainEdited(false);
      setPreviewNonce((current) => current + 1);
      setProcessedAudioFilters(effectiveAudioFilters(recommendedAudioDraft) || 'anull');
      setProcessedAudioChannelMode(recommendedAudioDraft.channelMode);
      setAudioPreviewStatus('loading');
      setAudioPreviewNonce((current) => current + 1);
      scrollToPreviews();
    }
    audioReasons.push(
      incompatibleAudio.length > 0
        ? `${incompatibleAudio.length} audio track(s) use less browser-friendly codecs; an AAC compatibility copy was prepared while preserving the originals.`
        : 'The audio codecs are already broadly compatible; the draft remains loss-safe and preserves every original track.',
    );
    if (multichannelAudio.length > 0) {
      audioReasons.push('A stereo AAC compatibility copy is proposed for multichannel audio; the surround originals remain intact.');
    }
    audioReasons.push('No EQ, denoise, or loudness correction was inferred from metadata alone.');

    const commentaryIndexes = scan.audioStreams.filter(isCommentaryStream).map((stream) => stream.index);
    const keptAudioIndexes = scan.audioStreams.filter((stream) => !commentaryIndexes.includes(stream.index)).map((stream) => stream.index);
    const trackReasons = [
      `${scan.videoStreams.length} video, ${scan.audioStreams.length} audio, and ${scan.subtitleStreams.length} subtitle track(s) were read from the snapshot.`,
    ];
    const recommendedTrackDraft: TrackProfile = {
      ...emptyTrackDraft,
      key: uniqueTrackKey(slugify(`auto-tracks-${sourceName}`), trackProfiles),
      name: `Recommended tracks - ${sourceName}`,
      description: `Track selection derived from the current snapshot of ${sourceName}.`,
      sourceAssetPath: selectedAsset?.path ?? scan.path,
      sourceAssetName: sourceName,
      videoMode: 'all',
      audioMode: 'all',
      subtitleMode: 'all',
      audioRequired: scan.audioStreams.length > 0,
      subtitlesRequired: false,
      dropCommentary: commentaryIndexes.length > 0,
      notes: 'Review language/default choices before saving. Missing tracks are validated in review mode.',
    };
    const recommendedTrackConversion: AssetConversionOverrideState = {
      keepAudioStreams: commentaryIndexes.length > 0 ? keptAudioIndexes : undefined,
    };
    if (section === 'tracks') {
      setSavedTrackProfileKey(null);
      setTrackDraft(recommendedTrackDraft);
      setTrackConversionDraft(recommendedTrackConversion);
    }
    trackReasons.push(
      commentaryIndexes.length > 0
        ? `${commentaryIndexes.length} commentary track(s) were identified by metadata and deselected; review before saving.`
        : 'No commentary tracks were identified, so no tracks were removed automatically.',
    );
    trackReasons.push('All subtitle tracks are preserved; language and forced/default metadata remain editable.');

    setRecommendationReport({
      summary: suggestion.summary,
      match: suggestion.matchType === 'existing' && suggestion.suggestedProfile
        ? `Closest existing profile: ${suggestion.suggestedProfile.name}`
        : 'A new profile draft is recommended.',
      video: videoReasons,
      audio: audioReasons,
      tracks: trackReasons,
      general: suggestion.insights.recommendations,
    });
    setRecommendationApplied((current) => ({ ...current, [section]: true }));
  }

  function resetAutoRecommendation(section: LabSection) {
    const defaults = recommendationDefaultsRef.current;
    if (!defaults) {
      return;
    }
    if (section === 'video') {
      setVideoDraft(defaults.video);
      setSavedVideoProfileId(defaults.savedVideoProfileId);
      setVideoProfileSaveMessage('');
      setSelectedVideoStarterPreset(defaults.selectedVideoStarterPreset);
      setPreviewNonce((current) => current + 1);
      setProcessedVideoCodec(defaults.video.videoCodec);
      setProcessedVideoQualityValue(defaults.video.qualityValue);
      setProcessedVideoOptions(videoPreviewOptions(defaults.video));
      setVideoPreviewStatus('loading');
      setVideoPreviewNonce((current) => current + 1);
    } else if (section === 'audio') {
      setAudioDraft(defaults.audio);
      setAudioFilterChainEdited(false);
      setSelectedAudioStarterPreset(defaults.selectedAudioStarterPreset);
      setPreviewNonce((current) => current + 1);
      setProcessedAudioFilters(effectiveAudioFilters(defaults.audio) || 'anull');
      setProcessedAudioChannelMode(defaults.audio.channelMode);
      setAudioPreviewStatus('loading');
      setAudioPreviewNonce((current) => current + 1);
    } else {
      setTrackDraft(defaults.tracks);
      setTrackConversionDraft(defaults.trackConversion);
    }
    setRecommendationApplied((current) => ({ ...current, [section]: false }));
  }

  function applyStarterVideoPreset(presetKey: string) {
    const preset = videoStarterPresets.find((candidate) => candidate.key === presetKey);
    if (!preset) {
      return;
    }
    setSavedVideoProfileId(null);
    setVideoProfileSaveMessage('');
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
    setSavedAudioProfileKey(null);
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
    setSavedAudioProfileKey(null);
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
    if (videoNameConflict) return;
    saveVideoProfileMutation.reset();
    setVideoSaveReviewOpen(true);
  }

  function confirmSaveVideoProfile() {
    if (videoNameConflict) return;
    saveVideoProfileMutation.mutate({
      ...videoDraft,
      audioCodec: 'copy',
      workerConfig: {
        ...videoDraft.workerConfig,
        source: 'profile-lab',
        derivedFromAsset: selectedAsset?.path ?? '',
      },
    });
  }

  function saveAudioProfile() {
    if (audioNameConflict) return;
    updateSetting.reset();
    setAudioSaveReviewOpen(true);
  }

  function confirmSaveAudioProfile() {
    if (audioNameConflict) return;
    const normalized = normalizedAudioProfileForSave(audioDraft, audioFilterChainEdited, previewAudioFilters);
    const existing = allAudioProfiles.filter((profile) => profile.key !== (savedAudioProfileKey ?? normalized.key));
    updateSetting.mutate({ key: 'audioEnhancementProfiles', value: { profiles: [...existing, normalized] } });
  }

  function saveTrackProfile() {
    if (trackNameConflict) return;
    updateSetting.reset();
    setPendingTrackProfileSave(normalizedTrackProfileDraft());
    setTrackSaveReviewOpen(true);
  }

  function confirmSaveTrackProfile() {
    if (trackNameConflict || !pendingTrackProfileSave) return;
    const existing = allTrackProfiles.filter((profile) => profile.key !== (savedTrackProfileKey ?? pendingTrackProfileSave.key));
    updateSetting.mutate({ key: 'trackProfiles', value: { profiles: [...existing, pendingTrackProfileSave] } });
  }

  function normalizedTrackProfileDraft(): TrackProfile {
    const conversion = cleanTrackConversionOverride(trackConversionDraft);
    return {
      ...trackDraft,
      key: slugify(trackDraft.key || trackDraft.name),
      sourceAssetPath: selectedAsset?.path ?? trackDraft.sourceAssetPath ?? '',
      sourceAssetName: selectedAsset?.fileName ?? trackDraft.sourceAssetName ?? '',
      keepVideoStreams: conversion.keepVideoStreams ?? undefined,
      keepAudioStreams: conversion.keepAudioStreams ?? undefined,
      keepSubtitleStreams: conversion.keepSubtitleStreams ?? undefined,
      videoMetadata: conversion.videoMetadata,
      audioMetadata: conversion.audioMetadata,
      subtitleMetadata: conversion.subtitleMetadata,
      subtitleTransforms: conversion.subtitleTransforms,
      audioLanguages: normalizeStringList(trackDraft.audioLanguages),
      subtitleLanguages: normalizeStringList(trackDraft.subtitleLanguages),
      defaultAudioLanguage: trackDraft.defaultAudioLanguage.trim().toLowerCase(),
      defaultSubtitleLanguage: trackDraft.defaultSubtitleLanguage.trim().toLowerCase(),
    };
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
    setSavedTrackProfileKey(null);
    setTrackDraft({
      ...profile,
      key: uniqueTrackKey(`${profile.key}-lab`, trackProfiles),
      name: nextName,
      sourceAssetPath: selectedAsset?.path ?? profile.sourceAssetPath,
      sourceAssetName: selectedAsset?.fileName ?? profile.sourceAssetName,
    });
    setTrackConversionDraft(trackProfileOverride(profile));
  }

  function updateTrackSubtitleTransform(stream: MediaStreamInfo, format: '' | 'srt' | 'ass') {
    setTrackConversionDraft((current) => {
      const remaining = (current.subtitleTransforms ?? []).filter((item) => item.streamIndex !== stream.index);
      if (!format) return { ...current, subtitleTransforms: remaining.length ? remaining : undefined };
      const bitmap = isBitmapSubtitleStream(stream);
      const allSubtitleIndexes = trackSnapshot.data ? streamIndexesForType(trackSnapshot.data, 'subtitle') : [];
      const selectedSubtitleIndexes = trackSnapshot.data
        ? conversionStreamIndexes(current, trackSnapshot.data, 'subtitle').filter((index) => index !== stream.index)
        : (current.keepSubtitleStreams ?? []).filter((index) => index !== stream.index);
      return {
        ...current,
        keepSubtitleStreams: selectedOrUndefined(selectedSubtitleIndexes, allSubtitleIndexes),
        subtitleTransforms: [...remaining, {
          streamIndex: stream.index,
          format,
          removeEmbedded: true,
          makeDefault: stream.default,
          language: stream.language || 'und',
          ocrLanguage: bitmap ? defaultTrackOCRLanguage(stream.language) : undefined,
          ocrMode: bitmap ? 'accurate' : undefined,
          title: stream.title || undefined,
        }],
      };
    });
  }

  function updateTrackSubtitleTransformValue(streamIndex: number, patch: Partial<NonNullable<AssetConversionOverrideState['subtitleTransforms']>[number]>) {
    setTrackConversionDraft((current) => ({
      ...current,
      subtitleTransforms: (current.subtitleTransforms ?? []).map((item) => item.streamIndex === streamIndex ? { ...item, ...patch } : item),
    }));
  }

  function toggleTrackStream(type: MediaStreamInfo['type'], index: number, keep: boolean) {
    const scan = trackSnapshot.data;
    if (!scan) {
      return;
    }
    setTrackConversionDraft((current) => {
      if (type === 'subtitle' && keep && current.subtitleTransforms?.some((item) => item.streamIndex === index && item.removeEmbedded)) {
        const allIndexes = streamIndexesForType(scan, 'subtitle');
        const selected = conversionStreamIndexes(current, scan, 'subtitle');
        const next = normalizeNumberList([...selected, index]);
        const transforms = (current.subtitleTransforms ?? []).filter((item) => item.streamIndex !== index);
        return {
          ...current,
          keepSubtitleStreams: selectedOrUndefined(next, allIndexes),
          subtitleTransforms: transforms.length ? transforms : undefined,
        };
      }
      const allIndexes = streamIndexesForType(scan, type);
      const selected = conversionStreamIndexes(current, scan, type);
      const next = keep ? normalizeNumberList([...selected, index]) : selected.filter((candidate) => candidate !== index);
      const indexes = selectedOrUndefined(next, allIndexes);
      if (type === 'video') return { ...current, keepVideoStreams: indexes };
      if (type === 'audio') return { ...current, keepAudioStreams: indexes };
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
    const options = videoPreviewOptions(videoDraft);
    setProcessedVideoCodec(videoDraft.videoCodec);
    setProcessedVideoQualityValue(videoDraft.qualityValue);
    setProcessedVideoOptions(options);
    setVideoPreviewStatus('loading');
    setVideoPreviewNonce((current) => current + 1);
    if (assetPath) {
      fidelityInspection.mutate({
        reference: {
          path: assetPath,
          start,
          seconds,
          mode: 'quality',
          videoCodec: 'x264',
          qualityValue: 16,
          videoPreset: 'medium',
          pixFmt: 'yuv420p',
          videoEncoder: 'libx264',
          useHardwareIfAvailable: false,
          globalQuality: 21,
          previewNormalization,
        },
        conversion: {
          path: assetPath,
          start,
          seconds,
          videoCodec: videoDraft.videoCodec,
          qualityValue: videoDraft.qualityValue,
          mode: 'quality',
          previewNormalization,
          ...options,
        },
      });
    }
    scrollToPreviews();
  }

  function changePreviewColorDomain(next: 'preserve' | 'normalize_bt709') {
    setPreviewNormalization(next);
    if (!assetPath || previewNonce === 0 || videoPreviewNonce === 0) {
      return;
    }
    const options = videoPreviewOptions(videoDraft);
    fidelityInspection.mutate({
      reference: {
        path: assetPath,
        start,
        seconds,
        mode: 'quality',
        videoCodec: 'x264',
        qualityValue: 16,
        videoPreset: 'medium',
        pixFmt: 'yuv420p',
        videoEncoder: 'libx264',
        useHardwareIfAvailable: false,
        globalQuality: 21,
        previewNormalization: next,
      },
      conversion: {
        path: assetPath,
        start,
        seconds,
        videoCodec: videoDraft.videoCodec,
        qualityValue: videoDraft.qualityValue,
        mode: 'quality',
        previewNormalization: next,
        ...options,
      },
    });
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
    fidelityInspection.reset();
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

  const combinedTerminalCommand = buildCombinedLabFFmpegCommand({
    assetPath,
    start,
    seconds,
    video: videoDraft,
    audio: audioDraft,
    tracks: trackConversionDraft,
    scan: trackSnapshot.data,
    selectedAudioStreamIndex,
  });
  const sourceFrameOptions: CompatiblePreviewOptions = {
    path: assetPath,
    start,
    seconds,
    mode: 'quality',
    videoCodec: 'x264',
    qualityValue: 16,
    videoPreset: 'medium',
    pixFmt: 'yuv420p',
    videoEncoder: 'libx264',
    useHardwareIfAvailable: false,
    globalQuality: 21,
    previewNormalization,
  };
  const outputFrameOptions: CompatiblePreviewOptions = {
    path: assetPath,
    start,
    seconds,
    videoCodec: processedVideoCodec,
    qualityValue: processedVideoQualityValue,
    mode: 'quality',
    previewNormalization,
    ...processedVideoOptions,
  };
  const sourceFrameUrl = assetPath && videoPreviewNonce > 0
    ? `${api.compatibleAssetFrameUrl(sourceFrameOptions, 'source')}&nonce=${videoPreviewNonce}`
    : '';
  const outputFrameUrl = assetPath && videoPreviewNonce > 0
    ? `${api.compatibleAssetFrameUrl(outputFrameOptions, 'output')}&nonce=${videoPreviewNonce}`
    : '';

  async function copyCombinedTerminalCommand() {
    if (!combinedTerminalCommand) return;
    await copyTextToClipboard(combinedTerminalCommand);
    setTerminalCommandCopied(true);
    window.setTimeout(() => setTerminalCommandCopied(false), 1800);
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
              <Grid size={{ xs: 12, lg: 5 }}>
                <AssetAutocomplete
                  assets={labAssets}
                  value={selectedAsset}
                  onChange={(asset) => {
                    if (!savedTrackProfileKey) {
                      setTrackConversionDraft({});
                    }
                    setAssetPath(asset?.path ?? '');
                    setAudioPreviewStreamIndex(asset?.conversion?.enhancedAudioSourceStreamIndex ?? null);
                    resetProcessedPreviews();
                    setRecommendationReport(null);
                    setRecommendationSuggestion(null);
                    setRecommendationApplied({ video: false, audio: false, tracks: false });
                    setRecommendationOpen(false);
                  }}
                />
              </Grid>
              <Grid size={{ xs: 6, sm: 3, lg: 1.25 }}>
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
              <Grid size={{ xs: 3, sm: 2, lg: 0.75 }}>
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
              <Grid size={{ xs: 12, sm: 5, lg: 2 }}>
                <Stack direction="row" spacing={0.75}>
                  <Button
                    startIcon={<AutoFixHighIcon />}
                    variant="outlined"
                    size="small"
                    disabled={!assetPath || autoRecommendation.isPending}
                    onClick={() => autoRecommendation.mutate(assetPath)}
                    fullWidth
                    sx={{ minHeight: 40 }}
                  >
                    {autoRecommendation.isPending ? 'Processing…' : 'Process Asset'}
                  </Button>
                  {recommendationReport ? (
                    <Button size="small" variant="text" onClick={() => setRecommendationOpen(true)} sx={{ minWidth: 'auto', px: 1 }}>
                      {Object.values(recommendationApplied).some(Boolean) ? 'Applied' : 'View'}
                    </Button>
                  ) : null}
                </Stack>
              </Grid>
            </Grid>
            <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5} alignItems={{ xs: 'stretch', md: 'center' }} sx={{ mt: 1.5 }}>
              <TextField
                label="Preview color domain"
                size="small"
                value={previewNormalization}
                onChange={(event) => changePreviewColorDomain(event.target.value as 'preserve' | 'normalize_bt709')}
                select
                sx={{ width: { xs: '100%', md: 280 } }}
              >
                <MenuItem value="normalize_bt709">Canonical BT.709 · recommended</MenuItem>
                <MenuItem value="preserve">Preserve source characteristics</MenuItem>
              </TextField>
              <Typography variant="body2" color="text.secondary">
                {previewNormalization === 'normalize_bt709'
                  ? 'A and B receive the same mathematical color conversion before encoding. This affects LAB previews only; it is not saved into the final profile.'
                  : 'A and B retain the declared source color domain where their encoders support it. Browser rendering may still differ by codec or platform.'}
              </Typography>
            </Stack>
            {autoRecommendation.isError ? (
              <Alert severity="error" sx={{ mt: 1.5 }}>
                Auto recommendation failed: {autoRecommendation.error instanceof Error ? autoRecommendation.error.message : 'unknown error'}
              </Alert>
            ) : null}
          </CardContent>
        </Card>
        <Card sx={{ mb: 2 }}>
          <CardContent sx={{ py: 1.5, '&:last-child': { pb: 1.5 } }}>
            <Stack spacing={1.25}>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between">
                <Stack spacing={0.25}>
                  <Typography fontWeight={700}>Combined FFmpeg terminal command</Typography>
                  <Typography variant="body2" color="text.secondary">
                    Uses the current {seconds}s range and combines the Video, Audio, and Tracks drafts in one output.
                  </Typography>
                  <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap sx={{ pt: 0.5 }}>
                    <Chip size="small" label={`Video: ${videoDraft.name || 'Draft'}`} />
                    <Chip size="small" label={`Audio: ${audioDraft.name || 'Draft'}`} />
                    <Chip size="small" label={`Tracks: ${trackDraft.name || 'Draft'}`} />
                  </Stack>
                </Stack>
                <Stack direction="row" spacing={1}>
                  <Button size="small" variant="text" onClick={() => setTerminalCommandOpen((current) => !current)}>
                    {terminalCommandOpen ? 'Hide' : 'Show command'}
                  </Button>
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={<ContentCopyIcon />}
                    disabled={!combinedTerminalCommand}
                    onClick={copyCombinedTerminalCommand}
                  >
                    {terminalCommandCopied ? 'Copied' : 'Copy'}
                  </Button>
                </Stack>
              </Stack>
              <Collapse in={terminalCommandOpen}>
                <Stack spacing={1}>
                  <TextField
                    value={combinedTerminalCommand || 'Select an asset to generate the command.'}
                    multiline
                    minRows={8}
                    maxRows={18}
                    inputProps={{ readOnly: true, spellCheck: false }}
                    fullWidth
                    sx={{ '& textarea': { fontFamily: 'monospace', fontSize: '0.78rem', lineHeight: 1.45 } }}
                  />
                  <Alert severity="info">
                    The command uses the media path visible to the MVForge backend and writes a temporary MKV/MP4 under <code>/tmp</code>. Run it in an environment where that input path and FFmpeg are available. Hardware Auto uses the matching software encoder as a portable terminal fallback.
                  </Alert>
                  {trackConversionDraft.subtitleTransforms?.length ? (
                    <Alert severity="warning">
                      Subtitle sidecar extraction and OCR run as a pre-conversion pipeline phase and are not represented inside this single FFmpeg media command.
                    </Alert>
                  ) : null}
                </Stack>
              </Collapse>
            </Stack>
          </CardContent>
        </Card>
        <Dialog open={videoSaveReviewOpen} onClose={() => setVideoSaveReviewOpen(false)} fullWidth maxWidth="lg">
          <DialogTitle>Review Video Profile</DialogTitle>
          <DialogContent dividers>
            <VideoProfileSaveReview profile={videoDraft} source={trackSnapshot.data} asset={selectedAsset} previewNormalization={previewNormalization} />
            {videoNameConflict ? <Alert severity="error" sx={{ mt: 2 }}>A video profile named “{videoDraft.name.trim()}” already exists. Choose a different name.</Alert> : null}
            {saveVideoProfileMutation.isError ? <Alert severity="error" sx={{ mt: 2 }}>{saveVideoProfileMutation.error instanceof Error ? saveVideoProfileMutation.error.message : 'Video profile could not be saved.'}</Alert> : null}
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setVideoSaveReviewOpen(false)}>Continue editing</Button>
            <Button variant="contained" startIcon={<SaveIcon />} disabled={saveVideoProfileMutation.isPending || videoNameConflict} onClick={confirmSaveVideoProfile}>
              {saveVideoProfileMutation.isPending ? 'Saving…' : savedVideoProfileId ? 'Update profile' : 'Create profile'}
            </Button>
          </DialogActions>
        </Dialog>
        <Dialog open={audioSaveReviewOpen} onClose={() => !updateSetting.isPending && setAudioSaveReviewOpen(false)} fullWidth maxWidth="lg">
          <DialogTitle>Review Audio Profile</DialogTitle>
          <DialogContent dividers>
            <AudioProfileSaveReview profile={normalizedAudioProfileForSave(audioDraft, audioFilterChainEdited, previewAudioFilters)} source={trackSnapshot.data} asset={selectedAsset} />
            {audioNameConflict ? <Alert severity="error" sx={{ mt: 2 }}>An audio profile named “{audioDraft.name.trim()}” already exists. Choose a different name.</Alert> : null}
            {updateSetting.isError ? <Alert severity="error" sx={{ mt: 2 }}>{updateSetting.error instanceof Error ? updateSetting.error.message : 'Audio profile could not be saved.'}</Alert> : null}
          </DialogContent>
          <DialogActions>
            <Button disabled={updateSetting.isPending} onClick={() => setAudioSaveReviewOpen(false)}>Continue editing</Button>
            <Button variant="contained" startIcon={<SaveIcon />} disabled={updateSetting.isPending || audioNameConflict} onClick={confirmSaveAudioProfile}>
              {updateSetting.isPending ? 'Saving…' : savedAudioProfileKey ? 'Update profile' : 'Create profile'}
            </Button>
          </DialogActions>
        </Dialog>
        <Dialog open={trackSaveReviewOpen} onClose={() => { if (!updateSetting.isPending) { setTrackSaveReviewOpen(false); setPendingTrackProfileSave(null); } }} fullWidth maxWidth="lg">
          <DialogTitle>Review Track Profile</DialogTitle>
          <DialogContent dividers>
            {pendingTrackProfileSave ? <TrackProfileSaveReview profile={pendingTrackProfileSave} conversion={trackProfileOverride(pendingTrackProfileSave)} source={trackSnapshot.data} asset={selectedAsset} /> : null}
            {trackNameConflict ? <Alert severity="error" sx={{ mt: 2 }}>A track profile named “{trackDraft.name.trim()}” already exists. Choose a different name.</Alert> : null}
            {updateSetting.isError ? <Alert severity="error" sx={{ mt: 2 }}>{updateSetting.error instanceof Error ? updateSetting.error.message : 'Track profile could not be saved.'}</Alert> : null}
          </DialogContent>
          <DialogActions>
            <Button disabled={updateSetting.isPending} onClick={() => { setTrackSaveReviewOpen(false); setPendingTrackProfileSave(null); }}>Continue editing</Button>
            <Button variant="contained" startIcon={<SaveIcon />} disabled={updateSetting.isPending || trackNameConflict || !pendingTrackProfileSave} onClick={confirmSaveTrackProfile}>
              {updateSetting.isPending ? 'Saving…' : savedTrackProfileKey ? 'Update profile' : 'Create profile'}
            </Button>
          </DialogActions>
        </Dialog>
        <Dialog open={recommendationOpen && Boolean(recommendationReport)} onClose={() => setRecommendationOpen(false)} fullWidth maxWidth="md">
          <DialogTitle>MVForge Suggestions</DialogTitle>
          <DialogContent dividers>
            {recommendationReport ? (
              <LabRecommendationDetails
                report={recommendationReport}
                selected={recommendationSelected}
                onToggle={(section, checked) => setRecommendationSelected((current) => ({ ...current, [section]: checked }))}
              />
            ) : null}
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setRecommendationOpen(false)}>Open without suggestions</Button>
            <Button
              variant="contained"
              disabled={!recommendationSuggestion || !Object.values(recommendationSelected).some(Boolean)}
              onClick={() => {
                if (!recommendationSuggestion) return;
                (Object.keys(recommendationSelected) as LabSection[]).forEach((section) => {
                  if (recommendationSelected[section]) applyAutoRecommendation(recommendationSuggestion, section);
                });
                setRecommendationOpen(false);
              }}
            >
              Apply suggestions
            </Button>
          </DialogActions>
        </Dialog>
        <Box ref={previewsRef} sx={{ scrollMarginTop: 16, mb: 2 }}>
          <Stack spacing={2}>
              {assetPath && previewNonce > 0 ? (
                <Alert severity="info">
                  Sample A is a browser-compatible reference generated from the source; it is not the original stream. Sample B uses the same segment and the current profile draft. The fidelity validation below reports what FFmpeg actually produced.
                </Alert>
              ) : null}
              {assetPath && previewNonce > 0 ? (
                <Card variant="outlined">
                  <CardContent sx={{ py: 0, '&:last-child': { pb: 0 } }}>
                    <Tabs value={fidelityTab} onChange={(_, value) => setFidelityTab(value)} variant="scrollable" scrollButtons="auto">
                      <Tab value="previews" label="A/B Previews" />
                      <Tab value="frames" label="Frame Fidelity" />
                      <Tab value="characteristics" label="Characteristics" />
                    </Tabs>
                  </CardContent>
                </Card>
              ) : null}
              {assetPath && previewNonce > 0 && fidelityTab === 'previews' ? (
                <Grid container spacing={2} alignItems="stretch">
                  <Grid size={{ xs: 12, lg: 6 }}>
                    <SampleCard
                      title="Source reference"
                      subtitle="Temporary H.264/AAC representation for browser playback"
                    >
                      <Stack spacing={2}>
                        <VideoPreview
                          key={`original-compatible-${previewNonce}`}
                          label="Source reference · browser-compatible H.264"
                          src={api.compatibleAssetPreviewUrl({
                            path: assetPath,
                            start,
                            seconds,
                            mode: 'quality',
                            videoCodec: 'x264',
                            qualityValue: 16,
                            videoPreset: 'medium',
                            pixFmt: 'yuv420p',
                            videoEncoder: 'libx264',
                            useHardwareIfAvailable: false,
                            globalQuality: 21,
                            previewNormalization,
                          })}
                        />
                        <AudioPreview
                          label="Source audio reference · AAC stereo 192 kbps"
                          src={api.audioPreviewUrl({ path: assetPath, start, seconds, compatibility: true, streamIndex: selectedAudioStreamIndex })}
                        />
                      </Stack>
                    </SampleCard>
                  </Grid>
                  <Grid size={{ xs: 12, lg: 6 }}>
                    <SampleCard title="Profile result" subtitle="Current video and audio profile drafts">
                      <Stack spacing={2}>
                        {videoPreviewNonce > 0 ? (
                          <VideoPreview
                            key={`draft-${videoPreviewNonce}`}
                            label="Video draft"
                            src={api.compatibleAssetPreviewUrl({
                              path: assetPath,
                              start,
                              seconds,
                              videoCodec: processedVideoCodec,
                              qualityValue: processedVideoQualityValue,
                              mode: 'quality',
                              previewNormalization,
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
                            src={`${api.audioPreviewUrl({ path: assetPath, start, seconds, filters: processedAudioFilters, streamIndex: selectedAudioStreamIndex })}&nonce=${audioPreviewNonce}`}
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
              {assetPath && previewNonce > 0 && fidelityTab !== 'previews' ? (
                <Card variant="outlined">
                  <CardContent>
                    <Stack spacing={1.5}>
                      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} justifyContent="space-between" alignItems={{ xs: 'flex-start', sm: 'center' }}>
                        <Box>
                          <Typography variant="h3">Fidelity validation</Typography>
                          <Typography variant="body2" color="text.secondary">
                            Source, browser reference, and profile result are probed separately after generation.
                          </Typography>
                        </Box>
                        <Button size="small" onClick={() => setFidelityOpen((current) => !current)} endIcon={<ExpandMoreIcon />}>
                          {fidelityOpen ? 'Hide' : 'Show'}
                        </Button>
                      </Stack>
                      {fidelityInspection.isPending ? <Alert severity="info">Generating both previews and validating their video characteristics…</Alert> : null}
                      <Stack direction="row" spacing={1} alignItems="center" flexWrap="wrap">
                        <Button size="small" variant="outlined" disabled={profileSampleEstimate.isPending || !assetPath} onClick={() => profileSampleEstimate.mutate({ path: assetPath, profileId: savedVideoProfileId ?? 0, profile: videoDraft, seconds: 20 })}>Measure 5 samples</Button>
                        {profileSampleEstimate.isPending ? <Typography variant="body2" color="text.secondary">Encoding five distributed samples…</Typography> : null}
                        {currentProfileSampleEstimate ? <Chip size="small" color={currentProfileSampleEstimate.persisted ? 'success' : 'warning'} label={`Video estimate ${formatBytes(currentProfileSampleEstimate.estimatedVideoBytes)} · ${currentProfileSampleEstimate.effectiveEncoder}${currentProfileSampleEstimate.persisted ? ' · saved for Queue' : ' · profile changed or is not saved'}`} /> : null}
                      </Stack>
                      {profileSampleEstimate.isError && profileSampleEstimate.variables?.path === assetPath ? <Alert severity="warning">Sample estimate failed: {profileSampleEstimate.error instanceof Error ? profileSampleEstimate.error.message : 'unknown error'}</Alert> : null}
                      {fidelityInspection.isError && !isCanceledFidelityRequest(fidelityInspection.error) ? (
                        <Alert severity="warning">
                          Fidelity validation failed: {fidelityInspection.error instanceof Error ? fidelityInspection.error.message : 'unknown backend error'}
                        </Alert>
                      ) : null}
                      <Collapse in={fidelityOpen}>
                        {currentFidelityInspection ? (
                          fidelityTab === 'frames' ? (
                            <FrameFidelityComparison sourceUrl={sourceFrameUrl} outputUrl={outputFrameUrl} inspection={currentFidelityInspection} />
                          ) : (
                            <FidelityCharacteristicsTable inspection={currentFidelityInspection} />
                          )
                        ) : (
                          !fidelityInspection.isPending
                            ? <Alert severity="info">Process Video to generate and validate the current profile result.</Alert>
                            : null
                        )}
                      </Collapse>
                    </Stack>
                  </CardContent>
                </Card>
              ) : null}
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
                        disabled={!videoDraft.name || videoNameConflict || saveVideoProfileMutation.isPending}
                        onClick={saveVideoProfile}
                        sx={{ minHeight: 32 }}
                      >
                        {savedVideoProfileId ? 'Update Profile' : 'Save Profile'}
                      </Button>
                    </Stack>
                  </Stack>
                  <Stack spacing={1}>
                    {videoPreviewStatus === 'ready' && !videoPreviewStale ? (
                      <Alert severity="success">Video sample ready in Sample B.</Alert>
                    ) : null}
                    {videoPreviewStale ? (
                      <Alert severity="info">Video settings changed. Process video again to refresh Sample B.</Alert>
                    ) : null}
                    {videoPreviewStatus === 'error' ? (
                      <Alert severity="warning">Video sample could not be processed.</Alert>
                    ) : null}
                    {saveVideoProfileMutation.isSuccess && videoProfileSaveMessage ? (
                      <Alert severity="success">{videoProfileSaveMessage}</Alert>
                    ) : null}
                    {saveVideoProfileMutation.isError ? (
                      <Alert severity="error">{saveVideoProfileMutation.error instanceof Error ? saveVideoProfileMutation.error.message : 'Video profile could not be saved.'}</Alert>
                    ) : null}
                  </Stack>
                  <Alert severity="info">
                    <strong>copy</strong> keeps the original stream untouched. Use it to preserve quality; choose x264/x265 or another codec when
                    you need smaller files, compatibility, or filters.
                  </Alert>
                  <Grid container spacing={2}>
                    <Grid size={{ xs: 12, lg: 6 }}>
                      <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, height: '100%', bgcolor: 'rgba(255,255,255,0.018)' }}>
                        <Stack spacing={2}>
                          <Typography fontWeight={700}>Profile information</Typography>
                          <TextField label="Starter preset" value={selectedVideoStarterPreset} onChange={(event) => applyStarterVideoPreset(event.target.value)} select fullWidth>
                            <MenuItem value="" disabled>Choose a video preset</MenuItem>
                            {videoStarterPresets.map((preset) => (
                              <MenuItem key={preset.key} value={preset.key}>{preset.label}</MenuItem>
                            ))}
                          </TextField>
                          <VideoProfileAutocomplete profiles={profiles.data ?? []} onChange={(profile) => profile ? selectVideoProfile(profile.id) : undefined} />
                          <TextField label="New video profile name" value={videoDraft.name} onChange={(event) => setVideoDraft({ ...videoDraft, name: event.target.value })} error={videoNameConflict} helperText={videoNameConflict ? 'A video profile with this name already exists.' : ' '} fullWidth />
                          <Grid container spacing={1}>
                            <Grid size={{ xs: 12, sm: 6 }}>
                              <FormControlLabel
                                control={<Checkbox checked={videoDraft.preserveHdr} onChange={(event) => setVideoDraft((current) => ({ ...current, preserveHdr: event.target.checked }))} />}
                                label="Preserve HDR"
                              />
                            </Grid>
                            <Grid size={{ xs: 12, sm: 6 }}>
                              <FormControlLabel
                                control={<Checkbox checked={videoDraft.preserveChapters} onChange={(event) => setVideoDraft((current) => ({ ...current, preserveChapters: event.target.checked }))} />}
                                label="Preserve chapters"
                              />
                            </Grid>
                          </Grid>
                          <TextField
                            select
                            fullWidth
                            label="Convert all subtitles"
                            value={videoExternalSubtitleFormat(videoDraft)}
                            onChange={(event) => updateVideoSubtitleExternalization(setVideoDraft, event.target.value)}
                            helperText="Creates validated sidecars and removes every embedded subtitle. A selected Tracks Profile takes priority."
                          >
                            <MenuItem value="disabled">Disabled · defer to Tracks Profile</MenuItem>
                            <MenuItem value="source">Keep embedded subtitle tracks</MenuItem>
                            <MenuItem value="srt">External SRT · remove embedded tracks</MenuItem>
                            <MenuItem value="ass">External ASS · remove embedded tracks</MenuItem>
                            <MenuItem value="remove">Remove embedded subtitle tracks</MenuItem>
                          </TextField>
                          {videoExternalSubtitleFormat(videoDraft) === 'srt' || videoExternalSubtitleFormat(videoDraft) === 'ass' ? (
                            <Grid container spacing={1.5}>
                              <Grid size={{ xs: 12, sm: 6 }}>
                                <TextField select fullWidth label="Bitmap OCR quality" value={videoWorkerValue(videoDraft, 'subtitleOCRMode', 'accurate')} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'subtitleOCRMode', event.target.value)} helperText="Accurate compares two traditional OCR passes.">
                                  <MenuItem value="raw">Raw · one pass</MenuItem><MenuItem value="clean">Clean · corrected</MenuItem><MenuItem value="accurate">Accurate · two passes</MenuItem>
                                </TextField>
                              </Grid>
                              <Grid size={{ xs: 12, sm: 6 }}>
                                <TextField select fullWidth label="Bitmap OCR language" value={videoWorkerValue(videoDraft, 'subtitleOCRLanguage', 'auto')} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'subtitleOCRLanguage', event.target.value)} helperText="Automatic reads each track language.">
                                  <MenuItem value="auto">Automatic per track</MenuItem><MenuItem value="eng">English</MenuItem><MenuItem value="spa">Spanish</MenuItem><MenuItem value="jpn">Japanese</MenuItem><MenuItem value="jpn_vert">Japanese vertical</MenuItem>
                                </TextField>
                              </Grid>
                            </Grid>
                          ) : null}
                        </Stack>
                      </Box>
                    </Grid>
                    <Grid size={{ xs: 12, lg: 6 }}>
                      <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, height: '100%', bgcolor: 'rgba(255,255,255,0.018)' }}>
                        <Stack spacing={2}>
                          <Typography fontWeight={700}>Technical settings</Typography>
                          <Grid container spacing={2}>
                            <Grid size={{ xs: 12, sm: 6 }}>
                              <TextField label="Video codec" value={displayVideoCodec(videoDraft.videoCodec)} onChange={(event) => updateVideoCodecDraft(setVideoDraft, event.target.value)} select fullWidth>
                                {videoCodecOptions
                                  .filter((codec) => videoWorkerValue(videoDraft, 'preferredEncoder', 'software') !== 'hardware' || hardwareEncodingSupportedForCodec(codec.value))
                                  .map((codec) => (
                                    <MenuItem key={codec.value} value={codec.value}>{codec.label}</MenuItem>
                                  ))}
                              </TextField>
                            </Grid>
                            <Grid size={{ xs: 12, sm: 6 }}>
                              <TextField
                                select
                                fullWidth
                                label="AAC source track"
                                value={numberWorkerValue(videoDraft, 'aacStereoSourceStreamIndex', -1)}
                                onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'aacStereoSourceStreamIndex', Number(event.target.value))}
                                disabled={!videoAACTrackEnabled(videoDraft)}
                                helperText="Automatic uses the selected default audio track; an asset override takes priority."
                              >
                                <MenuItem value={-1}>Automatic</MenuItem>
                                {availableAudioStreams.map((stream) => (
                                  <MenuItem key={stream.index} value={stream.index}>
                                    #{stream.index} · {stream.language || 'und'} · {stream.title || stream.codec || 'audio'}{stream.default ? ' · default' : ''}
                                  </MenuItem>
                                ))}
                              </TextField>
                            </Grid>
                            <Grid size={{ xs: 12, sm: 6 }}>
                              <TextField label="Container type" value={videoDraft.container} onChange={(event) => setVideoDraft({ ...videoDraft, container: event.target.value })} select fullWidth>
                                {['mkv', 'mp4'].map((container) => (
                                  <MenuItem key={container} value={container}>{container}</MenuItem>
                                ))}
                              </TextField>
                            </Grid>
                            <Grid size={{ xs: 12, sm: 6 }}>
                              <TextField
                                label="Encoding speed"
                                value={videoWorkerValue(videoDraft, 'videoPreset', 'medium')}
                                onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoPreset', event.target.value)}
                                helperText={videoWorkerValue(videoDraft, 'preferredEncoder', 'software') === 'software'
                                  ? encoderPresetDescription(videoWorkerValue(videoDraft, 'videoPreset', 'medium'))
                                  : 'Hardware-supported FFmpeg preset.'}
                                select
                                size="small"
                                fullWidth
                              >
                                {encoderPresetOptions.map((preset) => (
                                  <MenuItem key={preset.value} value={preset.value}>{preset.label}</MenuItem>
                                ))}
                              </TextField>
                            </Grid>
                            <Grid size={{ xs: 12, sm: 6 }}>
                              <FormControlLabel
                                control={<Checkbox checked={videoAACTrackEnabled(videoDraft)} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'addAacStereoTrack', event.target.checked)} />}
                                label="Add AAC stereo track"
                              />
                            </Grid>
                            <Grid size={{ xs: 12, sm: 6 }}>
                              <TextField
                                label="AAC compatibility quality"
                                value={numberWorkerValue(videoDraft, 'aacStereoBitrateKbps', 192)}
                                onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'aacStereoBitrateKbps', Number(event.target.value))}
                                disabled={!videoAACTrackEnabled(videoDraft)}
                                helperText="192 kb/s default"
                                select
                                fullWidth
                              >
                                {[128, 160, 192, 224, 256].map((value) => (
                                  <MenuItem key={value} value={value}>{value} kb/s</MenuItem>
                                ))}
                              </TextField>
                            </Grid>
                            <Grid size={{ xs: 12 }}>
                              <FormControlLabel
                                control={
                                  <Checkbox
                                    checked={videoWorkerBool(videoDraft, 'preserveOriginalAudio', true)}
                                    disabled={!videoAACTrackEnabled(videoDraft)}
                                    onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'preserveOriginalAudio', event.target.checked)}
                                  />
                                }
                                label="Original audio secondary"
                              />
                            </Grid>
                          </Grid>
                        </Stack>
                      </Box>
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, bgcolor: 'rgba(255,255,255,0.018)' }}>
                        <Grid container spacing={2} alignItems="flex-start">
                          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                            <TextField
                              label="Processing preference"
                              value={videoWorkerValue(videoDraft, 'preferredEncoder', 'software')}
                              onChange={(event) => updateVideoProcessingPreference(setVideoDraft, event.target.value as 'software' | 'hardware', defaultHardwareEncoder, trackSnapshot.data)}
                              helperText="Software follows Video Codec; Hardware exposes a validated hardware encoder."
                              select
                              size="small"
                              disabled={videoDraft.videoCodec === 'copy'}
                              fullWidth
                            >
                              <MenuItem value="software">Software · match Video Codec</MenuItem>
                              <MenuItem
                                value="hardware"
                                disabled={!hardwareEncodingSupportedForCodec(videoDraft.videoCodec) || runtimeSnapshot.isLoading || !hasUsableHardwareEncoder}
                              >
                                Hardware
                              </MenuItem>
                            </TextField>
                          </Grid>
                          {videoWorkerValue(videoDraft, 'preferredEncoder', 'software') === 'hardware' ? <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                            <TextField
                              label="Hardware encoder"
                              value={videoWorkerValue(videoDraft, 'videoEncoder', 'auto')}
                              onChange={(event) => updateVideoHardwareEncoder(setVideoDraft, event.target.value, trackSnapshot.data)}
                              helperText={selectedHardwareCapability && !selectedHardwareCapability.usable
                                ? selectedHardwareCapability.reason
                                : videoEncoderDescription(selectedHardwareEncoder)}
                              select
                              size="small"
                              fullWidth
                            >
                              {videoEncoderOptions.filter((encoder) => isHardwareEncoderOption(encoder.value)).map((encoder) => (
                                <MenuItem
                                  key={encoder.value}
                                  value={encoder.value}
                                  disabled={runtimeSnapshot.data?.encoders?.[encoder.value]?.usable === false}
                                >
                                  {encoder.label}
                                  {runtimeSnapshot.data?.encoders?.[encoder.value]?.usable === false ? ' · unavailable' : ''}
                                </MenuItem>
                              ))}
                            </TextField>
                          </Grid> : null}
                          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                            <TextField
                              label="Color depth"
                              value={videoWorkerValue(videoDraft, 'pixFmt', videoWorkerValue(videoDraft, 'preferredEncoder', 'software') === 'hardware' ? defaultHardwareMain10PixelFormat(selectedHardwareEncoder) : 'yuv420p10le')}
                              onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'pixFmt', event.target.value)}
                              helperText={pixelFormatDescription(videoWorkerValue(videoDraft, 'pixFmt', 'yuv420p10le'))}
                              select
                              size="small"
                              fullWidth
                            >
                              {compatiblePixelFormatOptions(videoWorkerValue(videoDraft, 'preferredEncoder', 'software'), selectedHardwareEncoder).map((pixFmt) => (
                                <MenuItem key={pixFmt.value} value={pixFmt.value}>{pixFmt.label}</MenuItem>
                              ))}
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                            <TextField
                              label="Final color policy"
                              value={videoWorkerValue(videoDraft, 'finalColorPolicy', 'preserve')}
                              onChange={(event) => {
                                const policy = event.target.value;
                                updateVideoWorkerConfig(setVideoDraft, 'finalColorPolicy', policy);
                                changePreviewColorDomain(policy === 'normalize_bt709' ? 'normalize_bt709' : 'preserve');
                              }}
                              helperText="No correction is applied unless Automatic or BT.709 normalization is selected."
                              select
                              size="small"
                              fullWidth
                            >
                              <MenuItem value="preserve">No correction · preserve source</MenuItem>
                              <MenuItem value="automatic">Automatic correction when justified</MenuItem>
                              <MenuItem value="normalize_bt709">Normalize mathematically to BT.709</MenuItem>
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                            <TextField
                              label={videoWorkerValue(videoDraft, 'preferredEncoder', 'software') === 'hardware'
                                ? 'Software fallback CRF'
                                : 'Software CRF'}
                              value={videoDraft.qualityValue}
                              onChange={(event) => {
                                const value = Number(event.target.value);
                                setVideoDraft((current) => ({
                                  ...current,
                                  qualityValue: Number.isFinite(value) ? Math.min(30, Math.max(14, value)) : current.qualityValue,
                                }));
                              }}
                              type="number"
                              inputProps={{ min: 14, max: 30, step: 1 }}
                              disabled={videoDraft.videoCodec === 'copy'}
                              helperText={videoWorkerValue(videoDraft, 'preferredEncoder', 'software') === 'hardware'
                                ? 'Only used if hardware falls back to software.'
                                : 'Lower preserves more detail; higher creates smaller files.'}
                              size="small"
                              fullWidth
                            />
                          </Grid>
                          {videoWorkerValue(videoDraft, 'preferredEncoder', 'software') === 'hardware' ? (
                            <Grid size={{ xs: 12 }}>
                              {selectedHardwareCapability && !selectedHardwareCapability.usable ? (
                                <Alert severity="warning">
                                  {selectedHardwareEncoder} is listed by FFmpeg but failed its real encoding smoke test. MVForge will use the matching software encoder instead.
                                </Alert>
                              ) : null}
                            </Grid>
                          ) : null}
                          {videoWorkerValue(videoDraft, 'preferredEncoder', 'software') === 'hardware' && hardwareUsesGlobalQuality(selectedHardwareEncoder) ? <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                            <TextField
                              label={selectedHardwareEncoder === 'hevc_qsv' ? 'QSV quality (ICQ)' : 'Hardware quality'}
                              value={numberWorkerValue(videoDraft, 'globalQuality', qsvQualityRangeForCrf(videoDraft.qualityValue || 20).recommended)}
                              onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'globalQuality', Number(event.target.value))}
                              type="number"
                              inputProps={{ min: 15, max: 35 }}
                              helperText={hardwareQualityHelper(videoDraft.qualityValue)}
                              size="small"
                              fullWidth
                            />
                          </Grid> : null}
                          {videoWorkerValue(videoDraft, 'preferredEncoder', 'software') === 'hardware' && videoWorkerValue(videoDraft, 'videoEncoder', 'auto') === 'hevc_qsv' ? (
                            <>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                <TextField
                                  label="QSV rate control"
                                  value={videoWorkerValue(videoDraft, 'qsvRateControl', 'icq')}
                                  onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'qsvRateControl', event.target.value)}
                                  helperText="LA-ICQ falls back to ICQ unless the active worker probe confirms Look Ahead."
                                  select
                                  size="small"
                                  fullWidth
                                >
                                  <MenuItem value="icq">ICQ · safe default</MenuItem>
                                <MenuItem value="la_icq" disabled={!qsvLookAheadAvailable}>LA-ICQ · Main10 capability required</MenuItem>
                                </TextField>
                              </Grid>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                <TextField
                                  label="QSV look-ahead depth"
                                  type="number"
                                  value={numberWorkerValue(videoDraft, 'qsvLookAheadDepth', 40)}
                                  onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'qsvLookAheadDepth', Number(event.target.value))}
                                  inputProps={{ min: 10, max: 100 }} disabled={!qsvLookAheadAvailable}
                                  size="small"
                                  fullWidth
                                />
                              </Grid>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField label="Quality preset" select value={videoWorkerValue(videoDraft, 'hardwareQualityPreset', 'recommended')} onChange={(event) => applyHardwareQualityPreset(setVideoDraft, event.target.value, trackSnapshot.data)} size="small" fullWidth>{hardwareQualityPresetOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}</TextField></Grid>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                <FormControlLabel
                                  control={<Checkbox disabled={!qsvAdvancedAvailable} checked={videoWorkerBool(videoDraft, 'qsvExtendedBRC')} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'qsvExtendedBRC', event.target.checked)} />}
                                  label="Extended BRC"
                                />
                              </Grid>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                <Stack>
                                  <FormControlLabel
                                    control={<Checkbox disabled={!qsvAdvancedAvailable} checked={videoWorkerBool(videoDraft, 'qsvAdaptiveI')} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'qsvAdaptiveI', event.target.checked)} />}
                                    label="Adaptive I"
                                  />
                                  <FormControlLabel
                                    control={<Checkbox disabled={!qsvAdvancedAvailable} checked={videoWorkerBool(videoDraft, 'qsvAdaptiveB')} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'qsvAdaptiveB', event.target.checked)} />}
                                    label="Adaptive B"
                                  />
                                </Stack>
                              </Grid>
                            </>
                          ) : null}
                          {videoWorkerValue(videoDraft, 'preferredEncoder', 'software') === 'hardware' && videoWorkerValue(videoDraft, 'videoEncoder', 'auto') === 'hevc_videotoolbox' ? (
                            <>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField label="Quality preset" select value={videoWorkerValue(videoDraft, 'hardwareQualityPreset', 'recommended')} onChange={(event) => applyHardwareQualityPreset(setVideoDraft, event.target.value, trackSnapshot.data)} size="small" fullWidth>{hardwareQualityPresetOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}</TextField></Grid>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                <TextField label="Custom target bitrate (Mbps)" type="number" value={numberWorkerValue(videoDraft, 'videoToolboxBitrateMbps', 6)} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoToolboxBitrateMbps', Number(event.target.value))} inputProps={{ min: 1, max: 200 }} helperText="Named presets adapt to the selected asset. Editing uses Custom." size="small" fullWidth />
                              </Grid>
                              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                <TextField label="Custom maxrate (Mbps)" type="number" value={numberWorkerValue(videoDraft, 'videoToolboxMaxrateMbps', 8)} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoToolboxMaxrateMbps', Number(event.target.value))} inputProps={{ min: 1, max: 250 }} size="small" fullWidth />
                              </Grid>
                                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                                  <TextField label="Custom buffer (Mbps)" type="number" value={numberWorkerValue(videoDraft, 'videoToolboxBufferMbps', 12)} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoToolboxBufferMbps', Number(event.target.value))} inputProps={{ min: 1, max: 500 }} size="small" fullWidth />
                                </Grid>
                                <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField title="HEVC Main is 8-bit; Main10 is 10-bit and requires a compatible pixel format." label="Profile" value={videoWorkerValue(videoDraft, 'videoToolboxProfile', '')} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoToolboxProfile', event.target.value)} placeholder="main or main10" helperText="Blank follows bit depth" size="small" fullWidth /></Grid>
                                <Grid size={{ xs: 12, sm: 6, md: 3 }}><TextField title="Maximum distance between keyframes. Smaller values improve seeking but increase size." label="GOP" type="number" value={numberWorkerValue(videoDraft, 'videoToolboxGop', 0)} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoToolboxGop', Number(event.target.value))} inputProps={{ min: 0, max: 1000 }} helperText="0 = automatic" size="small" fullWidth /></Grid>
                                <Grid size={{ xs: 12 }}><Stack direction="row" spacing={2} flexWrap="wrap"><FormControlLabel title="Available only after the matching VideoToolbox Main/Main10 B-frame probe succeeds." control={<Checkbox disabled={!videoToolboxBFramesAvailable} checked={videoWorkerBool(videoDraft, 'videoToolboxAllowFrameReordering')} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoToolboxAllowFrameReordering', event.target.checked)} />} label="Allow frame reordering" /><FormControlLabel title="Available only after the matching VideoToolbox Main/Main10 power-efficiency probe succeeds." control={<Checkbox disabled={!videoToolboxPowerAvailable} checked={videoWorkerBool(videoDraft, 'videoToolboxPowerEfficiency')} onChange={(event) => updateVideoWorkerConfig(setVideoDraft, 'videoToolboxPowerEfficiency', event.target.checked)} />} label="Power efficiency" /></Stack></Grid>
                              </>
                          ) : null}
                        </Grid>
                      </Box>
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <Grid container spacing={2}>
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
                                <Grid size={{ xs: 12, lg: 4 }}>
                                  <Stack spacing={1.5}>
                                    <Typography fontWeight={700} variant="body2">Cleanup filters</Typography>
                                    <Grid container spacing={1.5}>
                                <Grid size={{ xs: 12, sm: 6 }}>
                                  <TextField
                                    label="Deinterlace"
                                    value={videoFilterControlValue(videoDraft, 'deinterlaceMode', 'auto')}
                                    onChange={(event) => updateVideoFilterControl(setVideoDraft, 'deinterlaceMode', event.target.value)}
                                    helperText={videoFilterControlValue(videoDraft, 'deinterlaceMode', 'auto') === 'force'
                                      ? 'FFmpeg: bwdif=mode=send_frame:parity=auto:deint=all · output metadata becomes progressive automatically.'
                                      : 'Output field metadata follows the analyzed correction automatically.'}
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
                                <Grid size={{ xs: 12, sm: 6 }}>
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
                                <Grid size={{ xs: 12, sm: 6 }}>
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
                                <Grid size={{ xs: 12, sm: 6 }}>
                                  <TextField
                                    label="Deflicker"
                                    value={videoFilterControlValue(videoDraft, 'deflicker', 'off')}
                                    onChange={(event) => updateVideoFilterControl(setVideoDraft, 'deflicker', event.target.value)}
                                    helperText="Reduces frame-to-frame brightness variation in old transfers."
                                    select
                                    fullWidth
                                  >
                                    {deflickerOptions.map((option) => (
                                      <MenuItem key={option.value} value={option.value}>
                                        {option.label}
                                      </MenuItem>
                                    ))}
                                  </TextField>
                                </Grid>
                                <Grid size={{ xs: 12, sm: 6 }}>
                                  <TextField
                                    label="Deblock"
                                    value={videoFilterControlValue(videoDraft, 'deblockFilter', 'off')}
                                    onChange={(event) => updateVideoFilterControl(setVideoDraft, 'deblockFilter', event.target.value)}
                                    helperText="Softens visible MPEG/DVD block boundaries. Preview before saving."
                                    select
                                    fullWidth
                                  >
                                    {deblockOptions.map((option) => (
                                      <MenuItem key={option.value} value={option.value}>
                                        {option.label}
                                      </MenuItem>
                                    ))}
                                  </TextField>
                                </Grid>
                                <Grid size={{ xs: 12, sm: 6 }}>
                                  <TextField
                                    label="Unsharp"
                                    value={videoFilterControlValue(videoDraft, 'unsharp', 'off')}
                                    onChange={(event) => updateVideoFilterControl(setVideoDraft, 'unsharp', event.target.value)}
                                    helperText="Restores luma edge definition after denoise."
                                    select
                                    fullWidth
                                  >
                                    {unsharpOptions.map((option) => (
                                      <MenuItem key={option.value} value={option.value}>
                                        {option.label}
                                      </MenuItem>
                                    ))}
                                  </TextField>
                                </Grid>
                                <Grid size={{ xs: 12 }}>
                                  <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'center' }}>
                                    <FormControlLabel
                                      control={
                                        <Checkbox
                                          checked={videoFilterControlValue(videoDraft, 'crop', 'off') === 'manual'}
                                          onChange={(event) => updateVideoFilterControl(setVideoDraft, 'crop', event.target.checked ? 'manual' : 'off')}
                                        />
                                      }
                                      label="Enable crop"
                                      sx={{ minWidth: 150, m: 0 }}
                                    />
                                    <TextField
                                      label={videoWorkerValue(videoDraft, 'cropValue') ? 'Suggested crop' : 'Manual crop'}
                                      value={videoWorkerValue(videoDraft, 'cropValue')}
                                      onChange={(event) => updateVideoFilterControl(setVideoDraft, 'cropValue', event.target.value)}
                                      placeholder="iw:ih-80:0:40"
                                      helperText={
                                        videoFilterControlValue(videoDraft, 'crop', 'off') === 'manual'
                                          ? 'Enabled for Sample B and the saved profile.'
                                          : 'Saved as a suggestion, but not applied until enabled.'
                                      }
                                      size="small"
                                      fullWidth
                                    />
                                  </Stack>
                                </Grid>
                                    </Grid>
                                  </Stack>
                                </Grid>
                                <Grid size={{ xs: 12, lg: 8 }}>
                                  <Stack spacing={1.5}>
                                    <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={1}>
                                      <Typography fontWeight={700} variant="body2">Image adjustments</Typography>
                                      <Button
                                        size="small"
                                        variant="outlined"
                                        onClick={() => resetImageAdjustmentControls(setVideoDraft)}
                                      >
                                        Reset image controls
                                      </Button>
                                    </Stack>
                                    <Box
                                      sx={{
                                        display: 'grid',
                                        gridTemplateColumns: { xs: 'repeat(5, minmax(64px, 1fr))', md: 'repeat(10, minmax(64px, 1fr))' },
                                        gap: 0.75,
                                        minWidth: 0,
                                      }}
                                    >
                                      <ImageAdjustmentSlider
                                    label="Exposure"
                                    tooltip="Adjust overall exposure in stops. Keep changes small."
                                    value={numberWorkerValue(videoDraft, 'exposure', 0)}
                                    min={-200}
                                    max={200}
                                    step={5}
                                    scaleLabels={['+2', '0', '-2']}
                                    valueLabel={(value) => `${value > 0 ? '+' : ''}${(value / 100).toFixed(1)} EV`}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'exposure', String(value))}
                                  />
                                      <ImageAdjustmentSlider
                                    label="Brightness"
                                    tooltip="Small luma correction for sources that are too dark or washed out."
                                    value={numberWorkerValue(videoDraft, 'brightness', 0)}
                                    min={-100}
                                    max={100}
                                    valueLabel={(value) => (value === 0 ? 'Neutral' : `${value > 0 ? '+' : ''}${value}%`)}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'brightness', String(value))}
                                  />
                                  <ImageAdjustmentSlider
                                    label="Contrast"
                                    tooltip="Adjust image separation. Keep changes subtle for animation."
                                    value={numberWorkerValue(videoDraft, 'contrast', 100)}
                                    min={50}
                                    max={150}
                                    valueLabel={(value) => `${value}%`}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'contrast', String(value))}
                                  />
                                  <ImageAdjustmentSlider
                                    label="Saturation"
                                    tooltip="Color intensity correction. Useful for faded sources."
                                    value={numberWorkerValue(videoDraft, 'saturation', 100)}
                                    min={50}
                                    max={150}
                                    valueLabel={(value) => `${value}%`}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'saturation', String(value))}
                                  />
                                  <ImageAdjustmentSlider
                                    label="Vibrance"
                                    tooltip="Boosts weaker colors more selectively than saturation."
                                    value={numberWorkerValue(videoDraft, 'vibrance', 0)}
                                    min={-100}
                                    max={100}
                                    valueLabel={(value) => (value === 0 ? 'Neutral' : `${value > 0 ? '+' : ''}${value}%`)}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'vibrance', String(value))}
                                  />
                                  <ImageAdjustmentSlider
                                    label="Gamma"
                                    tooltip="Midtone correction. Helps sources that look flat, too dark, or too bright without moving black/white as much."
                                    value={numberWorkerValue(videoDraft, 'gamma', 100)}
                                    min={70}
                                    max={130}
                                    valueLabel={(value) => `${value}%`}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'gamma', String(value))}
                                  />
                                  <ImageAdjustmentSlider
                                    label="Temperature"
                                    tooltip="Warmer/cooler correction. Positive warms, negative cools."
                                    value={numberWorkerValue(videoDraft, 'temperature', 0)}
                                    min={-100}
                                    max={100}
                                    valueLabel={(value) => (value === 0 ? 'Neutral' : `${value > 0 ? '+' : ''}${value}%`)}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'temperature', String(value))}
                                  />
                                  <ImageAdjustmentSlider
                                    label="Tint"
                                    tooltip="Green/magenta correction. Useful for color casts after old transfers."
                                    value={numberWorkerValue(videoDraft, 'tint', 0)}
                                    min={-100}
                                    max={100}
                                    valueLabel={(value) => (value === 0 ? 'Neutral' : `${value > 0 ? '+' : ''}${value}%`)}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'tint', String(value))}
                                  />
                                  <ImageAdjustmentSlider
                                    label="Black point"
                                    tooltip="Raises the input black point to deepen washed-out blacks."
                                    value={numberWorkerValue(videoDraft, 'blackPoint', 0)}
                                    min={0}
                                    max={10}
                                    step={1}
                                    valueLabel={(value) => `${value}%`}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'blackPoint', String(value))}
                                  />
                                  <ImageAdjustmentSlider
                                    label="White point"
                                    tooltip="Lowers the input white point to recover contrast in dull highlights."
                                    value={numberWorkerValue(videoDraft, 'whitePoint', 100)}
                                    min={90}
                                    max={100}
                                    step={1}
                                    valueLabel={(value) => `${value}%`}
                                    onChange={(value) => updateVideoFilterControl(setVideoDraft, 'whitePoint', String(value))}
                                  />
                                    </Box>
                                  </Stack>
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
                        disabled={!audioDraft.name || audioNameConflict || updateSetting.isPending}
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
                      <TextField label="New audio profile name" value={audioDraft.name} onChange={(event) => setAudioDraft({ ...audioDraft, name: event.target.value, key: slugify(event.target.value) })} error={audioNameConflict} helperText={audioNameConflict ? 'An audio profile with this name already exists.' : ' '} size="small" fullWidth />
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
                      <Button
                        size="small"
                        variant="outlined"
                        disabled={!assetPath || trackSnapshot.isPending}
                        onClick={() => scanTrackAsset(Boolean(trackSnapshot.data))}
                        sx={{ minHeight: 32 }}
                      >
                        {trackSnapshot.data ? 'Rescan' : 'Scan'}
                      </Button>
                      <Button
                        startIcon={<SaveIcon />}
                        variant="contained"
                        size="small"
                        disabled={!trackDraft.name || trackNameConflict || updateSetting.isPending}
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
                        profiles={allTrackProfiles}
                        selectedKey={savedTrackProfileKey}
                        onChange={selectTrackProfile}
                      />
                    </Grid>
                    <Grid size={{ xs: 12, md: 4 }}>
                      <TextField
                        label={savedTrackProfileKey ? 'Track profile name' : 'New track profile name'}
                        value={trackDraft.name}
                        onChange={(event) => setTrackDraft({ ...trackDraft, name: event.target.value, key: slugify(event.target.value) })}
                        error={trackNameConflict}
                        helperText={trackNameConflict ? 'A track profile with this name already exists.' : ' '}
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
                        {trackSnapshot.data?.subtitleStreams.length ? (
                          <Stack spacing={1.25}>
                            <Stack>
                              <Typography fontWeight={700}>External subtitle transformations</Typography>
                              <Typography color="text.secondary" variant="body2">Create validated sidecars and remove the selected embedded tracks. Bitmap tracks run OCR before FFmpeg.</Typography>
                            </Stack>
                            {trackSnapshot.data.subtitleStreams.map((stream) => {
                              const transform = trackConversionDraft.subtitleTransforms?.find((item) => item.streamIndex === stream.index);
                              const bitmap = isBitmapSubtitleStream(stream);
                              return <Box key={stream.index} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.5 }}>
                                <Stack spacing={1.25}>
                                  <Typography fontWeight={700}>#{stream.index} · {(stream.language || 'und').toUpperCase()} · {stream.codec.toUpperCase()}</Typography>
                                  <Grid container spacing={1.25}>
                                    <Grid size={{ xs: 12, md: bitmap ? 3 : 6 }}><TextField select fullWidth label="Subtitle action" value={transform?.format ?? ''} onChange={(event) => updateTrackSubtitleTransform(stream, event.target.value as '' | 'srt' | 'ass')}><MenuItem value="">Keep embedded track</MenuItem><MenuItem value="srt">Create SRT and remove embedded</MenuItem><MenuItem value="ass">Create ASS and remove embedded</MenuItem></TextField></Grid>
                                    {transform ? <Grid size={{ xs: 12, md: bitmap ? 3 : 6 }}><TextField fullWidth label="Sidecar language" value={transform.language} onChange={(event) => updateTrackSubtitleTransformValue(stream.index, { language: event.target.value.toLowerCase() })} /></Grid> : null}
                                    {transform && bitmap ? <>
                                      <Grid size={{ xs: 12, md: 3 }}><TextField select fullWidth label="OCR quality" value={transform.ocrMode || 'accurate'} onChange={(event) => updateTrackSubtitleTransformValue(stream.index, { ocrMode: event.target.value as 'raw' | 'clean' | 'accurate' })}><MenuItem value="raw">Raw</MenuItem><MenuItem value="clean">Clean</MenuItem><MenuItem value="accurate">Accurate</MenuItem></TextField></Grid>
                                      <Grid size={{ xs: 12, md: 3 }}><TextField select fullWidth label="OCR language" value={transform.ocrLanguage || defaultTrackOCRLanguage(stream.language)} onChange={(event) => updateTrackSubtitleTransformValue(stream.index, { ocrLanguage: event.target.value })}><MenuItem value="eng">English</MenuItem><MenuItem value="spa">Spanish</MenuItem><MenuItem value="jpn">Japanese</MenuItem><MenuItem value="jpn_vert">Japanese vertical</MenuItem></TextField></Grid>
                                    </> : null}
                                  </Grid>
                                </Stack>
                              </Box>;
                            })}
                          </Stack>
                        ) : null}
                      </Stack>
                    </Grid>
                    <Grid size={{ xs: 12 }}>
                      <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2 }}>
                        <Stack spacing={2}>
                          <Stack><Typography fontWeight={700}>Reusable selection rules</Typography><Typography color="text.secondary" variant="body2">Saved fallback rules used when stream indexes differ on another asset.</Typography></Stack>
                          <Grid container spacing={1.5}>
                            <Grid size={{ xs: 12, md: 4 }}><TextField select fullWidth label="Video rule" value={trackDraft.videoMode} onChange={(event) => setTrackDraft({ ...trackDraft, videoMode: event.target.value as TrackProfile['videoMode'] })}><MenuItem value="first">Keep first video</MenuItem><MenuItem value="all">Keep all video</MenuItem><MenuItem value="require-one">Require one video</MenuItem></TextField></Grid>
                            <Grid size={{ xs: 12, md: 4 }}><TextField select fullWidth label="Audio rule" value={trackDraft.audioMode} onChange={(event) => setTrackDraft({ ...trackDraft, audioMode: event.target.value as TrackProfile['audioMode'] })}><MenuItem value="all">Keep all audio</MenuItem><MenuItem value="default">Keep default audio</MenuItem><MenuItem value="languages">Keep selected languages</MenuItem><MenuItem value="none">Remove all audio</MenuItem></TextField></Grid>
                            <Grid size={{ xs: 12, md: 4 }}><TextField select fullWidth label="Subtitle rule" value={trackDraft.subtitleMode} onChange={(event) => setTrackDraft({ ...trackDraft, subtitleMode: event.target.value as TrackProfile['subtitleMode'] })}><MenuItem value="all">Keep all subtitles</MenuItem><MenuItem value="none">Remove all subtitles</MenuItem><MenuItem value="forced">Forced only</MenuItem><MenuItem value="languages">Selected languages</MenuItem><MenuItem value="forced-or-languages">Forced or selected languages</MenuItem></TextField></Grid>
                            <Grid size={{ xs: 12, md: 6 }}><Autocomplete multiple freeSolo options={languageOptions.map((option) => option.value)} value={trackDraft.audioLanguages} onChange={(_, values) => setTrackDraft({ ...trackDraft, audioLanguages: normalizeStringList(values) })} disabled={trackDraft.audioMode !== 'languages'} renderInput={(params) => <TextField {...params} label="Audio languages" helperText="Select or enter ISO language codes." />} /></Grid>
                            <Grid size={{ xs: 12, md: 6 }}><Autocomplete multiple freeSolo options={languageOptions.map((option) => option.value)} value={trackDraft.subtitleLanguages} onChange={(_, values) => setTrackDraft({ ...trackDraft, subtitleLanguages: normalizeStringList(values) })} disabled={trackDraft.subtitleMode !== 'languages' && trackDraft.subtitleMode !== 'forced-or-languages'} renderInput={(params) => <TextField {...params} label="Subtitle languages" helperText="Select or enter ISO language codes." />} /></Grid>
                            <Grid size={{ xs: 12, md: 6 }}><TextField fullWidth label="Default audio language" value={trackDraft.defaultAudioLanguage} onChange={(event) => setTrackDraft({ ...trackDraft, defaultAudioLanguage: event.target.value.toLowerCase() })} /></Grid>
                            <Grid size={{ xs: 12, md: 6 }}><TextField fullWidth label="Default subtitle language" value={trackDraft.defaultSubtitleLanguage} onChange={(event) => setTrackDraft({ ...trackDraft, defaultSubtitleLanguage: event.target.value.toLowerCase() })} /></Grid>
                            <Grid size={{ xs: 12 }}><Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} flexWrap="wrap" useFlexGap><FormControlLabel control={<Checkbox checked={trackDraft.audioRequired} onChange={(event) => setTrackDraft({ ...trackDraft, audioRequired: event.target.checked })} />} label="Require matching audio" /><FormControlLabel control={<Checkbox checked={trackDraft.subtitlesRequired} onChange={(event) => setTrackDraft({ ...trackDraft, subtitlesRequired: event.target.checked })} />} label="Require matching subtitles" /><FormControlLabel control={<Checkbox checked={trackDraft.dropCommentary} onChange={(event) => setTrackDraft({ ...trackDraft, dropCommentary: event.target.checked })} />} label="Remove commentary tracks" /></Stack></Grid>
                          </Grid>
                        </Stack>
                      </Box>
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

function FrameFidelityComparison({ sourceUrl, outputUrl, inspection }: { sourceUrl: string; outputUrl: string; inspection: LabFidelityInspection }) {
  const [mode, setMode] = useState<'slider' | 'side-by-side' | 'blink'>('slider');
  const [position, setPosition] = useState(50);
  const [blinkSource, setBlinkSource] = useState(true);
  const [loadError, setLoadError] = useState('');

  useEffect(() => {
    setLoadError('');
  }, [sourceUrl, outputUrl]);

  useEffect(() => {
    if (mode !== 'blink') {
      setBlinkSource(true);
      return;
    }
    const timer = window.setInterval(() => setBlinkSource((current) => !current), 650);
    return () => window.clearInterval(timer);
  }, [mode]);

  if (!sourceUrl || !outputUrl) {
    return <Alert severity="info">Process Video to create the canonical source and decoded output frames.</Alert>;
  }

  const frameStyle = {
    display: 'block',
    width: '100%',
    height: '100%',
    objectFit: 'contain' as const,
    bgcolor: 'black',
  };
  const sourceDimensions = squarePixelDimensions(inspection.source);
  const outputDimensions = squarePixelDimensions(inspection.conversion);
  const dimensionsMatch = sourceDimensions === outputDimensions;

  return (
    <Stack spacing={1.5}>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} justifyContent="space-between" alignItems={{ xs: 'stretch', sm: 'center' }}>
        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
          <Button size="small" variant={mode === 'slider' ? 'contained' : 'outlined'} onClick={() => setMode('slider')}>Before / after</Button>
          <Button size="small" variant={mode === 'side-by-side' ? 'contained' : 'outlined'} onClick={() => setMode('side-by-side')}>Side by side</Button>
          <Button size="small" variant={mode === 'blink' ? 'contained' : 'outlined'} onClick={() => setMode('blink')}>Blink</Button>
        </Stack>
        <Typography variant="caption" color="text.secondary">
          PNG frames decoded by FFmpeg · square-pixel display geometry
        </Typography>
      </Stack>
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
        <Chip size="small" label={`Source frame · ${sourceDimensions}`} />
        <Chip size="small" label={`Result frame · ${outputDimensions}`} />
        <Chip size="small" color={dimensionsMatch ? 'success' : 'warning'} label={dimensionsMatch ? 'Geometry aligned' : 'Geometry differs'} />
        {inspection.metrics.comparable ? <Chip size="small" color="info" label={`SSIM ${inspection.metrics.ssim?.toFixed(4)}`} /> : null}
        {inspection.metrics.comparable ? <Chip size="small" color="info" label={`PSNR ${inspection.metrics.psnr?.toFixed(2)} dB`} /> : null}
      </Stack>
      {!dimensionsMatch ? (
        <Alert severity="warning">The displayed frame dimensions differ. Overlay and future pixel metrics are not considered geometrically equivalent.</Alert>
      ) : null}
      <Typography variant="caption" color="text.secondary">{inspection.metrics.reason}</Typography>
      {loadError ? <Alert severity="warning">{loadError}</Alert> : null}
      {mode === 'side-by-side' ? (
        <Grid container spacing={1.5}>
          <Grid size={{ xs: 12, md: 6 }}>
            <FrameImage label="Canonical source" src={sourceUrl} onError={() => setLoadError('The canonical source PNG could not be generated.')} />
          </Grid>
          <Grid size={{ xs: 12, md: 6 }}>
            <FrameImage label="Decoded profile result" src={outputUrl} onError={() => setLoadError('The profile-result PNG could not be generated.')} />
          </Grid>
        </Grid>
      ) : (
        <Box>
          <Box sx={{ position: 'relative', width: '100%', height: { xs: 280, md: 520 }, overflow: 'hidden', bgcolor: 'black', borderRadius: 1 }}>
            <Box component="img" src={outputUrl} alt="Decoded profile result frame" onError={() => setLoadError('The profile-result PNG could not be generated.')} sx={frameStyle} />
            {mode === 'slider' ? (
              <>
                <Box
                  component="img"
                  src={sourceUrl}
                  alt="Canonical source frame"
                  onError={() => setLoadError('The canonical source PNG could not be generated.')}
                  sx={{ ...frameStyle, position: 'absolute', inset: 0, clipPath: `inset(0 ${100 - position}% 0 0)` }}
                />
                <Box sx={{ position: 'absolute', top: 0, bottom: 0, left: `${position}%`, width: 2, bgcolor: 'primary.main', transform: 'translateX(-1px)' }} />
                <Chip size="small" label="Canonical source" sx={{ position: 'absolute', left: 12, top: 12 }} />
                <Chip size="small" label="Profile result" sx={{ position: 'absolute', right: 12, top: 12 }} />
              </>
            ) : (
              <>
                <Box
                  component="img"
                  src={sourceUrl}
                  alt="Canonical source frame"
                  onError={() => setLoadError('The canonical source PNG could not be generated.')}
                  sx={{ ...frameStyle, position: 'absolute', inset: 0, opacity: blinkSource ? 1 : 0 }}
                />
                <Chip size="small" color={blinkSource ? 'info' : 'warning'} label={blinkSource ? 'Canonical source' : 'Profile result'} sx={{ position: 'absolute', left: 12, top: 12 }} />
              </>
            )}
          </Box>
          {mode === 'slider' ? (
            <Slider value={position} min={0} max={100} onChange={(_, value) => setPosition(Array.isArray(value) ? value[0] : value)} aria-label="Before and after frame position" />
          ) : null}
        </Box>
      )}
      <Alert severity="info">
        Frame Fidelity bypasses browser video decoding. The source frame receives the selected preview color policy; the result frame is decoded from the generated profile preview.
      </Alert>
    </Stack>
  );
}

function squarePixelDimensions(characteristics: PreviewVideoCharacteristics) {
  const parts = (characteristics.sampleAspectRatio || '1:1').split(':').map(Number);
  const ratio = parts.length === 2 && parts[0] > 0 && parts[1] > 0 ? parts[0] / parts[1] : 1;
  return `${Math.round(characteristics.width * ratio)}×${characteristics.height}`;
}

function FrameImage({ label, src, onError }: { label: string; src: string; onError: () => void }) {
  return (
    <Stack spacing={0.75}>
      <Typography fontWeight={700} variant="body2">{label}</Typography>
      <Box component="img" src={src} alt={`${label} frame`} onError={onError} sx={{ display: 'block', width: '100%', height: { xs: 260, md: 420 }, objectFit: 'contain', bgcolor: 'black', borderRadius: 1 }} />
    </Stack>
  );
}

function FidelityCharacteristicsTable({ inspection }: { inspection: LabFidelityInspection }) {
  const rows: Array<{
    label: string;
    value: (item: PreviewVideoCharacteristics) => string;
    expectedToChange?: boolean;
  }> = [
    { label: 'Codec', value: (item) => item.codec || 'Unknown', expectedToChange: true },
    { label: 'Profile', value: (item) => item.profile || 'Unknown', expectedToChange: true },
    { label: 'Pixel format', value: (item) => item.pixelFormat || 'Unknown', expectedToChange: true },
    { label: 'Bit depth', value: (item) => item.bitDepth ? `${item.bitDepth}-bit` : 'Unknown', expectedToChange: true },
    { label: 'Frame size', value: (item) => item.width && item.height ? `${item.width}×${item.height}` : 'Unknown' },
    { label: 'SAR', value: (item) => item.sampleAspectRatio || 'Unknown' },
    { label: 'DAR', value: (item) => item.displayAspectRatio || 'Unknown' },
    { label: 'Frame rate', value: (item) => item.frameRate || 'Unknown' },
    { label: 'Field order', value: (item) => item.fieldOrder || 'Unknown', expectedToChange: true },
    { label: 'Range', value: (item) => item.colorRange || 'Unknown', expectedToChange: inspection.normalization.applied },
    { label: 'Matrix', value: (item) => item.colorSpace || 'Unknown', expectedToChange: inspection.normalization.applied },
    { label: 'Transfer', value: (item) => item.colorTransfer || 'Unknown', expectedToChange: inspection.normalization.applied },
    { label: 'Primaries', value: (item) => item.colorPrimaries || 'Unknown', expectedToChange: inspection.normalization.applied },
    { label: 'Chroma location', value: (item) => item.chromaLocation || 'Unknown' },
  ];
  const unexpected = rows.filter((row) => {
    if (row.expectedToChange) return false;
    const source = row.value(inspection.source);
    const conversion = row.value(inspection.conversion);
    return source !== 'Unknown' && conversion !== 'Unknown' && source !== conversion;
  });
  const encoderFallback = Boolean(
    inspection.requestedEncoder &&
    inspection.requestedEncoder !== 'auto' &&
    inspection.effectiveEncoder &&
    inspection.requestedEncoder !== inspection.effectiveEncoder,
  );

  return (
    <Stack spacing={1.5}>
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
        <Chip size="small" label={`Source · ${inspection.source.codec || 'unknown'} ${inspection.source.width}×${inspection.source.height}`} />
        <Chip
          size="small"
          color={inspection.normalization.applied ? 'info' : 'default'}
          label={inspection.normalization.applied ? 'Preview · BT.709 converted' : `Preview · ${inspection.normalization.mode === 'preserve' ? 'source domain' : 'normalization skipped'}`}
        />
        <Chip size="small" color="info" label={`Browser reference · ${inspection.reference.codec || 'unknown'} ${inspection.reference.pixelFormat || ''}`} />
        <Chip size="small" color={encoderFallback ? 'warning' : 'success'} label={`Encoder · ${inspection.effectiveEncoder || 'unknown'}`} />
        <Chip size="small" color={unexpected.length ? 'warning' : 'success'} label={`Profile result · ${unexpected.length ? `${unexpected.length} unexpected difference(s)` : 'structure preserved'}`} />
      </Stack>
      {encoderFallback ? (
        <Alert severity="warning">
          Requested {inspection.requestedEncoder}, but the runtime smoke test rejected it. This preview was actually encoded with {inspection.effectiveEncoder}.
        </Alert>
      ) : null}
      <Alert severity={inspection.normalization.applied || inspection.normalization.mode === 'preserve' ? 'info' : 'warning'}>
        {inspection.normalization.reason}
        {inspection.normalization.filter ? <><br /><code>{inspection.normalization.filter}</code></> : null}
      </Alert>
      {inspection.normalization.aliasWarnings?.map((warning) => (
        <Alert key={warning} severity="warning">{warning}</Alert>
      ))}
      {unexpected.length ? (
        <Alert severity="warning">
          Unexpected output differences were found in: {unexpected.map((row) => row.label).join(', ')}. Codec, profile, pixel format, bit depth, and field order may change intentionally.
        </Alert>
      ) : (
        <Alert severity="success">No unexpected geometry or color metadata differences were detected in the generated profile preview.</Alert>
      )}
      <Box sx={{ overflowX: 'auto' }}>
        <Table size="small" aria-label="Preview fidelity characteristics">
          <TableHead>
            <TableRow>
              <TableCell>Characteristic</TableCell>
              <TableCell>Source</TableCell>
              <TableCell>Browser reference</TableCell>
              <TableCell>Profile result</TableCell>
              <TableCell>Result</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            <TableRow>
              <TableCell>Encoder</TableCell>
              <TableCell>Source stream</TableCell>
              <TableCell>{inspection.referenceEncoder || 'libx264'}</TableCell>
              <TableCell>
                <Stack spacing={0.25}>
                  <Typography variant="body2">{inspection.effectiveEncoder || 'Unknown'}</Typography>
                  <Typography variant="caption" color="text.secondary">
                    Requested: {inspection.requestedEncoder || 'auto'}
                  </Typography>
                </Stack>
              </TableCell>
              <TableCell>
                <Chip
                  size="small"
                  color={encoderFallback ? 'warning' : 'success'}
                  label={encoderFallback ? 'Software fallback' : inspection.requestedEncoder === 'auto' ? 'Selected automatically' : 'Requested encoder used'}
                />
              </TableCell>
            </TableRow>
            {rows.map((row) => {
              const source = row.value(inspection.source);
              const reference = row.value(inspection.reference);
              const conversion = row.value(inspection.conversion);
              const unknown = source === 'Unknown' || conversion === 'Unknown';
              const equal = source === conversion;
              const status = unknown ? 'Unknown' : equal ? 'Preserved' : row.expectedToChange ? 'Changed intentionally' : 'Changed unexpectedly';
              const color = status === 'Changed unexpectedly' ? 'warning' : status === 'Preserved' ? 'success' : 'default';
              return (
                <TableRow key={row.label}>
                  <TableCell>{row.label}</TableCell>
                  <TableCell>{source}</TableCell>
                  <TableCell>{reference}</TableCell>
                  <TableCell>{conversion}</TableCell>
                  <TableCell><Chip size="small" color={color} label={status} /></TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      </Box>
      <Typography variant="caption" color="text.secondary">
        The browser reference is a temporary H.264 representation. Its differences describe preview normalization only and do not change the final-output policy saved in the video profile.
      </Typography>
    </Stack>
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

function ImageAdjustmentSlider({
  label,
  tooltip,
  value,
  min,
  max,
  step = 1,
  scaleLabels,
  valueLabel,
  onChange,
}: {
  label: string;
  tooltip: string;
  value: number;
  min: number;
  max: number;
  step?: number;
  scaleLabels?: [string, string, string];
  valueLabel: (value: number) => string;
  onChange: (value: number) => void;
}) {
  return (
    <Tooltip title={tooltip}>
      <Stack
        spacing={1}
        alignItems="center"
        sx={{
          width: '100%',
          minWidth: 0,
          border: 1,
          borderColor: 'divider',
          borderRadius: 1,
          p: 1,
          bgcolor: 'rgba(255,255,255,0.02)',
          flexShrink: 0,
        }}
      >
        <Typography fontWeight={700} variant="caption" noWrap sx={{ maxWidth: '100%' }}>{label}</Typography>
        <Chip
          label={valueLabel(value)}
          size="small"
          sx={{ maxWidth: '100%', height: 22, '& .MuiChip-label': { px: 0.75, fontSize: 11 } }}
        />
        <Stack direction="row" spacing={0.5} alignItems="center" sx={{ height: 190 }}>
          <Stack justifyContent="space-between" sx={{ height: '100%' }}>
            <Typography color="text.secondary" variant="caption">{scaleLabels?.[0] ?? max}</Typography>
            <Typography color="text.secondary" variant="caption">{scaleLabels?.[1] ?? Math.round((min + max) / 2)}</Typography>
            <Typography color="text.secondary" variant="caption">{scaleLabels?.[2] ?? min}</Typography>
          </Stack>
          <Slider
            value={value}
            min={min}
            max={max}
            step={step}
            orientation="vertical"
            sx={{ height: 170 }}
            onChange={(_, nextValue) => onChange(Array.isArray(nextValue) ? nextValue[0] : nextValue)}
          />
        </Stack>
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
    globalQuality: numberWorkerValue(draft, 'globalQuality', qsvQualityRangeForCrf(draft.qualityValue || 20).recommended),
    qsvRateControl: videoWorkerValue(draft, 'qsvRateControl', 'icq'),
    qsvLookAheadDepth: numberWorkerValue(draft, 'qsvLookAheadDepth', 40),
    qsvExtendedBRC: videoWorkerBool(draft, 'qsvExtendedBRC'),
    qsvAdaptiveI: videoWorkerBool(draft, 'qsvAdaptiveI'),
    qsvAdaptiveB: videoWorkerBool(draft, 'qsvAdaptiveB'),
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
  const pixelFormat = videoWorkerValue(draft, 'pixFmt').toLowerCase();
  if (pixelFormat === 'nv12' || pixelFormat === 'yuv420p') return false;
  if (pixelFormat.includes('10') || pixelFormat.includes('p010')) return true;
  return (draft.bitDepth ?? 0) >= 10 || draft.videoCodec.includes('10bit');
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

function videoExternalSubtitleFormat(draft: ProfileInput) {
  const configured = videoWorkerValue(draft, 'externalSubtitleFormat').toLowerCase();
  if (configured === 'disabled' || configured === 'srt' || configured === 'ass' || configured === 'source' || configured === 'remove') {
    return configured;
  }
  // Treat the previous embedded-conversion setting as the new safe
  // externalization behavior when an older profile is opened in LAB.
  const legacy = videoSubtitleOutputFormat(draft);
  return legacy === 'srt' || legacy === 'ass' ? legacy : 'source';
}

function updateVideoSubtitleExternalization(
  setVideoDraft: Dispatch<SetStateAction<ProfileInput>>,
  format: string,
) {
  const normalized = format === 'disabled' || format === 'srt' || format === 'ass' || format === 'remove' ? format : 'source';
  setVideoDraft((current) => ({
    ...current,
    preserveSubtitles: normalized === 'disabled' || normalized === 'source',
    workerConfig: {
      ...current.workerConfig,
      externalSubtitleFormat: normalized,
      // Externalization is handled before the media command. Keep FFmpeg from
      // independently transcoding the remaining subtitle streams.
      subtitleOutputFormat: 'source',
      preferSrtSubtitles: false,
    },
  }));
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
  setVideoDraft((current) => {
    const pixelFormat = key === 'pixFmt' && typeof value === 'string' ? value.toLowerCase() : '';
    const explicitBitDepth = pixelFormat === 'nv12' || pixelFormat === 'yuv420p'
      ? 8
      : pixelFormat.includes('10') || pixelFormat.includes('p010')
        ? 10
        : undefined;
    const next = {
      ...current,
      ...(key === 'pixFmt' ? {
        videoCodec: displayVideoCodec(current.videoCodec),
        bitDepth: explicitBitDepth ?? 0,
        pixelFormat: typeof value === 'string' ? value : current.pixelFormat,
      } : {}),
      workerConfig: {
        ...current.workerConfig,
        [key]: value,
        ...(['globalQuality', 'qsvRateControl', 'qsvLookAheadDepth', 'qsvExtendedBRC', 'qsvAdaptiveI', 'qsvAdaptiveB', 'videoToolboxBitrateMbps', 'videoToolboxMaxrateMbps', 'videoToolboxBufferMbps', 'videoToolboxProfile', 'videoToolboxGop', 'videoToolboxRealtime', 'videoToolboxAllowFrameReordering', 'videoToolboxPowerEfficiency', 'pixFmt'].includes(key) ? { hardwareQualityPreset: 'custom' } : {}),
      },
    };
    return key === 'videoEncoder' || key === 'pixFmt'
      ? synchronizeLabAuthoritativeContract(next)
      : next;
  });
}

function updateVideoProcessingPreference(
  setVideoDraft: Dispatch<SetStateAction<ProfileInput>>,
  preference: 'software' | 'hardware',
  defaultHardwareEncoder = '',
  scan?: ScanResult,
) {
  setVideoDraft((current) => {
    const hardware = preference === 'hardware';
    const hardwareAvailable = hardwareEncodingSupportedForCodec(current.videoCodec);
    const currentEncoder = videoWorkerValue(current, 'videoEncoder', 'auto');
    const hardwareEncoder = isHardwareEncoderOption(currentEncoder) ? currentEncoder : defaultHardwareEncoder;
    const pixelFormat = hardware
      ? defaultHardwareMain10PixelFormat(hardwareEncoder)
      : 'yuv420p10le';
    const baseWorkerConfig = {
      ...current.workerConfig,
      preferredEncoder: preference,
      useHardwareIfAvailable: hardware && hardwareAvailable,
      videoEncoder: preference === 'software'
        ? 'auto'
        : hardwareEncoder,
      pixFmt: pixelFormat,
    };
    const workerConfig = hardware
      ? applySharedHardwareQualityPreset(baseWorkerConfig, hardwareEncoder, 'recommended', scan)
      : baseWorkerConfig;
    return synchronizeLabAuthoritativeContract({
      ...current,
      workerConfig: hardware ? workerConfig : {
        ...workerConfig,
        qsvRateControl: undefined,
        qsvLookAheadDepth: undefined,
        qsvExtendedBRC: undefined,
        qsvAdaptiveI: undefined,
        qsvAdaptiveB: undefined,
      },
    });
  });
}

function updateVideoHardwareEncoder(
  setVideoDraft: Dispatch<SetStateAction<ProfileInput>>,
  encoder: string,
  scan?: ScanResult,
) {
  setVideoDraft((current) => synchronizeLabAuthoritativeContract({
    ...current,
    workerConfig: applySharedHardwareQualityPreset({
      ...current.workerConfig,
      preferredEncoder: 'hardware',
      useHardwareIfAvailable: true,
      videoEncoder: encoder,
      pixFmt: defaultHardwareMain10PixelFormat(encoder),
    }, encoder, 'recommended', scan),
  }));
}

function applyHardwareQualityPreset(setter: Dispatch<SetStateAction<ProfileInput>>, preset: string, scan?: ScanResult) {
  setter((current) => {
    const encoder = videoWorkerValue(current, 'videoEncoder', 'auto');
    const workerConfig = applySharedHardwareQualityPreset(current.workerConfig ?? {}, encoder, preset, scan);
    // pixFmt is part of a hardware preset. Keep the top-level Color depth and
    // Pixel format fields in lockstep with it instead of leaving LAB in an
    // impossible Main/Main10 state until the user touches another control.
    return synchronizeLabAuthoritativeContract({
      ...current,
      workerConfig,
    });
  });
}

function updateVideoCodecDraft(
  setVideoDraft: Dispatch<SetStateAction<ProfileInput>>,
  videoCodec: string,
) {
  setVideoDraft((current) => {
    const preference = videoWorkerValue(current, 'preferredEncoder', 'software');
    const mustUseSoftware = videoCodec === 'copy' || preference === 'hardware' && !hardwareEncodingSupportedForCodec(videoCodec);
    const workerConfig = mustUseSoftware
      ? { ...current.workerConfig, preferredEncoder: 'software', useHardwareIfAvailable: false, videoEncoder: 'auto' }
      : current.workerConfig;
    return synchronizeLabAuthoritativeContract({ ...current, videoCodec, workerConfig });
  });
}

function encoderPresetDescription(value: string) {
  return encoderPresetOptions.find((option) => option.value === value)?.description ?? 'Controls how much time FFmpeg spends compressing video.';
}

function pixelFormatDescription(value: string) {
  return pixelFormatOptions.find((option) => option.value === value)?.description ?? 'Controls output color depth and playback compatibility.';
}

function compatiblePixelFormatOptions(preference: string, encoder: string) {
  if (preference !== 'hardware') {
    return pixelFormatOptions.filter((option) => ['auto', 'yuv420p10le', 'yuv420p'].includes(option.value));
  }
  if (encoder === 'hevc_qsv' || encoder === 'hevc_vaapi' || encoder === 'auto') {
    return pixelFormatOptions.filter((option) => ['auto', 'p010le', 'nv12'].includes(option.value));
  }
  if (encoder === 'hevc_videotoolbox') {
    return pixelFormatOptions.filter((option) => ['auto', 'p010le', 'yuv420p'].includes(option.value));
  }
  return pixelFormatOptions.filter((option) => ['auto', 'yuv420p10le', 'yuv420p'].includes(option.value));
}

function defaultHardwareMain10PixelFormat(encoder: string) {
  return encoder === 'hevc_qsv' || encoder === 'hevc_vaapi' || encoder === 'hevc_videotoolbox' ? 'p010le' : 'yuv420p10le';
}

function hardwareUsesGlobalQuality(encoder: string) {
  return encoder !== 'hevc_videotoolbox';
}

function videoEncoderDescription(value: string) {
  return videoEncoderOptions.find((option) => option.value === value)?.description ?? 'Controls whether the worker uses hardware HEVC or software x265.';
}

function displayVideoCodec(value: string) {
  return value.toLowerCase().includes('x265') || value.toLowerCase().includes('hevc') || value.toLowerCase().includes('h265') ? 'x265' : value;
}

function isHardwareEncoderOption(value: string) {
  return ['hevc_qsv', 'hevc_vaapi', 'hevc_nvenc', 'hevc_videotoolbox', 'hevc_amf'].includes(value);
}

function hardwareEncodingSupportedForCodec(codec: string) {
  return codec.toLowerCase().includes('265') || codec.toLowerCase().includes('hevc');
}

function synchronizeLabAuthoritativeContract(profile: ProfileInput): ProfileInput {
  const workerConfig = { ...profile.workerConfig };
  delete workerConfig.processingMode;
  profile = { ...profile, workerConfig };
  const codec = profile.videoCodec.toLowerCase();
  const codecFamily = codec === 'copy'
    ? 'copy'
    : codec.includes('265') || codec.includes('hevc')
      ? 'hevc'
      : codec.includes('264')
        ? 'h264'
        : codec.includes('av1')
          ? 'av1'
          : codec;
  if (codecFamily === 'copy') {
    return {
      ...profile,
      codecFamily: 'copy',
      encoderPolicy: 'locked',
      preferredEncoder: 'copy',
      allowedEncoders: ['copy'],
      fallbackPolicy: 'wait',
      bitDepth: 0,
      pixelFormat: '',
      qualityStrategy: 'source',
    };
  }
  const software = codecFamily === 'h264' ? 'libx264' : codecFamily === 'av1' ? 'libsvtav1' : 'libx265';
  const configured = videoWorkerValue(profile, 'videoEncoder', 'auto');
  const hardwareAllowed = videoWorkerBool(profile, 'useHardwareIfAvailable') && codecFamily === 'hevc';
  const hardware = hardwareAllowed ? ['hevc_qsv', 'hevc_vaapi', 'hevc_nvenc', 'hevc_videotoolbox', 'hevc_amf'] : [];
  const pixelFormat = videoWorkerValue(profile, 'pixFmt', profile.pixelFormat || 'yuv420p');
  const bitDepth = pixelFormat === 'auto' ? 0 : pixelFormat.includes('10') || pixelFormat.includes('p010') ? 10 : 8;
  if (configured === 'auto' || configured === 'ffmpeg') {
    return {
      ...profile,
      codecFamily,
      encoderPolicy: hardwareAllowed ? 'automatic' : 'locked',
      preferredEncoder: hardwareAllowed ? hardware[0] : software,
      allowedEncoders: hardwareAllowed ? [...hardware, software] : [software],
      fallbackPolicy: hardwareAllowed ? 'allowed_only' : 'wait',
      bitDepth,
      pixelFormat,
      qualityStrategy: 'crf',
    };
  }
  const validHardware = hardware.includes(configured);
  return {
    ...profile,
    codecFamily,
    encoderPolicy: validHardware ? 'restricted' : 'locked',
    preferredEncoder: validHardware ? configured : software,
    allowedEncoders: validHardware ? [configured, software] : [software],
    fallbackPolicy: validHardware ? 'allowed_only' : 'wait',
    bitDepth,
    pixelFormat,
    qualityStrategy: 'crf',
  };
}

function hardwareQualityHelper(softwareCRF: number) {
  return qsvQualityHelper(softwareCRF);
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

function resetImageAdjustmentControls(setVideoDraft: Dispatch<SetStateAction<ProfileInput>>) {
  setVideoDraft((current) => {
    const workerConfig = {
      ...current.workerConfig,
      exposure: 0,
      brightness: 0,
      contrast: 100,
      saturation: 100,
      vibrance: 0,
      gamma: 100,
      temperature: 0,
      tint: 0,
      blackPoint: 0,
      whitePoint: 100,
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
  const correctProgressiveFieldMetadata = workerConfig.correctProgressiveFieldMetadata === true;
  const denoise = stringValue(workerConfig.denoise, 'off');
  const deflicker = stringValue(workerConfig.deflicker, 'off');
  const deblockFilter = stringValue(workerConfig.deblockFilter, 'off');
  const unsharp = stringValue(workerConfig.unsharp, 'off');
  const deband = stringValue(workerConfig.deband, 'off');
  const crop = stringValue(workerConfig.crop, 'off');
  const cropValue = stringValue(workerConfig.cropValue, '');
  const brightness = numericWorkerConfigValue(workerConfig, 'brightness', 0);
  const contrast = numericWorkerConfigValue(workerConfig, 'contrast', 100);
  const saturation = numericWorkerConfigValue(workerConfig, 'saturation', 100);
  const gamma = numericWorkerConfigValue(workerConfig, 'gamma', 100);
  const temperature = numericWorkerConfigValue(workerConfig, 'temperature', 0);
  const tint = numericWorkerConfigValue(workerConfig, 'tint', 0);
  const exposure = numericWorkerConfigValue(workerConfig, 'exposure', 0);
  const vibrance = numericWorkerConfigValue(workerConfig, 'vibrance', 0);
  const blackPoint = numericWorkerConfigValue(workerConfig, 'blackPoint', 0);
  const whitePoint = numericWorkerConfigValue(workerConfig, 'whitePoint', 100);

  if (deinterlaceMode === 'force') {
    filters.push('bwdif=mode=send_frame:parity=auto:deint=all');
    filters.push('setfield=prog');
  } else if (deinterlaceMode === 'ivtc_tff') {
    filters.push('fieldmatch=order=tff,decimate');
    filters.push('setfield=prog');
  } else if (deinterlaceMode === 'ivtc_bff') {
    filters.push('fieldmatch=order=bff,decimate');
    filters.push('setfield=prog');
  } else if (correctProgressiveFieldMetadata) {
    filters.push('setfield=prog');
  }
  if (deflicker === 'light') {
    filters.push('deflicker=size=5:mode=pm');
  } else if (deflicker === 'medium') {
    filters.push('deflicker=size=9:mode=pm');
  }
  if (deblockFilter === 'light') {
    filters.push('deblock=filter=weak:block=8');
  } else if (deblockFilter === 'medium') {
    filters.push('deblock=filter=strong:block=8');
  }
  if (denoise === 'film-grain') {
    filters.push('hqdn3d=1.0:1.0:3.0:3.0');
  } else if (denoise === 'film-restore') {
    filters.push('hqdn3d=1.2:1.0:4.0:3.0');
  } else if (denoise === 'light') {
    filters.push('hqdn3d=1.5:1.5:6:6');
  } else if (denoise === 'medium') {
    filters.push('hqdn3d=2:2:7:7');
  } else if (denoise === 'strong') {
    filters.push('nlmeans=s=2:p=7:r=15');
  }
  if (unsharp === 'subtle') {
    filters.push('unsharp=5:5:0.10:5:5:0.0');
  } else if (unsharp === 'film-restore') {
    filters.push('unsharp=5:5:0.15:5:5:0.0');
  } else if (unsharp === 'light') {
    filters.push('unsharp=5:5:0.25:5:5:0.0');
  }
  if (deband === 'light') {
    filters.push('deband=1thr=0.018:2thr=0.018:3thr=0.018:4thr=0.018');
  } else if (deband === 'medium') {
    filters.push('deband=1thr=0.028:2thr=0.028:3thr=0.028:4thr=0.028');
  }
  if (crop === 'manual' && cropValue.trim()) {
    filters.push(`crop=${cropValue.trim()}`);
  }
  if (exposure !== 0) {
    filters.push(`exposure=exposure=${trimGain(Math.max(-200, Math.min(200, exposure)) / 100)}`);
  }
  if (blackPoint !== 0 || whitePoint !== 100) {
    const inputBlack = trimGain(Math.max(0, Math.min(10, blackPoint)) / 100);
    const inputWhite = trimGain(Math.max(90, Math.min(100, whitePoint)) / 100);
    filters.push(`colorlevels=rimin=${inputBlack}:gimin=${inputBlack}:bimin=${inputBlack}:rimax=${inputWhite}:gimax=${inputWhite}:bimax=${inputWhite}`);
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
  if (vibrance !== 0) {
    filters.push(`vibrance=intensity=${trimGain(Math.max(-100, Math.min(100, vibrance)) / 100)}`);
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

function VideoProfileSaveReview({ profile, source, asset, previewNormalization }: { profile: ProfileInput; source?: ScanResult; asset: Asset | null; previewNormalization: 'preserve' | 'normalize_bt709' }) {
  const video = source?.videoStreams?.[0];
  const encoder = videoWorkerValue(profile, 'videoEncoder', videoWorkerValue(profile, 'preferredEncoder', 'software') === 'hardware' ? 'auto hardware' : softwareEncoderForCodec(profile.videoCodec));
  const pixFmt = videoWorkerValue(profile, 'pixFmt', profile.pixelFormat || 'source default');
  const deinterlace = videoWorkerValue(profile, 'deinterlaceMode', 'auto');
  const filters = buildVideoFilterChain(profile.workerConfig ?? {});
  const crop = filters.match(/(?:^|,)crop=(\d+):(\d+):/);
  const cropAspectPolicy = videoWorkerValue(profile, 'cropAspectPolicy', 'preserve_dar');
  const qualityPreset = videoWorkerValue(profile, 'hardwareQualityPreset', 'custom');
  const quality = encoder === 'hevc_videotoolbox'
    ? `${numberWorkerValue(profile, 'videoToolboxBitrateMbps', 8)} Mbps target · ${numberWorkerValue(profile, 'videoToolboxMaxrateMbps', 10)} Mbps max · ${numberWorkerValue(profile, 'videoToolboxBufferMbps', 16)} Mbps buffer`
    : encoder.includes('qsv') || encoder.includes('vaapi') || encoder.includes('nvenc')
      ? `Global quality ${numberWorkerValue(profile, 'globalQuality', 20)} · ${qualityPreset}`
      : `${profile.qualityMode.toUpperCase()} ${profile.qualityValue}`;
  const rows = [
    ['Container', source?.container || 'Unknown', profile.container.toUpperCase(), source?.container === profile.container ? 'Preserved' : 'Changed intentionally'],
    ['Video codec', source?.videoCodec || 'Unknown', profile.videoCodec, profile.videoCodec === 'copy' ? 'Preserved' : 'Changed intentionally'],
    ['Encoder', 'Source stream', encoder, 'Conversion engine'],
    ['Profile', video?.profile || 'Unknown', videoWorkerValue(profile, 'videoToolboxProfile', profile.videoCodec.includes('10bit') ? 'Main 10' : 'Encoder default'), 'Configured output'],
    ['Pixel format / bit depth', video?.pixFmt || 'Unknown', pixFmt, video?.pixFmt === pixFmt ? 'Preserved' : 'Changed intentionally'],
    ['Frame size', source ? `${source.width}×${source.height}` : 'Unknown', source ? `${source.width}×${source.height}` : 'Preserve source', 'Preserved unless crop/filter changes it'],
    ...(crop ? [['Crop geometry', `${video?.sampleAspectRatio || 'Unknown'} SAR · ${video?.displayAspectRatio || 'Unknown'} DAR`, `${crop[1]}×${crop[2]} · ${cropAspectPolicy === 'source_sar' ? 'Preserve source SAR' : 'Recalculate SAR / preserve original DAR'}`, cropAspectPolicy === 'source_sar' ? 'Crop may change displayed aspect ratio' : 'Pipeline emits setsar and setdar after crop']] : []),
    ['Frame rate', video?.avgFrameRate || 'Unknown', 'Preserve source', 'No CFR override'],
    ['Field handling', video?.fieldOrder || source?.interlaceAnalysis?.status || 'Unknown', deinterlace, deinterlace === 'off' ? 'No deinterlacing' : 'Filter applied when required'],
    ['Preview color domain', [video?.colorSpace, video?.colorTransfer, video?.colorPrimaries, video?.colorRange].filter(Boolean).join(' · ') || 'Unknown', previewNormalization === 'normalize_bt709' ? 'Normalize preview to BT.709' : 'Preserve source domain', 'LAB/Fidelity only · not saved in the profile'],
    ['Final output color policy', [video?.colorSpace, video?.colorTransfer, video?.colorPrimaries, video?.colorRange].filter(Boolean).join(' · ') || 'Unknown', finalColorPolicyReviewLabel(videoWorkerValue(profile, 'finalColorPolicy', 'preserve')), videoWorkerValue(profile, 'finalColorPolicy', 'preserve') === 'normalize_bt709' ? 'Pixel conversion and BT.709 metadata applied by pipeline' : 'No forced BT.709 output conversion'],
    ['Quality / rate control', source?.bitrate ? `${(source.bitrate / 1_000_000).toFixed(2)} Mbps source` : 'Unknown', quality, 'Configured output target'],
    ['Video filters', 'Source image', filters || 'None', filters ? 'Image processing enabled' : 'No image cleanup'],
    ['HDR', source?.hdr ? 'HDR' : 'SDR or unknown', profile.preserveHdr ? 'Preserve' : 'Not preserved', profile.preserveHdr ? 'Protected' : 'Review required'],
    ['Subtitles', `${source?.subtitleTracks ?? 0} embedded`, videoWorkerValue(profile, 'externalSubtitleFormat', 'source'), profile.preserveSubtitles ? 'Preserved' : 'Externalize/remove as configured'],
    ['Bitmap subtitle OCR', 'Only applies to PGS/VobSub/DVB tracks', `${videoWorkerValue(profile, 'subtitleOCRMode', 'accurate')} · ${videoWorkerValue(profile, 'subtitleOCRLanguage', 'auto')}`, videoExternalSubtitleFormat(profile) === 'source' ? 'Not used while embedded tracks are preserved' : 'Runs before FFmpeg; Tracks Profile takes priority'],
    ['AAC compatibility track', `${source?.audioStreams?.length ?? 0} source tracks`, videoAACTrackEnabled(profile) ? `${numberWorkerValue(profile, 'aacStereoBitrateKbps', 192)} kb/s · source ${numberWorkerValue(profile, 'aacStereoSourceStreamIndex', -1) < 0 ? 'automatic/default' : `stream #${numberWorkerValue(profile, 'aacStereoSourceStreamIndex', -1)}`}` : 'Disabled', 'Asset override takes priority; missing sources fall back with a warning'],
    ['Chapters', `${source?.chapters ?? 0}`, profile.preserveChapters ? 'Preserve' : 'Remove', profile.preserveChapters ? 'Preserved' : 'Changed intentionally'],
  ];
  return (
    <Stack spacing={2}>
      <Alert severity="info">Review the technical contract that will be saved. This does not run FFmpeg or queue the asset.</Alert>
      {previewNormalization === 'normalize_bt709' && videoWorkerValue(profile, 'finalColorPolicy', 'preserve') !== 'normalize_bt709' ? (
        <Alert severity="warning">
          Fidelity is displaying a BT.709-normalized preview, but the saved profile will not normalize the final output. Select “Normalize mathematically to BT.709” in Final color policy if the conversion must match that preview color domain.
        </Alert>
      ) : null}
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
        <Chip label={profile.name || 'Unnamed profile'} color="primary" />
        <Chip label={asset?.fileName || 'No LAB asset selected'} />
        <Chip label={videoWorkerValue(profile, 'preferredEncoder', 'software') === 'hardware' ? 'Hardware' : 'Software'} />
        {qualityPreset !== 'custom' ? <Chip label={`Preset: ${qualityPreset.replaceAll('_', ' ')}`} color="success" /> : null}
      </Stack>
      <Table size="small" aria-label="Video profile technical review">
        <TableHead><TableRow><TableCell>Characteristic</TableCell><TableCell>Source</TableCell><TableCell>Profile result</TableCell><TableCell>Effect</TableCell></TableRow></TableHead>
        <TableBody>{rows.map(([label, sourceValue, targetValue, effect]) => <TableRow key={label}><TableCell sx={{ fontWeight: 700 }}>{label}</TableCell><TableCell>{sourceValue}</TableCell><TableCell>{targetValue}</TableCell><TableCell>{effect}</TableCell></TableRow>)}</TableBody>
      </Table>
      {profile.description ? <Alert severity="info">{profile.description}</Alert> : null}
    </Stack>
  );
}

function AudioProfileSaveReview({ profile, source, asset }: { profile: AudioEnhancementProfile; source?: ScanResult; asset: Asset | null }) {
  const adjustedBands = Object.entries(normalizeEqBands(profile.eqBands)).filter(([, value]) => value !== 0);
  const sourceAudio = source?.audioStreams ?? [];
  const sourceAudioLabel = sourceAudio.length
    ? sourceAudio.map((stream) => `#${stream.index} ${stream.codec}${stream.language ? ` · ${stream.language}` : ''}${stream.channels ? ` · ${stream.channels}ch` : ''}${stream.default ? ' · default' : ''}`).join(' | ')
    : 'Unknown';
  const stereoDetail = profile.channelMode === 'force-stereo'
    ? `${profile.forceStereoMode} · delay ${profile.stereoDelayMs} ms · width ${profile.stereoWidth}%`
    : 'Not applicable';
  const rows = [
    ['Audio tracks', sourceAudioLabel, profile.preserveOriginalTrack ? 'Original track(s) + enhanced track' : 'Enhanced track from selected source', profile.preserveOriginalTrack ? 'Source audio is retained' : 'Selected source may be replaced'],
    ['Codec', sourceAudio.length ? Array.from(new Set(sourceAudio.map((stream) => stream.codec))).join(' · ') : 'Unknown', profile.outputCodec || 'copy', profile.outputCodec === 'copy' ? 'Filters cannot be applied while copying' : 'Enhanced output is encoded'],
    ['Channel layout', sourceAudio.length ? sourceAudio.map((stream) => stream.channelLayout || `${stream.channels || '?'}ch`).join(' | ') : 'Unknown', profile.channelMode, profile.channelMode === 'preserve' ? 'Preserve source layout' : 'Channel layout transformation'],
    ['Stereo method', 'Source channel layout', stereoDetail, profile.channelMode === 'force-stereo' ? 'Applied by the audio filter chain' : 'Disabled for this channel mode'],
    ['Loudness', 'Source / unmeasured in profile review', `${profile.targetLoudness} LUFS · ${profile.truePeak} dBTP`, 'Applied only when present in the effective filter chain'],
    ['Noise reduction', 'Source noise floor', profile.rnnoiseModelPath || 'Disabled', profile.rnnoiseModelPath ? 'External RNNoise model required by worker' : 'No RNNoise processing'],
    ['Graphic EQ', 'Neutral source', adjustedBands.length ? adjustedBands.map(([frequency, value]) => `${frequency}: ${value > 0 ? '+' : ''}${value} dB`).join(' · ') : 'Neutral', adjustedBands.length ? 'EQ filters will be generated' : 'No EQ adjustment'],
    ['Effective filter chain', 'Unfiltered source', profile.filters.trim() || 'anull', profile.filters.trim() && profile.filters.trim() !== 'anull' ? 'FFmpeg audio processing enabled' : 'No audio filter'],
  ];
  return (
    <Stack spacing={2}>
      <Alert severity="info">Review the audio processing contract that will be saved. This does not process or queue the asset.</Alert>
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
        <Chip label={profile.name || 'Unnamed profile'} color="primary" />
        <Chip label={asset?.fileName || 'No LAB asset selected'} />
        <Chip label={profile.outputCodec.toUpperCase()} />
      </Stack>
      <Table size="small" aria-label="Audio profile technical review">
        <TableHead><TableRow><TableCell>Characteristic</TableCell><TableCell>Source</TableCell><TableCell>Profile result</TableCell><TableCell>Effect</TableCell></TableRow></TableHead>
        <TableBody>{rows.map(([label, sourceValue, targetValue, effect]) => <TableRow key={label}><TableCell sx={{ fontWeight: 700 }}>{label}</TableCell><TableCell sx={{ wordBreak: 'break-word' }}>{sourceValue}</TableCell><TableCell sx={{ wordBreak: 'break-word' }}>{targetValue}</TableCell><TableCell>{effect}</TableCell></TableRow>)}</TableBody>
      </Table>
      {profile.description ? <Alert severity="info">{profile.description}</Alert> : null}
    </Stack>
  );
}

function TrackProfileSaveReview({ profile, conversion, source, asset }: { profile: TrackProfile; conversion: AssetConversionOverrideState; source?: ScanResult; asset: Asset | null }) {
  const clean = cleanTrackConversionOverride(conversion);
  const streamLabel = (stream: MediaStreamInfo) => `#${stream.index} ${stream.codec}${stream.language ? ` · ${stream.language}` : ''}${stream.title ? ` · ${stream.title}` : ''}`;
  const streamDelta = (streams: MediaStreamInfo[] | undefined, kept: number[] | null | undefined) => {
    const available = streams ?? [];
    const keptSet = kept == null ? new Set(available.map((stream) => stream.index)) : new Set(kept);
    return {
      kept: available.filter((stream) => keptSet.has(stream.index)),
      removed: available.filter((stream) => !keptSet.has(stream.index)),
    };
  };
  const videoDelta = streamDelta(source?.videoStreams, clean.keepVideoStreams);
  const audioDelta = streamDelta(source?.audioStreams, clean.keepAudioStreams);
  const subtitleDelta = streamDelta(source?.subtitleStreams, clean.keepSubtitleStreams);
  const transforms = clean.subtitleTransforms ?? [];
  const metadataActions = ([
    ['Video', clean.videoMetadata],
    ['Audio', clean.audioMetadata],
    ['Subtitle', clean.subtitleMetadata],
  ] as const).flatMap(([type, values]) => Object.entries(values ?? {}).map(([index, override]) => {
    const changes = [
      override.language ? `language=${override.language}` : '',
      override.title ? `title=${override.title}` : '',
      override.default !== undefined ? `default=${override.default ? 'yes' : 'no'}` : '',
      override.forced !== undefined ? `forced=${override.forced ? 'yes' : 'no'}` : '',
    ].filter(Boolean);
    return `${type} #${index}: ${changes.join(' · ') || 'no effective metadata change'}`;
  }));
  const removalActions = [
    ...videoDelta.removed.map((stream) => `Remove video ${streamLabel(stream)}`),
    ...audioDelta.removed.map((stream) => `Remove audio ${streamLabel(stream)}`),
    ...subtitleDelta.removed.map((stream) => `Remove subtitle ${streamLabel(stream)}`),
  ];
  const subtitleActions = transforms.map((item) => {
    const stream = source?.subtitleStreams?.find((candidate) => candidate.index === item.streamIndex);
    const details = [
      `Convert subtitle ${stream ? streamLabel(stream) : `#${item.streamIndex}`} to ${item.format.toUpperCase()}`,
      item.removeEmbedded ? 'remove embedded track' : 'keep embedded track',
      item.makeDefault ? 'external subtitle becomes default' : '',
      item.ocrMode ? `OCR ${item.ocrMode}${item.ocrLanguage ? ` (${item.ocrLanguage})` : ''}` : '',
    ].filter(Boolean);
    return details.join(' · ');
  });
  const ruleActions = [
    profile.dropCommentary ? 'Rule: remove tracks identified as commentary' : '',
    profile.audioMode === 'none' ? 'Rule: remove all audio tracks' : '',
    profile.audioMode === 'default' ? 'Rule: keep only the default audio track' : '',
    profile.audioMode === 'languages' ? `Rule: keep audio languages ${profile.audioLanguages.join(', ') || 'none configured'}` : '',
    profile.subtitleMode === 'none' ? 'Rule: remove all embedded subtitle tracks' : '',
    profile.subtitleMode === 'forced' ? 'Rule: keep only forced subtitles' : '',
    profile.subtitleMode === 'languages' || profile.subtitleMode === 'forced-or-languages' ? `Rule: keep subtitle languages ${profile.subtitleLanguages.join(', ') || 'none configured'}${profile.subtitleMode === 'forced-or-languages' ? ' plus forced tracks' : ''}` : '',
  ].filter(Boolean);
  const effectiveActions = [...removalActions, ...subtitleActions, ...metadataActions, ...ruleActions];
  const sourceSummary = (delta: { kept: MediaStreamInfo[]; removed: MediaStreamInfo[] }) => [...delta.kept, ...delta.removed].length
    ? [...delta.kept, ...delta.removed].sort((left, right) => left.index - right.index).map(streamLabel).join(' | ')
    : 'None detected';
  const resultSummary = (delta: { kept: MediaStreamInfo[]; removed: MediaStreamInfo[] }) => [
    `Keep: ${delta.kept.length ? delta.kept.map(streamLabel).join(' | ') : 'none'}`,
    `Remove: ${delta.removed.length ? delta.removed.map(streamLabel).join(' | ') : 'none'}`,
  ].join(' — ');
  const rows = [
    ['Video streams', sourceSummary(videoDelta), resultSummary(videoDelta), `Selection mode: ${profile.videoMode}`],
    ['Audio streams', sourceSummary(audioDelta), resultSummary(audioDelta), `${profile.audioMode} · languages ${profile.audioLanguages.join(', ') || 'none'}`],
    ['Subtitle streams', sourceSummary(subtitleDelta), resultSummary(subtitleDelta), `${profile.subtitleMode} · languages ${profile.subtitleLanguages.join(', ') || 'none'}`],
    ['Commentary tracks', audioDelta.kept.concat(audioDelta.removed).filter(isCommentaryStream).map(streamLabel).join(' | ') || 'None identified', profile.dropCommentary ? 'Remove identified commentary' : 'Keep', 'Metadata-based detection; validate against the source snapshot'],
    ['Default audio', source?.audioStreams?.filter((stream) => stream.default).map(streamLabel).join(' | ') || 'Unknown', profile.defaultAudioLanguage || 'Unchanged', profile.audioRequired ? 'Required track/language' : 'Optional'],
    ['Default subtitle', source?.subtitleStreams?.filter((stream) => stream.default).map(streamLabel).join(' | ') || 'Unknown', profile.defaultSubtitleLanguage || 'Unchanged', profile.subtitlesRequired ? 'Required track/language' : 'Optional'],
    ['Subtitle exports', source?.subtitleStreams?.length ? source.subtitleStreams.map(streamLabel).join(' | ') : 'None detected', transforms.length ? transforms.map((item) => `#${item.streamIndex} → ${item.format.toUpperCase()}${item.removeEmbedded ? ' · remove embedded' : ''}${item.makeDefault ? ' · default' : ''}`).join(' | ') : 'None', transforms.some((item) => item.ocrMode) ? 'Bitmap exports run OCR before media conversion' : 'Text conversion or no export'],
    ['Metadata', metadataActions.length ? 'Selected source-track metadata' : 'Preserve source metadata', metadataActions.length ? metadataActions.join(' | ') : 'Unchanged', 'Language, title, default and forced values are applied per stream'],
    ['Validation', 'Source snapshot', profile.validationMode, 'Missing required tracks follow this policy'],
  ];
  return (
    <Stack spacing={2}>
      <Alert severity="info">Review the stream-selection contract that will be saved. A snapshot stores metadata only; restoring a removed stream requires the archived original.</Alert>
      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
        <Chip label={profile.name || 'Unnamed profile'} color="primary" />
        <Chip label={asset?.fileName || profile.sourceAssetName || 'No LAB asset selected'} />
        <Chip label={`${source?.videoStreams?.length ?? 0}V · ${source?.audioStreams?.length ?? 0}A · ${source?.subtitleStreams?.length ?? 0}S source`} />
      </Stack>
      <Alert severity={effectiveActions.length ? 'warning' : 'success'}>
        <Typography fontWeight={700}>{effectiveActions.length ? 'Changes that this profile will apply' : 'No explicit stream removals, subtitle conversions, or metadata changes'}</Typography>
        {effectiveActions.length ? (
          <Stack spacing={0.5} sx={{ mt: 1 }}>
            {effectiveActions.map((action, index) => <Typography key={`${index}-${action}`} variant="body2">• {action}</Typography>)}
          </Stack>
        ) : null}
      </Alert>
      <Table size="small" aria-label="Track profile technical review">
        <TableHead><TableRow><TableCell>Characteristic</TableCell><TableCell>Source</TableCell><TableCell>Profile result</TableCell><TableCell>Effect</TableCell></TableRow></TableHead>
        <TableBody>{rows.map(([label, sourceValue, targetValue, effect]) => <TableRow key={label}><TableCell sx={{ fontWeight: 700 }}>{label}</TableCell><TableCell sx={{ wordBreak: 'break-word' }}>{sourceValue}</TableCell><TableCell sx={{ wordBreak: 'break-word' }}>{targetValue}</TableCell><TableCell>{effect}</TableCell></TableRow>)}</TableBody>
      </Table>
      {profile.description ? <Alert severity="info">{profile.description}</Alert> : null}
    </Stack>
  );
}

function sameProfileName(left: string, right: string) {
  return left.trim().toLocaleLowerCase() === right.trim().toLocaleLowerCase();
}

function normalizedAudioProfileForSave(profile: AudioEnhancementProfile, filterChainEdited: boolean, editedFilters: string): AudioEnhancementProfile {
  return {
    ...profile,
    key: slugify(profile.key || profile.name),
    filters: filterChainEdited ? editedFilters : profile.filters,
    channelMode: filterChainEdited ? 'preserve' : profile.channelMode,
    eqBands: filterChainEdited ? defaultEqBands() : normalizeEqBands(profile.eqBands),
  };
}

function softwareEncoderForCodec(codec: string) {
  const value = codec.toLowerCase();
  if (value === 'copy') return 'copy';
  if (value.includes('264')) return 'libx264';
  if (value.includes('av1')) return 'libsvtav1';
  return 'libx265';
}

function finalColorPolicyReviewLabel(policy: string) {
  if (policy === 'normalize_bt709') return 'Normalize final output to BT.709';
  if (policy === 'automatic') return 'Automatic correction when justified';
  return 'No correction · preserve source';
}

function isCanceledFidelityRequest(error: unknown) {
  if (error instanceof DOMException && error.name === 'AbortError') return true;
  const message = error instanceof Error ? error.message : String(error ?? '');
  return /context canceled|context cancelled|request canceled|request cancelled|aborted/i.test(message);
}

function LabRecommendationDetails({
  report,
  selected,
  onToggle,
}: {
  report: LabRecommendationReport;
  selected: RecommendationSectionState;
  onToggle: (section: LabSection, checked: boolean) => void;
}) {
  const sections: Array<{ key: LabSection; label: string; items: string[] }> = [
    { key: 'video', label: 'Video', items: report.video },
    { key: 'audio', label: 'Audio', items: report.audio },
    { key: 'tracks', label: 'Tracks', items: report.tracks },
  ];
  const selectedCount = Object.values(selected).filter(Boolean).length;
  return (
    <Stack spacing={1.5}>
      <Alert severity="info">
        {selectedCount > 0
          ? `${selectedCount} recommendation section${selectedCount === 1 ? '' : 's'} selected. Review them before applying.`
          : 'No suggestions are selected. You can open the LAB without modifying its drafts.'}
      </Alert>
      <Typography variant="h3">{report.summary}</Typography>
      <Typography color="text.secondary" variant="body2">{report.match}</Typography>
      <Grid container spacing={1.5}>
        {sections.map((section) => (
          <Grid key={section.key} size={{ xs: 12, md: 4 }}>
            <Box sx={{ height: '100%', p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1 }}>
              <Stack spacing={1}>
                <FormControlLabel
                  control={<Checkbox checked={selected[section.key]} onChange={(event) => onToggle(section.key, event.target.checked)} />}
                  label={<Typography fontWeight={700}>{section.label}</Typography>}
                />
                {section.items.map((item) => (
                  <Typography key={item} variant="body2">• {item}</Typography>
                ))}
              </Stack>
            </Box>
          </Grid>
        ))}
      </Grid>
      <Alert severity="info">
        <Stack spacing={0.5}>
          {report.general.map((item) => <Typography key={item} variant="body2">{item}</Typography>)}
        </Stack>
      </Alert>
      <Typography color="text.secondary" variant="caption">
        Apply suggestions updates only the selected LAB drafts. Open without suggestions keeps the current values. Neither action saves a profile or adds a Queue job.
      </Typography>
    </Stack>
  );
}

function previewRecommendationReport(suggestion: ProfileSuggestion): LabRecommendationReport {
  const scan = suggestion.scan;
  const interlace = scan.interlaceAnalysis.status ?? 'unknown';
  const crop = scan.cropAnalysis?.status ?? 'unknown';
  const commentary = scan.audioStreams.filter(isCommentaryStream).length;
  const video = [
    `Proposed ${suggestion.proposedProfile.videoCodec} at CRF ${suggestion.insights.recommendedCrf} for ${scan.width}×${scan.height}.`,
    scan.hdr ? 'Preserve detected HDR metadata and 10-bit output.' : 'No HDR preservation adjustment is required.',
    interlace === 'interlaced'
      ? 'Enable bwdif because interlacing was detected.'
      : interlace === 'progressive'
        ? 'Keep deinterlacing disabled because the analyzed window is progressive.'
        : interlace === 'telecine_suspected'
          ? `Enable inverse telecine ${(scan.interlaceAnalysis.detectedFieldOrder || scan.interlaceAnalysis.fieldOrder)?.toLowerCase().startsWith('b') ? 'BFF' : 'TFF'} with ${scan.interlaceAnalysis.recommendedFilter || 'fieldmatch and decimate'}.`
        : `Keep conversion-time motion analysis enabled because the source was classified as ${interlace}.`,
    crop === 'detected' && scan.subtitleStreams.some(isBitmapSubtitleStream)
      ? 'Keep crop disabled because bitmap subtitles may be positioned inside the detected black bars.'
      : crop === 'detected'
        ? `Add crop ${scan.cropAnalysis.recommendedCrop} as a disabled suggestion for visual confirmation.`
      : crop === 'variable' && scan.cropAnalysis.recommendedCrop
        ? `Add conservative candidate crop ${scan.cropAnalysis.recommendedCrop}, disabled by default because the detected borders vary slightly between scenes.`
        : crop === 'variable'
          ? 'Keep crop disabled because the detected borders vary between scenes.'
        : 'Keep crop disabled because no stable black bars were detected.',
  ];
  const incompatibleAudio = scan.audioStreams.filter((stream) => !['aac', 'ac3', 'eac3', 'opus', 'flac'].includes(stream.codec.toLowerCase())).length;
  const audio = [
    incompatibleAudio > 0
      ? `Prepare an AAC compatibility copy for ${incompatibleAudio} less-compatible audio track(s), preserving every original.`
      : 'Preserve the compatible original audio tracks; an editable AAC compatibility draft remains available.',
    'Do not infer EQ, denoise, or loudness correction from metadata alone.',
  ];
  const tracks = [
    `Preserve ${scan.videoStreams.length} video, ${scan.audioStreams.length} audio, and ${scan.subtitleStreams.length} subtitle track(s).`,
    commentary > 0
      ? `Deselect ${commentary} track(s) identified as commentary; review before saving.`
      : 'Do not remove tracks automatically because no commentary metadata was identified.',
  ];
  return {
    summary: suggestion.summary,
    match: suggestion.matchType === 'existing' && suggestion.suggestedProfile
      ? `Closest existing profile: ${suggestion.suggestedProfile.name}`
      : 'A new profile draft is recommended.',
    video,
    audio,
    tracks,
    general: suggestion.insights.recommendations,
  };
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
      renderInput={(params) => <TextField {...params} label="Asset from Raw or Library" size="small" />}
      renderOption={(props, asset) => (
        <Box component="li" {...props} key={asset.path}>
          <Stack sx={{ minWidth: 0 }}>
            <Typography fontWeight={700} noWrap>
              {asset.fileName}
            </Typography>
            <Typography color="text.secondary" variant="body2" noWrap>
              {labAssetLocationLabel(asset)} · {asset.relativePath || asset.path}
            </Typography>
          </Stack>
        </Box>
      )}
      fullWidth
    />
  );
}

function labAssetLocationLabel(asset: Asset) {
  if (asset.status === 'unprocessed') {
    return 'RAW · Unprocessed';
  }
  return `${asset.status.toUpperCase()} · ${asset.libraryName || 'Unassigned library'}`;
}

function uniqueAssets(assets: Asset[]) {
  const byPath = new Map<string, Asset>();
  assets.forEach((asset) => {
    const current = byPath.get(asset.path);
    if (!current || asset.status === 'library' || asset.status === 'published_as_is' || asset.status === 'converted') {
      byPath.set(asset.path, asset);
    }
  });
  return [...byPath.values()].sort((left, right) =>
    (left.relativePath || left.path).localeCompare(right.relativePath || right.path),
  );
}

function isCommentaryStream(stream: MediaStreamInfo) {
  const metadata = `${stream.title} ${stream.codecLong}`.toLowerCase();
  return stream.comment || ['commentary', 'comentario', 'director comments'].some((term) => metadata.includes(term));
}

function isBitmapSubtitleStream(stream: MediaStreamInfo) {
  const codec = stream.codec.toLowerCase();
  return ['dvd_subtitle', 'hdmv_pgs_subtitle', 'pgssub', 'dvb_subtitle', 'xsub'].includes(codec);
}

function defaultTrackOCRLanguage(language: string) {
  const value = (language || '').trim().toLowerCase();
  if (['spa', 'es', 'esp'].includes(value)) return 'spa';
  if (['jpn', 'ja', 'jp'].includes(value)) return 'jpn';
  if (['jpn_vert', 'ja_vert'].includes(value)) return 'jpn_vert';
  return 'eng';
}

function TrackProfileAutocomplete({
  profiles,
  selectedKey,
  onChange,
}: {
  profiles: TrackProfile[];
  selectedKey: string | null;
  onChange: (profile: TrackProfile | null) => void;
}) {
  return (
    <Autocomplete
      options={profiles}
      value={profiles.find((profile) => profile.key === selectedKey) ?? null}
      onChange={(_, profile) => onChange(profile)}
      getOptionLabel={(profile) => `${profile.name} · ${profileTrackSummary(profile)}`}
      isOptionEqualToValue={(option, selected) => option.key === selected.key}
      renderInput={(params) => <TextField {...params} label={selectedKey ? 'Editing track profile' : 'Start from track profile'} />}
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
  if (Array.isArray(value.subtitleTransforms) && value.subtitleTransforms.length) {
    clean.subtitleTransforms = value.subtitleTransforms;
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

function getAudioProfiles(settings?: AppSetting[], includeInactive = false) {
  const value = settings?.find((setting) => setting.key === 'audioEnhancementProfiles')?.value.profiles;
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((profile) => normalizeAudioProfile(profile))
    .filter((profile): profile is AudioEnhancementProfile => Boolean(profile))
    .filter((profile) => includeInactive || (!profile.disabled && !profile.deletedAt));
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

type CombinedLabCommandInput = {
  assetPath: string;
  start: string;
  seconds: number;
  video: ProfileInput;
  audio: AudioEnhancementProfile;
  tracks: AssetConversionOverrideState;
  scan?: ScanResult;
  selectedAudioStreamIndex?: number;
};

function buildCombinedLabFFmpegCommand(input: CombinedLabCommandInput) {
  if (!input.assetPath || !input.scan) return '';

  const videoIndexes = conversionStreamIndexes(input.tracks, input.scan, 'video');
  let audioIndexes = conversionStreamIndexes(input.tracks, input.scan, 'audio');
  const subtitleIndexes = conversionStreamIndexes(input.tracks, input.scan, 'subtitle');
  const audioExplicitlyRemoved = Array.isArray(input.tracks.keepAudioStreams) && input.tracks.keepAudioStreams.length === 0;
  const sourceAudioIndex = audioExplicitlyRemoved
    ? undefined
    : input.selectedAudioStreamIndex !== undefined && input.scan.audioStreams.some((stream) => stream.index === input.selectedAudioStreamIndex)
      ? input.selectedAudioStreamIndex
      : audioIndexes[0];
  if (sourceAudioIndex !== undefined && !audioIndexes.includes(sourceAudioIndex)) {
    audioIndexes = [...audioIndexes, sourceAudioIndex];
  }

  const args = ['ffmpeg', '-hide_banner', '-y', '-ss', input.start || '00:00:00', '-i', shellQuote(input.assetPath), '-t', String(Math.max(1, Math.min(60, input.seconds || 20)))];
  videoIndexes.forEach((index) => args.push('-map', `0:${index}`));
  audioIndexes.forEach((index) => args.push('-map', `0:${index}`));
  subtitleIndexes.forEach((index) => args.push('-map', `0:${index}`));

  let processedAudioOutputIndex = sourceAudioIndex === undefined ? -1 : audioIndexes.indexOf(sourceAudioIndex);
  if (sourceAudioIndex !== undefined && input.audio.preserveOriginalTrack) {
    args.push('-map', `0:${sourceAudioIndex}`);
    processedAudioOutputIndex = audioIndexes.length;
  }
  args.push('-c', 'copy');
  args.push(...combinedVideoCommandArgs(input.video, input.scan));

  if (processedAudioOutputIndex >= 0) {
    const filters = effectiveAudioFilters(input.audio);
    const codec = terminalAudioCodec(input.audio.outputCodec);
    if (filters) args.push(`-filter:a:${processedAudioOutputIndex}`, shellQuote(filters));
    args.push(`-c:a:${processedAudioOutputIndex}`, codec);
    args.push(`-metadata:s:a:${processedAudioOutputIndex}`, shellQuote(`title=${input.audio.name || 'MVForge Lab audio'}`));
  }

  appendTerminalMetadata(args, 'v', videoIndexes, input.tracks.videoMetadata);
  appendTerminalMetadata(args, 'a', audioIndexes, input.tracks.audioMetadata);
  appendTerminalMetadata(args, 's', subtitleIndexes, input.tracks.subtitleMetadata);
  if (input.video.preserveChapters) args.push('-map_chapters', '0');

  const baseName = input.assetPath.split('/').pop()?.replace(/\.[^.]+$/, '') || 'mvforge-lab';
  const container = input.video.container === 'mp4' ? 'mp4' : 'mkv';
  args.push(shellQuote(`/tmp/${baseName}.mvforge-lab.${container}`));
  return args.join(' ');
}

function combinedVideoCommandArgs(profile: ProfileInput, scan: ScanResult) {
  if (profile.videoCodec === 'copy') return ['-c:v', 'copy'];
  const preference = videoWorkerValue(profile, 'preferredEncoder', 'software');
  const configuredEncoder = videoWorkerValue(profile, 'videoEncoder', 'auto');
  const encoder = preference === 'hardware' && configuredEncoder !== 'auto'
    ? configuredEncoder
    : softwareEncoderForLabCodec(profile.videoCodec);
  const worker = profile.workerConfig ?? {};
  let filters = buildVideoFilterChain(worker);
  const args = ['-c:v', encoder];
  const automaticMotion = combinedAutomaticMotionFilters(profile, scan);
  if (automaticMotion.length) {
    filters = [automaticMotion.join(','), filters].filter(Boolean).join(',');
  }

  if (encoder === 'hevc_qsv') {
    args.push('-init_hw_device', 'qsv=hw,child_device=/dev/dri/renderD128');
    args.push('-global_quality', String(numberWorkerValue(profile, 'globalQuality', qsvQualityRangeForCrf(profile.qualityValue).recommended)));
    args.push('-preset', videoWorkerValue(profile, 'videoPreset', 'medium'));
    const pixFmt = videoWorkerValue(profile, 'pixFmt', 'yuv420p10le') === 'yuv420p10le' ? 'p010le' : videoWorkerValue(profile, 'pixFmt', 'nv12');
    args.push('-pix_fmt', pixFmt);
    if (videoWorkerValue(profile, 'qsvRateControl', 'icq') === 'la_icq') args.push('-look_ahead', '1');
  } else if (encoder === 'hevc_vaapi') {
    const format = videoWorkerValue(profile, 'pixFmt', 'yuv420p10le').includes('10') ? 'p010le' : 'nv12';
    filters = [filters, `format=${format}`, 'hwupload'].filter(Boolean).join(',');
    args.push('-vaapi_device', '/dev/dri/renderD128');
    args.push('-global_quality', String(numberWorkerValue(profile, 'globalQuality', qsvQualityRangeForCrf(profile.qualityValue).recommended)));
  } else if (encoder === 'hevc_videotoolbox') {
    const rates = adaptiveVideoToolboxPreviewRates(profile, scan);
    args.push('-b:v', rates.target, '-maxrate', rates.maxrate, '-bufsize', rates.buffer);
    const tenBit = isTenBitDraft(profile);
    args.push('-profile:v', tenBit ? 'main10' : 'main', '-pix_fmt', tenBit ? 'p010le' : 'yuv420p');
    const color = combinedVideoToolboxColorConversion(scan);
    if (color.filter) {
      filters = [filters, color.filter].filter(Boolean).join(',');
    }
    if (color.outputColor) {
      args.push(
        '-colorspace', color.outputColor.space,
        '-color_trc', color.outputColor.transfer,
        '-color_primaries', color.outputColor.primaries,
        '-color_range', color.outputColor.range,
      );
    }
  } else if (isHardwareEncoderOption(encoder)) {
    args.push('-cq', String(numberWorkerValue(profile, 'globalQuality', qsvQualityRangeForCrf(profile.qualityValue).recommended)));
  } else {
    args.push('-crf', String(profile.qualityValue || 20));
    args.push('-preset', videoWorkerValue(profile, 'videoPreset', 'medium'));
    const pixFmt = videoWorkerValue(profile, 'pixFmt', 'auto');
    if (pixFmt !== 'auto') args.push('-pix_fmt', pixFmt);
    if (encoder === 'libx265') {
      const x265Params = videoWorkerValue(profile, 'x265Params');
      if (x265Params) args.push('-x265-params', shellQuote(x265Params));
    }
  }
  if (filters) args.push('-vf', shellQuote(filters));
  return args;
}

function adaptiveVideoToolboxPreviewRates(profile: ProfileInput, scan: ScanResult) {
  const preset = videoWorkerValue(profile, 'hardwareQualityPreset', 'custom');
  if (preset === 'custom') {
    const bitrate = numberWorkerValue(profile, 'videoToolboxBitrateMbps', 6);
    return { target: `${bitrate}M`, maxrate: `${numberWorkerValue(profile, 'videoToolboxMaxrateMbps', Math.ceil(bitrate * 1.5))}M`, buffer: `${numberWorkerValue(profile, 'videoToolboxBufferMbps', Math.ceil(bitrate * 2.5))}M` };
  }
  const rates = adaptiveVideoToolboxPresetKbps(profile, scan);
  if (rates) return { target: `${rates.target}k`, maxrate: `${rates.maxrate}k`, buffer: `${rates.buffer}k` };
  const bitrate = numberWorkerValue(profile, 'videoToolboxBitrateMbps', 6);
  return { target: `${bitrate}M`, maxrate: `${Math.ceil(bitrate * 1.5)}M`, buffer: `${Math.ceil(bitrate * 2.5)}M` };
}

function adaptiveVideoToolboxPresetKbps(profile: ProfileInput, scan: ScanResult) {
  return sharedAdaptiveVideoToolboxPresetKbps(profile.workerConfig ?? {}, scan);
}

function combinedAutomaticMotionFilters(profile: ProfileInput, scan: ScanResult) {
  if (videoFilterControlValue(profile, 'deinterlaceMode', 'auto') !== 'auto') return [];
  const status = scan.interlaceAnalysis.status;
  if (status === 'interlaced') {
    const detected = (scan.interlaceAnalysis.detectedFieldOrder || scan.interlaceAnalysis.fieldOrder || '').toLowerCase();
    const parity = detected.startsWith('b') ? 'bff' : detected.startsWith('t') ? 'tff' : 'auto';
    return [`bwdif=mode=send_frame:parity=${parity}:deint=all`, 'setfield=prog'];
  }
  if (status === 'telecine_suspected') {
    const mode = scan.interlaceAnalysis.recommendedMode;
    const order = mode === 'ivtc_bff' ? 'bff' : 'tff';
    return [`fieldmatch=order=${order}`, 'decimate', 'setfield=prog'];
  }
  if (status === 'progressive' && scan.interlaceAnalysis.fieldOrderMismatch) {
    return ['setfield=prog'];
  }
  return [];
}

function combinedVideoToolboxColorConversion(scan: ScanResult): {
  filter?: string;
  outputColor?: { space: string; transfer: string; primaries: string; range: string };
} {
  const source = scan.videoStreams[0];
  if (!source) return {};
  const space = (source.colorSpace || '').toLowerCase();
  const transfer = (source.colorTransfer || '').toLowerCase();
  const primaries = (source.colorPrimaries || '').toLowerCase();
  const range = (source.colorRange || '').toLowerCase();
  const outputRange = range === 'pc' || range === 'full' ? 'pc' : 'tv';
  if (space === 'bt709' && transfer === 'bt709' && primaries === 'bt709') {
    return { outputColor: { space: 'bt709', transfer: 'bt709', primaries: 'bt709', range: outputRange } };
  }
  const supportedSpace = new Set(['bt470bg', 'smpte170m', 'smpte240m', 'bt709']);
  const supportedPrimaries = new Set(['bt470m', 'bt470bg', 'smpte170m', 'smpte240m', 'bt709']);
  const supportedTransfer = new Set(['bt470m', 'bt470bg', 'smpte170m', 'smpte240m', 'bt709']);
  if (!supportedSpace.has(space) || !supportedPrimaries.has(primaries) || !supportedTransfer.has(transfer) || !['tv', 'limited', 'pc', 'full'].includes(range)) {
    return {};
  }
  const inputRange = range === 'pc' || range === 'full' ? 'pc' : 'tv';
  return {
    filter: `colorspace=ispace=${space}:iprimaries=${primaries}:itrc=${transfer}:irange=${inputRange}:space=bt709:primaries=bt709:trc=bt709:range=tv`,
    outputColor: { space: 'bt709', transfer: 'bt709', primaries: 'bt709', range: 'tv' },
  };
}

function softwareEncoderForLabCodec(codec: string) {
  const normalized = codec.toLowerCase();
  if (normalized.includes('264')) return 'libx264';
  if (normalized.includes('av1')) return 'libsvtav1';
  return 'libx265';
}

function terminalAudioCodec(codec: string) {
  switch (codec.toLowerCase()) {
    case 'opus':
      return 'libopus';
    case 'copy':
      return 'aac';
    default:
      return codec || 'aac';
  }
}

function appendTerminalMetadata(
  args: string[],
  type: 'v' | 'a' | 's',
  inputIndexes: number[],
  metadata?: Record<string, StreamMetadataOverride>,
) {
  inputIndexes.forEach((inputIndex, outputIndex) => {
    const value = metadata?.[String(inputIndex)];
    if (!value) return;
    if (value.language) args.push(`-metadata:s:${type}:${outputIndex}`, shellQuote(`language=${value.language}`));
    if (value.title) args.push(`-metadata:s:${type}:${outputIndex}`, shellQuote(`title=${value.title}`));
    if (value.default !== undefined || value.forced !== undefined) {
      const disposition = [value.default ? 'default' : '', value.forced ? 'forced' : ''].filter(Boolean).join('+') || '0';
      args.push(`-disposition:${type}:${outputIndex}`, disposition);
    }
  });
}

function shellQuote(value: string) {
  return `'${value.replaceAll("'", "'\"'\"'")}'`;
}

async function copyTextToClipboard(value: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const textarea = document.createElement('textarea');
  textarea.value = value;
  textarea.style.position = 'fixed';
  textarea.style.opacity = '0';
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand('copy');
  textarea.remove();
}

function channelFilter(profile: AudioEnhancementProfile) {
  switch (profile.channelMode) {
    case 'dual-mono':
      return 'pan=stereo|c0=c0|c1=c0';
    case 'force-stereo':
      return forceStereoFilter(profile.forceStereoMode);
    case 'downmix-mono':
      return 'aresample=ochl=mono';
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
      return 'aresample=ochl=stereo';
  }
}

function normalizedBaseFilters(profile: AudioEnhancementProfile) {
  const filters = profile.filters
    .split(',')
    .map((filter) => filter.trim())
    .filter(Boolean);
  const loudnorm = loudnormFilter(profile);
  const normalized = filters.map((filter) => {
    if (!filter.startsWith('loudnorm=')) {
      return filter;
    }
    return loudnorm;
  });
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
  return filterChain.replace(/aresample=ocl=/g, 'aresample=ochl=').replace(/afftdn=([^,]*\bnf=)(-?\d+(?:\.\d+)?)/g, (_match, prefix: string, rawValue: string) => {
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

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return 'Unknown';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size.toFixed(index > 1 ? 2 : 0)} ${units[index]}`;
}
