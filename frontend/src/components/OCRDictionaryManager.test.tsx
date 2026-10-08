// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { OCRDictionaryEntry } from '../api/types';
import { api } from '../api/client';
import { OCRDictionaryManager } from './OCRDictionaryManager';

vi.mock('../api/client', () => ({
  api: {
    searchOCRDictionary: vi.fn(),
    createOCRDictionaryEntry: vi.fn(),
    updateOCRDictionaryEntry: vi.fn(),
    setOCRDictionaryEntryEnabled: vi.fn(),
    deleteOCRDictionaryEntry: vi.fn(),
    testOCRDictionaryMatch: vi.fn(),
  },
}));

const builtin: OCRDictionaryEntry = {
  id: 'builtin:eng:im',
  language: 'eng',
  pattern: "|'m",
  replacement: "I'm",
  matchMode: 'literal',
  boundaryPolicy: 'word',
  casePolicy: 'exact',
  priority: 100,
  enabled: true,
  source: 'builtin',
  ruleVersion: 1,
  createdAt: '',
  updatedAt: '',
};

const custom: OCRDictionaryEntry = {
  ...builtin,
  id: 'custom-1',
  pattern: "|'ve",
  replacement: "I've",
  source: 'user',
  priority: 10,
};

let currentEntries: OCRDictionaryEntry[];

afterEach(cleanup);

beforeEach(() => {
  currentEntries = [builtin];
  vi.clearAllMocks();
  vi.mocked(api.searchOCRDictionary).mockImplementation(async () => ({ entries: currentEntries, page: 1, pageSize: 100, total: currentEntries.length }));
  vi.mocked(api.setOCRDictionaryEntryEnabled).mockImplementation(async (id, enabled) => {
    currentEntries = currentEntries.map((entry) => entry.id === id ? { ...entry, enabled } : entry);
    return currentEntries.find((entry) => entry.id === id)!;
  });
});

describe('OCRDictionaryManager', () => {
  it('loads language-scoped built-ins and lets users disable a protected rule', async () => {
    render(<OCRDictionaryManager />);

    expect(await screen.findByText("|'m → I'm")).toBeTruthy();
    expect(api.searchOCRDictionary).toHaveBeenCalledWith(expect.objectContaining({ language: 'eng', page: 1, pageSize: 100 }));
    expect(screen.getByText('Protected')).toBeTruthy();
    fireEvent.click(screen.getByRole('switch', { name: "Enable |'m" }));

    await waitFor(() => expect(api.setOCRDictionaryEntryEnabled).toHaveBeenCalledWith('builtin:eng:im', false));
    await waitFor(() => expect((screen.getByRole('switch', { name: "Enable |'m" }) as HTMLInputElement).checked).toBe(false));
  });

  it('previews a custom rule and creates it through the API', async () => {
    currentEntries = [];
    vi.mocked(api.testOCRDictionaryMatch).mockResolvedValue({
      language: 'eng', matched: true, matchCount: 1, result: "They've arrived", dictionaryRevision: 'dict-r1',
    });
    vi.mocked(api.createOCRDictionaryEntry).mockImplementation(async (input) => {
      const entry = { ...custom, language: input.language, pattern: input.pattern, replacement: input.replacement };
      currentEntries = [entry];
      return entry;
    });
    vi.mocked(api.searchOCRDictionary).mockImplementation(async () => ({ entries: currentEntries, page: 1, pageSize: 100, total: currentEntries.length }));
    render(<OCRDictionaryManager />);

    fireEvent.change(screen.getByRole('textbox', { name: 'OCR text' }), { target: { value: "|'ve" } });
    fireEvent.change(screen.getByRole('textbox', { name: 'Suggested text' }), { target: { value: "I've" } });
    fireEvent.change(screen.getByRole('textbox', { name: 'Sample subtitle text' }), { target: { value: "They |'ve arrived" } });
    fireEvent.click(screen.getByRole('button', { name: 'Test match' }));
    expect(await screen.findByText("They've arrived")).toBeTruthy();
    expect(api.testOCRDictionaryMatch).toHaveBeenCalledWith(expect.objectContaining({
      language: 'eng', sample: "They |'ve arrived", rule: expect.objectContaining({ pattern: "|'ve", replacement: "I've" }),
    }));

    fireEvent.click(screen.getByRole('button', { name: 'Add rule' }));
    await waitFor(() => expect(api.createOCRDictionaryEntry).toHaveBeenCalledWith(expect.objectContaining({
      language: 'eng', pattern: "|'ve", replacement: "I've", matchMode: 'literal', boundaryPolicy: 'word',
    })));
    expect(await screen.findByText("|'ve → I've")).toBeTruthy();
  });

  it('edits and deletes custom rules', async () => {
    currentEntries = [custom];
    vi.mocked(api.searchOCRDictionary).mockImplementation(async () => ({ entries: currentEntries, page: 1, pageSize: 100, total: currentEntries.length }));
    vi.mocked(api.updateOCRDictionaryEntry).mockImplementation(async (id, input) => {
      const updated = { ...custom, ...input, id };
      currentEntries = [updated];
      return updated;
    });
    vi.mocked(api.deleteOCRDictionaryEntry).mockImplementation(async () => {
      currentEntries = [];
      return { message: 'deleted' };
    });
    render(<OCRDictionaryManager />);

    await screen.findByText("|'ve → I've");
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Suggested text' }), { target: { value: "I've arrived" } });
    fireEvent.click(screen.getByRole('button', { name: 'Save rule' }));
    await waitFor(() => expect(api.updateOCRDictionaryEntry).toHaveBeenCalledWith('custom-1', expect.objectContaining({ replacement: "I've arrived" })));
    expect(await screen.findByText("|'ve → I've arrived")).toBeTruthy();

    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(api.deleteOCRDictionaryEntry).toHaveBeenCalledWith('custom-1'));
    expect(await screen.findByText('No matching rules.')).toBeTruthy();
  });

  it('keeps the draft visible when server validation rejects a rule', async () => {
    vi.mocked(api.createOCRDictionaryEntry).mockRejectedValue(new Error('bar rules require a literal word context'));
    render(<OCRDictionaryManager />);

    fireEvent.change(screen.getByRole('textbox', { name: 'OCR text' }), { target: { value: '|' } });
    fireEvent.change(screen.getByRole('textbox', { name: 'Suggested text' }), { target: { value: 'I' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add rule' }));

    expect(await screen.findByText('bar rules require a literal word context')).toBeTruthy();
    expect((screen.getByRole('textbox', { name: 'OCR text' }) as HTMLInputElement).value).toBe('|');
    expect(api.createOCRDictionaryEntry).toHaveBeenCalledTimes(1);
  });
});
