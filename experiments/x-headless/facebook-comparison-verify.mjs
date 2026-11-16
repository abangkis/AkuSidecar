// Reads immutable capture evidence. Does not launch Chrome or write results.
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {createHash} from 'node:crypto';
import {comparePosts,assessBinding} from './facebook-comparison.mjs';
const [capturePath,receiptPath]=process.argv.slice(2);
assert.ok(capturePath&&receiptPath,'Provide capture.json and lifecycle receipt paths.');
const raw=await readFile(resolve(capturePath),'utf8'),report=JSON.parse(raw);
const receipt=JSON.parse(await readFile(resolve(receiptPath),'utf8'));
assert.equal(report.schema,'aku.facebook-extractor-comparison.v1');
assert.equal(report.scope,'production_extractor_and_poc_on_same_headless_dom');
assert.equal(receipt.restored,true);assert.equal(receipt.afterRestore.health,'healthy');
assert.equal(receipt.bridgeAfterRestore.compatible,true);assert.equal(receipt.profileOwnersAfterCapture.length,0);
assert.equal(receipt.capture.durationMs,report.durationMs);
assert.ok(Date.parse(receipt.startedAt)<=Date.parse(report.startedAt));
assert.ok(Date.parse(receipt.completedAt)>=Date.parse(report.startedAt)+report.durationMs);
const results=[];
for(const c of report.cases){
  if(!c.poc){results.push({category:c.category,status:c.status});continue;}
  assert.ok(c.authenticated,'Signed-in UI evidence must be present for each live case.');
  const before=comparePosts(c.poc,c.productionBefore,c.id,c.baseline);
  const after=comparePosts(c.poc,c.productionAfter,c.id,c.baseline);
  const binding=assessBinding(c.poc.posts,c.id,c.baseline),p=binding.post;
  const contentMatches=c.poc.posts.filter(post=>post.author===c.baseline.author&&post.text===c.baseline.text);
  results.push({category:c.category,status:before.comparable?'paired':after.comparable?'paired_after_poc_hydration':'target_binding_unverified',before,after,
    sameAuthorTextObservations:contentMatches.length,
    unresolvedDifferentNativeIds:contentMatches.filter(post=>post.id!==c.id).length,
    foreignAuthorWithTargetId:c.poc.posts.filter(post=>post.id===c.id&&post.author!==c.baseline.author).length,
    nativeIdAliasStatus:contentMatches.some(post=>post.id!==c.id)?'unverified':'not_required',
    verifiedPoc:p?{textCharacters:p.text.length,baselineTextEqual:p.text===c.baseline.text,media:p.media.length,
      mediaKinds:p.media.map(m=>m.kind),expectedMedia:p.mediaExpected,timestampSource:p.timestampSource,
      timestampEstimated:p.timestampEstimated,structuredTimeBounded:p.timestampEvidence?.diagnostics?.bounded??null,
      permalinkChangedByHover:p.hoverEvidence?.permalinkChangedByHover??false}:null});
}
console.log(JSON.stringify({evidenceVerified:true,scope:report.scope,captureSha256:createHash('sha256').update(raw).digest('hex'),
  restoredReceiptVerified:true,liveCases:results.filter(c=>c.before).length,
  validPairs:results.filter(c=>c.before?.comparable).length,
  validPairsAfterPocHydration:results.filter(c=>c.after?.comparable).length,
  verifiedPocTargets:results.filter(c=>c.verifiedPoc).length,
  sameAuthorTextCases:results.filter(c=>c.sameAuthorTextObservations>0).length,
  casesWithUnresolvedDifferentNativeIds:results.filter(c=>c.unresolvedDifferentNativeIds>0).length,
  foreignAuthorWithTargetId:results.reduce((sum,c)=>sum+(c.foreignAuthorWithTargetId||0),0),
  ambiguousPocIdentities:results.filter(c=>c.before?.pocBinding.ambiguous).length,
  productionIdentityWithoutBaselineBinding:results.filter(c=>c.before?.productionBinding.identityMatches>0&&!c.before.targetMatchedProduction).length,
  results},null,2));
