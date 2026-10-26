import {readFile} from 'node:fs/promises';
import {createCaptureStartGate} from './x-qa-capture-start-gate.mjs';

// Render only. This module cannot spawn a browser, inspect a profile or seed a session.
export async function renderXQAController({id,commit,sourceSHA256}) {
  if(!/^[a-f0-9]{32}$/.test(id??'')||!/^[a-f0-9]{40}$/.test(commit??'')||!/^[a-f0-9]{64}$/.test(sourceSHA256??''))
    throw new Error('QA controller pins invalid');
  let source=await readFile(new URL('./x-qa-run-once.mjs.template',import.meta.url),'utf8');
  for(const [token,value] of Object.entries({'@@PREPARED_ID@@':id,'@@COMMIT@@':commit,'@@SOURCE_SHA@@':sourceSHA256,
    '@@CAPTURE_START_GATE@@':createCaptureStartGate.toString()})){
    if(!source.includes(token))throw new Error('QA controller template invalid');
    source=source.replaceAll(token,()=>value);
  }
  if(/@@[A-Z_]+@@/u.test(source))throw new Error('QA controller template unresolved');
  return source;
}
