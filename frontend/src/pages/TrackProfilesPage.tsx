import { Alert, Box, Button, Card, CardContent, Chip, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, MenuItem, Stack, Switch, Table, TableBody, TableCell, TableHead, TableRow, TextField, Typography } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../api/client';
import type { AppSetting, MediaStreamInfo, ScanResult, StreamMetadataOverride } from '../api/types';
import { MediaSnapshotDetails } from '../components/MediaSnapshotDetails';
import { PageHeader } from '../components/PageHeader';
import { emptyTrackProfile, getTrackProfiles, type SubtitleTransform, type TrackProfile } from '../trackProfiles';

export function TrackProfilesPage() {
  const client = useQueryClient();
  const navigate = useNavigate();
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const profiles = getTrackProfiles(settings.data, true);
  const [showDisabled, setShowDisabled] = useState(false);
  const [draft, setDraft] = useState<TrackProfile | null>(null);
  const update = useMutation({ mutationFn: api.updateSetting, onSuccess: () => client.invalidateQueries({ queryKey: ['settings'] }) });
  const save = () => {
    if (!draft) return;
    const key = slug(draft.key || draft.name);
    const next = [...profiles.filter((profile) => profile.key !== key), { ...draft, key }];
    update.mutate({ key: 'trackProfiles', value: { profiles: next } }, { onSuccess: () => setDraft(null) });
  };
  const toggle = (profile: TrackProfile) => update.mutate({ key: 'trackProfiles', value: { profiles: profiles.map((item) => item.key === profile.key ? { ...item, disabled: !item.disabled } : item) } });
  const visible = profiles.filter((profile) => showDisabled || (!profile.disabled && !profile.deletedAt));
  const sourceScan = findSourceScan(settings.data, draft);
  const updateStreamSelection = (type: MediaStreamInfo['type'], index: number, keep: boolean) => {
    if (!draft || !sourceScan) return;
    const key = type === 'video' ? 'keepVideoStreams' : type === 'audio' ? 'keepAudioStreams' : 'keepSubtitleStreams';
    const streams = type === 'video' ? sourceScan.videoStreams : type === 'audio' ? sourceScan.audioStreams : sourceScan.subtitleStreams;
    const selected = draft[key] ?? streams.map((stream) => stream.index);
    const next = keep ? uniqueNumbers([...selected, index]) : selected.filter((candidate) => candidate !== index);
    setDraft({ ...draft, [key]: next });
  };
  const updateStreamMetadata = (type: MediaStreamInfo['type'], index: number, patch: StreamMetadataOverride) => {
    if (!draft) return;
    const key = type === 'video' ? 'videoMetadata' : type === 'audio' ? 'audioMetadata' : 'subtitleMetadata';
    setDraft({ ...draft, [key]: { ...(draft[key] ?? {}), [String(index)]: { ...(draft[key]?.[String(index)] ?? {}), ...patch } } });
  };
  const updateSubtitleTransform = (stream: MediaStreamInfo, format: '' | 'srt' | 'ass') => {
    if (!draft) return;
    const existing = draft.subtitleTransforms ?? [];
    const remaining = existing.filter((item) => item.streamIndex !== stream.index);
    setDraft({
      ...draft,
      subtitleTransforms: format ? [...remaining, {
        streamIndex: stream.index,
        format,
        removeEmbedded: true,
        makeDefault: true,
        language: stream.language || 'und',
        title: stream.title || undefined,
      }] : remaining,
    });
  };
  const updateSubtitleTransformValue = (streamIndex: number, patch: Partial<SubtitleTransform>) => {
    if (!draft) return;
    setDraft({
      ...draft,
      subtitleTransforms: (draft.subtitleTransforms ?? []).map((item) => item.streamIndex === streamIndex ? { ...item, ...patch } : item),
    });
  };
  return <>
    <PageHeader title="Track Profiles" eyebrow="Stream selection"><Typography color="text.secondary" sx={{ mt: 1 }}>Reusable video, audio, subtitle and metadata selection rules.</Typography></PageHeader>
    <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}><Stack spacing={2}>
      {settings.isError ? <Alert severity="warning">Unable to load track profiles.</Alert> : null}
      <Stack direction="row" justifyContent="space-between"><FormControlLabel control={<Switch checked={showDisabled} onChange={(_, value) => setShowDisabled(value)} />} label="Show disabled" /><Button startIcon={<AddIcon />} variant="contained" onClick={() => setDraft({ ...emptyTrackProfile })}>Add Track Profile</Button></Stack>
      <Card><CardContent sx={{ p: 0 }}><Table><TableHead><TableRow><TableCell>Name</TableCell><TableCell>Streams</TableCell><TableCell>Rules</TableCell><TableCell>Status</TableCell><TableCell /></TableRow></TableHead><TableBody>
        {visible.map((profile) => <TableRow key={profile.key}><TableCell><Typography fontWeight={700}>{profile.name}</Typography><Typography variant="body2" color="text.secondary">{profile.description || profile.key}</Typography></TableCell><TableCell>V {list(profile.keepVideoStreams)} · A {list(profile.keepAudioStreams)} · S {list(profile.keepSubtitleStreams)}</TableCell><TableCell>{profile.audioMode} / {profile.subtitleMode}</TableCell><TableCell><Chip size="small" label={profile.disabled ? 'Disabled' : 'Active'} color={profile.disabled ? 'default' : 'success'} /></TableCell><TableCell><Button startIcon={<EditIcon />} onClick={() => navigate(`/profile-lab?trackProfileKey=${encodeURIComponent(profile.key)}`)}>Edit</Button><Button onClick={() => toggle(profile)}>{profile.disabled ? 'Enable' : 'Disable'}</Button></TableCell></TableRow>)}
        {!visible.length ? <TableRow><TableCell colSpan={5}><Alert severity="info">No track profiles yet. Create one here or from Profile Lab.</Alert></TableCell></TableRow> : null}
      </TableBody></Table></CardContent></Card>
    </Stack></Box>
    <Dialog open={Boolean(draft)} onClose={() => setDraft(null)} maxWidth="lg" fullWidth><DialogTitle>New Track Profile</DialogTitle><DialogContent>{draft ? <Stack spacing={2} sx={{ mt: 1 }}>
      <TextField label="Name" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} required /><TextField label="Description" value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} multiline />
      {sourceScan ? (
        <>
          <Alert severity="info">
            Editing from the original snapshot for {draft.sourceAssetName || draft.sourceAssetPath}. Tracks removed from the converted asset remain available here as profile rules; applying them requires reprocessing from the archived original.
          </Alert>
          <MediaSnapshotDetails
            scan={sourceScan}
            streamControls={{
              video: { selected: selectedStreams(draft.keepVideoStreams, sourceScan.videoStreams), onToggle: (index, keep) => updateStreamSelection('video', index, keep) },
              audio: { selected: selectedStreams(draft.keepAudioStreams, sourceScan.audioStreams), onToggle: (index, keep) => updateStreamSelection('audio', index, keep) },
              subtitle: { selected: selectedStreams(draft.keepSubtitleStreams, sourceScan.subtitleStreams), onToggle: (index, keep) => updateStreamSelection('subtitle', index, keep) },
            }}
            metadataControls={{
              video: { values: draft.videoMetadata ?? {}, onChange: (index, patch) => updateStreamMetadata('video', index, patch) },
              audio: { values: draft.audioMetadata ?? {}, onChange: (index, patch) => updateStreamMetadata('audio', index, patch) },
              subtitle: { values: draft.subtitleMetadata ?? {}, onChange: (index, patch) => updateStreamMetadata('subtitle', index, patch) },
            }}
          />
          <Stack spacing={1.5}>
            <Typography variant="h3">External subtitle transformations</Typography>
            <Typography color="text.secondary" variant="body2">
              Generate a sidecar beside the published video, validate it, and remove the embedded source track. Video and audio remain untouched in a track-only job.
            </Typography>
            {sourceScan.subtitleStreams.map((stream) => {
              const transform = draft.subtitleTransforms?.find((item) => item.streamIndex === stream.index);
              const bitmap = isBitmapSubtitle(stream.codec);
              return (
                <Box key={stream.index} sx={{ p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1 }}>
                  <Stack spacing={1}>
                    <Typography fontWeight={700}>
                      #{stream.index} · {(stream.language || 'und').toUpperCase()} · {stream.codec.toUpperCase()} {stream.title ? `· ${stream.title}` : ''}
                    </Typography>
                    {bitmap ? (
                      <Alert severity="warning">This is a bitmap subtitle. OCR is required before SRT/ASS replacement, so direct transformation is disabled.</Alert>
                    ) : null}
                    <Stack direction={{ xs: 'column', md: 'row' }} spacing={1.5}>
                      <TextField
                        select
                        fullWidth
                        label="Action"
                        value={transform?.format ?? ''}
                        onChange={(event) => updateSubtitleTransform(stream, event.target.value as '' | 'srt' | 'ass')}
                      >
                        <MenuItem value="">Keep embedded track</MenuItem>
                        <MenuItem value="srt" disabled={bitmap}>Create SRT and remove embedded</MenuItem>
                        <MenuItem value="ass" disabled={bitmap}>Create ASS and remove embedded</MenuItem>
                      </TextField>
                      {transform ? (
                        <>
                          <TextField
                            label="Language"
                            value={transform.language}
                            onChange={(event) => updateSubtitleTransformValue(stream.index, { language: event.target.value.toLowerCase() })}
                            fullWidth
                          />
                          <FormControlLabel
                            control={<Switch checked={transform.makeDefault} onChange={(_, value) => updateSubtitleTransformValue(stream.index, { makeDefault: value })} />}
                            label="Default sidecar"
                          />
                        </>
                      ) : null}
                    </Stack>
                  </Stack>
                </Box>
              );
            })}
          </Stack>
        </>
      ) : draft.sourceAssetPath || draft.sourceAssetName ? (
        <Alert severity="warning">The profile keeps its source asset reference, but no matching original analysis snapshot is available.</Alert>
      ) : null}
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}><IndexField label="Video stream indexes" value={draft.keepVideoStreams} onChange={(value) => setDraft({ ...draft, keepVideoStreams: value })} /><IndexField label="Audio stream indexes" value={draft.keepAudioStreams} onChange={(value) => setDraft({ ...draft, keepAudioStreams: value })} /><IndexField label="Subtitle stream indexes" value={draft.keepSubtitleStreams} onChange={(value) => setDraft({ ...draft, keepSubtitleStreams: value })} /></Stack>
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={2}><TextField select fullWidth label="Audio rule" value={draft.audioMode} onChange={(e) => setDraft({ ...draft, audioMode: e.target.value as TrackProfile['audioMode'] })}>{['all','default','languages','none'].map(v => <MenuItem key={v} value={v}>{v}</MenuItem>)}</TextField><TextField select fullWidth label="Subtitle rule" value={draft.subtitleMode} onChange={(e) => setDraft({ ...draft, subtitleMode: e.target.value as TrackProfile['subtitleMode'] })}>{['all','none','forced','languages','forced-or-languages'].map(v => <MenuItem key={v} value={v}>{v}</MenuItem>)}</TextField><TextField select fullWidth label="Validation" value={draft.validationMode} onChange={(e) => setDraft({ ...draft, validationMode: e.target.value as TrackProfile['validationMode'] })}>{['block','review','warn'].map(v => <MenuItem key={v} value={v}>{v}</MenuItem>)}</TextField></Stack>
      <TextField label="Audio languages (comma separated)" value={draft.audioLanguages.join(', ')} onChange={(e) => setDraft({ ...draft, audioLanguages: csv(e.target.value) })} /><TextField label="Subtitle languages (comma separated)" value={draft.subtitleLanguages.join(', ')} onChange={(e) => setDraft({ ...draft, subtitleLanguages: csv(e.target.value) })} /><TextField label="Notes" value={draft.notes} onChange={(e) => setDraft({ ...draft, notes: e.target.value })} multiline />
    </Stack> : null}</DialogContent><DialogActions><Button onClick={() => setDraft(null)}>Cancel</Button><Button variant="contained" onClick={save} disabled={!draft?.name || update.isPending}>Save</Button></DialogActions></Dialog>
  </>;
}

function IndexField({ label, value, onChange }: { label: string; value?: number[]; onChange: (value?: number[]) => void }) { return <TextField fullWidth label={label} value={value?.join(', ') ?? ''} onChange={(e) => { const values = e.target.value.split(',').map(Number).filter(Number.isInteger); onChange(e.target.value.trim() ? values : undefined); }} helperText="FFprobe stream indexes" />; }
function csv(value: string) { return value.split(',').map((item) => item.trim().toLowerCase()).filter(Boolean); }
function list(value?: number[]) { return value ? value.join(', ') || 'none' : 'rule'; }
function slug(value: string) { return value.trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, ''); }
function uniqueNumbers(values: number[]) { return [...new Set(values)].sort((a, b) => a - b); }
function selectedStreams(value: number[] | undefined, streams: MediaStreamInfo[]) { return value ?? streams.map((stream) => stream.index); }
function findSourceScan(settings: AppSetting[] | undefined, profile: TrackProfile | null): ScanResult | undefined {
  if (!profile?.sourceAssetPath && !profile?.sourceAssetName) return undefined;
  const records = settings?.find((setting) => setting.key === 'analysisRecords')?.value.records;
  if (!Array.isArray(records)) return undefined;
  const normalizedPath = normalizePath(profile.sourceAssetPath ?? '');
  const record = records.find((value) => {
    if (!value || typeof value !== 'object') return false;
    const item = value as Record<string, unknown>;
    const assetPath = typeof item.assetPath === 'string' ? item.assetPath : '';
    const assetName = typeof item.assetName === 'string' ? item.assetName : '';
    return Boolean((normalizedPath && normalizePath(assetPath) === normalizedPath) || (profile.sourceAssetName && assetName === profile.sourceAssetName));
  });
  if (!record || typeof record !== 'object') return undefined;
  const scan = (record as Record<string, unknown>).scan;
  return scan && typeof scan === 'object' ? scan as ScanResult : undefined;
}
function normalizePath(value: string) { return value.replaceAll('\\', '/').replace(/\/+/g, '/').replace(/\/$/, '').toLowerCase(); }
function isBitmapSubtitle(codec: string) { return ['dvd_subtitle', 'hdmv_pgs_subtitle', 'pgssub', 'dvb_subtitle', 'xsub'].includes(codec.toLowerCase()); }
