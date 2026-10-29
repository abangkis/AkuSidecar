import test from 'node:test';
import assert from 'node:assert/strict';
import {sourceTransientClass} from './source-retry-policy.mjs';
import {instagramIdentityFailure} from './instagram-native-session.mjs';
import {nativeSourceFailure} from './native-source-kernel.mjs';
import {manualCaptureFailure,manualCaptureRPCCode} from '../../../../../scripts/auth-session-fixture/x-native-manual-capture.mjs';

test('operational transient brands cannot be supplied by error text or diagnostic objects',()=>{
  for(const fake of [Error('network_failed'),{stage:'navigation',reason:'deadline'},{retryable:true,code:'manual_capture_transient_network'}])assert.equal(sourceTransientClass(fake),'');
  assert.equal(sourceTransientClass(instagramIdentityFailure('reader','fetch_timeout')),'timeout');
  for(const reason of ['fetch_failed','http_server','http_forbidden','login_visible','account_format'])assert.equal(sourceTransientClass(instagramIdentityFailure('reader',reason)),'');
  assert.equal(sourceTransientClass(nativeSourceFailure('capture','deadline')),'timeout');
  assert.equal(sourceTransientClass(nativeSourceFailure('ownership','deadline')),'');
});

test('X sends a separate operational retry code only for branded transport failures',()=>{
  for(const [reason,kind] of [['deadline','timeout'],['network_failed','network']]){
    const error=manualCaptureFailure(Error('PRIVATE_NATIVE_ERROR'),'navigation',reason);
    assert.equal(sourceTransientClass(error),kind);assert.equal(manualCaptureRPCCode(error),'manual_capture_transient_'+kind);
  }
  for(const [stage,reason] of [['owner_before','deadline'],['worker','login_required'],['observer','header_guard'],['cleanup','close_failed']]){
    const error=manualCaptureFailure(null,stage,reason);
    assert.equal(sourceTransientClass(error),'');assert.equal(manualCaptureRPCCode(error),'manual_capture_rejected');
  }
  const challenge=manualCaptureFailure({code:'challenge_required',challengeCategory:'cloudflare'},'navigation','deadline');
  assert.equal(sourceTransientClass(challenge),'');assert.equal(manualCaptureRPCCode(challenge),'manual_capture_challenge_cloudflare');
});
