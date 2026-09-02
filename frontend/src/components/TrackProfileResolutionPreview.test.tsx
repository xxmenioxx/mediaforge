// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import type { ScanResult, TrackProfileResolutionPreview as ResolutionPreview } from '../api/types';
import { TrackProfileResolutionPreview } from './TrackProfileResolutionPreview';

afterEach(cleanup);

describe('TrackProfileResolutionPreview', () => {
  it('renders per-stream Path decisions and reasons without editable controls', () => {
    const scan = {
      videoStreams: [{ index: 0, codec: 'mpeg2video', language: '', title: '' }],
      audioStreams: [
        { index: 1, codec: 'ac3', language: 'jpn', title: '' },
        { index: 2, codec: 'ac3', language: 'eng', title: 'Commentary' },
      ],
      subtitleStreams: [{ index: 4, codec: 'ass', language: 'spa', title: '' }],
      attachmentStreams: [],
      attachmentInventoryAvailable: true,
    } as unknown as ScanResult;
    const preview: ResolutionPreview = {
      assetPath: '/media/Arbegas 21.mkv', keepVideoStreams: [0], keepAudioStreams: [1], keepSubtitleStreams: [4], warnings: [],
      video: [{ index: 0, type: 'video', kept: true, reason: 'first_video' }],
      audio: [
        { index: 1, type: 'audio', kept: true, reason: 'language_match' },
        { index: 2, type: 'audio', kept: false, reason: 'commentary' },
      ],
      subtitle: [{ index: 4, type: 'subtitle', kept: true, reason: 'language_match' }],
      resolvedTrackPlan: {
        videoStreams: [{ streamIndex: 0 }], audioStreams: [{ streamIndex: 1 }], removedAudioStreams: [{ streamIndex: 2 }],
        subtitleStreams: [{ streamIndex: 4, action: 'keep_and_extract' }], attachmentPolicy: 'auto', attachmentsKept: false,
        attachmentReason: 'no embedded ASS/SSA subtitles remain', attachmentStreams: [], fontAttachmentsExported: false,
        chapterPolicy: 'keep', chaptersKept: true,
        sidecarOutputs: [
          { streamIndex: 4, format: 'ass', mode: 'original', forced: false, default: false },
          { streamIndex: 4, format: 'srt', mode: 'converted', forced: false, default: false },
        ],
      },
    };

    render(<TrackProfileResolutionPreview scan={scan} preview={preview} loading={false} error={null} />);

    expect(screen.getByText(/#1 · AC3 · JPN/i)).toBeTruthy();
    expect(screen.getByText(/commentary rule/i)).toBeTruthy();
    expect(screen.getAllByText('Keep')).toHaveLength(2);
    expect(screen.getByText('Keep + Extract')).toBeTruthy();
    expect(screen.getByText('Remove')).toBeTruthy();
    expect(screen.getByText(/#4 · ASS · Original format/i)).toBeTruthy();
    expect(screen.getByText(/#4 · SRT · Compatibility conversion/i)).toBeTruthy();
    expect(screen.queryByRole('checkbox')).toBeNull();
    expect(screen.getByText('No source attachments.')).toBeTruthy();
  });

  it('renders canonical font, image, unknown, and missing-filename attachments', () => {
    const scan = {
      videoStreams: [], audioStreams: [], subtitleStreams: [],
      attachmentStreams: [
        { index: 7, type: 'attachment', codec: 'ttf', filename: 'CustomFont-Regular.ttf', mimeType: 'application/x-truetype-font', attachmentKind: 'FONT', fontFormat: 'TTF' },
      ],
      attachmentInventoryAvailable: true,
    } as unknown as ScanResult;
    const preview = attachmentPreview(true, [
      { streamIndex: 7, codec: 'ttf', filename: 'CustomFont-Regular.ttf', mimeType: 'application/x-truetype-font', attachmentKind: 'FONT', fontFormat: 'TTF' },
      { streamIndex: 8, codec: 'otf', filename: 'Display.otf', attachmentKind: 'FONT', fontFormat: 'OTF' },
      { streamIndex: 9, codec: 'ttc', filename: 'Collection.ttc', attachmentKind: 'FONT', fontFormat: 'TTC' },
      { streamIndex: 10, codec: 'otc', filename: 'Collection.otc', attachmentKind: 'FONT', fontFormat: 'OTC' },
      { streamIndex: 11, codec: 'mjpeg', filename: 'fake.otf', mimeType: 'image/jpeg', attachmentKind: 'IMAGE' },
      { streamIndex: 12, codec: 'png', filename: 'fake.ttf', mimeType: 'image/png', attachmentKind: 'IMAGE' },
      { streamIndex: 13, codec: 'bin_data', filename: 'payload.bin', attachmentKind: 'ATTACHMENT' },
      { streamIndex: 14, codec: 'bin_data', attachmentKind: 'ATTACHMENT' },
    ]);

    render(<TrackProfileResolutionPreview scan={scan} preview={preview} loading={false} error={null} />);

    expect(screen.getByText('#7 · CustomFont-Regular.ttf')).toBeTruthy();
    expect(screen.getByText('FONT · TTF')).toBeTruthy();
    expect(screen.getByText('FONT · OTF')).toBeTruthy();
    expect(screen.getByText('FONT · TTC')).toBeTruthy();
    expect(screen.getByText('FONT · OTC')).toBeTruthy();
    expect(screen.getByText('IMAGE · JPEG')).toBeTruthy();
    expect(screen.getByText('IMAGE · PNG')).toBeTruthy();
    expect(screen.getAllByText('ATTACHMENT · BIN_DATA')).toHaveLength(2);
    expect(screen.getByText('#14 · Attachment #14')).toBeTruthy();
    expect(screen.getByText('MIME: application/x-truetype-font')).toBeTruthy();
    expect(screen.getAllByText('Final MKV: Keep')).toHaveLength(8);
  });

  it('shows aggregate Remove disposition while retaining attachment visibility', () => {
    const scan = { videoStreams: [], audioStreams: [], subtitleStreams: [], attachmentStreams: [], attachmentInventoryAvailable: true } as unknown as ScanResult;
    const preview = attachmentPreview(false, [
      { streamIndex: 9, codec: 'mjpeg', filename: 'cover.jpg', mimeType: 'image/jpeg', attachmentKind: 'IMAGE' },
    ]);

    render(<TrackProfileResolutionPreview scan={scan} preview={preview} loading={false} error={null} />);

    expect(screen.getByText('#9 · cover.jpg')).toBeTruthy();
    expect(screen.getByText('Final MKV: Remove')).toBeTruthy();
  });

  it('uses explicit availability when the persisted collection reloads empty', () => {
    const scan = { videoStreams: [], audioStreams: [], subtitleStreams: [], attachmentStreams: [], attachmentInventoryAvailable: false } as unknown as ScanResult;
    const preview = attachmentPreview(true, []);

    render(<TrackProfileResolutionPreview scan={scan} preview={preview} loading={false} error={null} />);

    expect(screen.getByText('Attachment metadata unavailable.')).toBeTruthy();
    expect(screen.queryByText('No source attachments.')).toBeNull();
  });

  it('handles legacy null attachment arrays without crashing', () => {
    const scan = { videoStreams: [], audioStreams: [], subtitleStreams: [], attachmentStreams: null, attachmentInventoryAvailable: false } as unknown as ScanResult;
    const preview = attachmentPreview(true, []) as ResolutionPreview;
    preview.resolvedTrackPlan.attachmentStreams = null as unknown as ResolutionPreview['resolvedTrackPlan']['attachmentStreams'];

    render(<TrackProfileResolutionPreview scan={scan} preview={preview} loading={false} error={null} />);

    expect(screen.getByText('Attachments')).toBeTruthy();
    expect(screen.getByText('Attachment metadata unavailable.')).toBeTruthy();
  });
});

function attachmentPreview(
  kept: boolean,
  attachments: ResolutionPreview['resolvedTrackPlan']['attachmentStreams'],
): ResolutionPreview {
  return {
    assetPath: '/media/movie.mkv', keepVideoStreams: [], keepAudioStreams: [], keepSubtitleStreams: [], warnings: [],
    video: [], audio: [], subtitle: [],
    resolvedTrackPlan: {
      videoStreams: [], audioStreams: [], removedAudioStreams: [], subtitleStreams: [],
      attachmentPolicy: kept ? 'keep' : 'remove', attachmentsKept: kept,
      attachmentReason: kept ? 'attachments explicitly kept' : 'attachments explicitly removed',
      attachmentStreams: attachments, fontAttachmentsExported: false,
      chapterPolicy: 'keep', chaptersKept: true, sidecarOutputs: [],
    },
  };
}
