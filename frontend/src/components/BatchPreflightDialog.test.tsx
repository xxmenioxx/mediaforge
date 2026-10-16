// @vitest-environment jsdom

import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { QueueBatchPreflightResponse } from '../api/types';
import { BatchPreflightDialog } from './BatchPreflightDialog';

const mocks = vi.hoisted(() => ({
  edit: vi.fn(),
  update: vi.fn(),
  accept: vi.fn(),
  remove: vi.fn(),
  discard: vi.fn(),
  getCandidate: vi.fn(),
  preview: vi.fn(),
  testEncode: vi.fn(),
  waitPreview: vi.fn(),
  previewURL: vi.fn(),
  open: vi.fn(),
}));

vi.mock('../api/client', () => ({
  api: {
    editQueueBatchPreflight: mocks.edit,
    updateQueueBatchPreflight: mocks.update,
    acceptQueueBatchPreflight: mocks.accept,
    removeQueueBatchPreflightAsset: mocks.remove,
    discardQueueBatchPreflight: mocks.discard,
    getQueueBatchPreflight: mocks.getCandidate,
    createCompatiblePreviewRequest: mocks.preview,
    createTestEncode: mocks.testEncode,
    compatibleAssetPreviewUrl: mocks.previewURL,
  },
  waitForCompatiblePreviewReady: mocks.waitPreview,
}));

const profile = {
  id: 7,
  name: 'Frozen profile v3',
  scope: 'asset',
  description: 'candidate profile',
  container: 'mkv',
  videoCodec: 'x265',
  codecFamily: 'hevc',
  encoderPolicy: 'locked',
  preferredEncoder: 'libx265',
  allowedEncoders: ['libx265'],
  fallbackPolicy: 'wait',
  bitDepth: 10,
  pixelFormat: 'yuv420p10le',
  qualityStrategy: 'high',
  profileVersion: 3,
  audioCodec: 'copy',
  qualityMode: 'crf',
  qualityValue: 18,
  preserveHdr: true,
  preserveSubtitles: true,
  preserveChapters: true,
  workerConfig: { videoEncoder: 'libx265', crop: 'crop=1920:800:0:140', frameRate: '24000/1001' },
  disabled: false,
  createdAt: '',
  updatedAt: '',
};

function candidate(overrides: Partial<QueueBatchPreflightResponse> = {}): QueueBatchPreflightResponse {
  return {
    id: 'candidate-1',
    revision: 1,
    resolved: true,
    status: 'active',
    batchId: 'batch-1',
    batchName: 'Season 1',
    assets: [{
      id: 'asset-1', mediaPath: '/raw/Season 1/Episode 1.mkv', assetRecordId: 9,
      batchId: 'batch-1', batchName: 'Season 1',
      input: { mediaPath: '/raw/Season 1/Episode 1.mkv', libraryId: 2, profileId: 7, audioProfileKey: 'audio-v2', trackProfileKey: 'tracks-v4', processingMode: 'full_encode', priority: 5 },
      status: 'ready', blockers: [], warnings: ['Encoder fallback reviewed'],
      plan: {
        profile: profile as never,
        profileSnapshot: { profileVersion: 3 }, profileVersion: 3, profileCapturedAt: '2026-09-28T12:00:00Z',
        profileResolution: { video: { source: 'path_assignment' } }, audioProfileSnapshot: { key: 'audio-v2' },
        trackProfileSnapshot: { key: 'tracks-v4' }, constraints: { qualityValue: 18 }, selectedEncoder: 'libx265',
        qualityEvidence: {
          sourceBitrateEvidence: { schemaVersion: 1, samplingPolicyVersion: 1, bitrate: 1_100_000, method: 'packet_sampling', confidence: 'low', requestedWindows: 5, successfulWindows: 4, sampledWindows: [] },
          bpppfAssessment: { policyVersion: 1, encoderModelVersion: 1, status: 'high-risk', bpppf: 0.02, referenceBpppf: 0.045, explanation: 'Planning heuristic for reviewed output geometry and cadence.', heuristicOnly: true },
          profileQualification: {
            status: 'PASS_WITH_WARNINGS', qualificationId: 'qualification:reviewed', policyVersion: 'profile-qualification-policy-v1',
            testEncodeId: 41, sourceRevision: 'source-revision-1', sourceFingerprint: 'source-fingerprint-1',
            resolvedProfileFingerprint: 'resolved-profile-v1:abc', perceptualEvidenceIdentity: 'evidence:abc',
            referenceIdentity: 'reference:abc', candidateIdentity: 'candidate:abc',
            rules: [{ key: 'artifact.blocking', outcome: 'PASS', disposition: 'blocking', explanation: 'No candidate blocking regression.' }],
            warnings: ['One selected high motion window needs manual review.'],
          },
        },
        runtimeSnapshotId: 11, runtimeProfile: 'desktop_safe', destination: { destinationRoot: '/library' },
        publishMode: 'standard', processingMode: 'full_encode',
      },
    }],
    ...overrides,
  };
}

describe('BatchPreflightDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.edit.mockImplementation(async (id: string, revision: number) => candidate({ revision: revision + 1, resolved: false }));
    mocks.update.mockImplementation(async (id: string, input: { revision: number; batchName: string; jobs: unknown[] }) => candidate({ revision: input.revision + 1, batchName: input.batchName }));
    mocks.accept.mockImplementation(async (id: string, revision: number) => candidate({ revision: revision + 1, assets: candidate().assets.map((asset) => ({ ...asset, status: 'queued' as const, queuedJobId: 32 })) }));
    mocks.remove.mockImplementation(async (_id: string, revision: number) => candidate({ revision: revision + 1 }));
    mocks.discard.mockImplementation(async (_id: string, revision: number) => candidate({ revision: revision + 1, status: 'discarded', assets: [] }));
    mocks.getCandidate.mockImplementation(async () => candidate());
    mocks.preview.mockResolvedValue({ requestId: 'preview-1' });
    mocks.waitPreview.mockResolvedValue({ state: 'ready' });
    mocks.previewURL.mockReturnValue('/preview/preview-1');
    mocks.testEncode.mockResolvedValue({ id: 5 });
    mocks.open.mockReturnValue({ location: { href: '' } });
    vi.stubGlobal('open', mocks.open);
  });

  it('preserves edit inputs, invalidates and resolves again, then accepts the ready identity', async () => {
    const onAccepted = vi.fn();
    render(<BatchPreflightDialog candidate={candidate()} onClose={vi.fn()} onAccepted={onAccepted} onDiscarded={vi.fn()} />);

    expect(screen.getByText('Frozen profile v3')).toBeTruthy();
    expect(screen.getByText(/video: path_assignment/)).toBeTruthy();
    expect(screen.getByText(/v3 · captured 2026-09-28/)).toBeTruthy();
    expect(screen.getByText(/Encoder fallback reviewed/)).toBeTruthy();
    expect(screen.getByText('Source video bitrate: 1.10 Mbps')).toBeTruthy();
    expect(screen.getByText('Samples analyzed: 4 of 5')).toBeTruthy();
    expect(screen.getByText(/Encoding estimate: High risk/)).toBeTruthy();
    expect(screen.getByText(/Profile Qualification: PASS_WITH_WARNINGS/)).toBeTruthy();
    expect(screen.getAllByText(/qualification:reviewed/).length).toBeGreaterThan(0);
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await screen.findByRole('button', { name: 'Resolve Preflight again' });
    expect(mocks.edit).toHaveBeenCalledWith('candidate-1', 1);
    expect((screen.getByLabelText('Profile ID') as HTMLInputElement).value).toBe('7');
    expect((screen.getByLabelText('Track profile key') as HTMLInputElement).value).toBe('tracks-v4');

    fireEvent.change(screen.getByLabelText('Batch name'), { target: { value: 'Season 1 revised' } });
    fireEvent.change(screen.getByLabelText('Track profile key'), { target: { value: 'tracks-v5' } });
    fireEvent.click(screen.getByRole('button', { name: 'Resolve Preflight again' }));
    await screen.findByRole('button', { name: /Accept & Queue/ });
    expect(mocks.update).toHaveBeenCalledWith('candidate-1', expect.objectContaining({
      revision: 2,
      batchId: 'batch-1',
      batchName: 'Season 1 revised',
      jobs: [expect.objectContaining({ mediaPath: '/raw/Season 1/Episode 1.mkv', profileId: 7, libraryId: 2, audioProfileKey: 'audio-v2', trackProfileKey: 'tracks-v5' })],
    }));

    fireEvent.click(screen.getByRole('button', { name: /Accept & Queue/ }));
    await waitFor(() => expect(mocks.accept).toHaveBeenCalledWith('candidate-1', 3, ['asset-1']));
    expect(onAccepted).toHaveBeenCalledWith(expect.objectContaining({ revision: 4 }));
  });

  it('passes the exact candidate plan to Preview and Test Encode and opens LAB with its identity', async () => {
    render(<BatchPreflightDialog candidate={candidate()} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    const previewWindow = { location: { href: '' } };
    mocks.open.mockReturnValue(previewWindow);
    fireEvent.click(screen.getByRole('button', { name: 'Preview exact plan' }));
    await waitFor(() => expect(mocks.preview).toHaveBeenCalledWith(expect.objectContaining({
      path: '/raw/Season 1/Episode 1.mkv', profile, ephemeral: true,
      preflightCandidateId: 'candidate-1', preflightRevision: 1, preflightAssetId: 'asset-1',
    })));
    expect(mocks.open).toHaveBeenNthCalledWith(1, 'about:blank', '_blank', 'noopener,noreferrer');
    await waitFor(() => expect(previewWindow.location.href).toBe('/preview/preview-1'));

    fireEvent.click(screen.getByRole('button', { name: 'Test Encode exact plan' }));
    await waitFor(() => expect(mocks.testEncode).toHaveBeenCalledWith(expect.objectContaining({
      sourcePath: '/raw/Season 1/Episode 1.mkv',
      libraryId: 2,
      configurationSource: 'batch_preflight',
      preflightCandidateId: 'candidate-1',
      preflightRevision: 1,
      preflightAssetId: 'asset-1',
    })));

    fireEvent.click(screen.getByRole('button', { name: 'Open LAB with candidate context' }));
    expect(mocks.open).toHaveBeenLastCalledWith('/profile-lab?preflightCandidateId=candidate-1&preflightRevision=1&assetId=asset-1', '_blank', 'noopener,noreferrer');
  });

  it('keeps Close, Discard candidate, and Discard edits as separate actions', async () => {
    const onClose = vi.fn();
    render(<BatchPreflightDialog candidate={candidate()} onClose={onClose} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(mocks.discard).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await screen.findByRole('button', { name: 'Discard edits & re-resolve' });
    expect(screen.getByRole('button', { name: 'Close' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Discard candidate' })).toBeTruthy();
  });

  it('persists removal of a blocked candidate asset', async () => {
    const base = candidate();
    const blocked = { ...base.assets[0], id: 'asset-blocked', mediaPath: '/raw/missing.mkv', status: 'blocked' as const, plan: undefined, blockers: ['Source asset is missing or unreadable'] };
    const onDiscarded = vi.fn();
    mocks.remove.mockResolvedValue(candidate({ revision: 2, assets: [base.assets[0]] }));
    render(<BatchPreflightDialog candidate={candidate({ assets: [base.assets[0], blocked] })} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={onDiscarded} />);

    const removeButton = screen.getAllByRole('button', { name: 'Remove from candidate' }).at(-1)!;
    fireEvent.click(removeButton);

    await waitFor(() => expect(mocks.remove).toHaveBeenCalledWith('candidate-1', 1, 'asset-blocked'));
    expect(onDiscarded).not.toHaveBeenCalled();
  });

  it('offers removal for every unqueued review status', () => {
    const base = candidate();
    const review = { ...base.assets[0], id: 'asset-review', mediaPath: '/raw/review.mkv', status: 'requires_review' as const };
    const blocked = { ...base.assets[0], id: 'asset-blocked', mediaPath: '/raw/blocked.mkv', status: 'blocked' as const, blockers: ['Missing source'] };
    render(<BatchPreflightDialog candidate={candidate({ assets: [base.assets[0], review, blocked] })} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    expect(screen.getAllByRole('button', { name: 'Remove from candidate' })).toHaveLength(3);
  });

  it('confirms and discards pending assets while preserving accepted work', async () => {
    const onDiscarded = vi.fn();
    const discarded = candidate({ revision: 2, status: 'discarded', assets: [
      { ...candidate().assets[0], status: 'queued', queuedJobId: 32 },
    ] });
    mocks.discard.mockResolvedValue(discarded);
    render(<BatchPreflightDialog candidate={candidate()} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={onDiscarded} />);

    fireEvent.click(screen.getByRole('button', { name: 'Discard candidate' }));
    expect(screen.getByText(/Previously accepted Queue jobs remain unchanged/)).toBeTruthy();
    expect(mocks.discard).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: 'Discard pending assets' }));

    await waitFor(() => expect(mocks.discard).toHaveBeenCalledWith('candidate-1', 1));
    expect(onDiscarded).toHaveBeenCalledWith(discarded);
  });

  it('keeps unresolved assets after partial acceptance and Close does not discard them', async () => {
    const initial = candidate();
    const blocked = { ...initial.assets[0], id: 'asset-blocked', mediaPath: '/raw/blocked.mkv', status: 'blocked' as const, blockers: ['Source is missing'] };
    const accepted = candidate({
      revision: 2,
      status: 'active',
      assets: [{ ...initial.assets[0], status: 'queued', queuedJobId: 32 }, blocked],
    });
    const onAccepted = vi.fn();
    const onClose = vi.fn();
    mocks.accept.mockResolvedValue(accepted);
    render(<BatchPreflightDialog candidate={candidate({ assets: [initial.assets[0], blocked] })} onClose={onClose} onAccepted={onAccepted} onDiscarded={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Accept & Queue (1)' }));
    await waitFor(() => expect(mocks.accept).toHaveBeenCalledWith('candidate-1', 1, ['asset-1']));
    expect(onAccepted).toHaveBeenCalledWith(accepted);
    expect(screen.getByText(/1 queued; 1 remain in this candidate/)).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(mocks.discard).not.toHaveBeenCalled();
  });

  it('offers an explicit reload after a stale revision error', async () => {
    mocks.remove.mockRejectedValue(new Error('Queue Preflight revision is stale'));
    mocks.getCandidate.mockResolvedValue(candidate({ revision: 2 }));
    render(<BatchPreflightDialog candidate={candidate()} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Remove from candidate' }));
    expect(await screen.findByText('Queue Preflight revision is stale')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Reload latest candidate' }));
    await waitFor(() => expect(mocks.getCandidate).toHaveBeenCalledWith('candidate-1'));
    await waitFor(() => expect(screen.queryByText('Queue Preflight revision is stale')).toBeNull());
  });
});
