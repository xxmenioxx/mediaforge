// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { SubtitleExtractionOperation, SubtitleTranslationCapability, SubtitleTranslationSourceSelector } from '../api/types';
import { SubtitleTranslationDialog } from './SubtitleTranslationDialog';

vi.mock('../api/client', () => ({
  api: {
    subtitleTranslationCapability: vi.fn(),
    startSubtitleTranslation: vi.fn(),
    subtitleOperation: vi.fn(),
    cancelSubtitleOperation: vi.fn(),
    retrySubtitleTranslation: vi.fn(),
  },
}));

afterEach(cleanup);

const source: SubtitleTranslationSourceSelector = {
  type: 'embedded' as const,
  embedded: { streamIndex: 4 },
};

function capability(overrides: Partial<SubtitleTranslationCapability> = {}): SubtitleTranslationCapability {
  return {
    source: {
      type: 'embedded',
      sourceIdentity: 'embedded:4',
      sourceRevision: 'revision-1',
      representation: 'ass',
      streamIndex: 4,
      streamId: 'stream-4',
      codec: 'ass',
      language: 'eng',
      title: 'English forced',
      default: false,
      sourceForced: true,
      effectiveForced: true,
      role: 'forced',
    },
    eligible: true,
    available: true,
    sourceLanguages: ['auto', 'en', 'ja'],
    targetLocales: ['es-419', 'es-ES'],
    provider: { name: 'ollama', model: 'llama3', modelSelectable: true },
    outputRepresentations: ['ass'],
    context: { status: 'unavailable' },
    ...overrides,
  };
}

function operation(overrides: Partial<SubtitleExtractionOperation> = {}): SubtitleExtractionOperation {
  return {
    id: 'translation-1',
    assetPath: '/media/episode.mkv',
    status: 'running',
    phase: 'translating',
    progress: 45,
    processed: 3,
    total: 8,
    streamIndex: -1,
    format: 'srt',
    kind: 'translation',
    translation: {
      targetLocale: 'es-419',
      outputRepresentation: 'srt',
      retryable: false,
      cancellable: true,
    },
    createdAt: '2026-09-20T00:00:00Z',
    updatedAt: '2026-09-20T00:00:01Z',
    ...overrides,
  };
}

function renderDialog(selectedSource = source, queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })) {
  return render(
    <QueryClientProvider client={queryClient}>
      <SubtitleTranslationDialog
        open
        path="/media/episode.mkv"
        source={selectedSource}
        onClose={() => undefined}
      />
    </QueryClientProvider>,
  );
}

describe('SubtitleTranslationDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability());
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({ operation: operation() });
    vi.mocked(api.subtitleOperation).mockResolvedValue(operation());
    vi.mocked(api.cancelSubtitleOperation).mockResolvedValue(operation({ status: 'cancelled', phase: 'cancelled', translation: { targetLocale: 'es-419', outputRepresentation: 'srt', retryable: true, cancellable: false } }));
    vi.mocked(api.retrySubtitleTranslation).mockResolvedValue({ operation: operation({ id: 'translation-2' }) });
  });

  it('requests capability for the exact embedded stream selector', async () => {
    renderDialog();

    await waitFor(() => expect(api.subtitleTranslationCapability).toHaveBeenCalledWith({
      path: '/media/episode.mkv',
      source: { type: 'embedded', embedded: { streamIndex: 4 } },
    }));
  });

  it('requests capability for the exact external sidecar path', async () => {
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({
      source: {
        ...capability().source,
        type: 'external',
        representation: 'srt',
        externalPath: '/media/episode.eng.forced.srt',
      },
    }));
    renderDialog({ type: 'external', external: { subtitlePath: '/media/episode.eng.forced.srt' } });

    await waitFor(() => expect(api.subtitleTranslationCapability).toHaveBeenCalledWith({
      path: '/media/episode.mkv',
      source: { type: 'external', external: { subtitlePath: '/media/episode.eng.forced.srt' } },
    }));
  });

  it('keeps an eligible source visible but disables Start with the backend reason', async () => {
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({
      available: false,
      reason: 'Translation provider configuration must be tested.',
    }));
    renderDialog();

    expect(await screen.findByText('Translation provider configuration must be tested.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', true);
    expect(api.startSubtitleTranslation).not.toHaveBeenCalled();
  });

  it('uses the single backend output and sends only the approved start fields', async () => {
    const user = userEvent.setup();
    renderDialog();

    await waitFor(() => expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', false));
    expect(screen.queryByRole('combobox', { name: 'Output format' })).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Start translation' }));

    await waitFor(() => expect(api.startSubtitleTranslation).toHaveBeenCalled());
    expect(vi.mocked(api.startSubtitleTranslation).mock.calls[0]?.[0]).toEqual({
      path: '/media/episode.mkv',
      source,
      sourceLanguage: 'auto',
      targetLocale: 'es-419',
      outputRepresentation: 'ass',
    });
    expect(Object.keys(vi.mocked(api.startSubtitleTranslation).mock.calls[0]?.[0] ?? {})).toEqual([
      'path',
      'source',
      'sourceLanguage',
      'targetLocale',
      'outputRepresentation',
    ]);
  });

  it('renders exactly the backend-provided multiple output representations', async () => {
    const user = userEvent.setup();
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({
      outputRepresentations: ['srt', 'ass', 'both'],
      context: { status: 'available', source: 'configured metadata' },
    }));
    renderDialog();

    const output = await screen.findByRole('combobox', { name: 'Output format' });
    await user.click(output);
    expect(await screen.findByRole('option', { name: 'SRT' })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'ASS' })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'BOTH' })).toBeTruthy();
    expect(screen.getByText('Media context: available · configured metadata')).toBeTruthy();
  });

  it('polls a started operation through progress and stops after completion', async () => {
    const user = userEvent.setup();
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({
      operation: operation({ id: 'translation-1', progress: 45, processed: 3, total: 8 }),
    });
    vi.mocked(api.subtitleOperation)
      .mockResolvedValueOnce(operation({ phase: 'validating', progress: 85, processed: 8, total: 8 }))
      .mockResolvedValueOnce(operation({
        status: 'completed',
        phase: 'completed',
        progress: 100,
        processed: 8,
        total: 8,
        translation: { targetLocale: 'es-419', outputRepresentation: 'srt', retryable: false, cancellable: false },
      }));
    renderDialog();

    await waitFor(() => expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', false));
    await user.click(screen.getByRole('button', { name: 'Start translation' }));
    expect(await screen.findByText('Validating · 8 / 8')).toBeTruthy();
    expect(await screen.findByText('Translation completed.')).toBeTruthy();
    expect(api.subtitleOperation).toHaveBeenCalledTimes(2);
  }, 10000);

  it('cancels through the generic subtitle operation endpoint and distinguishes cancellation', async () => {
    const user = userEvent.setup();
    renderDialog();

    await waitFor(() => expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', false));
    await user.click(screen.getByRole('button', { name: 'Start translation' }));
    await user.click(await screen.findByRole('button', { name: 'Cancel translation' }));

    expect(api.cancelSubtitleOperation).toHaveBeenCalledWith('translation-1');
    expect(await screen.findByText('Translation cancelled.')).toBeTruthy();
    expect(screen.queryByText('Subtitle translation failed.')).toBeNull();
    expect(screen.getByRole('button', { name: 'Retry translation' })).toBeTruthy();
  });

  it('retries a terminal operation with a new ID and never starts a second operation', async () => {
    const user = userEvent.setup();
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({
      operation: operation({
        status: 'error',
        phase: 'error',
        error: 'bounded failure',
        translation: { failureClass: 'provider_unavailable', diagnostic: 'Provider unavailable.', retryable: true, cancellable: false },
      }),
    });
    vi.mocked(api.retrySubtitleTranslation).mockResolvedValue({
      operation: operation({ id: 'translation-2', phase: 'translating', progress: 45 }),
    });
    vi.mocked(api.subtitleOperation)
      .mockResolvedValueOnce(operation({
        status: 'error',
        phase: 'error',
        error: 'bounded failure',
        translation: { failureClass: 'provider_unavailable', diagnostic: 'Provider unavailable.', retryable: true, cancellable: false },
      }))
      .mockResolvedValueOnce(operation({ id: 'translation-2' }));
    renderDialog();

    await waitFor(() => expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', false));
    await user.click(screen.getByRole('button', { name: 'Start translation' }));
    expect(await screen.findByText(/provider_unavailable: Provider unavailable\./)).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Retry translation' }));

    expect(api.retrySubtitleTranslation).toHaveBeenCalledWith('translation-1');
    expect(api.startSubtitleTranslation).toHaveBeenCalledTimes(1);
    expect(await screen.findByText('Translating · 3 / 8')).toBeTruthy();
    expect(api.subtitleOperation).toHaveBeenCalledWith('translation-2');
  });

  it('keeps the predecessor visible when retry is rejected', async () => {
    const user = userEvent.setup();
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({
      operation: operation({
        status: 'error',
        phase: 'error',
        translation: { failureClass: 'stale_source', diagnostic: 'Subtitle source changed.', retryable: true, cancellable: false },
      }),
    });
    vi.mocked(api.retrySubtitleTranslation).mockRejectedValue(new Error('Exact subtitle source is unavailable.'));
    vi.mocked(api.subtitleOperation).mockResolvedValue(operation({
      status: 'error',
      phase: 'error',
      translation: { failureClass: 'stale_source', diagnostic: 'Subtitle source changed.', retryable: true, cancellable: false },
    }));
    renderDialog();

    await waitFor(() => expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', false));
    await user.click(screen.getByRole('button', { name: 'Start translation' }));
    await user.click(await screen.findByRole('button', { name: 'Retry translation' }));

    expect(await screen.findByText('Exact subtitle source is unavailable.')).toBeTruthy();
    expect(screen.getByText(/stale_source: Subtitle source changed\./)).toBeTruthy();
  });

  it('renders translated result artifacts and invalidates external subtitle inventory once', async () => {
    const user = userEvent.setup();
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const invalidateQueries = vi.spyOn(queryClient, 'invalidateQueries');
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({
      operation: operation({ id: 'translation-1', status: 'completed', phase: 'completed', progress: 100, translation: { targetLocale: 'es-419', outputRepresentation: 'both', retryable: false, cancellable: false }, result: {
        created: [], existing: [], unsupported: [], artifacts: [
          { artifactId: 'translation-srt', type: 'subtitle_sidecar', streamIndex: -1, sourceCodec: 'srt', sourceType: 'text', format: 'srt', mode: 'translation', publishedPath: '/media/episode.es-419.srt', stagedPath: '', sizeBytes: 10, status: 'published', translation: {} as never },
          { artifactId: 'translation-ass', type: 'subtitle_sidecar', streamIndex: -1, sourceCodec: 'ass', sourceType: 'text', format: 'ass', mode: 'translation', publishedPath: '/media/episode.es-419.ass', stagedPath: '', sizeBytes: 10, status: 'published', translation: {} as never },
        ],
      } }),
    });
    vi.mocked(api.subtitleOperation).mockResolvedValue(operation({
      id: 'translation-1',
      status: 'completed',
      phase: 'completed',
      progress: 100,
      translation: { targetLocale: 'es-419', outputRepresentation: 'both', retryable: false, cancellable: false },
      result: {
        created: [], existing: [], unsupported: [], artifacts: [
          { artifactId: 'translation-srt', type: 'subtitle_sidecar', streamIndex: -1, sourceCodec: 'srt', sourceType: 'text', format: 'srt', mode: 'translation', publishedPath: '/media/episode.es-419.srt', stagedPath: '', sizeBytes: 10, status: 'published', translation: {} as never },
          { artifactId: 'translation-ass', type: 'subtitle_sidecar', streamIndex: -1, sourceCodec: 'ass', sourceType: 'text', format: 'ass', mode: 'translation', publishedPath: '/media/episode.es-419.ass', stagedPath: '', sizeBytes: 10, status: 'published', translation: {} as never },
        ],
      },
    }));
    renderDialog(source, queryClient);

    await waitFor(() => expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', false));
    await user.click(screen.getByRole('button', { name: 'Start translation' }));
    expect(await screen.findByText('Translation completed.')).toBeTruthy();
    expect(screen.getByText('SRT')).toBeTruthy();
    expect(screen.getByText('ASS')).toBeTruthy();
    expect(screen.getByText('/media/episode.es-419.srt')).toBeTruthy();
    await waitFor(() => expect(invalidateQueries).toHaveBeenCalledWith({ queryKey: ['externalSubtitles', '/media/episode.mkv'] }));
    expect(invalidateQueries).toHaveBeenCalledTimes(1);
  });
});
