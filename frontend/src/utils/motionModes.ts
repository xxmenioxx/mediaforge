export function semanticMotionModes(worker: Record<string, unknown> | undefined) {
  const legacy = String(worker?.deinterlaceMode ?? 'auto').toLowerCase();
  const fieldStructureMode = String(worker?.fieldStructureMode ?? (legacy === 'force' ? 'deinterlace' : legacy === 'auto' ? 'auto' : 'preserve'));
  const cadenceMode = String(worker?.cadenceMode ?? (legacy === 'ivtc_tff' || legacy === 'ivtc_bff' ? 'inverse_telecine' : legacy === 'auto' ? 'auto' : 'preserve'));
  const cadenceFieldOrder = String(worker?.cadenceFieldOrder ?? (legacy === 'ivtc_bff' ? 'bff' : legacy === 'ivtc_tff' ? 'tff' : 'auto'));
  const deinterlaceFieldOrder = String(worker?.deinterlaceFieldOrder ?? 'auto');
  return { fieldStructureMode, cadenceMode, cadenceFieldOrder, deinterlaceFieldOrder };
}
