import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import { sourceTextIsPartial } from '../internal/httpapi/web/source-text-completeness.js';

test('short feed excerpts remain explicitly partial independently of collapse thresholds', () => {
  assert.equal(sourceTextIsPartial({ textStatus: 'requires_permalink_capture' }), true);
  assert.equal(sourceTextIsPartial({ textStatus: 'expand_failed' }), true);
  assert.equal(sourceTextIsPartial({ limitations: ['text_may_be_collapsed'] }), true);
  assert.equal(sourceTextIsPartial({ textStatus: 'permalink_text_verified', limitations: ['text_truncated'] }), true);
  assert.equal(sourceTextIsPartial({ textStatus: 'permalink_text_verified', limitations: ['visible_dom_only'] }), false);
  assert.equal(sourceTextIsPartial({ textStatus: 'visible_text_no_collapse_control' }), false);
  assert.equal(sourceTextIsPartial(null), false);
});

test('source cards omit the incomplete-text notice and preserve expansion for longer stored text', () => {
  class Element {
    children = []; className = ''; textContent = '';
    classList = { toggle() {} }; style = { setProperty() {} };
    append(...children) { this.children.push(...children); }
    setAttribute() {} addEventListener() {}
  }
  const app = readFileSync(new URL('../internal/httpapi/web/app.js', import.meta.url), 'utf8');
  const card = app.slice(app.indexOf('function buildSourceCard('), app.indexOf('function applyPostFreshness('));
  const expander = app.slice(app.indexOf('function buildExpandableText('), app.indexOf('function buildMedia('));
  const context = vm.createContext({ document: { createElement: () => new Element() },
    sourceTextIsPartial, SOURCE_TEXT_COLLAPSE_CHARACTERS: 420, SOURCE_TEXT_COLLAPSE_LINES: 6,
    sourceDescriptor: () => ({}), sourceIdentity: () => ({ displayName: 'Theo' }),
    buildAvatar: () => new Element(), postHeaderContext: () => '', formatDate: () => '',
    applyPostFreshness() {}, postHeaderContexts: new WeakMap(), state: { expandedTimelineText: new Set() },
    buildQuotedPost: () => null, buildAttachments: () => null, safeSourceUrl: () => null,
    buildMedia: () => null, buildEngagement: () => null });
  vm.runInContext(card + expander, context);
  const find = (element, className) => [element, ...element.children.flatMap(child => find(child, className))]
    .filter(node => node.className === className);
  context.entry = { evidence: { text: 'line\n'.repeat(5) + 'a'.repeat(255), captureQuality: { textStatus: 'requires_permalink_capture' } } };
  const partial = vm.runInContext('buildSourceCard(entry)', context);
  assert.equal(find(partial, 'source-text-partial').length, 0);
  assert.equal(find(partial, 'content-expander').length, 0);
  context.entry = { evidence: { text: 'a'.repeat(600), captureQuality: { textStatus: 'permalink_text_verified' } } };
  const full = vm.runInContext('buildSourceCard(entry)', context);
  assert.equal(find(full, 'source-text-partial').length, 0);
  assert.equal(find(full, 'content-expander').length, 1);
});
