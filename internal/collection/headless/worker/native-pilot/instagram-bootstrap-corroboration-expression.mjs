import {INSTAGRAM_PRIVATE_BOOTSTRAP_SCANNER_SOURCE} from './instagram-self-bootstrap-probe.mjs';
import {INSTAGRAM_BOOTSTRAP_BUDGET} from './instagram-bootstrap-budget.mjs';
import {STRICT_JSON_PARSER_SOURCE} from '../../../../../scripts/auth-session-fixture/x-evidence-ingress.mjs';
import {INSTAGRAM_CORROBORATION_POLICY as policy} from './instagram-bootstrap-corroboration-policy.mjs';

// Body/ID/cookie values stay on the private CDP channel. The diagnostic returns
// only equality and closed categories; trusted native composition can request
// the corroborated ID transiently to bind its existing HMAC proof.
export function instagramBootstrapCorroborationExpression(body,nonce,privateIdentity=false){
 if(!Buffer.isBuffer(body)||body.length>policy.maxBodyBytes||!/^[a-f0-9]{48}$/.test(nonce))throw Error('corroboration_expression_rejected');
 return `(async()=>{
 const out={nonce:${JSON.stringify(nonce)},reason:'bootstrap_rejected',scanComplete:false,idMatched:false,cookieMatched:false};
 const actual=document,home=()=>location.origin==='https://www.instagram.com'&&location.pathname==='/';
 const guard=()=>!home()?'page_scope':actual.querySelector('input[type="password"],form[action*="login"]')?'login_visible':actual.querySelector('form[action*="challenge"],iframe[src*="captcha"]')?'challenge':null;
 let bytes,html='',bodyID='',domID='',cookieRows,template;
 const stopped=guard();if(stopped)return {...out,reason:stopped};
 try{
  bytes=Uint8Array.from(atob('${body.toString('base64')}'),c=>c.charCodeAt(0));
  try{html=new TextDecoder('utf-8',{fatal:true}).decode(bytes);}catch{return {...out,reason:'body_encoding'};}
  // Template contents are inert: no script execution or subresource loading,
  // and HTML parsing follows browser script/raw-text/comment rules.
  template=actual.createElement('template');template.innerHTML=html;html='';bytes.fill(0);
  const parseJSON=${STRICT_JSON_PARSER_SOURCE};
  const inspect=(document,onUnique)=>(${INSTAGRAM_PRIVATE_BOOTSTRAP_SCANNER_SOURCE})(${JSON.stringify(INSTAGRAM_BOOTSTRAP_BUDGET)},parseJSON,onUnique);
  const delivered=inspect(template.content,id=>{bodyID=id;});
  const live=inspect(actual,id=>{domID=id;});
  out.scanComplete=delivered.scanComplete===true&&live.scanComplete===true;
  if(delivered.reason!=='candidate'||live.reason!=='candidate')return out;
  out.idMatched=bodyID===domID;if(!out.idMatched)return {...out,reason:'id_mismatch'};
  if(typeof cookieStore==='undefined'||typeof cookieStore.getAll!=='function')return {...out,reason:'cookie_unsupported'};
  try{cookieRows=await cookieStore.getAll({name:'ds_user_id'});}catch{return {...out,reason:'cookie_failed'};}
  const after=guard();if(after)return {...out,reason:after};
  if(!Array.isArray(cookieRows))return {...out,reason:'cookie_invalid'};
  if(cookieRows.length===0)return {...out,reason:'cookie_missing'};
  if(cookieRows.length!==1)return {...out,reason:'cookie_ambiguous'};
  const cookie=cookieRows[0];
  if(cookie?.name!=='ds_user_id'||typeof cookie.value!=='string'||!/^[1-9][0-9]{0,29}$/.test(cookie.value)||
   cookie.domain!==undefined&&cookie.domain!==null&&!['instagram.com','.instagram.com','www.instagram.com'].includes(cookie.domain)||
   cookie.path!==undefined&&cookie.path!=='/'||cookie.secure!==undefined&&cookie.secure!==true||cookie.partitioned!==undefined&&cookie.partitioned!==false)return {...out,reason:'cookie_invalid'};
  // The asynchronous cookie API may yield to page code. Recheck the candidate
  // after it resolves without retaining or exporting its private value.
  domID='';const confirmed=inspect(actual,id=>{domID=id;});
  out.scanComplete=out.scanComplete&&confirmed.scanComplete===true;
  if(confirmed.reason!=='candidate')return {...out,reason:'bootstrap_rejected',idMatched:false};
  if(domID!==bodyID)return {...out,reason:'id_mismatch',idMatched:false};
  out.cookieMatched=cookie.value===bodyID;
  return {...out,reason:out.cookieMatched?'corroborated':'cookie_mismatch',...(${privateIdentity === true}&&out.cookieMatched?{accountId:bodyID}:{})};
 }catch{return {...out,reason:'body_invalid'};}
 finally{bytes?.fill(0);html='';bodyID='';domID='';cookieRows=undefined;if(template)template.innerHTML='';}
 })()`;
}
