import { useEffect, useRef, useState, type ReactNode } from 'react';
import {
  Alert,
  Box,
  Button,
  Card,
  CardContent,
  Chip,
  IconButton,
  LinearProgress,
  Slider,
  Stack,
  Typography,
} from '@mui/material';
import FullscreenOutlinedIcon from '@mui/icons-material/FullscreenOutlined';
import MovieOutlinedIcon from '@mui/icons-material/MovieOutlined';
import PauseOutlinedIcon from '@mui/icons-material/PauseOutlined';
import PlayArrowOutlinedIcon from '@mui/icons-material/PlayArrowOutlined';
import PhotoCameraOutlinedIcon from '@mui/icons-material/PhotoCameraOutlined';
import type { Asset, ScanResult, TestEncode } from '../api/types';
import { api } from '../api/client';

type Props = {
  asset: Asset | null;
  snapshot?: ScanResult;
  tests: TestEncode[];
  selectedTest: TestEncode | null;
  retainedReferenceUrl?: string;
  loading: boolean;
  error?: string;
  qualityEvidence?: ReactNode;
  onSelectTest: (id: number) => void;
  onManageTests: () => void;
};

const HISTORY_LIMIT = 6;

function formatBytes(value?: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return 'Unavailable';
  if (value < 1024) return `${value} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let amount = value / 1024;
  let unit = units[0];
  for (let index = 1; amount >= 1024 && index < units.length; index += 1) {
    amount /= 1024;
    unit = units[index];
  }
  return `${amount.toFixed(2)} ${unit}`;
}

function formatBitrate(value?: number): string {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
    ? `${(value / 1000).toFixed(0)} kbps`
    : 'Unavailable';
}

function formatDuration(value?: number): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) return 'Unavailable';
  const total = Math.floor(value);
  const hours = Math.floor(total / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const seconds = total % 60;
  return [hours, minutes, seconds].map((part) => String(part).padStart(2, '0')).join(':');
}

function formatDate(value?: string): string {
  if (!value) return 'Time unavailable';
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? 'Time unavailable' : date.toLocaleString();
}

function recordValue(record: unknown, ...keys: string[]): string {
  if (!record || typeof record !== 'object') return 'Unavailable';
  const values = record as Record<string, unknown>;
  const value = keys.map((key) => values[key]).find((item) => typeof item === 'string' || typeof item === 'number');
  return value === undefined || value === '' || value === 0 ? 'Unavailable' : String(value);
}

function metricValue(metric: string, value?: { kind?: string; value?: number }): string {
  if (!value) return 'Unavailable';
  if (value.kind === 'positive_infinity') return '+∞';
  if (typeof value.value !== 'number' || !Number.isFinite(value.value)) return 'Unavailable';
  return metric === 'PSNR' ? `${value.value.toFixed(2)} dB` : value.value.toFixed(4);
}

function runHasOutput(test: TestEncode): boolean {
  return test.status === 'ready' && test.outputExists && Boolean(test.outputPath);
}

function MetricRow({ label, value }: { label: string; value: string }) {
  return (
    <Stack direction="row" spacing={1} justifyContent="space-between" sx={{ minWidth: 0 }}>
      <Typography variant="caption" color="text.secondary" noWrap>{label}</Typography>
      <Typography variant="caption" sx={{ textAlign: 'right', overflowWrap: 'anywhere' }}>{value}</Typography>
    </Stack>
  );
}

export function LabTestEncodeComparison({
  asset,
  snapshot,
  tests,
  selectedTest,
  retainedReferenceUrl,
  loading,
  error,
  qualityEvidence,
  onSelectTest,
  onManageTests,
}: Props) {
  const comparisonRef = useRef<HTMLDivElement | null>(null);
  const sourceRef = useRef<HTMLVideoElement | null>(null);
  const resultRef = useRef<HTMLVideoElement | null>(null);
  const syncLock = useRef(false);
  const [position, setPosition] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [sourceError, setSourceError] = useState(false);
  const [resultError, setResultError] = useState(false);
  const [snapshotMessage, setSnapshotMessage] = useState('');
  const startSeconds = selectedTest?.startSeconds ?? 0;
  const sourceOffsetSeconds = retainedReferenceUrl ? 0 : startSeconds;
  const durationSeconds = selectedTest?.durationSeconds ?? 0;
  const sourceSize = snapshot?.sizeBytes ?? asset?.sizeBytes;
  const outputSize = selectedTest?.outputSizeBytes;
  const videoStream = snapshot?.videoStreams?.find((stream) => stream.type === 'video');
  const sourceCodec = snapshot?.videoCodec || asset?.technical?.videoCodec || videoStream?.codec;
  const sourceResolution = snapshot?.width && snapshot.height
    ? `${snapshot.width} × ${snapshot.height}`
    : asset?.technical?.width && asset.technical.height
      ? `${asset.technical.width} × ${asset.technical.height}`
      : 'Unavailable';
  const evidenceRecord = selectedTest?.validationReport?.perceptualEvidence;
  const evidence = evidenceRecord && typeof evidenceRecord === 'object'
    ? evidenceRecord as {
      status?: string;
      windows?: Array<{ primary?: boolean; metrics?: Array<{ status?: string; value?: { kind?: string; value?: number }; identity?: { metric?: Record<string, unknown> } }> }>;
      aggregates?: Array<Record<string, unknown>>;
    }
    : undefined;
  const primaryWindow = evidence?.windows?.find((window) => window.primary) ?? evidence?.windows?.[0];
  const primaryMetrics = (primaryWindow?.metrics ?? []).slice(0, 3);
  const resultURL = selectedTest && runHasOutput(selectedTest) ? api.assetPreviewUrl(selectedTest.outputPath) : '';
  const sourceURL = retainedReferenceUrl || (asset?.path ? api.assetPreviewUrl(asset.path) : '');
  const visibleTests = tests.slice(0, HISTORY_LIMIT);

  useEffect(() => {
    setPosition(0);
    setPlaying(false);
    setSourceError(false);
    setResultError(false);
    setSnapshotMessage('');
  }, [asset?.path, selectedTest?.id]);

  function seekTo(nextPosition: number) {
    const bounded = Math.max(0, Math.min(durationSeconds, nextPosition));
    setPosition(bounded);
    syncLock.current = true;
    const source = sourceRef.current;
    const result = resultRef.current;
    if (source && source.readyState >= HTMLMediaElement.HAVE_METADATA) source.currentTime = sourceOffsetSeconds + bounded;
    if (result && result.readyState >= HTMLMediaElement.HAVE_METADATA) result.currentTime = bounded;
    requestAnimationFrame(() => { syncLock.current = false; });
  }

  function synchronizeFrom(side: 'source' | 'result') {
    if (syncLock.current || durationSeconds <= 0) return;
    const source = sourceRef.current;
    const result = resultRef.current;
    const current = side === 'source' ? source?.currentTime : result?.currentTime;
    if (typeof current !== 'number' || !Number.isFinite(current)) return;
    const nextPosition = side === 'source' ? current - sourceOffsetSeconds : current;
    if (nextPosition >= durationSeconds) {
      source?.pause();
      result?.pause();
      setPosition(durationSeconds);
      setPlaying(false);
      if (source && source.readyState >= HTMLMediaElement.HAVE_METADATA) source.currentTime = sourceOffsetSeconds + durationSeconds;
      if (result && result.readyState >= HTMLMediaElement.HAVE_METADATA) result.currentTime = durationSeconds;
      return;
    }
    const bounded = Math.max(0, Math.min(durationSeconds, nextPosition));
    setPosition(bounded);
    syncLock.current = true;
    if (side === 'source' && result && result.readyState >= HTMLMediaElement.HAVE_METADATA && Math.abs(result.currentTime - bounded) > 0.25) {
      result.currentTime = bounded;
    }
    if (side === 'result' && source && source.readyState >= HTMLMediaElement.HAVE_METADATA && Math.abs(source.currentTime - (sourceOffsetSeconds + bounded)) > 0.25) {
      source.currentTime = sourceOffsetSeconds + bounded;
    }
    requestAnimationFrame(() => { syncLock.current = false; });
  }

  async function togglePlayback() {
    const source = sourceRef.current;
    const result = resultRef.current;
    if (!source || !result || sourceError || resultError) return;
    if (playing) {
      source.pause();
      result.pause();
      setPlaying(false);
      return;
    }
    seekTo(position >= durationSeconds ? 0 : position);
    const outcomes = await Promise.allSettled([source.play(), result.play()]);
    if (outcomes.some((outcome) => outcome.status === 'rejected')) {
      source.pause();
      result.pause();
      setPlaying(false);
      setSourceError(source.error !== null);
      setResultError(result.error !== null);
      return;
    }
    setPlaying(true);
  }

  function saveResultFrame() {
    const video = resultRef.current;
    if (!video || video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA || !video.videoWidth || !video.videoHeight || !selectedTest) {
      setSnapshotMessage('The result frame is not ready to save.');
      return;
    }
    try {
      const canvas = document.createElement('canvas');
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      const context = canvas.getContext('2d');
      if (!context) throw new Error('Frame capture is unavailable.');
      context.drawImage(video, 0, 0, canvas.width, canvas.height);
      canvas.toBlob((blob) => {
        if (!blob) {
          setSnapshotMessage('The result frame could not be saved.');
          return;
        }
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = `test-encode-${selectedTest.id}-frame.png`;
        link.click();
        URL.revokeObjectURL(url);
        setSnapshotMessage('Result frame saved.');
      }, 'image/png');
    } catch {
      setSnapshotMessage('The result frame could not be saved from this media source.');
    }
  }

  async function enterFullscreen() {
    if (!comparisonRef.current?.requestFullscreen) return;
    try {
      await comparisonRef.current.requestFullscreen();
    } catch {
      setSnapshotMessage('Fullscreen is unavailable in this browser.');
    }
  }

  const comparisonHasOutput = Boolean(resultURL && !resultError && !sourceError);

  return (
    <Stack spacing={1.25} sx={{ minWidth: 0 }}>
      <Box ref={comparisonRef} sx={{ minWidth: 0 }}>
        <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" spacing={1} alignItems={{ sm: 'center' }} sx={{ mb: 1 }}>
          <Box sx={{ minWidth: 0 }}>
            <Typography variant="h2" sx={{ fontSize: { xs: '1rem', sm: '1.1rem' }, fontWeight: 700 }}>Original vs Test Encode</Typography>
            <Typography variant="caption" color="text.secondary">
              {selectedTest ? `Comparing Test Encode T${selectedTest.id} · ${formatDuration(startSeconds)} + ${formatDuration(durationSeconds)}` : 'Select a retained result to compare its sample window.'}
            </Typography>
          </Box>
          <Button size="small" variant="outlined" onClick={onManageTests}>Manage Test Encodes</Button>
        </Stack>

        <Box sx={{ display: 'grid', gridTemplateColumns: { xs: 'minmax(0, 1fr)', md: 'repeat(2, minmax(0, 1fr))' }, gap: 1.25, minWidth: 0 }}>
          <Card variant="outlined" sx={{ minWidth: 0, overflow: 'hidden', bgcolor: 'background.paper' }}>
            <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
              <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={1} sx={{ mb: 0.75 }}>
                <Stack direction="row" spacing={0.75} alignItems="center" minWidth={0}>
                  <Box aria-hidden="true" sx={{ width: 3, height: 20, bgcolor: 'primary.main', borderRadius: 1 }} />
                  <Typography variant="subtitle2" noWrap>Original (Source)</Typography>
                  <Chip size="small" label={formatBytes(sourceSize)} />
                </Stack>
              </Stack>
              <Box sx={{ position: 'relative', width: '100%', aspectRatio: '16 / 9', bgcolor: '#05080d', borderRadius: 1, overflow: 'hidden' }}>
                {sourceURL ? (
                  <Box
                    component="video"
                    ref={sourceRef}
                    src={sourceURL}
                    preload="metadata"
                    playsInline
                    aria-label="Original source preview"
                    onLoadedMetadata={() => seekTo(position)}
                    onTimeUpdate={() => synchronizeFrom('source')}
                    onEnded={() => synchronizeFrom('source')}
                    onError={() => setSourceError(true)}
                    sx={{ width: '100%', height: '100%', objectFit: 'contain' }}
                  />
                ) : <EmptyVideoState>Choose an asset to show the Original.</EmptyVideoState>}
                {sourceError ? <EmptyVideoState>Original preview is unavailable in this browser.</EmptyVideoState> : null}
              </Box>
            </CardContent>
          </Card>

          <Card variant="outlined" sx={{ minWidth: 0, overflow: 'hidden', bgcolor: 'background.paper' }}>
            <CardContent sx={{ p: 1, '&:last-child': { pb: 1 } }}>
              <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={1} sx={{ mb: 0.75 }}>
                <Stack direction="row" spacing={0.75} alignItems="center" minWidth={0}>
                  <Box aria-hidden="true" sx={{ width: 3, height: 20, bgcolor: 'success.main', borderRadius: 1 }} />
                  <Typography variant="subtitle2" noWrap>Test Encode (Result)</Typography>
                  {selectedTest ? <Chip size="small" color="success" label={formatBytes(outputSize)} /> : null}
                </Stack>
              </Stack>
              <Box sx={{ position: 'relative', width: '100%', aspectRatio: '16 / 9', bgcolor: '#05080d', borderRadius: 1, overflow: 'hidden' }}>
                {resultURL ? (
                  <Box
                    component="video"
                    ref={resultRef}
                    src={resultURL}
                    preload="metadata"
                    playsInline
                    aria-label="Selected Test Encode result preview"
                    onLoadedMetadata={() => seekTo(position)}
                    onTimeUpdate={() => synchronizeFrom('result')}
                    onEnded={() => synchronizeFrom('result')}
                    onError={() => setResultError(true)}
                    sx={{ width: '100%', height: '100%', objectFit: 'contain' }}
                  />
                ) : <EmptyVideoState>{selectedTest ? 'The selected Test Encode output is unavailable.' : 'Run a Test Encode and select a ready result.'}</EmptyVideoState>}
                {resultError ? <EmptyVideoState>Result preview is unavailable in this browser.</EmptyVideoState> : null}
              </Box>
            </CardContent>
          </Card>
        </Box>

        <Stack direction="row" alignItems="center" spacing={0.5} sx={{ mt: 0.75 }}>
          <IconButton size="small" aria-label={playing ? 'Pause comparison' : 'Play comparison'} onClick={() => void togglePlayback()} disabled={!comparisonHasOutput || durationSeconds <= 0}>
            {playing ? <PauseOutlinedIcon /> : <PlayArrowOutlinedIcon />}
          </IconButton>
          <Typography variant="caption" sx={{ whiteSpace: 'nowrap', fontVariantNumeric: 'tabular-nums' }}>
            {formatDuration(position)} / {formatDuration(durationSeconds)}
          </Typography>
          <Slider
            size="small"
            value={Math.min(position, durationSeconds || 0)}
            min={0}
            max={Math.max(1, durationSeconds)}
            step={0.1}
            aria-label="Synchronized comparison position"
            onChange={(_, value) => seekTo(Array.isArray(value) ? value[0] : value)}
            disabled={!comparisonHasOutput || durationSeconds <= 0}
            sx={{ mx: 1, minWidth: 30 }}
          />
          <IconButton size="small" aria-label="Save Test Encode frame" onClick={saveResultFrame} disabled={!resultURL || resultError}>
            <PhotoCameraOutlinedIcon />
          </IconButton>
          <IconButton size="small" aria-label="Fullscreen comparison" onClick={() => void enterFullscreen()} disabled={!asset}>
            <FullscreenOutlinedIcon />
          </IconButton>
        </Stack>
        {snapshotMessage ? <Typography role="status" variant="caption" color="text.secondary">{snapshotMessage}</Typography> : null}
        {sourceError || resultError ? <Alert severity="info">One of the retained media files cannot be played by this browser. Its run and evidence remain available.</Alert> : null}
      </Box>

      {loading ? <LinearProgress aria-label="Loading Test Encode history" /> : null}
      {error ? <Alert severity="warning">{error}</Alert> : null}

      <Card component="section" variant="outlined" aria-label="Comparison metrics" sx={{ minWidth: 0, bgcolor: 'background.paper' }}>
        <CardContent sx={{ p: 1.25, '&:last-child': { pb: 1.25 } }}>
          <Typography variant="subtitle2" sx={{ mb: 1 }}>Metrics</Typography>
          <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' }, gap: { xs: 1.25, md: 2 } }}>
            <Stack spacing={0.5}>
              <Typography variant="caption" color="text.secondary">Original</Typography>
              <MetricRow label="Codec" value={sourceCodec || 'Unavailable'} />
              <MetricRow label="Average bitrate" value={formatBitrate(snapshot?.bitrate ?? asset?.technical?.bitrate)} />
              <MetricRow label="Source file size" value={formatBytes(sourceSize)} />
              <MetricRow label="Resolution" value={sourceResolution} />
              <MetricRow label="FPS" value={recordValue(videoStream, 'frameRate', 'avgFrameRate', 'rFrameRate')} />
            </Stack>
            <Stack spacing={0.5}>
              <Typography variant="caption" color="text.secondary">Test Encode result</Typography>
              <MetricRow label="Effective encoder" value={selectedTest?.effectiveEncoder || 'Unavailable'} />
              <MetricRow label="Average bitrate" value="Unavailable in retained summary" />
              <MetricRow label="Test sample size" value={formatBytes(outputSize)} />
              <MetricRow label="Size difference" value="Unavailable · source sample size is not retained" />
              <MetricRow label="Resolution / FPS" value="Unavailable in retained summary" />
            </Stack>
          </Box>
          {selectedTest?.profileQualification ? (
            <Stack direction="row" spacing={0.75} alignItems="center" flexWrap="wrap" useFlexGap sx={{ mt: 1.25 }}>
              <Chip
                size="small"
                color={selectedTest.profileQualification.stale ? 'warning' : selectedTest.profileQualification.outcome === 'FAIL' ? 'error' : selectedTest.profileQualification.outcome === 'PASS' ? 'success' : 'default'}
                label={selectedTest.profileQualification.stale ? 'Qualification stale' : `Qualification · ${selectedTest.profileQualification.outcome}`}
              />
              <Typography variant="caption" color="text.secondary">
                Policy {selectedTest.profileQualification.qualityPolicyVersion || 'unavailable'}
              </Typography>
            </Stack>
          ) : selectedTest?.status === 'ready' ? (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1.25 }}>No current profile qualification is stored for this run.</Typography>
          ) : null}
          {primaryMetrics.length > 0 ? (
            <Stack direction="row" spacing={0.75} flexWrap="wrap" useFlexGap sx={{ mt: 1 }} aria-label="Canonical quality metrics">
              {primaryMetrics.map((metric, index) => {
                const metricName = String(metric.identity?.metric?.logicalMetric ?? 'Quality metric');
                return <Chip key={`${metricName}-${index}`} size="small" variant="outlined" label={`${metricName} · ${metric.status}${metric.status === 'available' ? ` · ${metricValue(metricName, metric.value)}` : ''}`} />;
              })}
            </Stack>
          ) : selectedTest ? (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>Canonical PSNR, SSIM, and VMAF evidence is unavailable for this run.</Typography>
          ) : null}
          {qualityEvidence ? <Box sx={{ mt: 1 }}>{qualityEvidence}</Box> : null}
        </CardContent>
      </Card>

      <Card component="section" variant="outlined" aria-label="Previous Test Encodes" sx={{ minWidth: 0, bgcolor: 'background.paper' }}>
        <CardContent sx={{ p: 1.25, '&:last-child': { pb: 1.25 } }}>
          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" alignItems={{ sm: 'center' }} spacing={0.75} sx={{ mb: 1 }}>
            <Stack direction="row" spacing={0.75} alignItems="center">
              <Typography variant="subtitle2">Original reference and recent runs</Typography>
              <Chip size="small" label={tests.length} aria-label={`${tests.length} retained Test Encodes`} />
            </Stack>
            <Button size="small" onClick={onManageTests}>Open Test Encode history</Button>
          </Stack>
          <Stack direction="row" spacing={0.75} sx={{ minWidth: 0, overflowX: 'auto', pb: 0.5 }}>
            <HistoryCard original label="Original reference" size={formatBytes(sourceSize)} />
            {visibleTests.map((test) => {
              const canSelect = runHasOutput(test);
              const selected = selectedTest?.id === test.id;
              return (
                <Button
                  key={test.id}
                  variant="outlined"
                  aria-pressed={selected}
                  aria-label={`Select Test Encode T${test.id}, ${test.status}, ${formatBytes(test.outputSizeBytes)}`}
                  disabled={!canSelect}
                  onClick={() => onSelectTest(test.id)}
                  sx={{
                    flex: '0 0 190px',
                    minWidth: 0,
                    minHeight: 68,
                    justifyContent: 'flex-start',
                    textAlign: 'left',
                    borderColor: selected ? 'primary.main' : 'divider',
                    bgcolor: selected ? 'action.selected' : 'transparent',
                  }}
                >
                  <Stack direction="row" spacing={0.75} alignItems="center" minWidth={0}>
                    <MovieOutlinedIcon fontSize="small" color={selected ? 'primary' : 'disabled'} />
                    <Stack spacing={0.15} sx={{ minWidth: 0 }}>
                      <Typography variant="caption" fontWeight={700} noWrap>Test Encode T{test.id}{test.keep ? ' · Keep' : ''}</Typography>
                      <Typography variant="caption" color="text.secondary" noWrap>{test.status} · {formatBytes(test.outputSizeBytes)}</Typography>
                      <Typography variant="caption" color="text.secondary" noWrap>{formatDate(test.completedAt || test.createdAt)}</Typography>
                    </Stack>
                  </Stack>
                </Button>
              );
            })}
          </Stack>
          {!loading && tests.length === 0 ? <Typography variant="caption" color="text.secondary">No retained Test Encodes for this asset yet.</Typography> : null}
          {tests.length > HISTORY_LIMIT ? <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.75 }}>Showing the {HISTORY_LIMIT} most recent runs. Open Test Encode history for the bounded inventory.</Typography> : null}
        </CardContent>
      </Card>
    </Stack>
  );
}

function EmptyVideoState({ children }: { children: ReactNode }) {
  return (
    <Stack
      alignItems="center"
      justifyContent="center"
      spacing={0.75}
      sx={{ position: 'absolute', inset: 0, px: 2, color: 'text.secondary', textAlign: 'center', bgcolor: '#05080d' }}
      role="status"
    >
      <MovieOutlinedIcon fontSize="large" />
      <Typography variant="caption">{children}</Typography>
    </Stack>
  );
}

function HistoryCard({ original, label, size }: { original?: boolean; label: string; size: string }) {
  return (
    <Box
      component="div"
      aria-label={label}
      sx={{
        display: 'flex',
        flex: '0 0 190px',
        minWidth: 0,
        minHeight: 68,
        alignItems: 'center',
        gap: 0.75,
        px: 1,
        border: 1,
        borderColor: 'divider',
        borderRadius: 1,
        bgcolor: original ? 'action.hover' : 'transparent',
      }}
    >
      <MovieOutlinedIcon fontSize="small" color={original ? 'primary' : 'disabled'} />
      <Stack spacing={0.15} sx={{ minWidth: 0 }}>
        <Typography variant="caption" fontWeight={700} noWrap>{label}</Typography>
        <Typography variant="caption" color="text.secondary" noWrap>{size}</Typography>
        <Typography variant="caption" color="text.secondary" noWrap>{original ? 'Selected source' : 'Retained source reference'}</Typography>
      </Stack>
    </Box>
  );
}
