// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import type { ScanResult } from '../api/types';
import { MediaSnapshotDetails } from './MediaSnapshotDetails';

afterEach(cleanup);

function scanWithColor(colorPolicy: ScanResult['colorPolicy'], transfer?: string): ScanResult {
  return {
    videoStreams: [{
      index: 0,
      type: 'video',
      codec: 'h264',
      codecLong: 'H.264',
      profile: 'High',
      language: 'und',
      title: '',
      duration: 60,
      bitrate: 1_000_000,
      default: true,
      forced: false,
      comment: false,
      hearingImpaired: false,
      colorSpace: 'bt709',
      colorPrimaries: 'bt709',
      colorTransfer: transfer,
      colorRange: 'tv',
    }],
    audioStreams: [],
    subtitleStreams: [],
    attachmentStreams: [],
    colorPolicy,
    rawProbe: {},
  } as unknown as ScanResult;
}

describe('MediaSnapshotDetails color metadata', () => {
  it('renders complete backend color metadata without inventing a decision', () => {
    render(<MediaSnapshotDetails scan={scanWithColor({ status: 'complete', action: 'none' }, 'bt709')} section="general" />);

    expect(screen.getByText('Color characteristics')).toBeTruthy();
    expect(screen.getByText('Complete')).toBeTruthy();
    expect(screen.getByText('No review required')).toBeTruthy();
    expect(screen.getAllByText('bt709').length).toBeGreaterThanOrEqual(3);
  });

  it('keeps missing metadata Unknown and shows the backend review reason', () => {
    render(<MediaSnapshotDetails scan={scanWithColor({
      status: 'incomplete_unsupported',
      action: 'review',
      reason: 'Transfer characteristics are missing.',
      warning: 'Transfer characteristics are missing. Automatic color normalization was not applied.',
    })} section="general" />);

    expect(screen.getByText('Incomplete / unsupported')).toBeTruthy();
    expect(screen.getByText('Review required')).toBeTruthy();
    expect(screen.getByText('Transfer characteristics are missing.')).toBeTruthy();
    expect(screen.getByText('Transfer characteristics are missing. Automatic color normalization was not applied.')).toBeTruthy();
    expect(screen.getAllByText('Unknown').length).toBeGreaterThan(0);
  });

  it('renders no-video scans as not applicable without a color review warning', () => {
    const scan = {
      videoStreams: [],
      audioStreams: [],
      subtitleStreams: [],
      attachmentStreams: [],
      colorPolicy: { status: 'not_applicable', action: 'none' },
      rawProbe: {},
    } as unknown as ScanResult;

    render(<MediaSnapshotDetails scan={scan} section="general" />);

    expect(screen.getByText('Metadata status').parentElement?.textContent).toContain('Not applicable');
    expect(screen.queryByText('Review required')).toBeNull();
    expect(screen.queryByRole('alert')).toBeNull();
  });
});
