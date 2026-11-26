import { describe, it, mock } from 'node:test';
import assert from 'node:assert/strict';
import { HttpBroker } from '../src/infrastructure/messaging/http-broker.mjs';

const baseConfig = {
  participationInternalUrl: 'http://participation-api:8084',
  participationInternalToken: 'test-token',
  onsiteInternalUrl: 'http://onsite-operations-api:8082',
  onsiteInternalToken: 'test-token',
  httpTimeoutMs: 2000,
};

const silentLogger = { log: () => {} };

describe('HttpBroker', () => {
  it('dispatches release_published to Participation and Onsite and marks consumers active', async () => {
    const upserts = [];
    const repo = {
      async getReleaseBundle() {
        return {
          release: { id: 'rel-1', release_version: 2, manifest: { site_slug: 'default' } },
          event: { id: 'evt-1', slug: 'summit-2026', title: 'Summit 2026', status: 'published', starts_at: '2026-12-01T09:00:00Z', ends_at: '2026-12-01T18:00:00Z', timezone: 'America/Lima' },
          formsArtifact: { id: 'fv-1', body: { consent: { version: '1.0', text: 'Ok' }, schema: { fields: [] } } },
          scheduleArtifact: { body: { sessions: [{ id: 'ses-1', title: 'Keynote' }] } },
        };
      },
      async upsertReleaseConsumer(_orgId, _relId, consumer) {
        upserts.push(consumer);
      },
    };

    const calls = [];
    const fetchMock = mock.method(globalThis, 'fetch', async (url, init) => {
      calls.push({ url, body: JSON.parse(init.body), token: init.headers['X-Internal-Token'] });
      return new Response(JSON.stringify({ status: 'ok' }), { status: 200 });
    });

    try {
      const broker = new HttpBroker(repo, baseConfig, silentLogger);
      const res = await broker.publish({
        id: 'out-1',
        organization_id: 'org-1',
        aggregate_id: 'rel-1',
        event_type: 'configuration.event.release_published.v1',
        payload: {
          topic: 'configuration.event.release_published.v1',
          business_event_id: 'evt-1',
          occurred_at: new Date().toISOString(),
          data: { publication_id: 'rel-1' },
        },
      });

      assert.equal(res.accepted, true);
      assert.equal(calls.length, 2);
      assert.equal(calls[0].body.binding_state, 'open');
      assert.equal(calls[1].body.scopes.length, 2);
      assert.deepEqual(upserts.map((u) => [u.consumer_name, u.status]), [
        ['participation', 'active'],
        ['onsite-operations', 'active'],
      ]);
    } finally {
      fetchMock.mock.restore();
    }
  });

  it('records failed consumer and throws on partial failure for retry', async () => {
    const upserts = [];
    const repo = {
      async getReleaseBundle() {
        return {
          release: { id: 'rel-1', release_version: 1, manifest: {} },
          event: { id: 'evt-1', slug: 'summit-2026', title: 'Summit 2026', status: 'published' },
          formsArtifact: null,
          scheduleArtifact: null,
        };
      },
      async upsertReleaseConsumer(_orgId, _relId, consumer) {
        upserts.push(consumer);
      },
    };

    const fetchMock = mock.method(globalThis, 'fetch', async (url) => {
      if (String(url).includes('onsite')) return new Response('down', { status: 500 });
      return new Response(JSON.stringify({ status: 'projected' }), { status: 200 });
    });

    try {
      const broker = new HttpBroker(repo, baseConfig, silentLogger);
      await assert.rejects(() => broker.publish({
        id: 'out-2',
        organization_id: 'org-1',
        aggregate_id: 'rel-1',
        event_type: 'configuration.event.release_published.v1',
        payload: { topic: 'configuration.event.release_published.v1', business_event_id: 'evt-1', data: { publication_id: 'rel-1' } },
      }), /onsite-operations/);

      assert.deepEqual(upserts.map((u) => [u.consumer_name, u.status]), [
        ['participation', 'active'],
        ['onsite-operations', 'failed'],
      ]);
    } finally {
      fetchMock.mock.restore();
    }
  });

  it('propagates closed, cancelled, and schedule.changed events', async () => {
    const repo = {
      async getEventWithSessions() {
        return {
          event: { id: 'evt-1', slug: 'summit-2026', title: 'Summit 2026', status: 'closed' },
          sessions: [{ id: 'ses-1', title: 'Track A', status: 'scheduled' }],
          release: { id: 'rel-1', release_version: 1, manifest: {} },
        };
      },
    };

    const calls = [];
    const fetchMock = mock.method(globalThis, 'fetch', async (url, init) => {
      calls.push({ url: String(url), body: JSON.parse(init.body) });
      return new Response(JSON.stringify({ status: 'ok' }), { status: 200 });
    });

    try {
      const broker = new HttpBroker(repo, baseConfig, silentLogger);
      await broker.publish({
        id: 'out-3',
        organization_id: 'org-1',
        aggregate_id: 'evt-1',
        event_type: 'configuration.event.cancelled.v1',
        payload: { topic: 'configuration.event.cancelled.v1', business_event_id: 'evt-1' },
      });
      await broker.publish({
        id: 'out-4',
        organization_id: 'org-1',
        aggregate_id: 'evt-1',
        event_type: 'configuration.schedule.changed.v1',
        payload: { topic: 'configuration.schedule.changed.v1', business_event_id: 'evt-1' },
      });

      assert.equal(calls.length, 3);
      assert.equal(calls[0].body.binding_state, 'cancelled');
      assert.equal(calls[1].body.status, 'closed');
      assert.equal(calls[2].body.scopes.length, 2);
      assert.equal(calls[2].body.scopes[1].code, 'ses-1');
    } finally {
      fetchMock.mock.restore();
    }
  });
});
