// Synthetic private-pipe peer. No browser, network, profile or account access.
import {PassThrough} from 'node:stream';
import {serveMainBrowserTransport} from '../collection/headless/worker/native-pilot/main-browser-transport-protocol.mjs';
import {runInstagramBootstrapCorroboration} from '../collection/headless/worker/native-pilot/instagram-bootstrap-corroboration.mjs';
import {readInstagramBootstrapBodyStream} from '../collection/headless/worker/native-pilot/instagram-bootstrap-body-stream.mjs';
const scenario=process.argv[2],input=new PassThrough(),output=new PassThrough();
let bytes='';output.on('data',chunk=>{bytes+=chunk;let at;while((at=bytes.indexOf('\n'))>=0){const q=JSON.parse(bytes.slice(0,at));bytes=bytes.slice(at+1);input.write(JSON.stringify({id:q.id,error:{message:'PRIVATE_MARKER'}})+'\n');}});
const options={input:process.stdin,output:process.stdout,connect:async()=>({input,output}),onClosed:()=>process.exit(0)};
if(scenario==='scope')options.diagnoseInstagramBootstrapCorroboration=runInstagramBootstrapCorroboration;
if(scenario==='throw')options.diagnoseInstagramBootstrapCorroboration=()=>{throw Object.assign(new Error('PRIVATE_MARKER'),{diagnostic:{body:'PRIVATE_MARKER'}});};
if(['read_command','read_reply','read_empty','read_cleanup'].includes(scenario))options.diagnoseInstagramBootstrapCorroboration=async(_transport,_request,{trace})=>{
 trace.note('body_read');
 const scope={async call(method){
  if(method==='IO.close'){if(scenario==='read_cleanup')throw Error('PRIVATE_MARKER');return {};}
  if(scenario==='read_command'||scenario==='read_cleanup')throw Error('PRIVATE_MARKER');
  return scenario==='read_reply'?{data:'synthetic'}:{data:'',eof:false};
 }};
 const value=await readInstagramBootstrapBodyStream(scope,'synthetic-session','synthetic-stream',{deadlineAt:Date.now()+1000,trace});
 trace.cleanup(value.cleanup,value.cleanup==='verified'?'none':'stream_close_failed');
 return {schema:'aku.instagram-bootstrap-corroboration.v1',reason:value.reason,bodyBound:false,documentBound:false,serverFresh:false,idMatched:false,cookieMatched:false,scanComplete:false,qualified:false,cleanup:value.cleanup};
};
if(scenario==='eof'||scenario==='lost')options.diagnoseInstagramBootstrapCorroboration=(_transport,_request,{trace})=>{trace.note('target_create');trace.cleanup('pending');if(scenario==='lost'){trace.cleanup('verified');trace.done('projection');trace.disposition('completed');}process.stdout.write('',()=>process.exit(0));return new Promise(()=>{});};
serveMainBrowserTransport(options);
