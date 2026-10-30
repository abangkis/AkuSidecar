// Synthetic private-pipe peer. No browser, network, profile or account access.
import {PassThrough} from 'node:stream';
import {serveMainBrowserTransport} from '../collection/headless/worker/native-pilot/main-browser-transport-protocol.mjs';
import {runInstagramBootstrapCorroboration} from '../collection/headless/worker/native-pilot/instagram-bootstrap-corroboration.mjs';
const scenario=process.argv[2],input=new PassThrough(),output=new PassThrough();
let bytes='';output.on('data',chunk=>{bytes+=chunk;let at;while((at=bytes.indexOf('\n'))>=0){const q=JSON.parse(bytes.slice(0,at));bytes=bytes.slice(at+1);input.write(JSON.stringify({id:q.id,error:{message:'PRIVATE_MARKER'}})+'\n');}});
const options={input:process.stdin,output:process.stdout,connect:async()=>({input,output}),onClosed:()=>process.exit(0)};
if(scenario==='scope')options.diagnoseInstagramBootstrapCorroboration=runInstagramBootstrapCorroboration;
if(scenario==='throw')options.diagnoseInstagramBootstrapCorroboration=()=>{throw Object.assign(new Error('PRIVATE_MARKER'),{diagnostic:{body:'PRIVATE_MARKER'}});};
if(scenario==='eof'||scenario==='lost')options.diagnoseInstagramBootstrapCorroboration=(_transport,_request,{trace})=>{trace.note('target_create');trace.cleanup('pending');if(scenario==='lost'){trace.cleanup('verified');trace.done('projection');trace.disposition('completed');}process.stdout.write('',()=>process.exit(0));return new Promise(()=>{});};
serveMainBrowserTransport(options);
