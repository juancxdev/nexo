const encoder = new TextEncoder();

export async function solveParticipationChallenge(challenge: string, difficulty: number): Promise<string> {
  if (!Number.isInteger(difficulty) || difficulty < 0 || difficulty > 16) {
    throw new Error('Unsupported challenge difficulty');
  }
  if (difficulty === 0) return '0';
  if (!globalThis.crypto?.subtle) throw new Error('Web Crypto is required to solve the registration challenge');

  for (let counter = 0; counter <= 0xffffffff; counter += 1) {
    const input = encoder.encode(`${challenge}:${counter}`);
    const digest = new Uint8Array(await globalThis.crypto.subtle.digest('SHA-256', input));
    if (hasLeadingZeroBits(digest, difficulty)) return String(counter);
    if (counter % 128 === 127) await new Promise<void>((resolve) => setTimeout(resolve, 0));
  }
  throw new Error('Challenge proof search exhausted');
}

function hasLeadingZeroBits(digest: Uint8Array, bits: number): boolean {
  const wholeBytes = Math.floor(bits / 8);
  for (let index = 0; index < wholeBytes; index += 1) {
    if (digest[index] !== 0) return false;
  }
  const remaining = bits % 8;
  return remaining === 0 || (digest[wholeBytes] >> (8 - remaining)) === 0;
}
