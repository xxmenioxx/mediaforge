import type { FrameStructureMode } from './frameStructureModes';

export type AssetGopRecommendation = {
  fps?: number;
  sourceFrames: number;
  sourceSeconds?: number;
  targetFrames?: number;
  targetSeconds?: number;
  confidence: string;
  mode: FrameStructureMode;
  warning?: string;
};

export function parseReliableFrameRate(avg?: string, real?: string) {
  for (const value of [avg, real]) {
    if (!value) continue;
    const parts = value.split('/').map(Number);
    const fps = parts.length === 2 ? parts[0] / parts[1] : Number(value);
    if (Number.isFinite(fps) && fps > 0 && fps <= 240) return fps;
  }
  return undefined;
}

export function assetDerivedGopRecommendation(input: {
  fps?: number;
  sourceAverageGop?: number;
  confidence?: string;
  mode?: FrameStructureMode;
}): AssetGopRecommendation {
  const mode = input.mode ?? 'balanced';
  const fps = input.fps && Number.isFinite(input.fps) && input.fps > 0 ? input.fps : undefined;
  const sourceFrames = input.sourceAverageGop && input.sourceAverageGop > 0 ? input.sourceAverageGop : 0;
  const confidence = fps ? (input.confidence || (sourceFrames > 0 ? 'medium' : 'low')) : 'low';
  if (!fps) return { mode, sourceFrames, confidence, warning: 'A reliable asset frame rate is required before MVForge can calculate GOP frames.' };
  const sourceSeconds = sourceFrames > 0 ? sourceFrames / fps : undefined;
  const baseline = sourceSeconds === undefined ? ({ compatible: 2.5, balanced: 2.75, maximum_compression: 3, custom: 3 }[mode]) : clamp(sourceSeconds, 2, 4);
  let targetSeconds = mode === 'compatible' ? baseline : mode === 'maximum_compression' ? Math.min(baseline + 2, 5.5) : mode === 'balanced' ? Math.min(baseline + 0.75, 4) : baseline;
  if (confidence.toLowerCase() === 'low') targetSeconds = Math.min(targetSeconds, 3.5);
  targetSeconds = clamp(targetSeconds, 2, 8);
  return { mode, fps, sourceFrames, sourceSeconds, targetSeconds, targetFrames: Math.max(1, Math.round(fps * targetSeconds)), confidence };
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.max(minimum, Math.min(maximum, value));
}
