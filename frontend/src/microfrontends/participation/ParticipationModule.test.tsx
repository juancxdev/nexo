import { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ParticipationModule } from './ParticipationModule';
import type { ModuleContext } from '../../platform/microfrontends/contracts';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;
let root: Root | undefined;
afterEach(() => { if (root) act(() => root?.unmount()); root = undefined; document.body.innerHTML = ''; vi.restoreAllMocks(); });

const context: ModuleContext = { version: 1, tenantId: '', workspaceId: '', capabilities: [], apiBaseUrl: 'http://participation.local', theme: 'light', route: '/participation' };

describe('ParticipationModule', () => {
  it('renders the public form and confirms a registration', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith('/form')) return new Response(JSON.stringify({ event_title: 'Demo Day', form_version_id: 'f1', consent_version: 'v1', binding_state: 'open', consent_text: 'Acepto', capacity_summary: { remaining: 20 } }), { status: 200 });
      if (url.endsWith('/challenge')) return new Response(JSON.stringify({ challenge: 'signed-challenge', difficulty: 14, expires_at: '2026-11-25T23:00:00Z' }), { status: 200 });
      if (url.endsWith('/credential')) return new Response(JSON.stringify({ state: 'failed' }), { status: 200 });
      if (url.endsWith('/ticket')) return new Response(JSON.stringify({ id: 't1', state: 'active', ticket_jws: 'jws', expires_at: '2027-01-01T00:00:00Z' }), { status: 200 });
      if (url.endsWith('/registrations')) return new Response(JSON.stringify({ id: 'r1', state: 'confirmed', self_access_token: 'token' }), { status: 201 });
      return new Response(JSON.stringify({ id: 't1', state: 'active', ticket_jws: 'jws', expires_at: '2027-01-01T00:00:00Z' }), { status: 200 });
    });
    const container = document.createElement('div'); document.body.appendChild(container); root = createRoot(container);
    await act(async () => { root!.render(<ParticipationModule context={context} />); });
    expect(container.textContent).toContain('Datos del participante');
    const inputs = container.querySelectorAll('input');
    await act(async () => { (inputs[0] as HTMLInputElement).value = 'Ana Lima'; inputs[0].dispatchEvent(new Event('input', { bubbles: true })); (inputs[1] as HTMLInputElement).value = 'ana@example.com'; inputs[1].dispatchEvent(new Event('input', { bubbles: true })); });
    await act(async () => { container.querySelector('form')?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); });
    await vi.waitFor(() => expect(container.textContent).toContain('¡Tu inscripción está lista!'), { timeout: 10000 });
    expect(container.textContent).toContain('¡Tu inscripción está lista!');
    expect(fetchMock.mock.calls.some(([input, init]) => String(input).includes('/participation/public/v2/sites/') && String(input).endsWith('/registrations') && Boolean((init?.headers as Record<string, string>)['X-Participation-Proof']))).toBe(true);
  });
});
