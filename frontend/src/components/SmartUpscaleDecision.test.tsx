// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { SmartUpscaleDecision } from './SmartUpscaleDecision';

describe('SmartUpscaleDecision', () => {
  it('renders backend Auto output, crop geometry, confidence, reasons, and warnings', () => {
    render(<SmartUpscaleDecision requestedMode="auto" requestedSharpen="light" storageWidth={720} storageHeight={480} decision={{
      requestedMode: 'auto', resolvedMode: '720p', sourceWidth: 704, sourceHeight: 448, sourceSAR: '40:33', sourceDAR: '4:3', targetWidth: 960, targetHeight: 720, targetSAR: '1:1', upscaleApplied: true, sharpenMode: 'light', confidence: 'high', reasons: ['reliable_sd_progressive_output'], warnings: ['geometry_warning_from_backend'],
    }} />);
    expect(screen.getByText('720×480')).toBeTruthy();
    expect(screen.getByText('704×448')).toBeTruthy();
    expect(screen.getByText('960×720')).toBeTruthy();
    expect(screen.getByText('1:1')).toBeTruthy();
    expect(screen.getByText(/source is SD and the effective output is progressive/i)).toBeTruthy();
    expect(screen.getByText(/Geometry warning from backend/)).toBeTruthy();
  });

  it('renders Keep Source and avoids a redundant effective geometry row', () => {
    render(<SmartUpscaleDecision storageWidth={1280} storageHeight={720} decision={{ requestedMode: 'auto', resolvedMode: 'keep_source', sourceWidth: 1280, sourceHeight: 720, targetWidth: 1280, targetHeight: 720, upscaleApplied: false, sharpenMode: 'off', confidence: 'medium', reasons: [], warnings: [] }} />);
    expect(screen.getAllByText('Keep Source')).toHaveLength(2);
    expect(screen.queryByText('Effective geometry after crop')).toBeNull();
  });
});
afterEach(cleanup);
