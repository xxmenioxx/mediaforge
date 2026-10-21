import { useEffect, useRef, useState } from 'react';
import {
  Alert,
  Box,
  Button,
  Checkbox,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  Grid,
  MenuItem,
  Stack,
  TextField,
  Typography,
} from '@mui/material';
import { api, createCompatiblePreviewOwnerId, waitForCompatiblePreviewReady } from '../api/client';
import { SourceBitrateQualityEvidence } from './SourceBitrateQualityEvidence';
import type {
  QueueBatchPreflightAsset,
  QueueBatchPreflightInput,
  QueueBatchPreflightPlan,
  QueueBatchPreflightResponse,
  QueueJobInput,
  SnapshotOperation,
} from '../api/types';

function asRecord<T extends object = Record<string, unknown>>(value: unknown): T | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  return value as T;
}

function recordText(record: unknown, key: string) {
  const value = asRecord<Record<string, unknown>>(record)?.[key];
  if (value === undefined || value === null || value === '') return '';
  return typeof value === 'object' ? JSON.stringify(value) : String(value);
}

function planSummary(asset: QueueBatchPreflightAsset) {
  const plan = asset.plan;
  if (!plan) return null;
  const profile = plan.profile as unknown as Record<string, unknown>;
  const workerConfig = asRecord(profile.workerConfig) ?? {};
  const profileSnapshot = asRecord(plan.profileSnapshot);
  const cadenceSnapshot = asRecord(profileSnapshot?.cadenceAnalysisSnapshot) ?? {};
  const profileResolution = asRecord(plan.profileResolution);
  const audioProfileSnapshot = asRecord(plan.audioProfileSnapshot);
  const trackProfileSnapshot = asRecord(plan.trackProfileSnapshot);
  const trackResolution = asRecord(profileResolution?.tracks);
  const qualityEvidence = asRecord<NonNullable<QueueBatchPreflightPlan['qualityEvidence']>>(plan.qualityEvidence);
  const cadenceMode = recordText(workerConfig, 'cadenceMode');
  const effectiveCadenceOperation = recordText(workerConfig, 'effectiveCadenceOperation') || cadenceMode;
  const outputFrameRate = workerConfig.effectiveOutputFrameRateUnknown === true
    ? 'Unknown'
    : recordText(workerConfig, 'effectiveOutputFrameRate') || (cadenceMode === 'preserve' && recordText(cadenceSnapshot, 'declaredFrameRate')
      ? `${recordText(cadenceSnapshot, 'declaredFrameRate')} · source cadence preserved`
      : 'from frozen plan');
  const destination = asRecord(plan.destination);
  const provenance = Object.entries(profileResolution ?? {}).flatMap(([domain, decision]) => {
    if (!asRecord(decision)) return [];
    const source = recordText(decision, 'source');
    return source ? [`${domain}: ${source}`] : [];
  }).join(' · ');
  const trackSelectionDisabled = recordText(trackResolution, 'selection') === 'disabled';
  const trackSource = recordText(trackResolution, 'source');
  const trackSummary = trackSelectionDisabled
    ? trackSource === 'source_group' ? 'No Track Profile selected (disabled at Source Group)' : 'No Track Profile selected (disabled)'
    : trackProfileSnapshot
      ? Object.keys(trackProfileSnapshot).length ? 'Frozen track selection and subtitle policy' : 'Preserved by track snapshot'
      : 'Track Profile snapshot unavailable';
  const items = [
    ['Profile', recordText(profile, 'name') || `Profile ${asset.input.profileId}`],
    ['Profile provenance', provenance || (profileResolution ? 'Frozen effective resolution' : 'Profile resolution unavailable')],
    ['Profile version', `v${plan.profileVersion}${plan.profileCapturedAt ? ` · captured ${plan.profileCapturedAt}` : ''}`],
    ['Encoder / codec', `${plan.selectedEncoder} / ${recordText(profile, 'videoCodec') || recordText(workerConfig, 'videoCodec') || 'configured codec'}`],
    ['Quality', [recordText(profile, 'qualityMode'), recordText(profile, 'qualityValue') || recordText(workerConfig, 'qualityValue')].filter(Boolean).join(' ') || 'from frozen profile'],
    ['Frame structure', [recordText(profile, 'frameRate'), recordText(profile, 'gopSize') || recordText(workerConfig, 'gopSize')].filter(Boolean).join(' · ') || 'preserved/resolved in snapshot'],
    ['Effective cadence', [effectiveCadenceOperation, recordText(workerConfig, 'effectiveCadenceFieldOrder')].filter(Boolean).join(' · ') || 'preserved/resolved in snapshot'],
    ['Effective output FPS', outputFrameRate],
    ['HEVC Level/Tier', [recordText(workerConfig, 'hevcLevelResolutionState'), [recordText(workerConfig, 'hevcLevelEffective'), recordText(workerConfig, 'hevcLevelTier')].filter(Boolean).join(' ')].filter(Boolean).join(' · ') || 'not applicable'],
    ['Geometry', `${recordText(profile, 'width') || recordText(workerConfig, 'width') || 'source'} × ${recordText(profile, 'height') || recordText(workerConfig, 'height') || 'source'}`],
    ['Crop / scale', [recordText(workerConfig, 'crop'), recordText(workerConfig, 'videoFilters')].filter(Boolean).join(' · ') || 'preserved/resolved in snapshot'],
    ['SAR / DAR', `${recordText(profile, 'sampleAspectRatio') || 'source'} / ${recordText(profile, 'displayAspectRatio') || 'resolved'}`],
    ['Color / HDR', [recordText(profile, 'colorPolicy'), recordText(workerConfig, 'colorPolicy')].filter(Boolean).join(' · ') || 'preserved/resolved in snapshot'],
    ['Restoration', recordText(workerConfig, 'restoration') || 'preserved/resolved in snapshot'],
    ['Audio actions', recordText(audioProfileSnapshot, 'key') || (audioProfileSnapshot ? Object.keys(audioProfileSnapshot).length ? 'Frozen audio profile' : 'Preserved by audio snapshot' : 'Audio profile snapshot unavailable')],
    ['Tracks / subtitles', trackSummary],
    ['Destination', recordText(destination, 'destinationRoot') || 'Unavailable'],
    ['Publish mode', plan.publishMode],
  ];
  return <Grid container spacing={0.75}>
    {items.map(([label, value]) => <Grid key={label} size={{ xs: 12, sm: 6 }}>
      <Typography variant="caption" color="text.secondary">{label}</Typography>
      <Typography variant="body2" sx={{ overflowWrap: 'anywhere' }}>{value}</Typography>
    </Grid>)}
    {qualityEvidence ? <Grid size={{ xs: 12 }}>
      <SourceBitrateQualityEvidence source={qualityEvidence.sourceBitrateEvidence ?? undefined} assessment={qualityEvidence.bpppfAssessment ?? undefined} />
      {qualityEvidence.profileQualification ? <ProfileQualificationPreflightEvidence qualification={qualityEvidence.profileQualification} /> : null}
    </Grid> : null}
    <Grid size={{ xs: 12 }}>
      <Typography variant="caption" color="text.secondary">Frozen plan evidence</Typography>
      <Box component="details" sx={{ mt: 0.5 }}>
        <Box component="summary" sx={{ cursor: 'pointer', typography: 'caption' }}>Profile provenance, audio, tracks, constraints and destination snapshots</Box>
        <Box component="pre" sx={{ overflow: 'auto', maxHeight: 240, whiteSpace: 'pre-wrap', fontSize: 11 }}>
          {JSON.stringify({ profile: plan.profileSnapshot, profileResolution: plan.profileResolution, qualityEvidence: plan.qualityEvidence, audio: plan.audioProfileSnapshot, tracks: plan.trackProfileSnapshot, constraints: plan.constraints, destination: plan.destination }, null, 2)}
        </Box>
      </Box>
    </Grid>
  </Grid>;
}

function ProfileQualificationPreflightEvidence({ qualification }: { qualification: NonNullable<NonNullable<NonNullable<QueueBatchPreflightAsset['plan']>['qualityEvidence']>['profileQualification']> }) {
  const status = qualification.status;
  const severity = status === 'PASS' ? 'success' : status === 'FAIL' ? 'error' : status === 'PASS_WITH_WARNINGS' ? 'warning' : 'warning';
  return (
    <Stack spacing={0.75} sx={{ mt: 1 }} data-testid="preflight-profile-qualification">
      <Alert severity={severity}>
        <Typography variant="body2" sx={{ fontWeight: 600 }}>Profile Qualification: {status}</Typography>
        {qualification.message ? <Typography variant="body2">{qualification.message}</Typography> : null}
      </Alert>
      {qualification.qualificationId ? <Typography variant="caption" color="text.secondary">
        Policy {qualification.policyVersion} · qualification {qualification.qualificationId} · Test Encode T{qualification.testEncodeId}
      </Typography> : null}
      {qualification.worstWindowIdentity ? <Typography variant="body2">
        Worst evidence window: {qualification.worstSceneCategory?.replace(/_/g, ' ') || 'scene'} · {qualification.worstWindowIdentity}
      </Typography> : null}
      {qualification.warnings?.map((warning, index) => <Alert key={`${index}-${warning}`} severity="warning">{warning}</Alert>)}
      {qualification.rules?.length ? <Box component="details">
        <Box component="summary" sx={{ cursor: 'pointer', typography: 'caption' }}>Reviewed qualification rules and evidence</Box>
        <Stack spacing={0.5} sx={{ mt: 0.75 }}>
          {qualification.rules.map((rule, index) => <Box key={`${rule.key}-${index}`} sx={{ border: 1, borderColor: 'divider', borderRadius: 1, p: 0.75 }}>
            <Typography variant="caption" sx={{ fontWeight: 600 }}>{rule.outcome} · {rule.key}{rule.sceneCategory ? ` · ${rule.sceneCategory.replace(/_/g, ' ')}` : ''}</Typography>
            <Typography variant="body2">{rule.explanation}</Typography>
            <Typography variant="caption" color="text.secondary">Window {rule.windowIdentity || 'not window-specific'} · observed {recordText({ value: rule.observed }, 'value') || 'unavailable'} · reference {recordText({ value: rule.reference }, 'value') || 'unavailable'} · tolerance {recordText({ value: rule.allowedTolerance }, 'value') || 'unavailable'}</Typography>
          </Box>)}
        </Stack>
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.75, overflowWrap: 'anywhere' }}>
          Frozen evidence provenance: {JSON.stringify({ sourceRevision: qualification.sourceRevision, sourceFingerprint: qualification.sourceFingerprint, profileFingerprint: qualification.resolvedProfileFingerprint, evidence: qualification.perceptualEvidenceIdentity, reference: qualification.referenceIdentity, candidate: qualification.candidateIdentity })}
        </Typography>
      </Box> : null}
    </Stack>
  );
}

type CompatiblePreviewLease = {
  requestId: string;
  ownerId: string;
  leaseToken: string;
};

function releaseCompatiblePreviewLease(lease: CompatiblePreviewLease | null) {
  if (!lease) return;
  void api.cancelCompatiblePreview(lease.requestId, lease.ownerId, lease.leaseToken).catch(() => undefined);
}

export function BatchPreflightDialog({
  candidate: initialCandidate,
  onClose,
  onAccepted,
  onDiscarded,
}: {
  candidate: QueueBatchPreflightResponse | null;
  onClose: () => void;
  onAccepted: (candidate: QueueBatchPreflightResponse) => void;
  onDiscarded: (candidate: QueueBatchPreflightResponse) => void;
}) {
  const [candidate, setCandidate] = useState(initialCandidate);
  const [selected, setSelected] = useState<string[]>(() => initialCandidate?.assets.filter((asset) => asset.status === 'ready').map((asset) => asset.id) ?? []);
  const [editing, setEditing] = useState(initialCandidate?.resolved === false);
  const [batchName, setBatchName] = useState(initialCandidate?.batchName ?? '');
  const [jobs, setJobs] = useState<QueueJobInput[]>(() => initialCandidate?.assets.filter((asset) => asset.status !== 'queued').map((asset) => ({ ...asset.input })) ?? []);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [deepOperation, setDeepOperation] = useState<SnapshotOperation | null>(null);
  const [deepOperationPath, setDeepOperationPath] = useState('');
  const [confirmingDiscard, setConfirmingDiscard] = useState(false);
  const [editBaseline, setEditBaseline] = useState<{ batchName: string; jobs: QueueJobInput[] } | null>(null);
  const [previewState, setPreviewState] = useState<'idle' | 'creating' | 'encoding' | 'ready' | 'error'>('idle');
  const [previewUrl, setPreviewUrl] = useState('');
  const [previewAsset, setPreviewAsset] = useState<QueueBatchPreflightAsset | null>(null);
  const [previewError, setPreviewError] = useState('');
  const previewController = useRef<AbortController | null>(null);
  const previewLease = useRef<CompatiblePreviewLease | null>(null);

  useEffect(() => () => {
    previewController.current?.abort();
    const lease = previewLease.current;
    previewLease.current = null;
    releaseCompatiblePreviewLease(lease);
  }, []);

  useEffect(() => {
    if (!initialCandidate) {
      setCandidate(null);
      return;
    }
    setCandidate(initialCandidate);
    setSelected(initialCandidate.assets.filter((asset) => asset.status === 'ready').map((asset) => asset.id));
    setBatchName(initialCandidate.batchName);
    setJobs(initialCandidate.assets.filter((asset) => asset.status !== 'queued').map((asset) => ({ ...asset.input })));
    setEditing(!initialCandidate.resolved);
    setConfirmingDiscard(false);
    setEditBaseline(initialCandidate.resolved ? null : {
      batchName: initialCandidate.batchName,
      jobs: initialCandidate.assets.filter((asset) => asset.status !== 'queued').map((asset) => ({ ...asset.input })),
    });
    setError('');
  }, [initialCandidate]);

  if (!candidate) return null;

  const updateJob = (index: number, patch: Partial<QueueJobInput>) => {
    setJobs((current) => current.map((job, jobIndex) => jobIndex === index ? { ...job, ...patch } : job));
  };

  const beginEdit = async () => {
    setBusy(true); setError('');
    try {
      const invalidated = await api.editQueueBatchPreflight(candidate.id, candidate.revision);
      setCandidate(invalidated);
      setBatchName(invalidated.batchName);
      setJobs(invalidated.assets.filter((asset) => asset.status !== 'queued').map((asset) => ({ ...asset.input })));
      setEditBaseline({ batchName: invalidated.batchName, jobs: invalidated.assets.filter((asset) => asset.status !== 'queued').map((asset) => ({ ...asset.input })) });
      setSelected([]);
      setEditing(true);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const resolveEdits = async () => {
    setBusy(true); setError('');
    try {
      const input: QueueBatchPreflightInput = { revision: candidate.revision, batchId: candidate.batchId, batchName, jobs };
      const resolved = await api.updateQueueBatchPreflight(candidate.id, input);
      setCandidate(resolved);
      setSelected(resolved.assets.filter((asset) => asset.status === 'ready').map((asset) => asset.id));
      setEditing(false);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const accept = async () => {
    setBusy(true); setError('');
    try {
      const accepted = await api.acceptQueueBatchPreflight(candidate.id, candidate.revision, selected);
      setCandidate(accepted);
      onAccepted(accepted);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const removeAsset = async (asset: QueueBatchPreflightAsset) => {
    setBusy(true); setError('');
    try {
      const updated = await api.removeQueueBatchPreflightAsset(candidate.id, candidate.revision, asset.id);
      if (updated.status !== 'active') {
        onDiscarded(updated);
        return;
      }
      setCandidate(updated);
      setSelected((current) => current.filter((id) => id !== asset.id));
      setJobs((current) => current.filter((job) => job.mediaPath !== asset.mediaPath));
      setEditBaseline((current) => current ? {
        ...current,
        jobs: current.jobs.filter((job) => job.mediaPath !== asset.mediaPath),
      } : null);
      setConfirmingDiscard(false);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const discardCandidate = async () => {
    setBusy(true); setError('');
    try {
      const discarded = await api.discardQueueBatchPreflight(candidate.id, candidate.revision);
      onDiscarded(discarded);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const reloadLatestCandidate = async () => {
    setBusy(true); setError('');
    try {
      const latest = await api.getQueueBatchPreflight(candidate.id);
      if (latest.status !== 'active') {
        onDiscarded(latest);
        return;
      }
      const unresolved = latest.assets.filter((asset) => asset.status !== 'queued');
      const latestJobs = unresolved.map((asset) => ({ ...asset.input }));
      setCandidate(latest);
      setSelected(latest.assets.filter((asset) => asset.status === 'ready').map((asset) => asset.id));
      setBatchName(latest.batchName);
      setJobs(latestJobs);
      setEditing(!latest.resolved);
      setEditBaseline(latest.resolved ? null : { batchName: latest.batchName, jobs: latestJobs });
      setConfirmingDiscard(false);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const runPreview = async (asset: QueueBatchPreflightAsset) => {
    if (!asset.plan || busy) return;
    previewController.current?.abort();
    const previousLease = previewLease.current;
    previewLease.current = null;
    releaseCompatiblePreviewLease(previousLease);
    const controller = new AbortController();
    previewController.current = controller;
    const ownerId = createCompatiblePreviewOwnerId();
    const timeout = window.setTimeout(() => controller.abort(new DOMException('Compatible Preview timed out after 5 minutes.', 'TimeoutError')), 5 * 60 * 1000);
    let requestCreated = false;
    setPreviewAsset(asset);
    setPreviewUrl('');
    setPreviewError('');
    setPreviewState('creating');
    setBusy(true); setError('');
    try {
      const preview = await api.createCompatiblePreviewRequest({
        path: asset.mediaPath,
        profile: asset.plan.profile,
        preflightCandidateId: candidate.id,
        preflightRevision: candidate.revision,
        preflightAssetId: asset.id,
        ephemeral: true,
      }, controller.signal);
      requestCreated = true;
      setPreviewState('encoding');
      const ready = await waitForCompatiblePreviewReady(preview.requestId, controller.signal, ownerId);
      if (ready.state !== 'ready') throw new Error('Compatible Preview did not produce a ready artifact.');
      if (!ready.leaseToken) throw new Error('Compatible Preview did not return a releasable lifecycle lease.');
      const lease = { requestId: preview.requestId, ownerId, leaseToken: ready.leaseToken };
      if (controller.signal.aborted) {
        releaseCompatiblePreviewLease(lease);
        if (previewController.current === controller) setPreviewState('idle');
        return;
      }
      previewLease.current = lease;
      if (!controller.signal.aborted) {
        setPreviewUrl(api.compatibleAssetPreviewUrl(preview.requestId));
        setPreviewState('ready');
      }
    } catch (caught) {
      if (!controller.signal.aborted) {
        const failure = caught as { code?: string; state?: string } | null;
        const code = failure?.code;
        const message = caught instanceof Error ? caught.message : String(caught);
        setPreviewError(code === 'batch_preflight_context_stale'
          ? 'This Batch Preflight snapshot is stale. Reload and review the latest candidate before previewing.'
          : requestCreated
            ? `Preview lifecycle ${failure?.state ?? code ?? 'failed'}: ${message}`
            : `Preview request rejected: ${message}`);
        setPreviewState('error');
      } else if (controller.signal.reason instanceof DOMException && controller.signal.reason.name === 'TimeoutError') {
        setPreviewError(controller.signal.reason.message);
        setPreviewState('error');
      } else if (previewController.current === controller) {
        setPreviewState('idle');
      }
    } finally {
      window.clearTimeout(timeout);
      if (previewController.current === controller) previewController.current = null;
      setBusy(false);
    }
  };

  const closeDialog = () => {
    previewController.current?.abort();
    const lease = previewLease.current;
    previewLease.current = null;
    releaseCompatiblePreviewLease(lease);
    onClose();
  };

  const runTestEncode = async (asset: QueueBatchPreflightAsset) => {
    if (!asset.plan) return;
    setBusy(true); setError('');
    try {
      await api.createTestEncode({
        sourcePath: asset.mediaPath,
        libraryId: asset.input.libraryId,
        configurationSource: 'batch_preflight',
        preflightCandidateId: candidate.id,
        preflightRevision: candidate.revision,
        preflightAssetId: asset.id,
        startMode: 'beginning',
        durationSeconds: 60,
      });
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const runDeepCadence = async (asset: QueueBatchPreflightAsset) => {
    setBusy(true); setError('');
    setDeepOperationPath(asset.mediaPath);
    try {
      let operation = await api.startSnapshotOperation({ path: asset.mediaPath, deepCadence: true });
      setDeepOperation(operation);
      while (operation.status === 'running') {
        await new Promise((resolve) => window.setTimeout(resolve, 500));
        operation = await api.snapshotOperation(operation.id);
        setDeepOperation(operation);
      }
      if (operation.status === 'cancelled') return;
      if (operation.status !== 'completed') throw new Error(operation.error || 'Deep Cadence Analysis did not complete.');
      const refreshed = await api.updateQueueBatchPreflight(candidate.id, {
        revision: candidate.revision,
        batchId: candidate.batchId,
        batchName: candidate.batchName,
        jobs: candidate.assets.filter((item) => item.status !== 'queued').map((item) => ({ ...item.input })),
      });
      setCandidate(refreshed);
      setJobs(refreshed.assets.filter((item) => item.status !== 'queued').map((item) => ({ ...item.input })));
      setSelected(refreshed.assets.filter((item) => item.status === 'ready').map((item) => item.id));
      setEditing(false);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const cancelDeepCadence = async () => {
    if (!deepOperation || deepOperation.status !== 'running') return;
    try {
      setDeepOperation(await api.cancelSnapshotOperation(deepOperation.id));
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    }
  };

  const openLab = (asset: QueueBatchPreflightAsset) => {
    const params = new URLSearchParams({ preflightCandidateId: candidate.id, preflightRevision: String(candidate.revision), assetId: asset.id });
    window.open(`/profile-lab?${params.toString()}`, '_blank', 'noopener,noreferrer');
  };

  const discardEdits = async () => {
    if (!editBaseline) { setEditing(false); return; }
    setBusy(true); setError('');
    try {
      const restored = await api.updateQueueBatchPreflight(candidate.id, {
        revision: candidate.revision,
        batchId: candidate.batchId,
        batchName: editBaseline.batchName,
        jobs: editBaseline.jobs,
      });
      setCandidate(restored);
      setSelected(restored.assets.filter((asset) => asset.status === 'ready').map((asset) => asset.id));
      setEditing(false);
      setEditBaseline(null);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
    } finally { setBusy(false); }
  };

  const unresolvedCount = candidate.assets.filter((asset) => asset.status !== 'queued').length;
  const blockerCount = candidate.assets.filter((asset) => asset.status !== 'queued' && asset.blockers.length > 0).length;

  return <Dialog open onClose={busy ? undefined : closeDialog} fullWidth maxWidth="lg" aria-labelledby="batch-preflight-title">
    <DialogTitle id="batch-preflight-title">Batch Preflight · {candidate.batchName || 'Selected assets'}</DialogTitle>
    <DialogContent dividers>
      <Stack spacing={2}>
        <Alert severity={candidate.resolved ? 'info' : 'warning'}>
          Reviewed plan is the plan frozen into Queue. {candidate.assets.filter((asset) => asset.status === 'queued').length} queued; {unresolvedCount} remain in this candidate.
        </Alert>
        {error ? <Alert severity="error">{error}</Alert> : null}
        {error ? <Button size="small" disabled={busy} onClick={() => void reloadLatestCandidate()}>Reload latest candidate</Button> : null}
        {previewState !== 'idle' && previewAsset ? <Box sx={{ p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1 }}>
          <Stack spacing={1}>
            <Typography variant="subtitle2">Exact plan preview · {previewAsset.mediaPath}</Typography>
            {previewState === 'creating' ? <Alert severity="info">Creating the preview request…</Alert> : null}
            {previewState === 'encoding' ? <Alert severity="info">Preview is encoding. This may take a few minutes…</Alert> : null}
            {previewState === 'error' ? <Alert severity="error">{previewError}</Alert> : null}
            {previewState === 'error' ? <Stack direction="row" spacing={1}>
              <Button size="small" disabled={busy} onClick={() => void runPreview(previewAsset)}>Retry exact plan preview</Button>
              {previewError.startsWith('This Batch Preflight snapshot is stale') ? <Button size="small" disabled={busy} onClick={() => void reloadLatestCandidate()}>Reload latest candidate</Button> : null}
            </Stack> : null}
            {previewState === 'ready' ? <video controls playsInline preload="metadata" src={previewUrl} onError={() => {
              setPreviewError('The preview artifact is ready, but the browser could not play it. Retry the preview or check artifact delivery and video compatibility.');
              setPreviewState('error');
            }} style={{ display: 'block', width: '100%', maxHeight: '60vh', background: '#000' }} /> : null}
            {busy ? <Button size="small" onClick={() => previewController.current?.abort()}>Cancel preview</Button> : null}
          </Stack>
        </Box> : null}
        {confirmingDiscard ? <Alert severity="warning" action={<Stack direction="row" spacing={1}>
          <Button color="inherit" size="small" disabled={busy} onClick={() => setConfirmingDiscard(false)}>Keep candidate</Button>
          <Button color="error" size="small" disabled={busy} onClick={() => void discardCandidate()}>Discard pending assets</Button>
        </Stack>}>
          Discard {unresolvedCount} unqueued asset(s) from this candidate? Previously accepted Queue jobs remain unchanged.
        </Alert> : null}
        {editing ? <>
          <TextField label="Batch name" value={batchName} onChange={(event) => setBatchName(event.target.value)} fullWidth size="small" />
          {jobs.map((job, index) => <Box key={`${job.mediaPath}-${index}`} sx={{ p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1 }}>
            {(() => {
              const asset = candidate.assets.find((item) => item.mediaPath === job.mediaPath && item.status !== 'queued');
              const needsCadenceReview = asset?.status === 'requires_review' && (asset.reviewCode === 'ivtc_field_order_required' || asset.reviewCode === 'cadence_output_fps_unknown');
              if (!needsCadenceReview && !job.cadenceFieldOrder && !job.cadenceModeOverride) return null;
              const cadenceMode = job.cadenceModeOverride ?? 'auto';
              return <Grid container spacing={1} sx={{ mb: 1 }}>
                <Grid size={{ xs: 12, md: 6 }}>
                  <TextField select label="Cadence decision for this asset" size="small" fullWidth value={cadenceMode} onChange={(event) => {
                    const mode = event.target.value as QueueJobInput['cadenceModeOverride'];
                    updateJob(index, { cadenceModeOverride: mode, cadenceFieldOrder: mode === 'preserve' ? 'auto' : (job.cadenceFieldOrder ?? 'auto') });
                  }} helperText="This decision applies only to this candidate asset. Preserve cadence is re-resolved against source FPS and HEVC constraints.">
                    <MenuItem value="auto">Auto (validated analysis only)</MenuItem>
                    <MenuItem value="preserve">Preserve cadence</MenuItem>
                    <MenuItem value="inverse_telecine">Inverse telecine</MenuItem>
                  </TextField>
                </Grid>
                {cadenceMode !== 'preserve' ? <Grid size={{ xs: 12, md: 6 }}>
                  <TextField select label="IVTC field order for this asset" size="small" fullWidth value={job.cadenceFieldOrder ?? 'auto'} onChange={(event) => updateJob(index, { cadenceModeOverride: cadenceMode, cadenceFieldOrder: event.target.value as QueueJobInput['cadenceFieldOrder'] })} helperText="Only an explicit choice or validated SelectedOrder resolves IVTC. Stream metadata alone does not select TFF/BFF.">
                    <MenuItem value="auto">Auto (validated analysis only)</MenuItem>
                    <MenuItem value="tff">TFF</MenuItem>
                    <MenuItem value="bff">BFF</MenuItem>
                  </TextField>
                </Grid> : null}
              </Grid>;
            })()}
            <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ sm: 'center' }} gap={1} sx={{ mb: 1 }}>
              <Typography variant="subtitle2" sx={{ overflowWrap: 'anywhere' }}>{job.mediaPath}</Typography>
              <Button size="small" color="error" disabled={busy || !candidate.assets.some((asset) => asset.mediaPath === job.mediaPath && asset.status !== 'queued')} onClick={() => {
                const asset = candidate.assets.find((item) => item.mediaPath === job.mediaPath && item.status !== 'queued');
                if (asset) void removeAsset(asset);
              }}>Remove from candidate</Button>
            </Stack>
            <Grid container spacing={1}>
              <Grid size={{ xs: 6, md: 2 }}><TextField label="Profile ID" type="number" size="small" value={job.profileId} onChange={(event) => updateJob(index, { profileId: Number(event.target.value) })} fullWidth /></Grid>
              <Grid size={{ xs: 6, md: 2 }}><TextField label="Library ID" type="number" size="small" value={job.libraryId} onChange={(event) => updateJob(index, { libraryId: Number(event.target.value) })} fullWidth /></Grid>
              <Grid size={{ xs: 12, md: 3 }}><TextField label="Audio profile key" size="small" value={job.audioProfileKey ?? ''} onChange={(event) => updateJob(index, { audioProfileKey: event.target.value })} fullWidth /></Grid>
              <Grid size={{ xs: 12, md: 3 }}><TextField label="Track profile key" size="small" value={job.trackProfileKey ?? ''} onChange={(event) => updateJob(index, { trackProfileKey: event.target.value })} fullWidth /></Grid>
              <Grid size={{ xs: 12, md: 2 }}><TextField select label="Processing" size="small" value={job.processingMode ?? 'full_encode'} onChange={(event) => updateJob(index, { processingMode: event.target.value as QueueJobInput['processingMode'] })} fullWidth><MenuItem value="full_encode">Full encode</MenuItem><MenuItem value="audio_only">Audio only</MenuItem></TextField></Grid>
            </Grid>
          </Box>)}
        </> : candidate.assets.map((asset) => <Box key={asset.id} sx={{ p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1 }}>
          <Stack spacing={1.25}>
            <Stack direction={{ xs: 'column', sm: 'row' }} alignItems={{ sm: 'center' }} justifyContent="space-between" gap={1}>
              <Stack direction="row" alignItems="center" flexWrap="wrap" useFlexGap gap={1}>
                {asset.status === 'ready' ? <FormControlLabel control={<Checkbox checked={selected.includes(asset.id)} onChange={(event) => setSelected((current) => event.target.checked ? [...current, asset.id] : current.filter((id) => id !== asset.id))} />} label="Queue this asset" /> : null}
                <Typography variant="subtitle2" sx={{ overflowWrap: 'anywhere' }}>{asset.mediaPath}</Typography>
              </Stack>
                <Chip size="small" label={asset.status} color={asset.status === 'ready' || asset.status === 'queued' ? 'success' : asset.status === 'review' || asset.status === 'requires_review' ? 'warning' : 'error'} />
            </Stack>
            {asset.status === 'requires_review' && asset.reviewCode === 'ivtc_field_order_required' ? <Alert severity="warning">
              Automatic IVTC has no explicit or validated field order. Stream metadata alone does not select TFF/BFF. Use Edit to choose an order for this asset, resolve Preflight again, then review the resulting plan.
            </Alert> : null}
            {asset.cadenceReview && (asset.status === 'requires_review' || (asset.cadenceReview.deepAnalysisVersion ?? 0) > 0) ? <Alert severity={asset.status === 'requires_review' ? 'warning' : 'info'} data-testid="preflight-cadence-review">
              <Stack spacing={0.5}>
                <Typography variant="subtitle2">{asset.status === 'requires_review' ? 'Cadence review required before Queue' : 'Cadence analysis evidence'}</Typography>
                <Typography variant="body2">
                  Detected {asset.cadenceReview.cadenceType || 'unknown cadence'} at {asset.cadenceReview.declaredFrameRate || 'unknown source FPS'}; recommended {asset.cadenceReview.recommendedOperation || 'review'}{asset.cadenceReview.recommendedFrameRate ? ` to ${asset.cadenceReview.recommendedFrameRate}` : ''}.
                </Typography>
                <Typography variant="body2">
                  Reported stream field order: {asset.cadenceReview.reportedFieldOrder || 'unknown'} (metadata only). Validated SelectedOrder: {asset.cadenceReview.validatedSelectedOrder || 'none'}; IVTC windows: {asset.cadenceReview.validWindows ?? asset.cadenceReview.validatedWindows}/{asset.cadenceReview.observedWindows || 'unknown'} valid.
                </Typography>
                <Typography variant="body2">
                  Effective output FPS: {asset.cadenceReview.effectiveOutputFrameRateUnknown ? 'Unknown' : asset.cadenceReview.effectiveOutputFrameRate || 'unavailable'} · HEVC Level/Tier: {asset.cadenceReview.hevcLevelResolutionState || 'unresolved'}{asset.cadenceReview.hevcLevelResolutionWarning ? ` — ${asset.cadenceReview.hevcLevelResolutionWarning}` : ''}
                </Typography>
                <Typography variant="body2">
                  Deep analysis: {asset.cadenceReview.deepAnalysisStatus || 'not_run'} · attempted {asset.cadenceReview.attemptedWindows ?? asset.cadenceReview.observedWindows}, valid {asset.cadenceReview.validWindows ?? asset.cadenceReview.validatedWindows}, inconclusive {asset.cadenceReview.inconclusiveWindows ?? 0}, contradictory {asset.cadenceReview.contradictoryWindows ?? 0}.
                </Typography>
                {asset.cadenceReview.deepAnalysisVersion ? <Typography variant="body2">
                  Source FPS {asset.cadenceReview.sourceFrameRate || asset.cadenceReview.declaredFrameRate || 'unknown'} · expected IVTC output {asset.cadenceReview.expectedOutputFrameRate || 'unknown'}; effective FPS is still resolved by the frozen Queue plan.
                </Typography> : null}
                {asset.cadenceReview.windows?.length ? <Box component="details" sx={{ mt: 0.5 }}>
                  <Box component="summary" sx={{ cursor: 'pointer', typography: 'caption' }}>IVTC window evidence</Box>
                  <Stack spacing={0.5} sx={{ mt: 0.75 }}>
                    {asset.cadenceReview.windows.map((window, index) => <Typography key={`${window.start}-${index}`} variant="caption" data-testid="ivtc-window-evidence">
                      {window.start.toFixed(1)}s · {window.outcome || 'unknown'} · TFF {window.tffProgressiveRatio.toFixed(2)} / BFF {window.bffProgressiveRatio.toFixed(2)} · combmatch residual before fallback TFF/BFF {window.tffResidualInterlaced ?? 'unknown'}/{window.bffResidualInterlaced ?? 'unknown'} · pre-fallback idet interlaced/classified TFF {window.tffCanonicalIdetInterlacedFrames ?? 'unknown'}/{window.tffCanonicalIdetClassifiedFrames ?? 'unknown'}, BFF {window.bffCanonicalIdetInterlacedFrames ?? 'unknown'}/{window.bffCanonicalIdetClassifiedFrames ?? 'unknown'} · repeated fields TFF top/bottom {window.tffRepeatedTop ?? 0}/{window.tffRepeatedBottom ?? 0}, BFF top/bottom {window.bffRepeatedTop ?? 0}/{window.bffRepeatedBottom ?? 0} · canonical FPS TFF/BFF {window.tffCanonicalOutputFrameRate?.toFixed(3) ?? 'unknown'}/{window.bffCanonicalOutputFrameRate?.toFixed(3) ?? 'unknown'} · timestamp anomalies {window.tffCanonicalTimestampAnomalies ?? 'unknown'}/{window.bffCanonicalTimestampAnomalies ?? 'unknown'} · {window.selectionBasis || 'initial analysis'}{window.reason ? ` · ${window.reason}` : ''}
                    </Typography>)}
                  </Stack>
                </Box> : null}
                {deepOperationPath === asset.mediaPath && deepOperation ? <Alert severity={deepOperation.status === 'error' ? 'error' : deepOperation.status === 'completed' ? 'success' : 'info'} data-testid="deep-cadence-operation">
                  {deepOperation.message || deepOperation.status}{deepOperation.status === 'running' ? ` · ${Math.round(deepOperation.progress)}%` : ''}
                </Alert> : null}
                {asset.cadenceReview.deepAnalysisAvailable ? <Button size="small" disabled={busy} onClick={() => void runDeepCadence(asset)}>Run Deep Cadence Analysis</Button> : null}
                {deepOperationPath === asset.mediaPath && deepOperation?.status === 'running' ? <Button size="small" disabled={false} onClick={() => void cancelDeepCadence()}>Cancel analysis</Button> : null}
              </Stack>
            </Alert> : null}
            {asset.blockers.map((blocker) => <Alert key={blocker} severity={asset.status === 'requires_review' ? 'warning' : 'error'} variant="outlined">{blocker}</Alert>)}
            {asset.warnings.map((warning) => <Alert key={warning} severity="warning" variant="outlined">{warning}</Alert>)}
            {planSummary(asset)}
            {asset.plan && recordText(asset.plan.profile.workerConfig as Record<string, unknown>, 'effectiveCadenceOperation') === 'inverse_telecine' ? <Alert severity="info">
              An explicit field order makes this plan resolvable, but does not prove visual fidelity. Review for combing, duplicate frames, and judder using Preview, Test Encode, or LAB before acceptance.
            </Alert> : null}
            <Divider />
            <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
              <Button size="small" disabled={busy || !asset.plan} onClick={() => void runPreview(asset)}>Preview exact plan</Button>
              <Button size="small" disabled={busy || !asset.plan} onClick={() => void runTestEncode(asset)}>Test Encode exact plan</Button>
              <Button size="small" disabled={!asset.plan} onClick={() => openLab(asset)}>Open LAB with candidate context</Button>
              {asset.status !== 'queued' ? <Button size="small" color="error" disabled={busy} onClick={() => void removeAsset(asset)}>Remove from candidate</Button> : null}
            </Stack>
          </Stack>
        </Box>)}
        {blockerCount ? <Alert severity="warning">{blockerCount} unresolved asset(s) need review or configuration. They remain in this candidate after approved assets are queued.</Alert> : null}
      </Stack>
    </DialogContent>
    <DialogActions>
      <Button color="error" disabled={busy || candidate.status !== 'active'} onClick={() => setConfirmingDiscard(true)}>Discard candidate</Button>
      {editing ? <>
        <Button disabled={busy} onClick={() => void discardEdits()}>Discard edits &amp; re-resolve</Button>
        <Button disabled={busy} onClick={closeDialog}>Close</Button>
        <Button variant="contained" disabled={busy || jobs.length === 0} onClick={() => void resolveEdits()}>Resolve Preflight again</Button>
      </> : <>
        <Button disabled={busy || !candidate.resolved || unresolvedCount === 0} onClick={() => void beginEdit()}>Edit</Button>
        <Button disabled={busy} onClick={closeDialog}>Close</Button>
        <Button variant="contained" disabled={busy || !candidate.resolved || selected.length === 0} onClick={() => void accept()}>Accept &amp; Queue {selected.length ? `(${selected.length})` : ''}</Button>
      </>}
    </DialogActions>
  </Dialog>;
}
