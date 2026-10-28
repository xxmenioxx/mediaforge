import { afterEach, describe, expect, it, vi } from 'vitest';
import { api } from './client';

afterEach(() => vi.unstubAllGlobals());

describe('OCR correction memory API', () => {
  it('uses the scoped search, export, edit, enable, feedback, and delete contracts', async () => {
    const entry = {
      id: 12,
      sourceLanguage: 'eng',
      originalText: 'The h0use',
      correctedText: 'The house',
      contextFingerprint: 'context-fingerprint',
      scopeType: 'title' as const,
      scopeId: 'movie-7',
      acceptedSuggestions: 2,
      rejectedSuggestions: 1,
      occurrenceCount: 3,
      approvedAt: '2026-10-28T00:00:00Z',
      expiresAt: '2027-04-26T00:00:00Z',
      enabled: true,
      approvalValid: true,
      provenance: [],
    };
    const responses: unknown[] = [
      { entries: [entry], total: 1, page: 2, pageSize: 20 },
      { exportedAt: '2026-10-28T00:00:00Z', entries: [entry] },
      entry,
      entry,
      entry,
      { message: 'Deleted' },
    ];
    const fetchMock = vi.fn().mockImplementation(async () => ({
      ok: true,
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => responses.shift(),
    }));
    vi.stubGlobal('fetch', fetchMock);

    await api.searchOCRCorrectionMemory({ search: '  The house  ', language: 'eng', scopeType: 'title', page: 2, pageSize: 20 });
    await api.exportOCRCorrectionMemory();
    await api.updateOCRCorrectionMemoryEntry(12, { originalText: 'The h0use', correctedText: 'The house', scopeType: 'title', scopeId: 'movie-7' });
    await api.setOCRCorrectionMemoryEntryEnabled(12, false);
    await api.recordOCRCorrectionMemoryFeedback(12, 'accepted');
    await api.deleteOCRCorrectionMemoryEntry(12);

    expect(fetchMock).toHaveBeenCalledTimes(6);
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/api/assets/ocr-correction-memory?search=The+house&language=eng&scopeType=title&page=2&pageSize=20');
    expect(fetchMock.mock.calls[1]?.[0]).toBe('/api/assets/ocr-correction-memory/export');
    expect(fetchMock.mock.calls[2]?.[0]).toBe('/api/assets/ocr-correction-memory/12');
    expect(fetchMock.mock.calls[2]?.[1]).toEqual(expect.objectContaining({ method: 'POST' }));
    expect(JSON.parse(String(fetchMock.mock.calls[2]?.[1]?.body))).toEqual({ originalText: 'The h0use', correctedText: 'The house', scopeType: 'title', scopeId: 'movie-7' });
    expect(fetchMock.mock.calls[3]?.[0]).toBe('/api/assets/ocr-correction-memory/12/enabled');
    expect(JSON.parse(String(fetchMock.mock.calls[3]?.[1]?.body))).toEqual({ enabled: false });
    expect(fetchMock.mock.calls[4]?.[0]).toBe('/api/assets/ocr-correction-memory/12/feedback');
    expect(JSON.parse(String(fetchMock.mock.calls[4]?.[1]?.body))).toEqual({ decision: 'accepted' });
    expect(fetchMock.mock.calls[5]?.[0]).toBe('/api/assets/ocr-correction-memory/12');
    expect(fetchMock.mock.calls[5]?.[1]).toEqual(expect.objectContaining({ method: 'DELETE' }));
  });
});
