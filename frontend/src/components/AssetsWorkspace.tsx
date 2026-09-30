import {
  Box,
  Button,
  ButtonBase,
  Collapse,
  Divider,
  Grid,
  IconButton,
  List,
  ListItemButton,
  ListItemText,
  Stack,
  Tooltip,
  Typography,
} from '@mui/material';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import { useState } from 'react';
import { api } from '../api/client';
import type { Asset, AssetGroup, Library, ScanResult } from '../api/types';
import { buildLibraryTree, formatDurationSummary, type LibraryTreeNode, type LibraryTreeSelection } from './assetsWorkspaceModel';

export function AssetsSummaryStrip({ total, processing, completed, needsReview }: {
  total: number;
  processing: number;
  completed: number;
  needsReview: number;
}) {
  const items = [
    ['Total Assets', total],
    ['Processing', processing],
    ['Completed', completed],
    ['Needs Review', needsReview],
  ] as const;

  return (
    <Stack
      component="section"
      aria-label="Asset summary"
      direction="row"
      divider={<Divider orientation="vertical" flexItem />}
      sx={{
        minHeight: 44,
        alignItems: 'center',
        borderBottom: 1,
        borderColor: 'divider',
        '& > *': { flex: 1, minWidth: 0 },
      }}
    >
      {items.map(([label, value]) => (
        <Box key={label} sx={{ px: { xs: 1, sm: 1.5 }, py: 0.5 }}>
          <Typography variant="caption" color="text.secondary" noWrap>{label}</Typography>
          <Typography variant="body2" fontWeight={700} aria-label={`${label}: ${value}`}>{value.toLocaleString()}</Typography>
        </Box>
      ))}
    </Stack>
  );
}

export function LibraryTreePanel({ groups, libraries, selection, onSelect }: {
  groups: AssetGroup[];
  libraries: Library[];
  selection: LibraryTreeSelection;
  onSelect: (selection: LibraryTreeSelection) => void;
}) {
  const nodes = buildLibraryTree(groups, libraries);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const toggleExpanded = (id: string, next?: boolean) => setExpanded((current) => {
    const updated = new Set(current);
    if (next ?? !updated.has(id)) updated.add(id);
    else updated.delete(id);
    return updated;
  });

  const renderNode = (node: LibraryTreeNode, level: number) => {
    const hasChildren = node.children.length > 0;
    const isExpanded = expanded.has(node.id);
    const isSelected = selection.libraryId === node.libraryId && selection.path === node.path;
    return (
      <Box component="li" key={node.id} role="none" sx={{ listStyle: 'none' }}>
        <ListItemButton
          component="div"
          role="treeitem"
          aria-level={level}
          aria-selected={isSelected}
          aria-expanded={hasChildren ? isExpanded : undefined}
          tabIndex={0}
          onClick={() => onSelect({ libraryId: node.libraryId, path: node.path })}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault();
              onSelect({ libraryId: node.libraryId, path: node.path });
            } else if (event.key === 'ArrowRight' && hasChildren) {
              event.preventDefault();
              toggleExpanded(node.id, true);
            } else if (event.key === 'ArrowLeft' && hasChildren) {
              event.preventDefault();
              toggleExpanded(node.id, false);
            }
          }}
          sx={{
            minHeight: 34,
            borderRadius: 1,
            py: 0.25,
            pl: Math.min(1 + (level - 1) * 1.25, 6),
            pr: 0.75,
            '&[aria-selected="true"]': { bgcolor: 'action.selected', color: 'primary.main' },
          }}
        >
          {hasChildren ? (
            <IconButton
              size="small"
              tabIndex={-1}
              aria-label={`${isExpanded ? 'Collapse' : 'Expand'} ${node.label}`}
              onClick={(event) => { event.stopPropagation(); toggleExpanded(node.id); }}
              sx={{ mr: 0.5, p: 0.25 }}
            >
              <ExpandMoreIcon fontSize="small" sx={{ transform: isExpanded ? 'rotate(0deg)' : 'rotate(-90deg)', transition: 'transform 120ms ease' }} />
            </IconButton>
          ) : <Box sx={{ width: 24, flexShrink: 0 }} />}
          <ListItemText
            primary={node.label}
            primaryTypographyProps={{ variant: 'body2', noWrap: true, title: node.label }}
            sx={{ my: 0 }}
          />
        </ListItemButton>
        {hasChildren ? (
          <Collapse in={isExpanded} timeout="auto" unmountOnExit>
            <List component="ul" role="group" disablePadding sx={{ py: 0.25 }}>
              {node.children.map((child) => renderNode(child, level + 1))}
            </List>
          </Collapse>
        ) : null}
      </Box>
    );
  };

  return (
    <Box
      component="nav"
      aria-label="Library tree"
      sx={{
        width: { xs: '100%', md: 240, lg: 268 },
        flex: '0 0 auto',
        minWidth: 0,
        borderRight: { xs: 0, md: 1 },
        borderBottom: { xs: 1, md: 0 },
        borderColor: 'divider',
        pr: { xs: 0, md: 1 },
        pb: { xs: 1, md: 0 },
      }}
    >
      <Typography variant="overline" color="text.secondary" sx={{ px: 1.25, lineHeight: 2.5 }}>Library</Typography>
      <List role="tree" aria-label="Library folders" dense disablePadding>
        <ListItemButton
          component="div"
          role="treeitem"
          aria-level={1}
          aria-selected={selection.libraryId === null}
          tabIndex={0}
          onClick={() => onSelect({ libraryId: null, path: '' })}
          onKeyDown={(event) => {
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault();
              onSelect({ libraryId: null, path: '' });
            }
          }}
          sx={{ minHeight: 34, borderRadius: 1, py: 0.25, px: 1.25, '&[aria-selected="true"]': { bgcolor: 'action.selected', color: 'primary.main' } }}
        >
          <ListItemText primary="All Library assets" primaryTypographyProps={{ variant: 'body2', noWrap: true }} sx={{ my: 0 }} />
        </ListItemButton>
        {nodes.map((node) => renderNode(node, 1))}
        {!nodes.length ? <Typography variant="caption" color="text.secondary" sx={{ display: 'block', px: 1.25, py: 1 }}>No Library paths available.</Typography> : null}
      </List>
    </Box>
  );
}

export function SelectedAssetDetail({
  asset,
  pathContext,
  status,
  assignedProfile,
  snapshot,
  snapshotLoading,
  subtitlesSummary,
  modifiedLabel,
  sizeLabel,
  onPlayPreview,
  onAssignProfile,
  onTestEncode,
  onMore,
  canAssignProfile,
  canTestEncode,
  testEncodeDisabled = false,
}: {
  asset: Asset;
  pathContext: string;
  status: string;
  assignedProfile: string;
  snapshot?: ScanResult | null;
  snapshotLoading: boolean;
  subtitlesSummary: string;
  modifiedLabel: string;
  sizeLabel: string;
  onPlayPreview: () => void;
  onAssignProfile: () => void;
  onTestEncode: () => void;
  onMore: (anchor: HTMLElement) => void;
  canAssignProfile: boolean;
  canTestEncode: boolean;
  testEncodeDisabled?: boolean;
}) {
  const audioSummary = snapshot
    ? snapshot.audioStreams.slice(0, 2).map((stream) => [stream.codec, stream.channels ? `${stream.channels}ch` : '', stream.language].filter(Boolean).join(' ')).join(' · ') || 'None found'
    : 'Unavailable';
  const metadata = asset.metadata;
  const metadataValues = [...(metadata?.categories ?? []), ...(metadata?.tags ?? [])].filter(Boolean);
  const extension = asset.extension.replace(/^\./, '').toUpperCase() || 'Unavailable';
  const container = snapshot?.container ? snapshot.container.toUpperCase() : extension;
  const duration = snapshot?.duration ?? asset.technical?.duration;
  const videoCodec = snapshot?.videoCodec || asset.technical?.videoCodec || 'Unavailable';
  const resolution = snapshot?.width && snapshot?.height
    ? `${snapshot.width} × ${snapshot.height}`
    : asset.technical?.width && asset.technical?.height
      ? `${asset.technical.width} × ${asset.technical.height}`
      : 'Unavailable';
  const subtitleCount = snapshot?.subtitleStreams.length;
  const subtitleValue = snapshotLoading ? 'Loading…' : subtitlesSummary || (subtitleCount === undefined ? 'Unavailable' : `${subtitleCount} ${subtitleCount === 1 ? 'track' : 'tracks'}`);

  return (
    <Box
      component="section"
      aria-label={`Details for ${asset.fileName}`}
      data-testid="selected-asset-detail"
      sx={{ borderTop: 1, borderColor: 'divider', px: { xs: 1, sm: 1.5 }, py: 1.25, bgcolor: 'background.paper' }}
    >
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.25} alignItems={{ xs: 'stretch', sm: 'center' }}>
        <Tooltip title={`Play preview of ${asset.fileName}`}>
          <span>
            <ButtonBase
              onClick={onPlayPreview}
              disabled={asset.missing}
              aria-label={`Play preview of ${asset.fileName}`}
              sx={{
                width: 68,
                height: 52,
                flexShrink: 0,
                borderRadius: 1,
                bgcolor: 'rgba(255,255,255,0.06)',
                color: 'primary.main',
                overflow: 'hidden',
                position: 'relative',
                '&:hover': { bgcolor: 'rgba(79,179,255,0.14)' },
              }}
            >
              {!asset.missing ? (
                <Box
                  component="img"
                  src={api.assetPreviewUrl(asset.path)}
                  alt={`${asset.fileName} preview`}
                  loading="lazy"
                  sx={{ position: 'absolute', inset: 0, width: '100%', height: '100%', objectFit: 'cover' }}
                />
              ) : null}
              <Box sx={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', bgcolor: 'rgba(0,0,0,0.28)' }}>
                <PlayCircleOutlineIcon fontSize="large" />
              </Box>
            </ButtonBase>
          </span>
        </Tooltip>

        <Box sx={{ minWidth: 0, flex: 1 }}>
          <Stack direction="row" spacing={0.75} alignItems="center" sx={{ minWidth: 0 }}>
            <Tooltip title={asset.fileName}>
              <Typography fontWeight={700} variant="body2" noWrap sx={{ minWidth: 0 }}>{asset.fileName}</Typography>
            </Tooltip>
            <Typography variant="caption" color="text.secondary" noWrap>{status}</Typography>
          </Stack>
          <Typography variant="caption" color="text.secondary" display="block" noWrap title={pathContext}>{pathContext}</Typography>
          <Stack direction="row" flexWrap="wrap" columnGap={1.25} rowGap={0.25} sx={{ mt: 0.4 }}>
            <Typography variant="caption" color="text.secondary">Profile · {assignedProfile || 'Unavailable'}</Typography>
            <Typography variant="caption" color="text.secondary">Subtitles · {subtitleValue}</Typography>
            <Typography variant="caption" color="text.secondary">Duration · {formatDurationSummary(duration)}</Typography>
            <Typography variant="caption" color="text.secondary">Modified · {modifiedLabel || 'Unavailable'}</Typography>
          </Stack>
          <Grid container columnSpacing={1.5} rowSpacing={0.15} sx={{ mt: 0.15 }}>
            <DetailValue label="Container" value={container} />
            <DetailValue label="Size" value={sizeLabel} />
            <DetailValue label="Video" value={videoCodec} />
            <DetailValue label="Resolution" value={resolution} />
            <DetailValue label="Audio" value={audioSummary} />
          </Grid>
          {metadataValues.length ? (
            <Typography variant="caption" color="text.secondary" display="block" noWrap title={metadataValues.join(', ')}>
              Metadata · {metadataValues.join(', ')}
            </Typography>
          ) : null}
        </Box>

        <Stack direction="row" spacing={0.5} alignItems="center" flexWrap="wrap" useFlexGap>
          {canAssignProfile ? <Button size="small" variant="outlined" onClick={onAssignProfile}>Assign Profile</Button> : null}
          {canTestEncode ? <Button size="small" variant="contained" color="secondary" onClick={onTestEncode} disabled={testEncodeDisabled}>Test Encode</Button> : null}
          <Tooltip title="More asset actions">
            <IconButton size="small" aria-label={`More actions for ${asset.fileName}`} onClick={(event) => onMore(event.currentTarget)}>
              <MoreVertIcon fontSize="small" />
            </IconButton>
          </Tooltip>
        </Stack>
      </Stack>
    </Box>
  );
}

function DetailValue({ label, value }: { label: string; value: string }) {
  return (
    <Grid size={{ xs: 6, sm: 'auto' }}>
      <Typography variant="caption" color="text.secondary" noWrap title={`${label} · ${value}`}>
        {label} · {value}
      </Typography>
    </Grid>
  );
}
