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
    setProfileForm({ ...form, [field]: value });
  }

  function updateWorkerConfig(key: string, value: string) {
    setProfileForm({
      ...form,
      workerConfig: {
        ...form.workerConfig,
        [key]: value,
      },
    });
  }

  function setProfileForm(next: ProfileInput) {
    setForm(next);
    setProfileJson(JSON.stringify(next, null, 2));
  }

  function applyGuidedPreset(preset: 'archive' | 'streaming' | 'smaller' | 'original') {
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
    };

    setProfileForm({ ...form, ...presets[preset] });
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
          Profiles preserve the media worker pipeline configuration shape while exposing it through a
          friendly form-based interface.
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
                </Grid>

                <Divider />

                <Grid container spacing={2}>
                  <Grid size={{ xs: 12 }}>
                    <Alert severity="info">
                      <strong>copy</strong> preserves the original stream without re-encoding. It is fast and lossless, but filters, loudness, EQ,
                      denoise, and compatibility changes require choosing a real codec instead.
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
                    <Stack spacing={1.25} sx={{ minHeight: 128 }}>
                      <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                        <Typography fontWeight={700}>Quality target</Typography>
                        <Chip label={qualityLabel(form.qualityValue, form.videoCodec)} size="small" />
                      </Stack>
                      <Slider
                        value={form.qualityValue}
                        min={15}
                        max={28}
                        step={1}
                        marks={[{ value: 15 }, { value: 18 }, { value: 22 }, { value: 28 }]}
                        disabled={form.videoCodec === 'copy'}
                        onChange={(_, value) => updateField('qualityValue', Array.isArray(value) ? value[0] : value)}
                        valueLabelDisplay="auto"
                        sx={{ mt: 1, mb: 0.5 }}
                      />
                      <Stack direction="row" justifyContent="space-between" sx={{ px: 0.25 }}>
                        <Typography color="text.secondary" variant="body2">
                          Best
                        </Typography>
                        <Typography color="text.secondary" variant="body2">
                          Balanced
                        </Typography>
                        <Typography color="text.secondary" variant="body2">
                          Small
                        </Typography>
                      </Stack>
                      <Typography color="text.secondary" variant="body2">
                        Lower values keep more quality. Higher values create smaller files.
                      </Typography>
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
                        <Grid size={{ xs: 12 }}>
                          <Stack spacing={0.4}>
                            <Typography fontWeight={700}>Video encoding</Typography>
                            <Typography color="text.secondary" variant="body2">
                              These defaults affect how FFmpeg writes the video stream. They keep audio, subtitles, and chapters untouched.
                            </Typography>
                          </Stack>
                        </Grid>
                        <Grid size={{ xs: 12, md: 6 }}>
                          <TextField
                            label="Encoding speed"
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
                        <Grid size={{ xs: 12, md: 6 }}>
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
                      </Grid>
                    </Box>
                  </Grid>
                  <Grid size={{ xs: 12 }}>
                    <Accordion variant="outlined" sx={{ bgcolor: 'transparent' }}>
                      <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                        <Stack>
                          <Typography fontWeight={700}>Advanced</Typography>
                          <Typography color="text.secondary" variant="body2">
                            Codecs, exact CRF, profile JSON, and worker command preview.
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
                        <Chip label={profile.videoCodec} size="small" color="primary" />
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

function encoderPresetDescription(value: string) {
  return encoderPresetOptions.find((option) => option.value === value)?.description ?? 'Controls how much time FFmpeg spends compressing video.';
}

function pixelFormatDescription(value: string) {
  return pixelFormatOptions.find((option) => option.value === value)?.description ?? 'Controls output color depth and playback compatibility.';
}

function buildDryRunCommand(profile: ProfileInput) {
  const input = '<input>';
  const output = `<output>.${profile.container}`;
  const videoArgs = profile.videoCodec === 'copy' ? '-c:v copy' : `-c:v ${profile.videoCodec}`;
  const presetArgs = profile.videoCodec === 'copy' ? '' : `-preset ${workerConfigString(profile, 'videoPreset', 'medium')}`;
  const pixFmtArgs = profile.videoCodec === 'copy' ? '' : `-pix_fmt ${workerConfigString(profile, 'pixFmt', 'yuv420p10le')}`;
  const audioArgs = profile.audioCodec === 'copy' ? '-c:a copy' : `-c:a ${profile.audioCodec}`;
  const subtitleArgs = profile.preserveSubtitles ? '-c:s copy' : '-sn';
  const chapterArgs = profile.preserveChapters ? '-map_chapters 0' : '-map_chapters -1';
  const hdrArgs = profile.preserveHdr ? '-map_metadata 0' : '-map_metadata -1';
  const qualityArgs = profile.qualityMode === 'crf' && profile.videoCodec !== 'copy' ? `-crf ${profile.qualityValue}` : '';

  return ['ffmpeg', '-i', input, videoArgs, presetArgs, pixFmtArgs, audioArgs, subtitleArgs, chapterArgs, hdrArgs, qualityArgs, output]
    .filter(Boolean)
    .join(' ');
}
