import {spawn,spawnSync} from 'node:child_process';

const root=new URL('../../',import.meta.url).pathname;
const composeArgs=['compose','-p','nexo-frontend-e2e','-f','docker-compose.yml','-f','docker-compose.e2e.yml'];
const initArgs=['compose','-p','nexo-frontend-e2e','-f','docker-compose.yml','-f','docker-compose.e2e.yml','-f','docker-compose.init.yml'];
const run=(args)=>{const result=spawnSync('docker',args,{cwd:root,stdio:'inherit'});return result.status??1};
run([...composeArgs,'down','-v','--remove-orphans']);
if(run([...composeArgs,'up','-d','--wait','--wait-timeout','120','postgres'])!==0)process.exit(1);
for(const job of ['migrate-core','migrate-onsite','migrate-participation','migrate-operations','seed-onsite','seed-participation','bootstrap-runtime-roles']){
  if(run([...initArgs,'run','--rm','--no-deps',job])!==0)process.exit(1);
}
const compose=spawn('docker',[...composeArgs,'up','-d','--build','--no-deps','onsite-operations-api','participation-api'],{cwd:root,stdio:'inherit'});
const exitCode=await new Promise(resolve=>compose.on('exit',code=>resolve(code??1)));
if(exitCode!==0)process.exit(exitCode);
let stopping=false;
const stop=()=>{if(stopping)return;stopping=true;run([...composeArgs,'down','-v','--remove-orphans']);process.exit(0)};
process.on('SIGINT',stop);process.on('SIGTERM',stop);process.on('exit',()=>{if(!stopping)spawnSync('docker',[...composeArgs,'down','-v','--remove-orphans'],{cwd:root,stdio:'inherit'})});
setInterval(()=>{},60_000);
