export type ParticipationForm = {
  site_slug: string;
  event_slug: string;
  event_id: string;
  event_title?: string;
  form_version_id: string;
  consent_version: string;
  binding_state: 'scheduled' | 'open' | 'closed' | 'cancelled';
  capacity_summary?: {
    remaining?: number;
    public_state?: 'available' | 'limited' | 'waitlist_only' | 'closed';
    waitlist_enabled?: boolean;
  };
  consent_text: string;
  schema?: { properties?: Record<string, { title?: string; type?: string }> };
};

export type RegistrationReceipt = {
  id: string;
  event_id: string;
  state: 'pending_verification' | 'pending_review' | 'waitlisted' | 'confirmed' | 'rejected' | 'cancelled';
  waitlist_position?: number | null;
  ticket_id?: string | null;
  self_access_token: string;
};

export type TicketResponse = {
  id: string;
  state: 'active' | 'revoked' | 'expired' | 'replaced';
  ticket_jws: string;
  expires_at: string;
};

export class ParticipationApiError extends Error {
  constructor(message: string, readonly status: number, readonly code?: string) {
    super(message);
  }
}

type RequestOptions = { method?: string; body?: unknown; token?: string; idempotencyKey?: string };

function randomKey() {
  return `participation-${crypto.randomUUID()}`;
}

export function createParticipationClient(baseUrl: string) {
  const request = async <T>(path: string, options: RequestOptions = {}): Promise<T> => {
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (options.body !== undefined) headers['Content-Type'] = 'application/json';
    if (options.token) headers.Authorization = `Bearer ${options.token}`;
    if (options.method && options.method !== 'GET') headers['Idempotency-Key'] = options.idempotencyKey || randomKey();
    const response = await fetch(`${baseUrl.replace(/\/$/, '')}${path}`, {
      method: options.method ?? 'GET',
      headers,
      body: options.body === undefined ? undefined : JSON.stringify(options.body),
    });
    const text = await response.text();
    let payload: unknown = null;
    try { payload = text ? JSON.parse(text) : null; } catch { payload = text; }
    if (!response.ok) {
      const data = payload as { detail?: string; title?: string; code?: string } | null;
      throw new ParticipationApiError(data?.detail || data?.title || `Error HTTP ${response.status}`, response.status, data?.code);
    }
    return payload as T;
  };

  return {
    loadForm: (site: string, event: string) => request<ParticipationForm>(`/participation/public/v1/sites/${encodeURIComponent(site)}/events/${encodeURIComponent(event)}/form`),
    register: (site: string, event: string, body: unknown, idempotencyKey: string) => request<RegistrationReceipt>(`/participation/public/v1/sites/${encodeURIComponent(site)}/events/${encodeURIComponent(event)}/registrations`, { method: 'POST', body, idempotencyKey }),
    getTicket: (id: string, token: string) => request<TicketResponse>(`/participation/self/v1/registrations/${encodeURIComponent(id)}/ticket`, { token }),
  };
}
