// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { TestEncode } from '../api/types';
import { TestEncodeSettingsDomain } from './TestEncodeSettingsDomain';

vi.mock('../api/client', () => ({ api: {
  testEncodeInventory: vi.fn(), testEncodeInventorySummary: vi.fn(), orphanedTestEncodeArtifacts: vi.fn(),
  keepTestEncode: vi.fn(), reconcileTestEncodes: vi.fn(), previewTestEncodeDelete: vi.fn(), deleteTestEncodes: vi.fn(), deleteOrphanedTestEncodeArtifact: vi.fn(),
} }));

const keptTest = {
  id: 7, sourceAssetId: 15, sourcePath: '/raw/episode.mkv', sourceFingerprint: '', sourceSizeBytes: 0,
  libraryId: 3, configurationSource: 'effective_asset', requestedConfiguration: {}, effectiveConfiguration: {}, configurationHash: 'hash',
  profileId: 2, profileVersion: 1, workerName: 'nas', effectiveEncoder: 'hevc_qsv', startSeconds: 60, durationSeconds: 20,
  status: 'ready', phase: 'ready', progress: 100, ffmpegCommand: '', outputPath: '/library/episode-test.mkv', outputSizeBytes: 100,
  subtitleArtifacts: [], validationReport: {}, keep: true, reconciliationStatus: 'missing_output', outputExists: false,
  artifactCount: 2, sidecarArtifactCount: 1, knownDiskUsageBytes: 20, unknownSizeArtifactCount: 0,
  errorMessage: '', stale: false, createdAt: '2026-09-01T00:00:00Z', updatedAt: '2026-09-01T00:00:00Z',
} satisfies TestEncode;

function renderDomain(onSave = vi.fn()) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return { onSave, ...render(<QueryClientProvider client={client}><TestEncodeSettingsDomain value={undefined} saving={false} onSave={onSave} /></QueryClientProvider>) };
}

describe('TestEncodeSettingsDomain', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.testEncodeInventory).mockResolvedValue([keptTest]);
    vi.mocked(api.testEncodeInventorySummary).mockResolvedValue({ totalRuns: 1, keepRuns: 1, totalKnownBytes: 20, keepKnownBytes: 20, cleanupEligibleRuns: 0, cleanupEligibleBytes: 0, unknownSizeArtifacts: 0 });
    vi.mocked(api.orphanedTestEncodeArtifacts).mockResolvedValue({ artifacts: [], limit: 100 });
    vi.mocked(api.keepTestEncode).mockResolvedValue(keptTest);
    vi.mocked(api.reconcileTestEncodes).mockResolvedValue({ processed: 1, limit: 100, results: [], orphanedArtifacts: [] });
    vi.mocked(api.previewTestEncodeDelete).mockResolvedValue({ testEncodeCount: 1, mediaOutputCount: 1, otherArtifactCount: 1, estimatedBytes: 120, keepCount: 1 });
    vi.mocked(api.deleteTestEncodes).mockResolvedValue({ results: [{ id: keptTest.id, deleted: true }] });
    vi.mocked(api.deleteOrphanedTestEncodeArtifact).mockResolvedValue({ id: 1, deleted: true });
  });

  it('shows reconciliation and storage evidence and requires Keep confirmation before deletion', async () => {
    const user = userEvent.setup();
    renderDomain();
    expect(await screen.findByText('missing_output')).toBeTruthy();
    expect(screen.getByText(/Known storage 20 B/)).toBeTruthy();
    await user.click(await screen.findByRole('button', { name: /^delete$/i }));
    expect(await screen.findByText('1 media outputs')).toBeTruthy();
    const deleteButton = screen.getByRole('button', { name: 'Delete artifacts' });
    expect((deleteButton as HTMLButtonElement).disabled).toBe(true);
    await user.click(screen.getByLabelText(/explicitly confirm deleting the selected keep/i));
    expect((deleteButton as HTMLButtonElement).disabled).toBe(false);
    await user.click(deleteButton);
    await waitFor(() => expect(api.deleteTestEncodes).toHaveBeenCalledWith([keptTest.id], true));
  });
});
