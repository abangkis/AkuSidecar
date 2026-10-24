import test from 'node:test';
import assert from 'node:assert/strict';
import { captureBehaviorDiagnostics } from '../internal/httpapi/web/capture-behavior-status.js';

test('pacing diagnostics distinguish waiting, active, backoff and manual recovery without retry actions', () => {
  const entries = [
    { enabled: false, allowed: false, source: 'x', reason: 'capture_active' },
    { enabled: true, allowed: true, source: 'x' },
    { enabled: true, allowed: false, source: 'x', reason: 'capture_spacing', notBefore: '2026-10-25T08:00:00Z' },
    { enabled: true, allowed: false, source: 'facebook', reason: 'temporary_backoff', notBefore: '2026-10-25T08:01:00Z' },
    { enabled: true, allowed: false, source: 'x', reason: 'challenge_required' },
    { enabled: true, allowed: false, source: 'x', reason: 'capture_active' },
  ];
  assert.deepEqual(captureBehaviorDiagnostics(entries, { sourceLabel: value => value.toUpperCase(), formatMoment: value => value.slice(11, 16) }), [
    'X: capture spacing until 08:00', 'FACEBOOK: temporary backoff until 08:01', 'X: manual verification required', 'X: capture already active',
  ]);
});

test('unknown diagnostic reasons are not echoed and invalid timestamps do not become deadlines', () => {
  assert.deepEqual(captureBehaviorDiagnostics(null), []);
  assert.deepEqual(captureBehaviorDiagnostics([{ enabled: true, allowed: false, source: 'x', reason: 'unsafe diagnostic', notBefore: 'invalid' }]), ['x: capture admission waiting']);
});
