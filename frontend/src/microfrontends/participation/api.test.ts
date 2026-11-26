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

  it('uses the v2 challenge headers and preserves the registration payload', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ challenge: 'signed-challenge', difficulty: 14, expires_at: '2026-11-25T23:00:00Z' }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ id: 'r2', state: 'confirmed', self_access_token: 'token' }), { status: 201 }));
    const client = createParticipationClient('http://participation.local');
    const challenge = await client.getChallenge('site', 'event');
    const payload = { attendee: { full_name: 'Ana Lima', email: 'ana@example.com' } };
    await client.registerV2('site', 'event', payload, 'stable-key', challenge.challenge, '42');

    expect(fetchMock.mock.calls[0][0]).toContain('/participation/public/v2/sites/site/events/event/challenge');
    expect(fetchMock.mock.calls[0][1]?.credentials).toBe('include');
    expect(fetchMock.mock.calls[1][0]).toContain('/participation/public/v2/sites/site/events/event/registrations');
    expect(fetchMock.mock.calls[1][1]).toMatchObject({
      credentials: 'include',
      headers: {
        'Idempotency-Key': 'stable-key',
        'X-Participation-Challenge': 'signed-challenge',
        'X-Participation-Proof': '42',
      },
      body: JSON.stringify(payload),
    });
  });

  it('keeps the existing v1 registration client contract', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ id: 'r1', state: 'confirmed', self_access_token: 'token' }), { status: 201 }));
    const payload = { attendee: { full_name: 'Juan Campos', email: 'juan@example.com' } };
    await createParticipationClient('http://participation.local').register('site', 'event', payload, 'stable-key');
    const request = fetchMock.mock.calls[0][1] as RequestInit;

    expect(fetchMock.mock.calls[0][0]).toContain('/participation/public/v1/sites/site/events/event/registrations');
    expect(request.headers).toMatchObject({ 'Idempotency-Key': 'stable-key' });
    expect(JSON.parse(String(request.body))).toEqual(payload);
  });

  it('normalizes RFC 9457 errors for the UI', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ title: 'capacity_exhausted', detail: 'No quedan cupos.' }), { status: 409 }));
    await expect(createParticipationClient('http://participation.local').register('site', 'event', {}, 'stable-key')).rejects.toEqual(expect.objectContaining({ name: 'Error', status: 409, message: 'No quedan cupos.' } satisfies Partial<ParticipationApiError>));
  });
});
