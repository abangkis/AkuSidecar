import test from 'node:test';
import assert from 'node:assert/strict';
import {cleanupDailyBatch} from './x-daily-batch-cleanup.mjs';
import {registerFreshStartupSession,startupRootControls,mustRetireStartupOwner} from './x-manual-root-controls.mjs';

test('failed control restoration attempts both commands and every independent batch cleanup stage',async()=>{
 const owner={owner:1,targetId:'target',sessionId:'session'},calls=[];
 registerFreshStartupSession(owner,'session','reader');
 const controls=startupRootControls(owner,async(method,params)=>{calls.push([method,params]);if(params.bypass===false)throw Error('PRIVATE');return{acknowledged:true};},'reader');
 await controls.apply();await assert.rejects(controls.restore());assert.equal(calls.at(-1)[1].cacheDisabled,false);assert(mustRetireStartupOwner(owner));
 const released=[];const receipt=await cleanupDailyBatch({readerControls:'uncertain',collector:async()=>{released.push('collector');throw Error('PRIVATE');},enrollment:async()=>released.push('enrollment'),attachments:async()=>{released.push('attachments');throw Error('PRIVATE');},bootstrap:async()=>released.push('bootstrap')});
 assert.deepEqual(released,['collector','enrollment','attachments','bootstrap']);assert.equal(receipt.closed,false);assert.equal(receipt.cleanup.bootstrap,'verified');assert.equal(JSON.stringify(receipt).includes('PRIVATE'),false);
});
test('verified cleanup succeeds and absent resources are explicit',async()=>{
 const receipt=await cleanupDailyBatch({bootstrap:async()=>{}});assert.equal(receipt.closed,true);assert.deepEqual(receipt.cleanup,{readerControls:'not_needed',collectorControls:'not_needed',collector:'not_needed',enrollment:'not_needed',attachments:'not_needed',bootstrap:'verified'});
});
