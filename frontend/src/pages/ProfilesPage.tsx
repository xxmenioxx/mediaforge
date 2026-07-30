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
  Slider,
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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import type { Profile, ProfileInput } from '../api/types';
import { qsvQualityHelper, qsvQualityRangeForCrf } from '../utils/qsv';

const initialProfile: ProfileInput = {
  name: '',
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
    videoToolboxBitrateMbps: 6,
    videoToolboxMaxrateMbps: 8,
    videoToolboxBufferMbps: 12,
    preferredEncoder: 'software',
    addAacStereoTrack: false,
    aacStereoBitrateKbps: 192,
    aacStereoDefault: false,
    preserveOriginalAudio: true,
    preferSrtSubtitles: false,
    warnSubtitleFormats: true,
  },
};

const encoderPresetOptions = [
  { value: 'veryfast', label: 'Fast preview', description: 'Faster conversions, larger files. Useful while testing.' },
  { value: 'medium', label: 'Balanced', description: 'Recommended default for quality, size, and speed.' },
  { value: 'slow', label: 'Higher compression', description: 'Slower, usually smaller files at the same quality.' },
  { value: 'slower', label: 'Archive patience', description: 'Very slow. Use only when size matters more than time.' },
] as const;

const pixelFormatOptions = [
  { value: 'auto', label: 'Auto / codec default', description: 'Lets MVForge choose a compatible pixel format from the codec and encoder.' },
  { value: 'yuv420p10le', label: '10-bit Main10', description: 'Recommended for x265, anime, and DVD sources. Helps reduce banding.' },
  { value: 'p010le', label: 'QSV 10-bit Main10 (P010)', description: 'Native 10-bit input format for Intel Quick Sync HEVC Main10.' },
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
  const [form, setForm] = useState<ProfileInput>(initialProfile);
  const [showForm, setShowForm] = useState(false);
  const [showInactive, setShowInactive] = useState(false);
  const [profileJson, setProfileJson] = useState(JSON.stringify(initialProfile, null, 2));
  const visibleProfiles = (profiles.data ?? []).filter((profile) => showInactive || (!profile.disabled && !profile.deletedAt));

  const createProfile = useMutation({
    mutationFn: api.createProfile,
    onSuccess: async () => {
      setForm(initialProfile);
      setProfileJson(JSON.stringify(initialProfile, null, 2));
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
        ...(disablingHardware ? { videoEncoder: 'libx265', preferredEncoder: 'software' } : {}),
      },
    };
    setProfileForm(['videoEncoder', 'useHardwareIfAvailable', 'pixFmt'].includes(key) ? synchronizeAuthoritativeContract(next) : next);
  }

  function updateProcessingPreference(preference: 'auto' | 'software' | 'hardware') {
    const currentEncoder = workerConfigString(form, 'videoEncoder', 'auto');
    const hardware = preference === 'hardware';
    const hardwareAvailable = hardwareEncodersFor(codecFamilyFor(form.videoCodec)).length > 0;
    const next = {
      ...form,
      workerConfig: {
        ...form.workerConfig,
        preferredEncoder: preference,
        useHardwareIfAvailable: preference !== 'software' && hardwareAvailable,
        videoEncoder: preference === 'software' || preference === 'auto'
          ? 'auto'
          : isHardwareEncoderOption(currentEncoder) ? currentEncoder : 'auto',
        ...(!hardware ? {
          qsvRateControl: undefined,
          qsvLookAheadDepth: undefined,
          qsvExtendedBRC: undefined,
          qsvAdaptiveI: undefined,
          qsvAdaptiveB: undefined,
        } : {}),
      },
    };
    setProfileForm(synchronizeAuthoritativeContract(next));
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
        videoCodec: 'x265_10bit',
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
        videoCodec: 'x265_10bit',
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
        videoCodec: 'x265_10bit',
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
        videoCodec: 'x265_10bit',
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

    createProfile.mutate(payload);
  }

  function addProfile() {
    setForm(initialProfile);
    setProfileJson(JSON.stringify(initialProfile, null, 2));
    setShowForm(true);
  }

  function editProfile(profile: Profile) {
    navigate(`/profile-lab?videoProfileId=${profile.id}`);
  }

  function cancelEdit() {
    setForm(initialProfile);
    setProfileJson(JSON.stringify(initialProfile, null, 2));
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
        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', sm: 'center' }} sx={{ mb: 2 }} spacing={1}>
          <FormControlLabel
            control={<Checkbox checked={showInactive} onChange={(event) => setShowInactive(event.target.checked)} />}
            label="Show disabled/deleted profiles"
          />
          <Button startIcon={<AddIcon />} variant="contained" onClick={addProfile}>
            Add Profile
          </Button>
        </Stack>
        <Dialog open={showForm} onClose={cancelEdit} maxWidth="lg" fullWidth>
          <DialogTitle>New Profile</DialogTitle>
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
                  <Grid size={{ xs: 12, md: 8 }}>
                    <TextField
                      label="What this profile is for"
                      value={form.description}
                      onChange={(event) => updateField('description', event.target.value)}
                      fullWidth
                    />
                  </Grid>
                  <Grid size={{ xs: 12, md: 7 }}>
                    <Stack spacing={1.25}>
                      <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                        <Typography fontWeight={700}>Quality</Typography>
                        <Chip label={form.videoCodec === 'copy' ? 'Original' : `CRF ${form.qualityValue}`} size="small" />
                      </Stack>
                      <Slider value={form.qualityValue} min={14} max={30} step={1} marks={[{ value: 14, label: '14' }, { value: 22, label: '22' }, { value: 30, label: '30' }]} disabled={form.videoCodec === 'copy'} onChange={(_, value) => updateField('qualityValue', Array.isArray(value) ? value[0] : value)} valueLabelDisplay="on" />
                      <Typography variant="body2" color="text.secondary">Lower CRF preserves more detail; higher CRF reduces file size.</Typography>
                    </Stack>
                  </Grid>
                  <Grid size={{ xs: 12, md: 5 }}>
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
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Processing preference"
                            value={workerConfigString(form, 'preferredEncoder', 'software')}
                            onChange={(event) => updateProcessingPreference(event.target.value as 'auto' | 'software' | 'hardware')}
                            helperText="Software follows Video Codec; Hardware exposes a validated hardware encoder."
                            disabled={form.videoCodec === 'copy'}
                            select
                            fullWidth
                          >
                            <MenuItem value="auto">Auto</MenuItem>
                            <MenuItem value="software">Software · match Video Codec</MenuItem>
                            <MenuItem value="hardware" disabled={hardwareEncodersFor(codecFamilyFor(form.videoCodec)).length === 0}>Hardware</MenuItem>
                          </TextField>
                        </Grid>
                        {workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' ? <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Hardware encoder"
                            value={workerConfigString(form, 'videoEncoder', 'auto')}
                            onChange={(event) => updateWorkerConfig('videoEncoder', event.target.value)}
                            helperText={videoEncoderDescription(workerConfigString(form, 'videoEncoder', 'auto'))}
                            select
                            fullWidth
                          >
                            <MenuItem value="auto">Auto</MenuItem>
                            {videoEncoderOptions.filter((option) => isHardwareEncoderOption(option.value)).map((option) => (
                              <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>
                            ))}
                          </TextField>
                        </Grid> : null}
                        {workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' ? <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Hardware quality"
                            value={workerConfigNumber(form, 'globalQuality', qsvQualityRangeForCrf(form.qualityValue || 20).recommended)}
                            onChange={(event) => updateWorkerConfig('globalQuality', Number(event.target.value))}
                            helperText={hardwareQualityHelper(form.qualityValue)}
                            type="number"
                            inputProps={{ min: 15, max: 35 }}
                            disabled={workerConfigString(form, 'videoEncoder', 'auto') === 'hevc_videotoolbox'}
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
                                <MenuItem value="la_icq">LA-ICQ · capability required</MenuItem>
                              </TextField>
                            </Grid>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <TextField
                                label="QSV look-ahead depth"
                                type="number"
                                value={workerConfigNumber(form, 'qsvLookAheadDepth', 40)}
                                onChange={(event) => updateWorkerConfig('qsvLookAheadDepth', Number(event.target.value))}
                                inputProps={{ min: 10, max: 100 }}
                                helperText="Used with supported extended BRC; 40 is a conservative starting point."
                                fullWidth
                              />
                            </Grid>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <Stack>
                                <FormControlLabel
                                  control={<Checkbox checked={workerConfigBool(form, 'qsvExtendedBRC')} onChange={(event) => updateWorkerConfig('qsvExtendedBRC', event.target.checked)} />}
                                  label="QSV Extended BRC"
                                />
                                <FormControlLabel
                                  control={<Checkbox checked={workerConfigBool(form, 'qsvAdaptiveI')} onChange={(event) => updateWorkerConfig('qsvAdaptiveI', event.target.checked)} />}
                                  label="QSV Adaptive I"
                                />
                                <FormControlLabel
                                  control={<Checkbox checked={workerConfigBool(form, 'qsvAdaptiveB')} onChange={(event) => updateWorkerConfig('qsvAdaptiveB', event.target.checked)} />}
                                  label="QSV Adaptive B"
                                />
                              </Stack>
                            </Grid>
                          </>
                        ) : null}
                        {workerConfigString(form, 'preferredEncoder', 'software') === 'hardware' && workerConfigString(form, 'videoEncoder', 'auto') === 'hevc_videotoolbox' ? (
                          <>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <TextField label="VideoToolbox bitrate (Mbps)" type="number" value={workerConfigNumber(form, 'videoToolboxBitrateMbps', 6)} onChange={(event) => updateWorkerConfig('videoToolboxBitrateMbps', Number(event.target.value))} inputProps={{ min: 1, max: 200 }} fullWidth />
                            </Grid>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <TextField label="VideoToolbox maxrate (Mbps)" type="number" value={workerConfigNumber(form, 'videoToolboxMaxrateMbps', 8)} onChange={(event) => updateWorkerConfig('videoToolboxMaxrateMbps', Number(event.target.value))} inputProps={{ min: 1, max: 250 }} fullWidth />
                            </Grid>
                            <Grid size={{ xs: 12, md: 4 }}>
                              <TextField label="VideoToolbox buffer (Mbps)" type="number" value={workerConfigNumber(form, 'videoToolboxBufferMbps', 12)} onChange={(event) => updateWorkerConfig('videoToolboxBufferMbps', Number(event.target.value))} inputProps={{ min: 1, max: 500 }} fullWidth />
                            </Grid>
                          </>
                        ) : null}
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
                                onChange={(event) => updateWorkerConfig('preserveOriginalAudio', event.target.checked)}
                              />
                            }
                            label="Keep original audio as secondary"
                          />
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            select
                            fullWidth
                            label="Subtitle output format"
                            value={subtitleOutputFormat(form)}
                            onChange={(event) => updateWorkerConfig('subtitleOutputFormat', event.target.value)}
                            helperText="Text tracks only; bitmap subtitles are preserved"
                          >
                            <MenuItem value="source">Preserve source format</MenuItem>
                            <MenuItem value="srt">Convert text subtitles to SRT</MenuItem>
                            <MenuItem value="ass">Convert text subtitles to ASS</MenuItem>
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={workerConfigBool(form, 'warnSubtitleFormats', true)}
                                onChange={(event) => updateWorkerConfig('warnSubtitleFormats', event.target.checked)}
                              />
                            }
                            label="Warn about ASS/PGS/VobSub"
                          />
                        </Grid>
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
                              label="Audio codec"
                              value={form.audioCodec}
                              onChange={(event) => updateField('audioCodec', event.target.value)}
                              select
                              fullWidth
                            >
                              {['copy', 'aac', 'ac3', 'opus'].map((value) => (
                                <MenuItem key={value} value={value}>
                                  {value}
                                </MenuItem>
                              ))}
                            </TextField>
                          </Grid>
                          <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                            <Stack spacing={0.5}>
                              <Typography variant="body2">CRF {form.qualityValue}</Typography>
                              <Slider value={form.qualityValue} min={14} max={30} step={1} disabled={form.videoCodec === 'copy'} onChange={(_, value) => updateField('qualityValue', Array.isArray(value) ? value[0] : value)} valueLabelDisplay="auto" />
                            </Stack>
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
                              label="Color depth"
                              value={workerConfigString(form, 'pixFmt', 'yuv420p10le')}
                              onChange={(event) => updateWorkerConfig('pixFmt', event.target.value)}
                              helperText={pixelFormatDescription(workerConfigString(form, 'pixFmt', 'yuv420p10le'))}
                              disabled={form.videoCodec === 'copy'}
                              select
                              fullWidth
                            >
                              {pixelFormatOptions.map((option) => (
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
                      startIcon={<AddIcon />}
                      variant="contained"
                      disabled={createProfile.isPending}
                      fullWidth
                    >
                      Create Profile
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
                          <Chip label={`${profile.qualityMode.toUpperCase()} ${profile.qualityValue}`} size="small" />
                          <Chip label={profile.videoCodec === 'copy' ? 'Original quality' : `CRF ${profile.qualityValue}`} size="small" />
                          {profile.disabled ? <Chip label="Disabled" size="small" color="warning" /> : null}
                          {profile.deletedAt ? <Chip label="Deleted" size="small" color="default" /> : null}
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
                      <Stack direction="row" spacing={1} justifyContent="flex-end">
                        <Button startIcon={<EditIcon />} variant="outlined" onClick={() => editProfile(profile)} disabled={Boolean(profile.deletedAt)}>
                          Edit
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
              <Alert severity="info">No profiles have been configured yet.</Alert>
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
  const x265Args = profile.videoCodec === 'copy' || isHardware || !workerConfigString(profile, 'x265Params') ? '' : `-x265-params ${workerConfigString(profile, 'x265Params')}`;
  const hardwareQualityArgs = isVideoToolbox
    ? `-b:v ${workerConfigNumber(profile, 'videoToolboxBitrateMbps', 6)}M -maxrate ${workerConfigNumber(profile, 'videoToolboxMaxrateMbps', 8)}M -bufsize ${workerConfigNumber(profile, 'videoToolboxBufferMbps', 12)}M`
    : isHardware ? `-global_quality ${workerConfigNumber(profile, 'globalQuality', qsvQualityRangeForCrf(profile.qualityValue || 20).recommended)}` : '';
  const qsvArgs = resolvedEncoder === 'hevc_qsv'
    ? [
        workerConfigString(profile, 'qsvRateControl', 'icq') === 'la_icq' ? '-look_ahead 1' : '',
        workerConfigBool(profile, 'qsvExtendedBRC') ? `-extbrc 1 -look_ahead_depth ${workerConfigNumber(profile, 'qsvLookAheadDepth', 40)}` : '',
        workerConfigBool(profile, 'qsvAdaptiveI') ? '-adaptive_i 1' : '',
        workerConfigBool(profile, 'qsvAdaptiveB') ? '-adaptive_b 1' : '',
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

  return ['ffmpeg', '-i', input, videoArgs, presetArgs, pixFmtArgs, tuneArgs, x265Args, hardwareQualityArgs, qsvArgs, audioArgs, aacArgs, subtitleArgs, chapterArgs, hdrArgs, qualityArgs, output]
    .filter(Boolean)
    .join(' ');
}

function softwareEncoderForVideoCodec(videoCodec: string) {
  const family = codecFamilyFor(videoCodec);
  if (family === 'h264') return 'libx264';
  if (family === 'hevc') return 'libx265';
  if (family === 'av1') return 'libsvtav1';
  return family === 'copy' ? 'copy' : videoCodec;
}
