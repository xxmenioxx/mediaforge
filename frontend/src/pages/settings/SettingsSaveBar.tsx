import { Box, Button, Paper, Stack, Typography } from '@mui/material';
import SaveIcon from '@mui/icons-material/Save';

export type SettingsSaveState = 'saved' | 'dirty' | 'saving' | 'failed';

export function SettingsSaveBar({
  state,
  canSave = true,
  onDiscard,
  onSave,
}: {
  state: SettingsSaveState;
  canSave?: boolean;
  onDiscard: () => void;
  onSave: () => void;
}) {
  const labels: Record<SettingsSaveState, string> = {
    saved: 'Saved',
    dirty: 'Unsaved changes',
    saving: 'Saving…',
    failed: 'Save failed — Retry',
  };

  return (
    <Paper
      component="section"
      aria-label="Settings save actions"
      elevation={8}
      sx={{
        position: 'sticky',
        bottom: 0,
        zIndex: 2,
        mt: 2,
        p: 1.5,
        border: 1,
        borderColor: 'divider',
        bgcolor: 'background.paper',
      }}
    >
      <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ xs: 'stretch', sm: 'center' }} justifyContent="space-between" spacing={1}>
        <Typography role="status" aria-live="polite" fontWeight={600}>
          {labels[state]}
        </Typography>
        <Box display="flex" gap={1} justifyContent="flex-end">
          <Button onClick={onDiscard} disabled={state === 'saved' || state === 'saving'}>
            Discard changes
          </Button>
          <Button
            startIcon={<SaveIcon />}
            variant="contained"
            onClick={onSave}
            disabled={!canSave || state === 'saved' || state === 'saving'}
          >
            {state === 'failed' ? 'Retry save' : 'Save changes'}
          </Button>
        </Box>
      </Stack>
    </Paper>
  );
}
