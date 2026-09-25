import { describe, expect, it } from 'vitest';
import { liveRuntimeStateLabel, runtimeRefreshSuccessMessage } from './runtimeStatePresentation';

describe('runtime state presentation', () => {
  it('labels live telemetry with detection time rather than snapshot identity', () => {
    expect(liveRuntimeStateLabel('2026-09-24T12:00:00Z', () => 'Sep 24, 12:00')).toBe('Live runtime state · detected Sep 24, 12:00');
    expect(liveRuntimeStateLabel('2026-09-24T12:00:00Z', () => 'Sep 24, 12:00')).not.toContain('Host snapshot');
  });

  it('does not claim that every refresh creates immutable evidence', () => {
    expect(runtimeRefreshSuccessMessage).toBe('Runtime detection refreshed successfully.');
    expect(runtimeRefreshSuccessMessage).not.toContain('snapshot');
  });
});
