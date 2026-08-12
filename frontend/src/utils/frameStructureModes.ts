export type FrameStructureMode = 'compatible' | 'balanced' | 'maximum_compression' | 'custom';

export const frameStructureManagedKeys = new Set([
  'frameStructureGopMode', 'frameStructureGopFrames', 'frameStructureBFrameMode',
  'frameStructureMaxBFrames', 'qsvAdaptiveI', 'qsvAdaptiveB', 'qsvPStrategy',
]);

export function frameStructureModePatch(selected: FrameStructureMode, recommendedGop?: number, recommendedBFrames = 3) {
  if (selected === 'custom') return { frameStructureMode: selected };
  const baseGop = recommendedGop && recommendedGop > 0 ? Math.max(1, Math.min(1000, Math.round(recommendedGop))) : undefined;
  if (selected === 'compatible') return {
    frameStructureMode: selected, frameStructureGopMode: baseGop ? 'recommended' : 'auto', ...(baseGop ? { frameStructureGopFrames: baseGop } : {}),
    frameStructureBFrameMode: 'off', frameStructureMaxBFrames: 0,
    qsvAdaptiveI: true, qsvAdaptiveB: false, qsvPStrategy: 1,
  };
  const bFrames = selected === 'maximum_compression'
    ? Math.max(3, Math.min(4, Math.round(recommendedBFrames || 3)))
    : Math.max(1, Math.min(3, Math.round(recommendedBFrames || 3)));
  return {
    frameStructureMode: selected,
    frameStructureGopMode: baseGop ? 'recommended' : 'auto',
    ...(baseGop ? { frameStructureGopFrames: baseGop } : {}),
    frameStructureBFrameMode: 'custom', frameStructureMaxBFrames: bFrames,
    qsvAdaptiveI: true, qsvAdaptiveB: true, qsvPStrategy: 0,
  };
}
