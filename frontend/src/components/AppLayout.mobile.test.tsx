// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it } from 'vitest';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { AppLayout } from './AppLayout';

afterEach(cleanup);

function LocationProbe() {
  const location = useLocation();
  return <div data-testid="location">{location.pathname}</div>;
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
});
