import { describe, expect, it } from 'vitest';
import { solveParticipationChallenge } from './proof-of-work';

describe('Participation proof of work', () => {
  it('returns a counter whose SHA-256 digest satisfies the challenge difficulty', async () => {
    const challenge = 'signed-challenge';
    const proof = await solveParticipationChallenge(challenge, 14);
    const digest = new Uint8Array(await crypto.subtle.digest('SHA-256', new TextEncoder().encode(`${challenge}:${proof}`)));

    expect(digest[0]).toBe(0);
    expect(digest[1] >> 2).toBe(0);
  });
});
