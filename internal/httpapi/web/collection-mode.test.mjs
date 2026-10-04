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

test('Facebook collection hold reports automatic return rather than asking to close its source',()=>{
 const runtime={available:true,requested:'headless',effective:'browser',pending:true,state:'ready',headlessAvailable:true,collectionBorrowSource:'facebook'};
 const view=collectionModeState(runtime,true);
 assert.equal(view.canCollect,false);
 assert.match(view.detail,/Facebook is collecting through Browser/);
 assert.match(view.detail,/returns to your selected mode/);
 assert.doesNotMatch(view.detail,/open source windows/);
 const failed=collectionModeState({...runtime,state:'blocked',failure:'profile release unverified'},true);
 assert.equal(failed.canCollect,false);
 assert.match(failed.detail,/unavailable.*profile release unverified/);
});
test('unconfirmed cleanup blocks collection and exposes the reason instead of claiming capture is running',()=>{
 const view=collectionModeState({available:true,requested:'headless',effective:'browser',state:'ready',pending:true,
   collectionBorrowSource:'facebook',collectionBorrowFailure:'cleanup acknowledgement timed out'},true);
 assert.equal(view.canCollect,false);
 assert.match(view.detail,/cleanup is unconfirmed: cleanup acknowledgement timed out/);
 assert.match(view.detail,/Collection remains paused/);
 assert.doesNotMatch(view.detail,/Facebook is collecting/);
});
