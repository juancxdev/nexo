import { randomUUID } from 'node:crypto';

export class HttpBroker {
  constructor(repository, config, logger = console) {
    this.repository = repository;
    this.config = config;
    this.logger = logger;
  }

  async postJson(url, token, body) {
    const res = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Internal-Token': token,
      },
      body: JSON.stringify(body),
      signal: AbortSignal.timeout(this.config.httpTimeoutMs),
    });
    if (!res.ok) {
      const text = await res.text().catch(() => '');
      throw new Error(`HTTP ${res.status} from ${url}: ${text.slice(0, 200)}`);
    }
    return res.json().catch(() => ({}));
  }

  buildParticipationPayload(bundle, row, overrideBindingState = 'open') {
    const event = bundle.event || {};
    const manifest = bundle.release?.manifest || {};
    const formsBody = bundle.formsArtifact?.body || {};
    const consent = formsBody.consent || {};
    const siteSlug = manifest.site_slug || manifest.site?.slug || 'default';
    const eventSlug = event.slug || manifest.event?.slug || row.aggregate_id;

    return {
      site_slug: siteSlug,
      event_slug: eventSlug,
      event_id: event.id || row.payload?.business_event_id || row.aggregate_id,
      event_title: event.title || manifest.event?.title || 'Evento Nexo',
      form_version_id: bundle.formsArtifact?.id || bundle.release?.id || row.aggregate_id,
      consent_version: String(consent.version || '1.0'),
      binding_state: overrideBindingState,
      consent_text: String(consent.text || ''),
      schema: formsBody.schema || formsBody.fields || formsBody,
      capacity: Number(event.max_capacity ?? manifest.event?.capacity ?? 0),
      waitlist_enabled: Boolean(manifest.registration_policy?.waitlist_enabled),
    };
  }

  buildOnsitePayload(bundle, row, overrideStatus = null) {
    const event = bundle.event || {};
    const manifest = bundle.release?.manifest || {};
    const scheduleBody = bundle.scheduleArtifact?.body || {};
    const rawSessions = Array.isArray(bundle.sessions)
      ? bundle.sessions
      : (Array.isArray(scheduleBody.sessions) ? scheduleBody.sessions : []);

    const scopes = [{ code: 'main', name: 'Acceso general', kind: 'event' }];
    for (const s of rawSessions) {
      const code = String(s.code || s.id || '').trim();
      if (!code || code === 'main') continue;
      scopes.push({
        code,
        name: String(s.title || s.name || code),
        kind: 'session',
      });
    }

    return {
      tenant_id: row.organization_id,
      source_event_id: event.id || row.payload?.business_event_id || row.aggregate_id,
      name: event.title || manifest.event?.title || 'Evento Nexo',
      status: overrideStatus || event.status || 'published',
      starts_at: event.starts_at ? new Date(event.starts_at).toISOString() : undefined,
      ends_at: event.ends_at ? new Date(event.ends_at).toISOString() : undefined,
      timezone: event.timezone || 'America/Lima',
      published_revision: Number(bundle.release?.release_version || 1),
      participation_event_id: event.id || row.payload?.business_event_id || row.aggregate_id,
      scopes,
    };
  }

  async publish(row) {
    const topic = row.payload?.topic || row.event_type;

    if (topic === 'configuration.event.release_published.v1') {
      const releaseId = row.payload?.data?.publication_id || row.aggregate_id;
      const eventId = row.payload?.business_event_id || row.aggregate_id;
      const bundle = await this.repository.getReleaseBundle(row.organization_id, releaseId, eventId);
      if (!bundle.release) {
        throw new Error(`RELEASE_NOT_FOUND:${releaseId}`);
      }

      const occurredAtMs = Date.parse(row.payload?.occurred_at || '');
      const lagMs = Number.isFinite(occurredAtMs) ? Math.max(0, Date.now() - occurredAtMs) : 0;
      const targets = [];

      if (this.config.participationInternalUrl) {
        targets.push({
          name: 'participation',
          version: 'v1',
          run: () => this.postJson(
            `${this.config.participationInternalUrl.replace(/\/$/, '')}/participation/internal/v1/releases`,
            this.config.participationInternalToken,
            this.buildParticipationPayload(bundle, row),
          ),
        });
      }

      if (this.config.onsiteInternalUrl) {
        targets.push({
          name: 'onsite-operations',
          version: 'v1',
          run: () => this.postJson(
            `${this.config.onsiteInternalUrl.replace(/\/$/, '')}/onsite/internal/v1/releases`,
            this.config.onsiteInternalToken,
            this.buildOnsitePayload(bundle, row),
          ),
        });
      }

      const settled = await Promise.allSettled(targets.map((t) => t.run()));
      const failures = [];

      for (let i = 0; i < targets.length; i += 1) {
        const target = targets[i];
        const outcome = settled[i];
        if (outcome.status === 'fulfilled') {
          await this.repository.upsertReleaseConsumer(row.organization_id, releaseId, {
            consumer_name: target.name,
            consumer_version: target.version,
            status: 'active',
            acknowledged_at: new Date().toISOString(),
            last_error: null,
            lag_ms: lagMs,
            metadata: outcome.value || {},
          });
        } else {
          const msg = String(outcome.reason?.message || outcome.reason || 'HTTP_DISPATCH_FAILED');
          failures.push(`${target.name}: ${msg}`);
          await this.repository.upsertReleaseConsumer(row.organization_id, releaseId, {
            consumer_name: target.name,
            consumer_version: target.version,
            status: 'failed',
            acknowledged_at: null,
            last_error: msg.slice(0, 500),
            lag_ms: null,
            metadata: {},
          });
        }
      }

      if (failures.length > 0) {
        throw new Error(failures.join('; '));
      }

      this.logger.log(JSON.stringify({
        level: 'info',
        event: 'http_broker_release_dispatched',
        outbox_id: row.id,
        release_id: releaseId,
        consumers: targets.map((t) => t.name),
        lag_ms: lagMs,
      }));
      return { accepted: true, broker_message_id: randomUUID() };
    }

    if (topic === 'configuration.event.closed.v1' || topic === 'configuration.event.cancelled.v1') {
      const eventId = row.payload?.business_event_id || row.aggregate_id;
      const bundle = await this.repository.getEventWithSessions(row.organization_id, eventId);
      if (bundle.event) {
        const bindingState = topic === 'configuration.event.cancelled.v1' ? 'cancelled' : 'closed';
        const dispatches = [];
        if (this.config.participationInternalUrl) {
          dispatches.push(this.postJson(
            `${this.config.participationInternalUrl.replace(/\/$/, '')}/participation/internal/v1/releases`,
            this.config.participationInternalToken,
            this.buildParticipationPayload(bundle, row, bindingState),
          ));
        }
        if (this.config.onsiteInternalUrl) {
          dispatches.push(this.postJson(
            `${this.config.onsiteInternalUrl.replace(/\/$/, '')}/onsite/internal/v1/releases`,
            this.config.onsiteInternalToken,
            this.buildOnsitePayload(bundle, row, 'closed'),
          ));
        }
        await Promise.all(dispatches);
      }
    }

    if (topic === 'configuration.schedule.changed.v1' && this.config.onsiteInternalUrl) {
      const eventId = row.payload?.business_event_id || row.aggregate_id;
      const bundle = await this.repository.getEventWithSessions(row.organization_id, eventId);
      if (bundle.event) {
        await this.postJson(
          `${this.config.onsiteInternalUrl.replace(/\/$/, '')}/onsite/internal/v1/releases`,
          this.config.onsiteInternalToken,
          this.buildOnsitePayload(bundle, row),
        );
      }
    }

    return { accepted: true, broker_message_id: randomUUID() };
  }
}
