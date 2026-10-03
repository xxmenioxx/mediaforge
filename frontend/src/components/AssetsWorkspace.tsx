import {
  Box,
  Breadcrumbs,
  Button,
  ButtonBase,
  Card,
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
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import StorageOutlinedIcon from '@mui/icons-material/StorageOutlined';
import AutorenewOutlinedIcon from '@mui/icons-material/AutorenewOutlined';
import TaskAltOutlinedIcon from '@mui/icons-material/TaskAltOutlined';
import ReportProblemOutlinedIcon from '@mui/icons-material/ReportProblemOutlined';
import MovieOutlinedIcon from '@mui/icons-material/MovieOutlined';
import { useMemo, useRef, useState, type ReactNode } from 'react';
import type { Asset, AssetGroup, AssetSourceGroup, Library, ScanResult } from '../api/types';
import { buildLibraryTree, buildRawHierarchy, formatDurationSummary, type LibraryTreeNode, type LibraryTreeSelection, type RawHierarchyNode, type RawHierarchySelection } from './assetsWorkspaceModel';

export const ASSETS_HIERARCHY_WIDTH_STORAGE_KEY = 'mvforge.assets.hierarchyWidth';
export const ASSETS_HIERARCHY_MIN_WIDTH = 220;
export const ASSETS_HIERARCHY_DEFAULT_WIDTH = 300;
export const ASSETS_HIERARCHY_MAX_WIDTH = 420;

function clampHierarchyWidth(width: number) {
  return Math.min(ASSETS_HIERARCHY_MAX_WIDTH, Math.max(ASSETS_HIERARCHY_MIN_WIDTH, Math.round(width)));
}

function readStoredHierarchyWidth() {
  if (typeof window === 'undefined') return ASSETS_HIERARCHY_DEFAULT_WIDTH;
  try {
    const stored = Number(window.localStorage.getItem(ASSETS_HIERARCHY_WIDTH_STORAGE_KEY));
    return Number.isFinite(stored) && stored > 0 ? clampHierarchyWidth(stored) : ASSETS_HIERARCHY_DEFAULT_WIDTH;
  } catch {
    return ASSETS_HIERARCHY_DEFAULT_WIDTH;
  }
}

function persistHierarchyWidth(width: number) {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(ASSETS_HIERARCHY_WIDTH_STORAGE_KEY, String(width));
  } catch {
    // The tree remains resizable for the current session when storage is unavailable.
  }
}

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
  const [hierarchyWidth, setHierarchyWidth] = useState(readStoredHierarchyWidth);
  const resizeGesture = useRef<{ startX: number; startWidth: number } | null>(null);
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

  const resizeTo = (width: number, save: boolean) => {
    const nextWidth = clampHierarchyWidth(width);
    setHierarchyWidth(nextWidth);
    if (save) persistHierarchyWidth(nextWidth);
  };

  return (
    <Box
      data-testid="assets-hierarchy-layout"
      sx={{
        display: 'flex',
        alignItems: 'stretch',
        width: { xs: '100%', lg: `${hierarchyWidth + 8}px` },
        minWidth: 0,
        maxWidth: '100%',
        flex: { xs: '1 1 auto', lg: `0 0 ${hierarchyWidth + 8}px` },
      }}
    >
    <Box
      component="nav"
      id="assets-hierarchy-navigation"
      aria-label="Assets hierarchy"
      sx={{
        width: { xs: '100%', lg: `${hierarchyWidth}px` },
        minWidth: { xs: 0, lg: `${ASSETS_HIERARCHY_MIN_WIDTH}px` },
        maxWidth: { lg: `${ASSETS_HIERARCHY_MAX_WIDTH}px` },
        flex: { xs: '1 1 auto', lg: `0 0 ${hierarchyWidth}px` },
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
    <Box
      component="div"
      role="separator"
      aria-label="Resize Assets hierarchy"
      aria-controls="assets-hierarchy-navigation"
      aria-orientation="vertical"
      aria-valuemin={ASSETS_HIERARCHY_MIN_WIDTH}
      aria-valuemax={ASSETS_HIERARCHY_MAX_WIDTH}
      aria-valuenow={hierarchyWidth}
      aria-valuetext={`${hierarchyWidth} pixels`}
      tabIndex={0}
      data-testid="assets-hierarchy-resizer"
      onKeyDown={(event) => {
        const nextWidth = event.key === 'ArrowLeft'
          ? hierarchyWidth - 16
          : event.key === 'ArrowRight'
            ? hierarchyWidth + 16
            : event.key === 'Home'
              ? ASSETS_HIERARCHY_MIN_WIDTH
              : event.key === 'End'
                ? ASSETS_HIERARCHY_MAX_WIDTH
                : null;
        if (nextWidth === null) return;
        event.preventDefault();
        resizeTo(nextWidth, true);
      }}
      onPointerDown={(event) => {
        if (event.button !== 0) return;
        event.preventDefault();
        resizeGesture.current = { startX: event.clientX, startWidth: hierarchyWidth };
        event.currentTarget.setPointerCapture?.(event.pointerId);
      }}
      onPointerMove={(event) => {
        const gesture = resizeGesture.current;
        if (gesture) resizeTo(gesture.startWidth + event.clientX - gesture.startX, false);
      }}
      onPointerUp={(event) => {
        const gesture = resizeGesture.current;
        if (!gesture) return;
        resizeTo(gesture.startWidth + event.clientX - gesture.startX, true);
        resizeGesture.current = null;
      }}
      onPointerCancel={(event) => {
        const gesture = resizeGesture.current;
        if (!gesture) return;
        resizeTo(gesture.startWidth + event.clientX - gesture.startX, true);
        resizeGesture.current = null;
      }}
      sx={{
        display: { xs: 'none', lg: 'block' },
        width: 8,
        flex: '0 0 8px',
        cursor: 'col-resize',
        touchAction: 'none',
        userSelect: 'none',
        borderRadius: 99,
        '&::after': { content: '""', display: 'block', width: 2, height: '100%', mx: 'auto', bgcolor: 'divider' },
        '&:hover::after, &:focus-visible::after': { bgcolor: 'primary.main' },
        '&:focus-visible': { outline: '2px solid', outlineColor: 'primary.main', outlineOffset: -2 },
      }}
    />
    </Box>
  );
}

export function SelectedAssetArtwork({ fileName }: { fileName: string }) {
  return (
    <Box sx={{ position: 'absolute', inset: 0, overflow: 'hidden' }}>
      <Box
        role="img"
        aria-label={`MVForge artwork placeholder for ${fileName}`}
        data-artwork-source="placeholder"
        sx={{
          width: '100%',
          height: '100%',
          display: 'grid',
          placeItems: 'center',
          color: 'text.secondary',
          background: 'linear-gradient(135deg, rgba(35, 48, 77, 0.95), rgba(8, 19, 35, 0.98) 62%, rgba(57, 38, 94, 0.85))',
        }}
      >
        <Stack spacing={0.4} alignItems="center">
          <MovieOutlinedIcon sx={{ fontSize: 42, opacity: 0.75 }} />
          <Typography variant="caption">Artwork unavailable</Typography>
        </Stack>
      </Box>
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
  const [expanded, setExpanded] = useState(true);
  const audioSummary = snapshot
    ? snapshot.audioStreams.slice(0, 2).map((stream) => [stream.codec, stream.channels ? `${stream.channels}ch` : '', stream.language].filter(Boolean).join(' ')).join(' · ') || 'None found'
    : 'Unavailable';
  const metadata = asset.metadata;
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
  const statusValues = [
    ['Estado', status || 'Unavailable'],
    ['Perfil', assignedProfile || 'Unavailable'],
    ['Subtítulos', subtitleValue],
    ['Duración', formatDurationSummary(duration)],
    ['Modificado', modifiedLabel || 'Unavailable'],
  ];
  const quickFacts = [
    ['Category / Library', metadata?.categories?.[0] || asset.libraryName || 'Unavailable'],
    ['Container', container],
    ['Size', sizeLabel || 'Unavailable'],
    ['Resolution', resolution],
    ['Codec', videoCodec],
  ];
  const technicalValues = [
    ['Video codec', videoCodec],
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
      <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1} sx={{ mb: expanded ? 1 : 0 }}>
        <Box sx={{ minWidth: 0 }}>
          <Typography component="h2" variant="subtitle2" fontWeight={700}>Selected Asset Workspace</Typography>
          {!expanded ? <Typography variant="caption" color="text.secondary" noWrap>{asset.fileName}</Typography> : null}
        </Box>
        <IconButton
          size="small"
          aria-label={`${expanded ? 'Collapse' : 'Expand'} selected asset workspace`}
          aria-expanded={expanded}
          aria-controls="selected-asset-workspace-content"
          onClick={() => setExpanded((current) => !current)}
        >
          <ExpandMoreIcon sx={{ transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)', transition: 'transform 120ms ease' }} />
        </IconButton>
      </Stack>
      <Collapse in={expanded} id="selected-asset-workspace-content" unmountOnExit>
      <Stack direction={{ xs: 'column', lg: 'row' }} spacing={{ xs: 1.25, lg: 1.5 }} alignItems={{ xs: 'stretch', lg: 'center' }}>
        <Tooltip title={`Play preview of ${asset.fileName}`}>
          <span>
            <ButtonBase
              onClick={onPlayPreview}
              disabled={asset.missing}
              aria-label={`Play preview of ${asset.fileName}`}
              sx={{
                width: { xs: '100%', sm: 240, lg: 220, xl: 260 },
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
              <SelectedAssetArtwork fileName={asset.fileName} />
              <Box sx={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', bgcolor: 'rgba(0,0,0,0.2)' }}>
                <PlayCircleOutlineIcon fontSize="large" />
              </Box>
              <Typography
                component="span"
                variant="caption"
                fontWeight={700}
                sx={{ position: 'absolute', right: 0.75, bottom: 0.75, px: 0.65, py: 0.15, borderRadius: 0.5, bgcolor: 'rgba(0,0,0,0.78)', color: 'common.white' }}
              >
                {formatDurationSummary(duration)}
              </Typography>
            </ButtonBase>
          </span>
        </Tooltip>

        <Box
          sx={{
            minWidth: 0,
            flex: 1,
            display: 'grid',
            gridTemplateColumns: { xs: 'minmax(0, 1fr)', lg: 'minmax(0, 1.2fr) minmax(220px, 0.8fr)' },
            gap: { xs: 1, lg: 1.25 },
          }}
        >
          <Stack spacing={0.75} sx={{ minWidth: 0 }}>
            <Box sx={{ minWidth: 0 }}>
              <Tooltip title={asset.fileName}>
                <Typography component="h2" fontWeight={700} variant="h2" noWrap sx={{ minWidth: 0, fontSize: { xs: '1rem', sm: '1.1rem' }, lineHeight: 1.25 }}>
                  {asset.fileName}
                </Typography>
              </Tooltip>
              <Typography variant="caption" color="text.secondary" noWrap title={pathContext}>{pathContext}</Typography>
            </Box>
            <Typography variant="caption" fontWeight={700} color="text.secondary" sx={{ mt: 0.2 }}>Quick facts</Typography>
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', sm: 'repeat(3, minmax(0, 1fr))' }, gap: 0.6 }}>
              {quickFacts.map(([label, value]) => (
                <Box key={label} sx={{ minWidth: 0 }}>
                  <Typography component="div" variant="caption" color="text.secondary" noWrap>{label}</Typography>
                  <Typography variant="body2" fontWeight={500} noWrap title={value}>{value}</Typography>
                </Box>
              ))}
            </Box>
            <Divider sx={{ my: 0.25 }} />
            <Typography variant="caption" fontWeight={700} color="text.secondary">Status / context</Typography>
            <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', sm: 'repeat(3, minmax(0, 1fr))' }, gap: 0.6 }}>
              {statusValues.map(([label, value]) => (
                <Box key={label} sx={{ minWidth: 0 }}>
                  <Typography component="div" variant="caption" color="text.secondary" noWrap>{label}</Typography>
                  <Typography variant="body2" fontWeight={500} noWrap title={value}>{value}</Typography>
                </Box>
              ))}
            </Box>
          </Stack>

          <Box
            sx={{
              minWidth: 0,
              borderLeft: { lg: 1 },
              borderTop: { xs: 1, lg: 0 },
              borderColor: 'divider',
              pl: { lg: 1.25 },
              pt: { xs: 1, lg: 0 },
            }}
          >
            <Typography variant="caption" fontWeight={700} color="text.secondary">Technical information</Typography>
            <Grid container columnSpacing={1.25} rowSpacing={0.5} sx={{ mt: 0.3 }}>
              {technicalValues.map(([label, value]) => <DetailValue key={label} label={label} value={value} />)}
            </Grid>
          </Box>
        </Box>

        <Stack direction="row" spacing={0.5} alignItems="center" justifyContent={{ xs: 'flex-start', lg: 'center' }} flexWrap="wrap" useFlexGap sx={{ flexShrink: 0 }}>
          {canAssignProfile ? <Button size="small" variant="outlined" onClick={onAssignProfile}>Assign Profile</Button> : null}
          {canTestEncode ? <Button size="small" variant="contained" color="secondary" onClick={onTestEncode} disabled={testEncodeDisabled}>Test Encode</Button> : null}
          <Tooltip title="More asset actions">
            <IconButton size="small" aria-label={`More actions for ${asset.fileName}`} onClick={(event) => onMore(event.currentTarget)}>
              <MoreVertIcon fontSize="small" />
            </IconButton>
          </Tooltip>
        </Stack>
      </Stack>
      </Collapse>
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
