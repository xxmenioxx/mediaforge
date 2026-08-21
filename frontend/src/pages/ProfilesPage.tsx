import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  Dialog,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  Grid,
  MenuItem,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import CloseIcon from '@mui/icons-material/Close';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import EditIcon from '@mui/icons-material/Edit';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import FileUploadIcon from '@mui/icons-material/FileUpload';
import ScienceIcon from '@mui/icons-material/Science';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import { FrameStructureControls } from '../components/FrameStructureControls';
import type { Profile, ProfileInput } from '../api/types';
import { qsvQualityHelper, qsvQualityRangeForCrf } from '../utils/qsv';
import { applyHardwareQualityPreset as applySharedHardwareQualityPreset, hardwareQualityPresetOptions } from '../utils/hardwareQualityPresets';
import { qsvPStrategySupported, qsvSelectionWarnings, resolveQSVFeatures } from '../utils/qsvCapabilities';
import { videoToolboxRatesFromTargetMbps } from '../utils/videoToolboxRates';
import { frameStructureManagedKeys } from '../utils/frameStructureModes';
import { encoderNamesForWorker, selectedWorker as resolveSelectedWorker } from '../utils/workerEncoders';
import { applyMVForgeVideoPreferences, getMVForgePreferences } from '../mvforgePreferences';
import { normalizeLegacyVideoCodec } from '../utils/videoCodec';

const initialProfile: ProfileInput = {
  name: '',
  scope: 'asset',
  description: '',
  container: 'mkv',
  videoCodec: 'x265',
  codecFamily: 'hevc',
  encoderPolicy: 'locked',
  preferredEncoder: 'libx265',
  allowedEncoders: ['libx265'],
  fallbackPolicy: 'wait',
  bitDepth: 10,
  pixelFormat: 'yuv420p10le',
  qualityStrategy: 'crf',
  optimizationIntent: 'balanced',
  audioCodec: 'copy',
  qualityMode: 'crf',
  qualityValue: 20,
  preserveHdr: true,
  preserveSubtitles: true,
  preserveChapters: true,
  disabled: false,
  workerConfig: {
    encoder: 'ffmpeg',
    preset: 'custom',
    videoPreset: 'medium',
    pixFmt: 'yuv420p10le',
    videoEncoder: 'auto',
    useHardwareIfAvailable: false,
    videoToolboxBitrateMbps: 2,
    videoToolboxMaxrateMbps: 3,
    videoToolboxBufferMbps: 5,
    preferredEncoder: 'software',
    addAacStereoTrack: false,
    aacStereoBitrateKbps: 192,
    aacStereoDefault: false,
    preserveOriginalAudio: true,
    preferSrtSubtitles: false,
    warnSubtitleFormats: true,
    subtitleOCRMode: 'accurate',
    subtitleOCRLanguage: 'auto',
    frameStructureMode: 'auto',
    frameStructureGopMode: 'recommended',
    frameStructureBFrameMode: 'recommended',
  },
};

const encoderPresetOptions = [
  { value: 'veryfast', label: 'Fast preview', description: 'Faster conversions, larger files. Useful while testing.' },
  { value: 'fast', label: 'Fast', description: 'Faster than Balanced while retaining more compression efficiency than Fast preview.' },
  { value: 'medium', label: 'Balanced', description: 'Recommended default for quality, size, and speed.' },
  { value: 'slow', label: 'Higher compression', description: 'Slower, usually smaller files at the same quality.' },
  { value: 'slower', label: 'Archive patience', description: 'Very slow. Use only when size matters more than time.' },
] as const;

const pixelFormatOptions = [
  { value: 'auto', label: 'Auto / codec default', description: 'Lets MVForge choose a compatible pixel format from the codec and encoder.' },
  { value: 'yuv420p10le', label: '10-bit Main10', description: 'Recommended for x265, anime, and DVD sources. Helps reduce banding.' },
  { value: 'p010le', label: 'Hardware 10-bit Main10 (P010)', description: 'Native 10-bit format for hardware HEVC Main10, including Quick Sync and VideoToolbox.' },
  { value: 'yuv420p', label: '8-bit compatibility', description: 'Use for older devices or simple compatibility-focused outputs.' },
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

export function ProfilesPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const profiles = useQuery({
    queryKey: ['profiles', 'admin'],
    queryFn: api.profilesAdmin,
  });
  const runtimeSnapshot = useQuery({ queryKey: ['runtime-snapshot'], queryFn: api.runtimeSnapshot });
  const workerNodes = useQuery({ queryKey: ['worker-nodes'], queryFn: api.workerNodes });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const [form, setForm] = useState<ProfileInput>(initialProfile);
  const [editingProfileId, setEditingProfileId] = useState<number | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [showInactive, setShowInactive] = useState(false);
  const [profileSearch, setProfileSearch] = useState('');
  const [profileJson, setProfileJson] = useState(JSON.stringify(initialProfile, null, 2));
  const normalizedProfileSearch = profileSearch.trim().toLowerCase();
  const visibleProfiles = (profiles.data ?? [])
    .filter((profile) => showInactive || (!profile.disabled && !profile.deletedAt))
    .filter((profile) => !normalizedProfileSearch || [
      profile.name,
      profile.description,
      profile.container,
      profile.videoCodec,
      profile.audioCodec,
      profile.codecFamily,
      profile.preferredEncoder,
      profile.qualityMode,
      String(profile.qualityValue),
    ].some((value) => String(value ?? '').toLowerCase().includes(normalizedProfileSearch)));
  const profileWorker = resolveSelectedWorker(workerNodes.data, workerConfigString(form, 'targetWorkerName'));
  const profileWorkerEncoders = encoderNamesForWorker(profileWorker);
  const availableProfileEncoderOptions = videoEncoderOptions.filter((option) => option.value === 'auto' || profileWorkerEncoders.has(option.value));
  const defaultHardwareEncoder = availableProfileEncoderOptions.find(
    (option) => isHardwareEncoderOption(option.value) && runtimeSnapshot.data?.encoders?.[option.value]?.usable,
  )?.value ?? '';
  const qsvCapability = runtimeSnapshot.data?.encoders?.hevc_qsv;
  const qsvMain10Selected = ['p010le', 'yuv420p10le'].includes(workerConfigString(form, 'pixFmt', '').toLowerCase());
 
  const qsvRateControl = workerConfigString(
    form,
    'qsvRateControl',
    'icq',
  );
  const qsvFeatures = resolveQSVFeatures(qsvCapability, {
    main10: qsvMain10Selected,
    rateControl: qsvRateControl,
  });
  const qsvBFramesDisabled = workerConfigString(form, 'frameStructureBFrameMode', 'auto') === 'off';
  const qsvWarnings = qsvSelectionWarnings(qsvFeatures, {
    extendedBRC: workerConfigBool(form, 'qsvExtendedBRC'),
    adaptiveI: workerConfigBool(form, 'qsvAdaptiveI'),
    adaptiveB: !qsvBFramesDisabled && workerConfigBool(form, 'qsvAdaptiveB'),
  });
 
  const videoToolboxCapability = runtimeSnapshot.data?.encoders?.hevc_videotoolbox;
  const videoToolboxMain10Selected = workerConfigString(form, 'videoToolboxProfile', '').toLowerCase() === 'main10'
    || ['p010le', 'yuv420p10le'].includes(workerConfigString(form, 'pixFmt', '').toLowerCase());
  const videoToolboxPowerAvailable = videoToolboxCapability?.videoToolboxPowerEfficient === true
    && (!videoToolboxMain10Selected || videoToolboxCapability.testedModes?.videoToolboxPowerEfficientMain10 === true);

  const createProfile = useMutation({
    mutationFn: api.createProfile,
    onSuccess: async () => {
      setForm(initialProfile);
      setProfileJson(JSON.stringify(initialProfile, null, 2));
      setEditingProfileId(null);
      setShowForm(false);
      await queryClient.invalidateQueries({ queryKey: ['profiles'] });
      await queryClient.invalidateQueries({ queryKey: ['profiles', 'admin'] });
    },
  });

  const updateProfile = useMutation({
    mutationFn: api.updateProfile,
    onSuccess: async () => {
      setForm(initialProfile);
      setProfileJson(JSON.stringify(initialProfile, null, 2));
      setEditingProfileId(null);
      setShowForm(false);
      await queryClient.invalidateQueries({ queryKey: ['profiles'] });
      await queryClient.invalidateQueries({ queryKey: ['profiles', 'admin'] });
    },
  });

  const setProfileDisabled = useMutation({
    mutationFn: api.setProfileDisabled,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['profiles'] });
      await queryClient.invalidateQueries({ queryKey: ['profiles', 'admin'] });
    },
  });

  const deleteProfile = useMutation({
    mutationFn: api.deleteProfile,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['profiles'] });
      await queryClient.invalidateQueries({ queryKey: ['profiles', 'admin'] });
    },
  });
  const encoderQualityRecommendation = useMutation({ mutationFn: api.recommendEncoderQuality });
  const profileEvaluationSignature = JSON.stringify(form);
  useEffect(() => {
    if (!showForm) return;
    const timer = window.setTimeout(() => encoderQualityRecommendation.mutate({ profile: form }), 250);
    return () => window.clearTimeout(timer);
  }, [profileEvaluationSignature, showForm]);

  function updateField<K extends keyof ProfileInput>(field: K, value: ProfileInput[K]) {
    let next = { ...form, [field]: value };
    if (field === 'videoCodec' && typeof value === 'string' &&
      workerConfigString(next, 'preferredEncoder', 'software') === 'hardware' &&
      hardwareEncodersFor(codecFamilyFor(value)).length === 0) {
      next = {
        ...next,
        workerConfig: {
          ...next.workerConfig,
          preferredEncoder: 'software',
          useHardwareIfAvailable: false,
          videoEncoder: 'auto',
        },
      };
    }
    setProfileForm(field === 'videoCodec' || field === 'qualityValue' ? synchronizeAuthoritativeContract(next) : next);
  }

  function updateWorkerConfig(key: string, value: unknown) {
    const disablingHardware = key === 'useHardwareIfAvailable' && value === false;
    const next = {
      ...form,
      ...(key === 'pixFmt' ? { videoCodec: displayVideoCodec(form.videoCodec) } : {}),
      workerConfig: {
        ...form.workerConfig,
        [key]: value,
        ...(frameStructureManagedKeys.has(key) ? { frameStructureMode: 'custom' } : {}),
        ...(key === 'videoToolboxBitrateMbps' ? (() => {
          const rates = videoToolboxRatesFromTargetMbps(value);
          return { videoToolboxBitrateMbps: rates.target, videoToolboxMaxrateMbps: rates.maxrate, videoToolboxBufferMbps: rates.buffer };
        })() : {}),
        ...(['globalQuality', 'qsvRateControl', 'qsvLookAheadDepth', 'qsvExtendedBRC', 'qsvAdaptiveI', 'qsvAdaptiveB', 'qsvPStrategy', 'videoToolboxBitrateMbps', 'videoToolboxMaxrateMbps', 'videoToolboxBufferMbps', 'videoToolboxProfile', 'videoToolboxGop', 'videoToolboxRealtime', 'videoToolboxBFramePolicy', 'videoToolboxBFrames', 'videoToolboxAutoAdjustBitrate', 'videoToolboxPowerEfficiency', 'pixFmt'].includes(key) ? { hardwareQualityPreset: 'custom' } : {}),
        ...(disablingHardware ? { videoEncoder: 'libx265', preferredEncoder: 'software' } : {}),
      },
    };
    setProfileForm(['videoEncoder', 'useHardwareIfAvailable', 'pixFmt'].includes(key) ? synchronizeAuthoritativeContract(next) : next);
  }

  function updateProcessingPreference(preference: 'software' | 'hardware') {
    const currentEncoder = workerConfigString(form, 'videoEncoder', 'auto');
    const hardware = preference === 'hardware';
    const hardwareAvailable = hardwareEncodersFor(codecFamilyFor(form.videoCodec)).length > 0;
    const hardwareEncoder = isHardwareEncoderOption(currentEncoder) ? currentEncoder : defaultHardwareEncoder;
    const baseWorkerConfig = {
      ...form.workerConfig,
      preferredEncoder: preference,
      useHardwareIfAvailable: hardware && hardwareAvailable,
      videoEncoder: preference === 'software' ? 'auto' : hardwareEncoder,
      pixFmt: hardware ? defaultProfileHardwareMain10PixelFormat(hardwareEncoder) : 'yuv420p10le',
      ...(!hardware ? {
        qsvRateControl: undefined,
        qsvLookAheadDepth: undefined,
        qsvExtendedBRC: undefined,
        qsvAdaptiveI: undefined,
        qsvAdaptiveB: undefined,
        qsvPStrategy: undefined,
      } : {}),
    };
    const next = {
      ...form,
      workerConfig: hardware ? applySharedHardwareQualityPreset(baseWorkerConfig, hardwareEncoder, 'recommended') : baseWorkerConfig,
    };
    const requested = synchronizeAuthoritativeContract(next);
    setProfileForm(requested);
    if (hardware && hardwareEncoder) requestProfileHardwareRecommendation(requested);
  }

  function updateHardwareEncoder(encoder: string) {
    const requested = synchronizeAuthoritativeContract({
      ...form,
      workerConfig: applySharedHardwareQualityPreset({
        ...form.workerConfig,
        preferredEncoder: 'hardware',
        useHardwareIfAvailable: true,
        videoEncoder: encoder,
        pixFmt: defaultProfileHardwareMain10PixelFormat(encoder),
      }, encoder, 'recommended'),
    });
    setProfileForm(requested);
    requestProfileHardwareRecommendation(requested);
  }

  function applyProfileHardwareQualityPreset(preset: string, encoder: string) {
    const workerConfig = applySharedHardwareQualityPreset(form.workerConfig ?? {}, encoder, preset);
    const requested = synchronizeAuthoritativeContract({ ...form, workerConfig });
    setProfileForm(requested);
    if (preset === 'custom') return;
    requestProfileHardwareRecommendation(requested);
  }

  function requestProfileHardwareRecommendation(requested: ProfileInput) {
    encoderQualityRecommendation.mutate({ profile: requested }, {
      onSuccess: (result) => setProfileForm(synchronizeAuthoritativeContract(result.effectiveProfile)),
    });
  }

  function updateVideoToolboxCustomRate(key: 'videoToolboxBitrateMbps' | 'videoToolboxMaxrateMbps' | 'videoToolboxBufferMbps', value: number) {
    const rates = key === 'videoToolboxBitrateMbps' ? videoToolboxRatesFromTargetMbps(value) : null;
    const requested = synchronizeAuthoritativeContract({
      ...form,
      workerConfig: {
        ...form.workerConfig,
        [key]: value,
        ...(rates ? { videoToolboxBitrateMbps: rates.target, videoToolboxMaxrateMbps: rates.maxrate, videoToolboxBufferMbps: rates.buffer } : {}),
        hardwareQualityPreset: 'custom',
      },
    });
    setProfileForm(requested);
    encoderQualityRecommendation.mutate({ profile: requested });
  }

  function updateFrameStructurePolicy(patch: Record<string, unknown>) {
    const requested = synchronizeAuthoritativeContract({ ...form, workerConfig: { ...form.workerConfig, ...patch } });
    setProfileForm(requested);
    const encoder = workerConfigString(requested, 'videoEncoder', 'auto');
    if (encoder === 'hevc_qsv' || encoder === 'hevc_videotoolbox') encoderQualityRecommendation.mutate({ profile: requested });
  }

  function updateExternalSubtitleFormat(value: string) {
    const format = value === 'disabled' || value === 'srt' || value === 'ass' || value === 'remove' ? value : 'source';
    setProfileForm({
      ...form,
      preserveSubtitles: format === 'disabled' || format === 'source',
      workerConfig: {
        ...form.workerConfig,
        externalSubtitleFormat: format,
        subtitleOutputFormat: 'source',
        preferSrtSubtitles: false,
      },
    });
  }

  function updateX265Param(key: string, value: string) {
    const params = parseX265Params(workerConfigString(form, 'x265Params'));
    if (value.trim()) {
      params.set(key, value.trim());
    } else {
      params.delete(key);
    }
    updateWorkerConfig('x265Params', formatX265Params(params));
  }

  function setProfileForm(next: ProfileInput) {
    setForm(next);
    setProfileJson(JSON.stringify(next, null, 2));
  }

  function applyGuidedPreset(preset: 'archive' | 'streaming' | 'smaller' | 'original' | 'hevc-small' | 'hevc-balanced-fast' | 'hevc-archive' | 'hevc-bulk') {
    const presets: Record<typeof preset, Partial<ProfileInput>> = {
      archive: {
        name: form.name || 'Archive Quality',
        description: 'Best for preserving high-quality sources. Keeps all audio tracks, subtitles, and chapters.',
        container: 'mkv',
        videoCodec: 'x265',
        audioCodec: 'copy',
        qualityValue: 20,
        preserveHdr: true,
        preserveSubtitles: true,
        preserveChapters: true,
        workerConfig: {
          ...form.workerConfig,
          videoPreset: 'medium',
          pixFmt: 'yuv420p10le',
          videoEncoder: 'libx265',
          preferredEncoder: 'software',
          useHardwareIfAvailable: false,
          addAacStereoTrack: true,
          aacStereoDefault: false,
          preserveOriginalAudio: true,
          preferSrtSubtitles: false,
          warnSubtitleFormats: true,
        },
      },
      streaming: {
        name: form.name || 'Streaming Compatible',
        description: 'Broad playback-oriented video while still preserving all source audio tracks, subtitles, and chapters.',
        container: 'mkv',
        videoCodec: 'x264',
        audioCodec: 'copy',
        qualityValue: 22,
        preserveHdr: false,
        preserveSubtitles: true,
        preserveChapters: true,
        workerConfig: {
          ...form.workerConfig,
          videoPreset: 'medium',
          pixFmt: 'yuv420p',
          videoEncoder: 'auto',
          preferredEncoder: 'hardware',
          useHardwareIfAvailable: true,
          addAacStereoTrack: true,
          aacStereoDefault: false,
          preserveOriginalAudio: true,
          preferSrtSubtitles: true,
          warnSubtitleFormats: true,
        },
      },
      smaller: {
        name: form.name || 'Smaller File',
        description: 'Best for reducing video size while preserving all source audio tracks, subtitles, and chapters.',
        container: 'mkv',
        videoCodec: 'x265',
        audioCodec: 'copy',
        qualityValue: 25,
        preserveHdr: false,
        preserveSubtitles: true,
        preserveChapters: true,
        workerConfig: {
          ...form.workerConfig,
          videoPreset: 'slow',
          pixFmt: 'yuv420p10le',
          videoEncoder: 'libx265',
          preferredEncoder: 'software',
          useHardwareIfAvailable: false,
          addAacStereoTrack: true,
          aacStereoDefault: false,
          preserveOriginalAudio: true,
          preferSrtSubtitles: false,
          warnSubtitleFormats: true,
        },
      },
      original: {
        name: form.name || 'Preserve Original',
        description: 'Best for remuxing or keeping every stream untouched.',
        container: 'mkv',
        videoCodec: 'copy',
        audioCodec: 'copy',
        qualityValue: 0,
        preserveHdr: true,
        preserveSubtitles: true,
        preserveChapters: true,
        workerConfig: {
          ...form.workerConfig,
          videoPreset: 'medium',
          pixFmt: 'yuv420p10le',
        },
      },
      'hevc-small': {
        name: form.name || 'HEVC Small Size',
        description: 'Software x265 for assets where saving space matters more than speed.',
        container: 'mkv',
        videoCodec: 'x265',
        audioCodec: 'copy',
        qualityValue: 21,
        preserveHdr: true,
        preserveSubtitles: true,
        preserveChapters: true,
        workerConfig: {
          ...form.workerConfig,
          videoEncoder: 'libx265',
          preferredEncoder: 'software',
          useHardwareIfAvailable: false,
          videoPreset: 'slow',
          pixFmt: 'yuv420p10le',
          addAacStereoTrack: true,
          aacStereoDefault: false,
          preserveOriginalAudio: true,
          warnSubtitleFormats: true,
          preferSrtSubtitles: false,
        },
      },
      'hevc-balanced-fast': {
        name: form.name || 'HEVC Balanced Fast',
        description: 'Hardware HEVC for faster queues and lower NAS pressure.',
        container: 'mkv',
        videoCodec: 'x265',
        audioCodec: 'copy',
        qualityValue: 20,
        preserveHdr: true,
        preserveSubtitles: true,
        preserveChapters: true,
        workerConfig: {
          ...form.workerConfig,
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
          addAacStereoTrack: true,
          aacStereoDefault: false,
          preserveOriginalAudio: true,
          warnSubtitleFormats: true,
          preferSrtSubtitles: false,
        },
      },
      'hevc-archive': {
        name: form.name || 'HEVC Archive Quality',
        description: 'Software x265 for important movies, concerts, and difficult sources.',
        container: 'mkv',
        videoCodec: 'x265',
        audioCodec: 'copy',
        qualityValue: 19,
        preserveHdr: true,
        preserveSubtitles: true,
        preserveChapters: true,
        workerConfig: {
          ...form.workerConfig,
          videoEncoder: 'libx265',
          preferredEncoder: 'software',
          useHardwareIfAvailable: false,
          videoPreset: 'slow',
          pixFmt: 'yuv420p10le',
          addAacStereoTrack: true,
          aacStereoDefault: false,
          preserveOriginalAudio: true,
          warnSubtitleFormats: true,
          preferSrtSubtitles: false,
        },
      },
      'hevc-bulk': {
        name: form.name || 'HEVC Bulk Convert',
        description: 'Hardware HEVC for large libraries and unattended bulk conversion.',
        container: 'mkv',
        videoCodec: 'x265',
        audioCodec: 'copy',
        qualityValue: 23,
        preserveHdr: true,
        preserveSubtitles: true,
        preserveChapters: true,
        workerConfig: {
          ...form.workerConfig,
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
          addAacStereoTrack: true,
          aacStereoDefault: false,
          preserveOriginalAudio: true,
          warnSubtitleFormats: true,
          preferSrtSubtitles: false,
        },
      },
    };

    setProfileForm(synchronizeAuthoritativeContract({ ...form, ...presets[preset] }));
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const payload = synchronizeAuthoritativeContract({
      ...form,
      workerConfig: {
        ...form.workerConfig,
        encoder: 'ffmpeg',
        preset: form.workerConfig?.preset || form.name.toLowerCase().replaceAll(' ', '-'),
      },
    });

    if (editingProfileId) {
      updateProfile.mutate({ id: editingProfileId, ...payload });
    } else {
      createProfile.mutate(payload);
    }
  }

  function addProfile() {
    setEditingProfileId(null);
    const available = new Set((workerNodes.data ?? []).filter((worker) => worker.status === 'online').flatMap((worker) => [...encoderNamesForWorker(worker)]));
    setProfileForm(synchronizeAuthoritativeContract(applyMVForgeVideoPreferences(initialProfile, getMVForgePreferences(settings.data), available)));
    setShowForm(true);
  }

  function editProfile(profile: Profile) {
    setEditingProfileId(profile.id);
    setProfileForm(profileInputFromProfile(profile));
    setShowForm(true);
  }

  function testProfileInLab(profile: Profile) {
    navigate(`/profile-lab?videoProfileId=${profile.id}&section=video`);
  }

  function cancelEdit() {
    setForm(initialProfile);
    setProfileJson(JSON.stringify(initialProfile, null, 2));
    setEditingProfileId(null);
    setShowForm(false);
  }

  function importProfileJson() {
    try {
      const imported = JSON.parse(profileJson) as Partial<ProfileInput>;
      const next = {
        ...initialProfile,
        ...imported,
        qualityValue: Number(imported.qualityValue ?? initialProfile.qualityValue),
        preserveHdr: Boolean(imported.preserveHdr ?? initialProfile.preserveHdr),
        preserveSubtitles: Boolean(imported.preserveSubtitles ?? initialProfile.preserveSubtitles),
        preserveChapters: Boolean(imported.preserveChapters ?? initialProfile.preserveChapters),
        workerConfig:
          imported.workerConfig && typeof imported.workerConfig === 'object'
            ? imported.workerConfig
            : initialProfile.workerConfig,
      };

      const synchronized = synchronizeAuthoritativeContract(next);
      setForm(synchronized);
      setProfileJson(JSON.stringify(synchronized, null, 2));
    } catch {
      setProfileJson(JSON.stringify(form, null, 2));
    }
  }

  return (
    <>
      <PageHeader title="Profiles" eyebrow="Worker-ready presets">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Build reusable conversion profiles from human choices first. Technical encoder details stay available in Advanced.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {profiles.isError ? <Alert severity="warning">Unable to load profiles.</Alert> : null}
        <Stack direction={{ xs: 'column', md: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', md: 'center' }} sx={{ mb: 2 }} spacing={1}>
          <FormControlLabel
            control={<Checkbox checked={showInactive} onChange={(event) => setShowInactive(event.target.checked)} />}
            label="Show disabled/deleted profiles"
          />
          <TextField
            label="Search video profiles"
            value={profileSearch}
            onChange={(event) => setProfileSearch(event.target.value)}
            placeholder="Name, codec, encoder, quality…"
            size="small"
            sx={{ width: { xs: '100%', md: 360 } }}
          />
          <Button startIcon={<AddIcon />} variant="contained" onClick={addProfile}>
            Add Profile
          </Button>
        </Stack>
        <Dialog open={showForm} onClose={cancelEdit} maxWidth="lg" fullWidth>
          <DialogTitle>{editingProfileId ? 'Edit Video Profile' : 'New Video Profile'}</DialogTitle>
          <DialogContent>
            <Box component="form" onSubmit={submit}>
              <Stack spacing={3}>
                <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                  <Stack>
                    <Typography color="text.secondary" variant="body2">
                      Configure the profile with friendly defaults, then use Advanced only when needed.
                    </Typography>
                  </Stack>
                  <Button startIcon={<CloseIcon />} onClick={cancelEdit}>
                    Close
                  </Button>
                </Stack>
                <Divider />
                <Stack>
                  <Typography variant="h3">Guided Setup</Typography>
                  <Typography color="text.secondary" variant="body2">
                    Choose a starting point, then tune only what matters.
                  </Typography>
                </Stack>
                <Grid container spacing={1.5}>
                  <GuidedPresetButton
                    label="Archive Quality"
                    description="Keep quality, HDR, audio, subtitles, and chapters."
                    onClick={() => applyGuidedPreset('archive')}
                  />
                  <GuidedPresetButton
                    label="Streaming Compatible"
                    description="Broad playback support for common devices."
                    onClick={() => applyGuidedPreset('streaming')}
                  />
                  <GuidedPresetButton
                    label="Smaller File"
                    description="Reduce file size with sensible defaults."
                    onClick={() => applyGuidedPreset('smaller')}
                  />
                  <GuidedPresetButton
                    label="Preserve Original"
                    description="Keep streams untouched when possible."
                    onClick={() => applyGuidedPreset('original')}
                  />
                  <GuidedPresetButton
                    label="HEVC Small Size"
                    description="Software x265 for better savings per GB."
                    onClick={() => applyGuidedPreset('hevc-small')}
                  />
                  <GuidedPresetButton
                    label="HEVC Balanced Fast"
                    description="Hardware HEVC for fast queues."
                    onClick={() => applyGuidedPreset('hevc-balanced-fast')}
                  />
                  <GuidedPresetButton
                    label="HEVC Archive Quality"
                    description="Software x265 for important assets."
                    onClick={() => applyGuidedPreset('hevc-archive')}
                  />
                  <GuidedPresetButton
                    label="HEVC Bulk Convert"
                    description="Hardware HEVC for large libraries."
                    onClick={() => applyGuidedPreset('hevc-bulk')}
                  />
                </Grid>

                <Divider />

                <Grid container spacing={2}>
                  <Grid size={{ xs: 12 }}>
                    <Alert severity="info">
                      Choose the result you want. MVForge will translate it into encoder settings; Advanced is only for exact technical overrides.
                    </Alert>
                  </Grid>
                  <Grid size={{ xs: 12, md: 4 }}>
                    <TextField
                      label="Profile name"
                      value={form.name}
                      onChange={(event) => updateField('name', event.target.value)}
                      required
                      fullWidth
                    />
                  </Grid>
                  <Grid size={{ xs: 12, md: 4 }}>
                    <TextField label="Profile applies to" value={form.scope ?? 'asset'} onChange={(event) => updateField('scope', event.target.value as 'asset' | 'path')} select fullWidth>
                      <MenuItem value="asset">Asset</MenuItem>
                      <MenuItem value="path">Path</MenuItem>
                    </TextField>
                  </Grid>
                  <Grid size={{ xs: 12, md: 4 }}>
                    <TextField label="Quality / storage intent" value={form.optimizationIntent ?? 'balanced'} onChange={(event) => updateField('optimizationIntent', event.target.value as ProfileInput['optimizationIntent'])} select fullWidth>
                      <MenuItem value="maximum_savings">Maximum space saving</MenuItem><MenuItem value="balanced">Balanced</MenuItem><MenuItem value="conservative">Conservative quality</MenuItem><MenuItem value="maximum_quality">Maximum quality</MenuItem><MenuItem value="archive">Archive</MenuItem>
                    </TextField>
                  </Grid>
                  <Grid size={{ xs: 12 }}>
                    <TextField
                      label="What this profile is for"
                      value={form.description}
                      onChange={(event) => updateField('description', event.target.value)}
                      fullWidth
                    />
                  </Grid>
                  <Grid size={{ xs: 12 }}>
                    <Stack spacing={1.25} sx={{ minHeight: 128 }}>
                      <Typography fontWeight={700}>Preserve</Typography>
                      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                        <FormControlLabel
                          control={
                            <Checkbox
                              checked={form.preserveHdr}
                              onChange={(event) => updateField('preserveHdr', event.target.checked)}
                            />
                          }
                          label="HDR"
                        />
                        <FormControlLabel
                          control={
                            <Checkbox
                              checked={form.preserveSubtitles}
                              onChange={(event) => updateField('preserveSubtitles', event.target.checked)}
                            />
                          }
                          label="Subtitles"
                        />
                        <FormControlLabel
                          control={
                            <Checkbox
                              checked={form.preserveChapters}
                              onChange={(event) => updateField('preserveChapters', event.target.checked)}
                            />
                          }
                          label="Chapters"
                        />
                      </Stack>
                    </Stack>
                  </Grid>
                  <Grid size={{ xs: 12 }}>
                    <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, bgcolor: 'rgba(255,255,255,0.018)' }}>
                      <Grid container spacing={2} alignItems="flex-start">
                        <Grid size={{ xs: 12, md: 4 }}><TextField select label="Execution worker" value={profileWorker?.name ?? ''} onChange={(event) => updateWorkerConfig('targetWorkerName', event.target.value)} helperText={profileWorker ? `${profileWorkerEncoders.size} usable encoder(s) reported` : 'No online worker is reporting encoders'} fullWidth>{(workerNodes.data ?? []).filter((worker) => worker.status === 'online').map((worker) => <MenuItem key={worker.id} value={worker.name}>{worker.name} · {encoderNamesForWorker(worker).size} encoders</MenuItem>)}</TextField></Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Processing preference"
                            value={workerConfigString(form, 'preferredEncoder', 'software')}
                            onChange={(event) => updateProcessingPreference(event.target.value as 'software' | 'hardware')}
                            helperText="Software follows Video Codec; Hardware exposes a validated hardware encoder."
                            disabled={form.videoCodec === 'copy'}
                            select
                            fullWidth
                          >
                            <MenuItem value="software" disabled={!profileWorkerEncoders.has(softwareEncoderForVideoCodec(form.videoCodec))}>Software · match Video Codec</MenuItem>
                            <MenuItem value="hardware" disabled={hardwareEncodersFor(codecFamilyFor(form.videoCodec)).length === 0 || runtimeSnapshot.isLoading || !defaultHardwareEncoder}>Hardware</MenuItem>
                          </TextField>
                        </Grid>
                        {workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' ? <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Hardware encoder"
                            value={isHardwareEncoderOption(workerConfigString(form, 'videoEncoder', 'auto')) ? workerConfigString(form, 'videoEncoder', 'auto') : defaultHardwareEncoder}
                            onChange={(event) => updateHardwareEncoder(event.target.value)}
                            helperText={videoEncoderDescription(workerConfigString(form, 'videoEncoder', 'auto'))}
                            select
                            fullWidth
                          >
                            {availableProfileEncoderOptions.filter((option) => isHardwareEncoderOption(option.value)).map((option) => (
                              <MenuItem key={option.value} value={option.value} disabled={runtimeSnapshot.data?.encoders?.[option.value]?.usable === false}>{option.label}</MenuItem>
                            ))}
                          </TextField>
                        </Grid> : null}
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Color depth"
                            value={workerConfigString(form, 'pixFmt', workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' ? defaultProfileHardwareMain10PixelFormat(workerConfigString(form, 'videoEncoder', defaultHardwareEncoder)) : 'yuv420p10le')}
                            onChange={(event) => updateWorkerConfig('pixFmt', event.target.value)}
                            helperText={pixelFormatDescription(workerConfigString(form, 'pixFmt', 'yuv420p10le'))}
                            select
                            fullWidth
                          >
                            {compatibleProfilePixelFormats(workerConfigString(form, 'preferredEncoder', 'software'), workerConfigString(form, 'videoEncoder', defaultHardwareEncoder)).map((option) => (
                              <MenuItem key={option.value} value={option.value} disabled={hardwareTenBitUnavailable(workerConfigString(form, 'videoEncoder', defaultHardwareEncoder), option.value, runtimeSnapshot.data?.encoders)}>{option.label}</MenuItem>
                            ))}
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Final color policy"
                            value={workerConfigString(form, 'finalColorPolicy', 'preserve')}
                            onChange={(event) => updateWorkerConfig('finalColorPolicy', event.target.value)}
                            helperText="Defaults to no correction. Any selected correction is validated in the result snapshot."
                            select
                            fullWidth
                          >
                            <MenuItem value="preserve">No correction · preserve source</MenuItem>
                            <MenuItem value="automatic">Automatic correction when justified</MenuItem>
                            <MenuItem value="normalize_bt709">Normalize mathematically to BT.709</MenuItem>
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Crop aspect handling"
                            value={workerConfigString(form, 'cropAspectPolicy', 'source_sar')}
                            onChange={(event) => updateWorkerConfig('cropAspectPolicy', event.target.value)}
                            helperText="Used whenever this profile applies a crop. Source SAR avoids stretching the remaining image."
                            select
                            fullWidth
                          >
                            <MenuItem value="source_sar">Preserve source SAR (recommended)</MenuItem>
                            <MenuItem value="preserve_dar">Preserve original DAR</MenuItem>
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label={workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' ? 'Software fallback CRF' : 'Software CRF'}
                            value={form.qualityValue}
                            onChange={(event) => updateField('qualityValue', Math.min(30, Math.max(14, Number(event.target.value))))}
                            helperText={workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' ? 'Used only if hardware falls back to software.' : 'Lower preserves more detail.'}
                            type="number"
                            inputProps={{ min: 14, max: 30, step: 1 }}
                            disabled={form.videoCodec === 'copy'}
                            fullWidth
                          />
                        </Grid>
                        {workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' ? <Grid size={{ xs: 12, md: 4 }}><TextField label="Quality preset" select value={workerConfigString(form, 'hardwareQualityPreset', 'recommended')} onChange={(event) => applyProfileHardwareQualityPreset(event.target.value, workerConfigString(form, 'videoEncoder', defaultHardwareEncoder))} helperText="Recommended and Best use Main10 when supported by the selected worker." fullWidth>{hardwareQualityPresetOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}</TextField></Grid> : null}
                        <Grid size={{ xs: 12 }}>
                          <FrameStructureControls
                            config={form.workerConfig ?? {}}
                            recommendedGop={frameStructureRecommendationNumber(form.workerConfig, 'targetGopFrames')}
                            recommendedBFrames={frameStructureRecommendationNumber(form.workerConfig, 'maxBFrames')}
                            onChange={updateWorkerConfig}
                            onChangeMany={updateFrameStructurePolicy}
                            encoder={workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' ? workerConfigString(form, 'videoEncoder', defaultHardwareEncoder) : softwareEncoderForVideoCodec(form.videoCodec)}
                            disabled={form.videoCodec === 'copy'}
                          />
                        </Grid>
                        {workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' && workerConfigString(form, 'videoEncoder', defaultHardwareEncoder) !== 'hevc_videotoolbox' ? <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label={workerConfigString(form, 'videoEncoder', defaultHardwareEncoder) === 'hevc_qsv' ? 'QSV quality (ICQ)' : 'Hardware quality'}
                            value={workerConfigNumber(form, 'globalQuality', qsvQualityRangeForCrf(form.qualityValue || 20).recommended)}
                            onChange={(event) => updateWorkerConfig('globalQuality', Number(event.target.value))}
                            helperText={hardwareQualityHelper(form.qualityValue)}
                            type="number"
                            inputProps={{ min: 15, max: 35 }}
                            fullWidth
                          />
                        </Grid> : null}
                        {workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' && workerConfigString(form, 'videoEncoder', 'auto') === 'hevc_qsv' ? (
                          <>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <TextField
                                label="QSV rate control"
                                value={workerConfigString(form, 'qsvRateControl', 'icq')}
                                onChange={(event) => updateWorkerConfig('qsvRateControl', event.target.value)}
                                helperText="LA-ICQ is applied only when the worker probe confirms Look Ahead; otherwise MVForge uses ICQ."
                                select
                                fullWidth
                              >
                                <MenuItem value="icq">ICQ · safest default</MenuItem>
                                <MenuItem value="la_icq" disabled={!qsvFeatures.rateControls.laIcq}>LA-ICQ · worker validation required</MenuItem>
                              </TextField>
                            </Grid>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <TextField
                                label="QSV look-ahead depth"
                                type="number"
                                value={workerConfigNumber(form, 'qsvLookAheadDepth', 40)}
                                onChange={(event) => updateWorkerConfig('qsvLookAheadDepth', Number(event.target.value))}
                                inputProps={{ min: 10, max: 100 }} disabled={!qsvFeatures.lookAhead}
                                helperText="Used with supported extended BRC; 40 is a conservative starting point."
                                fullWidth
                              />
                            </Grid>
                            <Grid size={{ xs: 12, md: 4 }}><TextField label="P strategy" select value={workerConfigNumber(form, 'qsvPStrategy', 0)} onChange={(event) => updateWorkerConfig('qsvPStrategy', Number(event.target.value))} helperText="Available only when B-frames are Off and validated by the worker." fullWidth><MenuItem value={0}>Default</MenuItem><MenuItem value={1} disabled={workerConfigString(form, 'frameStructureBFrameMode', 'auto') !== 'off' || !qsvPStrategySupported(qsvCapability, qsvMain10Selected, 1)}>Simple · requires Off</MenuItem><MenuItem value={2} disabled={workerConfigString(form, 'frameStructureBFrameMode', 'auto') !== 'off' || !qsvPStrategySupported(qsvCapability, qsvMain10Selected, 2)}>Pyramid · requires Off</MenuItem></TextField></Grid>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <Stack>
                                <FormControlLabel
                                  control={<Checkbox disabled={!qsvFeatures.extBrc && !workerConfigBool(form, 'qsvExtendedBRC')} checked={workerConfigBool(form, 'qsvExtendedBRC')} onChange={(event) => updateWorkerConfig('qsvExtendedBRC', event.target.checked)} />}
                                  label="QSV Extended BRC"
                                />
                                <FormControlLabel
                                  control={
                                    <Checkbox
                                      disabled={!qsvFeatures.adaptiveI && !workerConfigBool(form, 'qsvAdaptiveI')}
                                      checked={workerConfigBool(form, 'qsvAdaptiveI')}
                                      onChange={(event) =>
                                        updateWorkerConfig(
                                          'qsvAdaptiveI',
                                          event.target.checked,
                                        )
                                      }
                                    />
                                  }
                                  label="QSV Adaptive I"
                                />
                                <FormControlLabel
                                  control={
                                    <Checkbox
                                      disabled={qsvBFramesDisabled || (!qsvFeatures.adaptiveB && !workerConfigBool(form, 'qsvAdaptiveB'))}
                                      checked={!qsvBFramesDisabled && workerConfigBool(form, 'qsvAdaptiveB')}
                                      onChange={(event) =>
                                        updateWorkerConfig(
                                          'qsvAdaptiveB',
                                          event.target.checked,
                                        )
                                      }
                                    />
                                  }
                                  label={qsvBFramesDisabled ? 'QSV Adaptive B · disabled by BF0' : 'QSV Adaptive B'}
                                />
                              </Stack>
                            </Grid>
                            {qsvWarnings.map((warning) => <Grid key={warning} size={{ xs: 12 }}><Alert severity="warning">{warning}</Alert></Grid>)}
                          </>
                        ) : null}
                        {workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' && workerConfigString(form, 'videoEncoder', 'auto') === 'hevc_videotoolbox' ? (
                          <>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <TextField label="VideoToolbox bitrate (Mbps)" type="number" value={workerConfigNumber(form, 'videoToolboxBitrateMbps', 2)} onChange={(event) => updateVideoToolboxCustomRate('videoToolboxBitrateMbps', Number(event.target.value))} helperText="Updates maxrate ×1.5, buffer ×2.5, and the effective estimate." inputProps={{ min: 0.01, max: 200, step: 0.01 }} fullWidth />
                            </Grid>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <TextField label="VideoToolbox maxrate (Mbps)" type="number" value={workerConfigNumber(form, 'videoToolboxMaxrateMbps', 3)} onChange={(event) => updateVideoToolboxCustomRate('videoToolboxMaxrateMbps', Number(event.target.value))} inputProps={{ min: 0.01, max: 250, step: 0.01 }} fullWidth />
                            </Grid>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <TextField label="VideoToolbox buffer (Mbps)" type="number" value={workerConfigNumber(form, 'videoToolboxBufferMbps', 5)} onChange={(event) => updateVideoToolboxCustomRate('videoToolboxBufferMbps', Number(event.target.value))} inputProps={{ min: 0.01, max: 500, step: 0.01 }} fullWidth />
                            </Grid>
                            <Grid size={{ xs: 12, md: 4 }}><TextField title="HEVC Main is 8-bit; Main10 is 10-bit and requires a compatible pixel format." label="Profile" value={workerConfigString(form, 'videoToolboxProfile', '')} onChange={(event) => updateWorkerConfig('videoToolboxProfile', event.target.value)} placeholder="main or main10" helperText="Blank follows bit depth" fullWidth /></Grid>
                            <Grid size={{ xs: 12 }}><Stack direction="row" spacing={2} flexWrap="wrap"><FormControlLabel title="Off is the default for offline conversion. Enable only for explicit low-latency work." control={<Checkbox checked={workerConfigBool(form, 'videoToolboxRealtime')} onChange={(event) => updateWorkerConfig('videoToolboxRealtime', event.target.checked)} />} label="Realtime" /><FormControlLabel title="Adjust target, maxrate and buffer for the effective B-frame strategy." control={<Checkbox checked={workerConfigBool(form, 'videoToolboxAutoAdjustBitrate')} onChange={(event) => updateWorkerConfig('videoToolboxAutoAdjustBitrate', event.target.checked)} />} label="Auto-adjust bitrate for encoder strategy" /><FormControlLabel title="Available only after the matching VideoToolbox Main/Main10 power-efficiency probe succeeds." control={<Checkbox disabled={!videoToolboxPowerAvailable} checked={workerConfigBool(form, 'videoToolboxPowerEfficiency')} onChange={(event) => updateWorkerConfig('videoToolboxPowerEfficiency', event.target.checked)} />} label="Power efficiency" /></Stack></Grid>
                          </>
                        ) : null}
                        {encoderQualityRecommendation.isError ? <Grid size={{ xs: 12 }}><Alert severity="warning">Quality recommendation failed: {encoderQualityRecommendation.error instanceof Error ? encoderQualityRecommendation.error.message : 'unknown error'}</Alert></Grid> : null}
                        {encoderQualityRecommendation.data ? <Grid size={{ xs: 12 }}><Stack spacing={1}><Alert severity={encoderQualityRecommendation.data.recommendation.warnings.length ? 'warning' : 'success'}>Effective {encoderQualityRecommendation.data.recommendation.effectiveRateControl || 'bitrate'} · confidence {encoderQualityRecommendation.data.recommendation.estimateConfidence}{encoderQualityRecommendation.data.recommendation.rateControlFallback ? ` · ${encoderQualityRecommendation.data.recommendation.rateControlFallback}` : ''}</Alert><Typography component="code" variant="caption" sx={{ overflowWrap: 'anywhere' }}>FFmpeg video: {encoderQualityRecommendation.data.ffmpegVideoArguments.join(' ')}</Typography></Stack></Grid> : null}
                        {encoderQualityRecommendation.data?.recommendation.effectiveBFramePolicy ? <Grid size={{ xs: 12 }}><Alert severity={encoderQualityRecommendation.data.recommendation.bFrameDowngradeReason ? 'warning' : 'info'}>VideoToolbox B-frames: {encoderQualityRecommendation.data.recommendation.requestedBFramePolicy} → {encoderQualityRecommendation.data.recommendation.effectiveBFramePolicy} · efficiency ×{encoderQualityRecommendation.data.recommendation.bFrameEfficiencyMultiplier?.toFixed(2)} · target {((encoderQualityRecommendation.data.recommendation.targetBitrate ?? 0) / 1_000_000).toFixed(2)} Mbps{encoderQualityRecommendation.data.recommendation.bFrameDowngradeReason ? ` · ${encoderQualityRecommendation.data.recommendation.bFrameDowngradeReason}` : ''}</Alert></Grid> : null}
                        <Grid size={{ xs: 12, md: 4 }}>
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={aacTrackEnabled(form)}
                                onChange={(event) => updateWorkerConfig('addAacStereoTrack', event.target.checked)}
                              />
                            }
                            label="Add AAC stereo track"
                          />
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="AAC compatibility quality"
                            value={workerConfigNumber(form, 'aacStereoBitrateKbps', 192)}
                            onChange={(event) => updateWorkerConfig('aacStereoBitrateKbps', Number(event.target.value))}
                            disabled={!aacTrackEnabled(form)}
                            helperText="192 kb/s is recommended for movies."
                            select
                            fullWidth
                          >
                            {[128, 160, 192, 224, 256].map((value) => (
                              <MenuItem key={value} value={value}>{value} kb/s</MenuItem>
                            ))}
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={aacTrackDefault(form)}
                                disabled={!aacTrackEnabled(form)}
                                onChange={(event) => updateWorkerConfig('aacStereoDefault', event.target.checked)}
                              />
                            }
                            label="Make AAC stereo default"
                          />
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={workerConfigBool(form, 'preserveOriginalAudio', true)}
                                onChange={(event) => {
                                  updateWorkerConfig('preserveOriginalAudio', event.target.checked);
                                  if (event.target.checked) {
                                    updateWorkerConfig('aacStereoDefault', true);
                                  }
                                }}
                              />
                            }
                            label="Keep original audio as secondary (AAC is default)"
                          />
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="AAC source stream index"
                            type="number"
                            value={workerConfigNumber(form, 'aacStereoSourceStreamIndex', -1)}
                            onChange={(event) => updateWorkerConfig('aacStereoSourceStreamIndex', Number(event.target.value))}
                            disabled={!aacTrackEnabled(form)}
                            inputProps={{ min: -1 }}
                            helperText="-1 = automatic/default. LAB can select a track from an analyzed asset."
                            fullWidth
                          />
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            select
                            fullWidth
                            label="Convert all subtitles"
                            value={externalSubtitleFormat(form)}
                            onChange={(event) => updateExternalSubtitleFormat(event.target.value)}
                            helperText="Creates validated sidecars and removes embedded tracks. A Tracks Profile takes priority."
                          >
                            <MenuItem value="disabled">Disabled · defer to Tracks Profile</MenuItem>
                            <MenuItem value="source">Keep embedded tracks</MenuItem>
                            <MenuItem value="srt">External SRT · remove embedded</MenuItem>
                            <MenuItem value="ass">External ASS · remove embedded</MenuItem>
                            <MenuItem value="remove">Remove embedded tracks</MenuItem>
                          </TextField>
                        </Grid>
                        {externalSubtitleFormat(form) === 'srt' || externalSubtitleFormat(form) === 'ass' ? <>
                          <Grid size={{ xs: 12, md: 4 }}>
                            <TextField select fullWidth label="Bitmap OCR quality" value={workerConfigString(form, 'subtitleOCRMode', 'accurate')} onChange={(event) => updateWorkerConfig('subtitleOCRMode', event.target.value)} helperText="Accurate compares isolated-color and original-color OCR passes.">
                              <MenuItem value="raw">Raw · one pass</MenuItem><MenuItem value="clean">Clean · corrected</MenuItem><MenuItem value="accurate">Accurate · two passes</MenuItem>
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, md: 4 }}>
                            <TextField select fullWidth label="Bitmap OCR language" value={workerConfigString(form, 'subtitleOCRLanguage', 'auto')} onChange={(event) => updateWorkerConfig('subtitleOCRLanguage', event.target.value)} helperText="Automatic uses the language metadata of each subtitle track.">
                              <MenuItem value="auto">Automatic per track</MenuItem><MenuItem value="eng">English</MenuItem><MenuItem value="spa">Spanish</MenuItem><MenuItem value="jpn">Japanese</MenuItem><MenuItem value="jpn_vert">Japanese vertical</MenuItem>
                            </TextField>
                          </Grid>
                        </> : null}
                      </Grid>
                    </Box>
                  </Grid>
                  <Grid size={{ xs: 12 }}>
                    <Accordion variant="outlined" sx={{ bgcolor: 'transparent' }}>
                      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                        <Stack>
                          <Typography fontWeight={700}>Advanced</Typography>
                          <Typography color="text.secondary" variant="body2">
                            CRF, codecs, tune, x265 params, profile JSON, and command preview.
                          </Typography>
                        </Stack>
                      </AccordionSummary>
                      <AccordionDetails>
                        <Grid container spacing={2}>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="Container"
                              value={form.container}
                              onChange={(event) => updateField('container', event.target.value)}
                              select
                              fullWidth
                            >
                              {['mkv', 'mp4'].map((value) => (
                                <MenuItem key={value} value={value}>
                                  {value}
                                </MenuItem>
                              ))}
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="Video codec"
                              value={displayVideoCodec(form.videoCodec)}
                              onChange={(event) => updateField('videoCodec', event.target.value)}
                              select
                              fullWidth
                            >
                              <MenuItem value="x264">H.264 / x264</MenuItem>
                              <MenuItem value="x265">HEVC / x265</MenuItem>
                              <MenuItem value="copy">Keep original video</MenuItem>
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="Preset"
                              value={workerConfigString(form, 'videoPreset', 'medium')}
                              onChange={(event) => updateWorkerConfig('videoPreset', event.target.value)}
                              helperText={encoderPresetDescription(workerConfigString(form, 'videoPreset', 'medium'))}
                              disabled={form.videoCodec === 'copy'}
                              select
                              fullWidth
                            >
                              {encoderPresetOptions.map((option) => (
                                <MenuItem key={option.value} value={option.value}>
                                  {option.label}
                                </MenuItem>
                              ))}
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="Deinterlacing"
                              value={workerConfigString(form, 'deinterlaceMode', 'auto')}
                              onChange={(event) => updateWorkerConfig('deinterlaceMode', event.target.value)}
                              helperText={workerConfigString(form, 'deinterlaceMode', 'auto') === 'force'
                                ? 'bwdif=mode=send_frame:parity=auto:deint=all; output is marked progressive automatically.'
                                : 'Auto analyzes the asset before encoding and updates output field metadata when a correction is applied.'}
                              disabled={form.videoCodec === 'copy'}
                              select
                              fullWidth
                            >
                              <MenuItem value="auto">Auto at conversion (uses Analysis)</MenuItem>
                              <MenuItem value="off">Off</MenuItem>
                              <MenuItem value="force">Force · bwdif (single-rate)</MenuItem>
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="Tune"
                              value={workerConfigString(form, 'tune')}
                              onChange={(event) => updateWorkerConfig('tune', event.target.value)}
                              select
                              fullWidth
                            >
                              <MenuItem value="">Auto</MenuItem>
                              <MenuItem value="film">Film</MenuItem>
                              <MenuItem value="animation">Animation</MenuItem>
                              <MenuItem value="grain">Grain</MenuItem>
                              <MenuItem value="fastdecode">Fast decode</MenuItem>
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="AQ mode"
                              value={x265ParamValue(form, 'aq-mode')}
                              onChange={(event) => updateX265Param('aq-mode', event.target.value)}
                              placeholder="3"
                              fullWidth
                            />
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="AQ strength"
                              value={x265ParamValue(form, 'aq-strength')}
                              onChange={(event) => updateX265Param('aq-strength', event.target.value)}
                              placeholder="0.9"
                              fullWidth
                            />
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <TextField
                              label="psy-rd"
                              value={x265ParamValue(form, 'psy-rd')}
                              onChange={(event) => updateX265Param('psy-rd', event.target.value)}
                              placeholder="2.0"
                              fullWidth
                            />
                          </Grid>
                          <Grid size={{ xs: 12, md: 6 }}>
                            <TextField
                              label="x265 params"
                              value={workerConfigString(form, 'x265Params')}
                              onChange={(event) => updateWorkerConfig('x265Params', event.target.value)}
                              placeholder="aq-mode=3:aq-strength=0.9:deblock=-1,-1"
                              fullWidth
                            />
                          </Grid>
                          <Grid size={{ xs: 12, md: 7 }}>
                            <Stack spacing={1.5}>
                              <TextField
                                label="Profile JSON"
                                value={profileJson}
                                onChange={(event) => setProfileJson(event.target.value)}
                                multiline
                                minRows={10}
                                fullWidth
                              />
                              <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                                <Button startIcon={<FileUploadIcon />} variant="outlined" onClick={importProfileJson}>
                                  Import Into Form
                                </Button>
                                <Button
                                  startIcon={<ContentCopyIcon />}
                                  variant="outlined"
                                  onClick={() => setProfileJson(JSON.stringify(form, null, 2))}
                                >
                                  Export Current Form
                                </Button>
                              </Stack>
                            </Stack>
                          </Grid>
                          <Grid size={{ xs: 12, md: 5 }}>
                            <Stack spacing={1.5}>
                              <Typography variant="h3">Dry-run Command</Typography>
                              <Box
                                component="pre"
                                sx={{
                                  border: 1,
                                  borderColor: 'divider',
                                  borderRadius: 1,
                                  bgcolor: 'rgba(255,255,255,0.03)',
                                  m: 0,
                                  p: 2,
                                  whiteSpace: 'pre-wrap',
                                  wordBreak: 'break-word',
                                }}
                              >
                                {buildDryRunCommand(form)}
                              </Box>
                            </Stack>
                          </Grid>
                        </Grid>
                      </AccordionDetails>
                    </Accordion>
                  </Grid>
                  <Grid size={{ xs: 12, md: 3 }}>
                    <Button
                      type="submit"
                      startIcon={editingProfileId ? <EditIcon /> : <AddIcon />}
                      variant="contained"
                      disabled={createProfile.isPending || updateProfile.isPending}
                      fullWidth
                    >
                      {editingProfileId ? 'Save Changes' : 'Create Profile'}
                    </Button>
                  </Grid>
                </Grid>
              </Stack>
            </Box>
            {createProfile.isError ? (
              <Alert severity="warning" sx={{ mt: 2 }}>
                Profile could not be created. Profile names must be unique.
              </Alert>
            ) : null}
            {updateProfile.isError ? (
              <Alert severity="warning" sx={{ mt: 2 }}>
                Profile could not be updated. Verify the name and encoder settings.
              </Alert>
            ) : null}
          </DialogContent>
        </Dialog>
        <Card>
          <Box sx={{ overflowX: 'auto' }}>
            <Table size="small" sx={{ minWidth: 920 }}>
              <TableHead>
                <TableRow>
                  <TableCell>Profile</TableCell>
                  <TableCell>Purpose</TableCell>
                  <TableCell>Format</TableCell>
                  <TableCell>Preserve</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {visibleProfiles.map((profile) => (
                  <TableRow key={profile.id} hover>
                    <TableCell>
                      <Stack spacing={0.5}>
                        <Typography fontWeight={700}>{profile.name}</Typography>
                        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
						  <Chip label={profile.scope === 'path' ? 'Path profile' : 'Asset profile'} size="small" color="info" variant="outlined" />
                          {profile.optimizationIntent ? <Chip label={profile.optimizationIntent.replaceAll('_', ' ')} size="small" variant="outlined" /> : null}
                          <Chip label={`${profile.qualityMode.toUpperCase()} ${profile.qualityValue}`} size="small" />
                          <Chip label={profile.videoCodec === 'copy' ? 'Original quality' : `CRF ${profile.qualityValue}`} size="small" />
                          {profile.disabled ? <Chip label="Disabled" size="small" color="warning" /> : null}
                          {profile.deletedAt ? <Chip label="Deleted" size="small" color="default" /> : null}
                          {profileFrameRecommendation(profile) ? <Chip label={`GOP ${profileFrameRecommendation(profile)?.targetGopFrames} · B max ${profileFrameRecommendation(profile)?.maxBFrames}`} size="small" color="info" variant="outlined" /> : null}
                          {profileFrameValidation(profile) ? <Chip label={`Frame validation: ${profileFrameValidation(profile)?.verdict}`} size="small" color={profileFrameValidation(profile)?.verdict === 'safe' ? 'success' : profileFrameValidation(profile)?.verdict === 'reject' ? 'error' : 'warning'} /> : null}
                        </Stack>
                      </Stack>
                    </TableCell>
                    <TableCell>
                      <Typography color="text.secondary" variant="body2">
                        {profile.description || 'No description'}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                        <Chip label={profile.container.toUpperCase()} size="small" />
                        <Chip label={profile.codecFamily || profile.videoCodec} size="small" color="primary" />
                        <Chip label={`${profile.preferredEncoder || 'legacy'} · ${profile.encoderPolicy || 'unresolved'}`} size="small" variant="outlined" />
                        <Chip label={profile.audioCodec} size="small" color="secondary" />
                      </Stack>
                    </TableCell>
                    <TableCell>
                      <Typography color="text.secondary" variant="body2">
                        {preservationSummary(profile)}
                      </Typography>
                    </TableCell>
                    <TableCell align="right">
                      <Stack direction="row" spacing={1} justifyContent="flex-end" flexWrap="wrap" useFlexGap>
                        <Button startIcon={<EditIcon />} variant="outlined" onClick={() => editProfile(profile)} disabled={Boolean(profile.deletedAt)}>
                          Edit
                        </Button>
                        <Button startIcon={<ScienceIcon />} variant="outlined" onClick={() => testProfileInLab(profile)} disabled={Boolean(profile.deletedAt)}>
                          Probar en Lab
                        </Button>
                        <Button
                          variant="outlined"
                          color={profile.disabled ? 'success' : 'warning'}
                          disabled={Boolean(profile.deletedAt) || setProfileDisabled.isPending}
                          onClick={() => setProfileDisabled.mutate({ id: profile.id, disabled: !profile.disabled })}
                        >
                          {profile.disabled ? 'Enable' : 'Disable'}
                        </Button>
                        <Button
                          startIcon={<DeleteOutlineIcon />}
                          variant="outlined"
                          color="error"
                          disabled={Boolean(profile.deletedAt) || deleteProfile.isPending}
                          onClick={() => deleteProfile.mutate(profile.id)}
                        >
                          Delete
                        </Button>
                      </Stack>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </Box>
          {!profiles.isLoading && visibleProfiles.length === 0 ? (
            <CardContent>
              <Alert severity="info">{normalizedProfileSearch ? 'No video profiles match this search.' : 'No profiles have been configured yet.'}</Alert>
            </CardContent>
          ) : null}
        </Card>
      </Box>
    </>
  );
}

function GuidedPresetButton({
  label,
  description,
  onClick,
}: {
  label: string;
  description: string;
  onClick: () => void;
}) {
  return (
    <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
      <Button
        variant="outlined"
        onClick={onClick}
        title={description}
        fullWidth
        sx={{ minHeight: 48, justifyContent: 'flex-start' }}
      >
        {label}
      </Button>
    </Grid>
  );
}

function preservationSummary(profile: Profile) {
  const preserved = [
    profile.preserveHdr ? 'HDR' : null,
    profile.preserveSubtitles ? 'Subtitles' : null,
    profile.preserveChapters ? 'Chapters' : null,
  ].filter(Boolean);

  return preserved.length ? preserved.join(', ') : 'None';
}

function profileFrameRecommendation(profile: Profile) {
  const value = profile.workerConfig?.frameStructureRecommendation;
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const recommendation = value as Record<string, unknown>;
  const targetGopFrames = Number(recommendation.targetGopFrames);
  const maxBFrames = Number(recommendation.maxBFrames);
  return Number.isFinite(targetGopFrames) && Number.isFinite(maxBFrames) ? { targetGopFrames, maxBFrames } : undefined;
}

function frameStructureRecommendationNumber(config: Record<string, unknown> | undefined, key: 'targetGopFrames' | 'maxBFrames') {
  const recommendation = config?.frameStructureRecommendation;
  if (!recommendation || typeof recommendation !== 'object' || Array.isArray(recommendation)) return undefined;
  const value = Number((recommendation as Record<string, unknown>)[key]);
  return Number.isFinite(value) && value > 0 ? value : undefined;
}

function profileFrameValidation(profile: Profile) {
  const value = profile.workerConfig?.frameStructureValidation;
  if (!value || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const verdict = String((value as Record<string, unknown>).verdict || '');
  return verdict === 'safe' || verdict === 'review' || verdict === 'reject' ? { verdict } : undefined;
}

function workerConfigString(profile: ProfileInput, key: string, fallback = '') {
  const value = profile.workerConfig?.[key];
  return typeof value === 'string' ? value : fallback;
}

function subtitleOutputFormat(profile: ProfileInput) {
  const configured = workerConfigString(profile, 'subtitleOutputFormat').toLowerCase();
  if (configured === 'srt' || configured === 'ass' || configured === 'source') {
    return configured;
  }
  return workerConfigBool(profile, 'preferSrtSubtitles') ? 'srt' : 'source';
}

function externalSubtitleFormat(profile: ProfileInput) {
  const configured = workerConfigString(profile, 'externalSubtitleFormat').toLowerCase();
  if (configured === 'disabled' || configured === 'srt' || configured === 'ass' || configured === 'source' || configured === 'remove') {
    return configured;
  }
  return subtitleOutputFormat(profile);
}

function synchronizeAuthoritativeContract(profile: ProfileInput): ProfileInput {
  const workerConfig = { ...profile.workerConfig };
  delete workerConfig.processingMode;
  profile = { ...profile, workerConfig };
  const codecFamily = codecFamilyFor(profile.videoCodec);
  const qualityStrategy = profile.videoCodec === 'copy' ? 'source' : 'crf';
  if (codecFamily === 'copy') {
    return {
      ...profile,
      codecFamily,
      encoderPolicy: 'locked',
      preferredEncoder: 'copy',
      allowedEncoders: ['copy'],
      fallbackPolicy: 'wait',
      bitDepth: 0,
      pixelFormat: '',
      qualityStrategy: 'source',
    };
  }

  const configured = workerConfigString(profile, 'videoEncoder', 'auto');
  const software = codecFamily === 'h264' ? 'libx264' : codecFamily === 'av1' ? 'libsvtav1' : 'libx265';
  const hardware = hardwareEncodersFor(codecFamily);
  const hardwareAllowed = workerConfigBool(profile, 'useHardwareIfAvailable') && hardware.length > 0;
  const pixelFormat = workerConfigString(profile, 'pixFmt', profile.pixelFormat || 'yuv420p');
  const normalizedPixelFormat = pixelFormat.toLowerCase();
  const bitDepth = normalizedPixelFormat === 'auto'
    ? 0
    : normalizedPixelFormat.includes('10') || normalizedPixelFormat.includes('p010')
      ? 10
      : 8;

  if (configured === 'auto' || configured === 'ffmpeg') {
    return hardwareAllowed
      ? {
          ...profile,
          codecFamily,
          encoderPolicy: 'automatic',
          preferredEncoder: hardware[0] || software,
          allowedEncoders: [...hardware, software],
          fallbackPolicy: 'allowed_only',
          bitDepth,
          pixelFormat,
          qualityStrategy,
        }
      : {
          ...profile,
          codecFamily,
          encoderPolicy: 'locked',
          preferredEncoder: software,
          allowedEncoders: [software],
          fallbackPolicy: 'wait',
          bitDepth,
          pixelFormat,
          qualityStrategy,
        };
  }

  const isHardware = hardware.includes(configured);
  return {
    ...profile,
    codecFamily,
    encoderPolicy: isHardware && hardwareAllowed ? 'restricted' : 'locked',
    preferredEncoder: configured,
    allowedEncoders: isHardware && hardwareAllowed ? [configured, software] : [configured],
    fallbackPolicy: isHardware && hardwareAllowed ? 'allowed_only' : 'wait',
    bitDepth,
    pixelFormat,
    qualityStrategy,
  };
}

function codecFamilyFor(videoCodec: string) {
  const value = videoCodec.toLowerCase();
  if (value === 'copy') return 'copy';
  if (value.includes('265') || value.includes('hevc')) return 'hevc';
  if (value.includes('264') || value === 'h264') return 'h264';
  if (value.includes('av1')) return 'av1';
  return value;
}

function hardwareEncodersFor(codecFamily: string) {
  if (codecFamily === 'hevc') return ['hevc_qsv', 'hevc_vaapi', 'hevc_nvenc', 'hevc_videotoolbox', 'hevc_amf'];
  return [];
}

function workerConfigBool(profile: ProfileInput, key: string, fallback = false) {
  const value = profile.workerConfig?.[key];
  if (typeof value === 'boolean') {
    return value;
  }
  if (typeof value === 'string') {
    return ['true', '1', 'yes', 'enabled', 'on'].includes(value.toLowerCase());
  }
  return fallback;
}

function aacTrackEnabled(profile: ProfileInput) {
  return profile.workerConfig && 'addAacStereoTrack' in profile.workerConfig
    ? workerConfigBool(profile, 'addAacStereoTrack')
    : workerConfigBool(profile, 'addAacStereoDefault');
}

function aacTrackDefault(profile: ProfileInput) {
  return workerConfigBool(profile, 'aacStereoDefault');
}

function workerConfigNumber(profile: ProfileInput, key: string, fallback: number) {
  const value = profile.workerConfig?.[key];
  const parsed = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : NaN;
  return Number.isFinite(parsed) ? parsed : fallback;
}

function x265ParamValue(profile: ProfileInput, key: string) {
  return parseX265Params(workerConfigString(profile, 'x265Params')).get(key) ?? '';
}

function parseX265Params(value: string) {
  const params = new Map<string, string>();
  value
    .split(':')
    .map((part) => part.trim())
    .filter(Boolean)
    .forEach((part) => {
      const [key, ...rest] = part.split('=');
      if (key) {
        params.set(key, rest.join('='));
      }
    });
  return params;
}

function formatX265Params(params: Map<string, string>) {
  return Array.from(params.entries())
    .filter(([key, value]) => key.trim() && value.trim())
    .map(([key, value]) => `${key.trim()}=${value.trim()}`)
    .join(':');
}

function encoderPresetDescription(value: string) {
  return encoderPresetOptions.find((option) => option.value === value)?.description ?? 'Controls how much time FFmpeg spends compressing video.';
}

function profileInputFromProfile(profile: Profile): ProfileInput {
  const normalized = normalizeLegacyVideoCodec(
    profile.videoCodec,
    profile.workerConfig,
  );
  return {
    name: profile.name,
    scope: profile.scope,
    description: profile.description,
    container: profile.container,
    videoCodec: normalized.videoCodec,
    codecFamily: profile.codecFamily,
    encoderPolicy: profile.encoderPolicy,
    preferredEncoder: profile.preferredEncoder,
    allowedEncoders: [...(profile.allowedEncoders ?? [])],
    fallbackPolicy: profile.fallbackPolicy,
    bitDepth: profile.bitDepth,
    pixelFormat: profile.pixelFormat,
    qualityStrategy: profile.qualityStrategy,
    optimizationIntent: profile.optimizationIntent,
    audioCodec: profile.audioCodec,
    qualityMode: profile.qualityMode,
    qualityValue: profile.qualityValue,
    preserveHdr: profile.preserveHdr,
    preserveSubtitles: profile.preserveSubtitles,
    preserveChapters: profile.preserveChapters,
    workerConfig: normalized.workerConfig,
    disabled: profile.disabled,
  };
}

function pixelFormatDescription(value: string) {
  return pixelFormatOptions.find((option) => option.value === value)?.description ?? 'Controls output color depth and playback compatibility.';
}

function defaultProfileHardwareMain10PixelFormat(encoder: string) {
  return encoder === 'hevc_qsv' || encoder === 'hevc_vaapi' || encoder === 'hevc_videotoolbox' ? 'p010le' : 'yuv420p10le';
}

function compatibleProfilePixelFormats(preference: string, encoder: string) {
  if (preference !== 'hardware') {
    return pixelFormatOptions.filter((option) => ['auto', 'yuv420p10le', 'yuv420p'].includes(option.value));
  }
  if (encoder === 'hevc_qsv' || encoder === 'hevc_vaapi') {
    return pixelFormatOptions.filter((option) => ['auto', 'p010le', 'nv12'].includes(option.value));
  }
  if (encoder === 'hevc_videotoolbox') {
    return pixelFormatOptions.filter((option) => ['auto', 'p010le', 'yuv420p'].includes(option.value));
  }
  return pixelFormatOptions.filter((option) => ['auto', 'yuv420p10le', 'yuv420p'].includes(option.value));
}

function hardwareTenBitUnavailable(encoder: string, pixelFormat: string, encoders?: Record<string, { main10?: boolean; videoToolboxMain10?: boolean }>) {
  if (pixelFormat !== 'p010le') return false;
  if (encoder === 'hevc_qsv') return encoders?.hevc_qsv?.main10 === false;
  if (encoder === 'hevc_videotoolbox') return encoders?.hevc_videotoolbox?.videoToolboxMain10 === false;
  return false;
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

function hardwareQualityHelper(softwareCRF: number) {
  return qsvQualityHelper(softwareCRF);
}

function buildDryRunCommand(profile: ProfileInput) {
  const input = '<input>';
  const output = `<output>.${profile.container}`;
  const encoder = workerConfigString(profile, 'videoEncoder', 'auto');
  const hardwareAvailable = hardwareEncodersFor(codecFamilyFor(profile.videoCodec)).length > 0;
  const resolvedEncoder = encoder === 'auto'
    ? (workerConfigBool(profile, 'useHardwareIfAvailable') && hardwareAvailable ? 'auto-hardware' : softwareEncoderForVideoCodec(profile.videoCodec))
    : encoder;
  const isHardware = ['hevc_qsv', 'hevc_nvenc', 'hevc_videotoolbox', 'hevc_amf', 'auto-hardware'].includes(resolvedEncoder);
  const isVideoToolbox = resolvedEncoder === 'hevc_videotoolbox';
  const videoArgs = profile.videoCodec === 'copy' ? '-c:v copy' : `-c:v ${resolvedEncoder}`;
  const presetArgs = profile.videoCodec === 'copy' || isHardware ? '' : `-preset ${workerConfigString(profile, 'videoPreset', 'medium')}`;
  const configuredPixFmt = workerConfigString(profile, 'pixFmt', 'yuv420p10le');
  const pixFmtArgs = profile.videoCodec === 'copy' || configuredPixFmt === 'auto' ? '' : isVideoToolbox ? `-profile:v ${profile.bitDepth === 10 || profile.videoCodec.includes('10bit') ? 'main10' : 'main'} -pix_fmt ${profile.bitDepth === 10 || profile.videoCodec.includes('10bit') ? 'p010le' : 'yuv420p'}` : `-pix_fmt ${configuredPixFmt}`;
  const tuneArgs = profile.videoCodec === 'copy' || !workerConfigString(profile, 'tune') ? '' : `-tune ${workerConfigString(profile, 'tune')}`;
  const effectiveX265 = effectiveDryRunX265Params(profile);
  const x265Args = profile.videoCodec === 'copy' || isHardware || !effectiveX265 ? '' : `-x265-params ${effectiveX265}`;
  const hardwareQualityArgs = isVideoToolbox
    ? `-b:v ${workerConfigNumber(profile, 'videoToolboxBitrateMbps', 2)}M -maxrate ${workerConfigNumber(profile, 'videoToolboxMaxrateMbps', 3)}M -bufsize ${workerConfigNumber(profile, 'videoToolboxBufferMbps', 5)}M`
    : isHardware ? `-global_quality ${workerConfigNumber(profile, 'globalQuality', qsvQualityRangeForCrf(profile.qualityValue || 20).recommended)}` : '';
  const qsvArgs = resolvedEncoder === 'hevc_qsv'
    ? [
        workerConfigString(profile, 'qsvRateControl', 'icq') === 'la_icq' ? `-look_ahead 1 -look_ahead_depth ${workerConfigNumber(profile, 'qsvLookAheadDepth', 40)}` : '',
        workerConfigBool(profile, 'qsvExtendedBRC') ? '-extbrc 1' : '',
        workerConfigBool(profile, 'qsvAdaptiveI') ? '-adaptive_i 1' : '',
        workerConfigBool(profile, 'qsvAdaptiveB') ? '-adaptive_b 1' : '',
        workerConfigNumber(profile, 'qsvPStrategy', 0) > 0 && workerConfigString(profile, 'frameStructureBFrameMode', 'auto') === 'off' ? `-p_strategy ${Math.min(2, workerConfigNumber(profile, 'qsvPStrategy', 0))}` : '',
      ].filter(Boolean).join(' ')
    : '';
  const audioArgs = profile.audioCodec === 'copy' ? '-c:a copy' : `-c:a ${profile.audioCodec}`;
  const aacDisposition = aacTrackDefault(profile) ? 'default' : '0';
  const aacArgs = aacTrackEnabled(profile) ? `-map 0:a:0 -c:a:1 aac -b:a:1 ${workerConfigNumber(profile, 'aacStereoBitrateKbps', 192)}k -ac:a:1 2 -disposition:a:1 ${aacDisposition}` : '';
  const subtitleFormat = subtitleOutputFormat(profile);
  const subtitleArgs = profile.preserveSubtitles ? (subtitleFormat === 'source' ? '-c:s copy' : `-c:s ${subtitleFormat}`) : '-sn';
  const chapterArgs = profile.preserveChapters ? '-map_chapters 0' : '-map_chapters -1';
  const hdrArgs = profile.preserveHdr ? '-map_metadata 0' : '-map_metadata -1';
  const qualityArgs = profile.qualityMode === 'crf' && profile.videoCodec !== 'copy' && !isHardware ? `-crf ${profile.qualityValue}` : '';
  const gopMode = workerConfigString(profile, 'frameStructureGopMode', 'auto');
  const bFrameMode = workerConfigString(profile, 'frameStructureBFrameMode', 'auto');
  const frameArgs = profile.videoCodec === 'copy' ? '' : [
    gopMode === 'recommended' || gopMode === 'custom' ? `-g ${Math.max(1, Math.min(1000, workerConfigNumber(profile, 'frameStructureGopFrames', 120)))}` : '',
    bFrameMode === 'recommended' || bFrameMode === 'custom' ? `-bf ${Math.max(1, Math.min(isVideoToolbox ? 4 : 16, workerConfigNumber(profile, 'frameStructureMaxBFrames', 3)))}` : bFrameMode === 'off' ? '-bf 0' : '',
  ].filter(Boolean).join(' ');

  return ['ffmpeg', '-i', input, videoArgs, presetArgs, pixFmtArgs, tuneArgs, x265Args, hardwareQualityArgs, qsvArgs, frameArgs, audioArgs, aacArgs, subtitleArgs, chapterArgs, hdrArgs, qualityArgs, output]
    .filter(Boolean)
    .join(' ');
}

function effectiveDryRunX265Params(profile: ProfileInput) {
  const params = parseX265Params(workerConfigString(profile, 'x265Params'));
  const gopMode = workerConfigString(profile, 'frameStructureGopMode', 'auto');
  const bFrameMode = workerConfigString(profile, 'frameStructureBFrameMode', 'auto');
  if (gopMode === 'recommended' || gopMode === 'custom') {
    params.delete('keyint');
    params.delete('min-keyint');
  }
  if (gopMode !== 'custom') params.delete('scenecut');
  if (bFrameMode !== 'auto') params.delete('bframes');
  if (bFrameMode !== 'custom') {
    params.delete('b-adapt');
    params.delete('b-pyramid');
  }
  return formatX265Params(params);
}

function softwareEncoderForVideoCodec(videoCodec: string) {
  const family = codecFamilyFor(videoCodec);
  if (family === 'h264') return 'libx264';
  if (family === 'hevc') return 'libx265';
  if (family === 'av1') return 'libsvtav1';
  return family === 'copy' ? 'copy' : videoCodec;
}
