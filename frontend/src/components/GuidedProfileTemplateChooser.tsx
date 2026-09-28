import { Alert, Button, Grid, Stack, Typography } from '@mui/material';
import type { ProfileTemplate } from '../api/types';

type GuidedProfileTemplateChooserProps = {
  templates: ProfileTemplate[];
  loading: boolean;
  error?: string;
  cloneError?: string;
  creating: boolean;
  onCreateTemplate: (template: ProfileTemplate) => void;
  onCreateBlank: () => void;
};

export function GuidedProfileTemplateChooser({
  templates,
  loading,
  error,
  cloneError,
  creating,
  onCreateTemplate,
  onCreateBlank,
}: GuidedProfileTemplateChooserProps) {
  return (
    <Stack spacing={2}>
      <Stack>
        <Typography variant="h3">Guided Setup</Typography>
        <Typography color="text.secondary" variant="body2">
          Create an editable profile from a built-in template, or start with a blank profile.
        </Typography>
      </Stack>
      {loading ? <Alert severity="info">Loading profile templates…</Alert> : null}
      {error ? <Alert severity="error">Could not load profile templates: {error}</Alert> : null}
      {cloneError ? <Alert severity="error">Could not create profile: {cloneError}</Alert> : null}
      <Grid container spacing={1.5}>
        {templates.map((template) => (
          <Grid key={`${template.key}:${template.version}`} size={{ xs: 12, sm: 6, lg: 3 }}>
            <Button
              variant="outlined"
              onClick={() => onCreateTemplate(template)}
              title={template.description}
              disabled={creating}
              fullWidth
              sx={{ minHeight: 64, justifyContent: 'flex-start', textAlign: 'left' }}
            >
              <Stack>
                <Typography variant="body2" fontWeight={600}>{template.name}</Typography>
                <Typography variant="caption" color="text.secondary">{template.category}</Typography>
              </Stack>
            </Button>
          </Grid>
        ))}
        <Grid size={{ xs: 12, sm: 6, lg: 3 }}>
          <Button
            variant="outlined"
            onClick={onCreateBlank}
            fullWidth
            sx={{ minHeight: 64, justifyContent: 'flex-start', textAlign: 'left' }}
          >
            <Stack>
              <Typography variant="body2" fontWeight={600}>Blank Profile</Typography>
              <Typography variant="caption" color="text.secondary">Start with editable defaults.</Typography>
            </Stack>
          </Button>
        </Grid>
      </Grid>
    </Stack>
  );
}
