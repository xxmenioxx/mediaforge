export type ArchiveDeleteTimingPhase = 'delete-request' | 'assets-refresh' | 'feedback-render';

const timingSlots = new Map<ArchiveDeleteTimingPhase, number>();
const retainedSamplesPerPhase = 20;

export function measureArchiveDeletePhase(phase: ArchiveDeleteTimingPhase, startedAt: number): void {
  if (typeof performance === 'undefined' || typeof performance.measure !== 'function') return;
  const slot = timingSlots.get(phase) ?? 0;
  timingSlots.set(phase, (slot + 1) % retainedSamplesPerPhase);
  const name = `mvforge.archive-delete.${phase}.${slot}`;
  performance.clearMeasures(name);
  performance.measure(name, { start: startedAt, end: performance.now() });
}
