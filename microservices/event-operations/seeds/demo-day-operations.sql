INSERT INTO integration.hukid_configs(id,tenant_id,issuer_id,credential_definition,base_url,oauth_url,status)
VALUES ('00000000-0000-0000-0000-000000000401','00000000-0000-0000-0000-000000000001','iss_incubaunas_dev','event-attendance-certificate@1.0.0','http://hukid-simulator:8090/v1','http://hukid-simulator:8090/oauth2/token','active')
ON CONFLICT (id) DO UPDATE SET base_url=EXCLUDED.base_url,oauth_url=EXCLUDED.oauth_url,status='active';
INSERT INTO certificates.certificates(id,tenant_id,event_id,registration_id,verification_code,display_name,state,artifact_key,artifact_sha256)
VALUES ('00000000-0000-0000-0000-000000000402','00000000-0000-0000-0000-000000000001','event-demo','registration-demo-001','NX-DEMO-000001','Participante Demo','issued','certificates/00000000-0000-0000-0000-000000000402.pdf','dev-seed-sha256')
ON CONFLICT (id) DO NOTHING;
