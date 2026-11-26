import { randomUUID } from 'node:crypto';
import { writeFile } from 'node:fs/promises';
import { createRequire } from 'node:module';
import { OutboxRelay } from './application/outbox-relay.mjs';
import { loadDatabaseConfig, loadWorkerConfig } from './infrastructure/config/worker-config.mjs';
import { HttpBroker } from './infrastructure/messaging/http-broker.mjs';
import { SimulatedBroker } from './infrastructure/messaging/simulated-broker.mjs';
import { OutboxRepository } from './infrastructure/persistence/outbox-repository.mjs';
import { verifyWorkerAccess } from './infrastructure/persistence/verify-worker-access.mjs';

const requireFromWorker = createRequire(new URL('../package.json', import.meta.url));
const { Pool } = requireFromWorker('pg');
const config = loadWorkerConfig(process.env);
const pool = new Pool(loadDatabaseConfig(process.env));
const workerId = `${process.env.HOSTNAME || 'configuration-worker'}:${process.pid}:${randomUUID()}`;
const heartbeatFile = process.env.WORKER_HEARTBEAT_FILE || '/tmp/worker-heartbeat';
const updateHeartbeat = () => writeFile(heartbeatFile, String(Date.now()), 'utf8');
const repository = new OutboxRepository(pool, config);
const broker = config.brokerMode === 'http'
  ? new HttpBroker(repository, config)
  : new SimulatedBroker(config);
const relay = new OutboxRelay(
  repository,
  broker,
  config,
  workerId,
  console,
  updateHeartbeat,
);

process.on('SIGTERM', () => relay.stop());
process.on('SIGINT', () => relay.stop());

verifyWorkerAccess(pool).then(() => relay.run()).then(async () => {
  await pool.end();
  console.log(JSON.stringify({ level: 'info', event: 'worker_stopped' }));
}).catch(async (error) => {
  console.error(JSON.stringify({ level: 'fatal', event: 'worker_crashed', error: error.message }));
  await pool.end();
  process.exitCode = 1;
});
