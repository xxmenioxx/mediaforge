import { Alert, Card, CardContent, Divider, Stack, TextField, Typography } from '@mui/material';
import type { Dispatch, SetStateAction } from 'react';
import type { SettingsForm } from '../SettingsPage';

export function AdvancedSettings({
  form,
  setForm,
  error,
}: {
  form: SettingsForm;
  setForm: Dispatch<SetStateAction<SettingsForm>>;
  error?: string;
}) {
  return (
    <Card>
      <CardContent>
        <Stack spacing={2}>
          <Stack>
            <Typography variant="h3">Raw configuration JSON</Typography>
            <Typography color="text.secondary" variant="body2">
              This expert editor owns paths, workers, pipeline automation, inventory, cancellation, archive retention, asset types, and categories. Saving it replaces the matching values shown in those standard sections. Each field must contain a valid JSON object; use the normal sections for everyday edits.
            </Typography>
          </Stack>
          <Divider />
          {error ? <Alert severity="error">{error}</Alert> : null}
          {(['paths', 'workers', 'pipelineAutomation', 'assetInventory', 'cancellationPolicy', 'originalRetentionPolicy', 'assetTypes', 'assetCategories'] as const).map((key) => (
            <TextField
              key={key}
              label={key}
              value={form.advancedJson[key]}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  advancedJson: { ...current.advancedJson, [key]: event.target.value },
                }))
              }
              multiline
              minRows={key === 'assetTypes' ? 10 : 5}
              fullWidth
            />
          ))}
          <Typography color="warning.main" variant="body2">
            Advanced values must be JSON objects. A successful save updates the corresponding standard settings together in one atomic batch.
          </Typography>
        </Stack>
      </CardContent>
    </Card>
  );
}
