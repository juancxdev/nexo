import {afterEach, describe, expect, it, vi} from 'vitest';
import {connectionErrorMessage, login} from './session';

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
});
