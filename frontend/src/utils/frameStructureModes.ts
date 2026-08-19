export type FrameStructureMode = 'auto' | 'off' | 'compatible' | 'balanced' | 'maximum_compression' | 'custom';

export const frameStructureManagedKeys = new Set([
  'frameStructureGopMode', 'frameStructureGopFrames', 'frameStructureBFrameMode',
  'frameStructureMaxBFrames', 'qsvAdaptiveI', 'qsvAdaptiveB', 'qsvPStrategy',
]);

export function frameStructureModePatch(
  selected: FrameStructureMode,
  recommendedGop?: number,
  recommendedBFrames = 3,
) {
  const baseGop =
    recommendedGop && recommendedGop > 0
      ? Math.max(1, Math.min(1000, Math.round(recommendedGop)))
      : undefined;

  const baseBFrames = Math.max(
    1,
    Math.min(4, Math.round(recommendedBFrames || 3)),
  );

  if (selected === 'custom') {
    return {
      frameStructureMode: 'custom',

      frameStructureGopMode: 'custom',
      ...(baseGop
        ? { frameStructureGopFrames: baseGop }
        : {}),

      frameStructureBFrameMode: 'custom',
      frameStructureMaxBFrames: baseBFrames,
    };
  }

  if (selected === 'off') {
    return {
      frameStructureMode: selected,
      frameStructureGopMode: 'auto',
      frameStructureBFrameMode: 'auto',
      qsvAdaptiveI: false,
      qsvAdaptiveB: false,
      qsvPStrategy: 0,
    };
  }

  if (selected === 'compatible') {
    return {
      frameStructureMode: selected,
      frameStructureGopMode: baseGop ? 'recommended' : 'auto',
      ...(baseGop
        ? { frameStructureGopFrames: baseGop }
        : {}),
      frameStructureBFrameMode: 'off',
      frameStructureMaxBFrames: 0,
      qsvAdaptiveI: true,
      qsvAdaptiveB: false,
      qsvPStrategy: 1,
    };
  }

  // Auto intentionally uses the same effective policy as Balanced.
  const bFrames =
    selected === 'maximum_compression'
      ? Math.max(3, baseBFrames)
      : Math.min(3, baseBFrames);

  return {
    frameStructureMode: selected,

    frameStructureGopMode:
      baseGop ? 'recommended' : 'auto',

    ...(baseGop
      ? { frameStructureGopFrames: baseGop }
      : {}),

    frameStructureBFrameMode: 'recommended',
    frameStructureMaxBFrames: bFrames,

    qsvAdaptiveI: true,
    qsvAdaptiveB: true,
    qsvPStrategy: 0,
  };
}