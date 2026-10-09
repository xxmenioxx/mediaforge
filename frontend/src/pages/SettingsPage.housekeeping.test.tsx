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
      setResult(report({ dryRun: false, previewId: undefined, previewExpiresAt: undefined, removedPaths: ['/work/job-1'] }));
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
});
