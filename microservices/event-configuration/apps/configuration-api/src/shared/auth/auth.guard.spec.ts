import { describe, expect, it, vi, afterEach } from 'vitest';
import { generateKeyPairSync, sign } from 'node:crypto';
import { Reflector } from '@nestjs/core';
import { ExecutionContext } from '@nestjs/common';
import {
  AuthGuard,
  ClaimsProvider,
  CoreJwksClaimsProvider,
  MockClaimsProvider,
} from './auth.guard';
import { selectClaimsProvider } from './claims-selection';
import { TenantClaims } from './tenant-claims.interface';

const issuer = 'http://core-api:8081';
const audience = 'nexo-services';
const tenantId = '0194eb00-0000-7000-8000-000000000001';
const valid: TenantClaims = {
  sub: '0194eb00-0000-7000-9000-000000000001',
  organization_id: tenantId,
  roles: ['event_manager'],
  scopes: ['events:write'],
};

function signCoreJwt(
  payload: Record<string, unknown>,
  key: ReturnType<typeof generateKeyPairSync>['privateKey'],
) {
  const header = Buffer.from(
    JSON.stringify({ alg: 'RS256', typ: 'JWT', kid: 'core-test-1' }),
  ).toString('base64url');
  const body = Buffer.from(JSON.stringify(payload)).toString('base64url');
  return `${header}.${body}.${sign('RSA-SHA256', Buffer.from(`${header}.${body}`), key).toString('base64url')}`;
}

function setup(provider: ClaimsProvider, scopes?: string[]) {
  const request: { headers: Record<string, string>; user?: TenantClaims } = {
    headers: { 'x-dev-actor': 'org-a-manager' },
  };
  const ctx = {
    switchToHttp: () => ({ getRequest: () => request }),
    getHandler: () => function handler() {},
    getClass: () => class Controller {},
  } as unknown as ExecutionContext;
  const reflector = new Reflector();
  vi.spyOn(reflector, 'getAllAndOverride').mockReturnValue(scopes);
  return { guard: new AuthGuard(reflector, provider), ctx, request };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('claims provider selection', () => {
  it('permits mock only in development and configures Core JWKS explicitly', () => {
    const mock = new MockClaimsProvider();
    expect(selectClaimsProvider({ AUTH_MODE: 'mock', NODE_ENV: 'development' }, mock)).toBe(mock);
    expect(
      selectClaimsProvider(
        {
          AUTH_MODE: 'nexo_jwks',
          CORE_JWKS_URL: 'https://core.test/jwks',
          NEXO_JWT_ISSUER: issuer,
          NEXO_JWT_AUDIENCE: audience,
        },
        mock,
      ),
    ).toBeInstanceOf(CoreJwksClaimsProvider);
    expect(() =>
      selectClaimsProvider({ AUTH_MODE: 'mock', NODE_ENV: 'production' }, mock),
    ).toThrow();
  });
});

describe('Core JWKS claims', () => {
  it('validates a signed Core token and derives tenant only from its claims', async () => {
    const keys = generateKeyPairSync('rsa', { modulusLength: 2048 });
    const jwk = keys.publicKey.export({ format: 'jwk' });
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ keys: [{ ...jwk, kid: 'core-test-1', use: 'sig', alg: 'RS256' }] }),
      }),
    );
    const token = signCoreJwt(
      {
        iss: issuer,
        aud: audience,
        sub: valid.sub,
        sid: 'sid',
        tenant_id: tenantId,
        exp: Math.floor(Date.now() / 1000) + 3600,
        roles: ['owner'],
        scopes: ['events:read', 'events:write'],
      },
      keys.privateKey,
    );
    const provider = new CoreJwksClaimsProvider({
      jwksUrl: 'https://core.test/jwks',
      issuer,
      audience,
    });
    await expect(
      provider.resolve({ headers: { authorization: `Bearer ${token}`, 'x-tenant-id': tenantId } }),
    ).resolves.toMatchObject({
      sub: valid.sub,
      organization_id: tenantId,
      scopes: ['events:read', 'events:write'],
    });
  });

  it('rejects invalid signature, expired claims and tenant-header mismatch', async () => {
    const keys = generateKeyPairSync('rsa', { modulusLength: 2048 });
    const jwk = keys.publicKey.export({ format: 'jwk' });
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ keys: [{ ...jwk, kid: 'core-test-1', use: 'sig', alg: 'RS256' }] }),
      }),
    );
    const provider = new CoreJwksClaimsProvider({
      jwksUrl: 'https://core.test/jwks',
      issuer,
      audience,
    });
    const token = signCoreJwt(
      {
        iss: issuer,
        aud: audience,
        sub: valid.sub,
        tenant_id: tenantId,
        exp: Math.floor(Date.now() / 1000) + 3600,
        scopes: ['events:read'],
      },
      keys.privateKey,
    );
    await expect(
      provider.resolve({ headers: { authorization: `Bearer ${token.slice(0, -3)}abc` } }),
    ).rejects.toMatchObject({ status: 401 });
    await expect(
      provider.resolve({
        headers: {
          authorization: `Bearer ${token}`,
          'x-tenant-id': '0194eb00-0000-7000-8000-000000000002',
        },
      }),
    ).rejects.toMatchObject({ status: 401 });
    const expired = signCoreJwt(
      {
        iss: issuer,
        aud: audience,
        sub: valid.sub,
        tenant_id: tenantId,
        exp: Math.floor(Date.now() / 1000) - 1,
        scopes: ['events:read'],
      },
      keys.privateKey,
    );
    await expect(
      provider.resolve({ headers: { authorization: `Bearer ${expired}` } }),
    ).rejects.toMatchObject({ status: 401 });
  });

  it('rejects a token that is not valid yet and malformed JWKS material', async () => {
    const keys = generateKeyPairSync('rsa', { modulusLength: 2048 });
    const jwk = keys.publicKey.export({ format: 'jwk' });
    const token = signCoreJwt(
      {
        iss: issuer,
        aud: audience,
        sub: valid.sub,
        tenant_id: tenantId,
        exp: Math.floor(Date.now() / 1000) + 3600,
        nbf: Math.floor(Date.now() / 1000) + 120,
        scopes: ['events:read'],
      },
      keys.privateKey,
    );
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ keys: [{ ...jwk, kid: 'core-test-1', use: 'sig', alg: 'RS256' }] }),
      }),
    );
    const provider = new CoreJwksClaimsProvider({
      jwksUrl: 'https://core.test/jwks',
      issuer,
      audience,
    });
    await expect(
      provider.resolve({ headers: { authorization: `Bearer ${token}` } }),
    ).rejects.toMatchObject({ status: 401 });

    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          keys: [{ kty: 'RSA', kid: 'core-test-1', use: 'sig', alg: 'RS256' }],
        }),
      }),
    );
    const malformedProvider = new CoreJwksClaimsProvider({
      jwksUrl: 'https://core.test/jwks',
      issuer,
      audience,
    });
    await expect(
      malformedProvider.resolve({ headers: { authorization: `Bearer ${token}` } }),
    ).rejects.toMatchObject({ status: 401 });
  });

  it('bounds JWKS retrieval and fails closed when the provider is unavailable', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error('timeout'));
    vi.stubGlobal('fetch', fetchMock);
    const keys = generateKeyPairSync('rsa', { modulusLength: 2048 });
    const token = signCoreJwt(
      {
        iss: issuer,
        aud: audience,
        sub: valid.sub,
        tenant_id: tenantId,
        exp: Math.floor(Date.now() / 1000) + 3600,
        scopes: ['events:read'],
      },
      keys.privateKey,
    );
    const provider = new CoreJwksClaimsProvider({
      jwksUrl: 'https://core.test/jwks',
      issuer,
      audience,
    });
    await expect(
      provider.resolve({ headers: { authorization: `Bearer ${token}` } }),
    ).rejects.toMatchObject({ status: 401 });
    expect(fetchMock.mock.calls[0]?.[1]).toMatchObject({ signal: expect.any(AbortSignal) });
  });
});

describe('authorization guard', () => {
  it('accepts allowed scopes and denies missing policies or scopes', async () => {
    const accepted = setup({ resolve: async () => valid }, ['events:read', 'events:write']);
    await expect(accepted.guard.canActivate(accepted.ctx)).resolves.toBe(true);
    expect(accepted.request.user?.organization_id).toBe(tenantId);
    for (const scopes of [['events:publish'], undefined, []]) {
      const denied = setup({ resolve: () => valid }, scopes);
      await expect(denied.guard.canActivate(denied.ctx)).rejects.toMatchObject({ status: 403 });
    }
  });

  it('does not grant write access to the fixed development reader', async () => {
    const { guard, ctx, request } = setup(new MockClaimsProvider(), ['events:write']);
    request.headers['x-dev-actor'] = 'org-a-reader';
    await expect(guard.canActivate(ctx)).rejects.toMatchObject({ status: 403 });
  });

  it('rejects malformed provider claims before evaluating authorization', async () => {
    const invalidClaims = [
      { ...valid, sub: 'not-a-uuid' },
      { ...valid, organization_id: 'not-a-uuid' },
      { ...valid, roles: ['owner', 'owner'] },
      { ...valid, scopes: ['events:read', ' events:write'] },
      { ...valid, scopes: 'events:read' },
    ];
    for (const value of invalidClaims) {
      const denied = setup({ resolve: () => value as TenantClaims }, ['events:read']);
      await expect(denied.guard.canActivate(denied.ctx)).rejects.toMatchObject({ status: 401 });
    }
  });
});
