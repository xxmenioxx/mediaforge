import Alert from '@mui/material/Alert';
import CircularProgress from '@mui/material/CircularProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import type { ReactNode } from 'react';

export type WorkspaceFeedbackKind = 'loading' | 'empty' | 'error';

export function WorkspaceFeedback({
  kind,
  children,
  action,
}: {
  kind: WorkspaceFeedbackKind;
  children: ReactNode;
  action?: ReactNode;
}) {
  if (kind === 'error') {
    return (
      <Alert severity="error" variant="outlined" action={action}>
        {children}
      </Alert>
    );
  }

  const content = (
    <>
      {kind === 'loading' ? <CircularProgress size={16} aria-label="Loading" /> : null}
      <Typography component="div" variant="body2" color="text.secondary">
        {children}
      </Typography>
      {action}
    </>
  );

  if (kind === 'loading') {
    return (
      <Stack
        role="status"
        aria-live="polite"
        direction="row"
        alignItems="center"
        gap={1}
        sx={{ color: 'text.secondary', px: 1.5, py: 1 }}
      >
        {content}
      </Stack>
    );
  }

  return (
    <Paper
      role="status"
      variant="outlined"
      sx={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 1,
        px: 1.5,
        py: 1,
      }}
    >
      {content}
    </Paper>
  );
}
