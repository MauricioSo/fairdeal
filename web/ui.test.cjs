const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');

function element(){return {innerHTML:'',children:[],textContent:'',style:{},hidden:true,appendChild(n){this.children.push(n);},remove(){this.removed=true;},scrollHeight:100};}
function merchantClass(){
 const document={getElementById:()=>null,createElement:()=>{const e=element();Object.defineProperty(e,'textContent',{set(v){e.innerHTML=String(v).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');}});return e;}};
 const context={document,console,Headers,fetch:async()=>({ok:true,json:async()=>({email:'qa@example.test'})}),sessionStorage:{removeItem(){}},window:{addEventListener(){},location:{replace(){}}}};
 let script=fs.readFileSync('web/static/js/merchant.js','utf8').replace(/^import .*;\r?\n/gm,'').replace('export async function','async function');
 vm.runInNewContext(script+'\nglobalThis.Dashboard = MerchantDashboard;',context);
 return context.Dashboard;
}

test('merchant copilot renders user markup as text, not executable HTML',()=>{
 const D=merchantClass(), dashboard=Object.create(D.prototype);dashboard.copilotMessages=element();
 const payload='<img src=x onerror="alert(1)"><svg onload=alert(2)> & text';
 const msg=dashboard.appendMessage('user',payload);
 assert.doesNotMatch(msg.innerHTML,/<img|<svg|onerror="/);
 assert.match(msg.innerHTML,/&lt;img/);assert.match(msg.innerHTML,/&amp; text/);
});

test('copilot preserves internally constructed proposal formatting',()=>{
 const D=merchantClass(), dashboard=Object.create(D.prototype);dashboard.copilotMessages=element();
 assert.match(dashboard.appendMessage('assistant','<strong>Reviewed draft</strong>').innerHTML,/<strong>Reviewed draft<\/strong>/);
});

test('copilot ignores a second submission while compilation is pending',async()=>{
 const D=merchantClass(), dashboard=Object.create(D.prototype);dashboard.copilotMessages=element();dashboard.copilotPending=true;
 dashboard.appendMessage=()=>assert.fail('duplicate submission rendered');
 await dashboard.sendCopilotMessage('another strategy');
});

const receiptHTML=fs.readFileSync('web/static/confirmed.html','utf8');
async function receipt(search,responses){
 const elements=new Map();
 for(const [,id,content] of receiptHTML.matchAll(/id="([^"]+)"[^>]*>([^<]*)/g)){const e=element();e.textContent=content;elements.set(id,e);}
 const panel=element();panel.hidden=true;elements.set('receipt-details',panel);
 let calls=0;
 const context={URLSearchParams,window:{location:{search}},document:{getElementById:id=>{assert.ok(elements.has(id),'missing fixture '+id);return elements.get(id);}},fetch:async()=>{calls++;return responses.shift();}};
 const script=receiptHTML.match(/<script>\s*([\s\S]*?)<\/script>/)[1].trim().replace(/;$/,'');
 await vm.runInNewContext(script,context);
 return {elements,calls};
}

test('receipt without an agreement never claims successful payment',async()=>{
 const {elements,calls}=await receipt('',[]);
 assert.equal(calls,0);assert.equal(elements.get('receipt-details').hidden,true);
 assert.equal(elements.get('status-icon').textContent,'…');
 assert.equal(elements.get('confirmed-title').textContent,'No Agreement Specified');
});

test('unavailable receipt never shows fabricated shipping, gift or payment success',async()=>{
 const {elements}=await receipt('?agreement=unknown',[{ok:false}]);
 assert.equal(elements.get('receipt-details').hidden,true);
 assert.equal(elements.get('confirmed-title').textContent,'Receipt Unavailable');
 assert.equal(elements.get('status-icon').textContent,'…');
});

test('uncaptured agreement keeps receipt details hidden',async()=>{
 const {elements}=await receipt('?agreement=offered',[{ok:true,json:async()=>({status:'OFFERED'})}]);
 assert.equal(elements.get('receipt-details').hidden,true);
 assert.equal(elements.get('confirmed-title').textContent,'Payment Not Captured');
});

test('verified mock capture displays the stored receipt and explicit simulation label',async()=>{
 const data={status:'CAPTURED',agreement_id:'qa-agreement',paypal_order_id:'qa-order',capture_id:'qa-capture',final_price:'96.00',currency:'USD',concessions:{payment_method:'PAYPAL',shipping_tier:'EXPRESS',shipping_days:'1-2',bundle:null}};
 const {elements}=await receipt('?agreement=qa-agreement',[{ok:true,json:async()=>data},{ok:true,json:async()=>({paypal_mode:'mock'})}]);
 assert.equal(elements.get('receipt-details').hidden,false);
 assert.equal(elements.get('status-icon').textContent,'✓');
 assert.equal(elements.get('confirmed-title').textContent,'Simulated Payment Completed');
 assert.equal(elements.get('receipt-amount').textContent,'$96.00 USD');
 assert.equal(elements.get('receipt-gift').textContent,'None');
});

function negotiatorClass(document) {
 const context={document,window:{addEventListener(){},scrollTo(){},innerWidth:390},clearInterval(){},clearTimeout(){},setTimeout(){return 1;}};
 const script=fs.readFileSync('web/static/js/negotiate.js','utf8').replace(/^import .*;\r?\n/gm,'').replace('export class','class');
 vm.runInNewContext(script+'\nglobalThis.App = NegotiatorApp;',context);return context.App;
}
test('constraint changes immediately remove stale mobile checkout and agreement',()=>{
 let hidden=false;
 const D=negotiatorClass({getElementById:()=>({classList:{remove(name){hidden=name==='visible';}}})});
 const app=Object.create(D.prototype);app.agreementCard={style:{display:'flex'}};app.lastMatchResponse={};
 app.scheduleConstraintsUpdate();
 assert.equal(hidden,true);assert.equal(app.agreementCard.style.display,'none');assert.equal(app.lastMatchResponse,null);
});
test('offer calculation ignores duplicate clicks while a request is pending',async()=>{
 const D=negotiatorClass({getElementById:()=>assert.fail('duplicate request reached the UI')});
 const app=Object.create(D.prototype);app.matchPending=true;await app.runWinWinMatch();
});

test('step navigation marks only the current stage visible at every breakpoint',()=>{
 const document={getElementById:()=>null};const D=negotiatorClass(document),app=Object.create(D.prototype);
 app.stage1Container={style:{}};app.gridStage2={style:{}};app.gridStage3={style:{}};
 for(const stage of [1,2,3,1]){
  app.setStage(stage);
  assert.equal(app.stage1Container.hidden,stage!==1);
  assert.equal(app.gridStage2.hidden,stage!==2);
  assert.equal(app.gridStage3.hidden,stage!==3);
 }
});
