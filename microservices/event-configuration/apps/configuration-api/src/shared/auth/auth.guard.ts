import { Injectable, CanActivate, ExecutionContext, Inject } from '@nestjs/common';
import { createPublicKey, JsonWebKey, KeyObject, verify as verifySignature } from 'node:crypto';
import { IncomingHttpHeaders } from 'node:http';
import { Reflector } from '@nestjs/core';
import { SCOPES_KEY } from './scopes.decorator';
import { TenantClaims } from './tenant-claims.interface';
import { ProblemDetailsException } from '../errors/problem-details.exception';
import { validateTenantClaims } from './validate-tenant-claims';

export interface ClaimsRequest {
  headers: IncomingHttpHeaders;
  ip?: string;
}
export interface ClaimsProvider {
  resolve(request: ClaimsRequest): TenantClaims | Promise<TenantClaims>;
}
export const CLAIMS_PROVIDER = 'CLAIMS_PROVIDER';

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
type Jwk = JsonWebKey & { kid?: string; alg?: string; use?: string };
type CoreJwtClaims = {
  iss: string;
  aud: string | string[];
  sub: string;
  sid?: string;
  tenant_id: string;
  exp: number;
  nbf?: number;
  roles?: string[];
  scopes?: string[];
};
const JWKS_TIMEOUT_MS = 3_000;
const CLOCK_SKEW_SECONDS = 30;

const actors: Record<string, Pick<TenantClaims, 'sub' | 'organization_id' | 'roles' | 'scopes'>> = {
  'org-a-manager': {
    sub: '0194eb00-0000-7000-9000-000000000001',
    organization_id: '0194eb00-0000-7000-8000-000000000001',
    roles: ['event_manager'],
    scopes: ['events:write', 'events:publish', 'events:read'],
  },
  'org-a-reader': {
    sub: '0194eb00-0000-7000-9000-000000000002',
    organization_id: '0194eb00-0000-7000-8000-000000000001',
    roles: ['report_viewer'],
    scopes: ['events:read'],
  },
  'org-b-manager': {
    sub: '0194eb00-0000-7000-9000-000000000010',
    organization_id: '0194eb00-0000-7000-8000-000000000002',
    roles: ['event_manager'],
    scopes: ['events:write', 'events:publish', 'events:read'],
  },
};

function readBearerToken(headers: IncomingHttpHeaders): string | undefined {
  const authHeader = headers.authorization;
  if (typeof authHeader !== 'string') return undefined;
  return /^Bearer\s+(.+)$/i.exec(authHeader.trim())?.[1]?.trim() || undefined;
}

function decodeCoreJwt(token: string): {
  header: { alg?: string; kid?: string };
  payload: CoreJwtClaims;
  signed: string;
  signature: Buffer;
} {
  const parts = token.split('.');
  if (parts.length !== 3)
    throw ProblemDetailsException.unauthorized('Invalid bearer token format.');
  const [headerPart, payloadPart, signaturePart] = parts;
  let header: { alg?: string; kid?: string };
  let payload: CoreJwtClaims;
  try {
    header = JSON.parse(Buffer.from(headerPart, 'base64url').toString('utf8')) as {
      alg?: string;
      kid?: string;
    };
    payload = JSON.parse(Buffer.from(payloadPart, 'base64url').toString('utf8')) as CoreJwtClaims;
  } catch {
    throw ProblemDetailsException.unauthorized('Invalid bearer token payload.');
  }
  if (header.alg !== 'RS256' || !header.kid)
    throw ProblemDetailsException.unauthorized('Unsupported bearer token algorithm.');
  try {
    return {
      header,
      payload,
      signed: `${headerPart}.${payloadPart}`,
      signature: Buffer.from(signaturePart, 'base64url'),
    };
  } catch {
    throw ProblemDetailsException.unauthorized('Invalid bearer token signature.');
  }
}

@Injectable()
export class CoreJwksClaimsProvider implements ClaimsProvider {
  private cachedKeys = new Map<string, KeyObject>();
  private refreshAt = 0;

  constructor(private readonly config: { jwksUrl: string; issuer: string; audience: string }) {
    if (!config.jwksUrl || !config.issuer || !config.audience)
      throw new Error(
        'CORE_JWKS_URL, NEXO_JWT_ISSUER and NEXO_JWT_AUDIENCE are required when AUTH_MODE=nexo_jwks.',
      );
  }

  private async keyFor(kid: string): Promise<KeyObject> {
    if (Date.now() >= this.refreshAt || !this.cachedKeys.has(kid)) {
      let response: Response;
      try {
        response = await fetch(this.config.jwksUrl, {
          signal: AbortSignal.timeout(JWKS_TIMEOUT_MS),
        });
      } catch {
        throw ProblemDetailsException.unauthorized('Core signing keys are unavailable.');
      }
      if (!response.ok)
        throw ProblemDetailsException.unauthorized('Core signing keys are unavailable.');
      let body: { keys?: Jwk[] };
      try {
        body = (await response.json()) as { keys?: Jwk[] };
      } catch {
        throw ProblemDetailsException.unauthorized('Core signing keys are unavailable.');
      }
      if (!Array.isArray(body.keys))
        throw ProblemDetailsException.unauthorized('Core signing keys are unavailable.');
      const next = new Map<string, KeyObject>();
      try {
        for (const jwk of body.keys) {
          if (
            jwk.kty === 'RSA' &&
            jwk.kid &&
            jwk.use === 'sig' &&
            (!jwk.alg || jwk.alg === 'RS256')
          )
            next.set(jwk.kid, createPublicKey({ key: jwk as JsonWebKey, format: 'jwk' }));
        }
      } catch {
        throw ProblemDetailsException.unauthorized('Core signing keys are unavailable.');
      }
      this.cachedKeys = next;
      this.refreshAt = Date.now() + 5 * 60_000;
    }
    const key = this.cachedKeys.get(kid);
    if (!key) throw ProblemDetailsException.unauthorized('Unknown bearer token key.');
    return key;
  }

  async resolve(request: ClaimsRequest): Promise<TenantClaims> {
    const token = readBearerToken(request.headers);
    if (!token) throw ProblemDetailsException.unauthorized('Missing bearer token.');
    const decoded = decodeCoreJwt(token);
    const key = await this.keyFor(decoded.header.kid!);
    if (!verifySignature('RSA-SHA256', Buffer.from(decoded.signed), key, decoded.signature))
      throw ProblemDetailsException.unauthorized('Invalid bearer token signature.');
    const jwtClaims = decoded.payload;
    const audience = Array.isArray(jwtClaims.aud) ? jwtClaims.aud : [jwtClaims.aud];
    const now = Math.floor(Date.now() / 1000);
    if (
      jwtClaims.iss !== this.config.issuer ||
      !audience.includes(this.config.audience) ||
      typeof jwtClaims.sub !== 'string' ||
      !UUID.test(jwtClaims.tenant_id) ||
      typeof jwtClaims.exp !== 'number' ||
      jwtClaims.exp <= now ||
      (jwtClaims.nbf !== undefined &&
        (typeof jwtClaims.nbf !== 'number' || jwtClaims.nbf > now + CLOCK_SKEW_SECONDS))
    )
      throw ProblemDetailsException.unauthorized('Invalid or expired bearer token claims.');
    const selectedTenant = request.headers['x-tenant-id'];
    if (typeof selectedTenant === 'string' && selectedTenant !== jwtClaims.tenant_id)
      throw ProblemDetailsException.unauthorized(
        'Token tenant does not match the selected tenant.',
      );
    return {
      sub: jwtClaims.sub,
      organization_id: jwtClaims.tenant_id,
      roles: jwtClaims.roles ?? [],
      scopes: jwtClaims.scopes ?? [],
      request_id: request.headers['x-request-id'] as string | undefined,
      client_ip: request.ip,
      user_agent: request.headers['user-agent'] as string | undefined,
    };
  }
}

@Injectable()
export class MockClaimsProvider implements ClaimsProvider {
  resolve(request: ClaimsRequest): TenantClaims {
    const bearer = readBearerToken(request.headers);
    if (bearer === 'test') {
      const tenantHeader = request.headers['x-tenant-id'];
      const organization_id =
        typeof tenantHeader === 'string' && UUID.test(tenantHeader)
          ? tenantHeader
          : actors['org-a-manager'].organization_id;
      return {
        ...actors['org-a-manager'],
        organization_id,
        request_id: request.headers['x-request-id'] as string | undefined,
        client_ip: request.ip,
        user_agent: request.headers['user-agent'] as string | undefined,
      };
    }
    if (bearer) throw ProblemDetailsException.unauthorized('Invalid bearer token.');
    const actorId = request.headers['x-dev-actor'];
    const actor =
      typeof actorId === 'string' && Object.prototype.hasOwnProperty.call(actors, actorId)
        ? actors[actorId]
        : undefined;
    if (!actor) throw ProblemDetailsException.unauthorized('Unknown development actor.');
    return {
      ...actor,
      request_id: request.headers['x-request-id'] as string | undefined,
      client_ip: request.ip,
      user_agent: request.headers['user-agent'] as string | undefined,
    };
  }
}

@Injectable()
export class AuthGuard implements CanActivate {
  constructor(
    private readonly reflector: Reflector,
    @Inject(CLAIMS_PROVIDER) private readonly claimsProvider: ClaimsProvider,
  ) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const request = context.switchToHttp().getRequest();
    const claims = validateTenantClaims(await this.claimsProvider.resolve(request));
    const requiredScopes = this.reflector.getAllAndOverride<string[]>(SCOPES_KEY, [
      context.getHandler(),
      context.getClass(),
    ]);
    if (!requiredScopes?.length || !requiredScopes.some((scope) => claims.scopes.includes(scope)))
      throw ProblemDetailsException.forbidden('Insufficient permissions.');
    request.user = claims;
    return true;
  }
}
