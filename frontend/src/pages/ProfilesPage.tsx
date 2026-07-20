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
import SaveIcon from '@mui/icons-material/Save';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useState } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import type { Profile, ProfileInput } from '../api/types';

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
  qualityStrategy: 'balanced',
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
  { value: 'yuv420p10le', label: '10-bit Main10', description: 'Recommended for x265, anime, and DVD sources. Helps reduce banding.' },
  { value: 'yuv420p', label: '8-bit compatibility', description: 'Use for older devices or simple compatibility-focused outputs.' },
] as const;

const videoEncoderOptions = [
  { value: 'auto', label: 'Auto', description: 'MediaForge chooses software unless hardware fallback is enabled and available.' },
  { value: 'hevc_qsv', label: 'Intel Quick Sync', description: 'Fast HEVC hardware encoding for bulk conversion on Intel systems.' },
  { value: 'hevc_nvenc', label: 'NVIDIA NVENC', description: 'Fast HEVC hardware encoding on NVIDIA GPUs.' },
  { value: 'hevc_videotoolbox', label: 'Apple VideoToolbox', description: 'HEVC hardware encoding on supported Apple Silicon and Intel Macs.' },
  { value: 'hevc_amf', label: 'AMD AMF', description: 'Fast HEVC hardware encoding on supported AMD GPUs.' },
  { value: 'libx265', label: 'Software x265', description: 'Slower, usually better compression and quality per GB.' },
] as const;

const qualityOptions = [
  { value: 'maximum', label: 'Maximum', description: 'Use when quality matters more than size.', crf: 15, preset: 'slow' },
  { value: 'high', label: 'High', description: 'Excellent quality with reasonable file sizes.', crf: 18, preset: 'medium' },
  { value: 'balanced', label: 'Balanced', description: 'Good default for most libraries.', crf: 22, preset: 'medium' },
  { value: 'small', label: 'Small', description: 'Prioritize smaller files.', crf: 25, preset: 'slow' },
] as const;

export function ProfilesPage() {
  const queryClient = useQueryClient();
  const profiles = useQuery({
    queryKey: ['profiles', 'admin'],
    queryFn: api.profilesAdmin,
  });
  const [form, setForm] = useState<ProfileInput>(initialProfile);
  const [editingId, setEditingId] = useState<number | null>(null);
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

  const updateProfile = useMutation({
    mutationFn: api.updateProfile,
    onSuccess: async () => {
      setForm(initialProfile);
      setProfileJson(JSON.stringify(initialProfile, null, 2));
      setEditingId(null);
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
    const next = { ...form, [field]: value };
    setProfileForm(field === 'videoCodec' || field === 'qualityValue' ? synchronizeAuthoritativeContract(next) : next);
  }

  function updateWorkerConfig(key: string, value: unknown) {
    const next = {
      ...form,
      workerConfig: {
        ...form.workerConfig,
        [key]: value,
      },
    };
    setProfileForm(['videoEncoder', 'useHardwareIfAvailable', 'pixFmt'].includes(key) ? synchronizeAuthoritativeContract(next) : next);
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
        qualityValue: 25,
        preserveHdr: true,
        preserveSubtitles: true,
        preserveChapters: true,
        workerConfig: {
          ...form.workerConfig,
          videoEncoder: 'hevc_qsv',
          preferredEncoder: 'hardware',
          useHardwareIfAvailable: true,
          globalQuality: 25,
          videoPreset: 'medium',
          pixFmt: 'yuv420p10le',
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
        qualityValue: 27,
        preserveHdr: true,
        preserveSubtitles: true,
        preserveChapters: true,
        workerConfig: {
          ...form.workerConfig,
          videoEncoder: 'hevc_qsv',
          preferredEncoder: 'hardware',
          useHardwareIfAvailable: true,
          globalQuality: 27,
          videoPreset: 'medium',
          pixFmt: 'yuv420p10le',
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

  function applyQualityOption(option: typeof qualityOptions[number]) {
    setProfileForm(synchronizeAuthoritativeContract({
      ...form,
      qualityMode: 'crf',
      qualityValue: option.crf,
      videoCodec: form.videoCodec === 'copy' ? 'x265' : form.videoCodec,
      workerConfig: {
        ...form.workerConfig,
        videoPreset: option.preset,
      },
    }));
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const payload = {
      ...form,
      workerConfig: {
        ...form.workerConfig,
        encoder: 'ffmpeg',
        preset: form.workerConfig?.preset || form.name.toLowerCase().replaceAll(' ', '-'),
      },
    };

    if (editingId) {
      updateProfile.mutate({ id: editingId, ...payload });
      return;
    }

    createProfile.mutate(payload);
  }

  function addProfile() {
    setEditingId(null);
    setForm(initialProfile);
    setProfileJson(JSON.stringify(initialProfile, null, 2));
    setShowForm(true);
  }

  function editProfile(profile: Profile) {
    setEditingId(profile.id);
    setShowForm(true);
    setForm({
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
      audioCodec: profile.audioCodec,
      qualityMode: profile.qualityMode,
      qualityValue: profile.qualityValue,
      preserveHdr: profile.preserveHdr,
      preserveSubtitles: profile.preserveSubtitles,
      preserveChapters: profile.preserveChapters,
      workerConfig: profile.workerConfig,
      disabled: profile.disabled,
    });
    setProfileJson(
      JSON.stringify(
        {
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
          audioCodec: profile.audioCodec,
          qualityMode: profile.qualityMode,
          qualityValue: profile.qualityValue,
          preserveHdr: profile.preserveHdr,
          preserveSubtitles: profile.preserveSubtitles,
          preserveChapters: profile.preserveChapters,
          workerConfig: profile.workerConfig,
          disabled: profile.disabled,
        },
        null,
        2,
      ),
    );
  }

  function cancelEdit() {
    setEditingId(null);
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

      setForm(next);
      setProfileJson(JSON.stringify(next, null, 2));
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
          <DialogTitle>{editingId ? 'Edit Profile' : 'New Profile'}</DialogTitle>
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
                      Choose the result you want. MediaForge will translate it into encoder settings; Advanced is only for exact technical overrides.
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
                        <Chip label={qualityLabel(form.qualityValue, form.videoCodec)} size="small" />
                      </Stack>
                      <Grid container spacing={1}>
                        {qualityOptions.map((option) => {
                          const selected = qualityChoiceForValue(form.qualityValue, form.videoCodec) === option.value;
                          return (
                            <Grid key={option.value} size={{ xs: 12, sm: 6 }}>
                              <Button
                                type="button"
                                variant={selected ? 'contained' : 'outlined'}
                                onClick={() => applyQualityOption(option)}
                                disabled={form.videoCodec === 'copy'}
                                title={option.description}
                                fullWidth
                                sx={{ minHeight: 54, justifyContent: 'space-between' }}
                              >
                                <span>{option.label}</span>
                                <Chip label={`CRF ${option.crf}`} size="small" sx={{ ml: 1 }} />
                              </Button>
                            </Grid>
                          );
                        })}
                      </Grid>
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
                            label="Encoder"
                            value={workerConfigString(form, 'videoEncoder', 'auto')}
                            onChange={(event) => updateWorkerConfig('videoEncoder', event.target.value)}
                            helperText={videoEncoderDescription(workerConfigString(form, 'videoEncoder', 'auto'))}
                            disabled={form.videoCodec === 'copy'}
                            select
                            fullWidth
                          >
                            {videoEncoderOptions.map((option) => (
                              <MenuItem key={option.value} value={option.value}>
                                {option.label}
                              </MenuItem>
                            ))}
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Hardware preference"
                            value={workerConfigString(form, 'preferredEncoder', 'software')}
                            onChange={(event) => updateWorkerConfig('preferredEncoder', event.target.value)}
                            helperText="Hardware is faster for bulk queues; software x265 is better for careful archival compression."
                            disabled={form.videoCodec === 'copy'}
                            select
                            fullWidth
                          >
                            <MenuItem value="software">Prefer software quality</MenuItem>
                            <MenuItem value="hardware">Prefer hardware speed</MenuItem>
                            <MenuItem value="auto">Auto</MenuItem>
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Hardware quality"
                            value={workerConfigNumber(form, 'globalQuality', form.qualityValue || 25)}
                            onChange={(event) => updateWorkerConfig('globalQuality', Number(event.target.value))}
                            helperText="Used by QSV, NVENC, and AMF. VideoToolbox uses the bitrate controls below."
                            type="number"
                            inputProps={{ min: 15, max: 35 }}
                            disabled={form.videoCodec === 'copy' || workerConfigString(form, 'videoEncoder', 'auto') === 'hevc_videotoolbox'}
                            fullWidth
                          />
                        </Grid>
                        {workerConfigString(form, 'videoEncoder', 'auto') === 'hevc_videotoolbox' ? (
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
                                checked={workerConfigBool(form, 'useHardwareIfAvailable')}
                                onChange={(event) => updateWorkerConfig('useHardwareIfAvailable', event.target.checked)}
                              />
                            }
                            label="Use hardware if available"
                          />
                        </Grid>
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
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={workerConfigBool(form, 'preferSrtSubtitles')}
                                onChange={(event) => updateWorkerConfig('preferSrtSubtitles', event.target.checked)}
                              />
                            }
                            label="Convert subtitles to SRT when possible"
                          />
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
                              value={form.videoCodec}
                              onChange={(event) => updateField('videoCodec', event.target.value)}
                              select
                              fullWidth
                            >
                              {['x264', 'x265', 'x265_10bit', 'copy'].map((value) => (
                                <MenuItem key={value} value={value}>
                                  {value}
                                </MenuItem>
                              ))}
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
                            <TextField
                              label="Exact CRF"
                              value={form.qualityValue}
                              onChange={(event) => updateField('qualityValue', Number(event.target.value))}
                              type="number"
                              inputProps={{ min: 0, max: 51 }}
                              required
                              fullWidth
                            />
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
                      startIcon={editingId ? <SaveIcon /> : <AddIcon />}
                      variant="contained"
                      disabled={createProfile.isPending || updateProfile.isPending}
                      fullWidth
                    >
                      {editingId ? 'Save Profile' : 'Create Profile'}
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
                Profile could not be updated. Profile names must be unique.
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
                          <Chip label={qualityLabel(profile.qualityValue, profile.videoCodec)} size="small" />
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

function qualityLabel(qualityValue: number, videoCodec: string) {
  if (videoCodec === 'copy') {
    return 'Original';
  }

  if (qualityValue <= 19) {
    return 'Best quality';
  }

  if (qualityValue <= 23) {
    return 'Balanced';
  }

  return 'Smaller file';
}

function qualityChoiceForValue(qualityValue: number, videoCodec: string) {
  if (videoCodec === 'copy') {
    return 'original';
  }
  if (qualityValue <= 16) {
    return 'maximum';
  }
  if (qualityValue <= 19) {
    return 'high';
  }
  if (qualityValue <= 23) {
    return 'balanced';
  }
  return 'small';
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

function synchronizeAuthoritativeContract(profile: ProfileInput): ProfileInput {
  const codecFamily = codecFamilyFor(profile.videoCodec);
  const qualityStrategy = profile.qualityValue > 0 && profile.qualityValue <= 18
    ? 'high'
    : profile.qualityValue >= 25
      ? 'small_size'
      : 'balanced';
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
  const hardwareAllowed = workerConfigBool(profile, 'useHardwareIfAvailable');
  const hardware = hardwareEncodersFor(codecFamily);
  const pixelFormat = workerConfigString(profile, 'pixFmt', profile.pixelFormat || 'yuv420p');
  const bitDepth = pixelFormat.includes('10') || profile.videoCodec.toLowerCase().includes('10bit') ? 10 : 8;

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
  if (codecFamily === 'h264') return ['h264_qsv', 'h264_nvenc', 'h264_videotoolbox', 'h264_amf'];
  if (codecFamily === 'av1') return ['av1_qsv', 'av1_nvenc', 'av1_amf'];
  if (codecFamily === 'hevc') return ['hevc_qsv', 'hevc_nvenc', 'hevc_videotoolbox', 'hevc_amf'];
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

function buildDryRunCommand(profile: ProfileInput) {
  const input = '<input>';
  const output = `<output>.${profile.container}`;
  const encoder = workerConfigString(profile, 'videoEncoder', 'auto');
  const resolvedEncoder = encoder === 'auto' ? (workerConfigBool(profile, 'useHardwareIfAvailable') ? 'auto-hardware' : profile.videoCodec) : encoder;
  const isHardware = ['hevc_qsv', 'hevc_nvenc', 'hevc_videotoolbox', 'hevc_amf', 'auto-hardware'].includes(resolvedEncoder);
  const isVideoToolbox = resolvedEncoder === 'hevc_videotoolbox';
  const videoArgs = profile.videoCodec === 'copy' ? '-c:v copy' : `-c:v ${resolvedEncoder}`;
  const presetArgs = profile.videoCodec === 'copy' || isHardware ? '' : `-preset ${workerConfigString(profile, 'videoPreset', 'medium')}`;
  const pixFmtArgs = profile.videoCodec === 'copy' ? '' : isVideoToolbox ? `-profile:v ${profile.bitDepth === 10 || profile.videoCodec.includes('10bit') ? 'main10' : 'main'} -pix_fmt ${profile.bitDepth === 10 || profile.videoCodec.includes('10bit') ? 'p010le' : 'yuv420p'}` : `-pix_fmt ${workerConfigString(profile, 'pixFmt', 'yuv420p10le')}`;
  const tuneArgs = profile.videoCodec === 'copy' || !workerConfigString(profile, 'tune') ? '' : `-tune ${workerConfigString(profile, 'tune')}`;
  const x265Args = profile.videoCodec === 'copy' || isHardware || !workerConfigString(profile, 'x265Params') ? '' : `-x265-params ${workerConfigString(profile, 'x265Params')}`;
  const hardwareQualityArgs = isVideoToolbox
    ? `-b:v ${workerConfigNumber(profile, 'videoToolboxBitrateMbps', 6)}M -maxrate ${workerConfigNumber(profile, 'videoToolboxMaxrateMbps', 8)}M -bufsize ${workerConfigNumber(profile, 'videoToolboxBufferMbps', 12)}M`
    : isHardware ? `-global_quality ${workerConfigNumber(profile, 'globalQuality', profile.qualityValue || 25)}` : '';
  const audioArgs = profile.audioCodec === 'copy' ? '-c:a copy' : `-c:a ${profile.audioCodec}`;
  const aacDisposition = aacTrackDefault(profile) ? 'default' : '0';
  const aacArgs = aacTrackEnabled(profile) ? `-map 0:a:0 -c:a:1 aac -ac:a:1 2 -disposition:a:1 ${aacDisposition}` : '';
  const subtitleArgs = profile.preserveSubtitles ? (workerConfigBool(profile, 'preferSrtSubtitles') ? '-c:s srt' : '-c:s copy') : '-sn';
  const chapterArgs = profile.preserveChapters ? '-map_chapters 0' : '-map_chapters -1';
  const hdrArgs = profile.preserveHdr ? '-map_metadata 0' : '-map_metadata -1';
  const qualityArgs = profile.qualityMode === 'crf' && profile.videoCodec !== 'copy' && !isHardware ? `-crf ${profile.qualityValue}` : '';

  return ['ffmpeg', '-i', input, videoArgs, presetArgs, pixFmtArgs, tuneArgs, x265Args, hardwareQualityArgs, audioArgs, aacArgs, subtitleArgs, chapterArgs, hdrArgs, qualityArgs, output]
    .filter(Boolean)
    .join(' ');
}
