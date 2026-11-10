import { defineConfig } from '@playwright/test';
export default defineConfig({testDir:'./e2e',use:{baseURL:'http://127.0.0.1:5174'},webServer:[{command:'node scripts/e2e-onsite-stack.mjs',url:'http://127.0.0.1:8082/health/ready',reuseExistingServer:false},{command:'npm run dev -- --host 127.0.0.1',url:'http://127.0.0.1:5174',reuseExistingServer:true}]});
