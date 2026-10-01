// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Asset, ScanResult } from '../api/types';
import { LabV2Header } from './LabV2Header';

const asset: Asset = {
  id: 12,
  libraryId: 2,
  libraryName: 'Anime',
  path: '/media/raw/Anime/S01E01.mkv',
  relativePath: 'Anime/S01E01.mkv',
  groupPath: '/media/raw/Anime',
  fileName: 'S01E01.mkv',
  extension: 'mkv',
  sizeBytes: 1_400_000_000,
  modifiedAt: '2026-09-01T00:00:00Z',
  status: 'unprocessed',
  missing: false,
  review: {} as Asset['review'],
  metadata: {} as Asset['metadata'],
  conversion: {} as Asset['conversion'],
};

const snapshot = {
  path: asset.path,
  fileName: asset.fileName,
  container: 'matroska,webm',
  sizeBytes: asset.sizeBytes,
  duration: 1450,
  bitrate: 9_824_000,
  videoCodec: 'h264',
  width: 1920,
  height: 1080,
} as ScanResult;

function renderHeader(overrides: Partial<React.ComponentProps<typeof LabV2Header>> = {}) {
  const props: React.ComponentProps<typeof LabV2Header> = {
    asset,
    snapshot,
    assetPicker: <div>Asset picker</div>,
    scope: 'asset',
    onScopeChange: vi.fn(),
    start: '600',
    seconds: 20,
    rangeValid: true,
    onStartChange: vi.fn(),
    onSecondsChange: vi.fn(),
    runDisabled: false,
    onRun: vi.fn(),
    onOpenAssetInfo: vi.fn(),
    onRestoreProfile: vi.fn(),
    onOpenProfileDetails: vi.fn(),
    ...overrides,
  };
  return { ...render(<LabV2Header {...props} />), props };
}

describe('LabV2Header', () => {
  afterEach(cleanup);

  it('shows compact asset context, one scope control, one range, and one primary action', () => {
    renderHeader();

    expect(screen.getByText('S01E01.mkv')).toBeTruthy();
    expect(screen.getByText('/media/raw/Anime')).toBeTruthy();
    expect(screen.getByText('00:10:00 → 00:10:20')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Path' }).getAttribute('aria-pressed')).toBe('false');
    expect(screen.getByRole('button', { name: 'Asset' }).getAttribute('aria-pressed')).toBe('true');
    expect(screen.getAllByRole('button', { name: 'Run Test Encode' })).toHaveLength(1);
    expect(screen.queryByText(/Profile applies to/i)).toBeNull();
  });

  it('uses the existing draft scope and lets users edit the active range in LAB', () => {
    const { props } = renderHeader();
    fireEvent.click(screen.getByRole('button', { name: 'Path' }));
    expect(props.onScopeChange).toHaveBeenCalledWith('path');

    fireEvent.click(screen.getByRole('button', { name: /edit test range/i }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Test range start time' }), { target: { value: '00:12:00' } });
    fireEvent.change(screen.getByRole('spinbutton', { name: 'Test range duration in seconds' }), { target: { value: '30' } });
    expect(props.onStartChange).toHaveBeenCalledWith('00:12:00');
    expect(props.onSecondsChange).toHaveBeenCalledWith(30);
  });

  it('keeps supported secondary actions in the overflow menu', () => {
    const { props } = renderHeader();
    fireEvent.click(screen.getByRole('button', { name: 'More LAB actions' }));
    fireEvent.click(screen.getByRole('menuitem', { name: /open asset info/i }));
    expect(props.onOpenAssetInfo).toHaveBeenCalledOnce();
    expect(screen.queryByRole('button', { name: /duplicate test|export results|save preset/i })).toBeNull();
  });
});
