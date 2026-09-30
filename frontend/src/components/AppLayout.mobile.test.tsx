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

    await user.click(screen.getByRole('button', { name: 'Open navigation' }));
    const assetsLinks = screen.getAllByRole('link', { name: 'Assets' });
    expect(assetsLinks.length).toBeGreaterThan(0);
    await user.click(assetsLinks[0]);
    expect(screen.getByTestId('location').textContent).toBe('/assets');
    expect(screen.queryByRole('button', { name: 'Close navigation' })).toBeNull();
  });

  it('keeps Reports and Logs reachable and marks the current destination', async () => {
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
    expect(reportsLinks.length).toBeGreaterThan(0);
    expect(logsLinks.length).toBeGreaterThan(0);

    await user.click(reportsLinks[0]);
    expect(screen.getByTestId('location').textContent).toBe('/assets?location=library&evidence=reports');
    expect(screen.queryByRole('button', { name: 'Close navigation' })).toBeNull();
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
    expect(within(rail).queryByText('Assets')).toBeNull();
    await user.hover(rail);
    expect(within(rail).getByText('Assets')).toBeTruthy();
    await user.unhover(rail);
    expect(within(rail).queryByText('Assets')).toBeNull();
  });
});
