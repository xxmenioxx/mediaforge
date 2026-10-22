import { Alert, Card, CardContent, Divider, FormControlLabel, Grid, Stack, Switch, TextField, Typography } from '@mui/material';
import type { Dispatch, SetStateAction } from 'react';
import type { SettingsForm } from '../SettingsPage';

type ArchiveSettingsProps = {
  form: SettingsForm;
  setForm: Dispatch<SetStateAction<SettingsForm>>;
};

export function ArchiveSettingsDomain(props: ArchiveSettingsProps) {
  return <OriginalArchiveCard {...props} />;
}

function OriginalArchiveCard({ form, setForm }: ArchiveSettingsProps) {
  const automaticDeletion = form.originalRetentionPolicy.autoDeleteEnabled;

  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Archive & Retention</Typography>
            <Typography color="text.secondary" variant="body2">
              Choose the archive location and retention period. These controls do not change general storage roles or Library destinations.
            </Typography>
          </Stack>
          <Grid container spacing={2}>
            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="Original Archive root"
                value={form.paths.originalsArchivePath}
                onChange={(event) => setForm((current) => ({
                  ...current,
                  paths: { ...current.paths, originalsArchivePath: event.target.value },
                }))}
                helperText="Controlled root. MVForge will never apply this policy outside this path."
                fullWidth
              />
            </Grid>
            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="Processed originals path"
                value={form.originalRetentionPolicy.processedOriginalsPath}
                onChange={(event) => setForm((current) => ({
                  ...current,
                  originalRetentionPolicy: {
                    ...current.originalRetentionPolicy,
                    processedOriginalsPath: event.target.value,
                  },
                }))}
                helperText="Destination for originals archived after a successful publication."
                fullWidth
              />
            </Grid>
            <Grid size={{ xs: 12, md: 6 }}>
              <TextField
                label="Keep originals for days"
                type="number"
                value={form.originalRetentionPolicy.keepOriginalsDays}
                onChange={(event) => {
                  const days = Math.max(0, Number(event.target.value));
                  setForm((current) => ({
                    ...current,
                    originalRetentionPolicy: {
                      ...current.originalRetentionPolicy,
                      keepOriginalsDays: days,
                      autoDeleteEnabled: days === 0 ? false : current.originalRetentionPolicy.autoDeleteEnabled,
                    },
                  }));
                }}
                helperText="0 keeps originals indefinitely and disables automatic deletion."
                inputProps={{ min: 0, max: 3650 }}
                fullWidth
              />
            </Grid>
          </Grid>
          <Divider />
          <FormControlLabel
            control={(
              <Switch
                checked={automaticDeletion}
                disabled={form.originalRetentionPolicy.keepOriginalsDays === 0}
                onChange={(event) => setForm((current) => ({
                  ...current,
                  originalRetentionPolicy: {
                    ...current.originalRetentionPolicy,
                    autoDeleteEnabled: event.target.checked,
                  },
                }))}
              />
            )}
            label="Automatically delete expired originals during inventory sync"
          />
          <Alert severity={automaticDeletion ? 'warning' : 'info'}>
            {automaticDeletion
              ? 'Physical deletion is enabled. MVForge only deletes an expired original when it is linked to an active completed job, validation passed or warned, and the published replacement still exists. Logs, reports, snapshots, and untracked archive files are preserved.'
              : 'Physical deletion is disabled. Expiration dates remain visible for tracked originals, but sync only reconciles inventory and does not remove media.'}
          </Alert>
        </Stack>
      </CardContent>
    </Card>
  );
}
