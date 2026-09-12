// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { LibraryRenamePlan } from '../api/types';
import { LibraryPathRenameDialog } from './LibraryPathRenameDialog';

vi.mock('../api/client', () => ({ api: {
  previewLibraryPathRename: vi.fn(),
  applyLibraryPathRename: vi.fn(),
} }));

const plan: LibraryRenamePlan = {
  libraryId: 4,
  sourcePath: '/media/library/series/Doctor Who/Season2',
  targetPath: '/media/library/series/Doctor Who/Season 02',
  assets: [{
    assetId: 11,
    sourcePath: '/media/library/series/Doctor Who/Season2/old.mkv',
    targetPath: '/media/library/series/Doctor Who/Season 02/Doctor Who - S02E01.mkv',
    sidecars: [{
      sourcePath: '/media/library/series/Doctor Who/Season2/old.eng.srt',
      targetPath: '/media/library/series/Doctor Who/Season 02/Doctor Who - S02E01.eng.srt',
    }],
  }],
  warnings: [],
  conflicts: [],
  planHash: 'sha256:reviewed-plan',
};

function renderDialog(overrides: Partial<LibraryRenamePlan> = {}) {
  const onClose = vi.fn();
  const onApplied = vi.fn();
  vi.mocked(api.previewLibraryPathRename).mockResolvedValue({ ...plan, ...overrides });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={client}>
      <LibraryPathRenameDialog
        open
        libraryId={4}
        path={plan.sourcePath}
        onClose={onClose}
        onApplied={onApplied}
      />
    </QueryClientProvider>,
  );
  return { onClose, onApplied };
}

describe('LibraryPathRenameDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.applyLibraryPathRename).mockResolvedValue({ status: 'applied', plan });
  });

  it('shows the backend plan and applies the reviewed planHash', async () => {
    const user = userEvent.setup();
    const { onClose, onApplied } = renderDialog();

    expect(await screen.findByText(plan.targetPath)).toBeTruthy();
    expect(screen.getByText(plan.assets[0].targetPath)).toBeTruthy();
    expect(screen.getByText(new RegExp(`${plan.assets[0].sidecars[0].sourcePath}.*${plan.assets[0].sidecars[0].targetPath}`))).toBeTruthy();
    await user.click(screen.getByRole('button', { name: /apply rename/i }));

    await waitFor(() => expect(api.applyLibraryPathRename).toHaveBeenCalledWith({
      libraryId: 4,
      path: plan.sourcePath,
      planHash: plan.planHash,
    }));
    await waitFor(() => expect(onApplied).toHaveBeenCalledTimes(1));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('shows conflicts and prevents Apply', async () => {
    renderDialog({ conflicts: [{ code: 'target_exists', sourcePath: plan.assets[0].sourcePath, targetPath: plan.assets[0].targetPath }] });

    expect(await screen.findByText(/target_exists/)).toBeTruthy();
    expect((screen.getByRole('button', { name: /apply rename/i }) as HTMLButtonElement).disabled).toBe(true);
    expect(api.applyLibraryPathRename).not.toHaveBeenCalled();
  });
});
