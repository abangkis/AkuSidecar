// Synthetic private-pipe peer. No browser, network, profile or account access.
import {PassThrough} from 'node:stream';
import {performance} from 'node:perf_hooks';
import {serveMainBrowserTransport} from '../collection/headless/worker/native-pilot/main-browser-transport-protocol.mjs';
import {runInstagramBootstrapCorroboration} from '../collection/headless/worker/native-pilot/instagram-bootstrap-corroboration.mjs';
import {readInstagramBootstrapBodyStream} from '../collection/headless/worker/native-pilot/instagram-bootstrap-body-stream.mjs';
const scenario=process.argv[2],input=new PassThrough(),output=new PassThrough();
let bytes='';output.on('data',chunk=>{bytes+=chunk;let at;while((at=bytes.indexOf('\n'))>=0){const q=JSON.parse(bytes.slice(0,at));bytes=bytes.slice(at+1);input.write(JSON.stringify({id:q.id,error:{message:'PRIVATE_MARKER'}})+'\n');}});
const options={input:process.stdin,output:process.stdout,connect:async()=>({input,output}),onClosed:()=>process.exit(0)};
if(scenario==='scope')options.diagnoseInstagramBootstrapCorroboration=runInstagramBootstrapCorroboration;
if(scenario==='throw')options.diagnoseInstagramBootstrapCorroboration=()=>{throw Object.assign(new Error('PRIVATE_MARKER'),{diagnostic:{body:'PRIVATE_MARKER'}});};
if(scenario==='document_handoff')options.diagnoseInstagramBootstrapCorroboration=(_transport,_request,{trace})=>{
 trace.note('handoff');trace.fail('handoff','document_within_navigation');trace.cleanup('verified');
 return {schema:'aku.instagram-bootstrap-corroboration.v1',reason:'document_changed',bodyBound:true,documentBound:false,serverFresh:false,idMatched:true,cookieMatched:true,scanComplete:true,qualified:false,cleanup:'verified'};
};
if(scenario==='elapsed_unknown')options.diagnoseInstagramBootstrapCorroboration=(_transport,_request,{trace})=>{
 trace.note('body_read');
 Object.defineProperty(performance,'now',{configurable:true,value:()=>NaN});
 trace.note('cleanup');trace.cleanup('verified');
 return {schema:'aku.instagram-bootstrap-corroboration.v1',reason:'wire_unmatched',bodyBound:false,documentBound:false,serverFresh:false,idMatched:false,cookieMatched:false,scanComplete:false,qualified:false,cleanup:'verified'};
};
if(['trace_saturation','trace_saturation_eof'].includes(scenario))options.diagnoseInstagramBootstrapCorroboration=(_transport,_request,{trace})=>{
 trace.note('root_delivery');trace.fail('root_delivery','root_request_duplicate');trace.cleanup('pending');
 for(let i=0;i<10020;i++)trace.note('cleanup');
 if(scenario==='trace_saturation_eof'){process.stdout.write('',()=>process.exit(0));return new Promise(()=>{});}
 trace.cleanup('verified');
 return {schema:'aku.instagram-bootstrap-corroboration.v1',reason:'wire_unmatched',bodyBound:false,documentBound:false,serverFresh:false,idMatched:false,cookieMatched:false,scanComplete:false,qualified:false,cleanup:'verified'};
};
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
