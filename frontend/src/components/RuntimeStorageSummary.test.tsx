// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import type { RuntimeState } from '../api/types';
import { RuntimeStorageSummary } from './RuntimeStorageSummary';

afterEach(cleanup);

function renderSummary(disks: Record<string, unknown>) {
  const snapshot = { disks } as unknown as RuntimeState;
  return render(<RuntimeStorageSummary snapshot={snapshot} loading={false} error={false} compact />);
}

describe('RuntimeStorageSummary', () => {
  it('shows a shared Library and Workspace volume only once', () => {
    renderSummary({
      library: { filesystems: [{ volumeId: 'device-1', measuredPath: '/mnt/media', paths: ['/mnt/library/a'], totalBytes: 1000, availableBytes: 400 }] },
      workspace: { filesystems: [{ volumeId: 'device-1', measuredPath: '/mnt/media', paths: ['/mnt/work'], totalBytes: 1000, availableBytes: 400 }] },
    });

    expect(screen.getAllByTestId('storage-filesystem')).toHaveLength(1);
    expect(screen.getByText('Library · Workspace')).toBeTruthy();
    expect(screen.getByText(/filesystem capacity/i)).toBeTruthy();
  });

  it('keeps capacities from distinct Library and Workspace volumes separate', () => {
    renderSummary({
      library: { filesystems: [{ volumeId: 'library-device', measuredPath: '/vol/library', paths: ['/media/library/anime', '/media/library/movies'], totalBytes: 2000, availableBytes: 1000 }] },
      workspace: { filesystems: [{ volumeId: 'work-device', measuredPath: '/vol/work', paths: ['/media/work'], totalBytes: 4000, availableBytes: 3000 }] },
    });

    expect(screen.getAllByTestId('storage-filesystem')).toHaveLength(2);
    expect(screen.getByText('Library')).toBeTruthy();
    expect(screen.getByText('Workspace')).toBeTruthy();
    expect(screen.queryByText(/\/vol\/library/)).toBeNull();
    expect(screen.queryByText(/\/vol\/work/)).toBeNull();
  });

  it('shows a configured destination as unavailable without capacity values', () => {
    renderSummary({
      library: {
        filesystems: [],
        destinations: [{ path: '/missing/library', accessible: false, error: 'path does not exist' }],
      },
    });

    expect(screen.getByTestId('storage-destination-unavailable')).toBeTruthy();
    expect(screen.getByText('Library')).toBeTruthy();
    expect(screen.queryByText(/\/missing\/library/)).toBeNull();
    expect(screen.getByText('Unavailable')).toBeTruthy();
    expect(screen.queryByText(/used · .* free/)).toBeNull();
  });
});
