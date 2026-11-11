const {test}=require('node:test');
const assert=require('node:assert/strict');
const fs=require('node:fs');
const vm=require('node:vm');
const path=require('node:path');
const dir=process.cwd();
const context={URL};vm.runInNewContext(fs.readFileSync(path.join(dir,'extension/product.js'),'utf8'),context);
const api=context.FairDealProduct;
function documentFor(json=[],meta={},imgs=[]){return {
 querySelectorAll(q){if(q==='script[type="application/ld+json"]')return json.map(v=>({textContent:typeof v==='string'?v:JSON.stringify(v)}));if(q==='img')return imgs;return [];},
 querySelector(q){const key=q.match(/meta\[property="([^"]+)"/);return key&&meta[key[1]]?{content:meta[key[1]]}:null;},
 defaultView:{getComputedStyle:()=>({display:'block',visibility:'visible',opacity:'1'})}
};}
const product={ '@type':'Product',name:'Winter Parka',sku:'WINTER-1',image:'https://shop.test/parka.jpg?width=1200',offers:{price:'149.99',priceCurrency:'USD'}};
const url='https://shop.test/products/parka?email=private@example.test&token=secret#tracking';
test('extracts selected product and excludes URL parameters and images from reference',()=>{
 const p=api.detect(documentFor([product]),url);assert.equal(p.name,'Winter Parka');assert.equal(p.price,'149.99');assert.equal(p.source,'https://shop.test/products/parka');assert.equal(p.images[0],'https://shop.test/parka.jpg');
 const ref=api.reference(p);assert.equal(Object.keys(ref).sort().join(','),'brand,currency,name,price,sku,source');assert.ok(!JSON.stringify(ref).includes('secret'));assert.ok(!JSON.stringify(ref).includes('budget'));
});
test('does not treat articles, malformed metadata or product listings as a selected product',()=>{
 assert.equal(api.detect(documentFor([],{ 'og:title':'An article','og:image':'https://shop.test/blog.jpg'}),url),null);
 assert.equal(api.detect(documentFor(['{broken']),url),null);
 assert.equal(api.detect(documentFor([{'@type':'ItemList',itemListElement:[product,{...product,sku:'SECOND',name:'Other product'}]}]),url),null);
 assert.equal(api.detect(documentFor([product,{...product,sku:'SECOND',name:'Other product'}]),url),null);
});
test('selects canonical product when recommendations are also present',()=>{
 const p=api.detect(documentFor([product,{...product,sku:'OTHER',name:'Actual chosen item',url:'https://shop.test/products/parka'}]),url);assert.equal(p.name,'Actual chosen item');
});
test('supports schema graph mainEntity and aggregate offers',()=>{
 const p=api.detect(documentFor([{'@graph':[{'@type':'WebPage',mainEntity:{...product,offers:{'@type':'AggregateOffer',lowPrice:'90',priceCurrency:'CLP'}}}]}]),url);assert.equal(p.price,'90');assert.equal(p.currency,'CLP');
});
test('requires explicit product metadata for OpenGraph fallback',()=>{
 const p=api.detect(documentFor([],{ 'og:type':'product','og:title':'Parka','product:price:amount':'120','product:price:currency':'usd'}),url);assert.equal(p.detection,'opengraph');assert.equal(p.currency,'USD');
});
test('only HTTP URLs without credentials are accepted',()=>{
 for(const u of ['javascript:alert(1)','file:///C:/private','https://user:pass@shop.test/p'])assert.equal(api.safeURL(u,url),'');assert.equal(api.detect(documentFor([product]),'file:///C:/private'),null);
});
function image(src,alt,w,h,hidden=false){return {src,currentSrc:src,alt,getBoundingClientRect:()=>({width:w,height:h}),closest:()=>hidden?{}:null,getAttribute:()=>null};}
test('matches product photo, not an unrelated large banner or hidden thumbnail',()=>{
 const hero=image('https://shop.test/parka.jpg?w=900','Winter Parka',500,600),banner=image('https://shop.test/banner.jpg','Summer sale',1000,700),hidden=image('https://shop.test/parka.jpg','Winter Parka',800,900,true);
 const doc=documentFor([product],{},[banner,hidden,hero]);const p=api.detect(doc,url);assert.equal(api.selectImage(doc,p,url),hero);
 assert.equal(api.selectImage(documentFor([product],{},[banner]),p,url),null);
});
test('background extracts only on authorized requests and opens only configured FairDeal destination',async()=>{
 const listeners={},opened=[];const c={URL,Set,Number,Error,serverURL:async()=> 'https://fairdeal-5h0w.onrender.com',chrome:{
 runtime:{id:'test-extension',getURL:p=>'chrome-extension://test-extension/'+p,onMessage:{addListener:f=>listeners.message=f},onInstalled:{addListener:f=>listeners.install=f}},
 contextMenus:{removeAll:f=>f(),create:()=>{},onClicked:{addListener:f=>listeners.context=f}},
 tabs:{sendMessage:async(id,msg)=>{assert.equal(id,7);assert.equal(msg.type,'FAIRDEAL_EXTRACT');return {...api.reference(api.detect(documentFor([product]),url)),source:url};},create:async value=>opened.push(value)},action:{setBadgeText:async()=>{}}
 }};
 vm.runInNewContext(fs.readFileSync(path.join(dir,'extension/background.js'),'utf8').replace(/^import[^\n]+\n/,''),c);
 listeners.message({type:'FAIRDEAL_OPEN_PRODUCT'},{id:'other',tab:{id:7},frameId:0},()=>assert.fail('external sender accepted'));
 listeners.message({type:'FAIRDEAL_OPEN_PRODUCT'},{id:'test-extension',tab:{id:7},frameId:2},()=>assert.fail('subframe accepted'));
 assert.equal(opened.length,0);
 await new Promise(resolve=>listeners.message({type:'FAIRDEAL_OPEN_PRODUCT'},{id:'test-extension',tab:{id:7},frameId:0},result=>{assert.equal(result.ok,true);resolve();}));
 assert.equal(opened.length,1);const target=new URL(opened[0].url);assert.equal(target.origin,'https://fairdeal-5h0w.onrender.com');const ref=JSON.parse(new URLSearchParams(target.hash.slice(1)).get('product'));assert.equal(ref.source,'https://shop.test/products/parka');assert.ok(!opened[0].url.includes('secret'));assert.equal(target.search,'');
});

test('never detects products on account, checkout or order pages',()=>{for(const route of ['checkout','cart','account','orders/123','payment'])assert.equal(api.detect(documentFor([product]),'https://shop.test/'+route),null);});

test('review metadata without an offer or product price is not an ecommerce offer',()=>{const review={...product};delete review.offers;assert.equal(api.detect(documentFor([review]),url),null);});
