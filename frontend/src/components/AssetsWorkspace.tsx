import {
  Box,
  Breadcrumbs,
  Button,
  ButtonBase,
  Card,
  Collapse,
  IconButton,
  List,
  ListItemButton,
  ListItemText,
  Stack,
  Tooltip,
  Typography,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import PlayCircleOutlineIcon from '@mui/icons-material/PlayCircleOutline';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import FolderOutlinedIcon from '@mui/icons-material/FolderOutlined';
import FolderOpenOutlinedIcon from '@mui/icons-material/FolderOpenOutlined';
import Inventory2OutlinedIcon from '@mui/icons-material/Inventory2Outlined';
import StorageOutlinedIcon from '@mui/icons-material/StorageOutlined';
import AutorenewOutlinedIcon from '@mui/icons-material/AutorenewOutlined';
import TaskAltOutlinedIcon from '@mui/icons-material/TaskAltOutlined';
import ReportProblemOutlinedIcon from '@mui/icons-material/ReportProblemOutlined';
import MovieOutlinedIcon from '@mui/icons-material/MovieOutlined';
import { useMemo, useRef, useState, type ReactNode } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import type { Asset, AssetGroup, AssetSourceGroup, Library, ScanResult } from '../api/types';
import { buildLibraryTree, buildRawHierarchy, formatDurationSummary, type LibraryTreeNode, type LibraryTreeSelection, type RawHierarchyNode, type RawHierarchySelection } from './assetsWorkspaceModel';

export const ASSETS_HIERARCHY_WIDTH_STORAGE_KEY = 'mvforge.assets.hierarchyWidth';
export const ASSETS_HIERARCHY_MIN_WIDTH = 220;
export const ASSETS_HIERARCHY_DEFAULT_WIDTH = 262;
export const ASSETS_HIERARCHY_MAX_WIDTH = 390;

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
    { label: 'Total Assets', value: total, icon: <StorageOutlinedIcon />, color: '#55a0ff', tint: '#0b3152' },
    { label: 'Processing', value: processing, icon: <AutorenewOutlinedIcon />, color: '#a78bfa', tint: '#27175d' },
    { label: 'Completed', value: completed, icon: <TaskAltOutlinedIcon />, color: '#00d99b', tint: '#073d35' },
    { label: 'Needs Review', value: needsReview, icon: <ReportProblemOutlinedIcon />, color: '#ff4e6a', tint: '#481827' },
  ] as const;

  return (
    <Box
      component="section"
      aria-label="Asset summary"
      sx={{
        display: 'grid',
        gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', md: 'repeat(4, minmax(0, 1fr))' },
        gap: { xs: 0.75, sm: 1.5 },
      }}
    >
      {items.map(({ label, value, icon, color, tint }) => (
        <Card
          key={label}
          variant="outlined"
          sx={{
            display: 'flex',
            minWidth: 0,
            minHeight: { xs: 68, sm: 96 },
            alignItems: 'center',
            gap: { xs: 0.85, sm: 1.5 },
            px: { xs: 0.9, sm: 1.6 },
            py: 1.1,
            borderRadius: 1.25,
            bgcolor: 'background.paper',
            borderColor: 'divider',
            background: 'linear-gradient(135deg, #091a2c, rgba(9, 26, 44, 0.72))',
            boxShadow: '0 8px 24px rgba(0, 0, 0, 0.12)',
          }}
        >
          <Box aria-hidden="true" sx={{
            display: 'grid',
            placeItems: 'center',
            width: { xs: 34, sm: 42 },
            height: { xs: 34, sm: 42 },
            flexShrink: 0,
            borderRadius: 1,
            bgcolor: tint,
            color,
            '& svg': { fontSize: { xs: 19, sm: 22 } },
          }}>
            {icon}
          </Box>
          <Stack spacing={0.15} sx={{ minWidth: 0 }}>
            <Typography variant="caption" color="text.secondary" noWrap sx={{ fontSize: '0.68rem', color: '#9cb0c8' }}>{label}</Typography>
            <Typography variant="h2" fontWeight={700} aria-label={`${label}: ${value}`} sx={{ fontSize: { xs: '1.1rem', sm: '1.3rem' }, lineHeight: 1.1, color: '#e7eef8' }}>
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
  | { kind: 'library'; id: string; label: string; selection: LibraryTreeSelection; children: AssetsHierarchyNode[] }
  | { kind: 'library-section'; id: string; label: string; children: AssetsHierarchyNode[] };

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
  if (node.kind === 'library-section') return false;
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
  }, {
    kind: 'library-section',
    id: 'assets-library-section',
    label: 'Libraries',
    children: libraryTreeNodes(buildLibraryTree(groups, libraries)),
  }], [sourceGroups, groups, libraries]);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set(['assets-library-section']));
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
    const isLibrarySection = node.kind === 'library-section';
    const hasChildren = node.children.length > 0;
    const isExpanded = expandedItems.has(node.id);
    const isSelected = (location === 'raw' && node.kind === 'raw' && hierarchySelectionMatches(node, rawSelection, librarySelection))
      || (location === 'library' && node.kind === 'library' && hierarchySelectionMatches(node, rawSelection, librarySelection));
    const selectNode = () => {
      if (isLibrarySection) {
        toggleExpanded(node.id);
        return;
      }
      setExpanded((current) => new Set([...current, ...selectedAncestors]));
      setCollapsed(new Set());
      if (node.kind === 'raw') onSelectRaw(node.selection);
      else if (node.kind === 'library') onSelectLibrary(node.selection);
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
          aria-label={node.label}
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
            minHeight: isRawRoot || isLibrarySection ? 35 : 31,
            borderRadius: 1,
            borderLeft: '2px solid transparent',
            py: 0.2,
            pl: Math.min(0.8 + (level - 1) * 1.2, 6),
            pr: 0.65,
            fontWeight: isRawRoot || isLibrarySection ? 700 : 400,
            color: isRawRoot || isLibrarySection ? '#9cb0c8' : '#59718b',
            '&[aria-selected="true"]': { background: 'linear-gradient(90deg, rgba(58, 43, 164, 0.56), rgba(34, 41, 96, 0.36))', color: '#e7eef8', borderColor: 'rgba(95, 75, 247, 0.48)', borderLeftColor: '#6c4cf6' },
            '&:hover': { bgcolor: 'rgba(17, 42, 68, 0.72)', color: '#e7eef8' },
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
          {isRawRoot ? <Inventory2OutlinedIcon sx={{ mr: 0.65, fontSize: 16, color: '#9cb0c8' }} /> : isLibrarySection || isSelected || isExpanded ? <FolderOpenOutlinedIcon sx={{ mr: 0.65, fontSize: 16, color: isSelected ? '#a99aff' : '#9cb0c8' }} /> : <FolderOutlinedIcon sx={{ mr: 0.65, fontSize: 16, color: '#59718b' }} />}
          <ListItemText
            primary={node.label}
            primaryTypographyProps={{ variant: 'body2', noWrap: true, title: node.label, fontWeight: isRawRoot || isLibrarySection ? 700 : undefined, sx: { fontSize: '0.72rem' } }}
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
        height: { xs: 'auto', lg: '100%' },
        flex: { xs: '0 0 auto', lg: `0 0 ${hierarchyWidth + 8}px` },
      }}
    >
    <Box
      component="nav"
      id="assets-hierarchy-navigation"
      aria-label="Asset Library"
      sx={{
        width: { xs: '100%', lg: `${hierarchyWidth}px` },
        minWidth: { xs: 0, lg: `${ASSETS_HIERARCHY_MIN_WIDTH}px` },
        maxWidth: { lg: `${ASSETS_HIERARCHY_MAX_WIDTH}px` },
        flex: { xs: '1 1 auto', lg: `0 0 ${hierarchyWidth}px` },
        alignSelf: 'stretch',
        height: { xs: 'auto', lg: '100%' },
        maxHeight: { xs: 180, lg: 'none' },
        minHeight: 0,
        display: 'flex',
        flexDirection: 'column',
        border: 1,
        borderColor: 'divider',
        borderRadius: 1,
        bgcolor: 'background.paper',
        px: 1,
        pt: 1.25,
        pb: 0.75,
      }}
    >
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ px: 0.4, pb: 0.75 }}>
        <Typography variant="subtitle2" fontWeight={700} sx={{ color: '#9cb0c8', fontSize: '0.72rem', letterSpacing: '0.08em', textTransform: 'uppercase' }}>Asset Library</Typography>
        <Tooltip title="Configure libraries">
          <IconButton component={RouterLink} to="/libraries" size="small" aria-label="Configure libraries" sx={{ width: 28, height: 28, border: 1, borderColor: 'divider', borderRadius: 1, color: '#9cb0c8', bgcolor: '#0c2035', '&:hover': { bgcolor: '#112a44', borderColor: '#1f4467' } }}>
            <AddIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      </Stack>
      <List role="tree" aria-label="Asset Library" dense disablePadding sx={{ flex: 1, minHeight: 0, overflowY: { xs: 'auto', lg: 'auto' } }}>
        {nodes.map((node) => renderNode(node, 1))}
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
        mt: 0.55,
        p: { xs: 0.7, sm: 0.85 },
        border: 1,
        borderColor: 'rgba(39, 75, 108, 0.72)',
        borderRadius: 1,
        background: 'linear-gradient(115deg, rgba(8, 25, 43, 0.98), rgba(8, 20, 35, 0.98))',
        boxShadow: '0 -4px 18px rgba(0, 0, 0, 0.22)',
        position: 'sticky',
        bottom: 0,
        zIndex: 2,
        flexShrink: 0,
        minHeight: 188,
      }}
    >
      {!expanded ? (
        <Stack direction="row" alignItems="center" spacing={1}>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography variant="body2" fontWeight={700} noWrap title={asset.fileName} sx={{ fontSize: '0.78rem' }}>
              {asset.fileName}
            </Typography>
            <Typography variant="caption" color="text.secondary" noWrap title={pathContext} sx={{ fontSize: '0.66rem' }}>
              {pathContext}
            </Typography>
          </Box>
          <IconButton
            size="small"
            aria-label={`Expand selected asset workspace for ${asset.fileName}`}
            aria-expanded={false}
            aria-controls="selected-asset-workspace-content"
            onClick={() => setExpanded(true)}
          >
            <ExpandMoreIcon sx={{ transform: 'rotate(0deg)' }} />
          </IconButton>
        </Stack>
      ) : (
        <Collapse in id="selected-asset-workspace-content">
      <Box
        sx={{
          display: 'grid',
          gridTemplateColumns: {
            xs: '92px minmax(0, 1fr)',
            sm: '150px minmax(0, 1.2fr) minmax(210px, 1fr)',
            lg: 'minmax(205px, 0.98fr) minmax(225px, 1.15fr) minmax(225px, 1.08fr) minmax(205px, 0.95fr)',
          },
          alignItems: 'stretch',
          columnGap: { xs: 0.85, sm: 1, lg: 1.15 },
          rowGap: 0.65,
          minWidth: 0,
          position: 'relative',
          pr: { xs: 0, sm: 0.2 },
        }}
      >
        <Tooltip title={`Play preview of ${asset.fileName}`}>
          <span>
            <ButtonBase
              data-testid="selected-asset-preview"
              onClick={onPlayPreview}
              disabled={asset.missing}
              aria-label={`Play preview of ${asset.fileName}`}
              sx={{
                width: '100%',
                maxWidth: { xs: 92, sm: 150, lg: 276 },
                aspectRatio: '16 / 9',
                height: 'auto',
                flexShrink: 0,
                borderRadius: 0.8,
                bgcolor: 'rgba(14, 35, 57, 0.95)',
                color: '#e6f2ff',
                overflow: 'hidden',
                position: 'relative',
                boxShadow: 'inset 0 0 0 1px rgba(85, 135, 177, 0.26)',
                '&:hover': { bgcolor: 'rgba(79,179,255,0.14)' },
                '&:focus-visible': { outline: '2px solid', outlineColor: 'primary.main', outlineOffset: 2 },
              }}
            >
              <SelectedAssetArtwork fileName={asset.fileName} />
              <Box sx={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', bgcolor: 'rgba(0,0,0,0.2)' }}>
                <PlayCircleOutlineIcon sx={{ fontSize: { xs: 24, sm: 30 }, filter: 'drop-shadow(0 2px 8px rgba(0,0,0,.7))' }} />
              </Box>
              <Typography
                component="span"
                variant="caption"
                fontWeight={700}
                sx={{ position: 'absolute', right: 0.45, bottom: 0.45, px: 0.5, py: 0.08, borderRadius: 0.45, bgcolor: 'rgba(0,0,0,0.82)', color: 'common.white', fontSize: '0.61rem' }}
              >
                {formatDurationSummary(duration)}
              </Typography>
            </ButtonBase>
          </span>
        </Tooltip>

        <Box data-testid="selected-asset-identity" sx={{ minWidth: 0, gridColumn: { xs: '2', sm: '2', lg: 'auto' }, alignSelf: 'center', py: { lg: 0.25 } }}>
          <Tooltip title={asset.fileName}>
            <Typography component="h2" fontWeight={700} noWrap sx={{ minWidth: 0, fontSize: { xs: '0.76rem', sm: '0.82rem' }, lineHeight: 1.2, color: '#f0f6ff' }}>
              {asset.fileName}
            </Typography>
          </Tooltip>
          <Typography variant="caption" color="text.secondary" noWrap title={pathContext} sx={{ display: 'block', mt: 0.2, fontSize: '0.63rem', color: '#91a9c0' }}>
            {pathContext}
          </Typography>
          <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', sm: 'repeat(3, minmax(0, 1fr))', lg: 'repeat(2, minmax(0, 1fr))' }, columnGap: 0.8, rowGap: 0.48, mt: 0.85 }}>
            {quickFacts.map(([label, value]) => <DetailValue key={label} label={label} value={value} />)}
          </Box>
        </Box>

        <Box
          data-testid="selected-asset-status"
          sx={{
            minWidth: 0,
            gridColumn: { xs: '1 / -1', sm: '3', lg: 'auto' },
            gridRow: { sm: '1 / span 2', lg: 'auto' },
            borderLeft: { sm: 1, lg: 1 },
            borderTop: { xs: 1, sm: 0 },
            borderColor: 'rgba(39, 75, 108, 0.72)',
            pl: { xs: 0, sm: 1 },
            pt: { xs: 0.55, sm: 0 },
            alignSelf: 'center',
          }}
        >
          <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'repeat(2, minmax(0, 1fr))', sm: 'repeat(2, minmax(0, 1fr))', lg: 'repeat(2, minmax(0, 1fr))' }, columnGap: 0.8, rowGap: 0.42 }}>
            {statusValues.map(([label, value]) => <DetailValue key={label} label={label} value={value} />)}
          </Box>
        </Box>

        <Box data-testid="selected-asset-technical" sx={{ minWidth: 0, gridColumn: { xs: '1 / -1', sm: '1 / -1', lg: 'auto' }, borderLeft: { lg: 1 }, borderTop: { xs: 1, sm: 1, lg: 0 }, borderColor: 'rgba(39, 75, 108, 0.72)', pl: { xs: 0, lg: 1 }, pt: { xs: 0.55, sm: 0.55, lg: 0 }, alignSelf: 'center' }}>
          <Typography variant="caption" color="text.secondary" sx={{ display: 'block', fontSize: '0.62rem', fontWeight: 700, letterSpacing: '0.035em', mb: 0.35 }}>
            Información técnica
          </Typography>
          <Stack spacing={0.25}>
            {technicalValues.map(([label, value]) => <DetailValue key={label} label={label} value={value} inline />)}
          </Stack>
          <Stack direction="row" spacing={0.35} alignItems="center" sx={{ mt: 0.7, pr: { xs: 0, lg: 3 } }}>
            {canAssignProfile ? <Button size="small" variant="outlined" onClick={onAssignProfile} sx={{ minHeight: 26, px: 0.7, fontSize: '0.62rem', whiteSpace: 'nowrap', borderColor: 'rgba(115, 140, 178, .55)' }}>Assign Profile</Button> : null}
            {canTestEncode ? <Button size="small" variant="contained" color="secondary" onClick={onTestEncode} disabled={testEncodeDisabled} sx={{ minHeight: 26, px: 0.7, fontSize: '0.62rem', whiteSpace: 'nowrap', boxShadow: 'none' }}>Test Encode</Button> : null}
            <Tooltip title="More asset actions">
              <IconButton size="small" aria-label={`More actions for ${asset.fileName}`} onClick={(event) => onMore(event.currentTarget)} sx={{ color: '#adc5de' }}>
                <MoreVertIcon fontSize="small" />
              </IconButton>
            </Tooltip>
          </Stack>
        </Box>
      </Box>
      <IconButton
        size="small"
        aria-label={`Collapse selected asset workspace for ${asset.fileName}`}
        aria-expanded
        aria-controls="selected-asset-workspace-content"
        onClick={() => setExpanded(false)}
        sx={{ position: 'absolute', top: 2, right: 2, width: 23, height: 23, color: '#91a9c0' }}
      >
        <ExpandMoreIcon sx={{ transform: 'rotate(180deg)', fontSize: 18 }} />
      </IconButton>
        </Collapse>
      )}
    </Box>
  );
}

function DetailValue({ label, value, inline = false }: { label: string; value: string; inline?: boolean }) {
  return (
    <Box sx={{ minWidth: 0, display: inline ? 'flex' : 'block', alignItems: 'baseline', gap: inline ? 0.55 : 0 }} title={`${label}: ${value}`}>
      <Typography component="div" variant="caption" color="text.secondary" noWrap sx={{ fontSize: '0.59rem', lineHeight: 1.1, flexShrink: 0 }}>{label}</Typography>
      <Typography variant="body2" fontWeight={500} noWrap sx={{ fontSize: '0.66rem', lineHeight: 1.2, color: '#dce8f4' }}>{value}</Typography>
    </Box>
  );
}
