// @vitest-environment jsdom

import { cleanup, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { EmbeddedSubtitleActions, ExternalSubtitleList } from './AssetsPage';
import type { ExternalSubtitle } from '../api/types';

afterEach(cleanup);

const stream = {
  index: 4,
  codec: 'ass',
  language: 'eng',
  title: 'English forced',
} as never;

const sidecar: ExternalSubtitle = {
  path: '/media/episode.eng.forced.ass',
  fileName: 'episode.eng.forced.ass',
  format: 'ass',
  language: 'eng',
  default: false,
  forced: true,
  sizeBytes: 100,
  modifiedAt: '',
};

const embeddedStreams = [
  { index: 3, codec: 'ass', language: 'eng', title: 'English full' },
  { index: 4, codec: 'ass', language: 'eng', title: 'English forced' },
  { index: 5, codec: 'ass', language: 'jpn', title: 'Japanese' },
] as never;

const externalSidecars: ExternalSubtitle[] = [
  sidecar,
  {
    path: '/media/episode.eng.ass',
    fileName: 'episode.eng.ass',
    format: 'ass',
    language: 'eng',
    default: true,
    forced: false,
    sizeBytes: 100,
    modifiedAt: '',
  },
  {
    path: '/media/episode.jpn.ass',
    fileName: 'episode.jpn.ass',
    format: 'ass',
    language: 'jpn',
    default: false,
    forced: false,
    sizeBytes: 100,
    modifiedAt: '',
  },
] as never;

describe('Asset Info translation source controls', () => {
  it.each(['srt', 'ass', 'ssa'] as const)('routes Review with AI for the exact %s sidecar', async (format) => {
    const user = userEvent.setup();
    const onProofread = vi.fn();
    const exact = {
      ...sidecar,
      path: `/media/episode.eng.${format}`,
      fileName: `episode.eng.${format}`,
      format,
    } as ExternalSubtitle;
    render(
      <ExternalSubtitleList
        values={[exact]}
        loading={false}
        deleting={false}
        renaming={false}
        onRename={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
        onProofread={onProofread}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Review with AI' }));
    expect(onProofread).toHaveBeenCalledTimes(1);
    expect(onProofread).toHaveBeenCalledWith(exact);
  });

  it('passes only the exact embedded stream index to Translate...', async () => {
    const user = userEvent.setup();
    const onTranslate = vi.fn();
    render(
      <EmbeddedSubtitleActions
        streams={[stream]}
        generations={{}}
        onGenerate={vi.fn()}
        onCancel={vi.fn()}
        onTranslate={onTranslate}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Translate subtitle stream 4' }));
    expect(onTranslate).toHaveBeenCalledWith(4);
  });

  it('selects the exact embedded stream among same-language and Japanese tracks', async () => {
    const user = userEvent.setup();
    const onTranslate = vi.fn();
    render(
      <EmbeddedSubtitleActions
        streams={embeddedStreams}
        generations={{}}
        onGenerate={vi.fn()}
        onCancel={vi.fn()}
        onTranslate={onTranslate}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Translate subtitle stream 4' }));
    expect(onTranslate).toHaveBeenCalledTimes(1);
    expect(onTranslate).toHaveBeenCalledWith(4);
    expect(onTranslate).not.toHaveBeenCalledWith(3);
    expect(onTranslate).not.toHaveBeenCalledWith(5);
  });

  it('passes the exact clicked external sidecar to Translate...', async () => {
    const user = userEvent.setup();
    const onTranslate = vi.fn();
    render(
      <ExternalSubtitleList
        values={[sidecar]}
        loading={false}
        deleting={false}
        renaming={false}
        onRename={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
        onTranslate={onTranslate}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Translate episode.eng.forced.ass' }));
    expect(onTranslate).toHaveBeenCalledWith(sidecar);
  });

  it('selects the exact external sidecar among same-language siblings', async () => {
    const user = userEvent.setup();
    const onTranslate = vi.fn();
    render(
      <ExternalSubtitleList
        values={externalSidecars}
        loading={false}
        deleting={false}
        renaming={false}
        onRename={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
        onTranslate={onTranslate}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Translate episode.eng.forced.ass' }));
    expect(onTranslate).toHaveBeenCalledTimes(1);
    expect(onTranslate).toHaveBeenCalledWith(expect.objectContaining({
      path: '/media/episode.eng.forced.ass',
    }));
    expect(onTranslate).not.toHaveBeenCalledWith(expect.objectContaining({ path: '/media/episode.eng.ass' }));
    expect(onTranslate).not.toHaveBeenCalledWith(expect.objectContaining({ path: '/media/episode.jpn.ass' }));
  });

  it('shows exact identity and explicit publication/override controls before restyle', async () => {
    const user = userEvent.setup();
    const onRestyle = vi.fn();
    const exact = {
      path: '/media/episode.eng.forced.ass',
      fileName: 'episode.eng.forced.ass',
      format: 'ass',
      language: 'eng',
      default: false,
      forced: true,
      sizeBytes: 100,
      modifiedAt: '',
      sourceRevision: 'sidecar-revision-1',
      eligibility: {
        restyle: { eligible: true },
        srtToAss: { eligible: false, reason: 'not an SRT source' },
        regenerate: { eligible: false, reason: 'embedded provenance required' },
        replaceEligible: true,
        nativeOverrideEligible: true,
        sourceIdentity: 'external:/media/episode.eng.forced.ass',
        sourceRevision: 'sidecar-revision-1',
        representation: 'ass',
        role: 'unknown',
        roleProvenance: 'external-sidecar',
        policyVersion: 'policy-v1',
      },
    } as ExternalSubtitle;
    render(
      <ExternalSubtitleList
        values={[exact]}
        loading={false}
        deleting={false}
        renaming={false}
        onRename={vi.fn()}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
        onRestyle={onRestyle}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Restyle' }));
    expect(screen.getByText('Identity: external:/media/episode.eng.forced.ass')).toBeTruthy();
    expect(screen.getByText('Revision: sidecar-revision-1')).toBeTruthy();
    expect(screen.getByText(/Policy: policy-v1/)).toBeTruthy();
    await user.click(screen.getByRole('switch', { name: 'Replace the selected sidecar (explicit opt-in)' }));
    await user.click(screen.getByRole('switch', { name: 'Override eligible native ASS styles for this operation' }));
    await user.click(screen.getByRole('button', { name: 'Start restyle' }));
    expect(onRestyle).toHaveBeenCalledWith(exact, 'restyle', { replace: true, overrideExistingASSStyles: true });
  });

  it('exposes embedded regeneration through the existing subtitle action lifecycle', async () => {
    const user = userEvent.setup();
    const onRegenerate = vi.fn();
    render(<EmbeddedSubtitleActions streams={[stream]} generations={{}} onGenerate={vi.fn()} onCancel={vi.fn()} onRegenerate={onRegenerate} />);
    await user.click(screen.getByRole('button', { name: 'Regenerate styled ASS' }));
    expect(onRegenerate).toHaveBeenCalledWith(4);
  });

  it('renders only backend-authorized external modes and displays exact identity', async () => {
    const denied = {
      ...sidecar,
      eligibility: {
        restyle: { eligible: false, reason: 'source revision is stale' },
        srtToAss: { eligible: false, reason: 'not an SRT source' },
        regenerate: { eligible: false, reason: 'embedded provenance required' },
        replaceEligible: false,
        nativeOverrideEligible: false,
        sourceIdentity: 'external:/media/episode.eng.forced.ass',
        sourceRevision: 'stale-revision',
        representation: 'ass',
        role: 'unknown',
        roleProvenance: 'external-sidecar',
      },
    } as never;
    render(<ExternalSubtitleList values={[denied]} loading={false} deleting={false} renaming={false} onRename={vi.fn()} onEdit={vi.fn()} onDelete={vi.fn()} onRestyle={vi.fn()} />);
    expect(screen.queryByRole('button', { name: 'Restyle' })).toBeNull();
    expect(screen.getByText('source revision is stale')).toBeTruthy();
  });

  it('shows progress, cancel, failure, and backend-authorized retry for restyle operations', async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();
    const onRetry = vi.fn();
    const operation = { operationId: 'subtitle-restyle-1', status: 'running' as const, progress: 42, phase: 'publishing', message: 'Publishing sibling' };
    render(<ExternalSubtitleList values={[sidecar]} loading={false} deleting={false} renaming={false} onRename={vi.fn()} onEdit={vi.fn()} onDelete={vi.fn()} restyleOperations={{ [sidecar.path]: operation }} onCancelRestyle={onCancel} onRetryRestyle={onRetry} />);
    expect(screen.getByText('Publishing sibling')).toBeTruthy();
    expect(screen.getByText('42%')).toBeTruthy();
    await user.click(screen.getByRole('button', { name: 'Cancel' }));
    expect(onCancel).toHaveBeenCalledWith(sidecar);
    cleanup();
    render(<ExternalSubtitleList values={[sidecar]} loading={false} deleting={false} renaming={false} onRename={vi.fn()} onEdit={vi.fn()} onDelete={vi.fn()} restyleOperations={{ [sidecar.path]: { operationId: 'subtitle-restyle-2', status: 'failed', progress: 80, error: 'stale_source', retryable: true } }} onRetryRestyle={onRetry} />);
    await user.click(screen.getByRole('button', { name: 'Retry' }));
    expect(onRetry).toHaveBeenCalledWith(sidecar);
  });
});
