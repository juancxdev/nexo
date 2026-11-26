export class OutboxRepository {
  constructor(pool, config) {
    this.pool = pool;
    this.config = config;
  }

  async claimBatch(tenantId, workerId) {
    return this.withTenantTransaction(tenantId, async (client) => {
      const selected = await client.query(`
        SELECT id
        FROM events.outbox
        WHERE organization_id = $1
          AND status = 'pending'
          AND next_retry_at <= clock_timestamp()
          AND (locked_until IS NULL OR locked_until < clock_timestamp())
        ORDER BY next_retry_at, created_at, id
        LIMIT $2
        FOR UPDATE SKIP LOCKED
      `, [tenantId, this.config.batchSize]);
      if (!selected.rowCount) return [];
      const ids = selected.rows.map((row) => row.id);
      const claimed = await client.query(`
        UPDATE events.outbox
        SET locked_by = $2, locked_until = clock_timestamp() + ($3 * interval '1 second')
        WHERE organization_id = $1 AND id = ANY($4::uuid[])
        RETURNING id, organization_id, event_type, aggregate_id, payload, headers, retry_count, max_retries
      `, [tenantId, workerId, this.config.leaseSeconds, ids]);
      return claimed.rows;
    });
  }

  async complete(row, workerId) {
    const result = await this.withTenantTransaction(row.organization_id, (client) => client.query(`
      UPDATE events.outbox
      SET status = 'delivered', delivered_at = clock_timestamp(), last_error = NULL,
          locked_by = NULL, locked_until = NULL
      WHERE organization_id = $1 AND id = $2 AND locked_by = $3
        AND status = 'pending' AND locked_until > clock_timestamp()
    `, [row.organization_id, row.id, workerId]));
    return result.rowCount === 1;
  }

  async fail(row, workerId, retry, errorMessage) {
    const result = await this.withTenantTransaction(row.organization_id, (client) => client.query(`
      UPDATE events.outbox
      SET retry_count = $4, status = $5::varchar,
          next_retry_at = CASE WHEN $5::varchar = 'pending' THEN clock_timestamp() + ($6 * interval '1 second') ELSE next_retry_at END,
          last_error = $7, locked_by = NULL, locked_until = NULL
      WHERE organization_id = $1 AND id = $2 AND locked_by = $3
        AND status = 'pending' AND locked_until > clock_timestamp()
    `, [row.organization_id, row.id, workerId, retry.attempts, retry.terminal ? 'failed' : 'pending', retry.jittered, errorMessage.slice(0, 500)]));
    return result.rowCount === 1;
  }

  async getReleaseBundle(tenantId, releaseId, eventId) {
    return this.withTenantTransaction(tenantId, async (client) => {
      const releaseRes = await client.query(`
        SELECT id, organization_id, event_id, release_version, manifest, status
        FROM events.releases
        WHERE organization_id = $1 AND id = $2
      `, [tenantId, releaseId]);
      const eventRes = await client.query(`
        SELECT id, organization_id, site_id, slug, title, status, starts_at, ends_at, timezone, policy
        FROM events.events
        WHERE organization_id = $1 AND id = $2
      `, [tenantId, eventId]);
      const artifactsRes = await client.query(`
        SELECT id, kind, version, body
        FROM events.release_artifacts
        WHERE organization_id = $1 AND release_id = $2
      `, [tenantId, releaseId]);
      const artifacts = artifactsRes.rows || [];
      return {
        release: releaseRes.rows[0] || null,
        event: eventRes.rows[0] || null,
        formsArtifact: artifacts.find((a) => a.kind === 'forms') || null,
        scheduleArtifact: artifacts.find((a) => a.kind === 'schedule') || null,
      };
    });
  }

  async listPendingOrganizations() {
    const res = await this.pool.query('SELECT organization_id FROM events.list_pending_outbox_organizations()');
    return res.rows.map((r) => r.organization_id);
  }

  async getEventWithSessions(tenantId, eventId) {
    return this.withTenantTransaction(tenantId, async (client) => {
      const eventRes = await client.query(`
        SELECT id, organization_id, site_id, slug, title, status, starts_at, ends_at, timezone, policy
        FROM events.events
        WHERE organization_id = $1 AND id = $2
      `, [tenantId, eventId]);
      const sessionsRes = await client.query(`
        SELECT id, title, status
        FROM events.sessions
        WHERE organization_id = $1 AND event_id = $2 AND status != 'cancelled'
        ORDER BY starts_at ASC, id ASC
      `, [tenantId, eventId]);
      const releaseRes = await client.query(`
        SELECT id, release_version, manifest
        FROM events.releases
        WHERE organization_id = $1 AND event_id = $2
        ORDER BY release_version DESC
        LIMIT 1
      `, [tenantId, eventId]);
      return {
        event: eventRes.rows[0] || null,
        sessions: sessionsRes.rows || [],
        release: releaseRes.rows[0] || null,
      };
    });
  }

  async upsertReleaseConsumer(tenantId, releaseId, consumer) {
    return this.withTenantTransaction(tenantId, (client) => client.query(`
      INSERT INTO events.release_consumers (
        organization_id, release_id, consumer_name, consumer_version, status, acknowledged_at, last_error, lag_ms, metadata, updated_at
      ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::jsonb, clock_timestamp())
      ON CONFLICT (organization_id, release_id, consumer_name) DO UPDATE SET
        consumer_version = EXCLUDED.consumer_version,
        status = EXCLUDED.status,
        acknowledged_at = EXCLUDED.acknowledged_at,
        last_error = EXCLUDED.last_error,
        lag_ms = EXCLUDED.lag_ms,
        metadata = EXCLUDED.metadata,
        updated_at = clock_timestamp()
    `, [
      tenantId,
      releaseId,
      consumer.consumer_name,
      consumer.consumer_version || null,
      consumer.status,
      consumer.acknowledged_at || null,
      consumer.last_error || null,
      consumer.lag_ms ?? null,
      JSON.stringify(consumer.metadata || {}),
    ]));
  }

  async withTenantTransaction(tenantId, action) {
    const client = await this.pool.connect();
    try {
      await client.query('BEGIN');
      await client.query("SELECT set_config('app.current_organization_id', $1, true)", [tenantId]);
      const result = await action(client);
      await client.query('COMMIT');
      return result;
    } catch (error) {
      await client.query('ROLLBACK');
      throw error;
    } finally {
      client.release();
    }
  }
}
