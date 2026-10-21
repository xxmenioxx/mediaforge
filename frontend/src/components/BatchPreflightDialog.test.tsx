// @vitest-environment jsdom

import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { QueueBatchPreflightInput, QueueBatchPreflightResponse } from '../api/types';
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
  startSnapshotOperation: vi.fn(),
  snapshotOperation: vi.fn(),
  cancelSnapshotOperation: vi.fn(),
  cancelPreview: vi.fn(),
  waitPreview: vi.fn(),
  previewOwnerId: vi.fn(),
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
    startSnapshotOperation: mocks.startSnapshotOperation,
    snapshotOperation: mocks.snapshotOperation,
    cancelSnapshotOperation: mocks.cancelSnapshotOperation,
    cancelCompatiblePreview: mocks.cancelPreview,
    compatibleAssetPreviewUrl: mocks.previewURL,
  },
  createCompatiblePreviewOwnerId: mocks.previewOwnerId,
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
        profileResolution: {
          video: { source: 'path', targetPath: '/raw/Season 1', selection: 'profile', videoProfileId: 7 },
          audio: { source: 'logical_group', targetPath: '/raw/Season 1', selection: 'profile', profileKey: 'audio-v2' },
          tracks: { source: 'source_group', targetPath: '/raw', selection: 'profile', profileKey: 'tracks-v4' },
          destination: { source: 'source_group', targetPath: '/raw', selection: 'value', libraryId: 2 },
        }, audioProfileSnapshot: { key: 'audio-v2', name: 'Dialogue polish' },
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
  afterEach(() => cleanup());

  beforeEach(() => {
    vi.clearAllMocks();
    mocks.edit.mockImplementation(async (id: string, revision: number) => candidate({ revision: revision + 1, resolved: false }));
    mocks.update.mockImplementation(async (id: string, input: { revision: number; batchName: string; jobs: unknown[] }) => candidate({ revision: input.revision + 1, batchName: input.batchName }));
    mocks.accept.mockImplementation(async (id: string, revision: number) => candidate({ revision: revision + 1, assets: candidate().assets.map((asset) => ({ ...asset, status: 'queued' as const, queuedJobId: 32 })) }));
    mocks.remove.mockImplementation(async (_id: string, revision: number) => candidate({ revision: revision + 1 }));
    mocks.discard.mockImplementation(async (_id: string, revision: number) => candidate({ revision: revision + 1, status: 'discarded', assets: [] }));
    mocks.getCandidate.mockImplementation(async () => candidate());
    mocks.preview.mockResolvedValue({ requestId: 'preview-1' });
    mocks.waitPreview.mockResolvedValue({ state: 'ready', leaseToken: 'lease-1', ownerId: 'owner-1' });
    mocks.previewOwnerId.mockReturnValue('owner-1');
    mocks.cancelPreview.mockResolvedValue({ state: 'ready', leaseReleased: true });
    mocks.previewURL.mockReturnValue('/preview/preview-1');
    mocks.testEncode.mockResolvedValue({ id: 5 });
    mocks.open.mockReturnValue({ location: { href: '' } });
    vi.stubGlobal('open', mocks.open);
  });

  it('preserves edit inputs, invalidates and resolves again, then accepts the ready identity', async () => {
    const onAccepted = vi.fn();
    render(<BatchPreflightDialog
      candidate={candidate()}
      profiles={[profile as never, { ...profile, id: 8, name: 'Alternate profile', profileVersion: 2 } as never]}
      libraries={[{ id: 2, name: 'Preflight library', sourcePath: '/raw', destinationPath: '/library', type: 'video', validationRules: {}, createdAt: '', updatedAt: '' }]}
      settings={[
        { key: 'audioEnhancementProfiles', value: { profiles: [{ key: 'audio-v2', name: 'Dialogue polish' }] }, createdAt: '', updatedAt: '' },
        { key: 'trackProfiles', value: { profiles: [{ key: 'tracks-v4', name: 'English subtitles' }] }, createdAt: '', updatedAt: '' },
      ] as never}
      onClose={vi.fn()}
      onAccepted={onAccepted}
      onDiscarded={vi.fn()}
    />);

    expect(screen.getByText('Frozen profile v3')).toBeTruthy();
    expect(screen.getByText(/video: path/)).toBeTruthy();
    expect(screen.getByText(/Inherited from Path \/raw\/Season 1/)).toBeTruthy();
    expect(screen.getByText(/Dialogue polish · key audio-v2/)).toBeTruthy();
    expect(screen.getByText(/Destination library.*Preflight library/)).toBeTruthy();
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
    expect((screen.getByLabelText('Video profile') as HTMLInputElement).value).toBe('7');
    expect((screen.getByLabelText('Destination library') as HTMLInputElement).value).toBe('2');
    expect((screen.getByLabelText('Audio profile') as HTMLInputElement).value).toBe('audio-v2');
    expect((screen.getByLabelText('Track profile') as HTMLInputElement).value).toBe('tracks-v4');

    fireEvent.change(screen.getByLabelText('Batch name'), { target: { value: 'Season 1 revised' } });
    fireEvent.change(screen.getByLabelText('Track profile'), { target: { value: 'tracks-v5' } });
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

  it('shows before-and-after assignment names and requires explicit review before queueing', async () => {
    const asset = candidate().assets[0];
    const reviewCandidate = candidate({
      assets: [{
        ...asset,
        assignmentReview: {
          required: true,
          changes: [{
            domain: 'video',
            before: { source: 'path', targetPath: '/raw/Season 1', selection: 'profile', profileId: 7, profileName: 'Path archive', profileVersion: 3 },
            after: { source: 'queue_batch_preflight_edit', assignmentOrigin: 'path', targetPath: '/raw/Season 1', selection: 'profile', profileId: 8, profileName: 'Alternate profile', profileVersion: 2, candidateOverride: true },
          }],
        },
      }],
    });
    render(<BatchPreflightDialog candidate={reviewCandidate} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} profiles={[profile as never, { ...profile, id: 8, name: 'Alternate profile', profileVersion: 2 } as never]} />);

    expect(screen.getByText(/Path archive.*Inherited from Path/)).toBeTruthy();
    expect(screen.getByText(/Alternate profile.*Candidate-only override over Path/)).toBeTruthy();
    const accept = screen.getByRole('button', { name: /Accept & Queue/ }) as HTMLButtonElement;
    expect(accept.disabled).toBe(true);
    fireEvent.click(screen.getByLabelText('I reviewed the before-and-after assignment changes'));
    expect(accept.disabled).toBe(false);
    fireEvent.click(accept);
    await waitFor(() => expect(mocks.accept).toHaveBeenCalledWith('candidate-1', 1, ['asset-1'], true));
  });

  it('keeps stale and missing inventory references visible and blocks acceptance', () => {
    const staleAsset = candidate().assets[0];
    const missingCandidate = candidate({ assets: [{
      ...staleAsset,
      input: { ...staleAsset.input, profileId: 77, libraryId: 88 },
      assignmentChanges: [{ domain: 'video', reason: 'assignment_changed', before: { profileId: 7, profileName: 'Previous' }, current: { profileId: 9, profileName: 'Current' } }],
    }] });
    render(<BatchPreflightDialog candidate={missingCandidate} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} profiles={[]} libraries={[]} settings={[]} />);

    expect(screen.getByTestId('preflight-stale-assignment')).toBeTruthy();
    expect(screen.getByTestId('preflight-missing-inventory').textContent).toContain('Video Profile ID 77');
    expect(screen.getByTestId('preflight-missing-inventory').textContent).toContain('Library ID 88');
    expect((screen.getByRole('button', { name: /Accept & Queue/ }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('labels audio-only fallback separately from a reusable Video Profile selection', () => {
    const base = candidate().assets[0];
    render(<BatchPreflightDialog candidate={candidate({ assets: [{
      ...base,
      input: { ...base.input, profileId: 0, processingMode: 'audio_only' },
      plan: { ...base.plan!, processingMode: 'audio_only', profileResolution: { video: { source: 'path', targetPath: '/raw/Season 1', selection: 'audio_only' } } },
    }] })} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    expect(screen.getByText(/Audio-only fallback: Frozen profile v3/)).toBeTruthy();
  });

  it('reviews and accepts a reopened selected title with nullable snapshots across nested groups', async () => {
    const baseAsset = candidate().assets[0];
    const assetIds = Array.from({ length: 10 }, (_, index) => `series-asset-${index + 1}`);
    const assets = assetIds.map((id, index) => {
      const mediaPath = index < 8
        ? `/raw/Series/Season1/Episode${index + 1}.mkv`
        : `/raw/Series/Season0/Episode${index - 7}.mkv`;
      const plan = baseAsset.plan!;
      return {
        ...baseAsset,
        id,
        mediaPath,
        batchId: index < 8 ? 'group-season1' : 'group-season0',
        batchName: index < 8 ? 'Season 1' : 'Season0',
        input: { ...baseAsset.input, mediaPath },
        plan: {
          ...plan,
          profile: { ...profile, workerConfig: index === 1 ? null : profile.workerConfig } as never,
          profileSnapshot: index === 0 ? null : plan.profileSnapshot,
          profileResolution: index === 7 ? null : {
            tracks: { selection: 'disabled', source: 'source_group', targetPath: '/media/raw/series' },
          },
          audioProfileSnapshot: null,
          trackProfileSnapshot: null,
          constraints: index === 3 ? null : plan.constraints,
          qualityEvidence: index === 4
            ? null
            : index === 5
              ? { sourceBitrateEvidence: null, bpppfAssessment: null, profileQualification: null }
              : plan.qualityEvidence,
          destination: index === 2 ? null : plan.destination,
        },
      };
    });
    const reopenedCandidate = candidate({
      id: 'selected-title-candidate',
      revision: 17,
      batchId: 'logical-series-title',
      batchName: 'Complete Series',
      assets,
    });
    mocks.edit.mockResolvedValue({ ...reopenedCandidate, revision: 18, resolved: false });
    mocks.update.mockResolvedValue({ ...reopenedCandidate, revision: 19, resolved: true });
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {});
    try {
      render(<BatchPreflightDialog candidate={reopenedCandidate} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

      expect(screen.getByText('Batch Preflight · Complete Series')).toBeTruthy();
      for (const asset of assets) expect(screen.getByText(asset.mediaPath)).toBeTruthy();
      expect(screen.getAllByText('No Track Profile selected (disabled at Source Group)')).toHaveLength(9);
      expect(screen.getAllByText('Audio profile snapshot unavailable')).toHaveLength(10);
      expect(screen.getByText('Profile resolution unavailable')).toBeTruthy();
      expect(screen.getByText('Track Profile snapshot unavailable')).toBeTruthy();

      const queueAssetInputs = screen.getAllByLabelText('Queue this asset') as HTMLInputElement[];
      expect(queueAssetInputs).toHaveLength(10);
      expect(queueAssetInputs.every((input) => input.checked)).toBe(true);
      fireEvent.click(queueAssetInputs[9]);
      expect(screen.getByRole('button', { name: 'Accept & Queue (9)' })).toBeTruthy();
      fireEvent.click(queueAssetInputs[9]);
      expect(screen.getByRole('button', { name: 'Accept & Queue (10)' })).toBeTruthy();
      expect(mocks.accept).not.toHaveBeenCalled();

      fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      await screen.findByRole('button', { name: 'Resolve Preflight again' });
      expect(mocks.edit).toHaveBeenCalledWith('selected-title-candidate', 17);
      fireEvent.click(screen.getByRole('button', { name: 'Resolve Preflight again' }));
      await screen.findByRole('button', { name: 'Accept & Queue (10)' });
      expect(mocks.update).toHaveBeenCalledWith('selected-title-candidate', expect.objectContaining({
        revision: 18,
        batchId: 'logical-series-title',
        batchName: 'Complete Series',
        jobs: expect.arrayContaining(assets.map((asset) => expect.objectContaining({ mediaPath: asset.mediaPath }))),
      }));
      expect(mocks.accept).not.toHaveBeenCalled();

      fireEvent.click(screen.getByRole('button', { name: 'Accept & Queue (10)' }));
      await waitFor(() => expect(mocks.accept).toHaveBeenCalledWith('selected-title-candidate', 19, assetIds));
      expect(consoleError).not.toHaveBeenCalled();
    } finally {
      consoleError.mockRestore();
    }
  });

  it('keeps ambiguous auto IVTC out of Queue until a candidate-scoped TFF/BFF plan is resolved', async () => {
    const base = candidate().assets[0];
    const reviewAsset = {
      ...base,
      status: 'requires_review' as const,
      reviewCode: 'ivtc_field_order_required',
      cadenceReview: {
        cadenceType: 'hard_telecine', declaredFrameRate: '30000/1001', recommendedOperation: 'inverse_telecine', recommendedFrameRate: '24000/1001',
        reportedFieldOrder: 'tt', validatedSelectedOrder: '', validatedWindows: 1, observedWindows: 3,
        effectiveOutputFrameRateUnknown: true, hevcLevelResolutionState: 'unresolved', hevcLevelResolutionWarning: 'effective output FPS is unknown',
      },
      plan: undefined,
      blockers: ['Automatic IVTC requires an explicit or validated field order; choose TFF or BFF for this asset.'],
    };
    const review = candidate({ assets: [reviewAsset] });
    mocks.edit.mockResolvedValue({ ...review, revision: 2, resolved: false });
    mocks.update.mockImplementation(async (_id: string, input: QueueBatchPreflightInput) => candidate({
      revision: (input.revision ?? 1) + 1,
      assets: [{ ...base, input: { ...base.input, cadenceModeOverride: input.jobs[0]?.cadenceModeOverride, cadenceFieldOrder: input.jobs[0]?.cadenceFieldOrder }, plan: { ...base.plan!, profile: { ...profile, workerConfig: { ...profile.workerConfig, effectiveCadenceOperation: 'inverse_telecine', effectiveCadenceFieldOrder: 'tff', effectiveOutputFrameRate: '24000/1001', hevcLevelResolutionState: 'resolved', hevcLevelEffective: '4.1', hevcLevelTier: 'main' } } as never } }],
    }));
    render(<BatchPreflightDialog candidate={review} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    expect(await screen.findByText(/no explicit or validated field order/i)).toBeTruthy();
    expect(screen.getByText(/Detected hard_telecine at 30000\/1001/)).toBeTruthy();
    expect(screen.getByText(/reported stream field order: tt \(metadata only\)/i)).toBeTruthy();
    expect(screen.getByText(/IVTC windows: 1\/3 valid/i)).toBeTruthy();
    const acceptButton = screen.getByRole('button', { name: /Accept & Queue/ });
    expect(acceptButton.getAttribute('disabled')).not.toBeNull();

    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'Edit' }));
    const order = await screen.findByRole('combobox', { name: 'IVTC field order for this asset' });
    await user.click(order);
    await user.click(await screen.findByRole('option', { name: 'TFF' }));
    await user.click(screen.getByRole('button', { name: 'Resolve Preflight again' }));

    await waitFor(() => expect(mocks.update).toHaveBeenCalledWith('candidate-1', expect.objectContaining({
      revision: 2,
      jobs: [expect.objectContaining({ cadenceModeOverride: 'auto', cadenceFieldOrder: 'tff' })],
    })));
    expect(await screen.findAllByText(/Effective output FPS/)).not.toHaveLength(0);
    expect(screen.getByText('24000/1001')).toBeTruthy();
    expect(screen.getByText(/4\.1 main/)).toBeTruthy();
  });

  it('runs Deep Cadence Analysis, shows its window evidence, and re-resolves the candidate', async () => {
    const base = candidate().assets[0];
    const reviewAsset = {
      ...base,
      status: 'requires_review' as const,
      reviewCode: 'ivtc_field_order_required',
      cadenceReview: {
        cadenceType: 'hard_telecine', declaredFrameRate: '30000/1001', recommendedOperation: 'inverse_telecine', recommendedFrameRate: '24000/1001',
        reportedFieldOrder: 'tt', validatedSelectedOrder: '', validatedWindows: 1, observedWindows: 3,
        effectiveOutputFrameRateUnknown: true, hevcLevelResolutionState: 'unresolved', deepAnalysisAvailable: true,
        attemptedWindows: 3, inconclusiveWindows: 2, contradictoryWindows: 0,
        windows: [{ start: 0, outcome: 'valid', selectionBasis: 'initial analysis', selectedOrder: 'tff', diagnosticsAvailable: true, tffProgressiveRatio: 0.96, bffProgressiveRatio: 0.5, tffResidualInterlaced: 4, tffRepeatedTop: 2, tffRepeatedBottom: 1, tffCanonicalOutputFrames: 120, tffCanonicalOutputProgressiveFrames: 120, tffCanonicalOutputFrameRate: 24000 / 1001, tffCanonicalTimestampAnomalies: 0, tffCanonicalIdetInterlacedFrames: 12, tffCanonicalIdetClassifiedFrames: 120, bffResidualInterlaced: 9, bffRepeatedTop: 5, bffRepeatedBottom: 4, bffCanonicalOutputFrames: 120, bffCanonicalOutputProgressiveFrames: 120, bffCanonicalOutputFrameRate: 24000 / 1001, bffCanonicalTimestampAnomalies: 0, bffCanonicalIdetInterlacedFrames: 64, bffCanonicalIdetClassifiedFrames: 120, confidence: 0.96 }],
      },
      plan: undefined,
      blockers: ['Automatic IVTC requires an explicit or validated field order.'],
    };
    const review = candidate({ assets: [reviewAsset] });
    const resolved = candidate({ revision: 2, assets: [{ ...base, status: 'ready', cadenceReview: { ...reviewAsset.cadenceReview, deepAnalysisVersion: 2, deepAnalysisStatus: 'resolved', validatedSelectedOrder: 'tff', validatedWindows: 3, attemptedWindows: 5 } }] });
    mocks.startSnapshotOperation.mockResolvedValue({ id: 'deep-1', assetPath: base.mediaPath, status: 'completed', phase: 'completed', progress: 100, message: 'Deep Cadence Analysis evidence saved', durationMs: 80, cacheHit: false, incrementalRefresh: true, deepCadence: true, createdAt: '', updatedAt: '' });
    mocks.update.mockResolvedValue(resolved);
    render(<BatchPreflightDialog candidate={review} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    expect(screen.getByText(/attempted 3, valid 1, inconclusive 2, contradictory 0/i)).toBeTruthy();
    expect(screen.getByTestId('ivtc-window-evidence').textContent).toContain('initial analysis');
    expect(screen.getByTestId('ivtc-window-evidence').textContent).toContain('combmatch residual before fallback TFF/BFF 4/9');
    expect(screen.getByTestId('ivtc-window-evidence').textContent).toContain('pre-fallback idet interlaced/classified TFF 12/120, BFF 64/120');
    expect(screen.getByTestId('ivtc-window-evidence').textContent).toContain('repeated fields TFF top/bottom 2/1, BFF top/bottom 5/4');
    expect(screen.getByTestId('ivtc-window-evidence').textContent).toContain('canonical FPS TFF/BFF 23.976/23.976 · timestamp anomalies 0/0');
    await userEvent.setup().click(screen.getByRole('button', { name: 'Run Deep Cadence Analysis' }));

    await waitFor(() => expect(mocks.startSnapshotOperation).toHaveBeenCalledWith({ path: base.mediaPath, deepCadence: true }));
    expect(mocks.update).toHaveBeenCalledWith('candidate-1', expect.objectContaining({ revision: 1, jobs: [expect.objectContaining({ mediaPath: base.mediaPath })] }));
    expect(await screen.findByText('Deep Cadence Analysis evidence saved')).toBeTruthy();
    expect(screen.getByText('Cadence analysis evidence')).toBeTruthy();
    expect(screen.getByRole('button', { name: /Accept & Queue/ }).getAttribute('disabled')).toBeNull();
  });

  it('offers preserve cadence as an asset-scoped alternative and re-resolves its output FPS', async () => {
    const base = candidate().assets[0];
    const review = candidate({ assets: [{
      ...base,
      status: 'requires_review',
      reviewCode: 'ivtc_field_order_required',
      plan: undefined,
      cadenceReview: {
        cadenceType: 'hard_telecine', declaredFrameRate: '30000/1001', recommendedOperation: 'inverse_telecine', recommendedFrameRate: '24000/1001',
        reportedFieldOrder: 'tt', validatedWindows: 1, observedWindows: 3, effectiveOutputFrameRateUnknown: true, hevcLevelResolutionState: 'unresolved',
      },
      blockers: ['Automatic IVTC requires an explicit or validated field order; choose TFF or BFF for this asset.'],
    }] });
    mocks.edit.mockResolvedValue({ ...review, revision: 2, resolved: false });
    mocks.update.mockImplementation(async (_id: string, input: QueueBatchPreflightInput) => candidate({
      revision: (input.revision ?? 1) + 1,
      assets: [{ ...base, input: { ...base.input, cadenceModeOverride: input.jobs[0]?.cadenceModeOverride, cadenceFieldOrder: input.jobs[0]?.cadenceFieldOrder }, plan: { ...base.plan!, profileSnapshot: { ...base.plan!.profileSnapshot, cadenceAnalysisSnapshot: { declaredFrameRate: '30000/1001' } }, profile: { ...profile, workerConfig: { ...profile.workerConfig, cadenceMode: 'preserve', effectiveCadenceOperation: 'preserve', hevcLevelResolutionState: 'resolved', hevcLevelEffective: '4.0', hevcLevelTier: 'main' } } as never } }],
    }));
    render(<BatchPreflightDialog candidate={review} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole('button', { name: 'Edit' }));
    const decision = await screen.findByRole('combobox', { name: 'Cadence decision for this asset' });
    await user.click(decision);
    await user.click(await screen.findByRole('option', { name: 'Preserve cadence' }));
    await user.click(screen.getByRole('button', { name: 'Resolve Preflight again' }));
    await waitFor(() => expect(mocks.update).toHaveBeenCalledWith('candidate-1', expect.objectContaining({
      jobs: [expect.objectContaining({ cadenceModeOverride: 'preserve', cadenceFieldOrder: 'auto' })],
    })));
    expect(await screen.findByText(/30000\/1001 · source cadence preserved/)).toBeTruthy();
    expect(screen.getByText(/4\.0 main/)).toBeTruthy();
  });

  it('passes the exact candidate snapshot to an inline preview player and preserves Test Encode and LAB flows', async () => {
    const onClose = vi.fn();
    const view = render(<BatchPreflightDialog candidate={candidate()} onClose={onClose} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);
    let resolveReady!: (value: { state: string; leaseToken: string }) => void;
    mocks.waitPreview.mockImplementationOnce(() => new Promise((resolve) => { resolveReady = resolve; }));

    mocks.open.mockReturnValue(null);
    fireEvent.click(screen.getByRole('button', { name: 'Preview exact plan' }));
    fireEvent.click(screen.getByRole('button', { name: 'Preview exact plan' }));
    await waitFor(() => expect(mocks.preview).toHaveBeenCalledWith(expect.objectContaining({
      path: '/raw/Season 1/Episode 1.mkv', profile, ephemeral: true,
      preflightCandidateId: 'candidate-1', preflightRevision: 1, preflightAssetId: 'asset-1',
    }), expect.any(AbortSignal)));
    expect(mocks.waitPreview).toHaveBeenCalledWith('preview-1', expect.any(AbortSignal), 'owner-1');
    expect(await screen.findByText('Preview is encoding. This may take a few minutes…')).toBeTruthy();
    resolveReady({ state: 'ready', leaseToken: 'lease-1' });
    await waitFor(() => expect(document.querySelector('video')?.getAttribute('src')).toBe('/preview/preview-1'));
    expect(mocks.preview).toHaveBeenCalledTimes(1);
    expect(mocks.open).not.toHaveBeenCalled();
    fireEvent.error(document.querySelector('video')!);
    expect(await screen.findByText(/preview artifact is ready, but the browser could not play it/)).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Retry exact plan preview' })).toBeTruthy();

    mocks.waitPreview.mockResolvedValueOnce({ state: 'ready', leaseToken: 'lease-2', ownerId: 'owner-1' });
    fireEvent.click(screen.getByRole('button', { name: 'Retry exact plan preview' }));
    await waitFor(() => expect(mocks.preview).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(document.querySelector('video')?.getAttribute('src')).toBe('/preview/preview-1'));
    await waitFor(() => expect(mocks.cancelPreview).toHaveBeenCalledWith('preview-1', 'owner-1', 'lease-1'));

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
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    await waitFor(() => expect(mocks.cancelPreview).toHaveBeenCalledWith('preview-1', 'owner-1', 'lease-2'));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('plays an exact-plan cache hit that is already ready without acquiring or releasing a lease', async () => {
    const onClose = vi.fn();
    mocks.waitPreview.mockResolvedValueOnce({ state: 'ready', cacheHit: true });
    render(<BatchPreflightDialog candidate={candidate()} onClose={onClose} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Preview exact plan' }));
    await waitFor(() => expect(document.querySelector('video')?.getAttribute('src')).toBe('/preview/preview-1'));
    expect(screen.queryByText('Compatible Preview did not return a releasable lifecycle lease.')).toBeNull();
    expect(mocks.preview).toHaveBeenCalledTimes(1);
    expect(mocks.cancelPreview).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(mocks.cancelPreview).not.toHaveBeenCalled();
  });

  it('keeps a fresh ready response without a lease blocked unless it is a cache hit', async () => {
    mocks.waitPreview.mockResolvedValueOnce({ state: 'ready', cacheHit: false });
    render(<BatchPreflightDialog candidate={candidate()} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Preview exact plan' }));

    expect(await screen.findByText('Preview lifecycle failed: Compatible Preview did not return a releasable lifecycle lease.')).toBeTruthy();
    expect(document.querySelector('video')).toBeNull();
    expect(mocks.cancelPreview).not.toHaveBeenCalled();
  });

  it('shows a lifecycle error and retries the same frozen candidate plan', async () => {
    mocks.waitPreview.mockRejectedValueOnce(Object.assign(new Error('worker failed'), { state: 'failed' }));
    render(<BatchPreflightDialog candidate={candidate()} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Preview exact plan' }));
    expect(await screen.findByText('Preview lifecycle failed: worker failed')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Retry exact plan preview' }));

    await waitFor(() => expect(document.querySelector('video')?.getAttribute('src')).toBe('/preview/preview-1'));
    expect(mocks.preview).toHaveBeenCalledTimes(2);
    for (const [input] of mocks.preview.mock.calls) {
      expect(input).toMatchObject({
        path: '/raw/Season 1/Episode 1.mkv',
        profile,
        preflightCandidateId: 'candidate-1',
        preflightRevision: 1,
        preflightAssetId: 'asset-1',
      });
    }
  });

  it('shows stale candidate context distinctly and offers to reload the latest candidate', async () => {
    mocks.preview.mockRejectedValueOnce(Object.assign(new Error('candidate revision changed'), { code: 'batch_preflight_context_stale' }));
    render(<BatchPreflightDialog candidate={candidate()} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);

    fireEvent.click(screen.getByRole('button', { name: 'Preview exact plan' }));
    expect(await screen.findByText('This Batch Preflight snapshot is stale. Reload and review the latest candidate before previewing.')).toBeTruthy();
    fireEvent.click(screen.getByRole('button', { name: 'Reload latest candidate' }));
    await waitFor(() => expect(mocks.getCandidate).toHaveBeenCalledWith('candidate-1'));
  });

  it('aborts the preview lifecycle when the dialog unmounts', async () => {
    let resolveReady!: (value: { state: string; leaseToken: string }) => void;
    mocks.waitPreview.mockImplementationOnce(() => new Promise((resolve) => { resolveReady = resolve; }));
    const view = render(<BatchPreflightDialog candidate={candidate()} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'Preview exact plan' }));
    await waitFor(() => expect(mocks.waitPreview).toHaveBeenCalled());
    const signal = mocks.waitPreview.mock.calls[0][1] as AbortSignal;
    view.unmount();
    expect(signal.aborted).toBe(true);
    resolveReady({ state: 'ready', leaseToken: 'lease-1' });
    await waitFor(() => expect(mocks.cancelPreview).toHaveBeenCalledWith('preview-1', 'owner-1', 'lease-1'));
  });

  it('cancels an in-progress preview and releases a lease that arrives after cancellation', async () => {
    let resolveReady!: (value: { state: string; leaseToken: string }) => void;
    mocks.waitPreview.mockImplementationOnce(() => new Promise((resolve) => { resolveReady = resolve; }));
    render(<BatchPreflightDialog candidate={candidate()} onClose={vi.fn()} onAccepted={vi.fn()} onDiscarded={vi.fn()} />);
    fireEvent.click(screen.getByRole('button', { name: 'Preview exact plan' }));
    await waitFor(() => expect(mocks.waitPreview).toHaveBeenCalled());
    const signal = mocks.waitPreview.mock.calls[0][1] as AbortSignal;

    fireEvent.click(screen.getByRole('button', { name: 'Cancel preview' }));
    expect(signal.aborted).toBe(true);
    resolveReady({ state: 'ready', leaseToken: 'lease-after-cancel' });
    await waitFor(() => expect(mocks.cancelPreview).toHaveBeenCalledWith('preview-1', 'owner-1', 'lease-after-cancel'));
    await waitFor(() => expect(document.querySelector('video')).toBeNull());
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
