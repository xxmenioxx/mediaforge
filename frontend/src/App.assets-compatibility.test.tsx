// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { AssetsRoute, LegacyAnalysisRoute, LegacyAssetsRoute } from './App';

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
        <Route path="/analysis" element={<LegacyAnalysisRoute />} />
        <Route path="/scanner" element={<LegacyAnalysisRoute />} />
        <Route path="/assets" element={<LocationProbe />} />
        <Route path="/reports/*" element={<LocationProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

function renderAssetsRoute(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/assets" element={<AssetsRoute />} />
        <Route path="/reports/*" element={<LocationProbe />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe('legacy Assets compatibility routes', () => {
  it.each([
    ['/assets/unprocessed?query=show', '/assets?query=show&location=raw'],
    ['/assets/converted?validation=passed', '/assets?validation=passed&location=library&provenance=converted'],
    ['/assets/archive?location=raw', '/assets?location=library'],
    ['/assets/reports?location=raw', '/reports'],
  ])('maps %s without dropping safe query context', (path, expected) => {
    renderLegacy(path);
    expect(screen.getByTestId('location').textContent).toBe(expected);
  });

  it.each([
    '/assets?evidence=reports',
    '/assets?location=library&tab=reports',
  ])('moves legacy report query state to the Reports route: %s', (path) => {
    renderAssetsRoute(path);
    expect(screen.getByTestId('location').textContent).toBe('/reports');
  });

  it('renders an explicit compatibility state for an unmappable legacy route', () => {
    renderLegacy('/assets/removed-context');
    expect(screen.getByRole('alert').textContent).toMatch(/no longer an inventory context/i);
  });
});

describe('legacy Analysis and Scanner compatibility routes', () => {
  it.each(['/analysis?asset=%2Fmedia%2Fraw%2Fmovie.mkv&autorun=1', '/scanner?path=%2Fmedia%2Fraw%2Fmovie.mkv&autorun=1'])('preserves asset identity and never autoruns: %s', (path) => {
    renderLegacy(path);
    const location = new URL(screen.getByTestId('location').textContent ?? '', 'http://localhost');
    expect(location.pathname).toBe('/assets');
    expect(location.searchParams.get('location')).toBe('raw');
    expect(location.searchParams.get('asset')).toBe('/media/raw/movie.mkv');
    expect(location.searchParams.has('autorun')).toBe(false);
  });
});
