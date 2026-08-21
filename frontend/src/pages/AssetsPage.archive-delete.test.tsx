// @vitest-environment jsdom

import { useState } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { ArchiveDeleteConfirmationDialog } from './AssetsPage';

function DialogHarness({ onConfirm = () => undefined, onCancel = () => undefined, pending = false }: { onConfirm?: () => void; onCancel?: () => void; pending?: boolean }) {
  const [accepted, setAccepted] = useState(false);
  return (
    <ArchiveDeleteConfirmationDialog
      paths={['/archive/show/episode-01.mkv', '/archive/show/episode-02.mkv']}
      pathLabel="show"
      accepted={accepted}
      pending={pending}
      onAcceptedChange={setAccepted}
      onCancel={onCancel}
      onConfirm={onConfirm}
    />
  );
}

describe('ArchiveDeleteConfirmationDialog', () => {
  it('keeps Continue disabled until the irreversible deletion checkbox is accepted', async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    render(<DialogHarness onConfirm={onConfirm} />);

    expect(screen.getByText(/cannot be recovered after deletion/i)).toBeTruthy();
    expect(screen.getByText(/show · 2 assets/i)).toBeTruthy();
    const continueButton = screen.getByRole('button', { name: /continue/i });
    expect((continueButton as HTMLButtonElement).disabled).toBe(true);

    await user.click(screen.getByRole('checkbox', { name: /cannot be recovered/i }));
    expect((continueButton as HTMLButtonElement).disabled).toBe(false);
    await user.click(continueButton);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('disables confirmation controls while deletion is pending', () => {
    render(<DialogHarness pending />);
    expect((screen.getByRole('checkbox', { name: /cannot be recovered/i }) as HTMLInputElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: /deleting/i }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: /cancel/i }) as HTMLButtonElement).disabled).toBe(true);
  });

  it('cancels without invoking permanent deletion', async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    const onConfirm = vi.fn();
    render(<DialogHarness onCancel={onCancel} onConfirm={onConfirm} />);
    await user.click(screen.getByRole('button', { name: /cancel/i }));
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
