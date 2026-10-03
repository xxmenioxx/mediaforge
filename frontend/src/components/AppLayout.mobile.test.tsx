// @vitest-environment jsdom

import { cleanup, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { AppLayout } from './AppLayout';

afterEach(cleanup);

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}{location.search}</div>;
}

describe('mobile navigation compatibility', () => {
  it('keeps Assets reachable from the temporary navigation drawer and closes after navigation', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={['/']}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="*" element={<LocationProbe />} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    const mobileHeader = screen.getByRole('banner');
    expect(within(mobileHeader).getByRole('img', { name: 'MVForge logo' })).toBeTruthy();
    expect(within(mobileHeader).getByText('MVForge')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Open navigation' }));
    const assetsLinks = screen.getAllByRole('link', { name: 'Assets' });
    expect(assetsLinks.length).toBeGreaterThan(0);
    await user.click(assetsLinks[0]);
    expect(screen.getByTestId('location').textContent).toBe('/assets');
    expect(screen.queryByRole('button', { name: 'Close navigation' })).toBeNull();
  });

  it('keeps the separate Reports destination and Logs reachable', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={['/assets']}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="*" element={<LocationProbe />} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    await user.click(screen.getByRole('button', { name: 'Open navigation' }));
    const assetsLinks = screen.getAllByRole('link', { name: 'Assets' });
    const reportsLinks = screen.getAllByRole('link', { name: 'Reports' });
    const logsLinks = screen.getAllByRole('link', { name: 'Logs' });
    expect(assetsLinks[0].classList.contains('active')).toBe(true);
    expect(reportsLinks[0].classList.contains('active')).toBe(false);
    expect(reportsLinks.length).toBeGreaterThan(0);
    expect(logsLinks.length).toBeGreaterThan(0);

    await user.click(reportsLinks[0]);
    expect(screen.getByTestId('location').textContent).toBe('/reports');
    expect(screen.queryByRole('button', { name: 'Close navigation' })).toBeNull();

    await user.click(screen.getByRole('button', { name: 'Open navigation' }));
    const activeLinks = screen.getAllByRole('link').filter((link) => link.classList.contains('active'));
    expect(activeLinks.map((link) => link.textContent?.trim())).toEqual(['Reports']);
  });

  it.each([
    ['/assets', 'Assets'],
    ['/assets?location=raw', 'Assets'],
    ['/assets?location=library', 'Assets'],
    ['/assets?location=raw&evidence=reports', 'Assets'],
    ['/reports', 'Reports'],
    ['/reports/history', 'Reports'],
  ])('marks only %s primary destination active', async (path, activeDestination) => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="*" element={<LocationProbe />} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    await user.click(screen.getByRole('button', { name: 'Open navigation' }));
    const activeLinks = screen.getAllByRole('link').filter((link) => link.classList.contains('active'));
    expect(activeLinks).toHaveLength(1);
    expect(activeLinks[0].textContent?.trim()).toBe(activeDestination);
  });
});

describe('desktop navigation rail', () => {
  it('stays icon-only while collapsed and expands on hover', async () => {
    const user = userEvent.setup();
    render(
      <MemoryRouter initialEntries={['/assets']}>
        <Routes>
          <Route element={<AppLayout />}>
            <Route path="*" element={<LocationProbe />} />
          </Route>
        </Routes>
      </MemoryRouter>,
    );

    const rail = screen.getByRole('complementary');
    expect(within(rail).getByRole('img', { name: 'MVForge logo' })).toBeTruthy();
    expect(within(rail).queryByText('Assets')).toBeNull();
    await user.hover(rail);
    expect(within(rail).getByText('Assets')).toBeTruthy();
    expect(within(rail).getByText('MVForge')).toBeTruthy();
    await user.unhover(rail);
    expect(within(rail).queryByText('Assets')).toBeNull();
  });
});
