import {performance} from 'node:perf_hooks';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';
import {startupFailure} from './x-manual-startup-diagnostic.mjs';

// Convert the orchestration deadline once; acquisition cannot renew it.
export class ManualObservationDeadline {
  #now; #start; #last; #end; #role; #prepared=null; #acquired=null;
  constructor(deadlineMS, {now=()=>performance.now(),wallNow=()=>Date.now(),role='unknown'}={}) {
    const wall=wallNow(),start=now(),remaining=deadlineMS-wall;
    if(!Number.isFinite(start)||!Number.isSafeInteger(deadlineMS)||!Number.isFinite(wall)||remaining<=0||remaining>MANUAL_TIMING.attemptAuthorizationMaxMS)throw startupFailure(role,'prepare','deadline',false,undefined,{deadlineCondition:remaining<=0?'prepared_expired':!Number.isFinite(start)||!Number.isFinite(wall)?'clock_nonfinite':'unknown'});
    this.#now=now;this.#start=this.#last=start;this.#end=start+remaining;this.#role=role;
  }
  get expiresAt(){return this.#end;}
  prepared(){this.remaining();this.#prepared??=this.#last;}
  acquired(){this.remaining();this.#acquired??=this.#last;}
  snapshot(){const age=(a,b)=>a===null||b===null?-1:Math.min(60000,Math.max(0,Math.floor(a-b)));return{observationAgeMS:age(this.#last,this.#start),prepareAgeMS:age(this.#prepared??this.#last,this.#start),pausedAgeMS:this.#prepared===null?-1:age(this.#acquired??this.#last,this.#prepared),acquireAgeMS:age(this.#acquired===null?null:this.#last,this.#acquired)};}
  remaining(condition='prepared_expired'){
    const t=this.#now();
    if(!Number.isFinite(t)||t<this.#last)throw startupFailure(this.#role,'bound_finish','time_invalid',false,undefined,{...this.snapshot(),deadlineCondition:!Number.isFinite(t)?'clock_nonfinite':'clock_regressed'});
    this.#last=t;
    if(t>=this.#end)throw startupFailure(this.#role,'bound_finish','deadline',false,undefined,{...this.snapshot(),deadlineCondition:condition});
    return Math.max(1,Math.floor(this.#end-t));
  }
  age(){return Math.min(MANUAL_TIMING.attemptAuthorizationMaxMS,Math.max(0,this.#last-this.#start));}
}
