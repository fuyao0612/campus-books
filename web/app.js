import {api,state,logout} from '/api.js';

// 页面只处理展示与交互，数据操作统一交给 api.js。
const $=id=>document.getElementById(id);
const esc=value=>String(value??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const money=cents=>(Number(cents)/100).toFixed(2);
const date=value=>value?new Date(value).toLocaleString('zh-CN',{hour12:false}):'刚刚发布';
const names={on_sale:'在售',off_shelf:'已下架',sold:'已售出',pending:'待支付',paid:'待交接',completed:'已完成',cancelled:'已取消'};
let renderId=0, toastTimer, pendingLogin=null;
let filter={keyword:'',category:'全部',condition:'全部',sort:'new',page:1};
let orderTab='all',orderRole='buy',mineTab='all';
const app=$('app');
function notify(text,error=false){clearTimeout(toastTimer);$('toast').textContent=text;$('toast').className=error?'error-toast':'';$('toast').hidden=false;toastTimer=setTimeout(()=>$('toast').hidden=true,3500);}
function nav(path){if(location.hash==='#'+path)render();else location.hash=path;}
function status(value){return `<span class="status ${esc(value)}">${esc(names[value]||value)}</span>`;}
function price(value){return `<span class="price"><small>¥</small>${money(value)}</span>`;}
function cover(book,extra=''){const theme=((Number(book.cover??book.id)||0)%7+7)%7;return `<div class="cover-area theme${theme} ${extra}" aria-hidden="true"><div class="book-cover"><strong>${esc(book.title)}</strong><small>CAMPUS · BOOKS<br>把知识传递下去</small></div></div>`;}
function empty(title,text,link='/',label='去教材广场'){return `<div class="empty"><span class="empty-symbol">▤</span><h3>${esc(title)}</h3><p>${esc(text)}</p>${link?`<a class="btn secondary" href="#${link}">${esc(label)} →</a>`:''}</div>`;}
function heading(title,text){return `<div class="page-heading"><h1>${esc(title)}</h1><p class="muted small">${esc(text)}</p></div>`;}
function breadcrumb(text){return `<div class="breadcrumb"><a href="#/">教材广场</a> / ${esc(text)}</div>`;}
function showLogin(after=null){pendingLogin=after;$('login-error').textContent='';$('login-caption').textContent='使用校园书集账号登录，登录状态将在当前浏览器标签页保留。';if(!$('login-dialog').open)$('login-dialog').showModal();}
function requireLogin(){if(state.user)return true;showLogin(()=>render());return false;}
function refreshShell(){
  $('mode-note').textContent='教材、留言与订单均通过 Go API 读取和保存。';
  $('account-nav').innerHTML=state.user?`<a class="account-button" href="#/profile">${esc(state.user.name)} · 个人中心</a>`:'<button class="account-button" data-action="login">登录 / 开始使用</button>';
  const page=(location.hash||'#/').slice(1);
  document.querySelectorAll('[data-nav]').forEach(a=>a.classList.toggle('active',a.dataset.nav==='home'?page==='/'||page.startsWith('/book/'):page.startsWith('/'+a.dataset.nav)));
}
function card(book){return `<article class="book-card"><a href="#/book/${book.id}" aria-label="查看${esc(book.title)}">${cover(book)}<div class="book-info"><h3>${esc(book.title)}</h3><p>${esc(book.course)} · ${esc(book.condition)}</p><div class="book-card-bottom">${price(book.price)}<span class="seller">${esc(book.seller_name||'同学 #'+book.seller_id)}</span></div></div></a></article>`;}

async function home(){
  const books=await api.books(filter.keyword);
  let list=books.filter(b=>(filter.category==='全部'||(b.category||classify(b))===filter.category)&&(filter.condition==='全部'||b.condition===filter.condition));
  if(filter.sort==='low')list.sort((a,b)=>a.price-b.price);else if(filter.sort==='high')list.sort((a,b)=>b.price-a.price);else list.sort((a,b)=>b.id-a.id);
  const count=list.length,pages=Math.max(1,Math.ceil(count/8));filter.page=Math.min(filter.page,pages);
  const view=list.slice((filter.page-1)*8,filter.page*8);
  return `<section class="hero"><div><span class="overline">A NEW CHAPTER FOR EVERY BOOK</span><h1>让闲置的教材，<br>遇见<em>下一位主人。</em></h1><p>少一点闲置，多一份收获。<br>在熟悉的校园，找到你需要的那一本。</p><div class="hero-links"><a class="btn" href="#/publish">发布我的教材 ↗</a><a class="text-link" href="#/guide">了解交易流程 →</a></div></div><div class="hero-art" aria-hidden="true"><div class="art-circle"></div><div class="art-book one"><b>知识<br>再出发</b><small>READ & REUSE</small></div><div class="art-book two"><b>下一章<br>属于你</b><small>CAMPUS COLLECTION</small></div><div class="art-stamp">旧书有价 · 知识无价</div></div></section>
  <div class="benefits"><span><b>01</b> 校园同学 · 友善交流</span><span><b>02</b> 教材信息 · 如实描述</span><span><b>03</b> 在线下单 · 校内交接</span></div>
  <section><div class="section-top"><div><h2>发现好教材<span style="color:#a9b397;font-size:13px;margin-left:12px">/ BOOKS</span></h2><p>每一本被珍惜的书，都值得再读一遍。</p></div><form id="search-form" class="search"><input name="keyword" value="${esc(filter.keyword)}" placeholder="搜索教材名称…" aria-label="搜索教材"><button class="btn" type="submit">搜索</button></form></div>
  <div class="filters">${['全部','计算机','公共课','语言','设计','其他'].map(c=>`<button class="chip ${filter.category===c?'active':''}" data-action="category" data-value="${c}">${c}</button>`).join('')}<select id="condition-filter" class="sort" aria-label="筛选成色">${['全部','全新','九成新','八成新','七成新'].map(c=>`<option value="${c}" ${c===filter.condition?'selected':''}>${c==='全部'?'全部成色':c}</option>`).join('')}</select><select id="sort-filter" class="sort" aria-label="排序"><option value="new" ${filter.sort==='new'?'selected':''}>最新发布</option><option value="low" ${filter.sort==='low'?'selected':''}>价格从低到高</option><option value="high" ${filter.sort==='high'?'selected':''}>价格从高到低</option></select></div>
  <p class="filter-count">找到 ${count} 本在售教材${filter.keyword?' · 关键词“'+esc(filter.keyword)+'”':''}</p><div class="books-grid">${view.length?view.map(card).join(''):empty('暂时没有找到这本书','换一个关键词或分类再试试。',null)}</div>
  <div class="page-controls"><button data-action="page" data-value="${filter.page-1}" ${filter.page<=1?'disabled':''} aria-label="上一页">‹</button>${Array.from({length:pages},(_,i)=>`<button data-action="page" data-value="${i+1}" class="${filter.page===i+1?'active':''}">${i+1}</button>`).join('')}<button data-action="page" data-value="${filter.page+1}" ${filter.page>=pages?'disabled':''} aria-label="下一页">›</button></div></section>
  <section class="join-banner"><div><h3>书架上的闲置，也能成为别人的新开始。</h3><p>写下教材信息，把它交给下一位需要的同学。</p></div><a href="#/publish" class="btn secondary">发布一本书 →</a></section>`;
}
function classify(b){if(/英语|语言/.test(b.course+b.title))return '语言';if(/数学|代数/.test(b.course+b.title))return '公共课';if(/计算机|程序|数据|网络|Go/.test(b.course+b.title))return '计算机';return '其他';}

async function detail(id){
  const b=await api.detail(id);let comments=[],commentsError='';
  try{comments=await api.comments(id);}catch(error){commentsError=error.message;}
  const own=state.user?.id===b.seller_id;
  return `${breadcrumb(b.title)}<section class="panel detail-layout">${cover(b,'detail-cover')}<div class="detail-info">${status(b.status)}<h1>${esc(b.title)}</h1>${price(b.price)}<dl><dt>适用课程</dt><dd>${esc(b.course)}</dd><dt>教材成色</dt><dd>${esc(b.condition)}</dd><dt>发布同学</dt><dd>${esc(b.seller_name||'同学 #'+b.seller_id)}</dd><dt>联系方式</dt><dd>${esc(b.contact)}</dd><dt>发布时间</dt><dd>${date(b.created_at)}</dd></dl><div class="description">${esc(b.description||'发布同学暂时没有补充说明。')}</div><div class="button-row detail-actions">${own&&b.status==='on_sale'?`<a class="btn" href="#/edit/${b.id}">编辑教材</a><button class="btn danger" data-action="remove" data-id="${b.id}">下架教材</button>`:!own&&b.status==='on_sale'?`<button class="btn" data-action="buy" data-id="${b.id}">立即购买 →</button><a class="btn secondary" href="#/book/${b.id}" data-action="scroll-comments">留言问问</a>`:`<button class="btn secondary" disabled>${esc(names[b.status]||'暂不可购买')}</button>`}</div><p class="small muted">下单前请确认教材版本与成色，交接时当面检查。</p></div></section>
  <section class="panel comment-section" id="comments"><h2>教材留言 <span class="muted small">${comments.length} 条</span></h2>${commentsError?'<div class="notice-box">'+esc(commentsError)+' <button class="text-link" data-action="retry">重新加载留言</button></div>':comments.length?comments.map(c=>`<article class="comment-item"><div class="row"><strong>${esc(c.name||c.user_name||'校园用户')}</strong><time>${date(c.created_at)}</time></div><p>${esc(c.content)}</p>${state.user?.id===c.user_id?`<button class="text-link" data-action="delete-comment" data-id="${c.id}">删除留言</button>`:''}</article>`).join(''):'<p class="muted small">还没有留言，你可以问问教材版本或使用情况。</p>'}${b.status==='on_sale'?`<form id="comment-form" class="comment-form" data-id="${b.id}"><label>写下你的问题<textarea name="content" maxlength="500" rows="3" required placeholder="文明交流，请勿发布无关信息。"></textarea></label><button class="btn" type="submit">${state.user?'发布留言':'登录后留言'}</button></form>`:''}</section>`;
}

async function editor(id){
  if(!state.user)return heading(id?'编辑教材':'发布教材','把闲置教材交给下一位同学。')+empty('登录后再发布','请先登录，发布的教材会关联到你的账号。',null)+'<div class="button-row"><button class="btn" data-action="login">去登录 →</button></div>';
  const b=id?await api.detail(id):{title:'',course:'',condition:'九成新',price:'',contact:'',description:'',category:'计算机',cover:0};
  if(id&&(b.seller_id!==state.user.id||b.status!=='on_sale'))throw new Error('只能编辑自己在售的教材。');
  return `${breadcrumb(id?'编辑教材':'发布教材')}${heading(id?'让教材信息更准确。':'给教材，一个新的开始。',id?'更新教材内容，帮助同学了解实际情况。':'如实填写信息，让需要它的同学更容易找到你。')}<div class="page-columns"><section class="panel"><h2>${id?'编辑教材信息':'填写教材信息'}</h2><form id="book-form" data-id="${id||''}"><div class="form-grid"><label class="wide">教材名称<input name="title" maxlength="100" required placeholder="例如：Go 语言程序设计（第 2 版）" value="${esc(b.title)}"></label><label>适用课程<input name="course" maxlength="100" required value="${esc(b.course)}" placeholder="例如：程序设计"></label><label>教材分类<select name="category">${['计算机','公共课','语言','设计','其他'].map(c=>`<option ${c===(b.category||classify(b))?'selected':''}>${c}</option>`).join('')}</select></label><label>教材成色<select name="condition">${['全新','九成新','八成新','七成新'].map(c=>`<option ${c===b.condition?'selected':''}>${c}</option>`).join('')}</select></label><label>售价（元）<input name="price" required inputmode="decimal" placeholder="例如：25.00" value="${b.price?money(b.price):''}"></label><label class="wide">联系方式<input name="contact" required maxlength="100" value="${esc(b.contact)}" placeholder="微信、电话或校内交接方式"></label><label class="wide">补充说明<textarea name="description" rows="5" maxlength="1000" placeholder="版本、笔记、缺页和交接位置等信息">${esc(b.description)}</textarea></label></div><p class="small muted">教材封面以书名生成文字示意，图片上传可以在后续版本加入。</p><p id="form-error" class="error" role="alert"></p><div class="button-row"><button class="btn" type="submit">${id?'保存修改':'确认发布'} →</button><a class="btn secondary" href="#/mine">取消</a></div></form></section><aside class="panel"><div id="cover-preview">${cover({...b,title:b.title||'你的下一本好书'},'preview-cover')}</div><h3>发布小贴士</h3><ul class="hint-list"><li>写清书名和版次，避免买错。</li><li>如实描述笔记、破损和缺页。</li><li>价格以元填写，最多两位小数。</li><li>仅能修改、下架自己在售的教材。</li></ul></aside></div>`;
}

async function mine(){
  if(!state.user)return loginPage('我发布的教材');
  const all=await api.mine(mineTab==='all'?'':mineTab);const books=all;
  return heading('我发布的教材','整理书架，让每本教材都有自己的下一章。')+`<div class="tabs">${[['all','全部'],['on_sale','在售'],['off_shelf','已下架'],['sold','已售出']].map(([v,n])=>`<button class="chip ${mineTab===v?'active':''}" data-action="mine-tab" data-value="${v}">${n}</button>`).join('')}<a href="#/publish" class="text-link" style="margin-left:auto">＋ 发布教材</a></div>${books.length?books.map(b=>`<article class="management-card"><div class="management-row">${cover(b,'mini-cover')}<div class="management-main">${status(b.status)}<h3>${b.status==='on_sale'?`<a href="#/book/${b.id}">${esc(b.title)}</a>`:esc(b.title)}</h3><p>${esc(b.course)} · ${esc(b.condition)}</p>${price(b.price)}</div><div class="button-row">${b.status==='on_sale'?`<a href="#/edit/${b.id}" class="btn secondary">编辑</a><button class="btn danger" data-action="remove" data-id="${b.id}">下架</button>`:`<span class="small muted">${esc(names[b.status]||'')}</span>`}</div></div></article>`).join(''):empty('这里暂时没有教材','发布一本教材，让闲置的知识继续流转。','/publish','发布教材')}`;
}
function loginPage(title){return heading(title,'登录后查看与管理你的校园书集。')+empty('还没有登录','登录后可以发布教材、留言和查看订单。',null)+'<div class="button-row"><button class="btn" data-action="login">立即登录 →</button></div>';}

async function orders(){
  if(!state.user)return loginPage('我的订单');
  const list=await api.orders(orderRole,orderTab==='all'?'':orderTab);
  return heading('每一笔交易，都有记录。','支付后请与对方联系，协商校内交接。')+`<div class="tabs">${[['buy','我买到的'],['sell','我卖出的']].map(([v,n])=>`<button class="chip ${orderRole===v?'active':''}" data-action="order-role" data-value="${v}">${n}</button>`).join('')}</div><div class="filters">${[['all','全部'],['pending','待支付'],['paid','待交接'],['completed','已完成'],['cancelled','已取消']].map(([v,n])=>`<button class="chip ${orderTab===v?'active':''}" data-action="order-tab" data-value="${v}">${n}</button>`).join('')}</div>${list.length?list.map(o=>`<article class="order-card"><div class="order-header"><span>订单 ${esc(o.number)}</span><span>${date(o.created_at)}</span>${status(o.status)}</div><div class="management-row">${cover(o,'mini-cover')}<div class="management-main"><h3><a href="#/book/${o.book_id}">${esc(o.title)}</a></h3><p>${orderRole==='buy'?'卖家：'+esc(o.seller_name||'同学'):'买家编号：'+o.buyer_id}</p><p>交接联系：${esc(o.contact)}</p></div>${price(o.price)}</div><div class="order-footer"><span>共 1 本教材 · 合计 ¥${money(o.price)}</span><div class="button-row">${orderRole==='buy'&&o.status==='pending'?`<button class="text-link" data-action="cancel-order" data-id="${o.id}">取消订单</button><a href="#/pay/${o.id}" class="btn">去支付</a>`:orderRole==='buy'&&o.status==='paid'?`<button class="btn" data-action="complete-order" data-id="${o.id}">确认收到教材</button>`:`<span class="small muted">${o.status==='paid'?'等待买家确认交接':names[o.status]}</span>`}</div></div></article>`).join(''):empty('暂无相关订单','去教材广场看看，也许正好有你需要的那一本。')}`;
}

async function payment(id){
  if(!state.user)return loginPage('订单支付');
  const o=await api.getOrder(id);if(o.buyer_id!==state.user.id)throw new Error('不能支付他人的订单。');
  if(o.status!=='pending')return `<section class="panel payment-layout"><div class="payment-success"><div class="success-icon">${['paid','completed'].includes(o.status)?'✓':'–'}</div><h1>${esc(names[o.status])}</h1><p class="muted">${['paid','completed'].includes(o.status)?'支付已确认，请与卖家联系完成交接。':'此订单已取消，无需支付。'}</p><p class="small muted">订单号 ${esc(o.number)}</p><a class="btn" href="#/orders">查看我的订单 →</a></div></section>`;
  return `<div class="payment-layout">${breadcrumb('订单支付')}${heading('确认订单，继续学习。','核对教材与金额后，将通过支付服务完成付款。')}<section class="panel"><h2>订单信息</h2><div class="management-row">${cover(o,'mini-cover')}<div class="management-main"><h3>${esc(o.title)}</h3><p>卖家：${esc(o.seller_name||'校园用户')}</p><p>订单：${esc(o.number)}</p></div></div><div class="payment-summary"><span>应付金额</span>${price(o.price)}</div><div class="notice-box">点击支付后由服务端创建支付请求并返回支付页面地址。付款结果以服务端异步通知和订单状态为准。</div><div class="button-row"><button class="btn" data-action="pay" data-id="${o.id}">前往支付 ¥${money(o.price)}</button><a class="btn secondary" href="#/orders">稍后支付</a></div></section></div>`;
}

async function profile(){
  if(!state.user)return loginPage('个人中心');
  const user=await api.me();
  return `<section class="profile-banner panel"><span class="avatar">${esc(user.name.slice(0,1))}</span><div><h1>${esc(user.name)}，你好。</h1><p>用户编号 ${user.id} · 校园书集用户</p></div></section><div class="profile-grid"><a class="profile-tile" href="#/mine"><h3>我发布的 ↗</h3><p>修改、下架和查看教材状态。</p></a><a class="profile-tile" href="#/orders"><h3>我的订单 ↗</h3><p>查看购买记录与交接进度。</p></a><a class="profile-tile" href="#/publish"><h3>发布教材 ↗</h3><p>整理闲置，让知识继续流转。</p></a></div><section class="panel" style="margin-top:24px"><h2>账号与使用</h2><p class="muted small">登录凭证保存在当前浏览器标签页中，关闭标签后清除。退出后需要重新登录。</p><div class="button-row"><button class="btn secondary" data-action="logout">退出登录</button></div></section>`;
}
function guide(){return `<div class="guide">${heading('让每次转手，都简单一点。','校园书集 · 使用指南')}<section class="panel"><h2>01 找到你需要的教材</h2><p>通过书名搜索、分类、成色筛选和价格排序浏览教材。打开详情，确认课程、版本、成色与联系方式。封面为自动生成的文字示意，不代表教材实际外观。</p><h2>02 如实发布，友善交流</h2><p>登录后填写教材信息。价格单位为元，支持两位小数。发布人可以修改或下架自己在售的教材。在留言区确认版本与交接方式。</p><h2>03 下单、支付与交接</h2><p>确认后下单，在订单页完成支付。支付状态由服务端确认；联系卖家完成校内交接后，买家可确认收货。待支付订单可以取消。本项目不包含预约功能。</p><h2>账号与安全</h2><p>账号由校园书集服务端验证。教材修改、下架、留言和订单权限都必须由服务端校验。在线支付应使用支付平台的服务端通知确认结果，不能仅依赖浏览器回跳。</p><a class="btn secondary" href="#/">回到教材广场 →</a></section></div>`;}

async function render(){
  const version=++renderId;refreshShell();app.innerHTML='<div class="loading">正在整理书架…</div>';
  const parts=(location.hash.slice(1)||'/').split('/').filter(Boolean);const [page,id]=parts;
  try{let html;if(!page)html=await home();else if(page==='book'&&/^\d+$/.test(id))html=await detail(id);else if(page==='publish')html=await editor();else if(page==='edit'&&/^\d+$/.test(id))html=await editor(id);else if(page==='mine')html=await mine();else if(page==='orders')html=await orders();else if(page==='pay'&&/^\d+$/.test(id))html=await payment(id);else if(page==='profile')html=await profile();else if(page==='guide')html=guide();else html=empty('这一页还没找到','回到广场，继续发现好教材。');if(version!==renderId)return;app.innerHTML=html;refreshShell();document.title=(page?({book:'教材详情',publish:'发布教材',edit:'编辑教材',mine:'我发布的',orders:'我的订单',pay:'订单支付',profile:'个人中心',guide:'使用指南'}[page]||'页面'):'教材广场')+' · 校园书集';}
  catch(error){if(version!==renderId)return;app.innerHTML=empty('暂时无法显示',error.message,null)+'<div class="button-row"><button class="btn secondary" data-action="retry">重试</button><a href="#/" class="btn">回到广场</a></div>';refreshShell();}
}

function confirmAction(title,message){return new Promise(resolve=>{$('confirm-title').textContent=title;$('confirm-message').textContent=message;$('confirm-dialog').returnValue='';$('confirm-dialog').addEventListener('close',()=>resolve($('confirm-dialog').returnValue==='yes'),{once:true});$('confirm-dialog').showModal();});}
async function busy(button,action){if(button.disabled)return;button.disabled=true;try{await action();}catch(e){notify(e.message,true);refreshShell();}finally{button.disabled=false;}}

document.addEventListener('click',event=>{
  const close=event.target.closest('[data-close]');if(close){$(close.dataset.close).close();return;}
  const button=event.target.closest('[data-action]');if(!button)return;event.preventDefault();
  const {action,id,value}=button.dataset;
  if(action==='login'){showLogin(()=>render());return;}
  if(action==='category'){filter.category=value;filter.page=1;render();return;}
  if(action==='page'){filter.page=Number(value);render();return;}
  if(action==='mine-tab'){mineTab=value;render();return;}
  if(action==='order-tab'){orderTab=value;render();return;}
  if(action==='order-role'){orderRole=value;orderTab='all';render();return;}
  if(action==='scroll-comments'){$('comments').scrollIntoView({behavior:'smooth'});return;}
  if(action==='retry'){render();return;}
  busy(button,async()=>{
    if(action==='logout'){logout();notify('已退出登录。');nav('/');}
    if(action==='buy'){if(!requireLogin())return;const order=await api.order(id);nav('/pay/'+order.id);}
    if(action==='remove'){if(await confirmAction('下架这本教材？','下架后将不再出现在教材广场，请确认没有正在进行的交易。')){await api.remove(id);notify('教材已下架。');nav('/mine');}}
    if(action==='delete-comment'){if(await confirmAction('删除这条留言？','删除后无法恢复。')){await api.deleteComment(id);notify('留言已删除。');render();}}
    if(action==='cancel-order'){if(await confirmAction('取消订单？','取消后不会扣款，可以继续浏览其他教材。')){await api.cancel(id);notify('订单已取消。');render();}}
    if(action==='complete-order'){if(await confirmAction('确认收到教材？','请在检查教材并完成当面交接后确认。')){await api.complete(id);notify('交易已完成，祝学习顺利。');render();}}
    if(action==='pay'){const result=await api.pay(id);const url=result?.payment_url||result?.checkout_url;if(!url)throw new Error('支付接口未返回 payment_url，请检查服务端支付订单接口。');const paymentUrl=new URL(url,location.origin);if(!['http:','https:'].includes(paymentUrl.protocol))throw new Error('支付接口返回了无效的支付地址。');location.assign(paymentUrl.href);}
  });
});

document.addEventListener('submit',event=>{
  const form=event.target;if(form.method==='dialog')return;
  if(!['search-form','login-form','book-form','comment-form'].includes(form.id))return;event.preventDefault();
  if(form.id==='search-form'){filter.keyword=new FormData(form).get('keyword').trim();filter.page=1;render();return;}
  busy(event.submitter,async()=>{
    const f=Object.fromEntries(new FormData(form));
    if(form.id==='login-form'){
      $('login-error').textContent='';try{if(!f.name.trim())throw new Error('姓名不能为空。');await api.login(f.name.trim(),f.password);form.reset();$('login-dialog').close();refreshShell();notify('登录成功。');const after=pendingLogin;pendingLogin=null;if(after)after();}catch(e){$('login-error').textContent=e.message;}return;
    }
    if(form.id==='comment-form'){if(!requireLogin())return;await api.comment(form.dataset.id,f.content);notify('留言已发布。');render();return;}
    if(form.id==='book-form'){
      const error=$('form-error');error.textContent='';try{
        for(const key of ['title','course','contact','description'])f[key]=f[key].trim();
        if(!f.title||!f.course||!f.contact)throw new Error('教材名称、课程和联系方式不能为空。');
        if(!/^\d+(\.\d{1,2})?$/.test(f.price.trim()))throw new Error('价格请输入正数，最多两位小数。');
        const [yuan,decimal='']=f.price.trim().split('.');f.price=Number(yuan)*100+Number(decimal.padEnd(2,'0'));
        if(!Number.isSafeInteger(f.price)||f.price<=0)throw new Error('价格必须大于零，且不能超过安全整数范围。');
        const payload=Object.fromEntries(['title','course','condition','price','description','contact','category'].map(k=>[k,f[k]]));
        const result=form.dataset.id?await api.update(form.dataset.id,payload):await api.create(payload);
        notify(form.dataset.id?'教材信息已更新。':'教材发布成功。');nav('/book/'+result.id);
      }catch(e){error.textContent=e.message;refreshShell();}
    }
  });
});

document.addEventListener('change',event=>{
  if(event.target.id==='condition-filter'){filter.condition=event.target.value;filter.page=1;render();}
  if(event.target.id==='sort-filter'){filter.sort=event.target.value;filter.page=1;render();}
});
document.addEventListener('input',event=>{const form=event.target.closest('#book-form');if(form&&event.target.name==='title'){$('cover-preview').innerHTML=cover({title:form.elements.title.value||'你的下一本好书'},'preview-cover');}});
window.addEventListener('hashchange',()=>{window.scrollTo(0,0);render();});
api.restoreSession().finally(render);
