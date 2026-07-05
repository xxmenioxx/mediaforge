import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  Divider,
  Grid,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import SearchIcon from '@mui/icons-material/Search';
import PlaylistAddIcon from '@mui/icons-material/PlaylistAdd';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { FormEvent, useState } from 'react';
import { api } from '../api/client';
import { PageHeader } from '../components/PageHeader';
import { MediaSnapshotDetails } from '../components/MediaSnapshotDetails';
import type { Asset, Library, Profile, QueueJobInput } from '../api/types';

export function ScannerPage() {
  const queryClient = useQueryClient();
  const [path, setPath] = useState('');
  const [jobDraft, setJobDraft] = useState<Pick<QueueJobInput, 'libraryId' | 'profileId' | 'priority'>>({
    libraryId: 0,
    profileId: 0,
    priority: 5,
  });
  const scan = useMutation({ mutationFn: api.scan });
  const libraries = useQuery({ queryKey: ['libraries'], queryFn: api.libraries });
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles });
  const assets = useQuery({ queryKey: ['assets'], queryFn: api.assets });
  const createJob = useMutation({
    mutationFn: api.createQueueJob,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['queueJobs'] });
    },
  });

  function submitJob(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!scan.data) {
      return;
    }

    createJob.mutate({
      mediaPath: scan.data.path,
      libraryId: jobDraft.libraryId,
      profileId: jobDraft.profileId,
      priority: jobDraft.priority,
      notes: `Created from scan ${scan.data.id}`,
    });
  }

  return (
    <>
      <PageHeader title="Scanner" eyebrow="Discovery only">
        <Typography color="text.secondary" sx={{ mt: 1, maxWidth: 760 }}>
          The scanner discovers media metadata and never starts conversions automatically.
        </Typography>
      </PageHeader>
      <Box sx={{ px: { xs: 2, md: 4 }, pb: 4 }}>
        <Card sx={{ mb: 2 }}>
          <CardContent>
            <Stack spacing={2}>
              <AssetAutocomplete assets={assets.data?.unprocessed ?? []} value={path} onChange={setPath} />
              <Button
                startIcon={<SearchIcon />}
                variant="contained"
                disabled={!path || scan.isPending}
                onClick={() => scan.mutate({ path })}
                sx={{ alignSelf: 'flex-start' }}
              >
                Scan
              </Button>
              {scan.isError ? <Alert severity="warning">Scan request failed.</Alert> : null}
            </Stack>
          </CardContent>
        </Card>
        {scan.data ? (
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, lg: 7 }}>
              <Card>
                <CardContent>
                  <Stack spacing={2}>
                    <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={2}>
                      <Stack spacing={0.5}>
                        <Typography variant="h2">{scan.data.fileName}</Typography>
                        <Typography color="text.secondary">{scan.data.path}</Typography>
                      </Stack>
                      <Chip label={scan.data.hdr ? 'HDR' : 'SDR'} color={scan.data.hdr ? 'warning' : 'default'} />
                    </Stack>
                    <Divider />
                    <MediaSnapshotDetails scan={scan.data} />
                  </Stack>
                </CardContent>
              </Card>
            </Grid>
            <Grid size={{ xs: 12, lg: 5 }}>
              <Card>
                <CardContent>
                  <Box component="form" onSubmit={submitJob}>
                    <Stack spacing={2}>
                      <Typography variant="h2">Queue Conversion</Typography>
                      <LibraryAutocomplete
                        libraries={libraries.data ?? []}
                        value={jobDraft.libraryId}
                        onChange={(libraryId) => setJobDraft((current) => ({ ...current, libraryId }))}
                      />
                      <ProfileAutocomplete
                        profiles={profiles.data ?? []}
                        value={jobDraft.profileId}
                        onChange={(profileId) => setJobDraft((current) => ({ ...current, profileId }))}
                      />
                      <TextField
                        label="Priority"
                        value={jobDraft.priority}
                        onChange={(event) =>
                          setJobDraft((current) => ({ ...current, priority: Number(event.target.value) }))
                        }
                        type="number"
                        inputProps={{ min: 1, max: 10 }}
                        fullWidth
                      />
                      <Button
                        type="submit"
                        startIcon={<PlaylistAddIcon />}
                        variant="contained"
                        disabled={createJob.isPending || !jobDraft.libraryId || !jobDraft.profileId}
                      >
                        Queue Conversion
                      </Button>
                      {createJob.isSuccess ? <Alert severity="success">Job queued for manual processing.</Alert> : null}
                      {createJob.isError ? <Alert severity="warning">Could not queue conversion.</Alert> : null}
                    </Stack>
                  </Box>
                </CardContent>
              </Card>
            </Grid>
          </Grid>
        ) : null}
      </Box>
    </>
  );
}

function AssetAutocomplete({ assets, value, onChange }: { assets: Asset[]; value: string; onChange: (path: string) => void }) {
  const selected = assets.find((asset) => asset.path === value) ?? null;

  return (
    <Autocomplete
      freeSolo
      options={assets}
      value={selected}
      inputValue={value}
      onInputChange={(_, nextValue) => onChange(nextValue)}
      onChange={(_, asset) => onChange(typeof asset === 'string' ? asset : asset?.path ?? '')}
      getOptionLabel={(asset) => (typeof asset === 'string' ? asset : asset.relativePath || asset.path)}
      filterOptions={(options, state) => filterByText(options, state.inputValue, (asset) => [asset.fileName, asset.relativePath, asset.path, asset.extension])}
      renderInput={(params) => <TextField {...params} label="Media file path" placeholder="/media/raw/movie.mkv" />}
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
  );
}

function LibraryAutocomplete({ libraries, value, onChange }: { libraries: Library[]; value: number; onChange: (id: number) => void }) {
  return (
    <Autocomplete
      options={libraries}
      value={libraries.find((library) => library.id === value) ?? null}
      onChange={(_, library) => onChange(library?.id ?? 0)}
      getOptionLabel={(library) => library.name}
      isOptionEqualToValue={(option, selected) => option.id === selected.id}
      filterOptions={(options, state) => filterByText(options, state.inputValue, (library) => [library.name, library.type, library.destinationPath])}
      renderInput={(params) => <TextField {...params} label="Library" required />}
      fullWidth
    />
  );
}

function ProfileAutocomplete({ profiles, value, onChange }: { profiles: Profile[]; value: number; onChange: (id: number) => void }) {
  return (
    <Autocomplete
      options={profiles}
      value={profiles.find((profile) => profile.id === value) ?? null}
      onChange={(_, profile) => onChange(profile?.id ?? 0)}
      getOptionLabel={(profile) => `${profile.name} · ${profile.videoCodec}`}
      isOptionEqualToValue={(option, selected) => option.id === selected.id}
      filterOptions={(options, state) =>
        filterByText(options, state.inputValue, (profile) => [
          profile.name,
          profile.description,
          profile.container,
          profile.videoCodec,
          profile.audioCodec,
        ])
      }
      renderInput={(params) => <TextField {...params} label="Profile" required />}
      fullWidth
    />
  );
}

function filterByText<T>(items: T[], inputValue: string, getValues: (item: T) => string[]) {
  const query = inputValue.trim().toLowerCase();
  if (!query) {
    return items.slice(0, 50);
  }

  return items
    .filter((item) => getValues(item).some((value) => value.toLowerCase().includes(query)))
    .slice(0, 50);
}
