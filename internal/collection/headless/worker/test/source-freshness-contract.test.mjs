import test from 'node:test';
import assert from 'node:assert/strict';
import {readFile} from 'node:fs/promises';
import {sourceFreshnessContractFor} from '../source-freshness-contract.mjs';

const sources=['x','linkedin','instagram','facebook'];
const assets=await Promise.all(['capture-primitives.js','source-adapter-runtime.js',...sources.map(source=>`adapters/${source}-adapter.js`)].map(async name=>({
  relative:'AkuBridge/'+name,content:await readFile(new URL('../../../../../../AkuBridge/'+name,import.meta.url),'utf8')})));

test('X source contract uses actual adapter route and primary identity rules',()=>{
  const contract=sourceFreshnessContractFor(assets,'x');
  assert.equal(contract.enabled,true);
  assert.equal(contract.matchesFeedURL('https://x.com/home?tab=following#feed'),true);
  for(const url of ['https://x.com/login','https://x.com/a/status/123','https://x.com:444/home','https://user@x.com/home','https://evil.test/home']){
    assert.equal(contract.matchesFeedURL(url),false,url);
  }
  for(const id of ['12345','x:status:12345']){
    const own=contract.primaryIdentity({id,permalink:'https://x.com/author/status/12345/photo/1?foo=1',quotedPost:{id:'99999'}});
    assert.equal(own.id,'12345');assert.equal(own.permalink,'https://x.com/author/status/12345');
  }
  for(const post of [{id:'123',permalink:'https://x.com/a/status/456'},
    {id:'x:status:123',permalink:'https://evil.test/a/status/123'},
    {id:'',permalink:'https://x.com/a/status/123',quotedPost:{id:'123'}},
    {id:123,permalink:'https://x.com/a/status/123'}])assert.equal(contract.primaryIdentity(post),null);
});

test('LinkedIn source contract requires its canonical feed route',()=>{
  const contract=sourceFreshnessContractFor(assets,'linkedin');
  assert.equal(contract.enabled,true);
  assert.equal(contract.version,'linkedin-headless-freshness-v1');
  assert.equal(contract.matchesFeedURL('https://www.linkedin.com/feed/'),true);
  assert.equal(contract.matchesFeedURL('https://www.linkedin.com/feed/?trk=feed'),true);
  assert.equal(contract.matchesFeedURL('https://www.linkedin.com/feed/#top'),true);
  for(const url of [
    'https://www.linkedin.com/feed',
    'http://www.linkedin.com/feed/',
    'https://www.linkedin.com/login',
    'https://www.linkedin.com/in/person/',
    'https://www.linkedin.com/feed/update/urn:li:activity:123456789/',
    'https://www.linkedin.com/feed/../feed/',
    'https://linkedin.com/feed/',
    'https://evil.test/feed/',
    'https://www.linkedin.com.evil.test/feed/',
    'https://www.linkedin.com@evil.test/feed/',
    'https://user@www.linkedin.com/feed/',
    'https://www.linkedin.com:444/feed/',
    'https://www.linkedin.com:443/feed/',
    'http://www.linkedin.com:443/feed/',
    'https://www.linkedin.com\\@evil.test/feed/',
    '',
    42,
  ]) assert.equal(contract.matchesFeedURL(url),false,String(url));
});

test('LinkedIn source contract binds exact adapter IDs to activity, ugcPost and share permalinks',()=>{
  const contract=sourceFreshnessContractFor(assets,'linkedin');
  for(const [kind,urnKind] of [['activity','activity'],['ugcpost','ugcPost'],['share','share']]){
    const id=`linkedin:${kind}:7412345678901234567`;
    const permalink=`https://www.linkedin.com/feed/update/urn:li:${urnKind}:7412345678901234567/`;
    const identity=contract.primaryIdentity({id,permalink,text:'primary'});
    assert.equal(identity?.id,id);
    assert.equal(identity?.permalink,permalink);
  }
  for(const post of [
    {id:'linkedin:activity:7412345678901234567',permalink:'https://www.linkedin.com/feed/update/urn:li:share:7412345678901234567/'},
    {id:'linkedin:activity:7412345678901234567',permalink:'https://www.linkedin.com/feed/update/urn:li:activity:7412345678901234568/'},
    {id:'linkedin:ugcpost:7412345678901234567',permalink:'https://www.linkedin.com/feed/update/urn:li:ugcpost:7412345678901234567/'},
    {id:'linkedin:organization:7412345678901234567',permalink:'https://www.linkedin.com/feed/update/urn:li:activity:7412345678901234567/'},
    {id:'linkedin:activity:1234',permalink:'https://www.linkedin.com/feed/update/urn:li:activity:1234/'},
    {id:'linkedin:activity:7412345678901234567',permalink:'https://linkedin.com/feed/update/urn:li:activity:7412345678901234567/'},
    {id:'linkedin:activity:7412345678901234567',permalink:'https://user@www.linkedin.com/feed/update/urn:li:activity:7412345678901234567/'},
    {id:'linkedin:activity:7412345678901234567',permalink:'https://www.linkedin.com:444/feed/update/urn:li:activity:7412345678901234567/'},
    {id:'linkedin:activity:7412345678901234567',permalink:'https://www.linkedin.com/feed/update/urn:li:activity:7412345678901234567/?trk=feed'},
    {id:'linkedin:activity:7412345678901234567',permalink:'https://www.linkedin.com/feed/update/urn:li:activity:7412345678901234567/#post'},
    {id:7412345678901234567,permalink:'https://www.linkedin.com/feed/update/urn:li:activity:7412345678901234567/'},
    {id:'',permalink:'https://www.linkedin.com/feed/update/urn:li:activity:7412345678901234567/',quotedPost:{id:'linkedin:activity:7412345678901234567'}},
  ]) assert.equal(contract.primaryIdentity(post),null,JSON.stringify(post));
});

test('Instagram and Facebook retain explicit disabled headless contracts',()=>{
  for(const source of sources.filter(source=>!['x','linkedin'].includes(source))){
    const contract=sourceFreshnessContractFor(assets,source);
    assert.equal(contract.source,source);assert.equal(contract.enabled,false);
    assert.equal(contract.matchesFeedURL,undefined);
  }
});

test('missing, malformed or unknown adapter capabilities never enable recovery',()=>{
  assert.equal(sourceFreshnessContractFor([], 'x').enabled,false);
  assert.equal(sourceFreshnessContractFor([], 'linkedin').enabled,false);
  assert.equal(sourceFreshnessContractFor(assets,'unknown').enabled,false);
  const broken=assets.map(asset=>asset.relative==='AkuBridge/adapters/x-adapter.js'?{...asset,content:'throw new Error("invalid adapter");'}:asset);
  assert.equal(sourceFreshnessContractFor(broken,'x').enabled,false);
  const brokenLinkedIn=assets.map(asset=>asset.relative==='AkuBridge/adapters/linkedin-adapter.js'?{...asset,content:'throw new Error("invalid adapter");'}:asset);
  assert.equal(sourceFreshnessContractFor(brokenLinkedIn,'linkedin').enabled,false);
  assert.equal(sourceFreshnessContractFor(assets,'x'),sourceFreshnessContractFor(assets,'x'));
  assert.equal(sourceFreshnessContractFor(assets,'linkedin'),sourceFreshnessContractFor(assets,'linkedin'));
});
