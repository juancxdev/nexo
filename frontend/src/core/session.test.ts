import {afterEach, describe, expect, it, vi} from 'vitest';
import {connectionErrorMessage, hydrateSession, login, readSession} from './session';

describe('cliente de sesión', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('muestra un mensaje claro cuando no hay conexión con el servidor', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')));
    vi.spyOn(console, 'error').mockImplementation(() => {});
    await expect(login('persona@nexo.test', 'Clave123!')).rejects.toThrow(connectionErrorMessage);
  });

  it('mantiene el mensaje del servidor cuando sí responde con error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({code: 'invalid_credentials', message: 'credenciales inválidas'}), {status: 401})));
    await expect(login('persona@nexo.test', 'Clave123!')).rejects.toThrow('credenciales inválidas');
  });

  it('guarda el token contextual del primer tenant al hidratar una sesión', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify({profile: {id: 'user-1', name: 'Persona', email: 'persona@nexo.test'}, tenants: [{id: 'tenant-1', name: 'IncubaUNAS', slug: 'incubaunas', created_at: '2026-01-01T00:00:00Z'}]}), {status: 200}))
      .mockResolvedValueOnce(new Response(JSON.stringify({access_token: 'context-token', expires_at: 4102444800, tenant_id: 'tenant-1', role: 'owner', scopes: ['events:read']}), {status: 200}));
    vi.stubGlobal('fetch', fetchMock);

    const result = await hydrateSession({access_token: 'login-token', refresh_token: 'refresh-token', expires_at: 4102440000, profile: {id: 'stale', name: 'Stale', email: 'stale@nexo.test'}});

    expect(fetchMock.mock.calls.map(([url]) => url)).toEqual(['http://localhost:8081/v1/auth/me', 'http://localhost:8081/v1/auth/tenant-context']);
    expect(JSON.parse(String(fetchMock.mock.calls[1][1]?.body))).toEqual({tenant_id: 'tenant-1'});
    expect(result.access_token).toBe('context-token');
    expect(readSession()?.access_token).toBe('context-token');
  });
});
