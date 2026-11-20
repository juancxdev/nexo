import { afterEach, describe, expect, it, vi } from 'vitest';
import { createParticipationClient, ParticipationApiError } from './api';

afterEach(() => vi.restoreAllMocks());

describe('Participation client', () => {
  it('sends the public registration contract with idempotency', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ id: 'r1', state: 'confirmed', self_access_token: 'token' }), { status: 201 }));
    await createParticipationClient('http://participation.local/').register('site', 'event', { attendee: { full_name: 'Juan Campos', email: 'juan@example.com' } }, 'stable-key');
    const request = fetchMock.mock.calls[0][1] as RequestInit;
    expect(fetchMock.mock.calls[0][0]).toContain('/participation/public/v1/sites/site/events/event/registrations');
    expect(request.headers).toMatchObject({ 'Idempotency-Key': 'stable-key' });
    expect(JSON.parse(String(request.body))).toMatchObject({ attendee: { email: 'juan@example.com' } });
  });

  it('normalizes RFC 9457 errors for the UI', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ title: 'capacity_exhausted', detail: 'No quedan cupos.' }), { status: 409 }));
    await expect(createParticipationClient('http://participation.local').register('site', 'event', {}, 'stable-key')).rejects.toEqual(expect.objectContaining({ name: 'Error', status: 409, message: 'No quedan cupos.' } satisfies Partial<ParticipationApiError>));
  });
});
