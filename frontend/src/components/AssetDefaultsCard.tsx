import {
  Alert,
  Button,
  Card,
  CardContent,
  FormControlLabel,
  Grid,
  IconButton,
  MenuItem,
  Stack,
  Switch,
  TextField,
  Typography,
} from '@mui/material';
import SaveIcon from '@mui/icons-material/Save';
import DeleteIcon from '@mui/icons-material/DeleteOutline';
import { useEffect, useState } from 'react';
import type { ReactNode } from 'react';
import type { AppSetting, Profile } from '../api/types';
import {
  assetDefaultProfileReference,
  assetDefaultsValue,
  preferredSubtitleRulesValue,
  type AssetDefaultsDomain,
  type AssetDefaultsValue,
  updateAssetDefaultsDomain,
} from '../utils/assetDefaults';
import { subtitleLanguageOptions, subtitlePreferredActionOptions } from '../utils/subtitleLanguages';

type Props = {
  value: unknown;
  settings: AppSetting[];
  profiles: Profile[];
  runtimeEncoders?: Record<string, { listed?: boolean; usable?: boolean; reason?: string }>;
  saving: boolean;
  onSave: (value: AssetDefaultsValue) => void;
};

function profileOptions(settings: AppSetting[], key: string) {
  const setting = settings.find((item) => item.key === key)?.value;
  const profiles = setting && Array.isArray(setting.profiles) ? setting.profiles : [];
  return profiles.filter((profile): profile is Record<string, unknown> => Boolean(profile) && typeof profile === 'object')
    .filter((profile) => profile.disabled !== true && !profile.deletedAt)
    .map((profile) => ({
      key: String(profile.key ?? ''),
      name: String(profile.name ?? profile.key ?? ''),
    }))
    .filter((profile) => profile.key);
}

function SourceSelector({
  domain,
  profileOptions: options,
  onChange,
}: {
  domain: AssetDefaultsDomain;
  profileOptions: Array<{ key: string; name: string }>;
  onChange: (source: 'custom' | 'profile', reference?: string) => void;
}) {
  return <Grid container spacing={1.5}>
    <Grid size={{ xs: 12, md: 4 }}>
      <TextField select fullWidth label="Source" value={domain.source} onChange={(event) => onChange(event.target.value as 'custom' | 'profile')}>
        <MenuItem value="custom">Custom</MenuItem>
        <MenuItem value="profile">Existing profile</MenuItem>
      </TextField>
    </Grid>
    {domain.source === 'profile' ? <Grid size={{ xs: 12, md: 8 }}>
      <TextField select fullWidth label="Profile reference" value={String(assetDefaultProfileReference(domain) ?? '')} onChange={(event) => onChange('profile', event.target.value)}>
        {options.map((option) => <MenuItem key={option.key} value={option.key}>{option.name}</MenuItem>)}
      </TextField>
      {options.length === 0 ? <Alert severity="info" sx={{ mt: 1 }}>No available profile references were returned by the existing profile settings.</Alert> : null}
    </Grid> : null}
  </Grid>;
}

function AssetDefaultsDomainCard({
  title,
  domain,
  options,
  children,
  onChange,
}: {
  title: string;
  domain: AssetDefaultsDomain;
  options: Array<{ key: string; name: string }>;
  children: ReactNode;
  onChange: (patch: Record<string, unknown>) => void;
}) {
  return <Card><CardContent><Stack spacing={2}>
    <Stack><Typography variant="h3">{title}</Typography><Typography color="text.secondary" variant="body2">Fallback for assets without a more-specific decision.</Typography></Stack>
    <SourceSelector
      domain={domain}
      profileOptions={options}
      onChange={(source, reference) => onChange(source === 'custom' ? { source, profileKey: '', profileId: 0 } : { source, profileKey: reference ?? '', profileId: 0, custom: {} })}
    />
    {domain.source === 'custom' ? children : <Typography color="text.secondary" variant="body2">The existing profile remains authoritative; its editable fields are not copied into Asset Defaults.</Typography>}
  </Stack></CardContent></Card>;
}

export function AssetDefaultsCard({ value, settings, profiles, runtimeEncoders = {}, saving, onSave }: Props) {
  const [draft, setDraft] = useState<AssetDefaultsValue>(() => assetDefaultsValue(value));
  useEffect(() => setDraft(assetDefaultsValue(value)), [value]);
  const update = (domain: 'video' | 'audio' | 'tracks', patch: Record<string, unknown>) => setDraft((current) => updateAssetDefaultsDomain(current, domain, patch));
  const videoOptions = profiles.filter((profile) => !profile.disabled).map((profile) => ({ key: String(profile.id), name: profile.name }));
  const encoderOptions = Object.entries(runtimeEncoders)
    .filter(([, capability]) => capability.usable)
    .map(([name]) => name)
    .sort();
  const audioOptions = profileOptions(settings, 'audioEnhancementProfiles');
  const trackOptions = profileOptions(settings, 'trackProfiles');
  const video = draft.video;
  const audio = draft.audio;
  const tracks = draft.tracks;
  const audioCustom = audio.custom;
  const trackCustom = tracks.custom;
  const formats = Array.isArray(trackCustom.subtitleSidecarFormats) ? trackCustom.subtitleSidecarFormats.join(',') : 'srt';
  const preferredRules = preferredSubtitleRulesValue(trackCustom.preferredSubtitleRules);
  const updatePreferredRules = (next: typeof preferredRules) => update('tracks', { custom: { preferredSubtitleRules: next } });

  return <Stack spacing={2}>
    <Alert severity="info">These defaults apply only when no asset, path, logical-group, or source-group policy is present. Existing assignments and queued snapshots remain authoritative.</Alert>
    <AssetDefaultsDomainCard title="Video General" domain={video} options={videoOptions} onChange={(patch) => {
      if (patch.source === 'profile') {
        const profileId = Number(patch.profileKey ?? 0);
        setDraft((current) => updateAssetDefaultsDomain(current, 'video', { source: 'profile', profileId, profileKey: '' }));
      } else update('video', patch);
    }}>
      <Grid container spacing={1.5}>
        <Grid size={{ xs: 12, md: 6 }}><TextField select fullWidth label="Video handling" value={String(video.custom.videoCodec ?? 'copy')} onChange={(event) => update('video', { custom: { videoCodec: event.target.value } })} helperText="Copy preserves the primary video stream; encoding uses the selected runtime encoder."><MenuItem value="copy">Copy</MenuItem><MenuItem value="x264">Encode as H.264</MenuItem><MenuItem value="x265">Encode as HEVC</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12, md: 6 }}><TextField select fullWidth label="Preferred encoder" value={String(video.custom.preferredEncoder ?? 'auto')} onChange={(event) => update('video', { custom: { preferredEncoder: event.target.value } })} helperText="Only encoders reported usable by the runtime are offered."><MenuItem value="auto">Automatic</MenuItem>{encoderOptions.map((encoder) => <MenuItem key={encoder} value={encoder}>{encoder}</MenuItem>)}</TextField></Grid>
        <Grid size={{ xs: 12 }}><FormControlLabel control={<Switch checked={Boolean(video.removeNonPrimaryVideoTracks)} onChange={(event) => update('video', { removeNonPrimaryVideoTracks: event.target.checked })} />} label="Remove eligible non-primary video tracks" /></Grid>
      </Grid>
    </AssetDefaultsDomainCard>
    <AssetDefaultsDomainCard title="Audio General" domain={audio} options={audioOptions} onChange={(patch) => update('audio', patch)}>
      <Grid container spacing={1.5}>
        <Grid size={{ xs: 12, md: 4 }}><FormControlLabel control={<Switch checked={Boolean(audioCustom.addCompatibleTrack)} onChange={(event) => update('audio', { custom: { addCompatibleTrack: event.target.checked } })} />} label="Add compatible track" /></Grid>
        <Grid size={{ xs: 12, md: 4 }}><TextField fullWidth type="number" label="Quality / bitrate" value={Number(audioCustom.compatibleTrackQuality ?? 192)} onChange={(event) => update('audio', { custom: { compatibleTrackQuality: Number(event.target.value) } })} /></Grid>
        <Grid size={{ xs: 12, md: 4 }}><TextField select fullWidth label="Channels" value={String(audioCustom.compatibleTrackChannels ?? 2)} onChange={(event) => update('audio', { custom: { compatibleTrackChannels: Number(event.target.value) } })}><MenuItem value="1">1</MenuItem><MenuItem value="2">2</MenuItem><MenuItem value="6">6</MenuItem><MenuItem value="8">8</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12, md: 6 }}><TextField select fullWidth label="Source stream" value={String(audioCustom.compatibleTrackSource ?? 'resolved')} onChange={(event) => update('audio', { custom: { compatibleTrackSource: event.target.value } })}><MenuItem value="resolved">Resolved audio stream</MenuItem><MenuItem value="default">Default audio stream</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12, md: 6 }}><TextField fullWidth label="Preferred default language" value={String(audioCustom.preferredDefaultLanguage ?? '')} onChange={(event) => update('audio', { custom: { preferredDefaultLanguage: event.target.value } })} placeholder="eng" /></Grid>
      </Grid>
    </AssetDefaultsDomainCard>
    <AssetDefaultsDomainCard title="Tracks General" domain={tracks} options={trackOptions} onChange={(patch) => update('tracks', patch)}>
      <Grid container spacing={1.5}>
        <Grid size={{ xs: 12, md: 4 }}><TextField select fullWidth label="Subtitle action" value={String(trackCustom.subtitleDisposition ?? 'keep')} onChange={(event) => update('tracks', { custom: { subtitleDisposition: event.target.value } })}><MenuItem value="keep">Keep</MenuItem><MenuItem value="remove">Remove</MenuItem><MenuItem value="extract">Extract</MenuItem><MenuItem value="keep_and_extract">Keep and extract</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12, md: 4 }}><TextField select fullWidth label="Converted sidecar formats" value={formats} onChange={(event) => update('tracks', { custom: { subtitleSidecarFormats: event.target.value.split(',').map((item) => item.trim()).filter(Boolean) } })}><MenuItem value="srt">SRT</MenuItem><MenuItem value="ass">ASS</MenuItem><MenuItem value="both">Both (SRT and ASS)</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12, md: 4 }}><TextField select fullWidth label="Attachment policy" value={String(trackCustom.attachmentPolicy ?? 'auto')} onChange={(event) => update('tracks', { custom: { attachmentPolicy: event.target.value } })}><MenuItem value="auto">Auto</MenuItem><MenuItem value="keep">Keep</MenuItem><MenuItem value="remove">Remove</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12, md: 6 }}><TextField select fullWidth label="Font attachment policy" value={String(trackCustom.fontAttachmentExportPolicy ?? 'none')} onChange={(event) => update('tracks', { custom: { fontAttachmentExportPolicy: event.target.value } })}><MenuItem value="none">Do not export</MenuItem><MenuItem value="all">Export font attachments</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12, md: 6 }}><TextField select fullWidth label="Bitmap OCR mode" value={String(trackCustom.ocrMode ?? 'accurate')} onChange={(event) => update('tracks', { custom: { ocrMode: event.target.value } })}><MenuItem value="raw">Raw · one pass</MenuItem><MenuItem value="clean">Clean · corrected</MenuItem><MenuItem value="accurate">Accurate · two passes</MenuItem></TextField></Grid>
        <Grid size={{ xs: 12 }}>
          <Stack spacing={1}>
            <Typography variant="subtitle1">Preferred subtitle languages</Typography>
            <Typography variant="body2" color="text.secondary">Apply these actions by canonical subtitle language when no more-specific stream or Track Profile rule applies.</Typography>
            {preferredRules.map((rule, index) => <Grid container spacing={1} key={`${rule.language}-${index}`} alignItems="center">
              <Grid size={{ xs: 12, md: 5 }}><TextField select fullWidth label={`Language ${index + 1}`} value={rule.language} onChange={(event) => updatePreferredRules(preferredRules.map((item, itemIndex) => itemIndex === index ? { ...item, language: event.target.value } : item))}>
                {subtitleLanguageOptions.filter((option) => option.value === rule.language || !preferredRules.some((item, itemIndex) => itemIndex !== index && item.language === option.value)).map((option) => <MenuItem key={option.value} value={option.value}>{option.label} · {option.value}</MenuItem>)}
              </TextField></Grid>
              <Grid size={{ xs: 12, md: 5 }}><TextField select fullWidth label="Action" value={rule.action} onChange={(event) => updatePreferredRules(preferredRules.map((item, itemIndex) => itemIndex === index ? { ...item, action: event.target.value as typeof rule.action } : item))}>{subtitlePreferredActionOptions.map((option) => <MenuItem key={option.value} value={option.value}>{option.label}</MenuItem>)}</TextField></Grid>
              <Grid size={{ xs: 12, md: 2 }}><IconButton aria-label={`Remove preferred subtitle language ${rule.language}`} onClick={() => updatePreferredRules(preferredRules.filter((_, itemIndex) => itemIndex !== index))}><DeleteIcon /></IconButton></Grid>
            </Grid>)}
            <Button variant="outlined" onClick={() => {
              const language = subtitleLanguageOptions.find((option) => !preferredRules.some((rule) => rule.language === option.value))?.value;
              if (language) updatePreferredRules([...preferredRules, { language, action: 'keep_and_extract' }]);
            }} disabled={preferredRules.length >= subtitleLanguageOptions.length}>+ Add language</Button>
          </Stack>
        </Grid>
        <Grid size={{ xs: 12, md: 6 }}><TextField fullWidth label="Default subtitle language" value={String(trackCustom.defaultSubtitleLanguage ?? '')} onChange={(event) => update('tracks', { custom: { defaultSubtitleLanguage: event.target.value } })} placeholder="eng" /></Grid>
      </Grid>
    </AssetDefaultsDomainCard>
    <Button startIcon={<SaveIcon />} variant="contained" disabled={saving} onClick={() => onSave(draft)}>Save Asset Defaults</Button>
  </Stack>;
}
