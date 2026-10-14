import test from "node:test";
import assert from "node:assert/strict";
import { developmentReaderPriority } from "../internal/httpapi/web/development-reader-priority.js";
import { nativePostWaitReason } from "../internal/httpapi/web/native-post-availability.js";
import { createNativePostRouter } from "../internal/httpapi/web/native-post-routing.js";
const verified = {developmentSession:true,available:true,requested:"headless",effective:"headless",state:"ready",pending:false,generation:1,supportedSources:["x","instagram"],activeLeases:1};
test("only verified ready development source bypasses the active headless wait",async()=>{
 assert.equal(nativePostWaitReason(verified,{status:"collecting"},false,"x"),"");
 assert.equal(nativePostWaitReason(verified,{status:"collecting"},true,"x"),"Membuka native post…");
 let calls=0;
 const route=createNativePostRouter({readRuntime:async()=>verified,openHeadless:()=>calls++,openForeground:()=>assert.fail("browser route")});
 await route({source:"x"});assert.equal(calls,1);
});
test("missing/default/nondev/browser/pending/unsupported contract never gains priority",async()=>{
 const invalid=[undefined,{...verified,developmentSession:undefined},{...verified,developmentSession:false},{...verified,developmentSession:"true"},{...verified,available:false},{...verified,effective:"browser"},{...verified,requested:"browser"},{...verified,pending:true},{...verified,pending:undefined},{...verified,state:"starting"},{...verified,generation:0},{...verified,supportedSources:[]},{...verified,batchPaused:true},{...verified,collectionBorrowSource:"facebook"}];
 for(const runtime of invalid){
  assert.equal(developmentReaderPriority(runtime,"x"),false);
  if(runtime){assert.match(nativePostWaitReason(runtime,{status:"collecting"},false,"x"),/Menunggu/);
   const route=createNativePostRouter({readRuntime:async()=>runtime,openHeadless:()=>assert.fail("headless dispatched"),openForeground:()=>assert.fail("browser dispatched")});
   await assert.rejects(route({source:"x"}));}
 }
 for(const source of [undefined,"facebook","linkedin","unknown"]){
  assert.equal(developmentReaderPriority(verified,source),false);
  assert.match(nativePostWaitReason(verified,{status:"collecting"},false,source),/Menunggu/);
 }
});
