// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { Library, TestEncode } from '../api/types';
import { TestEncodeDialog } from './TestEncodeDialog';

vi.mock('../api/client', () => ({ api: {
  testEncodes: vi.fn(), createTestEncode: vi.fn(), cancelTestEncode: vi.fn(), keepTestEncode: vi.fn(), deleteTestEncode: vi.fn(),
} }));

const readyTest = {
  id: 4, sourceAssetId: 1, sourcePath: '/raw/show/episode.mkv', sourceFingerprint: 'sha256:test', sourceSizeBytes: 100,
  libraryId: 2, configurationSource: 'effective_asset', requestedConfiguration: {}, effectiveConfiguration: {}, configurationHash: 'abc123',
  profileId: 9, profileVersion: 2, workerName: 'nas', effectiveEncoder: 'hevc_qsv', startSeconds: 40, durationSeconds: 20,
  status: 'ready', phase: 'ready', progress: 100, ffmpegCommand: 'ffmpeg -ss 40 -i input -t 20 output',
  outputPath: '/library/episode - MVForge Test T4.mkv', outputSizeBytes: 50, subtitleArtifacts: [], validationReport: { videoTracks: 1 },
  keep: false, errorMessage: '', createdAt: '2026-08-21T00:00:00Z', updatedAt: '2026-08-21T00:00:01Z',
  stale: false,
} satisfies TestEncode;

const testLibraries: Library[] = [{ id: 2, name: 'TV', sourcePath: '/raw', destinationPath: '/library', type: 'tv', validationRules: {}, createdAt: '', updatedAt: '' }];

function renderDialog(configurationSource: 'effective_asset' | 'lab_draft' = 'effective_asset', defaultLibraryId = 2, libraries = testLibraries) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TestEncodeDialog
        open
        onClose={() => undefined}
        sourcePath="/raw/show/episode.mkv"
        libraries={libraries}
        defaultLibraryId={defaultLibraryId}
        request={configurationSource === 'lab_draft'
          ? { configurationSource, labProfile: { name: 'Unsaved', videoCodec: 'x265' } as never, labAudioProfile: { filters: 'anull' }, labTrackOverride: { keepAudioStreams: [1] } }
          : { configurationSource, profileId: 9, resolveAssignments: true }}
      />
    </QueryClientProvider>,
  );
}

describe('TestEncodeDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.testEncodes).mockResolvedValue([]);
    vi.mocked(api.createTestEncode).mockResolvedValue(readyTest);
  });

  it('creates an independent test with the selected window and effective Asset configuration', async () => {
    const user = userEvent.setup();
    renderDialog();
    await user.click(screen.getByRole('combobox', { name: /sample position/i }));
    await user.click(await screen.findByRole('option', { name: /custom/i }));
    await user.type(screen.getByRole('spinbutton', { name: /start seconds/i }), '35');
    await user.click(screen.getByRole('button', { name: /generate new test encode/i }));
    await waitFor(() => expect(api.createTestEncode).toHaveBeenCalledWith(expect.objectContaining({
      sourcePath: '/raw/show/episode.mkv', libraryId: 2, configurationSource: 'effective_asset', profileId: 9,
      resolveAssignments: true, startMode: 'custom', startSeconds: 35, durationSeconds: 20,
    })));
  });

  it('labels unsaved Lab configuration and exposes lifecycle actions for prior tests', async () => {
    vi.mocked(api.testEncodes).mockResolvedValue([readyTest]);
    const user = userEvent.setup();
    renderDialog('lab_draft');
    expect(await screen.findByText(/uses the current lab configuration, including unsaved changes/i)).toBeTruthy();
    expect(await screen.findByText('/library/episode - MVForge Test T4.mkv')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: /details/i }));
    expect(screen.getByRole('tabpanel', { name: /summary/i })).toBeTruthy();
    expect(screen.queryByDisplayValue(/ffmpeg -ss 40/i)).toBeNull();
    await user.click(screen.getByRole('tab', { name: /ffmpeg/i }));
    expect(screen.getByDisplayValue(/ffmpeg -ss 40/i)).toBeTruthy();
    await user.click(screen.getByRole('tab', { name: /validation/i }));
    expect(screen.getByDisplayValue(/"videoTracks": 1/i)).toBeTruthy();
    expect(screen.getByDisplayValue('[]')).toBeTruthy();
  });

  it('keeps requested and effective configurations in a dedicated section', async () => {
    vi.mocked(api.testEncodes).mockResolvedValue([{
      ...readyTest,
      requestedConfiguration: { requestedEncoder: 'hevc_qsv' },
      effectiveConfiguration: { effectiveEncoder: 'hevc_qsv', gopRefDist: 1 },
    }]);
    const user = userEvent.setup();
    renderDialog();
    await user.click(await screen.findByRole('button', { name: /details/i }));
    await user.click(screen.getByRole('tab', { name: /configuration/i }));
    expect(screen.getByDisplayValue(/"requestedEncoder": "hevc_qsv"/i)).toBeTruthy();
    expect(screen.getByDisplayValue(/"gopRefDist": 1/i)).toBeTruthy();
  });

  it('opens without an inherited destination and allows choosing one inside', async () => {
    const user = userEvent.setup();
    renderDialog('effective_asset', 0, [
      ...testLibraries,
      { ...testLibraries[0], id: 3, name: 'Anime', destinationPath: '/library/anime' },
    ]);
    await user.click(screen.getByRole('combobox', { name: /destination library/i }));
    await user.click(await screen.findByRole('option', { name: 'Anime' }));
    await user.click(screen.getByRole('button', { name: /generate new test encode/i }));
    await waitFor(() => expect(api.createTestEncode).toHaveBeenCalledWith(expect.objectContaining({ libraryId: 3 })));
  });
});
