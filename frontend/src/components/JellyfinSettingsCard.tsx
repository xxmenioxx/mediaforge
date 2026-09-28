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
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import SaveIcon from '@mui/icons-material/Save';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { api } from '../api/client';
import type { JellyfinIntegrationOutcome, Library } from '../api/types';

type Mapping = { mvforgeLibraryId: number; jellyfinLibraryId: string; jellyfinName?: string };
type Draft = {
  enabled: boolean;
  baseUrl: string;
  credentialConfigured: boolean;
  refreshAfterPublish: boolean;
  refreshTestEncode: boolean;
  debounceSeconds: number;
  mappings: Mapping[];
};

type Props = {
  value?: Record<string, unknown>;
  libraries: Library[];
  saving: boolean;
  onSave: (value: Record<string, unknown>) => void;
};

function jellyfinDraft(value?: Record<string, unknown>): Draft {
  const mappings = Array.isArray(value?.mappings)
    ? value.mappings.flatMap((raw): Mapping[] => {
        if (typeof raw !== 'object' || raw === null) return [];
        const item = raw as Record<string, unknown>;
        return typeof item.mvforgeLibraryId === 'number' && typeof item.jellyfinLibraryId === 'string'
          ? [{ mvforgeLibraryId: item.mvforgeLibraryId, jellyfinLibraryId: item.jellyfinLibraryId, jellyfinName: typeof item.jellyfinName === 'string' ? item.jellyfinName : undefined }]
          : [];
      })
    : [];
  return {
    enabled: value?.enabled === true,
    baseUrl: typeof value?.baseUrl === 'string' ? value.baseUrl : '',
    credentialConfigured: value?.credentialConfigured === true,
    refreshAfterPublish: value?.refreshAfterPublish !== false,
    refreshTestEncode: value?.refreshTestEncode !== false,
    debounceSeconds: typeof value?.debounceSeconds === 'number' ? Math.max(0, Math.min(60, value.debounceSeconds)) : 10,
    mappings,
  };
}

function statusLabel(outcome: JellyfinIntegrationOutcome) {
  if (outcome.status === 'success') return outcome.effectiveScope === 'server_wide' ? 'Server scan requested' : 'Refresh accepted';
  if (outcome.status === 'retryable_failure' || outcome.status === 'permanent_configuration_error') return 'Failed';
  if (outcome.status === 'skipped_disabled') return 'Skipped · disabled';
  if (outcome.status === 'skipped_unmapped') return 'Skipped · no mapping';
  return 'Pending';
}

export function JellyfinSettingsCard({ value, libraries, saving, onSave }: Props) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<Draft>(() => jellyfinDraft(value));
  const [credentialDraft, setCredentialDraft] = useState('');
  const [clearCredential, setClearCredential] = useState(false);
  const [selectedMVForgeLibrary, setSelectedMVForgeLibrary] = useState<number | ''>('');
  const [selectedJellyfinLibrary, setSelectedJellyfinLibrary] = useState('');
  const [manualLibraryId, setManualLibraryId] = useState<number | ''>('');
  const outcomes = useQuery({ queryKey: ['jellyfinOutcomes'], queryFn: api.jellyfinOutcomes, refetchInterval: 5000 });
  const connection = useMutation({ mutationFn: api.testJellyfinConnection });
  const manualRefresh = useMutation({
    mutationFn: api.manualJellyfinRefresh,
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ['jellyfinOutcomes'] }),
  });
  const discoveredLibraries = connection.data?.libraries ?? [];
  const latestOutcome = outcomes.data?.[0];
  const canAddMapping = selectedMVForgeLibrary !== '' && selectedJellyfinLibrary !== '' && !draft.mappings.some((mapping) => mapping.jellyfinLibraryId === selectedJellyfinLibrary && mapping.mvforgeLibraryId !== selectedMVForgeLibrary);

  const save = () => onSave({
    ...draft,
    mappings: draft.mappings,
    ...(credentialDraft ? { apiToken: credentialDraft } : {}),
    ...(clearCredential ? { clearCredential: true } : {}),
  });

  const addMapping = () => {
    if (selectedMVForgeLibrary === '' || !selectedJellyfinLibrary) return;
    const selected = discoveredLibraries.find((library) => library.id === selectedJellyfinLibrary);
    const mapping = { mvforgeLibraryId: selectedMVForgeLibrary, jellyfinLibraryId: selectedJellyfinLibrary, jellyfinName: selected?.name };
    setDraft((current) => ({
      ...current,
      mappings: [...current.mappings.filter((item) => item.mvforgeLibraryId !== selectedMVForgeLibrary), mapping],
    }));
    setManualLibraryId(selectedMVForgeLibrary);
  };
  const effectiveManualLibraryId = draft.mappings.some((mapping) => mapping.mvforgeLibraryId === manualLibraryId)
    ? manualLibraryId
    : draft.mappings[0]?.mvforgeLibraryId ?? '';

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack spacing={0.5}>
            <Typography variant="h3">Jellyfin library refresh</Typography>
            <Typography color="text.secondary" variant="body2">
              Refresh after canonical publication and consumable Test Encode changes. Refresh requests use Jellyfin’s server-wide scan and are debounced by the effective scan target.
            </Typography>
          </Stack>
          <FormControlLabel
            control={<Switch checked={draft.enabled} onChange={(event) => setDraft((current) => ({ ...current, enabled: event.target.checked }))} />}
            label="Enable Jellyfin integration"
          />
          <Grid container spacing={1.5}>
            <Grid size={{ xs: 12, md: 7 }}>
              <TextField fullWidth label="Jellyfin Base URL" value={draft.baseUrl} onChange={(event) => setDraft((current) => ({ ...current, baseUrl: event.target.value }))} placeholder="http://jellyfin:8096" />
            </Grid>
            <Grid size={{ xs: 12, md: 5 }}>
              <TextField
                fullWidth
                type="password"
                autoComplete="new-password"
                label={draft.credentialConfigured ? 'Replace API key / token (optional)' : 'API key / token'}
                value={credentialDraft}
                onChange={(event) => { setCredentialDraft(event.target.value); setClearCredential(false); }}
                helperText={draft.credentialConfigured ? 'Credential is configured; stored value is never returned.' : 'Credential is write-only.'}
              />
            </Grid>
            {draft.credentialConfigured ? <Grid size={{ xs: 12 }}><Button size="small" color="warning" onClick={() => { setCredentialDraft(''); setClearCredential(true); }}>Clear stored credential</Button></Grid> : null}
            <Grid size={{ xs: 12, sm: 6 }}>
              <FormControlLabel control={<Switch checked={draft.refreshAfterPublish} onChange={(event) => setDraft((current) => ({ ...current, refreshAfterPublish: event.target.checked }))} />} label="Refresh after publish" />
            </Grid>
            <Grid size={{ xs: 12, sm: 6 }}>
              <FormControlLabel control={<Switch checked={draft.refreshTestEncode} onChange={(event) => setDraft((current) => ({ ...current, refreshTestEncode: event.target.checked }))} />} label="Refresh Test Encode changes" />
            </Grid>
            <Grid size={{ xs: 12, sm: 4 }}>
              <TextField fullWidth type="number" label="Debounce seconds" value={draft.debounceSeconds} inputProps={{ min: 0, max: 60 }} onChange={(event) => setDraft((current) => ({ ...current, debounceSeconds: Math.max(0, Math.min(60, Number(event.target.value) || 0)) }))} helperText="Default 10 seconds; maximum 60." />
            </Grid>
          </Grid>
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1}>
            <Button variant="contained" startIcon={<SaveIcon />} disabled={saving} onClick={save}>Save Jellyfin settings</Button>
            <Button variant="outlined" disabled={connection.isPending || !draft.baseUrl || !draft.credentialConfigured} onClick={() => connection.mutate()}>
              {connection.isPending ? 'Testing…' : 'Test connection and discover libraries'}
            </Button>
          </Stack>
          {connection.data ? (
            <Alert severity={connection.data.authenticated ? 'success' : 'warning'}>
              {connection.data.authenticated
                ? `Connected${connection.data.versionAvailable ? ` · Jellyfin ${connection.data.serverVersion}` : ' · version unavailable'} · ${discoveredLibraries.length} libraries discovered.`
                : `Connection unavailable (${connection.data.failureClass ?? 'unknown'}).`}
            </Alert>
          ) : null}
          {connection.isError ? <Alert severity="warning">Connection test failed.</Alert> : null}
          <Typography variant="h4">Library ID mappings</Typography>
          <Typography color="text.secondary" variant="body2">Mappings bind MVForge Library IDs to the exact IDs returned by Jellyfin. Display names can change without changing the mapping.</Typography>
          <Grid container spacing={1.5} alignItems="center">
            <Grid size={{ xs: 12, md: 4 }}>
              <TextField select fullWidth label="MVForge Library" value={selectedMVForgeLibrary} onChange={(event) => setSelectedMVForgeLibrary(Number(event.target.value))}>
                {libraries.map((library) => <MenuItem key={library.id} value={library.id}>{library.name} · ID {library.id}</MenuItem>)}
              </TextField>
            </Grid>
            <Grid size={{ xs: 12, md: 5 }}>
              <TextField select fullWidth label="Discovered Jellyfin Library" value={selectedJellyfinLibrary} onChange={(event) => setSelectedJellyfinLibrary(event.target.value)}>
                {discoveredLibraries.map((library) => <MenuItem key={library.id} value={library.id}>{library.name} · ID {library.id}</MenuItem>)}
              </TextField>
            </Grid>
            <Grid size={{ xs: 12, md: 3 }}><Button fullWidth startIcon={<AddIcon />} disabled={!canAddMapping} onClick={addMapping}>Add mapping</Button></Grid>
          </Grid>
          <Stack spacing={0.5}>
            {draft.mappings.map((mapping) => {
              const local = libraries.find((library) => library.id === mapping.mvforgeLibraryId);
              const discovered = discoveredLibraries.find((library) => library.id === mapping.jellyfinLibraryId);
              return (
                <Stack key={`${mapping.mvforgeLibraryId}:${mapping.jellyfinLibraryId}`} direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                  <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                    {local?.name ?? `MVForge Library ${mapping.mvforgeLibraryId}`} (ID {mapping.mvforgeLibraryId}) → {discovered?.name ?? mapping.jellyfinName ?? 'Jellyfin Library'} (ID {mapping.jellyfinLibraryId})
                  </Typography>
                  <IconButton aria-label="Remove mapping" size="small" onClick={() => setDraft((current) => ({ ...current, mappings: current.mappings.filter((item) => item.mvforgeLibraryId !== mapping.mvforgeLibraryId) }))}><DeleteIcon fontSize="small" /></IconButton>
                </Stack>
              );
            })}
            {draft.mappings.length === 0 ? <Typography color="text.secondary" variant="body2">No mappings configured.</Typography> : null}
          </Stack>
          <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ sm: 'center' }}>
            <TextField select size="small" label="Manual refresh target" value={effectiveManualLibraryId} onChange={(event) => setManualLibraryId(Number(event.target.value))} sx={{ minWidth: { sm: 300 } }}>
              {draft.mappings.map((mapping) => <MenuItem key={mapping.mvforgeLibraryId} value={mapping.mvforgeLibraryId}>{libraries.find((library) => library.id === mapping.mvforgeLibraryId)?.name ?? `Library ${mapping.mvforgeLibraryId}`}</MenuItem>)}
            </TextField>
            <Button variant="outlined" disabled={!effectiveManualLibraryId || manualRefresh.isPending} onClick={() => manualRefresh.mutate(Number(effectiveManualLibraryId))}>Request manual refresh</Button>
            {latestOutcome ? <Typography variant="body2" color="text.secondary">Last outcome: {statusLabel(latestOutcome)} · {new Date(latestOutcome.updatedAt).toLocaleString()}</Typography> : <Typography variant="body2" color="text.secondary">No refresh outcomes yet.</Typography>}
          </Stack>
          {manualRefresh.isError ? <Alert severity="warning">Unable to record manual refresh request.</Alert> : null}
          {latestOutcome?.effectiveScope === 'server_wide' ? <Typography variant="caption" color="text.secondary">Effective scope: server-wide scan. Jellyfin’s item metadata refresh does not guarantee discovery of added and removed descendant media.</Typography> : null}
        </Stack>
      </CardContent>
    </Card>
  );
}
