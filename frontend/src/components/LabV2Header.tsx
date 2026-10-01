import { useState, type ReactNode } from 'react';
import {
  Box,
  Button,
  ButtonGroup,
  Card,
  Collapse,
  IconButton,
  Menu,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import MoreHorizOutlinedIcon from '@mui/icons-material/MoreHorizOutlined';
import MovieOutlinedIcon from '@mui/icons-material/MovieOutlined';
import OpenInNewOutlinedIcon from '@mui/icons-material/OpenInNewOutlined';
import RestartAltOutlinedIcon from '@mui/icons-material/RestartAltOutlined';
import type { Asset, ScanResult } from '../api/types';
import { formatTimecode, parseStartSeconds } from '../utils/labV2Time';

type Scope = 'asset' | 'path';

type Props = {
  asset: Asset | null;
  snapshot?: ScanResult;
  assetPicker: ReactNode;
  scope: Scope;
  onScopeChange: (scope: Scope) => void;
  start: string;
  seconds: number;
  rangeValid: boolean;
  onStartChange: (start: string) => void;
  onSecondsChange: (seconds: number) => void;
  runDisabled: boolean;
  onRun: () => void;
  onOpenAssetInfo: () => void;
  onRestoreProfile?: () => void;
  onOpenProfileDetails?: () => void;
};

function formatBytes(value?: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return 'Unavailable';
  if (value < 1024) return `${value} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let amount = value / 1024;
  let unit = units[0];
  for (let index = 1; amount >= 1024 && index < units.length; index += 1) {
    amount /= 1024;
    unit = units[index];
  }
  return `${amount.toFixed(2)} ${unit}`;
}

function available(value: string | number | undefined): string {
  return value === undefined || value === '' || value === 0 ? 'Unavailable' : String(value);
}

export function LabV2Header({
  asset,
  snapshot,
  assetPicker,
  scope,
  onScopeChange,
  start,
  seconds,
  rangeValid,
  onStartChange,
  onSecondsChange,
  runDisabled,
  onRun,
  onOpenAssetInfo,
  onRestoreProfile,
  onOpenProfileDetails,
}: Props) {
  const [rangeOpen, setRangeOpen] = useState(false);
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
  const parsedStart = parseStartSeconds(start);
  const rangeLabel = rangeValid && parsedStart !== null
    ? `${formatTimecode(parsedStart)} → ${formatTimecode(parsedStart + seconds)}`
    : 'Set a valid test range';
  const resolution = snapshot?.width && snapshot.height ? `${snapshot.width} × ${snapshot.height}` : 'Unavailable';
  const codec = snapshot?.videoCodec || asset?.technical?.videoCodec;
  const duration = snapshot?.duration || asset?.technical?.duration;
  const size = snapshot?.sizeBytes || asset?.sizeBytes;
  const container = snapshot?.container || asset?.extension?.replace(/^\./, '').toUpperCase();
  const activeMenu = Boolean(menuAnchor);

  return (
    <Card
      component="section"
      variant="outlined"
      aria-label="LAB asset context"
      sx={{
        mb: 1.5,
        p: { xs: 1.25, sm: 1.5 },
        borderRadius: 1.5,
        bgcolor: 'background.paper',
        borderColor: 'divider',
      }}
    >
      <Box
        sx={{
          display: 'grid',
          minWidth: 0,
          alignItems: 'center',
          gap: { xs: 1.5, lg: 2 },
          gridTemplateColumns: { xs: 'minmax(0, 1fr)', lg: 'minmax(260px, 1.5fr) minmax(185px, .65fr) minmax(190px, .7fr) auto' },
        }}
      >
        <Stack direction="row" spacing={1.25} alignItems="center" sx={{ minWidth: 0 }}>
          <Box
            aria-hidden="true"
            sx={{
              display: 'grid',
              placeItems: 'center',
              flex: '0 0 52px',
              width: 52,
              height: 52,
              borderRadius: 1.25,
              bgcolor: 'action.hover',
              color: 'primary.main',
            }}
          >
            <MovieOutlinedIcon />
          </Box>
          <Stack spacing={0.6} sx={{ minWidth: 0, flex: 1 }}>
            <Box sx={{ minWidth: 0 }}>{assetPicker}</Box>
            <Typography variant="subtitle2" title={asset?.fileName} noWrap sx={{ minWidth: 0, fontWeight: 700 }}>
              {asset?.fileName || 'Select an asset for Test Encode'}
            </Typography>
            <Typography variant="caption" color="text.secondary" title={asset?.path} noWrap sx={{ minWidth: 0 }}>
              {asset?.groupPath || asset?.path || 'Asset path unavailable'}
            </Typography>
            <Stack direction="row" spacing={1} useFlexGap flexWrap="wrap" aria-label="Asset technical summary">
              <Typography variant="caption" color="text.secondary">{available(container)}</Typography>
              <Typography variant="caption" color="text.secondary">{formatBytes(size)}</Typography>
              <Typography variant="caption" color="text.secondary">{resolution}</Typography>
              <Typography variant="caption" color="text.secondary">{available(codec)}</Typography>
              <Typography variant="caption" color="text.secondary">
                {typeof duration === 'number' && duration > 0 ? `${formatTimecode(duration)}` : 'Duration unavailable'}
              </Typography>
            </Stack>
          </Stack>
        </Stack>

        <Stack spacing={0.6} sx={{ minWidth: 0 }}>
          <Typography variant="caption" color="text.secondary">Scope</Typography>
          <ButtonGroup fullWidth size="small" aria-label="Profile scope">
            <Button aria-pressed={scope === 'path'} variant={scope === 'path' ? 'contained' : 'outlined'} onClick={() => onScopeChange('path')}>Path</Button>
            <Button aria-pressed={scope === 'asset'} variant={scope === 'asset' ? 'contained' : 'outlined'} onClick={() => onScopeChange('asset')}>Asset</Button>
          </ButtonGroup>
          <Typography variant="caption" color="text.secondary">
            Profile scope; the test runs on this asset.
          </Typography>
        </Stack>

        <Stack spacing={0.6} sx={{ minWidth: 0 }}>
          <Typography variant="caption" color="text.secondary">Test range</Typography>
          <Button
            variant="outlined"
            size="small"
            aria-expanded={rangeOpen}
            aria-controls="lab-range-editor"
            aria-label={`Edit test range. ${rangeLabel}`}
            onClick={() => setRangeOpen((open) => !open)}
            sx={{ justifyContent: 'space-between', minHeight: 40, fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' }}
          >
            {rangeLabel}
            <Typography component="span" variant="caption" color="text.secondary" sx={{ ml: 1 }}>Edit</Typography>
          </Button>
        </Stack>

        <Stack direction="row" spacing={0.75} alignItems="center" justifyContent={{ xs: 'stretch', lg: 'flex-end' }}>
          <Button
            variant="contained"
            onClick={onRun}
            disabled={runDisabled}
            aria-label="Run Test Encode"
            sx={{ minHeight: 42, whiteSpace: 'nowrap', flex: { xs: 1, lg: 'initial' } }}
          >
            Run Test Encode
          </Button>
          <IconButton aria-label="More LAB actions" aria-haspopup="menu" onClick={(event) => setMenuAnchor(event.currentTarget)}>
            <MoreHorizOutlinedIcon />
          </IconButton>
          <Menu anchorEl={menuAnchor} open={activeMenu} onClose={() => setMenuAnchor(null)}>
            {onOpenProfileDetails ? (
              <MenuItem onClick={() => { setMenuAnchor(null); onOpenProfileDetails(); }}>
                <RestartAltOutlinedIcon fontSize="small" sx={{ mr: 1 }} /> Edit profile details
              </MenuItem>
            ) : null}
            {onRestoreProfile ? (
              <MenuItem onClick={() => { setMenuAnchor(null); onRestoreProfile(); }}>
                <RestartAltOutlinedIcon fontSize="small" sx={{ mr: 1 }} /> Restore selected profile
              </MenuItem>
            ) : null}
            <MenuItem disabled={!asset} onClick={() => { setMenuAnchor(null); onOpenAssetInfo(); }}>
              <OpenInNewOutlinedIcon fontSize="small" sx={{ mr: 1 }} /> Open Asset Info
            </MenuItem>
          </Menu>
        </Stack>
      </Box>

      <Collapse in={rangeOpen} unmountOnExit>
        <Stack
          id="lab-range-editor"
          direction={{ xs: 'column', sm: 'row' }}
          spacing={1}
          alignItems={{ sm: 'flex-start' }}
          sx={{ mt: 1.5, pt: 1.5, borderTop: 1, borderColor: 'divider' }}
        >
          <TextField
            label="Start time"
            size="small"
            value={start}
            onChange={(event) => onStartChange(event.target.value)}
            placeholder="HH:MM:SS"
            error={!rangeValid}
            helperText={!rangeValid ? 'Use non-negative seconds or HH:MM:SS.' : 'Seconds or HH:MM:SS'}
            inputProps={{ 'aria-label': 'Test range start time' }}
            sx={{ width: { xs: '100%', sm: 210 } }}
          />
          <TextField
            label="Duration (seconds)"
            size="small"
            type="number"
            value={seconds}
            inputProps={{ min: 5, max: 60, 'aria-label': 'Test range duration in seconds' }}
            onChange={(event) => onSecondsChange(Math.min(60, Number(event.target.value)))}
            onBlur={() => onSecondsChange(Math.max(5, Math.min(60, seconds || 5)))}
            sx={{ width: { xs: '100%', sm: 210 } }}
          />
          <Button size="small" onClick={() => setRangeOpen(false)} sx={{ minHeight: 40 }}>Done</Button>
        </Stack>
      </Collapse>
    </Card>
  );
}
