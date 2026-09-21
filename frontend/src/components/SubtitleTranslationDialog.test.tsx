// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { SubtitleTranslationCapability, SubtitleTranslationSourceSelector } from '../api/types';
import { SubtitleTranslationDialog } from './SubtitleTranslationDialog';

vi.mock('../api/client', () => ({
  api: {
    subtitleTranslationCapability: vi.fn(),
    startSubtitleTranslation: vi.fn(),
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

function renderDialog(selectedSource = source) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
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
    vi.mocked(api.startSubtitleTranslation).mockResolvedValue({ operation: {} as never });
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
});
