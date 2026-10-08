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
import { useEffect, useState } from 'react';
import { api } from '../api/client';
import type { JellyfinIntegrationOutcome, JellyfinIntegrationState, JellyfinLibrary, Library } from '../api/types';

type Mapping = { mvforgeLibraryId: number; jellyfinLibraryId: string; jellyfinName?: string; jellyfinServerId?: string; needsRemap?: boolean; remap?: boolean };
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
  saveError?: string;
  onSave: (value: Record<string, unknown>) => void | Promise<unknown>;
};

function jellyfinDraft(value?: Record<string, unknown>): Draft {
  const mappings = Array.isArray(value?.mappings)
    ? value.mappings.flatMap((raw): Mapping[] => {
        if (typeof raw !== 'object' || raw === null) return [];
        const item = raw as Record<string, unknown>;
        return typeof item.mvforgeLibraryId === 'number' && typeof item.jellyfinLibraryId === 'string'
          ? [{ mvforgeLibraryId: item.mvforgeLibraryId, jellyfinLibraryId: item.jellyfinLibraryId, jellyfinName: typeof item.jellyfinName === 'string' ? item.jellyfinName : undefined, jellyfinServerId: typeof item.jellyfinServerId === 'string' ? item.jellyfinServerId : undefined, needsRemap: item.needsRemap === true }]
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

function persistedJellyfinLibraries(value: unknown): JellyfinLibrary[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap((raw): JellyfinLibrary[] => {
    if (typeof raw !== 'object' || raw === null) return [];
    const library = raw as Record<string, unknown>;
    if (typeof library.id !== 'string' || typeof library.name !== 'string') return [];
    return [{ id: library.id, name: library.name, ...(typeof library.collectionType === 'string' ? { collectionType: library.collectionType } : {}) }];
  });
}

function draftConfigurationSignature(draft: Draft) {
  const mappings = draft.mappings
    .map(({ mvforgeLibraryId, jellyfinLibraryId }) => ({ mvforgeLibraryId, jellyfinLibraryId }))
    .sort((left, right) => left.mvforgeLibraryId - right.mvforgeLibraryId || left.jellyfinLibraryId.localeCompare(right.jellyfinLibraryId));
  return JSON.stringify({
    enabled: draft.enabled,
    baseUrl: draft.baseUrl,
    credentialConfigured: draft.credentialConfigured,
    refreshAfterPublish: draft.refreshAfterPublish,
    refreshTestEncode: draft.refreshTestEncode,
    debounceSeconds: draft.debounceSeconds,
    mappings,
  });
}

function persistedConfigurationSignature(value?: Record<string, unknown>) {
  const revision = value?.connectionConfigRevision;
  return JSON.stringify({
    configuration: draftConfigurationSignature(jellyfinDraft(value)),
    connectionConfigRevision: typeof revision === 'number' || typeof revision === 'string' ? revision : null,
  });
}

function savedJellyfinValue(result: unknown): Record<string, unknown> | undefined {
  if (typeof result !== 'object' || result === null || !('value' in result)) return undefined;
  const value = (result as { value?: unknown }).value;
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? value as Record<string, unknown>
    : undefined;
}

function statusLabel(outcome: JellyfinIntegrationOutcome) {
  if (outcome.status === 'success') return outcome.effectiveScope === 'server_wide' ? 'Server scan requested' : 'Refresh accepted';
  if (outcome.status === 'retryable_failure' || outcome.status === 'permanent_configuration_error') return 'Failed';
  if (outcome.status === 'skipped_disabled') return 'Skipped · disabled';
  if (outcome.status === 'skipped_unmapped') return 'Skipped · no mapping';
  return 'Pending';
}

export function JellyfinSettingsCard({ value, libraries, saving, saveError, onSave }: Props) {
  const queryClient = useQueryClient();
  const [persistedBaseline, setPersistedBaseline] = useState(() => jellyfinDraft(value));
  const [baselineSignature, setBaselineSignature] = useState(() => persistedConfigurationSignature(value));
  const [draft, setDraft] = useState<Draft>(() => jellyfinDraft(value));
  const [credentialDraft, setCredentialDraft] = useState('');
  const [clearCredential, setClearCredential] = useState(false);
  const [persistedConflict, setPersistedConflict] = useState<Record<string, unknown> | null>(null);
  const [selectedMVForgeLibrary, setSelectedMVForgeLibrary] = useState<number | ''>('');
  const [selectedJellyfinLibrary, setSelectedJellyfinLibrary] = useState('');
  const [manualLibraryId, setManualLibraryId] = useState<number | ''>('');
  const outcomes = useQuery({ queryKey: ['jellyfinOutcomes'], queryFn: api.jellyfinOutcomes, refetchInterval: 5000 });
  const integrationState = useQuery<JellyfinIntegrationState>({
    queryKey: ['jellyfinIntegrationState', value?.connectionConfigRevision, value?.baseUrl],
    queryFn: api.jellyfinIntegrationState,
    refetchOnMount: 'always',
  });
  const connection = useMutation({
    mutationFn: api.testJellyfinConnection,
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ['jellyfinIntegrationState'] }),
  });
  const manualRefresh = useMutation({
    mutationFn: api.manualJellyfinRefresh,
    onSuccess: async () => queryClient.invalidateQueries({ queryKey: ['jellyfinOutcomes'] }),
  });
  const discoveredLibraries = integrationState.data?.libraries ?? persistedJellyfinLibraries(value?.libraries);
  const latestOutcome = outcomes.data?.[0];
  const librariesLoaded = integrationState.data?.librariesLoaded ?? value?.librariesLoaded === true;
  const configurationNeedsRevalidation = integrationState.data?.configurationNeedsRevalidation ?? value?.configurationNeedsRevalidation === true;
  const lastConnectionSuccessful = integrationState.data?.lastConnectionSuccessful ?? value?.lastConnectionSuccessful === true;
  const lastSuccessfulConnectionAt = integrationState.data?.lastSuccessfulConnectionAt ?? (typeof value?.lastSuccessfulConnectionAt === 'string' ? value.lastSuccessfulConnectionAt : undefined);
  const liveReachability = integrationState.data?.liveReachability ?? 'unknown';
  const configurationValid = integrationState.data?.configurationValid ?? (value?.configurationValid === true || (Boolean(persistedBaseline.baseUrl.trim()) && persistedBaseline.credentialConfigured));
  const canAddMapping = librariesLoaded && !configurationNeedsRevalidation && selectedMVForgeLibrary !== '' && selectedJellyfinLibrary !== '' && !draft.mappings.some((mapping) => mapping.jellyfinLibraryId === selectedJellyfinLibrary && mapping.mvforgeLibraryId !== selectedMVForgeLibrary);
  const settingsDirty = draftConfigurationSignature(draft) !== draftConfigurationSignature(persistedBaseline)
    || credentialDraft.length > 0
    || clearCredential;

  useEffect(() => {
    const timer = window.setTimeout(() => {
      const nextSignature = persistedConfigurationSignature(value);
      if (nextSignature === baselineSignature) {
        if (persistedConflict) setPersistedConflict(null);
        return;
      }
      if (saving) return;

      if (settingsDirty) {
        setPersistedConflict(value ?? {});
        return;
      }

      const nextBaseline = jellyfinDraft(value);
      setPersistedBaseline(nextBaseline);
      setDraft(nextBaseline);
      setBaselineSignature(nextSignature);
      setCredentialDraft('');
      setClearCredential(false);
      setPersistedConflict(null);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [baselineSignature, persistedConflict, saving, settingsDirty, value]);
  const canTestConnection = Boolean(persistedBaseline.baseUrl.trim())
    && persistedBaseline.credentialConfigured
    && !settingsDirty
    && !connection.isPending;

  const rebasePersistedValue = (persistedValue: Record<string, unknown>) => {
    const nextBaseline = jellyfinDraft(persistedValue);
    setPersistedBaseline(nextBaseline);
    setDraft(nextBaseline);
    setBaselineSignature(persistedConfigurationSignature(persistedValue));
    setCredentialDraft('');
    setClearCredential(false);
    setPersistedConflict(null);
  };

  const save = async () => {
    const submittedValue = {
      ...draft,
      mappings: draft.mappings.map(({ mvforgeLibraryId, jellyfinLibraryId, jellyfinName, remap }) => ({ mvforgeLibraryId, jellyfinLibraryId, jellyfinName, ...(remap ? { remap: true } : {}) })),
      ...(credentialDraft ? { apiToken: credentialDraft } : {}),
      ...(clearCredential ? { clearCredential: true } : {}),
    };
    try {
      const result = await onSave(submittedValue);
      const savedValue = savedJellyfinValue(result) ?? {
        ...submittedValue,
        credentialConfigured: clearCredential ? false : credentialDraft ? true : persistedBaseline.credentialConfigured,
      };
      rebasePersistedValue(savedValue);
    } catch {
      // SettingsPage exposes the mutation error and keeps this draft editable.
    }
  };

  const reloadPersistedSettings = () => {
    if (persistedConflict) rebasePersistedValue(persistedConflict);
  };

  const addMapping = () => {
    if (selectedMVForgeLibrary === '' || !selectedJellyfinLibrary) return;
    const selected = discoveredLibraries.find((library) => library.id === selectedJellyfinLibrary);
    const mapping = { mvforgeLibraryId: selectedMVForgeLibrary, jellyfinLibraryId: selectedJellyfinLibrary, jellyfinName: selected?.name, jellyfinServerId: integrationState.data?.serverId, needsRemap: false, remap: true };
    setDraft((current) => ({
      ...current,
      mappings: [...current.mappings.filter((item) => item.mvforgeLibraryId !== selectedMVForgeLibrary), mapping],
    }));
    setManualLibraryId(selectedMVForgeLibrary);
  };
  const effectiveManualLibraryId = draft.mappings.some((mapping) => mapping.mvforgeLibraryId === manualLibraryId)
    ? manualLibraryId
    : draft.mappings[0]?.mvforgeLibraryId ?? '';
  const manualMapping = draft.mappings.find((mapping) => mapping.mvforgeLibraryId === effectiveManualLibraryId);
  const persistedManualMapping = integrationState.data?.mappings.find((mapping) => mapping.mvforgeLibraryId === effectiveManualLibraryId);
  const manualMappingNeedsRemap = persistedManualMapping?.needsRemap ?? manualMapping?.needsRemap === true;

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
            <Button variant="outlined" disabled={!canTestConnection} onClick={() => connection.mutate()}>
              {connection.isPending ? 'Testing…' : 'Test connection and discover libraries'}
            </Button>
          </Stack>
          {saveError ? <Alert severity="error">Jellyfin settings could not be saved: {saveError}. The mapping is still an unsaved draft.</Alert> : null}
          {persistedConflict ? (
            <Alert
              severity="warning"
              action={<Button color="inherit" size="small" onClick={reloadPersistedSettings}>Reload persisted settings</Button>}
            >
              Jellyfin settings changed elsewhere. Your draft is preserved. Reload the persisted settings to replace it.
            </Alert>
          ) : null}
          {settingsDirty ? <Alert severity="info">Save Jellyfin settings before testing the connection or requesting a manual refresh.</Alert> : null}
          {connection.data ? (
            <Alert severity={connection.data.authenticated ? 'success' : 'warning'}>
              {connection.data.authenticated
                ? `Connected${connection.data.versionAvailable ? ` · Jellyfin ${connection.data.serverVersion}` : ' · version unavailable'} · ${discoveredLibraries.length} libraries discovered.`
                : `Connection unavailable (${connection.data.failureClass ?? 'unknown'}).`}
            </Alert>
          ) : null}
          {connection.isError ? <Alert severity="warning">Connection test failed.</Alert> : null}
          {configurationNeedsRevalidation ? <Alert severity="info">Jellyfin connection settings changed. Libraries and mappings need successful revalidation.</Alert> : null}
          <Alert severity={liveReachability === 'unavailable' ? 'warning' : lastConnectionSuccessful ? 'success' : 'info'}>
            {`Integration ${draft.enabled ? 'enabled' : 'disabled'} · Credential ${integrationState.data?.credentialConfigured ?? persistedBaseline.credentialConfigured ? 'configured' : 'missing'} · Configuration ${configurationValid ? 'valid' : 'incomplete'} · `}
            {lastConnectionSuccessful
              ? `Last connection succeeded${lastSuccessfulConnectionAt ? ` ${new Date(lastSuccessfulConnectionAt).toLocaleString()}` : ''}`
              : 'No successful connection recorded'}
            {` · Live reachability: ${liveReachability} · Libraries loaded: ${librariesLoaded ? 'yes' : 'no'}`}
          </Alert>
          {integrationState.isError ? <Alert severity="warning">Unable to refresh Jellyfin state; showing saved discovery data.</Alert> : null}
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
            <Grid size={{ xs: 12, md: 3 }}><Button fullWidth startIcon={<AddIcon />} disabled={!canAddMapping} onClick={addMapping}>{draft.mappings.some((mapping) => mapping.mvforgeLibraryId === selectedMVForgeLibrary) ? 'Add mapping / remap' : 'Add mapping'}</Button></Grid>
          </Grid>
          <Stack spacing={0.5}>
            {draft.mappings.map((mapping) => {
              const local = libraries.find((library) => library.id === mapping.mvforgeLibraryId);
              const discovered = discoveredLibraries.find((library) => library.id === mapping.jellyfinLibraryId);
              const persistedMapping = integrationState.data?.mappings.find((item) => item.mvforgeLibraryId === mapping.mvforgeLibraryId);
              const needsRemap = persistedMapping?.needsRemap ?? mapping.needsRemap === true;
              return (
                <Stack key={`${mapping.mvforgeLibraryId}:${mapping.jellyfinLibraryId}`} direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
                  <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>
                    {local?.name ?? `MVForge Library ${mapping.mvforgeLibraryId}`} (ID {mapping.mvforgeLibraryId}) → {needsRemap ? 'Jellyfin Library unavailable' : discovered?.name ?? mapping.jellyfinName ?? 'Jellyfin Library'} (ID {mapping.jellyfinLibraryId}){needsRemap ? ' · Needs remap' : ''}
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
            <Button variant="outlined" disabled={!effectiveManualLibraryId || settingsDirty || manualRefresh.isPending || manualMappingNeedsRemap} onClick={() => manualRefresh.mutate(Number(effectiveManualLibraryId))}>Request manual refresh</Button>
            {latestOutcome ? <Typography variant="body2" color="text.secondary">Last outcome: {statusLabel(latestOutcome)} · {new Date(latestOutcome.updatedAt).toLocaleString()}</Typography> : <Typography variant="body2" color="text.secondary">No refresh outcomes yet.</Typography>}
          </Stack>
          {manualRefresh.isError ? <Alert severity="warning">Unable to record manual refresh request.</Alert> : null}
          {latestOutcome?.effectiveScope === 'server_wide' ? <Typography variant="caption" color="text.secondary">Effective scope: server-wide scan. Jellyfin’s item metadata refresh does not guarantee discovery of added and removed descendant media.</Typography> : null}
        </Stack>
      </CardContent>
    </Card>
  );
}
