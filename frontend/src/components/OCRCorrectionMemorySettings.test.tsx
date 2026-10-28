// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { OCRCorrectionMemoryEntry } from '../api/types';

vi.mock('../api/client', () => ({
  api: {
    searchOCRCorrectionMemory: vi.fn(),
    exportOCRCorrectionMemory: vi.fn(),
    updateOCRCorrectionMemoryEntry: vi.fn(),
    setOCRCorrectionMemoryEntryEnabled: vi.fn(),
    deleteOCRCorrectionMemoryEntry: vi.fn(),
  },
}));

import { api } from '../api/client';
import { OCRCorrectionMemoryManager, ocrCorrectionMemorySettingsValue } from './OCRCorrectionMemorySettings';

const entry: OCRCorrectionMemoryEntry = {
  id: 7,
  sourceLanguage: 'eng',
  originalText: 'The h0use',
  correctedText: 'The house',
  contextFingerprint: 'bounded-context-fingerprint',
  scopeType: 'title',
  scopeId: 'title-42',
  acceptedSuggestions: 3,
  rejectedSuggestions: 1,
  occurrenceCount: 4,
  approvedAt: '2026-10-01T00:00:00Z',
  expiresAt: '2027-03-30T00:00:00Z',
  enabled: true,
  approvalValid: true,
  provenance: [{
    sourceRevision: 'source-revision', artifactRevision: 'artifact-revision', sourceHash: 'source-hash',
    cueId: 17, approvalAction: 'manual_edit', approvedAt: '2026-10-01T00:00:00Z',
  }],
};

function renderManager() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={queryClient}><OCRCorrectionMemoryManager /></QueryClientProvider>);
}

beforeEach(() => vi.clearAllMocks());
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('OCR correction memory settings and manager', () => {
  it('defaults learning, retention, and prompt enrichment to the safe values', () => {
    expect(ocrCorrectionMemorySettingsValue(undefined)).toEqual({ enabled: false, retentionDays: 180, includeInAIPrompts: false });
  });

  it('edits entries with scope, shows provenance, and requires confirmation before deletion', async () => {
    const user = userEvent.setup();
    vi.mocked(api.searchOCRCorrectionMemory).mockResolvedValue({ entries: [entry], total: 1, page: 1, pageSize: 20 } as never);
    vi.mocked(api.updateOCRCorrectionMemoryEntry).mockResolvedValue({ ...entry, correctedText: 'The home' } as never);
    vi.mocked(api.deleteOCRCorrectionMemoryEntry).mockResolvedValue({ message: 'Deleted' });
    renderManager();

    await user.click(screen.getByRole('button', { name: /OCR correction memory manager/ }));
    expect(await screen.findByText('The h0use')).toBeTruthy();
    expect(screen.getByText(/3 accepted · 1 rejected/)).toBeTruthy();
    await user.click(screen.getByText('Provenance (1)'));
    expect(screen.getByText(/source source-revision · artifact artifact-revision · hash source-hash/)).toBeTruthy();

    await user.click(screen.getByRole('button', { name: 'Edit entry' }));
    const correctedText = screen.getByRole('textbox', { name: 'Corrected text' });
    await user.clear(correctedText);
    await user.type(correctedText, 'The home');
    await user.click(screen.getByRole('button', { name: 'Save entry' }));
    await waitFor(() => expect(api.updateOCRCorrectionMemoryEntry).toHaveBeenCalledWith(7, {
      originalText: 'The h0use', correctedText: 'The home', scopeType: 'title', scopeId: 'title-42',
    }));

    await user.click(screen.getByRole('button', { name: 'Delete entry' }));
    const confirmation = screen.getByRole('dialog', { name: 'Delete OCR correction memory entry?' });
    expect(within(confirmation).getByText(/cannot be undone/)).toBeTruthy();
    await user.click(within(confirmation).getByRole('button', { name: 'Delete entry' }));
    await waitFor(() => expect(api.deleteOCRCorrectionMemoryEntry).toHaveBeenCalledWith(7));
  }, 15000);

  it('does not allow a stale approval to be re-enabled and exports the memory', async () => {
    const user = userEvent.setup();
    const staleEntry = { ...entry, enabled: false, approvalValid: false, invalidatedAt: '2026-10-20T00:00:00Z' };
    vi.mocked(api.searchOCRCorrectionMemory).mockResolvedValue({ entries: [staleEntry], total: 1, page: 1, pageSize: 20 } as never);
    vi.mocked(api.exportOCRCorrectionMemory).mockResolvedValue({ exportedAt: '2026-10-28T00:00:00Z', entries: [staleEntry] } as never);
    const createObjectURL = vi.fn(() => 'blob:ocr-memory');
    const revokeObjectURL = vi.fn();
    const anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);
    vi.stubGlobal('URL', { createObjectURL, revokeObjectURL });
    renderManager();

    await user.click(screen.getByRole('button', { name: /OCR correction memory manager/ }));
    expect(await screen.findByText('Approval stale')).toBeTruthy();
    expect(screen.getByRole('switch', { name: 'Use for suggestions' })).toHaveProperty('disabled', true);
    await user.click(screen.getByRole('button', { name: 'Export memory' }));
    await waitFor(() => expect(api.exportOCRCorrectionMemory).toHaveBeenCalledTimes(1));
    expect(createObjectURL).toHaveBeenCalledTimes(1);
    expect(anchorClick).toHaveBeenCalledTimes(1);
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:ocr-memory');
  }, 15000);
});
