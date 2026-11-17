import { defineConfig } from '@playwright/test';
// El stack de Onsite se descarga y compila con Docker antes de las pruebas; en CI puede tardar más de 60 s.
const onsiteStartupTimeout=180_000;
export default defineConfig({testDir:'./e2e',use:{baseURL:'http://127.0.0.1:5174'},webServer:[{command:'node scripts/e2e-onsite-stack.mjs',url:'http://127.0.0.1:8082/health/ready',reuseExistingServer:false,timeout:onsiteStartupTimeout},{command:'npm run dev -- --host 127.0.0.1',url:'http://127.0.0.1:5174',reuseExistingServer:true}]});
