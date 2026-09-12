import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Stack,
  Typography,
} from '@mui/material';
import { useMutation, useQuery } from '@tanstack/react-query';
import { api } from '../api/client';

type LibraryPathRenameDialogProps = {
  open: boolean;
  libraryId: number;
  path: string;
  onClose: () => void;
  onApplied: () => void | Promise<void>;
};

function displayError(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export function LibraryPathRenameDialog({ open, libraryId, path, onClose, onApplied }: LibraryPathRenameDialogProps) {
  const preview = useQuery({
    queryKey: ['libraryPathRenamePreview', libraryId, path],
    queryFn: () => api.previewLibraryPathRename({ libraryId, path }),
    enabled: open && libraryId > 0 && Boolean(path),
  });
  const apply = useMutation({
    mutationFn: async () => {
      if (!preview.data) throw new Error('Preview is required before Apply.');
      return api.applyLibraryPathRename({ libraryId, path, planHash: preview.data.planHash });
    },
    onSuccess: async () => {
      await onApplied();
      onClose();
    },
  });
  const plan = preview.data;
  const hasChanges = plan?.assets.some((asset) =>
    asset.sourcePath !== asset.targetPath || asset.sidecars.some((sidecar) => sidecar.sourcePath !== sidecar.targetPath),
  ) ?? false;
  const sidecarCount = plan?.assets.reduce((count, asset) => count + asset.sidecars.length, 0) ?? 0;

  return (
    <Dialog
      open={open}
      onClose={() => { if (!apply.isPending) onClose(); }}
      disableEscapeKeyDown={apply.isPending}
      maxWidth="md"
      fullWidth
    >
      <DialogTitle>Rename Library path</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          {preview.isPending ? (
            <Stack direction="row" spacing={1} alignItems="center">
              <CircularProgress size={20} />
              <Typography>Building canonical rename preview…</Typography>
            </Stack>
          ) : null}
          {preview.isError ? (
            <Alert severity="error">
              Preview failed: {displayError(preview.error, 'Could not build the rename preview.')}
            </Alert>
          ) : null}
          {plan ? (
            <>
              <Stack spacing={1}>
                <Box>
                  <Typography color="text.secondary" variant="caption">Current</Typography>
                  <Typography sx={{ overflowWrap: 'anywhere' }}>{plan.sourcePath}</Typography>
                </Box>
                <Box>
                  <Typography color="text.secondary" variant="caption">Proposed</Typography>
                  <Typography sx={{ overflowWrap: 'anywhere' }}>{plan.targetPath}</Typography>
                </Box>
              </Stack>
              <Divider />
              <Stack spacing={1}>
                <Typography fontWeight={700}>{plan.assets.length} media file(s) · {sidecarCount} sidecar(s)</Typography>
                {plan.assets.map((asset) => (
                  <Box key={asset.assetId} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 1.25 }}>
                    <Typography color="text.secondary" variant="caption">Current media</Typography>
                    <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>{asset.sourcePath}</Typography>
                    <Typography color="text.secondary" variant="caption">Proposed media</Typography>
                    <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>{asset.targetPath}</Typography>
                    {asset.sidecars.length ? (
                      <Stack spacing={0.5} sx={{ mt: 1 }}>
                        <Typography fontWeight={700} variant="caption">Sidecars</Typography>
                        {asset.sidecars.map((sidecar) => (
                          <Typography key={sidecar.sourcePath} color="text.secondary" variant="caption" sx={{ overflowWrap: 'anywhere' }}>
                            {sidecar.sourcePath} → {sidecar.targetPath}
                          </Typography>
                        ))}
                      </Stack>
                    ) : null}
                  </Box>
                ))}
              </Stack>
              {plan.warnings.map((warning) => <Alert key={warning} severity="warning">{warning}</Alert>)}
              {plan.conflicts.map((conflict, index) => (
                <Alert key={`${conflict.code}-${conflict.sourcePath ?? ''}-${index}`} severity="error">
                  {conflict.code}: {conflict.sourcePath ?? plan.sourcePath}
                  {conflict.targetPath ? ` → ${conflict.targetPath}` : ''}
                </Alert>
              ))}
              {!hasChanges && plan.conflicts.length === 0 ? <Alert severity="info">This path is already canonical.</Alert> : null}
            </>
          ) : null}
          {apply.isError ? (
            <Alert severity="error">
              Apply failed: {displayError(apply.error, 'Could not rename this Library path.')}
            </Alert>
          ) : null}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={apply.isPending}>Cancel</Button>
        {preview.isError || apply.isError ? (
          <Button
            onClick={() => {
              apply.reset();
              preview.refetch();
            }}
            disabled={preview.isFetching || apply.isPending}
          >
            Refresh preview
          </Button>
        ) : null}
        <Button
          variant="contained"
          onClick={() => apply.mutate()}
          disabled={!plan || !hasChanges || plan.conflicts.length > 0 || apply.isPending}
        >
          {apply.isPending ? 'Applying…' : 'Apply rename'}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
