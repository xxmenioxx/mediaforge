import { Alert, Box, Button, Card, CardContent, Chip, Dialog, DialogActions, DialogContent, DialogTitle, FormControlLabel, MenuItem, Stack, Switch, Table, TableBody, TableCell, TableHead, TableRow, TextField, Typography } from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import EditIcon from '@mui/icons-material/Edit';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import { emptyTrackProfile, getTrackProfiles, type TrackProfile } from '../trackProfiles';

export function TrackProfilesPage() {
  const client = useQueryClient();
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
  return <>
    <PageHeader title="Track Profiles" eyebrow="Stream selection"><Typography color="text.secondary" sx={{ mt: 1 }}>Reusable video, audio, subtitle and metadata selection rules.</Typography></PageHeader>
    <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}><Stack spacing={2}>
      {settings.isError ? <Alert severity="warning">Unable to load track profiles.</Alert> : null}
      <Stack direction="row" justifyContent="space-between"><FormControlLabel control={<Switch checked={showDisabled} onChange={(_, value) => setShowDisabled(value)} />} label="Show disabled" /><Button startIcon={<AddIcon />} variant="contained" onClick={() => setDraft({ ...emptyTrackProfile })}>Add Track Profile</Button></Stack>
      <Card><CardContent sx={{ p: 0 }}><Table><TableHead><TableRow><TableCell>Name</TableCell><TableCell>Streams</TableCell><TableCell>Rules</TableCell><TableCell>Status</TableCell><TableCell /></TableRow></TableHead><TableBody>
        {visible.map((profile) => <TableRow key={profile.key}><TableCell><Typography fontWeight={700}>{profile.name}</Typography><Typography variant="body2" color="text.secondary">{profile.description || profile.key}</Typography></TableCell><TableCell>V {list(profile.keepVideoStreams)} · A {list(profile.keepAudioStreams)} · S {list(profile.keepSubtitleStreams)}</TableCell><TableCell>{profile.audioMode} / {profile.subtitleMode}</TableCell><TableCell><Chip size="small" label={profile.disabled ? 'Disabled' : 'Active'} color={profile.disabled ? 'default' : 'success'} /></TableCell><TableCell><Button startIcon={<EditIcon />} onClick={() => setDraft({ ...profile })}>Edit</Button><Button onClick={() => toggle(profile)}>{profile.disabled ? 'Enable' : 'Disable'}</Button></TableCell></TableRow>)}
        {!visible.length ? <TableRow><TableCell colSpan={5}><Alert severity="info">No track profiles yet. Create one here or from Profile Lab.</Alert></TableCell></TableRow> : null}
      </TableBody></Table></CardContent></Card>
    </Stack></Box>
    <Dialog open={Boolean(draft)} onClose={() => setDraft(null)} maxWidth="md" fullWidth><DialogTitle>{draft?.key ? 'Edit Track Profile' : 'New Track Profile'}</DialogTitle><DialogContent>{draft ? <Stack spacing={2} sx={{ mt: 1 }}>
      <TextField label="Name" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} required /><TextField label="Description" value={draft.description} onChange={(e) => setDraft({ ...draft, description: e.target.value })} multiline />
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
