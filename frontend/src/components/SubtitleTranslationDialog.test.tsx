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
  }), onReviewOCR?: () => void) {
  render(
    <QueryClientProvider client={queryClient}>
      <SubtitleTranslationDialog
        open
        path="/media/episode.mkv"
        source={selectedSource}
        onClose={() => undefined}
        onReviewOCR={onReviewOCR}
      />
    </QueryClientProvider>,
  );
  return queryClient;
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

  it('uses a sole backend-authorized SSA output without showing a format selector', async () => {
    const user = userEvent.setup();
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({ outputRepresentations: ['ssa'] }));
    renderDialog();

    await waitFor(() => expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', false));
    expect(screen.queryByRole('combobox', { name: 'Output format' })).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Start translation' }));
    await waitFor(() => expect(api.startSubtitleTranslation).toHaveBeenCalled());
    expect(vi.mocked(api.startSubtitleTranslation).mock.calls[0]?.[0].outputRepresentation).toBe('ssa');
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
    expect(screen.queryByRole('option', { name: 'SSA' })).toBeNull();
    expect(screen.queryByRole('option', { name: 'SUP' })).toBeNull();
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

  it.each([
    ['English', 'en'],
    ['Korean', 'ko'],
  ])('retries a terminal %s translation with a new ID and never starts a second operation', async (languageLabel, sourceLanguage) => {
    const user = userEvent.setup();
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({ sourceLanguages: ['auto', 'en', 'ko'] }));
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
    const sourceLanguageSelect = screen.getByRole('combobox', { name: 'Source language' });
    await user.click(sourceLanguageSelect);
    await user.click(await screen.findByRole('option', { name: languageLabel }));
    await user.click(screen.getByRole('button', { name: 'Start translation' }));
    expect(await screen.findByText(/provider_unavailable: Provider unavailable\./)).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Retry translation' }));

    expect(api.retrySubtitleTranslation).toHaveBeenCalledWith('translation-1');
    expect(vi.mocked(api.startSubtitleTranslation).mock.calls[0]?.[0]).toEqual(expect.objectContaining({ sourceLanguage }));
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

  it('disables capability refetches after start and across retry', async () => {
    const user = userEvent.setup();
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const failed = operation({
      status: 'error',
      phase: 'error',
      translation: { failureClass: 'provider_unavailable', diagnostic: 'Provider unavailable.', retryable: true, cancellable: false },
    });
    vi.mocked(api.subtitleOperation)
      .mockResolvedValueOnce(failed)
      .mockResolvedValue(operation({ id: 'translation-2' }));
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({ operation: operation() });
    vi.mocked(api.retrySubtitleTranslation).mockResolvedValue({ operation: operation({ id: 'translation-2' }) });
    renderDialog(source, queryClient);

    await waitFor(() => expect(api.subtitleTranslationCapability).toHaveBeenCalledTimes(1));
    await user.click(await screen.findByRole('button', { name: 'Start translation' }));
    await user.click(await screen.findByRole('button', { name: 'Retry translation' }));
    await queryClient.refetchQueries({ queryKey: ['subtitleTranslationCapability'] });

    expect(api.subtitleTranslationCapability).toHaveBeenCalledTimes(1);
    expect(api.startSubtitleTranslation).toHaveBeenCalledTimes(1);
    expect(api.retrySubtitleTranslation).toHaveBeenCalledTimes(1);
    expect(api.subtitleOperation).toHaveBeenCalledWith('translation-2');
  });

  it('renders only the backend-provided source languages', async () => {
    const user = userEvent.setup();
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({ sourceLanguages: ['auto', 'ja'] }));
    renderDialog();

    const sourceLanguage = await screen.findByRole('combobox', { name: 'Source language' });
    await user.click(sourceLanguage);

    expect(await screen.findByRole('option', { name: 'Auto' })).toBeTruthy();
    expect(screen.getByRole('option', { name: 'Japanese' })).toBeTruthy();
    expect(screen.queryByRole('option', { name: 'English' })).toBeNull();
  });

  it.each([
    ['ko', 'Korean'],
    ['fr', 'French'],
  ])('renders and submits the canonical %s source language code', async (code, label) => {
    const user = userEvent.setup();
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({
      sourceLanguages: ['auto', 'en', 'ja', 'ko', 'fr'],
    }));
    renderDialog();

    const sourceLanguage = await screen.findByRole('combobox', { name: 'Source language' });
    await user.click(sourceLanguage);
    expect(await screen.findByRole('option', { name: label })).toBeTruthy();
    await user.click(screen.getByRole('option', { name: label }));
    await user.click(screen.getByRole('button', { name: 'Start translation' }));

    await waitFor(() => expect(api.startSubtitleTranslation).toHaveBeenCalled());
    expect(vi.mocked(api.startSubtitleTranslation).mock.calls[0]?.[0].sourceLanguage).toBe(code);
  });

  it('keeps es-419 and es-ES distinct in the start request', async () => {
    const user = userEvent.setup();
    renderDialog();

    await user.click(await screen.findByRole('combobox', { name: 'Target locale' }));
    await user.click(screen.getByRole('option', { name: 'Spanish (Spain)' }));
    await user.click(screen.getByRole('button', { name: 'Start translation' }));

    await waitFor(() => expect(api.startSubtitleTranslation).toHaveBeenCalled());
    expect(vi.mocked(api.startSubtitleTranslation).mock.calls[0]?.[0].targetLocale).toBe('es-ES');
  });

  it('presents the configured provider and model without making them operation inputs', async () => {
    const user = userEvent.setup();
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({
      provider: { name: 'ollama', model: 'qwen2.5', modelSelectable: true },
    }));
    renderDialog();

    expect(await screen.findByText('Provider: Ollama · Model: qwen2.5')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Start translation' }));
    await waitFor(() => expect(api.startSubtitleTranslation).toHaveBeenCalled());
    expect(vi.mocked(api.startSubtitleTranslation).mock.calls[0]?.[0]).not.toHaveProperty('provider');
    expect(vi.mocked(api.startSubtitleTranslation).mock.calls[0]?.[0]).not.toHaveProperty('model');
    expect(vi.mocked(api.startSubtitleTranslation).mock.calls[0]?.[0]).not.toHaveProperty('endpoint');
  });

  it('hides the model when the backend says it is not selectable', async () => {
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({
      provider: { name: 'ollama', model: 'qwen2.5', modelSelectable: false },
    }));
    renderDialog();

    expect(await screen.findByText('Provider: Ollama')).toBeTruthy();
    expect(screen.queryByText(/qwen2\.5/)).toBeNull();
  });

  it('does not render an empty configured model', async () => {
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({
      provider: { name: 'ollama', model: '', modelSelectable: true },
    }));
    renderDialog();

    expect(await screen.findByText('Provider: Ollama')).toBeTruthy();
    expect(screen.queryByText(/Model:/)).toBeNull();
  });

  it('shows missing media context informationally without blocking translation', async () => {
    const user = userEvent.setup();
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({
      context: { status: 'unavailable', source: 'none' },
    }));
    renderDialog();

    expect(await screen.findByText('Media context: unavailable · none')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', false);
    await user.click(screen.getByRole('button', { name: 'Start translation' }));
    expect(api.startSubtitleTranslation).toHaveBeenCalledTimes(1);
  });

  it('disables Start and does not invent an output when the backend returns none', async () => {
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({ outputRepresentations: [] }));
    renderDialog();

    expect(await screen.findByText('No valid translation output is available for this subtitle.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', true);
    expect(screen.queryByRole('combobox', { name: 'Output format' })).toBeNull();
    expect(api.startSubtitleTranslation).not.toHaveBeenCalled();
  });

  it.each([
    'Translation provider is not configured.',
    'Translation provider configuration must be tested.',
    'Translation provider is unavailable.',
    'Configured translation model is unavailable.',
    'Translation provider configuration is invalid.',
    'Subtitle translation is disabled.',
  ])('surfaces the backend-authoritative unavailable reason: %s', async (reason) => {
    vi.mocked(api.subtitleTranslationCapability).mockResolvedValue(capability({ available: false, reason }));
    renderDialog();

    expect(await screen.findByText(reason)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', true);
    expect(api.startSubtitleTranslation).not.toHaveBeenCalled();
  });

  it('trusts refreshed capability availability after readiness is invalidated', async () => {
    const queryClient = new QueryClient({
      defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
    });
    const first = capability({ available: true });
    const invalidated = capability({ available: false, reason: 'Translation provider configuration must be tested.' });
    vi.mocked(api.subtitleTranslationCapability)
      .mockResolvedValueOnce(first)
      .mockResolvedValueOnce(invalidated);
    const view = render(
      <QueryClientProvider client={queryClient}>
        <SubtitleTranslationDialog open path="/media/episode.mkv" source={source} onClose={() => undefined} />
      </QueryClientProvider>,
    );

    await waitFor(() => expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', false));
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <SubtitleTranslationDialog open={false} path="/media/episode.mkv" source={source} onClose={() => undefined} />
      </QueryClientProvider>,
    );
    await queryClient.invalidateQueries({ queryKey: ['subtitleTranslationCapability'] });
    view.rerender(
      <QueryClientProvider client={queryClient}>
        <SubtitleTranslationDialog open path="/media/episode.mkv" source={source} onClose={() => undefined} />
      </QueryClientProvider>,
    );

    expect(await screen.findByText('Translation provider configuration must be tested.')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Start translation' })).toHaveProperty('disabled', true);
    expect(api.subtitleTranslationCapability).toHaveBeenCalledTimes(2);
  });

  it('keeps Auto unchanged when the backend reports ambiguous source language', async () => {
    const user = userEvent.setup();
    const diagnostic = 'Select English or Japanese explicitly because source language could not be determined.';
    const failed = operation({
      status: 'error',
      phase: 'error',
      error: 'SUPER_SECRET_TOKEN_123 PRIVATE_PROVIDER_RESPONSE_456 http://private-nas:11434',
      translation: { failureClass: 'translation_failed', diagnostic, retryable: true, cancellable: false },
    });
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({ operation: failed });
    vi.mocked(api.subtitleOperation).mockResolvedValue(failed);
    renderDialog();

    await user.click(await screen.findByRole('button', { name: 'Start translation' }));
    await waitFor(() => expect(api.startSubtitleTranslation).toHaveBeenCalled());
    expect(vi.mocked(api.startSubtitleTranslation).mock.calls[0]?.[0]).toEqual(expect.objectContaining({ sourceLanguage: 'auto' }));
    expect(await screen.findByText(`translation_failed: ${diagnostic}`)).toBeTruthy();
    expect(screen.queryByText(/SUPER_SECRET_TOKEN_123|PRIVATE_PROVIDER_RESPONSE_456|private-nas:11434/)).toBeNull();
  });

  it('surfaces OCR review as a paused translation with an action to the OCR review flow', async () => {
    const user = userEvent.setup();
    const onReviewOCR = vi.fn();
    const failed = operation({
      status: 'error',
      phase: 'ocr_review',
      error: 'private path /workspace/private',
      translation: { failureClass: 'ocr_review_required', diagnostic: 'OCR review is required before translation.', retryable: true, cancellable: false },
    });
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({ operation: failed });
    vi.mocked(api.subtitleOperation).mockResolvedValue(failed);
    renderDialog(source, undefined, onReviewOCR);

    await user.click(await screen.findByRole('button', { name: 'Start translation' }));
    expect(await screen.findByText('Translation paused')).toBeTruthy();
    expect(screen.getByText('OCR review is required before translation.')).toBeTruthy();
    expect(screen.getByText('Stage: OCR review')).toBeTruthy();
    expect(screen.queryByText(/private path|workspace\/private/)).toBeNull();
    await user.click(screen.getByRole('button', { name: 'Review OCR' }));
    expect(onReviewOCR).toHaveBeenCalledTimes(1);
  });

  it('renders locale-distinct translation artifacts and filters non-translation artifacts', async () => {
    const user = userEvent.setup();
    const result = {
      created: [],
      existing: [],
      unsupported: [],
      artifacts: [
        { artifactId: 'es-419-srt', type: 'subtitle_sidecar' as const, streamIndex: -1, sourceCodec: 'srt', sourceType: 'text', format: 'srt' as const, mode: 'translation', publishedPath: '/media/episode.es-419.srt', stagedPath: '', sizeBytes: 10, status: 'published', translation: { targetLocale: 'es-419' } as never },
        { artifactId: 'es-es-ass', type: 'subtitle_sidecar' as const, streamIndex: -1, sourceCodec: 'ass', sourceType: 'text', format: 'ass' as const, mode: 'translation', publishedPath: '/media/episode.es-ES.ass', stagedPath: '', sizeBytes: 10, status: 'published', translation: { targetLocale: 'es-ES' } as never },
        { artifactId: 'original', type: 'subtitle_sidecar' as const, streamIndex: 4, sourceCodec: 'ass', sourceType: 'embedded', format: 'ass' as const, mode: 'original', publishedPath: '/media/episode.ass', stagedPath: '', sizeBytes: 10, status: 'published' },
      ],
    };
    const completed = operation({ status: 'completed', phase: 'completed', progress: 100, result });
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({ operation: completed });
    vi.mocked(api.subtitleOperation).mockResolvedValue(completed);
    renderDialog();

    await user.click(await screen.findByRole('button', { name: 'Start translation' }));
    expect(await screen.findByText('ES-419')).toBeTruthy();
    expect(screen.getByText('ES-ES')).toBeTruthy();
    expect(screen.getByText('/media/episode.es-419.srt')).toBeTruthy();
    expect(screen.getByText('/media/episode.es-ES.ass')).toBeTruthy();
    expect(screen.queryByText('/media/episode.ass')).toBeNull();
  });

  it('honors backend cancellable and retryable flags instead of deriving controls from status', async () => {
    const user = userEvent.setup();
    const terminal = operation({
      status: 'error',
      phase: 'error',
      translation: { failureClass: 'translation_failed', diagnostic: 'Translation failed.', retryable: false, cancellable: false },
    });
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({ operation: terminal });
    vi.mocked(api.subtitleOperation).mockResolvedValue(terminal);
    renderDialog();

    await user.click(await screen.findByRole('button', { name: 'Start translation' }));
    expect(await screen.findByText('translation_failed: Translation failed.')).toBeTruthy();
    expect(screen.queryByRole('button', { name: 'Cancel translation' })).toBeNull();
    expect(screen.queryByRole('button', { name: 'Retry translation' })).toBeNull();
  });

  it.each([
    { status: 'error' as const, phase: 'error' as const, message: 'translation_failed: Translation failed.' },
    { status: 'cancelled' as const, phase: 'cancelled' as const, message: 'Translation cancelled.' },
  ])('stops polling after terminal $status', async ({ status, phase, message }) => {
    const user = userEvent.setup();
    const terminal = operation({
      status,
      phase,
      translation: { failureClass: status === 'error' ? 'translation_failed' : undefined, diagnostic: status === 'error' ? 'Translation failed.' : undefined, retryable: false, cancellable: false },
    });
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({ operation: terminal });
    vi.mocked(api.subtitleOperation).mockResolvedValue(terminal);
    renderDialog();

    await user.click(await screen.findByRole('button', { name: 'Start translation' }));
    expect(await screen.findByText(message)).toBeTruthy();
    await waitFor(() => expect(api.subtitleOperation).toHaveBeenCalledTimes(1));
    await new Promise((resolve) => setTimeout(resolve, 650));
    expect(api.subtitleOperation).toHaveBeenCalledTimes(1);
  }, 5000);
});
