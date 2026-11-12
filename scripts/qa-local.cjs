const fs=require('node:fs'), assert=require('node:assert/strict');
const base='http://localhost:8080';
if(process.env.FAIRDEAL_QA_CONFIRM_DISPOSABLE!=='1'||!process.env.FAIRDEAL_QA_ACCOUNTS_FILE)throw Error('Use a disposable local mock instance and set FAIRDEAL_QA_CONFIRM_DISPOSABLE=1 and FAIRDEAL_QA_ACCOUNTS_FILE.');
const accounts=JSON.parse(fs.readFileSync(process.env.FAIRDEAL_QA_ACCOUNTS_FILE,'utf8'));
let checks=0;
function ok(value, label){assert.ok(value,label);checks++;console.log('PASS '+label);}
function client(){const jar=new Map();let csrf='';return {
 async request(path,method='GET',body,extra={}) {
  const headers={Cookie:[...jar].map(([k,v])=>k+'='+v).join('; '),...extra};
  if(method!=='GET'){headers.Origin=extra.Origin||base;headers['Content-Type']='application/json';if(csrf)headers['X-CSRF-Token']=csrf;}
  const res=await fetch(base+path,{method,headers,body:body===undefined?undefined:JSON.stringify(body),redirect:'manual'});
  for(const c of res.headers.getSetCookie()){const pair=c.split(';')[0];const i=pair.indexOf('=');jar.set(pair.slice(0,i),pair.slice(i+1));}
  const raw=await res.text();let data;try{data=JSON.parse(raw);}catch{data=raw;}
  return {status:res.status,data,headers:res.headers};
 },
 async login(id){let r=await this.request('/api/merchant/auth/csrf');assert.equal(r.status,200);csrf=r.data.csrf_token;r=await this.request('/api/merchant/login','POST',accounts[id]);assert.equal(r.status,200,JSON.stringify(r.data));r=await this.request('/api/merchant/session');assert.equal(r.status,200);csrf=r.data.csrf_token;return r.data;}
};}
const privateKeys = /"(?:budget|budget_cents|buyer_surplus|seller_surplus|private_utility)"\s*:/;
(async()=>{
 const publicClient=client();
 for(const path of ['/healthz','/api/config','/api/products/CH-WINTER-PARKA-01','/'])ok((await publicClient.request(path)).status===200,'public '+path);
 for(const path of ['/api/merchant/catalog','/api/merchant/policy','/api/merchant/metrics','/api/merchant/agreements','/api/merchant/economics'])ok((await publicClient.request(path)).status===401,'unauthorized denied '+path);
 ok((await publicClient.request('/merchant')).status===303,'merchant page requires login');
 ok((await publicClient.request('/static/design-preview.html')).status===404,'private preview is not served');
 ok((await publicClient.request('/api/paypal/webhook','POST',{})).status===503,'unconfigured webhook fails closed');
 const config=(await publicClient.request('/api/config')).data;ok(config.paypal_mode==='mock'&&config.ai_mode==='heuristic'&&!config.demo_mode,'QA uses mock, heuristics and no reset');
 const merchants={};
 for(const id of Object.keys(accounts)){
  const c=client();merchants[id]=c;
  const session=await c.login(id);ok(session.email===accounts[id].email,'login isolated '+id);
  const catalog=await c.request('/api/merchant/catalog');ok(catalog.status===200&&catalog.data.products.length>=1,'catalog '+id);
  for(const path of ['/api/merchant/policy','/api/merchant/metrics','/api/merchant/agreements','/api/merchant/events','/api/merchant/economics']) {const r=await c.request(path);ok(r.status===200&&!privateKeys.test(JSON.stringify(r.data)),'private projection '+id+' '+path);}
  ok((await c.request('/api/merchant/demo/reset','POST',{})).status===403,'reset blocked '+id);
  const before=(await c.request('/api/merchant/agreements')).data.length;
  const sim=await c.request('/api/merchant/economics/simulate','POST',{sku:'CH-WINTER-PARKA-01',price_cents:0});
  ok(sim.status===200&&sim.data.binding===false&&sim.data.rows.length>0,'simulation '+id);
  ok((await c.request('/api/merchant/agreements')).data.length===before,'simulation creates no agreements '+id);
  const bad=await c.request('/api/merchant/policy/manual','POST',{min_margin_pct:0});ok(bad.status===400,'invalid policy blocked '+id);
  const missingCSRF=await fetch(base+'/api/merchant/policy/manual',{method:'POST',headers:{Origin:base,'Content-Type':'application/json'},body:'{}'});ok(missingCSRF.status===401||missingCSRF.status===403,'mutation without credentials denied '+id);
 }
 const alpine=merchants.alpine;
 const newProduct={name:'QA accessory',list_price_cents:1500,cost_cents:500,retail_value_cents:1500,holding_cost_cents:0,is_deadstock:true,stock:3};
 ok((await alpine.request('/api/merchant/catalog/QA-ACCESSORY','PUT',newProduct)).status===200,'create catalog item');
 ok(!(await merchants.nordic.request('/api/merchant/catalog')).data.products.some(p=>p.sku==='QA-ACCESSORY'),'catalog edit isolated');
 ok((await alpine.request('/api/merchant/catalog/QA-ACCESSORY','DELETE')).status===204,'archive catalog item');
 ok((await alpine.request('/api/merchant/catalog')).data.products.find(p=>p.sku==='QA-ACCESSORY').stock===0,'archive keeps item and clears availability');
 const buyer=client();
 const input={sku:'CH-WINTER-PARKA-01',budget:'130.00',shipping_flexibility:0.85,willing_to_bundle:true,prefers_pay_later:true,delivery_country:'US'};
 const match=await buyer.request('/api/fairdeal/match','POST',input);ok(match.status===200&&match.data.all_offers.length===5,'US matches five merchants');
 for(const o of match.data.all_offers)ok(Number(o.final_price)<=130&&o.clearing_token,'validated budget-bounded offer '+o.merchant_name);
 const chosen=match.data.all_offers.find(o=>o.merchant_name.startsWith('Alpine'));assert.ok(chosen);
 const stocks=(await alpine.request('/api/merchant/catalog')).data.products;
 const tampered=await buyer.request('/api/paypal/orders','POST',{agreement_id:chosen.agreement_id,clearing_token:chosen.clearing_token+'x'});ok(tampered.status>=400,'tampered contract rejected');
 const order=await buyer.request('/api/paypal/orders','POST',{agreement_id:chosen.agreement_id,clearing_token:chosen.clearing_token});ok(order.status===200||order.status===201,'create mock order');assert.ok(order.data.order_id);
 const endpoint='/api/paypal/orders/'+encodeURIComponent(order.data.order_id)+'/capture';
 const capture=await buyer.request(endpoint,'POST',{});ok(capture.status===200&&capture.data.status==='COMPLETED','capture mock payment');
 const again=await buyer.request(endpoint,'POST',{});ok(again.status===200&&again.data.capture_id===capture.data.capture_id,'capture retry is idempotent');
 const after=(await alpine.request('/api/merchant/catalog')).data.products;
 ok(after.find(p=>p.sku===input.sku).stock===stocks.find(p=>p.sku===input.sku).stock-1,'main stock deducted once');
 if(chosen.concessions.bundle){const sku=chosen.concessions.bundle.sku;ok(after.find(p=>p.sku===sku).stock===stocks.find(p=>p.sku===sku).stock-1,'accessory stock deducted once');}
 const receipt=await buyer.request('/api/agreements/'+chosen.agreement_id+'/public');ok(receipt.status===200&&receipt.data.status==='CAPTURED','captured receipt');
 const audit=await alpine.request('/api/merchant/agreements/'+chosen.agreement_id+'/economics');ok(audit.status===200&&!privateKeys.test(JSON.stringify(audit.data)),'economic audit without buyer budget');
 ok((await merchants.nordic.request('/api/merchant/agreements/'+chosen.agreement_id+'/economics')).status===404,'cross-merchant audit denied');
 const cl=await buyer.request('/api/fairdeal/match','POST',{...input,delivery_country:'CL'});ok(cl.status===200&&cl.data.all_offers.length===3,'CL matches only supported merchants');
 const express=await buyer.request('/api/fairdeal/match','POST',{...input,shipping_flexibility:0,willing_to_bundle:false,prefers_pay_later:false});ok(express.status===200&&express.data.all_offers.every(o=>o.concessions.shipping_tier==='EXPRESS'&&!o.concessions.bundle&&o.concessions.payment_method!=='PAY_LATER'),'buyer refusals respected');
 const invalid=await buyer.request('/api/fairdeal/match','POST',{...input,budget:'not-money'});ok(invalid.status===400,'invalid budget denied');
 const policy=(await alpine.request('/api/merchant/policy')).data.active.policy;
 const draft=await alpine.request('/api/merchant/policy/manual','POST',policy);ok(draft.status===201,'manual draft review');
 ok((await alpine.request('/api/merchant/policy/'+draft.data.draft.id+'/activate','POST',{})).status===200,'explicit policy activation');
 const stale=express.data.all_offers.find(o=>o.merchant_name.startsWith('Alpine'));
 if(stale)ok((await buyer.request('/api/paypal/orders','POST',{agreement_id:stale.agreement_id,clearing_token:stale.clearing_token})).status>=400,'changed policy invalidates pending quote');
 const compiled=await alpine.request('/api/merchant/policy/compile','POST',{text:'Keep minimum margin 25%, allow ground delivery and no more than 20% discount.'});ok(compiled.status===201&&compiled.data.source!=='llm','offline policy compilation');
 for(const [id,c] of Object.entries(merchants)){ok((await c.request('/api/merchant/logout','POST',{})).status===204,'logout '+id);ok((await c.request('/api/merchant/session')).status===401,'revoked session '+id);}
 console.log('TOTAL '+checks+' checks passed');
})().catch(e=>{console.error('FAIL',e.message);process.exitCode=1;});
