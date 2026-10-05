import {
  Box,
  Breadcrumbs,
  Button,
  ButtonBase,
  Card,
  Chip,
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
import AspectRatioOutlinedIcon from '@mui/icons-material/AspectRatioOutlined';
import InsertDriveFileOutlinedIcon from '@mui/icons-material/InsertDriveFileOutlined';
import MovieOutlinedIcon from '@mui/icons-material/MovieOutlined';
import VolumeUpOutlinedIcon from '@mui/icons-material/VolumeUpOutlined';
import AutorenewOutlinedIcon from '@mui/icons-material/AutorenewOutlined';
import TaskAltOutlinedIcon from '@mui/icons-material/TaskAltOutlined';
import ReportProblemOutlinedIcon from '@mui/icons-material/ReportProblemOutlined';
import { useMemo, useRef, useState, type ReactNode } from 'react';
import { Link as RouterLink } from 'react-router-dom';
import type { Asset, AssetGroup, AssetSourceGroup, Library, RuntimeState, ScanResult } from '../api/types';
import { buildLibraryTree, buildRawHierarchy, formatDurationSummary, type LibraryTreeNode, type LibraryTreeSelection, type RawHierarchyNode, type RawHierarchySelection } from './assetsWorkspaceModel';
import { RuntimeStorageSummary } from './RuntimeStorageSummary';

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
            <Typography variant="caption" color="text.secondary" noWrap sx={{ fontSize: '0.75rem', color: '#9cb0c8' }}>{label}</Typography>
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
export function AssetsHierarchyPanel({ sourceGroups, groups, libraries, location, rawSelection, librarySelection, onSelectRaw, onSelectLibrary, storageSnapshot, storageLoading = false, storageError = false }: {
  sourceGroups: AssetSourceGroup[];
  groups: AssetGroup[];
  libraries: Library[];
  location: 'raw' | 'library';
  rawSelection: RawHierarchySelection | null;
  librarySelection: LibraryTreeSelection;
  onSelectRaw: (selection: RawHierarchySelection) => void;
  onSelectLibrary: (selection: LibraryTreeSelection) => void;
  storageSnapshot?: RuntimeState;
  storageLoading?: boolean;
  storageError?: boolean;
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
            color: isRawRoot || isLibrarySection ? '#b1c2d6' : '#7b91a9',
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
          {isRawRoot ? <Inventory2OutlinedIcon sx={{ mr: 0.65, fontSize: 16, color: '#a9bfd5' }} /> : isLibrarySection || isSelected || isExpanded ? <FolderOpenOutlinedIcon sx={{ mr: 0.65, fontSize: 16, color: isSelected ? '#b6aaff' : '#a9bfd5' }} /> : <FolderOutlinedIcon sx={{ mr: 0.65, fontSize: 16, color: '#7188a1' }} />}
          <ListItemText
            primary={node.label}
            primaryTypographyProps={{ variant: 'body2', noWrap: true, title: node.label, fontWeight: isRawRoot || isLibrarySection ? 700 : undefined, sx: { fontSize: '0.75rem' } }}
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
        width: { xs: '100%', md: `${hierarchyWidth + 8}px` },
        minWidth: 0,
        maxWidth: '100%',
        height: { xs: 'auto', md: '100%' },
        flex: { xs: '0 0 auto', md: `0 0 ${hierarchyWidth + 8}px` },
      }}
    >
    <Box
      component="nav"
      id="assets-hierarchy-navigation"
      aria-label="Asset Library"
      sx={{
        width: { xs: '100%', md: `${hierarchyWidth}px` },
        minWidth: { xs: 0, md: `${ASSETS_HIERARCHY_MIN_WIDTH}px` },
        maxWidth: { md: `${ASSETS_HIERARCHY_MAX_WIDTH}px` },
        flex: { xs: '1 1 auto', md: `0 0 ${hierarchyWidth}px` },
        alignSelf: 'stretch',
        height: { xs: 'auto', md: '100%' },
        maxHeight: { xs: 180, md: 'none' },
        minHeight: 0,
        display: 'flex',
        flexDirection: 'column',
        border: 1,
        borderColor: '#1b3a56',
        borderRadius: 1,
        bgcolor: '#081a2b',
        px: 1,
        pt: 1.25,
        pb: 0.75,
      }}
    >
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ px: 0.4, pb: 0.75 }}>
        <Typography variant="subtitle2" fontWeight={700} sx={{ color: '#9cb0c8', fontSize: '0.75rem', letterSpacing: '0.08em', textTransform: 'uppercase' }}>Asset Library</Typography>
        <Tooltip title="Configure libraries">
        <IconButton component={RouterLink} to="/libraries" size="small" aria-label="Configure libraries" sx={{ width: 28, height: 28, border: 1, borderColor: '#274763', borderRadius: 1, color: '#a9bfd5', bgcolor: '#0c2035', '&:hover': { bgcolor: '#112a44', borderColor: '#426888' } }}>
            <AddIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      </Stack>
      <List role="tree" aria-label="Asset Library" dense disablePadding sx={{ flex: 1, minHeight: 0, overflowY: 'auto' }}>
        {nodes.map((node) => renderNode(node, 1))}
      </List>
      <RuntimeStorageSummary snapshot={storageSnapshot} loading={storageLoading} error={storageError} compact />
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
        display: { xs: 'none', md: 'block' },
        width: 8,
        flex: '0 0 8px',
        cursor: 'col-resize',
        touchAction: 'none',
        userSelect: 'none',
        borderRadius: 99,
          '&::after': { content: '""', display: 'block', width: 2, height: '100%', mx: 'auto', bgcolor: '#1b3a56' },
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
        <Typography
          variant="caption"
          sx={{ position: 'absolute', left: 0.7, bottom: 0.65, px: 0.6, py: 0.25, borderRadius: 0.5, bgcolor: 'rgba(5, 13, 24, 0.72)', color: '#9aa9bb', fontSize: '0.58rem' }}
        >
          Artwork unavailable
        </Typography>
      </Box>
    </Box>
  );
}

export function SelectedAssetDetail({
  asset,
  pathContext,
  categoryLabel,
  containerLabel,
  snapshot,
  snapshotLoading,
  statusLabel,
  statusColor,
  assignedProfileLabel,
  modifiedLabel,
  sizeLabel,
  assignProfileUnavailableReason,
  testEncodeUnavailableReason,
  onPlayPreview,
  onAssignProfile,
  onTestEncode,
  onMore,
  canAssignProfile,
  canTestEncode,
}: {
  asset: Asset;
  pathContext: string;
  categoryLabel: string;
  containerLabel: string;
  snapshot?: ScanResult | null;
  snapshotLoading: boolean;
  statusLabel: string;
  statusColor: 'default' | 'primary' | 'success' | 'warning' | 'error';
  assignedProfileLabel: string;
  modifiedLabel: string;
  sizeLabel: string;
  assignProfileUnavailableReason?: string;
  testEncodeUnavailableReason?: string;
  onPlayPreview: () => void;
  onAssignProfile: () => void;
  onTestEncode: () => void;
  onMore: (anchor: HTMLElement) => void;
  canAssignProfile: boolean;
  canTestEncode: boolean;
}) {
  const [expanded, setExpanded] = useState(true);
  const duration = snapshot?.duration ?? asset.technical?.duration;
  const videoCodec = snapshot?.videoCodec || asset.technical?.videoCodec;
  const width = snapshot?.width ?? asset.technical?.width;
  const height = snapshot?.height ?? asset.technical?.height;
  const resolutionSummary = width !== undefined && width !== null && height !== undefined && height !== null
    ? width + ' × ' + height
    : 'Unavailable';
  const audioStreams = snapshot?.audioStreams ?? [];
  const audioCodecs = [...new Set(audioStreams.map((stream) => {
    const layout = stream.channelLayout || (stream.channels ? stream.channels + ' ch' : '');
    return [stream.codec, layout].filter(Boolean).join(' ');
  }).filter(Boolean))];
  const audioSummary = snapshotLoading
    ? 'Loading…'
    : !snapshot
      ? 'Unavailable'
      : audioCodecs.length
        ? audioCodecs.slice(0, 2).join(' · ') + (audioCodecs.length > 2 ? ' +' + (audioCodecs.length - 2) : '')
        : '0 embedded tracks';
  const subtitleSummary = snapshotLoading
    ? 'Loading…'
    : !snapshot
      ? 'Unavailable'
      : snapshot.subtitleStreams.length
        ? snapshot.subtitleStreams.length + ' embedded tracks'
        : '0 embedded tracks';
  const stateValues = [
    ['Profile', assignedProfileLabel || 'Unavailable'],
    ['Subtitles', subtitleSummary],
    ['Duration', formatDurationSummary(duration)],
    ['Modified', modifiedLabel || 'Unavailable'],
  ];
  const mediaValues = [
    { label: 'Video', value: videoCodec || 'Unavailable', icon: <MovieOutlinedIcon sx={{ fontSize: 12, color: '#778fa9', flexShrink: 0 }} /> },
    { label: 'Resolution', value: resolutionSummary, icon: <AspectRatioOutlinedIcon sx={{ fontSize: 12, color: '#778fa9', flexShrink: 0 }} /> },
    { label: 'Audio', value: audioSummary, icon: <VolumeUpOutlinedIcon sx={{ fontSize: 12, color: '#778fa9', flexShrink: 0 }} /> },
  ];
  const quickFacts = [
    { label: 'Category / Library', value: categoryLabel || 'Unavailable', icon: <FolderOutlinedIcon sx={{ fontSize: 13, color: '#7289a5', flexShrink: 0 }} /> },
    { label: 'Container', value: containerLabel || 'Unavailable', icon: <InsertDriveFileOutlinedIcon sx={{ fontSize: 13, color: '#7289a5', flexShrink: 0 }} /> },
    { label: 'Size', value: sizeLabel || 'Unavailable', icon: <StorageOutlinedIcon sx={{ fontSize: 13, color: '#7289a5', flexShrink: 0 }} /> },
    { label: 'Resolution', value: resolutionSummary, icon: <AspectRatioOutlinedIcon sx={{ fontSize: 13, color: '#7289a5', flexShrink: 0 }} /> },
    { label: 'Codec', value: videoCodec || 'Unavailable', icon: <MovieOutlinedIcon sx={{ fontSize: 13, color: '#7289a5', flexShrink: 0 }} /> },
  ];
  const assignProfileFallbackReason = statusLabel === 'Published'
    ? 'Profile assignment is unavailable for assets with a Published lifecycle state.'
    : 'Unavailable under this asset’s current lifecycle or capability.';
  const testEncodeFallbackReason = statusLabel === 'Published'
    ? 'Test Encode is unavailable for assets with a Published lifecycle state.'
    : 'Unavailable under this asset’s current lifecycle or capability.';

  return (
    <Box
      component="section"
      aria-label={"Details for " + asset.fileName}
      data-testid="selected-asset-detail"
      sx={{
        px: { xs: 0.65, sm: 0.9 },
        py: { xs: 0.35, sm: 0.45 },
        borderTop: 1,
        borderColor: '#1d3b56',
        background: 'linear-gradient(105deg, #091a2a 0%, #0a1d30 100%)',
        position: 'sticky',
        bottom: 0,
        zIndex: 2,
        flexShrink: 0,
        minHeight: expanded ? undefined : 42,
      }}
    >
      {!expanded ? (
        <Stack direction="row" alignItems="center" spacing={1}>
          <Box sx={{ minWidth: 0, flex: 1 }}>
            <Typography variant="body2" fontWeight={700} noWrap title={asset.fileName} sx={{ fontSize: '0.78rem' }}>
              {asset.fileName}
            </Typography>
            <Typography variant="caption" color="text.secondary" noWrap title={pathContext} sx={{ fontSize: '0.75rem' }}>
              {pathContext}
            </Typography>
          </Box>
          <IconButton
            size="small"
            aria-label={"Expand selected asset details for " + asset.fileName}
            aria-expanded={false}
            aria-controls="selected-asset-workspace-content"
            onClick={() => setExpanded(true)}
          >
            <ExpandMoreIcon />
          </IconButton>
        </Stack>
      ) : (
        <Collapse in id="selected-asset-workspace-content">
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: {
                xs: '64px minmax(0, 1fr)',
                sm: '132px minmax(0, 1fr)',
                md: 'minmax(110px, 15%) minmax(0, 1fr) minmax(240px, 1.05fr)',
                lg: 'minmax(130px, 19%) minmax(0, 1.7fr) minmax(270px, 1fr) minmax(240px, 1fr)',
              },
              alignItems: 'center',
              columnGap: { xs: 0.7, sm: 1, md: 0, lg: 0 },
              rowGap: { xs: 0.7, md: 0.65, lg: 0 },
              minWidth: 0,
              position: 'relative',
              pr: { xs: 2.4, sm: 2.8 },
            }}
          >
            <Tooltip title={"Play preview of " + asset.fileName}>
              <Box component="span" sx={{ display: 'grid', placeItems: 'center', minWidth: 0 }}>
                <ButtonBase
                  data-testid="selected-asset-preview"
                  onClick={onPlayPreview}
                  disabled={asset.missing}
                  aria-label={"Play preview of " + asset.fileName}
                  sx={{
                    width: '100%',
                    maxWidth: { xs: 64, sm: 132, md: 290 },
                    aspectRatio: '16 / 10',
                    height: 'auto',
                    flexShrink: 0,
                    borderRadius: '4px',
                    bgcolor: '#061321',
                    color: '#e6f2ff',
                    overflow: 'hidden',
                    position: 'relative',
                    boxShadow: 'inset 0 0 0 1px #29445f',
                    '&:hover': { bgcolor: '#13283d' },
                    '&:focus-visible': { outline: '2px solid', outlineColor: 'primary.main', outlineOffset: 2 },
                  }}
                >
                  <SelectedAssetArtwork fileName={asset.fileName} />
                  <Box sx={{ position: 'absolute', inset: 0, display: 'grid', placeItems: 'center', bgcolor: 'rgba(0,0,0,0.18)' }}>
                    <PlayCircleOutlineIcon sx={{ fontSize: { xs: 26, sm: 34 }, filter: 'drop-shadow(0 2px 8px rgba(0,0,0,.7))' }} />
                  </Box>
                  <Typography
                    component="span"
                    variant="caption"
                    fontWeight={700}
                    sx={{ position: 'absolute', right: 0.45, bottom: 0.45, px: 0.5, py: 0.08, borderRadius: 0.45, bgcolor: 'rgba(0,0,0,0.82)', color: 'common.white', fontSize: '0.75rem' }}
                  >
                    {formatDurationSummary(duration)}
                  </Typography>
                </ButtonBase>
              </Box>
            </Tooltip>

            <Box
              data-testid="selected-asset-information"
              sx={{ display: 'flex', flexDirection: 'column', justifyContent: 'center', rowGap: { xs: 0.55, sm: 0.75 }, minWidth: 0, px: { xs: 0, md: 1.3 }, py: { xs: 0, md: 0.55 } }}
            >
              <Box data-testid="selected-asset-identity" sx={{ minWidth: 0, gridColumn: { xs: '2', md: 'auto' }, px: { xs: 0, md: 1.2 }, py: { xs: 0, md: 0.3 } }}>
                  <Tooltip title={asset.fileName}>
                    <Typography
                      component="h2"
                      fontWeight={700}
                      sx={{
                        minWidth: 0,
                        fontSize: { xs: '0.8rem', sm: '0.86rem', xl: '0.9rem' },
                        lineHeight: 1.16,
                        color: '#e5edf8',
                        whiteSpace: 'normal',
                        overflowWrap: 'anywhere',
                        overflow: 'hidden',
                        display: '-webkit-box',
                        WebkitBoxOrient: 'vertical',
                        WebkitLineClamp: 2,
                      }}
                    >
                      {asset.fileName}
                    </Typography>
                  </Tooltip>
                  <Typography data-testid="selected-asset-path-context" variant="caption" color="text.secondary" noWrap title={pathContext} sx={{ display: 'block', fontSize: { xs: '0.75rem', sm: '0.75rem' }, color: '#8ca1b8', mt: 0.2 }}>
                    {pathContext}
                  </Typography>
                </Box>
                <Box
                  data-testid="selected-asset-quick-facts"
                  role="group"
                  aria-label="Category, container, size, resolution, and codec"
                  sx={{ display: 'flex', alignItems: 'center', flexWrap: 'wrap', columnGap: { xs: 0.6, sm: 0.8 }, rowGap: 0.3, minWidth: 0, mt: 0.45 }}
                >
                  {quickFacts.map(({ label, value, icon }) => (
                    <Tooltip key={label} title={`${label}: ${value}`}>
                      <Stack direction="row" alignItems="center" spacing={0.3} aria-label={`${label}: ${value}`} sx={{ minWidth: 0, color: '#93a8c0' }}>
                        {icon}
                        <Typography variant="caption" noWrap sx={{ color: '#aabbd0', fontSize: '0.8rem', lineHeight: 1.2 }}>{value}</Typography>
                      </Stack>
                    </Tooltip>
                  ))}
                </Box>
            </Box>
              <Box
                data-testid="selected-asset-context"
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  justifyContent: 'center',
                  rowGap: 0.55,
                  minWidth: 0,
                  gridColumn: { xs: '1 / -1', md: 'auto' },
                  px: { xs: 0.2, md: 1.2 },
                  py: { xs: 0.5, md: 0.3 },
                  borderTop: { xs: 1, md: 0 },
                  borderLeft: { xs: 0, md: 1 },
                  borderColor: '#1e3c57',
                }}
              >
                <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={0.65}>
                  <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.75rem', color: '#8299b2' }}>Status</Typography>
                  <Chip
                    data-testid="selected-asset-status"
                    label={statusLabel || 'Unavailable'}
                    aria-label={"Status: " + (statusLabel || 'Unavailable')}
                    color={statusColor}
                    size="small"
                    variant="outlined"
                    sx={{ height: 17, maxWidth: '72%', fontSize: '0.75rem', '& .MuiChip-label': { px: 0.5, overflow: 'hidden', textOverflow: 'ellipsis' } }}
                  />
                </Stack>
                <Box data-testid="selected-asset-state-context" sx={{ display: 'grid', gridTemplateColumns: 'minmax(0, 1fr)', rowGap: 0.38 }}>
                  {stateValues.map(([label, value]) => <DetailValue key={label} label={label} value={value} inline />)}
                </Box>
              </Box>

              <Box
                data-testid="selected-asset-technical-actions"
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  justifyContent: 'space-between',
                  alignSelf: 'stretch',
                  rowGap: 0.55,
                  minWidth: 0,
                  gridColumn: { xs: '1 / -1', md: '2 / 4', lg: 'auto' },
                  px: { xs: 0.2, md: 1.2 },
                  py: { xs: 0.5, md: 0.3 },
                  borderTop: { xs: 1, md: 0 },
                  borderLeft: { xs: 0, md: 1 },
                  borderColor: '#1e3c57',
                }}
              >
                <Box data-testid="selected-asset-technical" sx={{ minWidth: 0 }}>
                  <Typography variant="caption" sx={{ display: 'block', color: '#8ea5be', fontSize: '0.75rem', fontWeight: 700, lineHeight: 1.1, mb: 0.35 }}>
                    Technical information
                  </Typography>
                  <Stack spacing={0.42} sx={{ minWidth: 0 }}>
                    {mediaValues.map(({ label, value, icon }) => <DetailValue key={label} label={label} value={value} inline icon={icon} technical />)}
                  </Stack>
                </Box>

                <Stack
                  data-testid="selected-asset-actions"
                  role="group"
                  aria-label="Selected asset actions"
                  direction="row"
                  spacing={0.5}
                  alignItems="center"
                  justifyContent="space-between"
                  useFlexGap
                  flexWrap="wrap"
                  sx={{ minWidth: 0, width: '100%' }}
                >
                  <Stack direction="row" spacing={0.5} alignItems="center">
                  <Tooltip title={canAssignProfile ? '' : assignProfileUnavailableReason || assignProfileFallbackReason} describeChild>
                    <span tabIndex={canAssignProfile ? undefined : 0} aria-label={canAssignProfile ? undefined : assignProfileUnavailableReason || assignProfileFallbackReason}>
                      <Button size="small" variant="outlined" onClick={onAssignProfile} disabled={!canAssignProfile} sx={{ minHeight: 28, px: 0.85, borderRadius: '4px', textTransform: 'none', fontSize: '0.8rem', whiteSpace: 'nowrap', borderColor: '#4656a5', bgcolor: 'rgba(80, 84, 255, 0.11)', color: '#c6ccff', '&:hover': { borderColor: '#7280df', bgcolor: 'rgba(80, 84, 255, 0.18)' }, '&.Mui-disabled': { bgcolor: 'rgba(82, 104, 129, 0.08)', color: '#71849b', borderColor: 'rgba(82, 104, 129, 0.2)' } }}>
                        Assign Profile
                      </Button>
                    </span>
                  </Tooltip>
                  <Tooltip title={canTestEncode ? '' : testEncodeUnavailableReason || testEncodeFallbackReason} describeChild>
                    <span tabIndex={canTestEncode ? undefined : 0} aria-label={canTestEncode ? undefined : testEncodeUnavailableReason || testEncodeFallbackReason}>
                      <Button size="small" variant="outlined" onClick={onTestEncode} disabled={!canTestEncode} sx={{ minHeight: 28, px: 0.85, borderRadius: '4px', textTransform: 'none', fontSize: '0.8rem', whiteSpace: 'nowrap', borderColor: '#30465f', color: '#b4c4d7', bgcolor: 'rgba(13, 31, 49, 0.68)', '&:hover': { borderColor: '#58718c', bgcolor: '#122b43' }, '&.Mui-disabled': { bgcolor: 'rgba(82, 104, 129, 0.05)', borderColor: 'rgba(82, 104, 129, 0.18)', color: '#71849b' } }}>
                        Test Encode
                      </Button>
                    </span>
                  </Tooltip>
                  </Stack>
                  <Tooltip title="More actions">
                    <IconButton size="small" onClick={(event) => onMore(event.currentTarget)} aria-label={"More actions for " + asset.fileName} sx={{ width: 28, height: 28, border: 1, borderColor: '#30465f', borderRadius: '4px', color: '#9fb1c6', bgcolor: 'rgba(13, 31, 49, 0.68)', '&:hover': { bgcolor: '#122b43', borderColor: '#58718c' } }}>
                      <MoreVertIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </Stack>
              </Box>
            <IconButton
              size="small"
              aria-label={"Collapse selected asset details for " + asset.fileName}
              aria-expanded
              aria-controls="selected-asset-workspace-content"
              onClick={() => setExpanded(false)}
              sx={{ position: 'absolute', top: -2, right: -3, width: 22, height: 22, p: 0.1, borderRadius: '4px', color: '#71849b', '&:hover': { bgcolor: '#1b2a3c', color: '#c7d2e2' } }}
            >
              <ExpandMoreIcon sx={{ transform: 'rotate(180deg)', fontSize: 18 }} />
            </IconButton>
          </Box>
        </Collapse>
      )}
    </Box>
  );
}
function DetailValue({ label, value, inline = false, icon, technical = false }: { label: string; value: string; inline?: boolean; icon?: ReactNode; technical?: boolean }) {
  return (
    <Box sx={{ minWidth: 0, display: inline ? 'flex' : 'block', alignItems: 'baseline', justifyContent: inline ? 'space-between' : undefined, gap: inline ? 0.42 : 0 }} title={`${label}: ${value}`}>
      {icon}
      <Typography component="div" variant="caption" color="text.secondary" noWrap sx={{ fontSize: '0.75rem', lineHeight: 1.15, color: '#8398b0', flexShrink: 0 }}>{label}</Typography>
      <Typography variant="body2" fontWeight={500} noWrap sx={{ minWidth: 0, maxWidth: '100%', color: technical ? '#bfcee0' : '#d0dbe8', fontSize: '0.8rem', lineHeight: 1.2, fontFamily: technical ? 'ui-monospace, SFMono-Regular, Menlo, monospace' : undefined }}>{value}</Typography>
    </Box>
  );
}
