import {
  Box,
  Breadcrumbs,
  Button,
  ButtonBase,
  Card,
  Collapse,
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
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import StorageOutlinedIcon from '@mui/icons-material/StorageOutlined';
import AutorenewOutlinedIcon from '@mui/icons-material/AutorenewOutlined';
import TaskAltOutlinedIcon from '@mui/icons-material/TaskAltOutlined';
import ReportProblemOutlinedIcon from '@mui/icons-material/ReportProblemOutlined';
import { useMemo, useRef, useState, type ReactNode } from 'react';
import { api } from '../api/client';
import type { Asset, AssetGroup, AssetSourceGroup, Library, ScanResult } from '../api/types';
import { buildLibraryTree, buildRawHierarchy, formatDurationSummary, type LibraryTreeNode, type LibraryTreeSelection, type RawHierarchyNode, type RawHierarchySelection } from './assetsWorkspaceModel';

export function AssetsSummaryStrip({ total, processing, completed, needsReview }: {
  total: number;
  processing: number;
  completed: number;
  needsReview: number;
}) {
  const items = [
    { label: 'Total Assets', value: total, icon: <StorageOutlinedIcon />, color: 'info.main', tint: 'rgba(42, 156, 255, 0.14)' },
    { label: 'Processing', value: processing, icon: <AutorenewOutlinedIcon />, color: 'secondary.main', tint: 'rgba(158, 93, 255, 0.14)' },
    { label: 'Completed', value: completed, icon: <TaskAltOutlinedIcon />, color: 'success.main', tint: 'rgba(0, 206, 145, 0.14)' },
    { label: 'Needs Review', value: needsReview, icon: <ReportProblemOutlinedIcon />, color: 'error.main', tint: 'rgba(255, 74, 98, 0.14)' },
  ] as const;

  return (
    <Box
      component="section"
      aria-label="Asset summary"
      sx={{
        display: 'grid',
        gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', md: 'repeat(4, minmax(0, 1fr))' },
        gap: { xs: 1, sm: 1.25 },
      }}
    >
      {items.map(({ label, value, icon, color, tint }) => (
        <Card
          key={label}
          variant="outlined"
          sx={{
            display: 'flex',
            minWidth: 0,
            minHeight: { xs: 72, sm: 82 },
            alignItems: 'center',
            gap: { xs: 1, sm: 1.5 },
            px: { xs: 1, sm: 1.5 },
            py: 1,
            borderRadius: 1.5,
            bgcolor: 'background.paper',
            borderColor: 'divider',
          }}
        >
          <Box aria-hidden="true" sx={{
            display: 'grid',
            placeItems: 'center',
            width: { xs: 36, sm: 44 },
            height: { xs: 36, sm: 44 },
            flexShrink: 0,
            borderRadius: 1.25,
            bgcolor: tint,
            color,
            '& svg': { fontSize: { xs: 20, sm: 24 } },
          }}>
            {icon}
          </Box>
          <Stack spacing={0.15} sx={{ minWidth: 0 }}>
            <Typography variant="caption" color="text.secondary" noWrap>{label}</Typography>
            <Typography variant="h2" fontWeight={700} aria-label={`${label}: ${value}`} sx={{ fontSize: { xs: '1.1rem', sm: '1.25rem' }, lineHeight: 1.1 }}>
              {value.toLocaleString()}
            </Typography>
          </Stack>
        </Card>
      ))}
    </Box>
  );
}

export function WorkspaceBreadcrumbs({ segments, label }: { segments: string[]; label: string }) {
  return (
    <Breadcrumbs
      aria-label={label}
      separator={<NavigateNextIcon fontSize="small" />}
      maxItems={4}
      itemsBeforeCollapse={2}
      itemsAfterCollapse={1}
      sx={{
        minWidth: 0,
        '& .MuiBreadcrumbs-ol': { flexWrap: 'nowrap' },
        '& .MuiBreadcrumbs-li': { minWidth: 0 },
        '& .MuiTypography-root': { maxWidth: { xs: '7rem', sm: '12rem', lg: '16rem' }, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
      }}
    >
      {segments.map((segment, index) => (
        <Typography key={`${index}-${segment}`} variant="caption" color={index === segments.length - 1 ? 'text.primary' : 'text.secondary'} title={segment}>
          {segment}
        </Typography>
      ))}
    </Breadcrumbs>
  );
}

type AssetsHierarchyNode =
  | { kind: 'raw'; id: string; label: string; selection: RawHierarchySelection; children: AssetsHierarchyNode[]; sourceRoot?: boolean }
  | { kind: 'library'; id: string; label: string; selection: LibraryTreeSelection; children: AssetsHierarchyNode[] };

function rawTreeNodes(nodes: RawHierarchyNode[]): AssetsHierarchyNode[] {
  return nodes.map((node) => ({
    kind: 'raw',
    id: node.id,
    label: node.label,
    selection: node.selection,
    children: rawTreeNodes(node.children),
  }));
}

function libraryTreeNodes(nodes: LibraryTreeNode[]): AssetsHierarchyNode[] {
  return nodes.map((node) => ({
    kind: 'library',
    id: node.id,
    label: node.label,
    selection: { libraryId: node.libraryId, path: node.path },
    children: libraryTreeNodes(node.children),
  }));
}

function hierarchySelectionMatches(node: AssetsHierarchyNode, raw: RawHierarchySelection | null, library: LibraryTreeSelection) {
  if (node.kind === 'raw') {
    if (!raw || node.selection.kind !== raw.kind) return false;
    if (node.selection.kind === 'all' && raw.kind === 'all') return true;
    if (node.selection.kind === 'source' && raw.kind === 'source') return node.selection.sourceGroupId === raw.sourceGroupId;
    if (node.selection.kind === 'logical' && raw.kind === 'logical') return node.selection.sourceGroupId === raw.sourceGroupId && node.selection.logicalGroupId === raw.logicalGroupId;
    return node.selection.kind === 'path' && raw.kind === 'path'
      && node.selection.sourceGroupId === raw.sourceGroupId
      && node.selection.logicalGroupId === raw.logicalGroupId
      && node.selection.pathId === raw.pathId;
  }
  return library.libraryId !== null && node.selection.libraryId === library.libraryId && node.selection.path === library.path;
}

function selectedHierarchyAncestors(nodes: AssetsHierarchyNode[], selected: (node: AssetsHierarchyNode) => boolean, parents: string[] = []): string[] {
  for (const node of nodes) {
    if (selected(node)) return parents;
    const path = selectedHierarchyAncestors(node.children, selected, [...parents, node.id]);
    if (path.length) return path;
  }
  return [];
}

/** One presentation tree over the existing, separate RAW and Library projections. */
export function AssetsHierarchyPanel({ sourceGroups, groups, libraries, location, rawSelection, librarySelection, onSelectRaw, onSelectLibrary }: {
  sourceGroups: AssetSourceGroup[];
  groups: AssetGroup[];
  libraries: Library[];
  location: 'raw' | 'library';
  rawSelection: RawHierarchySelection | null;
  librarySelection: LibraryTreeSelection;
  onSelectRaw: (selection: RawHierarchySelection) => void;
  onSelectLibrary: (selection: LibraryTreeSelection) => void;
}) {
  const nodes = useMemo<AssetsHierarchyNode[]>(() => [{
    kind: 'raw',
    id: 'assets-raw-root',
    label: 'RAW',
    selection: { kind: 'all' },
    children: rawTreeNodes(buildRawHierarchy(sourceGroups)),
    sourceRoot: true,
  }, ...libraryTreeNodes(buildLibraryTree(groups, libraries))], [sourceGroups, groups, libraries]);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set(['assets-raw-root']));
  const [collapsed, setCollapsed] = useState<Set<string>>(() => new Set());
  const treeItemRefs = useRef(new Map<string, HTMLElement>());
  const selectedAncestors = useMemo(() => selectedHierarchyAncestors(nodes, (node) => location === 'raw'
    ? node.kind === 'raw' && hierarchySelectionMatches(node, rawSelection, librarySelection)
    : node.kind === 'library' && hierarchySelectionMatches(node, rawSelection, librarySelection)), [nodes, location, rawSelection, librarySelection]);
  const expandedItems = useMemo(() => new Set([...expanded, ...selectedAncestors].filter((id) => !collapsed.has(id))), [expanded, selectedAncestors, collapsed]);
  const toggleExpanded = (id: string, next?: boolean) => {
    const shouldExpand = next ?? !expandedItems.has(id);
    setExpanded((current) => {
      const updated = new Set(current);
      if (shouldExpand) updated.add(id);
      else updated.delete(id);
      return updated;
    });
    setCollapsed((current) => {
      const updated = new Set(current);
      if (shouldExpand) updated.delete(id);
      else updated.add(id);
      return updated;
    });
  };

  const renderNode = (node: AssetsHierarchyNode, level: number, parentNodeId?: string): ReactNode => {
    const isRawRoot = node.kind === 'raw' && node.sourceRoot === true;
    const hasChildren = node.children.length > 0;
    const isExpanded = expandedItems.has(node.id);
    const isSelected = (location === 'raw' && node.kind === 'raw' && hierarchySelectionMatches(node, rawSelection, librarySelection))
      || (location === 'library' && node.kind === 'library' && hierarchySelectionMatches(node, rawSelection, librarySelection));
    const selectNode = () => {
      setExpanded((current) => new Set([...current, ...selectedAncestors]));
      setCollapsed(new Set());
      if (node.kind === 'raw') onSelectRaw(node.selection);
      else onSelectLibrary(node.selection);
    };
    return (
      <Box component="li" key={node.id} role="none" sx={{ listStyle: 'none' }}>
        <ListItemButton
          component="div"
          ref={(element) => {
            if (element) treeItemRefs.current.set(node.id, element);
            else treeItemRefs.current.delete(node.id);
          }}
          role="treeitem"
          aria-level={level}
          aria-label={isRawRoot ? 'RAW source inventory' : node.label}
          aria-selected={isSelected}
          aria-expanded={hasChildren ? isExpanded : undefined}
          tabIndex={0}
          onClick={selectNode}
          onKeyDown={(event) => {
            const treeItems = Array.from(event.currentTarget.closest('[role="tree"]')?.querySelectorAll<HTMLElement>('[role="treeitem"]') ?? []);
            const currentIndex = treeItems.indexOf(event.currentTarget);
            if (event.key === 'Enter' || event.key === ' ') {
              event.preventDefault();
              selectNode();
            } else if (event.key === 'ArrowRight' && hasChildren) {
              event.preventDefault();
              if (!isExpanded) toggleExpanded(node.id, true);
              else event.currentTarget.parentElement?.querySelector<HTMLElement>('[role="group"] [role="treeitem"]')?.focus();
            } else if (event.key === 'ArrowLeft') {
              event.preventDefault();
              if (hasChildren && isExpanded) {
                toggleExpanded(node.id, false);
              } else if (parentNodeId) {
                treeItemRefs.current.get(parentNodeId)?.focus();
              }
            } else if (event.key === 'ArrowDown' && treeItems[currentIndex + 1]) {
              event.preventDefault();
              treeItems[currentIndex + 1].focus();
            } else if (event.key === 'ArrowUp' && treeItems[currentIndex - 1]) {
              event.preventDefault();
              treeItems[currentIndex - 1].focus();
            }
          }}
          sx={{
            minHeight: isRawRoot ? 40 : 36,
            borderRadius: 1,
            borderLeft: '2px solid transparent',
            py: 0.3,
            pl: Math.min(1 + (level - 1) * 1.35, 6),
            pr: 0.75,
            fontWeight: isRawRoot ? 700 : 400,
            '&[aria-selected="true"]': { bgcolor: 'rgba(101, 86, 255, 0.16)', color: 'primary.main', borderLeftColor: 'primary.main' },
            '&:focus-visible': { outline: '2px solid', outlineColor: 'primary.main', outlineOffset: -2 },
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
            secondary={isRawRoot ? 'Source inventory' : undefined}
            primaryTypographyProps={{ variant: 'body2', noWrap: true, title: node.label, fontWeight: isRawRoot ? 700 : undefined }}
            secondaryTypographyProps={{ variant: 'caption', noWrap: true }}
            sx={{ my: 0 }}
          />
        </ListItemButton>
        {hasChildren ? (
          <Collapse in={isExpanded} timeout="auto" unmountOnExit>
            <List component="ul" role="group" disablePadding sx={{ py: 0.25 }}>
              {node.children.map((child) => renderNode(child, level + 1, node.id))}
            </List>
          </Collapse>
        ) : null}
      </Box>
    );
  };

  return (
    <Box
      component="nav"
      aria-label="Assets hierarchy"
      sx={{
        width: { xs: '100%', lg: '27%' },
        maxWidth: { lg: 350 },
        flex: { xs: '1 1 auto', lg: '0 0 27%' },
        minWidth: 0,
        alignSelf: 'stretch',
        border: 1,
        borderColor: 'divider',
        borderRadius: 1.5,
        bgcolor: 'background.paper',
        p: 1,
      }}
    >
      <Stack direction="row" alignItems="baseline" justifyContent="space-between" sx={{ px: 0.5, pb: 0.75 }}>
        <Typography variant="subtitle2" fontWeight={700}>Assets hierarchy</Typography>
        <Typography variant="caption" color="text.secondary">{nodes.length} roots</Typography>
      </Stack>
      <List role="tree" aria-label="Assets hierarchy" dense disablePadding>
        {nodes.map((node) => renderNode(node, 1))}
        {location === 'library' && librarySelection.libraryId === null ? (
          <Typography component="li" role="none" variant="caption" color="text.secondary" sx={{ display: 'block', px: 1.25, py: 0.75 }}>
            Showing all Library assets
          </Typography>
        ) : null}
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
  const summaryValues = [
    ['Profile', assignedProfile || 'Unavailable'],
    ['Subtitles', subtitleValue],
    ['Duration', formatDurationSummary(duration)],
    ['Modified', modifiedLabel || 'Unavailable'],
  ];
  const technicalValues = [
    ['Container', container],
    ['Size', sizeLabel],
    ['Video', videoCodec],
    ['Resolution', resolution],
    ['Audio', audioSummary],
  ];

  return (
    <Box
      component="section"
      aria-label={`Details for ${asset.fileName}`}
      data-testid="selected-asset-detail"
      sx={{
        mt: 1.25,
        p: { xs: 1, sm: 1.5 },
        border: 1,
        borderColor: 'divider',
        borderRadius: 1.5,
        bgcolor: 'background.paper',
        boxShadow: '0 8px 24px rgba(0, 0, 0, 0.12)',
      }}
    >
      <Stack direction={{ xs: 'column', xl: 'row' }} spacing={{ xs: 1.25, xl: 1.5 }} alignItems={{ xs: 'stretch', xl: 'center' }}>
        <Tooltip title={`Play preview of ${asset.fileName}`}>
          <span>
            <ButtonBase
              onClick={onPlayPreview}
              disabled={asset.missing}
              aria-label={`Play preview of ${asset.fileName}`}
              sx={{
                width: { xs: '100%', sm: 240, xl: 280 },
                aspectRatio: '16 / 9',
                height: 'auto',
                flexShrink: 0,
                borderRadius: 1.25,
                bgcolor: 'rgba(255,255,255,0.06)',
                color: 'primary.main',
                overflow: 'hidden',
                position: 'relative',
                boxShadow: 'inset 0 0 0 1px rgba(255,255,255,0.1)',
                '&:hover': { bgcolor: 'rgba(79,179,255,0.14)' },
                '&:focus-visible': { outline: '2px solid', outlineColor: 'primary.main', outlineOffset: 2 },
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

        <Box
          sx={{
            minWidth: 0,
            flex: 1,
            display: 'grid',
            gridTemplateColumns: { xs: 'minmax(0, 1fr)', xl: 'minmax(0, 1.1fr) minmax(220px, 0.9fr)' },
            gap: { xs: 1, xl: 1.5 },
          }}
        >
          <Stack spacing={0.75} sx={{ minWidth: 0 }}>
            <Stack direction="row" spacing={1} alignItems="center" sx={{ minWidth: 0 }}>
              <Tooltip title={asset.fileName}>
                <Typography component="h2" fontWeight={700} variant="h2" noWrap sx={{ minWidth: 0, fontSize: { xs: '1rem', sm: '1.1rem' }, lineHeight: 1.25 }}>
                  {asset.fileName}
                </Typography>
              </Tooltip>
              <Box sx={{ flexShrink: 0, border: 1, borderColor: 'divider', borderRadius: 99, px: 1, py: 0.2, bgcolor: 'rgba(255,255,255,0.035)' }}>
                <Typography variant="caption" fontWeight={600} noWrap>{status}</Typography>
              </Box>
            </Stack>
            <Typography variant="caption" color="text.secondary" noWrap title={pathContext}>{pathContext}</Typography>
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', sm: 'repeat(4, minmax(0, 1fr))' }, gap: 0.75, mt: 0.4 }}>
              {summaryValues.map(([label, value]) => (
                <Box key={label} sx={{ minWidth: 0 }}>
                  <Typography component="div" variant="caption" color="text.secondary" noWrap>{label}</Typography>
                  <Typography variant="body2" fontWeight={500} noWrap title={value}>{value}</Typography>
                </Box>
              ))}
            </Box>
            {metadataValues.length ? (
              <Typography variant="caption" color="text.secondary" display="block" noWrap title={metadataValues.join(', ')}>
                Metadata · {metadataValues.join(', ')}
              </Typography>
            ) : null}
          </Stack>

          <Box
            sx={{
              minWidth: 0,
              borderLeft: { xl: 1 },
              borderTop: { xs: 1, xl: 0 },
              borderColor: 'divider',
              pl: { xl: 1.5 },
              pt: { xs: 1, xl: 0 },
            }}
          >
            <Typography variant="caption" fontWeight={700} color="text.secondary">Technical information</Typography>
            <Grid container columnSpacing={1.25} rowSpacing={0.5} sx={{ mt: 0.3 }}>
              {technicalValues.map(([label, value]) => <DetailValue key={label} label={label} value={value} />)}
            </Grid>
          </Box>
        </Box>

        <Stack direction="row" spacing={0.5} alignItems="center" justifyContent={{ xs: 'flex-start', xl: 'center' }} flexWrap="wrap" useFlexGap sx={{ flexShrink: 0 }}>
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
