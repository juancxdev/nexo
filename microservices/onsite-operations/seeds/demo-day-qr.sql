-- Datos locales para probar el QR firmado y las sesiones. Requiere demo-day.sql.
-- Clave pública de DESARROLLO de Participation (semilla sha256("nexo-development-participation-ticket-key")).
INSERT INTO onsite.ticket_keys(kid,tenant_id,alg,public_key,status) VALUES
('participation-dev-1','00000000-0000-0000-0000-000000000001','EdDSA',decode('a73b5e172f8d462d7df1a15651e65d7af01f011bdcc6e558a8dee82d70fa6e0a','hex'),'active')
ON CONFLICT (kid) DO UPDATE SET public_key=EXCLUDED.public_key,status='active';

-- El Demo Day de Onsite es el "demo-day-2026" de Participation.
UPDATE onsite.events SET participation_event_id='demo-day-2026' WHERE id='00000000-0000-0000-0000-000000000101';

-- Lista de revocaciones recién sincronizada (permite resolver tickets provisionales).
INSERT INTO onsite.revocation_state(tenant_id,list_version,synced_at) VALUES('00000000-0000-0000-0000-000000000001',1,now())
ON CONFLICT (tenant_id) DO UPDATE SET synced_at=now();
INSERT INTO onsite.ticket_revocations(tenant_id,jti) VALUES('00000000-0000-0000-0000-000000000001','DEMO-2026-REVOCADO') ON CONFLICT DO NOTHING;

-- Sesiones: un taller abierto con aforo 2 y una charla restringida.
INSERT INTO onsite.scopes(id,event_id,code,name,kind) VALUES
('00000000-0000-0000-0000-000000000201','00000000-0000-0000-0000-000000000101','taller-ia','Taller de IA','session'),
('00000000-0000-0000-0000-000000000202','00000000-0000-0000-0000-000000000101','charla-vip','Charla VIP','session') ON CONFLICT DO NOTHING;
INSERT INTO onsite.session_rules(scope_id,access,capacity,opens_at,closes_at) VALUES
('00000000-0000-0000-0000-000000000201','open',2,now()-interval '1 hour',now()+interval '8 hours'),
('00000000-0000-0000-0000-000000000202','restricted',NULL,now()-interval '1 hour',now()+interval '8 hours')
ON CONFLICT (scope_id) DO UPDATE SET opens_at=EXCLUDED.opens_at,closes_at=EXCLUDED.closes_at;
INSERT INTO onsite.ticket_sessions(ticket_id,scope_id) VALUES('DEMO-2026-002','00000000-0000-0000-0000-000000000202') ON CONFLICT DO NOTHING;

-- El staff de demostración también controla las dos sesiones con el mismo dispositivo.
INSERT INTO onsite.operator_assignments(id,user_id,event_id,scope_id,device_id,role,valid_from,valid_until) VALUES
('00000000-0000-0000-0000-000000000211','00000000-0000-0000-0000-000000000004','00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000201','00000000-0000-0000-0000-000000000103','staff',now()-interval '1 day',now()+interval '1 day'),
('00000000-0000-0000-0000-000000000212','00000000-0000-0000-0000-000000000004','00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000202','00000000-0000-0000-0000-000000000103','staff',now()-interval '1 day',now()+interval '1 day')
ON CONFLICT (id) DO UPDATE SET valid_from=EXCLUDED.valid_from,valid_until=EXCLUDED.valid_until;

-- Supervisión y gerencia de demostración en la zona principal (resolver tickets provisionales).
INSERT INTO onsite.operator_assignments(id,user_id,event_id,scope_id,device_id,role,valid_from,valid_until) VALUES
('00000000-0000-0000-0000-000000000213','00000000-0000-0000-0000-000000000004','00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000102','00000000-0000-0000-0000-000000000103','supervisor',now()-interval '1 day',now()+interval '1 day'),
('00000000-0000-0000-0000-000000000214','00000000-0000-0000-0000-000000000004','00000000-0000-0000-0000-000000000101','00000000-0000-0000-0000-000000000102','00000000-0000-0000-0000-000000000103','manager',now()-interval '1 day',now()+interval '1 day')
ON CONFLICT (id) DO UPDATE SET valid_from=EXCLUDED.valid_from,valid_until=EXCLUDED.valid_until;
