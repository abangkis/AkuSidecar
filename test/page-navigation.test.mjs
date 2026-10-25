import test from 'node:test';
import assert from 'node:assert/strict';
import {PAGES,renderPageNavigation,renderSetupPage} from '../internal/httpapi/web/page-navigation.js';

function dom() {
 const nodes=new Map();let focused;
 const root={querySelector(selector){if(!nodes.has(selector)) {const classes=new Set(),attributes={};nodes.set(selector,{classes,attributes,classList:{add:c=>classes.add(c),toggle(c,on){on?classes.add(c):classes.delete(c);}},setAttribute:(k,v)=>attributes[k]=v,focus:()=>{focused=selector;}});}return nodes.get(selector);}};
 return {root,nodes,focused:()=>focused};
}
test('every page has exactly one visible panel and matching accessible navigation',()=>{
 const d=dom();
 for(const selected of PAGES) {
  renderPageNavigation(d.root,selected.name);
  for(const page of PAGES){assert.equal(d.nodes.get(page.panel).classes.has('hidden'),page!==selected);assert.equal(d.nodes.get(page.button).attributes['aria-current'],page===selected?'page':'false');}
  assert.equal(d.focused(),selected.heading);
 }
});
test('loading preserves the timeline placeholder without focus; invalid page recovers',()=>{
 const d=dom();renderPageNavigation(d.root,'settings',{loading:true});
 assert.equal(d.focused(),undefined);assert.equal(d.nodes.get('.view-switch').classes.has('hidden'),true);
 assert.equal(d.nodes.get('#timeline-panel').classes.has('hidden'),false);
 assert.equal(renderPageNavigation(d.root,'unknown'),'timeline');assert.equal(d.focused(),'#timeline-heading');
});
test('onboarding and calibration hide Living Topics as well as every ordinary page',()=>{
 const d=dom();
 for(const setup of ['onboarding','calibration']){
  renderPageNavigation(d.root,'topics');renderSetupPage(d.root,setup);
  for(const page of PAGES){assert.equal(d.nodes.get(page.panel).classes.has('hidden'),true);assert.equal(d.nodes.get(page.button).attributes['aria-current'],'false');}
  assert.equal(d.nodes.get(`#${setup}-panel`).classes.has('hidden'),false);
 }
 assert.throws(()=>renderSetupPage(d.root,'unknown'));
});
