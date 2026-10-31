// Collector CDP queue. Late resource replies are retained only by the private
// IG composition; ordinary collection keeps its existing timeout behavior.
export function createChromeCommandSender({pending,write,isClosed,retainLateResources=false,timeoutMS=15000}){
  let sequence=0;
  return function send(method,params={},sessionId,timeoutMs=timeoutMS,late){
    if(isClosed())return Promise.reject(new Error('Chrome is closed.'));
    const id=++sequence;
    return new Promise((resolve,reject)=>{
      const entry={resolve,reject,method,late,expired:false};
      entry.timer=setTimeout(()=>{
        if(retainLateResources&&typeof late==='function')entry.expired=true;else pending.delete(id);
        reject(Object.assign(new Error(`CDP timeout: ${method}`),{code:'capture_timeout'}));
      },Math.max(1,Math.min(timeoutMS,Number.isFinite(timeoutMs)?timeoutMs:timeoutMS)));
      pending.set(id,entry);
      write(JSON.stringify({id,method,params,...(sessionId?{sessionId}:{})})+'\0',error=>{
        if(error&&pending.has(id)){pending.delete(id);clearTimeout(entry.timer);reject(error);}
      });
    });
  };
}

export function settleChromeCommandReply(pending,message){
  const call=pending.get(message.id);if(!call)return;
  pending.delete(message.id);clearTimeout(call.timer);
  if(call.expired){
    if(!message.error&&message.result&&typeof message.result==='object'&&!Array.isArray(message.result))Promise.resolve().then(()=>call.late(message.result)).catch(()=>{});
    return;
  }
  if(message.error)call.reject(new Error(`CDP ${call.method}: ${message.error.message||'command failed'}`));
  else call.resolve(message.result||{});
}
