// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { LegacyAssetsRoute } from './App';

afterEach(cleanup);

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}{location.search}</div>;
}

function renderLegacy(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/assets/:legacyView" element={<LegacyAssetsRoute />} />
        <Route path="/assets" element={<LocationProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('legacy Assets compatibility routes', () => {
  it.each([
    ['/assets/unprocessed?query=show', '/assets?query=show&location=raw'],
    ['/assets/converted?validation=passed', '/assets?validation=passed&location=library&provenance=converted'],
    ['/assets/archive?location=raw', '/assets?location=library&original=archived'],
    ['/assets/reports?location=raw', '/assets?location=library&evidence=reports'],
  ])('maps %s without dropping safe query context', (path, expected) => {
    renderLegacy(path);
    expect(screen.getByTestId('location').textContent).toBe(expected);
  });

  it('renders an explicit compatibility state for an unmappable legacy route', () => {
    renderLegacy('/assets/removed-context');
    expect(screen.getByRole('alert').textContent).toMatch(/no longer an inventory context/i);
  });
});
