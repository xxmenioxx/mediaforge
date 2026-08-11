export function videoToolboxRatesFromTargetMbps(value: unknown) {
  const parsed = Number(value);
  const target = Number.isFinite(parsed) ? Math.max(0.01, Math.min(200, parsed)) : 2;
  return {
    target: roundMbps(target),
    maxrate: roundMbps(Math.min(250, target * 1.5)),
    buffer: roundMbps(Math.min(500, target * 2.5)),
  };
}

function roundMbps(value: number) {
  return Math.round(value * 100) / 100;
}
