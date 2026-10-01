import test from 'node:test';
import assert from 'node:assert/strict';
import { collectionModeState } from './collection-mode.js';
test('unmanaged browser keeps existing readiness',()=>{
 assert.equal(collectionModeState(null,true).canCollect,true);
 assert.equal(collectionModeState(null,false).canCollect,false);
 assert.equal(collectionModeState(null,true).canSelectHeadless,false);
});
test('requested headless is not active while draining',()=>{
 const view=collectionModeState({available:true,requested:'headless',effective:'browser',pending:true,state:'ready',headlessAvailable:true},true);
 assert.equal(view.canCollect,false);assert.match(view.detail,/Requested: headless. Active: browser/);
});
test('headless readiness uses worker and reports failure',()=>{
 assert.equal(collectionModeState({available:true,effective:'headless',state:'ready'},false).canCollect,true);
 const view=collectionModeState({available:true,effective:'',state:'failed',failure:'worker disconnected'},true);
 assert.equal(view.canCollect,false);assert.match(view.detail,/worker disconnected/);
});
