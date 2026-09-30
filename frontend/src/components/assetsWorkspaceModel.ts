import type { AssetGroup, Library } from '../api/types';

export type LibraryTreeSelection = {
  libraryId: number | null;
  path: string;
};

export type LibraryTreeNode = {
  id: string;
  label: string;
  libraryId: number;
  path: string;
  children: LibraryTreeNode[];
};

export function buildLibraryTree(groups: AssetGroup[], libraries: Library[]) {
  const roots = new Map<number, LibraryTreeNode>();

  for (const library of libraries) {
    roots.set(library.id, {
      id: `library-${library.id}`,
      label: library.name,
      libraryId: library.id,
      path: '',
      children: [],
    });
  }

  for (const group of groups) {
    let root = roots.get(group.libraryId);
    if (!root) {
      root = {
        id: `library-${group.libraryId}`,
        label: group.libraryName || 'Library',
        libraryId: group.libraryId,
        path: '',
        children: [],
      };
      roots.set(group.libraryId, root);
    }

    const libraryRoot = libraries.find((library) => library.id === group.libraryId)?.destinationPath ?? '';
    const relative = (group.relativePath || group.path.split(/[\\/]/).filter(Boolean).at(-1) || '')
      .replace(/\\/g, '/')
      .replace(/^\/+|\/+$/g, '');
    const segments = relative.split('/').filter((segment) => segment && segment !== '.');
    let parent = root;
    const prefix: string[] = [];
    for (const segment of segments) {
      prefix.push(segment);
      let node = parent.children.find((child) => child.label === segment);
      if (!node) {
        const rootPath = libraryRoot.replace(/[\\/]+$/g, '');
        node = {
          id: `${group.libraryId}:${prefix.join('/')}`,
          label: segment,
          libraryId: group.libraryId,
          path: rootPath ? `${rootPath}/${prefix.join('/')}` : group.path,
          children: [],
        };
        parent.children.push(node);
      }
      parent = node;
    }
  }

  const sortNodes = (nodes: LibraryTreeNode[]) => {
    nodes.sort((left, right) => left.label.localeCompare(right.label));
    nodes.forEach((node) => sortNodes(node.children));
  };
  const result = Array.from(roots.values()).sort((left, right) => left.label.localeCompare(right.label));
  result.forEach((root) => sortNodes(root.children));
  return result;
}

export function formatDurationSummary(value?: number) {
  if (value === undefined || !Number.isFinite(value) || value < 0) return 'Unavailable';
  const totalSeconds = Math.floor(value);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  return hours ? `${hours}h ${minutes}m` : minutes ? `${minutes}m ${seconds}s` : `${seconds}s`;
}

export function assetPathWithin(path: string, folder: string) {
  if (!folder) return true;
  const normalizedPath = path.replace(/\\/g, '/').replace(/\/+$/g, '');
  const normalizedFolder = folder.replace(/\\/g, '/').replace(/\/+$/g, '');
  return normalizedPath === normalizedFolder || normalizedPath.startsWith(`${normalizedFolder}/`);
}
