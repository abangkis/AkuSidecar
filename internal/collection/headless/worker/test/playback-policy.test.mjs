import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {mkdir} from 'node:fs/promises';
import {randomUUID} from 'node:crypto';
import {resolve} from 'node:path';
import {launchChrome,CAPTURE_PLAYBACK_GUARD_SOURCE} from '../chrome.mjs';

test('capture pauses repeated media attempts without replacing playback or media sources',()=>{
  const listeners=new Map();
  class HTMLMediaElement {
    paused=false; currentSrc='https://example.test/media.mp4'; readyState=4; pauses=0;
    play(){this.paused=false;}
    pause(){this.paused=true;this.pauses++;}
  }
  const nativePlay=HTMLMediaElement.prototype.play;
  vm.runInNewContext(CAPTURE_PLAYBACK_GUARD_SOURCE,{HTMLMediaElement,document:{addEventListener(type,handler,capture){assert.equal(capture,true);listeners.set(type,handler);}}});
  const media=new HTMLMediaElement();
  listeners.get('play')({target:media});
  assert.equal(media.paused,true);
  media.play();listeners.get('playing')({target:media});
  assert.equal(media.paused,true);assert.equal(media.pauses,2);
  listeners.get('play')({target:{paused:false}});
  assert.equal(media.currentSrc,'https://example.test/media.mp4');assert.equal(media.readyState,4);
  assert.equal(HTMLMediaElement.prototype.play,nativePlay);
});

test('real headless Chrome retains metadata and pauses autoplay/retries across navigation',{
  skip:!process.env.AKU_TEST_CHROME_PATH,timeout:30000,
},async()=>{
  const profile=resolve('build','playback-policy-'+randomUUID(),'profile');
  await mkdir(profile,{recursive:true});
  const browser=await launchChrome({chromePath:process.env.AKU_TEST_CHROME_PATH,profilePath:profile});
  try {
    // A generated silent WAV tests real HTMLMediaElement behavior without an
    // external media download, social account, playback gesture or network.
    const dataSize=8000*2*2,wav=Buffer.alloc(44+dataSize);
    wav.write('RIFF');wav.writeUInt32LE(36+dataSize,4);wav.write('WAVEfmt ',8);
    wav.writeUInt32LE(16,16);wav.writeUInt16LE(1,20);wav.writeUInt16LE(1,22);
    wav.writeUInt32LE(8000,24);wav.writeUInt32LE(16000,28);wav.writeUInt16LE(2,32);wav.writeUInt16LE(16,34);
    wav.write('data',36);wav.writeUInt32LE(dataSize,40);
    const html=`<video id="media" muted autoplay loop preload="auto" src="data:audio/wav;base64,${wav.toString('base64')}"></video><output id="attempts">0</output><output id="error"></output><script>
      const media=document.getElementById('media'),count=document.getElementById('attempts');
      media.muted=true;media.addEventListener('play',()=>count.textContent=String(Number(count.textContent)+1));
      let tries=0;const retry=setInterval(()=>{media.play().catch(e=>document.getElementById('error').textContent=e.name);if(++tries===6)clearInterval(retry);},150);
    </script>`;
    const page=await browser.forSource('facebook');
    for(let documentIndex=0;documentIndex<2;documentIndex++){
      await page.navigate('data:text/html,'+encodeURIComponent(html));
      const deadline=Date.now()+8000;let state;
      do {
        state=await page.evaluate(`(() => {const m=document.getElementById('media');return m?{paused:m.paused,time:m.currentTime,readyState:m.readyState,duration:m.duration,attempts:Number(document.getElementById('attempts').textContent),error:document.getElementById('error').textContent,source:m.currentSrc.startsWith('data:audio/wav;')}:null;})()`);
        if(state?.attempts>=3&&state.readyState>=2)break;
        await new Promise(r=>setTimeout(r,100));
      }while(Date.now()<deadline);
      assert.ok(state?.attempts>=3,'fixture must actually attempt playback: '+JSON.stringify(state));
      assert.ok(state.readyState>=2,'loaded media data remains available');
      assert.equal(state.duration,2);assert.equal(state.source,true);
      assert.equal(state.paused,true);assert.ok(state.time<0.15,`playback advanced ${state.time}s`);
    }
  }finally{await browser.close();}
});
