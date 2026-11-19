import { ClaimsProvider, CoreJwksClaimsProvider } from './auth.guard';

/** Selects either the verified Core JWKS provider or the explicit local mock adapter. */
export function selectClaimsProvider(env: NodeJS.ProcessEnv, mock: ClaimsProvider): ClaimsProvider {
  if (env.AUTH_MODE === 'nexo_jwks') {
    return new CoreJwksClaimsProvider({
      jwksUrl: env.CORE_JWKS_URL || '',
      issuer: env.NEXO_JWT_ISSUER || '',
      audience: env.NEXO_JWT_AUDIENCE || '',
    });
  }
  if (env.AUTH_MODE === 'mock' && (env.NODE_ENV === 'development' || env.NODE_ENV === 'test')) return mock;
  if (env.AUTH_MODE === 'mock') throw new Error('AUTH_MODE=mock is restricted to development or test.');
  throw new Error('A verified identity provider is not configured.');
}
