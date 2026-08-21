// @vitest-environment jsdom

import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import type { ScanResult, TrackProfileResolutionPreview as ResolutionPreview } from '../api/types';
import { TrackProfileResolutionPreview } from './TrackProfileResolutionPreview';

describe('TrackProfileResolutionPreview', () => {
  it('renders per-stream Path decisions and reasons without editable controls', () => {
    const scan = {
      videoStreams: [{ index: 0, codec: 'mpeg2video', language: '', title: '' }],
      audioStreams: [
        { index: 1, codec: 'ac3', language: 'jpn', title: '' },
        { index: 2, codec: 'ac3', language: 'eng', title: 'Commentary' },
      ],
      subtitleStreams: [{ index: 4, codec: 'hdmv_pgs_subtitle', language: 'spa', title: '' }],
    } as ScanResult;
    const preview: ResolutionPreview = {
      assetPath: '/media/Arbegas 21.mkv', keepVideoStreams: [0], keepAudioStreams: [1], keepSubtitleStreams: [4], warnings: [],
      video: [{ index: 0, type: 'video', kept: true, reason: 'first_video' }],
      audio: [
        { index: 1, type: 'audio', kept: true, reason: 'language_match' },
        { index: 2, type: 'audio', kept: false, reason: 'commentary' },
      ],
      subtitle: [{ index: 4, type: 'subtitle', kept: true, reason: 'language_match' }],
    };

    render(<TrackProfileResolutionPreview scan={scan} preview={preview} loading={false} error={null} />);

    expect(screen.getByText(/#1 · AC3 · JPN/i)).toBeTruthy();
    expect(screen.getByText(/commentary rule/i)).toBeTruthy();
    expect(screen.getAllByText('Keep')).toHaveLength(3);
    expect(screen.getByText('Remove')).toBeTruthy();
    expect(screen.queryByRole('checkbox')).toBeNull();
  });
});
