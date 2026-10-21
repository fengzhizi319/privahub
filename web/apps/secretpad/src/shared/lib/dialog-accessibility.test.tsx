import React, { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react';
import { Drawer, Modal } from '@secretpad/design-system';

// jsdom has no layout; simulate visible controls for the production focus cycle.
beforeEach(() => {
  vi.spyOn(HTMLElement.prototype, 'getClientRects').mockReturnValue([{ width: 1, height: 1 }] as unknown as DOMRectList);
});
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function DialogFlow() {
  const [drawer, setDrawer] = useState(false);
  const [modal, setModal] = useState(false);
  return <>
    <button onClick={() => setDrawer(true)}>Open resources</button>
    <Drawer isOpen={drawer} onClose={() => setDrawer(false)} title="Resources">
      <button onClick={() => setModal(true)}>Confirm change</button>
      <Modal isOpen={modal} onClose={() => setModal(false)} title="Confirmation">
        <input aria-label="Reason" />
        <button onClick={() => setModal(false)}>Finish</button>
      </Modal>
    </Drawer>
  </>;
}

describe('shared modal keyboard accessibility', () => {
  it('labels surfaces and closes only the topmost dialog, restoring the opener', () => {
    render(<DialogFlow />);
    const opener = screen.getByRole('button', { name: 'Open resources' });
    opener.focus(); fireEvent.click(opener);
    const drawer = screen.getByRole('dialog', { name: 'Resources' });
    expect(drawer.getAttribute('aria-modal')).toBe('true');
    const nestedOpener = within(drawer).getByRole('button', { name: 'Confirm change' });
    nestedOpener.focus(); fireEvent.click(nestedOpener);
    expect(screen.getByRole('dialog', { name: 'Confirmation' }).getAttribute('aria-modal')).toBe('true');
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog', { name: 'Confirmation' })).toBeNull();
    expect(document.activeElement).toBe(nestedOpener);
    expect(screen.getByRole('dialog', { name: 'Resources' })).toBeTruthy();
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(document.activeElement).toBe(opener);
  });

  it('cycles Tab inside the dialog and redirects background focus', () => {
    render(<DialogFlow />);
    fireEvent.click(screen.getByRole('button', { name: 'Open resources' }));
    const dialog = screen.getByRole('dialog', { name: 'Resources' });
    const first = within(dialog).getByRole('button', { name: 'close' });
    const last = within(dialog).getByRole('button', { name: 'Confirm change' });
    expect(document.activeElement).toBe(first);
    fireEvent.keyDown(document, { key: 'Tab', shiftKey: true });
    expect(document.activeElement).toBe(last);
    fireEvent.keyDown(document, { key: 'Tab' });
    expect(document.activeElement).toBe(first);
    screen.getByRole('button', { name: 'Open resources' }).focus();
    expect(dialog.contains(document.activeElement)).toBe(true);
  });
});
