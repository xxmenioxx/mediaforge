// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { HousekeepingCard } from './SettingsPage';
import type { HousekeepingReport } from '../api/types';

afterEach(() => {
  cleanup();
});

const value = {
  autoEnabled: true,
  intervalHours: 24,
  failedRetentionDays: 7,
  canceledRetentionDays: 3,
  orphanRetentionDays: 7,
  publishedWorkspaceRetentionDays: 0,
  publishedJobLogRetentionDays: 14,
  failedJobLogRetentionDays: 14,
  canceledJobLogRetentionDays: 7,
  orphanJobLogRetentionDays: 7,
  emptyRawDirectoriesEnabled: true,
  orphanStageArtifactsEnabled: true,
};

function report(overrides: Partial<HousekeepingReport> = {}): HousekeepingReport {
  return {
    ranAt: '2026-10-09T12:00:00Z',
    dryRun: true,
    previewId: 'preview-token-301',
    previewExpiresAt: new Date(Date.now() + 5 * 60_000).toISOString(),
    candidates: [{
      candidateId: 'candidate-1', kind: 'job_workspace', path: '/work/job-1', jobId: 1,
      reason: 'Workspace is retained past policy', ownership: 'Job 1', sizeBytes: 1024,
      modifiedAt: '2026-10-01T12:00:00Z', safeToDelete: true,
    }],
    summaries: [{ kind: 'job_workspace', candidateCount: 1, recoverableBytes: 1024 }],
    removedPaths: [],
    recoveredBytes: 0,
    errors: [],
    ...overrides,
  };
}

function Harness({ initialPreview, onRun }: { initialPreview?: HousekeepingReport; onRun: (previewId: string) => void }) {
  const [preview, setPreview] = useState(initialPreview);
  const [result, setResult] = useState<HousekeepingReport>();
  return <HousekeepingCard value={value} saving={false} preview={preview} result={result} busy={false}
    onSave={() => undefined} onPreview={() => undefined}
    onRun={(previewId) => {
      onRun(previewId);
      setPreview(undefined);
      setResult(report({ dryRun: false, outcome: 'cleaned', previewId: undefined, previewExpiresAt: undefined, removedPaths: ['/work/job-1'] }));
    }} />;
}

describe('Settings housekeeping preview authorization', () => {
  it('passes the preview ID to run and consumes the preview after success', async () => {
    const onRun = vi.fn();
    const user = userEvent.setup();
    render(<Harness initialPreview={report()} onRun={onRun} />);

    const run = screen.getByRole('button', { name: 'Remove previewed candidates' });
    expect((run as HTMLButtonElement).disabled).toBe(false);
    await user.click(run);

    expect(onRun).toHaveBeenCalledWith('preview-token-301');
    expect((screen.getByRole('button', { name: 'Remove previewed candidates' }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText('Cleanup result')).toBeTruthy();
    expect(screen.getByText('Outcome: cleaned')).toBeTruthy();
  });

  it('does not enable run for blocked or expired previews', () => {
    const onRun = vi.fn();
    const blocked = report({ candidates: [{ ...report().candidates[0], safeToDelete: false, blockedReason: 'Referenced by active operation' }] });
    const { rerender } = render(<Harness initialPreview={blocked} onRun={onRun} />);

    expect((screen.getByRole('button', { name: 'Remove previewed candidates' }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(/Referenced by active operation/)).toBeTruthy();

    rerender(<Harness key="expired" initialPreview={report({ previewExpiresAt: new Date(Date.now() - 5 * 60_000).toISOString() })} onRun={onRun} />);
    expect((screen.getByRole('button', { name: 'Remove previewed candidates' }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText(/This preview has expired/)).toBeTruthy();
    expect(onRun).not.toHaveBeenCalled();
  });

  it('shows RAW protection records and completed maintenance no-op state', () => {
    const preview = report({
      candidates: [
        {
          candidateId: 'raw-protected', kind: 'empty_directory', path: '/media/raw/movies/Whiplash',
          reason: 'directory is literally empty', ownership: 'empty_directory under raw storage role',
          sizeBytes: 0, modifiedAt: '2026-10-01T12:00:00Z', safeToDelete: false, outcome: 'blocked',
          blockedReason: 'directory is protected by AssetRecord #42 path (stale_reference_suspected)',
          protectionReferences: [{ source: 'AssetRecord', recordId: '42', field: 'path', path: '/media/raw/movies/Whiplash/Whiplash.mkv', state: 'stale_reference_suspected', reason: 'Re-scan or reconcile inventory before cleanup.' }],
          protectionReferenceCount: 1,
        },
        {
          candidateId: 'maintenance-noop', kind: 'stage_artifact', path: '/media/library/Akira/.Akira.mkv.maintenance-1.tmp.mkv',
          operationId: 'maintenance-1', reason: 'completed maintenance temporary output is already absent',
          ownership: 'asset_maintenance_operation.temporary_path under Library storage role', sizeBytes: 0,
          modifiedAt: '2026-09-01T12:00:00Z', safeToDelete: false, outcome: 'already_absent',
          maintenance: {
            assetPath: '/media/library/Akira/Akira.mkv', assetRecordId: 7, assetRecordStatus: 'processed', assetRecordMissing: false,
            status: 'completed', phase: 'completed', finishedAt: '2026-09-01T12:00:00Z', temporaryPresence: 'absent', temporarySizeKnown: true,
            backupPath: '/media/library/Akira/.Akira.mkv.maintenance-1.backup.mkv', backupPresence: 'absent',
            recoveryState: 'completed operation; temporary output is already absent', resultFingerprint: 'result-hash',
            assetFingerprintState: 'verified_match', currentAssetFingerprint: 'result-hash',
          },
        },
      ],
      summaries: [
        { kind: 'empty_directory', candidateCount: 1, recoverableBytes: 0 },
        { kind: 'stage_artifact', candidateCount: 1, recoverableBytes: 0 },
      ],
    });
    render(<HousekeepingCard value={value} saving={false} preview={preview} busy={false} onSave={() => undefined} onPreview={() => undefined} onRun={() => undefined} />);

    expect(screen.getByText(/AssetRecord #42/)).toBeTruthy();
    expect(screen.getByText(/Re-scan or reconcile inventory/)).toBeTruthy();
    expect(screen.getByText(/AssetRecord #7/)).toBeTruthy();
    expect(screen.getByText(/completed \/ completed/)).toBeTruthy();
    expect(screen.getByText(/Already absent · 0 recoverable bytes/)).toBeTruthy();
    expect((screen.getByRole('button', { name: 'Remove previewed candidates' }) as HTMLButtonElement).disabled).toBe(true);
  });
});
