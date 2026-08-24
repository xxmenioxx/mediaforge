type TimingTrack = { sourceAudioIndex: number; outputAudioIndex: number; sourceOffsetMs: number | null; outputOffsetMs: number | null; introducedOffsetMs: number | null; introducedOffsetFrames: number | null; withinTolerance: boolean; status: string };
export type AVTimingViewModel = { status: string; withinTolerance: boolean; frameRateUsed: string; toleranceMs: number | null; toleranceFrames: number | null; driftStatus: string; tracks: TimingTrack[] };

export function avTimingViewModel(report: unknown): AVTimingViewModel | null {
  if (!report || typeof report !== 'object' || Array.isArray(report)) return null;
  const value = report as Record<string, unknown>;
  const raw = value.avTiming && typeof value.avTiming === 'object' && !Array.isArray(value.avTiming) ? value.avTiming as Record<string, unknown> : value;
  if (typeof raw.status !== 'string') return null;
  const tracks = Array.isArray(raw.tracks) ? raw.tracks.flatMap((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return [];
    const track = item as Record<string, unknown>;
    return [{ sourceAudioIndex: numberValue(track.sourceAudioIndex), outputAudioIndex: numberValue(track.outputAudioIndex), sourceOffsetMs: optionalNumber(track.sourceOffsetMs), outputOffsetMs: optionalNumber(track.outputOffsetMs), introducedOffsetMs: optionalNumber(track.introducedOffsetMs), introducedOffsetFrames: optionalNumber(track.introducedOffsetFrames), withinTolerance: track.withinTolerance === true, status: typeof track.status === 'string' ? track.status : 'unverified' }];
  }) : [];
  return { status: raw.status, withinTolerance: raw.withinTolerance === true, frameRateUsed: typeof raw.frameRateUsed === 'string' ? raw.frameRateUsed : '', toleranceMs: optionalNumber(raw.toleranceMs), toleranceFrames: optionalNumber(raw.toleranceFrames), driftStatus: typeof raw.driftStatus === 'string' ? raw.driftStatus : 'not_measured', tracks };
}

function numberValue(value: unknown) { return typeof value === 'number' && Number.isFinite(value) ? value : 0; }
function optionalNumber(value: unknown) { return typeof value === 'number' && Number.isFinite(value) ? value : null; }
