import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Dialog,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  Grid,
  MenuItem,
  Switch,
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
import EditIcon from '@mui/icons-material/Edit';
import SaveIcon from '@mui/icons-material/Save';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useState } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import type { Library, LibraryInput, PathEntry, Profile } from '../api/types';

const initialLibrary: LibraryInput = {
  name: '',
  sourcePath: '',
  destinationPath: '',
  type: 'movies',
  validationRules: {
    episodeNamingEnabled: false,
    extrasPathEnabled: false,
    extrasPathName: 'extras',
  },
};

export function LibrariesPage() {
  const queryClient = useQueryClient();
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings });
  const libraryPaths = useQuery({ queryKey: ['paths', 'library'], queryFn: () => api.browsePaths('library') });
  const assetTypes = getAssetTypes(settings.data);
  const rawRoot = getRawRoot(settings.data);
  const [form, setForm] = useState<LibraryInput>(initialLibrary);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [showForm, setShowForm] = useState(false);
  const [useCustomPaths, setUseCustomPaths] = useState(false);

  const createLibrary = useMutation({
    mutationFn: api.createLibrary,
    onSuccess: async () => {
      setForm(initialLibrary);
      setShowForm(false);
      await queryClient.invalidateQueries({ queryKey: ['libraries'] });
    },
  });

  const updateLibrary = useMutation({
    mutationFn: api.updateLibrary,
    onSuccess: async () => {
      setForm(initialLibrary);
      setEditingId(null);
      setShowForm(false);
      await queryClient.invalidateQueries({ queryKey: ['libraries'] });
      await queryClient.invalidateQueries({ queryKey: ['assets'] });
    },
  });

  function updateField<K extends keyof LibraryInput>(field: K, value: LibraryInput[K]) {
    setForm((current) => ({ ...current, [field]: value }));
  }

  function updateValidationRule(key: string, value: unknown) {
    setForm((current) => ({
      ...current,
      validationRules: {
        ...(current.validationRules ?? {}),
        [key]: value,
      },
    }));
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const payload = {
      ...form,
      sourcePath: rawRoot,
      defaultProfileId: form.defaultProfileId || undefined,
    };

    if (editingId) {
      updateLibrary.mutate({ id: editingId, ...payload });
      return;
    }

    createLibrary.mutate(payload);
  }

  function addLibrary() {
    setEditingId(null);
    setForm({ ...initialLibrary, sourcePath: rawRoot });
    setUseCustomPaths(false);
    setShowForm(true);
  }

  function editLibrary(library: Library) {
    setEditingId(library.id);
    setShowForm(true);
    setUseCustomPaths(false);
    setForm({
      name: library.name,
      sourcePath: rawRoot,
      destinationPath: library.destinationPath,
      type: library.type,
      validationRules: normalizeLibraryRules(library.validationRules),
      defaultProfileId: library.defaultProfileId,
    });
  }

  function cancelEdit() {
    setEditingId(null);
    setForm(initialLibrary);
    setShowForm(false);
  }

  return (
    <>
      <PageHeader title="Libraries" eyebrow="Destinations">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 760 }}>
          Libraries define destination paths where converted and validated media will be published.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        {libraries.isError ? <Alert severity="warning">Unable to load libraries.</Alert> : null}
        <Stack direction="row" justifyContent="flex-end" sx={{ mb: 2 }}>
          <Button startIcon={<AddIcon />} variant="contained" onClick={addLibrary}>
            Add Library
          </Button>
        </Stack>
        <Dialog open={showForm} onClose={cancelEdit} maxWidth="md" fullWidth>
          <DialogTitle>{editingId ? 'Edit Library' : 'New Library'}</DialogTitle>
          <DialogContent>
            <Box component="form" onSubmit={submit}>
              <Stack spacing={2.5}>
                <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1}>
                  <Stack>
                    <Typography color="text.secondary" variant="body2">
                      Select the destination folder for this library. Originals are read from the global source root.
                    </Typography>
                  </Stack>
                  <Button startIcon={<CloseIcon />} onClick={cancelEdit}>
                    Close
                  </Button>
                </Stack>
                <Grid container spacing={2}>
                  <Grid size={{ xs: 12, md: 4 }}>
                    <TextField
                      label="Name"
                      value={form.name}
                      onChange={(event) => updateField('name', event.target.value)}
                      required
                      fullWidth
                    />
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <TextField
                      label="Type"
                      value={form.type}
                      onChange={(event) => updateField('type', event.target.value)}
                      select
                      fullWidth
                    >
                      {assetTypes.map((type) => (
                        <MenuItem key={type.key} value={type.key}>
                          {type.label}
                        </MenuItem>
                      ))}
                    </TextField>
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                    <ProfileAutocomplete
                      profiles={profiles.data ?? []}
                      value={form.defaultProfileId}
                      onChange={(profileId) => updateField('defaultProfileId', profileId || undefined)}
                    />
                  </Grid>
                  <Grid size={{ xs: 12, md: 8 }}>
                    {useCustomPaths ? (
                      <TextField
                        label="Destination path"
                        value={form.destinationPath}
                        onChange={(event) => updateField('destinationPath', event.target.value)}
                        placeholder="/media/library/movies"
                        required
                        fullWidth
                      />
                    ) : (
                      <PathSelect
                        label="Destination folder"
                        value={form.destinationPath}
                        rootLabel={libraryPaths.data?.root ?? '/media/library'}
                        paths={libraryPaths.data?.paths ?? []}
                        isLoading={libraryPaths.isLoading}
                        onRefresh={() => {
                          void libraryPaths.refetch();
                        }}
                        onChange={(path) => updateField('destinationPath', path)}
                      />
                    )}
                  </Grid>
                  <Grid size={{ xs: 12 }}>
                    <Stack spacing={1.5}>
                      <FormControlLabel
                        control={
                          <Switch
                            checked={libraryRuleEnabled(form.validationRules, 'episodeNamingEnabled')}
                            onChange={(event) => updateValidationRule('episodeNamingEnabled', event.target.checked)}
                          />
                        }
                        label="Rename folder batches as episodes"
                      />
                      <FormControlLabel
                        control={
                          <Switch
                            checked={libraryRuleEnabled(form.validationRules, 'extrasPathEnabled')}
                            onChange={(event) => updateValidationRule('extrasPathEnabled', event.target.checked)}
                          />
                        }
                        label="Route assets tagged as extras into an extras folder"
                      />
                      {libraryRuleEnabled(form.validationRules, 'extrasPathEnabled') ? (
                        <TextField
                          label="Extras folder name"
                          value={libraryRuleString(form.validationRules, 'extrasPathName', 'extras')}
                          onChange={(event) => updateValidationRule('extrasPathName', event.target.value)}
                          placeholder="extras"
                          fullWidth
                        />
                      ) : null}
                    </Stack>
                  </Grid>
                  <Grid size={{ xs: 12 }}>
                    <Alert severity="info" sx={{ mb: 1 }}>
                      Source originals root: {rawRoot}. Configure this once in Settings instead of per library.
                    </Alert>
                    <FormControlLabel
                      control={
                        <Switch checked={useCustomPaths} onChange={(event) => setUseCustomPaths(event.target.checked)} />
                      }
                        label="Advanced: type custom destination path"
                    />
                    <Alert severity="info" sx={{ mt: 1 }}>
                      MediaForge only lists destination folders that exist inside the mounted library root. Create folders on the host
                      under media/library before selecting them here.
                    </Alert>
                  </Grid>
                  <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                    <Button
                      type="submit"
                      startIcon={editingId ? <SaveIcon /> : <AddIcon />}
                      variant="contained"
                      disabled={createLibrary.isPending || updateLibrary.isPending}
                      fullWidth
                    >
                      {editingId ? 'Save Library' : 'Create Library'}
                    </Button>
                  </Grid>
                </Grid>
              </Stack>
            </Box>
            {createLibrary.isError ? (
              <Alert severity="warning" sx={{ mt: 2 }}>
                Library could not be created.
              </Alert>
            ) : null}
            {updateLibrary.isError ? (
              <Alert severity="warning" sx={{ mt: 2 }}>
                Library could not be updated.
              </Alert>
            ) : null}
          </DialogContent>
        </Dialog>
        <Card>
          <CardContent sx={{ p: 0, '&:last-child': { pb: 0 } }}>
            <Table sx={{ tableLayout: 'fixed' }}>
              <TableHead>
                <TableRow>
                  <TableCell>Library</TableCell>
                  <TableCell>Destination</TableCell>
                  <TableCell>Default profile</TableCell>
                  <TableCell align="right">Actions</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {(libraries.data ?? []).map((library) => (
                  <TableRow key={library.id} hover>
                    <TableCell>
                      <Stack spacing={0.5}>
                        <Typography fontWeight={700}>{library.name}</Typography>
                        <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                          <Chip label={labelForAssetType(library.type, assetTypes)} size="small" />
                          {libraryRuleEnabled(library.validationRules, 'episodeNamingEnabled') ? <Chip label="Episode rename" size="small" color="primary" variant="outlined" /> : null}
                          {libraryRuleEnabled(library.validationRules, 'extrasPathEnabled') ? <Chip label={`${libraryRuleString(library.validationRules, 'extrasPathName', 'extras')} folder`} size="small" color="secondary" variant="outlined" /> : null}
                        </Stack>
                      </Stack>
                    </TableCell>
                    <TableCell sx={{ wordBreak: 'break-all' }}>{library.destinationPath}</TableCell>
                    <TableCell>{profileName(library.defaultProfileId, profiles.data ?? [])}</TableCell>
                    <TableCell align="right">
                      <Button startIcon={<EditIcon />} variant="outlined" onClick={() => editLibrary(library)}>
                        Edit
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
        {!libraries.isLoading && libraries.data?.length === 0 ? (
          <Alert severity="info" sx={{ mt: 2 }}>
            No libraries have been configured yet.
          </Alert>
        ) : null}
      </Box>
    </>
  );
}

function labelForAssetType(type: string, assetTypes: Array<{ key: string; label: string }>) {
  return assetTypes.find((assetType) => assetType.key === type)?.label ?? type;
}

function normalizeLibraryRules(rules?: Record<string, unknown>) {
  return {
    ...(rules ?? {}),
    episodeNamingEnabled: libraryRuleEnabled(rules, 'episodeNamingEnabled'),
    extrasPathEnabled: libraryRuleEnabled(rules, 'extrasPathEnabled'),
    extrasPathName: libraryRuleString(rules, 'extrasPathName', 'extras'),
  };
}

function libraryRuleEnabled(rules: Record<string, unknown> | undefined, key: string) {
  return rules?.[key] === true;
}

function libraryRuleString(rules: Record<string, unknown> | undefined, key: string, fallback: string) {
  const value = rules?.[key];
  return typeof value === 'string' && value.trim() ? value : fallback;
}

function getRawRoot(settings?: Array<{ key: string; value: Record<string, unknown> }>) {
  const paths = settings?.find((setting) => setting.key === 'paths')?.value;
  return typeof paths?.rawRoot === 'string' && paths.rawRoot.trim() ? paths.rawRoot : '/media/raw';
}

function PathSelect({
  label,
  value,
  rootLabel,
  paths,
  isLoading,
  onRefresh,
  onChange,
}: {
  label: string;
  value: string;
  rootLabel: string;
  paths: PathEntry[];
  isLoading: boolean;
  onRefresh: () => void;
  onChange: (path: string) => void;
}) {
  const options = paths.some((entry) => entry.path === value) || !value
    ? paths
    : [{ path: value, relativePath: value, name: value }, ...paths];

  return (
    <Stack spacing={1}>
      <TextField
        label={label}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        select
        required
        fullWidth
        helperText={`Mounted root: ${rootLabel}`}
      >
        {options.map((entry) => (
          <MenuItem key={entry.path} value={entry.path}>
            {entry.relativePath ? entry.relativePath : `${entry.name} root`}
          </MenuItem>
        ))}
      </TextField>
      <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1}>
        <Typography color="text.secondary" variant="body2">
          {isLoading ? 'Loading folders...' : `${paths.length} folders available`}
        </Typography>
        <Button variant="outlined" onClick={onRefresh} size="small">
          Refresh
        </Button>
      </Stack>
      {!isLoading && paths.length === 0 ? (
        <Alert severity="warning">No folders found in this mounted root.</Alert>
      ) : null}
    </Stack>
  );
}

function profileName(profileId: number | undefined, profiles: Profile[]) {
  if (!profileId) {
    return 'None';
  }

  return profiles.find((profile) => profile.id === profileId)?.name ?? `Profile ${profileId}`;
}

function ProfileAutocomplete({
  profiles,
  value,
  onChange,
}: {
  profiles: Profile[];
  value?: number;
  onChange: (id: number) => void;
}) {
  return (
    <Autocomplete
      options={profiles}
      value={profiles.find((profile) => profile.id === value) ?? null}
      onChange={(_, profile) => onChange(profile?.id ?? 0)}
      getOptionLabel={(profile) => `${profile.name} · ${profile.videoCodec}`}
      isOptionEqualToValue={(option, selected) => option.id === selected.id}
      filterOptions={(options, state) => {
        const query = state.inputValue.trim().toLowerCase();
        if (!query) {
          return options.slice(0, 50);
        }
        return options
          .filter((profile) =>
            [profile.name, profile.description, profile.container, profile.videoCodec, profile.audioCodec].some((value) =>
              value.toLowerCase().includes(query),
            ),
          )
          .slice(0, 50);
      }}
      renderInput={(params) => <TextField {...params} label="Default profile" placeholder="None" />}
      fullWidth
    />
  );
}

function getAssetTypes(settings?: Array<{ key: string; value: Record<string, unknown> }>) {
  const assetTypes = settings?.find((setting) => setting.key === 'assetTypes')?.value.types;
  if (!Array.isArray(assetTypes)) {
    return [
      { key: 'movies', label: 'Movies' },
      { key: 'tv', label: 'TV Shows' },
      { key: 'anime', label: 'Anime' },
      { key: 'music-videos', label: 'Music Videos' },
      { key: 'concerts', label: 'Concerts' },
      { key: 'home-videos', label: 'Home Videos' },
    ];
  }

  return assetTypes
    .map((assetType) => {
      if (!assetType || typeof assetType !== 'object') {
        return null;
      }

      const candidate = assetType as Record<string, unknown>;
      if (typeof candidate.key !== 'string' || typeof candidate.label !== 'string') {
        return null;
      }

      return { key: candidate.key, label: candidate.label };
    })
    .filter((assetType): assetType is { key: string; label: string } => Boolean(assetType));
}
