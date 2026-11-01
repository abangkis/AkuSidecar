// Ordinary traffic metadata is explanatory only. It cannot reject capture,
// revoke an owner, skip header inspection or supply account/session evidence.
export const CAPTURE_TRAFFIC_CONDITIONS=Object.freeze(['orphan_response','orphan_extra','extra_flag_missing','extra_flag_invalid','request_reused','redirect','response_pending','completion_pending','extra_pending','unexpected_extra','network_failed','record_capacity','event_capacity','metadata_shape']);
const data=(value,key)=>value&&typeof value==='object'?Object.getOwnPropertyDescriptor(value,key)?.value:undefined;
const ref=value=>typeof value==='string'&&value.length>0&&value.length<=512;
const MAX_RECORDS=256,MAX_EVENTS=16384,MAX_COUNT=65535;
export class CaptureTrafficDiagnostics {
  #records=new Map(); #conditions=new Set(); #events=0; #dropped=0; #finished=null;
  #note(value){this.#conditions.add(value);}
  observe(raw){
    if(this.#finished)return;
    const method=data(raw,'method');if(typeof method!=='string'||!method.startsWith('Network.'))return;
    if(this.#events>=MAX_EVENTS){this.#note('event_capacity');return;}this.#events++;
    if(!['Network.requestWillBeSent','Network.responseReceived','Network.responseReceivedExtraInfo','Network.loadingFinished','Network.loadingFailed'].includes(method))return;
    const params=data(raw,'params'),id=data(params,'requestId');
    if(!ref(id)){this.#note('metadata_shape');return;}
    let record=this.#records.get(id);
    if(!record){
      if(method==='Network.responseReceived')this.#note('orphan_response');
      if(method==='Network.responseReceivedExtraInfo')this.#note('orphan_extra');
      if(this.#records.size>=MAX_RECORDS){this.#note('record_capacity');this.#dropped=Math.min(MAX_COUNT,this.#dropped+1);return;}
      record={request:false,response:false,extra:false,finished:false,needed:null};this.#records.set(id,record);
    }
    if(method==='Network.requestWillBeSent'){
      if(data(params,'redirectResponse')!==undefined)this.#note('redirect');
      else if(record.request)this.#note('request_reused');
      record.request=true;
    }else if(method==='Network.responseReceived'){
      if(!record.request)this.#note('orphan_response');
      record.response=true;const flag=data(params,'hasExtraInfo');
      if(typeof flag==='boolean')record.needed=flag;
      else this.#note(flag===undefined?'extra_flag_missing':'extra_flag_invalid');
    }else if(method==='Network.responseReceivedExtraInfo'){
      if(!record.request)this.#note('orphan_extra');record.extra=true;
    }else{
      record.finished=true;if(method==='Network.loadingFailed')this.#note('network_failed');
    }
  }
  finish(){
    if(this.#finished)return this.#finished;
    for(const record of this.#records.values()){
      if(!record.response)this.#note('response_pending');
      if(!record.finished)this.#note('completion_pending');
      if(record.needed===true&&!record.extra)this.#note('extra_pending');
      if(record.needed===false&&record.extra)this.#note('unexpected_extra');
    }
    this.#finished=Object.freeze({schema:'aku.capture-traffic.v1',coverage:this.#conditions.size?'partial':'complete',conditions:Object.freeze(CAPTURE_TRAFFIC_CONDITIONS.filter(value=>this.#conditions.has(value))),records:this.#records.size,dropped:this.#dropped});
    this.#records.clear();return this.#finished;
  }
}
