// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from '../api/client';
import type { Library, ProfileQualification, TestEncode } from '../api/types';
import { TestEncodeDialog } from './TestEncodeDialog';

vi.mock('../api/client', () => ({ api: {
  settings: vi.fn(), testEncodes: vi.fn(), jellyfinOutcomes: vi.fn(), retryJellyfinOutcome: vi.fn(), createTestEncode: vi.fn(), cancelTestEncode: vi.fn(), keepTestEncode: vi.fn(), deleteTestEncode: vi.fn(), evaluateTestEncodeProfileQualification: vi.fn(),
} }));

const readyTest = {
  id: 4, sourceAssetId: 1, sourcePath: '/raw/show/episode.mkv', sourceFingerprint: 'sha256:test', sourceSizeBytes: 100,
  libraryId: 2, configurationSource: 'effective_asset', requestedConfiguration: {}, effectiveConfiguration: {}, configurationHash: 'abc123',
  profileId: 9, profileVersion: 2, workerName: 'nas', effectiveEncoder: 'hevc_qsv', startSeconds: 40, durationSeconds: 20,
  status: 'ready', phase: 'ready', progress: 100, ffmpegCommand: 'ffmpeg -ss 40 -i input -t 20 output',
  outputPath: '/library/episode - MVForge Test T4.mkv', outputSizeBytes: 50, subtitleArtifacts: [], validationReport: { videoTracks: 1 },
  keep: false, reconciliationStatus: 'healthy', outputExists: true, artifactCount: 1, sidecarArtifactCount: 0, knownDiskUsageBytes: 50, unknownSizeArtifactCount: 0,
  errorMessage: '', createdAt: '2026-08-21T00:00:00Z', updatedAt: '2026-08-21T00:00:01Z',
  stale: false,
} satisfies TestEncode;

const testLibraries: Library[] = [{ id: 2, name: 'TV', sourcePath: '/raw', destinationPath: '/library', type: 'tv', validationRules: {}, createdAt: '', updatedAt: '' }];

const qualification: ProfileQualification = {
  id: 'qualification:41', testEncodeId: 41, sourceAssetId: 9, sourcePath: '/raw/show/episode.mkv',
  sourceRevision: 'source-revision', sourceFingerprint: 'source-fingerprint', profileId: 8, profileVersion: 3,
  resolvedProfileFingerprint: 'resolved-profile-v1:abc', candidateIdentity: 'candidate:41', candidateRevision: 'revision:41',
  samplingPlanSchemaVersion: 'sampling-schema-v1', samplingPlanPolicyVersion: 'sampling-policy-v1', samplingPlanIdentity: 'plan:41',
  perceptualEvidenceIdentity: 'evidence:41', referenceIdentity: 'reference:41', qualityPolicyVersion: 'profile-qualification-policy-v1',
  outcome: 'REVIEW_REQUIRED', rules: [{ key: 'metric.worst_window', outcome: 'REVIEW_REQUIRED', disposition: 'blocking',
    observed: 84, reference: 92, allowedTolerance: 0, windowIdentity: 'window:motion', sceneCategory: 'high_motion',
    explanation: 'The high motion window is below the MVForge policy floor.', evidenceProvenance: { pairIdentity: 'pair:motion' } }],
  warnings: ['Review the high motion sample.'], qualifiedScope: 'asset_profile_candidate_evidence',
  evidenceProvenance: { optimizationIntent: 'balanced' }, worstWindowIdentity: 'window:motion', worstSceneCategory: 'high_motion',
  worstRuleKeys: ['metric.worst_window'], createdAt: '2026-09-29T00:00:00Z',
};

function renderDialog(
  configurationSource: 'effective_asset' | 'lab_draft' = 'effective_asset',
  defaultLibraryId = 2,
  libraries = testLibraries,
  sampleRange?: { startSeconds: number; durationSeconds: number },
) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <TestEncodeDialog
        open
        onClose={() => undefined}
        sourcePath="/raw/show/episode.mkv"
        libraries={libraries}
        defaultLibraryId={defaultLibraryId}
        sampleRange={sampleRange}
        request={configurationSource === 'lab_draft'
          ? { configurationSource, labProfile: { name: 'Unsaved', videoCodec: 'x265' } as never, labAudioProfile: { filters: 'anull' }, labTrackOverride: { keepAudioStreams: [1] } }
          : { configurationSource, profileId: 9, resolveAssignments: true }}
      />
    </QueryClientProvider>,
  );
}

describe('TestEncodeDialog', () => {
  afterEach(cleanup);

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.settings).mockResolvedValue([]);
    vi.mocked(api.testEncodes).mockResolvedValue([]);
    vi.mocked(api.jellyfinOutcomes).mockResolvedValue([]);
    vi.mocked(api.retryJellyfinOutcome).mockResolvedValue({
      id: 91, reason: 'test_encode_ready', status: 'pending', attemptCount: 0, durationMillis: 0,
      updatedAt: '2026-09-27T12:00:00Z', testEncodeId: readyTest.id, retryEligible: false,
    });
    vi.mocked(api.createTestEncode).mockResolvedValue(readyTest);
    vi.mocked(api.deleteTestEncode).mockResolvedValue({ id: readyTest.id, status: 'deleted' });
    vi.mocked(api.evaluateTestEncodeProfileQualification).mockResolvedValue(qualification);
  });

  it('creates an independent test with the selected window and effective Asset configuration', async () => {
    const user = userEvent.setup();
    renderDialog();
    await user.click(screen.getByRole('combobox', { name: /sample position/i }));
    await user.click(await screen.findByRole('option', { name: /custom/i }));
    await user.type(screen.getByRole('spinbutton', { name: /start seconds/i }), '35');
    await user.click(screen.getByRole('button', { name: /generate new test encode/i }));
    await waitFor(() => expect(api.createTestEncode).toHaveBeenCalledWith(expect.objectContaining({
      sourcePath: '/raw/show/episode.mkv', libraryId: 2, configurationSource: 'effective_asset', profileId: 9,
      resolveAssignments: true, startMode: 'custom', startSeconds: 35, durationSeconds: 20,
    })));
  });

  it('uses the single LAB header range without repeating range controls in the run dialog', async () => {
    const user = userEvent.setup();
    renderDialog('lab_draft', 2, testLibraries, { startSeconds: 600, durationSeconds: 20 });

    expect(screen.getByText(/lab sample · 600s \+ 20s/i)).toBeTruthy();
    expect(screen.queryByRole('combobox', { name: /sample position/i })).toBeNull();
    expect(screen.queryByRole('spinbutton', { name: /duration/i })).toBeNull();
    await user.click(screen.getByRole('button', { name: /generate new test encode/i }));
    await waitFor(() => expect(api.createTestEncode).toHaveBeenCalledWith(expect.objectContaining({
      startMode: 'custom',
      startSeconds: 600,
      durationSeconds: 20,
    })));
  });

  it('labels unsaved Lab configuration and exposes lifecycle actions for prior tests', async () => {
    vi.mocked(api.testEncodes).mockResolvedValue([readyTest]);
    const user = userEvent.setup();
    renderDialog('lab_draft');
    expect(await screen.findByText(/uses the current lab configuration, including unsaved changes/i)).toBeTruthy();
    expect(await screen.findByText('/library/episode - MVForge Test T4.mkv')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: /details/i }));
    expect(screen.getByRole('tabpanel', { name: /summary/i })).toBeTruthy();
    expect(screen.queryByDisplayValue(/ffmpeg -ss 40/i)).toBeNull();
    await user.click(screen.getByRole('tab', { name: /ffmpeg/i }));
    expect(screen.getByDisplayValue(/ffmpeg -ss 40/i)).toBeTruthy();
    await user.click(screen.getByRole('tab', { name: /validation/i }));
    expect(screen.getByDisplayValue(/"videoTracks": 1/i)).toBeTruthy();
    expect(screen.getByDisplayValue('[]')).toBeTruthy();
  });

  it('preserves cancellation for a retained run that is still generating', async () => {
    vi.mocked(api.testEncodes).mockResolvedValue([{ ...readyTest, status: 'generating', phase: 'encoding', progress: 42 }]);
    const user = userEvent.setup();
    renderDialog('lab_draft');

    await user.click(await screen.findByRole('button', { name: /^cancel$/i }));
    await waitFor(() => expect(api.cancelTestEncode).toHaveBeenCalledWith(readyTest.id, expect.anything()));
  });

  it('keeps requested and effective configurations in a dedicated section', async () => {
    vi.mocked(api.testEncodes).mockResolvedValue([{
      ...readyTest,
      requestedConfiguration: { requestedEncoder: 'hevc_qsv' },
      effectiveConfiguration: { effectiveEncoder: 'hevc_qsv', gopRefDist: 1 },
    }]);
    const user = userEvent.setup();
    renderDialog();
    await user.click(await screen.findByRole('button', { name: /details/i }));
    await user.click(screen.getByRole('tab', { name: /configuration/i }));
    expect(screen.getByDisplayValue(/"requestedEncoder": "hevc_qsv"/i)).toBeTruthy();
    expect(screen.getByDisplayValue(/"gopRefDist": 1/i)).toBeTruthy();
  });

  it('shows Jellyfin status separately from READY and Keep', async () => {
    vi.mocked(api.testEncodes).mockResolvedValue([readyTest]);
    vi.mocked(api.jellyfinOutcomes).mockResolvedValue([{
      id: 91, reason: 'test_encode_ready', status: 'retryable_failure', failureClass: 'timeout', attemptCount: 5,
      durationMillis: 10000, updatedAt: '2026-09-27T12:00:00Z', testEncodeId: readyTest.id, retryEligible: true,
    }]);
    const user = userEvent.setup();
    renderDialog();
    expect(await screen.findByText(/Jellyfin: retryable failure/)).toBeTruthy();
    expect(screen.getAllByText('ready').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Keep').length).toBeGreaterThan(0);
    expect(screen.getByRole('button', { name: 'Retry Jellyfin refresh' })).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Retry Jellyfin refresh' }));
    await waitFor(() => expect(api.retryJellyfinOutcome).toHaveBeenCalledWith(91, expect.anything()));
  });

  it('shows rule explanations and jumps to the existing worst evidence window', async () => {
    vi.mocked(api.testEncodes).mockResolvedValue([{
      ...readyTest,
      id: qualification.testEncodeId,
      profileQualification: qualification,
      validationReport: { perceptualEvidence: { windows: [{ windowIdentity: 'window:motion', startSeconds: 22, durationSeconds: 5, roles: ['high_motion'], pairIdentity: { identity: 'pair:motion' }, metrics: [{ logicalMetric: 'VMAF', status: 'available', value: { kind: 'finite', value: 84 } }] }] } },
    }]);
    const user = userEvent.setup();
    renderDialog();
    await user.click(await screen.findByRole('button', { name: /details/i }));
    await user.click(screen.getByRole('tab', { name: /profile qualification/i }));
    expect(screen.getAllByText('REVIEW_REQUIRED').length).toBeGreaterThan(0);
    expect(screen.getByText('The high motion window is below the MVForge policy floor.')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: /compare worst scene/i }));
    expect(screen.getByText(/22s \+ 5s/)).toBeTruthy();
    expect(screen.getAllByText(/pair:motion/).length).toBeGreaterThan(0);
  });

  it('evaluates persisted evidence without creating another Test Encode', async () => {
    vi.mocked(api.testEncodes).mockResolvedValue([readyTest]);
    const user = userEvent.setup();
    renderDialog();
    await user.click(await screen.findByRole('button', { name: /details/i }));
    await user.click(screen.getByRole('tab', { name: /profile qualification/i }));
    await user.click(screen.getByRole('button', { name: /evaluate existing evidence/i }));
    await waitFor(() => expect(api.evaluateTestEncodeProfileQualification).toHaveBeenCalledWith(readyTest.id, expect.anything()));
    expect(api.createTestEncode).not.toHaveBeenCalled();
    expect((await screen.findAllByText('REVIEW_REQUIRED')).length).toBeGreaterThan(0);
  });

  it('opens without an inherited destination and allows choosing one inside', async () => {
    const user = userEvent.setup();
    renderDialog('effective_asset', 0, [
      ...testLibraries,
      { ...testLibraries[0], id: 3, name: 'Anime', destinationPath: '/library/anime' },
    ]);
    await user.click(screen.getByRole('combobox', { name: /destination library/i }));
    await user.click(await screen.findByRole('option', { name: 'Anime' }));
    await user.click(screen.getByRole('button', { name: /generate new test encode/i }));
    await waitFor(() => expect(api.createTestEncode).toHaveBeenCalledWith(expect.objectContaining({ libraryId: 3 })));
  });

  it('applies the Test Encode sample defaults from Settings', async () => {
    vi.mocked(api.settings).mockResolvedValue([{ key: 'testEncodePolicy', domain: 'test-encode', value: { defaultDurationSeconds: 42, defaultStartMode: 'middle' } } as never]);
    const user = userEvent.setup();
    renderDialog();
    expect(await screen.findByDisplayValue('42')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: /generate new test encode/i }));
    await waitFor(() => expect(api.createTestEncode).toHaveBeenCalledWith(expect.objectContaining({ startMode: 'middle', durationSeconds: 42 })));
  });

  it('passes explicit confirmation when deleting a Keep Test Encode', async () => {
    vi.mocked(api.testEncodes).mockResolvedValue([{ ...readyTest, keep: true }]);
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true);
    const user = userEvent.setup();
    renderDialog();
    await user.click(await screen.findByRole('button', { name: /^delete$/i }));
    await waitFor(() => expect(api.deleteTestEncode).toHaveBeenCalledWith(readyTest.id, true));
    expect(confirm).toHaveBeenCalledWith(expect.stringMatching(/delete keep test encode/i));
    confirm.mockRestore();
  });
});
