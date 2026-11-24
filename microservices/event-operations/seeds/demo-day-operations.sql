INSERT INTO integration.hukid_configs(id,tenant_id,issuer_id,credential_definition,base_url,oauth_url,status)
VALUES ('00000000-0000-0000-0000-000000000401','00000000-0000-0000-0000-000000000001','iss_incubaunas_dev','event-attendance-certificate@1.0.0','http://hukid-simulator:8090/v1','http://hukid-simulator:8090/oauth2/token','active')
ON CONFLICT (id) DO UPDATE SET base_url=EXCLUDED.base_url,oauth_url=EXCLUDED.oauth_url,status='active';
UPDATE integration.hukid_configs
SET participation_credential_definition='event-participation@1.0.0'
WHERE id='00000000-0000-0000-0000-000000000401';
INSERT INTO certificates.certificates(id,tenant_id,event_id,registration_id,verification_code,display_name,state,evidence_digest)
VALUES ('00000000-0000-0000-0000-000000000402','00000000-0000-0000-0000-000000000001','event-demo','registration-demo-001','NX-DEMO-000001','Participante Demo','credential_issued','dev-seed-evidence-digest')
ON CONFLICT (id) DO NOTHING;
