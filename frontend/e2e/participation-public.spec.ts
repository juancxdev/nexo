import { createHash } from 'node:crypto';
import { expect, test } from '@playwright/test';

const challenge = 'playwright-v2-challenge';

test('completa un registro público con challenge v2 simulado y proof válido', async ({ page }) => {
  let registrationUrl = '';
  let registrationHeaders: Record<string, string> | undefined;
  let registrationBody: unknown;
  let ticketAuthorization: string | undefined;

  await page.route('**/participation/public/v2/sites/*/events/*/form', (route) =>
    route.fulfill({
      json: {
        site_slug: 'incuba-unas',
        event_slug: 'demo',
        event_id: 'demo',
        event_title: 'Demo Day',
        form_version_id: 'form-1',
        consent_version: 'consent-v1',
        binding_state: 'open',
        consent_text: 'Acepto',
        capacity_summary: { remaining: 10, public_state: 'available', waitlist_enabled: true },
        schema: { properties: {} },
      },
    }),
  );
  await page.route('**/participation/public/v2/sites/*/events/*/challenge', (route) =>
    route.fulfill({
      json: {
        challenge,
        difficulty: 14,
        expires_at: new Date(Date.now() + 60_000).toISOString(),
      },
    }),
  );
  await page.route('**/participation/public/v2/sites/*/events/*/registrations', async (route) => {
    registrationUrl = route.request().url();
    registrationHeaders = route.request().headers();
    registrationBody = route.request().postDataJSON();
    await route.fulfill({
      status: 201,
      json: {
        id: 'registration-1',
        event_id: 'demo',
        state: 'confirmed',
        ticket_id: 'ticket-1',
        self_access_token: 'self-token',
      },
    });
  });
  await page.route('**/participation/self/v1/registrations/*/ticket', async (route) => {
    ticketAuthorization = route.request().headers().authorization;
    await route.fulfill({
      json: {
        id: 'ticket-1',
        state: 'active',
        ticket_jws: 'eyJhbGciOiJIUzI1NiJ9.eyJqdGkiOiJ0aWNrZXQtMSJ9.signature',
        expires_at: '2099-01-01T00:00:00Z',
      },
    });
  });
  await page.route('**/participation/self/v1/registrations/*/credential', (route) =>
    route.fulfill({ json: { state: 'failed' } }),
  );

  await page.goto('/participation?site=incuba-unas&event=demo');
  await expect(page.getByRole('heading', { name: 'Demo Day' })).toBeVisible();
  await page.getByLabel('Nombre completo').fill('Juan Campos');
  await page.getByLabel('Correo').fill('juan@example.com');
  await page.getByRole('checkbox').check();
  await page.getByRole('button', { name: /Confirmar inscripción|Confirm registration/ }).click();
  await expect(
    page.getByRole('heading', { name: /¡Tu inscripción está lista!|Your registration is ready!/ }),
  ).toBeVisible();

  expect(registrationUrl).toContain('/participation/public/v2/sites/incuba-unas/events/demo/registrations');
  expect(registrationHeaders).toMatchObject({
    'idempotency-key': expect.stringMatching(/^participation-/),
    'x-participation-challenge': challenge,
    'x-participation-proof': expect.stringMatching(/^\d+$/),
  });
  const proof = registrationHeaders?.['x-participation-proof'];
  expect(proof).toBeDefined();
  const digest = createHash('sha256').update(`${challenge}:${proof}`).digest();
  expect(digest[0]).toBe(0);
  expect(digest[1] >> 2).toBe(0);
  expect(registrationBody).toMatchObject({
    consent_accepted: true,
    attendee: { full_name: 'Juan Campos', email: 'juan@example.com' },
  });

  await expect.poll(() => ticketAuthorization).toBe('Bearer self-token');
});
