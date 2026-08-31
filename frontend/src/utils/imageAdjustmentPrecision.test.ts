import { describe, expect, it } from 'vitest';
import {
  brightnessFromStored,
  brightnessToStored,
  exposureFromStored,
  exposureToStored,
  formatFilterNumber,
  ratioFromStored,
  ratioToStored,
} from './imageAdjustmentPrecision';

describe('image adjustment precision', () => {
  it('round-trips exposure with 0.01 precision', () => {
    expect(formatFilterNumber(exposureFromStored(exposureToStored(0.12)))).toBe('0.12');
  });

  it('renders exact EQ coefficients', () => {
    expect([
      `brightness=${formatFilterNumber(brightnessFromStored(brightnessToStored(0)))}`,
      `contrast=${formatFilterNumber(ratioFromStored(ratioToStored(1)))}`,
      `saturation=${formatFilterNumber(ratioFromStored(ratioToStored(0.96)))}`,
      `gamma=${formatFilterNumber(ratioFromStored(ratioToStored(0.94)))}`,
    ].join(':')).toBe('brightness=0:contrast=1:saturation=0.96:gamma=0.94');
  });
});
