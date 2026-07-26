import {
  Accordion,
  AccordionDetails,
  AccordionSummary,
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Checkbox,
  Chip,
  Dialog,
  DialogContent,
  DialogTitle,
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
  Tooltip,
  Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import CloseIcon from '@mui/icons-material/Close';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline';
import EditIcon from '@mui/icons-material/Edit';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import SaveIcon from '@mui/icons-material/Save';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useEffect, useMemo, useRef, useState } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import type { AppSetting, AudioEnhancementProfile } from '../api/types';
import { audioProfileEqFrequencies, defaultAudioProfileEqBands, starterAudioProfiles } from '../audioProfiles';

const eqFrequencies = audioProfileEqFrequencies;
const defaultEqBands = defaultAudioProfileEqBands;

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

const emptyProfile: AudioEnhancementProfile = {
  key: '',
  name: '',
  description: '',
  intent: '',
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

export function AudioProfilesPage() {
  const queryClient = useQueryClient();
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const assets = useQuery({ queryKey: ['assets'], queryFn: api.assets });
  const audioProfiles = useMemo(() => getAudioProfiles(settings.data), [settings.data]);
  const activeAudioProfiles = audioProfiles.filter((profile) => !profile.disabled && !profile.deletedAt);
  const [showForm, setShowForm] = useState(false);
  const [showInactive, setShowInactive] = useState(false);
  const [editingKey, setEditingKey] = useState<string | null>(null);
  const [form, setForm] = useState<AudioEnhancementProfile>(emptyProfile);
  const [testAssetPath, setTestAssetPath] = useState('');
  const [testProfileKey, setTestProfileKey] = useState('');
  const [testStart, setTestStart] = useState('00:00:00');
  const [testSeconds, setTestSeconds] = useState(20);
  const [previewNonce, setPreviewNonce] = useState(0);
  const visibleAudioProfiles = audioProfiles.filter((profile) => showInactive || (!profile.disabled && !profile.deletedAt));
  const selectedTestProfile = activeAudioProfiles.find((profile) => profile.key === testProfileKey);
  const rawAssets = assets.data?.unprocessed ?? [];
  const selectedTestAsset = rawAssets.find((asset) => asset.path === testAssetPath) ?? null;

  const updateSetting = useMutation({
    mutationFn: api.updateSetting,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['settings'] });
    },
  });

  function saveProfiles(nextProfiles: AudioEnhancementProfile[]) {
    updateSetting.mutate({ key: 'audioEnhancementProfiles', value: { profiles: nextProfiles } });
  }

  function setProfileDisabled(profile: AudioEnhancementProfile, disabled: boolean) {
    saveProfiles(audioProfiles.map((candidate) => (candidate.key === profile.key ? { ...candidate, disabled } : candidate)));
  }

  function softDeleteProfile(profile: AudioEnhancementProfile) {
    saveProfiles(
      audioProfiles.map((candidate) =>
        candidate.key === profile.key ? { ...candidate, disabled: true, deletedAt: new Date().toISOString() } : candidate,
      ),
    );
  }

  function addProfile() {
    setEditingKey(null);
    setForm(emptyProfile);
    setShowForm(true);
  }

  function editProfile(profile: AudioEnhancementProfile) {
    setEditingKey(profile.key);
    setForm(profile);
    setShowForm(true);
  }

  function applyPreset(profile: AudioEnhancementProfile) {
    setEditingKey(null);
    setForm({ ...profile, key: uniqueKey(profile.key, audioProfiles), name: `${profile.name} Copy` });
    setShowForm(true);
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalized = {
      ...form,
      key: editingKey ?? slugify(form.key || form.name),
      outputCodec: form.outputCodec || 'aac',
    };
    const next = editingKey
      ? audioProfiles.map((profile) => (profile.key === editingKey ? { ...profile, ...normalized } : profile))
      : [...audioProfiles, normalized];
    saveProfiles(next);
    setShowForm(false);
    setEditingKey(null);
    setForm(emptyProfile);
  }

  return (
    <>
      <PageHeader title="Audio" eyebrow="Enhancement Profiles">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 820 }}>
          Build reusable FFmpeg audio chains for old anime, TV recordings, dialogue clarity, and loudness normalization.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {settings.isError ? <Alert severity="warning">Unable to load audio profiles.</Alert> : null}
        {updateSetting.isSuccess ? <Alert severity="success" sx={{ mb: 2 }}>Audio profiles saved.</Alert> : null}
        {updateSetting.isError ? <Alert severity="warning" sx={{ mb: 2 }}>Audio profiles could not be saved.</Alert> : null}

        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ xs: 'stretch', sm: 'center' }} sx={{ mb: 2 }} spacing={1}>
          <FormControlLabel
            control={<Checkbox checked={showInactive} onChange={(event) => setShowInactive(event.target.checked)} />}
            label="Show disabled/deleted audio profiles"
          />
          <Button startIcon={<AddIcon />} variant="contained" onClick={addProfile}>
            Add Audio Profile
          </Button>
        </Stack>

        <Card sx={{ mb: 2 }}>
          <CardContent>
            <Stack spacing={1}>
              <Typography variant="h3">Starter Presets</Typography>
              <Typography color="text.secondary">
                These are intentionally conservative. Keep the original audio track until you have reviewed the result.
              </Typography>
              <TextField
                label="Starter preset"
                value=""
                onChange={(event) => {
                  const profile = starterAudioProfiles.find((candidate) => candidate.key === event.target.value);
                  if (profile) {
                    applyPreset(profile);
                  }
                }}
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
            </Stack>
          </CardContent>
        </Card>

        <Card sx={{ mb: 2 }}>
          <CardContent>
            <Stack spacing={2}>
              <Stack>
                <Typography variant="h3">Audio Test Bench</Typography>
                <Typography color="text.secondary">
                  Pick a raw asset, choose a duration, and compare a browser-safe original sample against the selected profile.
                </Typography>
              </Stack>
              <Grid container spacing={2} alignItems="stretch">
                <Grid size={{ xs: 12, md: 5 }}>
                  <Autocomplete
                    options={rawAssets}
                    value={selectedTestAsset}
                    onChange={(_, asset) => setTestAssetPath(asset?.path ?? '')}
                    getOptionLabel={(asset) => asset.relativePath || asset.fileName}
                    isOptionEqualToValue={(option, value) => option.path === value.path}
                    filterOptions={(options, state) => {
                      const query = state.inputValue.trim().toLowerCase();
                      if (!query) {
                        return options.slice(0, 50);
                      }
                      return options
                        .filter((asset) =>
                          [asset.fileName, asset.relativePath, asset.path].some((value) =>
                            value.toLowerCase().includes(query),
                          ),
                        )
                        .slice(0, 50);
                    }}
                    renderInput={(params) => <TextField {...params} label="Raw asset" />}
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
                </Grid>
                <Grid size={{ xs: 12, md: 3 }}>
                  <TextField
                    label="Audio profile"
                    value={testProfileKey}
                    onChange={(event) => setTestProfileKey(event.target.value)}
                    select
                    fullWidth
                  >
                    <MenuItem value="">No profile</MenuItem>
                      {activeAudioProfiles.map((profile) => (
                        <MenuItem key={profile.key} value={profile.key}>
                          {profile.name}
                      </MenuItem>
                    ))}
                  </TextField>
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 2 }}>
                  <TextField
                    label="Start"
                    value={testStart}
                    onChange={(event) => setTestStart(event.target.value)}
                    placeholder="00:00:00"
                    helperText="HH:MM:SS"
                    fullWidth
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 1 }}>
                  <TextField
                    label="Seconds"
                    value={testSeconds}
                    onChange={(event) => setTestSeconds(Number(event.target.value))}
                    type="number"
                    inputProps={{ min: 5, max: 120 }}
                    fullWidth
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 1 }}>
                  <Button
                    startIcon={<PlayArrowIcon />}
                    variant="contained"
                    disabled={!testAssetPath}
                    onClick={() => setPreviewNonce((current) => current + 1)}
                    fullWidth
                    sx={{ height: '100%' }}
                  >
                    Generate
                  </Button>
                </Grid>
              </Grid>
              {selectedTestProfile ? (
                <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                  <Chip label={`Output: ${selectedTestProfile.outputCodec}`} size="small" />
                  <Chip label={selectedTestProfile.preserveOriginalTrack ? 'Preserve original track' : 'Replace audio'} color="success" size="small" />
                  <Chip label={`Filters: ${effectiveFilters(selectedTestProfile) || 'none'}`} size="small" />
                </Stack>
              ) : null}
              {testAssetPath && previewNonce > 0 ? (
                <Grid container spacing={2}>
                  <Grid size={{ xs: 12, md: 6 }}>
                    <AudioPreviewPlayer
                      label="Original sample"
                      src={`${api.audioPreviewUrl({ path: testAssetPath, start: testStart, seconds: testSeconds })}&nonce=${previewNonce}`}
                    />
                  </Grid>
                  <Grid size={{ xs: 12, md: 6 }}>
                    <AudioPreviewPlayer
                      label="Profile sample"
                      src={`${api.audioPreviewUrl({ path: testAssetPath, profileKey: testProfileKey, start: testStart, seconds: testSeconds })}&nonce=${previewNonce}`}
                    />
                  </Grid>
                </Grid>
              ) : null}
              {assets.data?.unprocessed.length === 0 ? <Alert severity="info">No raw assets are available for testing yet.</Alert> : null}
            </Stack>
          </CardContent>
        </Card>

        <Card>
          <CardContent sx={{ p: 0, '&:last-child': { pb: 0 } }}>
            <Table sx={{ tableLayout: 'fixed' }}>
              <TableHead>
                <TableRow>
                  <TableCell>Profile</TableCell>
                  <TableCell>Intent</TableCell>
                  <TableCell>Filter Chain</TableCell>
                  <TableCell>Output</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {visibleAudioProfiles.map((profile) => (
                  <TableRow key={profile.key} hover>
                    <TableCell>
                      <Stack spacing={0.6}>
                        <Typography fontWeight={700}>{profile.name}</Typography>
                        <Typography color="text.secondary" variant="body2">
                          {profile.description}
                        </Typography>
                        <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap>
                          {profile.disabled ? <Chip label="Disabled" size="small" color="warning" /> : null}
                          {profile.deletedAt ? <Chip label="Deleted" size="small" /> : null}
                        </Stack>
                      </Stack>
                    </TableCell>
                    <TableCell>{profile.intent}</TableCell>
                    <TableCell>
                      <Typography component="code" sx={{ fontFamily: 'monospace', wordBreak: 'break-all' }}>
                        {effectiveFilters(profile)}
                      </Typography>
                    </TableCell>
                    <TableCell>
                      <Stack spacing={0.75}>
                        <Chip label={profile.outputCodec} size="small" sx={{ alignSelf: 'flex-start' }} />
                        {profile.preserveOriginalTrack ? <Chip label="Preserve original" color="success" size="small" sx={{ alignSelf: 'flex-start' }} /> : null}
                      </Stack>
                    </TableCell>
                    <TableCell align="right">
                      <Stack direction="row" spacing={1} justifyContent="flex-end">
                        <Button startIcon={<ContentCopyIcon />} variant="outlined" onClick={() => applyPreset(profile)}>
                          Copy
                        </Button>
                        <Button startIcon={<EditIcon />} variant="outlined" onClick={() => editProfile(profile)}>
                          Edit
                        </Button>
                        <Button
                          variant="outlined"
                          color={profile.disabled ? 'success' : 'warning'}
                          disabled={Boolean(profile.deletedAt)}
                          onClick={() => setProfileDisabled(profile, !profile.disabled)}
                        >
                          {profile.disabled ? 'Enable' : 'Disable'}
                        </Button>
                        <Button
                          startIcon={<DeleteOutlineIcon />}
                          variant="outlined"
                          color="error"
                          disabled={Boolean(profile.deletedAt)}
                          onClick={() => softDeleteProfile(profile)}
                        >
                          Delete
                        </Button>
                      </Stack>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      </Box>

      <Dialog open={showForm} onClose={() => setShowForm(false)} maxWidth="lg" fullWidth>
        <DialogTitle>{editingKey ? 'Edit Audio Profile' : 'New Audio Profile'}</DialogTitle>
        <DialogContent>
          <Box component="form" onSubmit={submit}>
            <Stack spacing={2.5}>
              <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                <Typography color="text.secondary" variant="body2">
                  Shape restoration with simple controls. Technical filters stay in Advanced.
                </Typography>
                <Button startIcon={<CloseIcon />} onClick={() => setShowForm(false)}>
                  Close
                </Button>
              </Stack>
              <Grid container spacing={2}>
                <Grid size={{ xs: 12, md: 6 }}>
                  <TextField
                    label="Name"
                    value={form.name}
                    onChange={(event) =>
                      setForm((current) => ({
                        ...current,
                        name: event.target.value,
                        key: editingKey ? current.key : slugify(event.target.value),
                      }))
                    }
                    required
                    fullWidth
                  />
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <TextField
                    label="Key"
                    value={form.key}
                    onChange={(event) => setForm((current) => ({ ...current, key: slugify(event.target.value) }))}
                    disabled={Boolean(editingKey)}
                    required
                    fullWidth
                  />
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <TextField
                    label="Description"
                    value={form.description}
                    onChange={(event) => setForm((current) => ({ ...current, description: event.target.value }))}
                    fullWidth
                  />
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <Tooltip title="Short human-readable goal for this profile, such as dialogue clarity or old source cleanup.">
                    <TextField
                      label="Intent"
                      value={form.intent}
                      onChange={(event) => setForm((current) => ({ ...current, intent: event.target.value }))}
                      fullWidth
                    />
                  </Tooltip>
                </Grid>
                <Grid size={{ xs: 12, md: 6 }}>
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={form.preserveOriginalTrack}
                        onChange={(event) =>
                          setForm((current) => ({ ...current, preserveOriginalTrack: event.target.checked }))
                        }
                      />
                    }
                    label="Keep original audio track"
                  />
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, bgcolor: 'rgba(79,179,255,0.04)' }}>
                    <Stack spacing={2}>
                      <Stack>
                        <Typography variant="h3">Audio Restoration Studio</Typography>
                        <Typography color="text.secondary" variant="body2">
                          Each control writes a safe filter choice behind the scenes.
                        </Typography>
                      </Stack>
                      <RestorationSlider
                        label="Noise"
                        value={managedFilterLevel(form.filters, 'noise')}
                        onChange={(value) => setForm((current) => ({ ...current, filters: setManagedFilterLevel(current.filters, 'noise', value) }))}
                      />
                      <RestorationSlider
                        label="Hum"
                        value={managedFilterLevel(form.filters, 'hum')}
                        onChange={(value) => setForm((current) => ({ ...current, filters: setManagedFilterLevel(current.filters, 'hum', value) }))}
                      />
                      <RestorationSlider
                        label="Brightness"
                        value={managedFilterLevel(form.filters, 'brightness')}
                        onChange={(value) => setForm((current) => ({ ...current, filters: setManagedFilterLevel(current.filters, 'brightness', value) }))}
                      />
                      <RestorationSlider
                        label="Stereo Width"
                        value={form.stereoWidth}
                        onChange={(value) =>
                          setForm((current) => ({
                            ...current,
                            stereoWidth: value,
                            channelMode: value > 0 ? 'light-stereo' : 'preserve',
                          }))
                        }
                      />
                      <Stack spacing={0.5}>
                        <Stack direction="row" justifyContent="space-between" alignItems="center">
                          <Typography fontWeight={700}>Loudness</Typography>
                          <Chip label={`${form.targetLoudness} LUFS`} size="small" />
                        </Stack>
                        <Slider
                          value={form.targetLoudness}
                          min={-28}
                          max={-12}
                          step={1}
                          marks={[{ value: -24 }, { value: -18 }, { value: -14 }]}
                          onChange={(_, value) => setForm((current) => ({ ...current, targetLoudness: Array.isArray(value) ? value[0] : value }))}
                        />
                      </Stack>
                    </Stack>
                  </Box>
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <Accordion variant="outlined" sx={{ bgcolor: 'transparent' }}>
                    <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                      <Stack>
                        <Typography fontWeight={700}>Advanced</Typography>
                        <Typography color="text.secondary" variant="body2">
                          Codec, channel layout, true peak, neural model, and exact filter chain.
                        </Typography>
                      </Stack>
                    </AccordionSummary>
                    <AccordionDetails>
                      <Grid container spacing={2}>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Output codec"
                            value={form.outputCodec}
                            onChange={(event) => setForm((current) => ({ ...current, outputCodec: event.target.value }))}
                            select
                            fullWidth
                          >
                            {['aac', 'copy', 'flac', 'opus', 'ac3'].map((codec) => (
                              <MenuItem key={codec} value={codec}>
                                {codec}
                              </MenuItem>
                            ))}
                          </TextField>
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Channel mode"
                            value={form.channelMode}
                            onChange={(event) =>
                              setForm((current) => ({
                                ...current,
                                channelMode: event.target.value as AudioEnhancementProfile['channelMode'],
                              }))
                            }
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
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Stereo method"
                            value={form.forceStereoMode}
                            onChange={(event) =>
                              setForm((current) => ({
                                ...current,
                                forceStereoMode: event.target.value as AudioEnhancementProfile['forceStereoMode'],
                              }))
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
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="Stereo delay ms"
                            type="number"
                            value={form.stereoDelayMs}
                            onChange={(event) => setForm((current) => ({ ...current, stereoDelayMs: Number(event.target.value) }))}
                            inputProps={{ min: 1, max: 40 }}
                            fullWidth
                          />
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="True peak dB"
                            type="number"
                            value={form.truePeak}
                            onChange={(event) => setForm((current) => ({ ...current, truePeak: Number(event.target.value) }))}
                            inputProps={{ min: -8, max: 0 }}
                            fullWidth
                          />
                        </Grid>
                        <Grid size={{ xs: 12, md: 4 }}>
                          <TextField
                            label="ARNNDN model path"
                            value={form.rnnoiseModelPath}
                            onChange={(event) => setForm((current) => ({ ...current, rnnoiseModelPath: event.target.value }))}
                            placeholder="/mvforge/models/audio/rnnoise-model.rnnn"
                            fullWidth
                          />
                        </Grid>
                        <Grid size={{ xs: 12 }}>
                          <TextField
                            label="Base FFmpeg audio filters"
                            value={form.filters}
                            onChange={(event) => setForm((current) => ({ ...current, filters: event.target.value }))}
                            multiline
                            minRows={3}
                            fullWidth
                          />
                        </Grid>
                        <Grid size={{ xs: 12 }}>
                          <Alert severity="info">Preview command: {buildPreviewCommand(form)}</Alert>
                        </Grid>
                      </Grid>
                    </AccordionDetails>
                  </Accordion>
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2, bgcolor: 'rgba(79,179,255,0.04)' }}>
                    <Stack spacing={2}>
                      <Stack>
                        <Typography variant="h3">Profile Preview</Typography>
                        <Typography color="text.secondary" variant="body2">
                          Uses the selected asset and timing from Audio Test Bench so you can calibrate before saving.
                        </Typography>
                      </Stack>
                      {testAssetPath ? (
                        <Grid container spacing={2}>
                          <Grid size={{ xs: 12, md: 6 }}>
                            <AudioPreviewPlayer
                              label="Original sample"
                              src={`${api.audioPreviewUrl({ path: testAssetPath, start: testStart, seconds: testSeconds })}&draft=${previewNonce}`}
                            />
                          </Grid>
                          <Grid size={{ xs: 12, md: 6 }}>
                            <AudioPreviewPlayer
                              label="Current profile draft"
                              src={`${api.audioPreviewUrl({
                                path: testAssetPath,
                                start: testStart,
                                seconds: testSeconds,
                                filters: effectiveFilters(form),
                              })}&draft=${previewNonce}-${encodeURIComponent(effectiveFilters(form))}`}
                            />
                          </Grid>
                        </Grid>
                      ) : (
                        <Alert severity="info">Select a raw asset in Audio Test Bench to preview this draft profile.</Alert>
                      )}
                    </Stack>
                  </Box>
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <Box sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 2 }}>
                    <Stack spacing={2.5}>
                      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                        <Stack>
                          <Typography variant="h3">Graphic EQ</Typography>
                          <Typography color="text.secondary" variant="body2">
                            Boost or reduce frequency bands in dB. Keep changes small for old TV and anime sources.
                          </Typography>
                        </Stack>
                        <Button variant="outlined" onClick={() => setForm((current) => ({ ...current, eqBands: defaultEqBands() }))}>
                          Reset EQ
                        </Button>
                      </Stack>
                      <Box sx={{ overflowX: 'auto', pb: 1 }}>
                        <Stack
                          direction="row"
                          spacing={1.5}
                          sx={{
                            minWidth: { xs: 720, md: 'auto' },
                            justifyContent: { xs: 'flex-start', lg: 'space-between' },
                          }}
                        >
                          {eqFrequencies.map((frequency) => {
                            const key = String(frequency);
                            const value = form.eqBands?.[key] ?? 0;
                            return (
                              <Stack
                                key={key}
                                spacing={1}
                                alignItems="center"
                                sx={{
                                  width: 92,
                                  border: 1,
                                  borderColor: 'divider',
                                  borderRadius: 1,
                                  p: 1.25,
                                  bgcolor: 'rgba(255,255,255,0.02)',
                                  flexShrink: 0,
                                }}
                              >
                                <Chip label={eqBandLabel(frequency)} size="small" />
                                <Typography fontWeight={700} noWrap>
                                  {formatFrequency(frequency)}
                                </Typography>
                                <Typography color={value === 0 ? 'text.secondary' : 'primary.main'} fontWeight={700} variant="body2">
                                  {value > 0 ? '+' : ''}{value} dB
                                </Typography>
                                <Stack direction="row" spacing={0.75} alignItems="center" sx={{ height: 220 }}>
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
                                    sx={{ height: 200 }}
                                    onChange={(_, nextValue) =>
                                      setForm((current) => ({
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
                    </Stack>
                  </Box>
                </Grid>
                <Grid size={{ xs: 12 }}>
                  <TextField
                    label="Notes"
                    value={form.notes}
                    onChange={(event) => setForm((current) => ({ ...current, notes: event.target.value }))}
                    multiline
                    minRows={2}
                    fullWidth
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                  <Button type="submit" startIcon={<SaveIcon />} variant="contained" disabled={updateSetting.isPending} fullWidth>
                    Save Profile
                  </Button>
                </Grid>
              </Grid>
            </Stack>
          </Box>
        </DialogContent>
      </Dialog>
    </>
  );
}

function AudioPreviewPlayer({ label, src }: { label: string; src: string }) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const [status, setStatus] = useState<'idle' | 'loading' | 'ready' | 'error'>('idle');
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
    setAudioSrc('');
    clearWaveform(waveformCanvas, waveformContext);

    async function draw() {
      try {
        const response = await fetch(src);
        if (!response.ok) {
          throw new Error(`Preview failed with ${response.status}`);
        }
        const blob = await response.blob();
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
        objectUrl = URL.createObjectURL(blob);
        setAudioSrc(objectUrl);
        drawWaveform(waveformCanvas, waveformContext, buffer);
        setStatus('ready');
      } catch {
        if (!canceled) {
          setStatus('error');
          drawWaveformError(waveformCanvas, waveformContext);
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
  }, [src]);

  return (
    <Stack spacing={1}>
      <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
        <Typography fontWeight={700}>{label}</Typography>
        <Chip
          label={status === 'loading' ? 'Analyzing' : status === 'error' ? 'Waveform unavailable' : 'Waveform'}
          color={status === 'error' ? 'warning' : 'default'}
          size="small"
        />
      </Stack>
      <Box
        component="canvas"
        ref={canvasRef}
        height={128}
        sx={{
          width: '100%',
          height: 128,
          border: 1,
          borderColor: 'divider',
          borderRadius: 1,
          bgcolor: 'rgba(255,255,255,0.03)',
        }}
      />
      <Box component="audio" controls src={audioSrc} sx={{ width: '100%' }} />
    </Stack>
  );
}

function clearWaveform(canvas: HTMLCanvasElement, context: CanvasRenderingContext2D) {
  const width = canvas.clientWidth || 640;
  const height = canvas.clientHeight || 128;
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
  const width = canvas.clientWidth || 640;
  const height = canvas.clientHeight || 128;
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

function drawWaveformError(canvas: HTMLCanvasElement, context: CanvasRenderingContext2D) {
  clearWaveform(canvas, context);
  const height = canvas.clientHeight || 128;
  context.fillStyle = 'rgba(246,180,75,0.85)';
  context.font = '14px sans-serif';
  context.fillText('Waveform unavailable for this preview', 16, height / 2);
}

function RestorationSlider({
  label,
  value,
  onChange,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
}) {
  return (
    <Stack spacing={0.5}>
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Typography fontWeight={700}>{label}</Typography>
        <Chip label={value === 0 ? 'Off' : `${value}%`} size="small" />
      </Stack>
      <Slider
        value={value}
        min={0}
        max={100}
        step={5}
        onChange={(_, nextValue) => onChange(Array.isArray(nextValue) ? nextValue[0] : nextValue)}
      />
    </Stack>
  );
}

function getAudioProfiles(settings?: AppSetting[]) {
  const value = settings?.find((setting) => setting.key === 'audioEnhancementProfiles')?.value.profiles;
  if (!Array.isArray(value)) {
    return starterAudioProfiles;
  }

  const profiles = value
    .map((profile) => normalizeAudioProfile(profile))
    .filter((profile): profile is AudioEnhancementProfile => Boolean(profile));

  return profiles.length ? mergeDefaultAudioProfiles(profiles) : starterAudioProfiles;
}

function mergeDefaultAudioProfiles(profiles: AudioEnhancementProfile[]) {
  const existingKeys = new Set(profiles.map((profile) => profile.key));
  const missingDefaults = starterAudioProfiles.filter((profile) => !existingKeys.has(profile.key));
  return [...profiles, ...missingDefaults];
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

function buildPreviewCommand(profile: AudioEnhancementProfile) {
  const codec = profile.outputCodec === 'copy' ? 'aac' : profile.outputCodec || 'aac';
  return `ffmpeg -i input.mkv -map 0:a:0 -af "${effectiveFilters(profile)}" -c:a ${codec} sample-audio.mka`;
}

function managedFilterLevel(filterChain: string, control: 'noise' | 'hum' | 'brightness') {
  const filters = splitAudioFilters(filterChain);
  if (control === 'noise') {
    const match = filters.find((filter) => filter.startsWith('afftdn='))?.match(/(?:^|:)nf=(-?\d+(?:\.\d+)?)/);
    const nf = Number(match?.[1]);
    return Number.isFinite(nf) ? Math.max(0, Math.min(100, Math.round((Math.abs(nf) - 20) / 0.3))) : 0;
  }
  if (control === 'hum') {
    const match = filters.find((filter) => filter.startsWith('highpass='))?.match(/(?:^|:)f=(\d+(?:\.\d+)?)/);
    const frequency = Number(match?.[1]);
    return Number.isFinite(frequency) ? Math.max(0, Math.min(100, Math.round(frequency - 50))) : 0;
  }
  const match = filters.find((filter) => filter.startsWith('treble='))?.match(/(?:^|:)g=(-?\d+(?:\.\d+)?)/);
  const gain = Number(match?.[1]);
  return Number.isFinite(gain) ? Math.max(0, Math.min(100, Math.round(gain * 10 + 50))) : 50;
}

function setManagedFilterLevel(filterChain: string, control: 'noise' | 'hum' | 'brightness', level: number) {
  const cleanLevel = Math.max(0, Math.min(100, Math.round(level)));
  const filters = splitAudioFilters(filterChain).filter((filter) => {
    if (control === 'noise') {
      return !filter.startsWith('afftdn=');
    }
    if (control === 'hum') {
      return !filter.startsWith('highpass=');
    }
    return !filter.startsWith('treble=');
  });

  if (control === 'noise' && cleanLevel > 0) {
    filters.unshift(`afftdn=nf=${trimGain(-20 - cleanLevel * 0.3)}`);
  }
  if (control === 'hum' && cleanLevel > 0) {
    filters.unshift(`highpass=f=${50 + cleanLevel}`);
  }
  if (control === 'brightness' && cleanLevel !== 50) {
    filters.push(`treble=g=${trimGain((cleanLevel - 50) / 10)}`);
  }

  return sanitizeAudioFilterChain(filters.join(','));
}

function splitAudioFilters(filterChain: string) {
  return filterChain
    .split(',')
    .map((filter) => filter.trim())
    .filter(Boolean);
}

function effectiveFilters(profile: AudioEnhancementProfile) {
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
  return filterChain.replace(/afftdn=([^,]*\bnf=)(-?\d+(?:\.\d+)?)/g, (_match, prefix: string, rawValue: string) => {
    const parsed = Number(rawValue);
    if (!Number.isFinite(parsed)) {
      return `afftdn=${prefix}${rawValue}`;
    }
    return `afftdn=${prefix}${trimGain(Math.max(-80, Math.min(-20, parsed)))}`;
  });
}

function normalizeEqBands(value: unknown) {
  const bands = defaultEqBands();
  if (!value || typeof value !== 'object') {
    return bands;
  }

  const candidate = value as Record<string, unknown>;
  eqFrequencies.forEach((frequency) => {
    const key = String(frequency);
    const gain = numberValue(candidate[key], 0);
    bands[key] = Math.max(-12, Math.min(12, gain));
  });
  return bands;
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

function slugify(value: string) {
  return value
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '');
}

function uniqueKey(key: string, profiles: AudioEnhancementProfile[]) {
  let index = 2;
  let candidate = `${key}-copy`;
  while (profiles.some((profile) => profile.key === candidate)) {
    candidate = `${key}-copy-${index}`;
    index += 1;
  }
  return candidate;
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
