import {test,expect} from '@playwright/test';

test('permite completar un registro público de participación',async({page})=>{
  await page.route('**/participation/public/v1/sites/*/events/*/form',route=>route.fulfill({json:{site_slug:'incuba-unas',event_slug:'demo',event_id:'demo',event_title:'Demo Day',form_version_id:'form-1',consent_version:'consent-v1',binding_state:'open',consent_text:'Acepto',capacity_summary:{remaining:10,public_state:'available',waitlist_enabled:true},schema:{properties:{}}}}));
  await page.route('**/participation/public/v1/sites/*/events/*/registrations',route=>route.fulfill({status:201,json:{id:'registration-1',event_id:'demo',state:'confirmed',ticket_id:'ticket-1',self_access_token:'self-token'}}));
  await page.route('**/participation/self/v1/registrations/*/ticket',route=>route.fulfill({json:{id:'ticket-1',state:'active',ticket_jws:'eyJhbGciOiJIUzI1NiJ9.eyJqdGkiOiJ0aWNrZXQtMSJ9.signature',expires_at:'2099-01-01T00:00:00Z'}}));
  await page.goto('/participation?site=incuba-unas&event=demo');
  await expect(page.getByRole('heading',{name:'Demo Day'})).toBeVisible();
  await page.getByLabel('Nombre completo').fill('Juan Campos');
  await page.getByLabel('Correo').fill('juan@example.com');
  await page.getByRole('checkbox').check();
  await page.getByRole('button',{name:/Confirmar inscripción|Confirm registration/}).click();
  await expect(page.getByRole('heading',{name:/¡Tu inscripción está lista!|Your registration is ready!/})).toBeVisible();
});
