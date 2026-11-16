import { api, state, logout } from '/api.js';

// 各页面使用独立 HTML 地址，共享本文件的页面渲染与交互逻辑。
const $ = id => document.getElementById(id);
const params = new URLSearchParams(location.search);
const pageName = document.body.dataset.page || 'home';
const app = $('app');
const names = {
  on_sale: '在售', reserved: '交易中', off_shelf: '已下架', sold: '已售出',
  pending: '待支付', paid: '待交接', completed: '已完成', cancelled: '已取消',
};
const categories = ['全部', '计算机', '公共课', '语言', '设计', '其他'];
const conditions = ['全新', '九成新', '八成新', '七成新'];
let renderVersion = 0;
let toastTimer;
let filter = {
  keyword: params.get('keyword') || '', category: params.get('category') || '全部',
  condition: params.get('condition') || '全部', sort: params.get('sort') || 'new',
  page: Math.max(1, Number(params.get('page')) || 1),
};
let mineTab = params.get('status') || 'all';
let orderTab = params.get('status') || 'all';
let orderRole = params.get('role') === 'sell' ? 'sell' : 'buy';
let commentDraft = '';
let savedDraft = null;
let restoreError = null;

// 将来自数据库的文字转义，避免书名、留言等内容被当成 HTML 执行。
const esc = value => String(value ?? '').replace(/[&<>"']/g, char => ({
  '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
}[char]));
const sameID = (a, b) => String(a) === String(b);
const money = cents => Number.isSafeInteger(Number(cents)) ? (Number(cents) / 100).toFixed(2) : '—';
function date(value) {
  if (!value) return '暂无记录';
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? '暂无记录' : parsed.toLocaleString('zh-CN', { hour12: false });
}
function requireID() {
  const id = params.get('id');
  if (!id || !/^[1-9]\d*$/.test(id) || !Number.isSafeInteger(Number(id))) {
    throw new Error('编号不正确，请从列表重新打开。');
  }
  return id;
}
function array(value) {
  if (!Array.isArray(value)) throw new Error('列表数据暂时无法读取，请稍后重试。');
  return value;
}
function link(page, id) {
  const pages = { home: 'index', book: 'detail', publish: 'publish', edit: 'edit', mine: 'mine', orders: 'orders', order: 'order', pay: 'payment', profile: 'profile', guide: 'guide', comments: 'comments' };
  return `/${pages[page] || 'index'}.html${id === undefined ? '' : '?id=' + encodeURIComponent(id)}`;
}
function notify(message, isError = false) {
  clearTimeout(toastTimer);
  $('toast').textContent = message;
  $('toast').className = isError ? 'error-toast' : '';
  $('toast').hidden = false;
  toastTimer = setTimeout(() => { $('toast').hidden = true; }, 4500);
}
function heading(title, text) {
  return `<div class="page-heading"><span class="overline">CAMPUS BOOKS</span><h1>${esc(title)}</h1><p class="muted">${esc(text)}</p></div>`;
}
function status(value) { return `<span class="status ${esc(value)}">${esc(names[value] || '状态待确认')}</span>`; }
function price(value) { return `<span class="price"><small>¥</small>${money(value)}</span>`; }
function empty(title, text, href = link('home'), label = '去教材广场') {
  return `<section class="empty"><span class="empty-symbol" aria-hidden="true">▤</span><h2>${esc(title)}</h2><p>${esc(text)}</p>${href ? `<a class="btn secondary" href="${href}">${esc(label)} →</a>` : ''}</section>`;
}
function errorBox(error, title = '暂时无法加载') {
  return `<section class="empty error-state" role="alert"><span class="empty-symbol" aria-hidden="true">↻</span><h2>${esc(title)}</h2><p>${esc(error.message || '请稍后重试。')}</p><div class="button-row center"><button class="btn secondary" data-action="retry">重新加载</button><a class="text-link" href="${link('home')}">返回教材广场</a></div></section>`;
}
function cover(book, extra = '') {
  const theme = (Number(book.id || book.book_id) || 0) % 7;
  return `<div class="cover-area theme${theme} ${extra}" aria-hidden="true"><div class="book-cover"><strong>${esc(book.title || '你的下一本好书')}</strong><small>CAMPUS · BOOKS<br>把知识传递下去</small></div></div>`;
}
function seller(book) { return book.seller_name || (book.seller_id ? `校园同学 #${book.seller_id}` : '校园同学'); }
function classify(book) {
  if (book.category) return book.category;
  const text = (book.course || '') + (book.title || '');
  if (/计算机|程序|数据|网络|Go|Java|Python/i.test(text)) return '计算机';
  if (/英语|语言/.test(text)) return '语言';
  if (/数学|代数|思政|体育/.test(text)) return '公共课';
  if (/设计|美术/.test(text)) return '设计';
  return '其他';
}
// 只允许回到本站的已知页面，避免 next 参数形成外部跳转。
function safeNext(value) {
  try {
    const url = new URL(value || '/index.html', location.origin);
    const allowed = /^\/(index|detail|publish|edit|mine|orders|order|payment|profile|guide|comments)\.html$/;
    return url.origin === location.origin && allowed.test(url.pathname) ? url.pathname + url.search : '/index.html';
  } catch { return '/index.html'; }
}
function goLogin() {
  location.assign('/login.html?next=' + encodeURIComponent(safeNext(location.pathname + location.search)));
}
function refreshShell() {
  $('account-nav').innerHTML = state.user
    ? `<a class="account-button" href="${link('profile')}">${esc(state.user.name || '同学')} · 个人中心</a>`
    : '<a class="account-button" href="/login.html">登录 / 开始使用</a>';
  document.querySelectorAll('[data-nav]').forEach(node => {
    const active = node.dataset.nav === pageName || (node.dataset.nav === 'orders' && ['order', 'pay'].includes(pageName));
    node.classList.toggle('active', active);
    if (active) node.setAttribute('aria-current', 'page'); else node.removeAttribute('aria-current');
  });
}
function updateSearch(values) {
  const url = new URL(location.href);
  url.search = '';
  for (const [key, value] of Object.entries(values)) if (value !== '' && value !== '全部' && value !== 'all') url.searchParams.set(key, value);
  history.replaceState(null, '', url);
}

// 登录是独立页面；成功后进入 index.html 或回到原先需要登录的页面。
function loginPage() {
  return `<div class="login-layout"><section class="login-story"><span class="overline">A NEW CHAPTER</span><h1>下一本好书，<br>就在校园里。</h1><p>让书架上的闲置，成为另一位同学的新开始。<br>找教材、问版本、约交接，把知识继续传下去。</p><div class="login-books" aria-hidden="true">${cover({ title: '知识\n再出发', id: 0 })}${cover({ title: '下一章\n属于你', id: 1 })}</div><span class="small">校园二手教材 · 校内交流 · 友善交易</span></section><section class="panel login-panel"><span class="overline">WELCOME BACK</span><h2>你好，同学。</h2><p class="muted">登录校园书集，找到你需要的那一本。</p><form id="login-form"><label for="login-name">姓名</label><input id="login-name" name="name" autocomplete="username" maxlength="50" required placeholder="请输入姓名"><label for="login-password">密码</label><div class="password-field"><input id="login-password" name="password" type="password" autocomplete="current-password" required placeholder="请输入密码"><button id="toggle-password" type="button" data-action="toggle-password" aria-controls="login-password" aria-pressed="false">显示</button></div><p class="small muted">使用已开通的姓名账号，初始密码为学号。</p><p id="login-error" class="error" role="alert"></p><button class="btn full" type="submit">登录校园书集 →</button></form><p class="login-foot muted small">暂不提供在线注册。如需开通账号，请联系课程项目维护人。</p><a class="text-link" href="${link('guide')}">先了解使用方式 ↗</a></section></div>`;
}
function card(book) {
  return `<article class="book-card"><a href="${link('book', book.id)}" aria-label="查看${esc(book.title)}">${cover(book)}<div class="book-info"><span class="book-category">${esc(classify(book))}</span><h3>${esc(book.title)}</h3><p>${esc(book.course)} · ${esc(book.condition)}</p><div class="book-card-bottom">${price(book.price)}<span class="seller">${esc(seller(book))}</span></div></div></a></article>`;
}

async function home() {
  // 列表错误只影响数据区域；搜索框和页面介绍仍保留，用户可以重试。
  let books = [], failure = null;
  try { books = array(await api.books(filter.keyword)); } catch (error) { failure = error; }
  let list = books.filter(book => (filter.category === '全部' || classify(book) === filter.category) && (filter.condition === '全部' || book.condition === filter.condition));
  list.sort(filter.sort === 'low' ? (a, b) => a.price - b.price : filter.sort === 'high' ? (a, b) => b.price - a.price : (a, b) => b.id - a.id);
  const count = list.length, pages = Math.max(1, Math.ceil(count / 8));
  filter.page = Math.max(1, Math.min(filter.page, pages));
  const view = list.slice((filter.page - 1) * 8, filter.page * 8);
  return `<section class="hero"><div><span class="overline">A NEW CHAPTER FOR EVERY BOOK</span><h1>让闲置的教材，<br>遇见<em>下一位主人。</em></h1><p>少一点闲置，多一份收获。<br>在熟悉的校园，找到你需要的那一本。</p><div class="hero-links"><a class="btn" href="${link('publish')}">发布我的教材 ↗</a><a class="text-link" href="${link('guide')}">了解交易流程 →</a></div></div><div class="hero-art" aria-hidden="true"><div class="art-circle"></div><div class="art-book one"><b>知识<br>再出发</b><small>READ & REUSE</small></div><div class="art-book two"><b>下一章<br>属于你</b><small>CAMPUS COLLECTION</small></div><div class="art-stamp">旧书有价 · 知识无价</div></div></section><div class="benefits"><span><b>01</b> 校园同学 · 友善交流</span><span><b>02</b> 教材信息 · 如实描述</span><span><b>03</b> 在线下单 · 校内交接</span></div><section><div class="section-top"><div><h2>发现好教材 <span class="small muted">/ BOOKS</span></h2><p>让需要的教材，刚好遇见你。</p></div><form id="search-form" class="search"><label class="sr-only" for="search-keyword">搜索书名或课程</label><input id="search-keyword" name="keyword" value="${esc(filter.keyword)}" maxlength="100" placeholder="搜索书名或课程…"><button class="btn" type="submit">搜索</button></form></div><div class="filters">${categories.map(value => `<button class="chip ${value === filter.category ? 'active' : ''}" data-action="category" data-value="${value}" aria-pressed="${value === filter.category}">${value}</button>`).join('')}<label class="sr-only" for="condition-filter">教材成色</label><select id="condition-filter" class="sort"><option value="全部">全部成色</option>${conditions.map(value => `<option ${value === filter.condition ? 'selected' : ''}>${value}</option>`).join('')}</select><label class="sr-only" for="sort-filter">排序方式</label><select id="sort-filter" class="sort">${[['new', '最新发布'], ['low', '价格从低到高'], ['high', '价格从高到低']].map(([value, label]) => `<option value="${value}" ${value === filter.sort ? 'selected' : ''}>${label}</option>`).join('')}</select><button class="text-link" data-action="reset-filters">重置筛选</button></div>${failure ? errorBox(failure, '教材暂时无法加载') : `<p class="filter-count">找到 ${count} 本在售教材${filter.keyword ? ' · 关键词“' + esc(filter.keyword) + '”' : ''}</p><div class="books-grid">${view.length ? view.map(card).join('') : empty('暂时没有找到教材', '换个关键词或重置筛选，再看看吧。', null)}</div>${count ? `<nav class="page-controls" aria-label="教材分页"><button data-action="page" data-value="${filter.page - 1}" ${filter.page <= 1 ? 'disabled' : ''} aria-label="上一页">‹</button><span>第 ${filter.page} / ${pages} 页</span><button data-action="page" data-value="${filter.page + 1}" ${filter.page >= pages ? 'disabled' : ''} aria-label="下一页">›</button></nav>` : ''}`}</section><section class="join-banner"><div><h3>书架上的闲置，也能成为别人的新开始。</h3><p>写下教材信息，把它交给下一位需要的同学。</p></div><a class="btn secondary" href="${link('publish')}">发布一本书 →</a></section>`;
}

// 单独渲染留言区域，发布失败时保留草稿，查询失败也不遮住教材详情。
async function commentSection(id, writable) {
  let comments = [], failure = null;
  try { comments = array(await api.comments(id)); } catch (error) { failure = error; }
  return `<section class="panel comment-section" id="comments"><div class="section-heading"><h2>教材留言 <span class="small muted">${failure ? '' : comments.length + ' 条'}</span></h2><button class="text-link" data-action="retry">刷新留言 ↻</button></div>${failure ? `<p class="notice-box" role="alert">${esc(failure.message)}</p>` : comments.length ? comments.map(comment => `<article class="comment-item"><div class="row"><strong>${esc(comment.name || comment.user_name || '校园同学')}</strong><time>${date(comment.created_at)}</time></div><p>${esc(comment.content)}</p>${sameID(state.user?.id, comment.user_id) ? `<button class="text-link" data-action="delete-comment" data-id="${esc(comment.id)}">删除留言</button>` : ''}</article>`).join('') : '<p class="muted">还没有留言，问问教材版本或交接方式吧。</p>'}${writable ? `<form id="comment-form" class="comment-form" data-id="${esc(id)}"><label for="comment-content">写下你的问题<textarea id="comment-content" name="content" maxlength="500" rows="3" required placeholder="文明交流，请勿发布无关信息。">${esc(commentDraft)}</textarea></label><div class="comment-footer"><span id="comment-count" class="small muted">${commentDraft.length}/500</span><button class="btn" type="submit">发布留言 →</button></div><p id="comment-error" class="error" role="alert"></p></form>` : '<p class="notice-box">教材已停止出售，历史留言仍保留，暂不接受新留言。</p>'}</section>`;
}
async function detail() {
  const id = requireID();
  let book;
  try { book = await api.detail(id); } catch (error) {
    if (error.status !== 404) throw error;
    // 当前后端不返回下架详情，仍允许进入历史留言页读取已保存的留言。
    return heading('教材暂不可查看', '这本教材可能已下架，历史留言仍可查询。') + `<section class="panel"><p class="muted">${esc(error.message)}</p><div class="button-row"><a class="btn secondary" href="${link('comments', id)}">查看历史留言</a><a class="text-link" href="${link('mine')}">我的发布 →</a></div></section>`;
  }
  const own = sameID(book.seller_id, state.user.id);
  const actions = book.status !== 'on_sale' ? '<span class="muted">这本教材暂不出售。</span>' : own
    ? `<a class="btn" href="${link('edit', id)}">编辑教材</a><button class="btn danger" data-action="remove" data-id="${id}">下架教材</button>`
    : `<button class="btn" data-action="buy" data-id="${id}">立即购买 →</button><a class="btn secondary" href="#comments">留言问问</a>`;
  return `<div class="breadcrumb"><a href="${link('home')}">教材广场</a> / 教材详情</div><section class="panel detail-layout">${cover(book, 'detail-cover')}<div class="detail-info">${status(book.status)}<h1>${esc(book.title)}</h1>${price(book.price)}<dl><dt>适用课程</dt><dd>${esc(book.course)}</dd><dt>教材成色</dt><dd>${esc(book.condition)}</dd><dt>发布同学</dt><dd>${esc(seller(book))}</dd><dt>联系方式</dt><dd>${esc(book.contact)}</dd><dt>发布时间</dt><dd>${date(book.created_at)}</dd></dl><div class="description">${esc(book.description || '发布同学暂时没有补充说明。')}</div><div class="button-row detail-actions">${actions}</div><p class="small muted">封面为文字示意。购买前请确认版次、成色和交接方式。</p><p id="action-error" class="error" role="alert"></p></div></section>${await commentSection(id, book.status === 'on_sale')}`;
}
async function historyComments() {
  const id = requireID();
  return heading('历史留言', `教材编号 ${id} · 已发布的交流记录`) + await commentSection(id, false);
}

async function editor() {
  const editing = pageName === 'edit';
  const id = editing ? requireID() : null;
  const original = editing ? await api.detail(id) : { title: '', course: '', condition: '九成新', price: '', description: '', contact: '', category: '计算机' };
  if (editing && (!sameID(original.seller_id, state.user.id) || original.status !== 'on_sale')) throw new Error('只能修改自己发布的在售教材。');
  const book = savedDraft ? { ...original, ...savedDraft } : original;
  const yuan = savedDraft ? savedDraft.price : (book.price === '' ? '' : money(book.price));
  return `<div class="breadcrumb"><a href="${link('mine')}">我的发布</a> / ${editing ? '编辑教材' : '发布教材'}</div>${heading(editing ? '让教材信息更准确。' : '给教材，一个新的开始。', '如实填写版本、成色与价格，让需要它的同学更容易找到。')}<div class="page-columns"><section class="panel"><h2>${editing ? '编辑教材信息' : '填写教材信息'}</h2><form id="book-form" data-id="${id || ''}"><div class="form-grid"><label class="wide">教材名称<input name="title" maxlength="100" required value="${esc(book.title)}" placeholder="例如：Go 语言程序设计（第 2 版）"></label><label>适用课程<input name="course" maxlength="100" required value="${esc(book.course)}" placeholder="例如：程序设计基础"></label><label>教材分类<select name="category">${categories.slice(1).map(value => `<option ${value === classify(book) ? 'selected' : ''}>${value}</option>`).join('')}</select></label><label>教材成色<select name="condition">${[...new Set([...conditions, book.condition])].filter(Boolean).map(value => `<option ${value === book.condition ? 'selected' : ''}>${esc(value)}</option>`).join('')}</select></label><label>售价（元）<input name="price" required inputmode="decimal" maxlength="12" value="${esc(yuan)}" placeholder="例如：25.00"></label><label class="wide">联系方式<input name="contact" maxlength="100" required value="${esc(book.contact)}" placeholder="微信、电话或校内交接方式"></label><label class="wide">补充说明<textarea name="description" maxlength="1000" rows="5" placeholder="版次、笔记、破损和交接位置等">${esc(book.description)}</textarea></label></div><p id="form-error" class="error" role="alert"></p><div class="button-row"><button class="btn" type="submit">${editing ? '保存修改' : '确认发布'} →</button><a class="btn secondary" href="${link('mine')}">返回我的发布</a></div></form></section><aside class="panel"><div id="cover-preview">${cover(book, 'preview-cover')}</div><h3>发布小贴士</h3><ul class="hint-list"><li>写清教材名称和版次。</li><li>如实描述笔记、破损和缺页。</li><li>价格以元填写，最多两位小数。</li><li>封面是文字示意，请在描述中说明实际成色。</li></ul></aside></div>`;
}
async function mine() {
  let books = [], failure = null;
  try { books = array(await api.mine(mineTab === 'all' ? '' : mineTab)); } catch (error) { failure = error; }
  return heading('我发布的教材', '整理书架，管理每一本教材的下一章。') + `<div class="tabs">${[['all', '全部'], ['on_sale', '在售'], ['reserved', '交易中'], ['off_shelf', '已下架'], ['sold', '已售出']].map(([value, label]) => `<button class="chip ${value === mineTab ? 'active' : ''}" data-action="mine-tab" data-value="${value}">${label}</button>`).join('')}<a href="${link('publish')}" class="text-link push-right">＋ 发布教材</a></div>${failure ? errorBox(failure) : books.length ? books.map(book => `<article class="management-card"><div class="management-row">${cover(book, 'mini-cover')}<div class="management-main">${status(book.status)}<h2 class="item-title">${esc(book.title)}</h2><p>${esc(book.course)} · ${esc(book.condition)}</p>${price(book.price)}</div><div class="button-row">${book.status === 'on_sale' ? `<a class="btn secondary" href="${link('book', book.id)}">查看</a><a class="btn secondary" href="${link('edit', book.id)}">编辑</a><button class="btn danger" data-action="remove" data-id="${esc(book.id)}">下架</button>` : `<a class="text-link" href="${link('comments', book.id)}">历史留言 →</a>`}</div></div></article>`).join('') : empty('这里暂时没有教材', '发布一本教材，让闲置的知识继续流转。', link('publish'), '发布教材')}`;
}

// 订单金额和交易快照完全以服务端为准，浏览器不会重新计算应付金额。
function orderActions(order, isBuyer) {
  if (!isBuyer) return '<span class="small muted">等待买家操作，可联系对方完成交接。</span>';
  if (order.status === 'pending') return `<button class="text-link" data-action="cancel-order" data-id="${esc(order.id)}">取消订单</button><a class="btn" href="${link('pay', order.id)}">去支付 →</a>`;
  if (order.status === 'paid') return `<button class="btn" data-action="complete-order" data-id="${esc(order.id)}">确认收到教材</button>`;
  return `<span class="muted small">${esc(names[order.status] || '请刷新确认状态')}</span>`;
}
function orderCard(order) {
  const isBuyer = sameID(order.buyer_id, state.user.id);
  return `<article class="order-card"><div class="order-header"><span>订单 ${esc(order.number || order.id)}</span><span>${date(order.created_at)}</span>${status(order.status)}</div><div class="management-row">${cover(order, 'mini-cover')}<div class="management-main"><h2 class="item-title"><a href="${link('order', order.id)}">${esc(order.title || '教材订单')}</a></h2><p>${isBuyer ? '卖家：' + esc(order.seller_name || `校园同学 #${order.seller_id}`) : '买家：' + esc(order.buyer_name || `校园同学 #${order.buyer_id}`)}</p><p>交接联系：${esc(order.contact || '请在订单详情中确认')}</p></div>${price(order.price)}</div><div class="order-footer"><a class="text-link" href="${link('order', order.id)}">订单详情 ↗</a><div class="button-row">${orderActions(order, isBuyer)}</div></div></article>`;
}
async function orders() {
  let list = [], failure = null;
  try { list = array(await api.orders(orderRole, orderTab === 'all' ? '' : orderTab)); } catch (error) { failure = error; }
  return heading('每一笔交易，都有记录。', '购买、付款与校内交接，在这里查看进度。') + `<div class="tabs">${[['buy', '我买到的'], ['sell', '我卖出的']].map(([value, label]) => `<button class="chip ${value === orderRole ? 'active' : ''}" data-action="order-role" data-value="${value}">${label}</button>`).join('')}</div><div class="filters">${[['all', '全部'], ['pending', '待支付'], ['paid', '待交接'], ['completed', '已完成'], ['cancelled', '已取消']].map(([value, label]) => `<button class="chip ${value === orderTab ? 'active' : ''}" data-action="order-tab" data-value="${value}">${label}</button>`).join('')}</div>${failure ? errorBox(failure, '订单暂时无法加载') : list.length ? list.map(orderCard).join('') : empty('暂无相关订单', '去教材广场看看，也许正好有你需要的那一本。')}`;
}
async function orderDetail(payment = false) {
  const id = requireID();
  let order;
  try { order = await api.getOrder(id); } catch (error) {
    return heading(payment ? '订单支付' : '订单详情', '核对教材与订单信息。') + errorBox(error, '订单暂时无法加载');
  }
  const isBuyer = sameID(order.buyer_id, state.user.id);
  if (!isBuyer && !sameID(order.seller_id, state.user.id)) throw new Error('你无权查看这笔订单。');
  if (payment && !isBuyer) throw new Error('只有买家可以支付这笔订单。');
  const paid = ['paid', 'completed'].includes(order.status);
  const returning = params.get('return') === '1';
  const message = order.status === 'completed' ? '教材交接已完成，祝学习顺利。'
    : paid ? '付款已确认，请联系卖家完成校内交接。'
    : order.status === 'cancelled' ? '订单已取消，无需继续付款。'
    : returning ? '尚未收到付款确认。请稍后刷新，避免重复付款。' : '请核对教材与金额，再前往支付。';
  return `<div class="payment-layout"><div class="breadcrumb"><a href="${link('orders')}">我的订单</a> / ${payment ? '订单支付' : '订单详情'}</div>${heading(payment ? '确认订单，继续学习。' : '每次交接，都有记录。', message)}<section class="panel"><div class="section-heading"><h2>订单 ${esc(order.number || order.id)}</h2>${status(order.status)}</div><ol class="order-steps"><li class="done">提交订单</li><li class="${paid ? 'done' : ''}">付款确认</li><li class="${order.status === 'completed' ? 'done' : ''}">完成交接</li></ol><div class="management-row">${cover(order, 'mini-cover')}<div class="management-main"><h2 class="item-title">${esc(order.title || '教材订单')}</h2><p>卖家：${esc(order.seller_name || `校园同学 #${order.seller_id}`)}</p><p>买家：${esc(order.buyer_name || `校园同学 #${order.buyer_id}`)}</p></div></div><dl class="order-info"><dt>下单时间</dt><dd>${date(order.created_at)}</dd><dt>付款时间</dt><dd>${date(order.paid_at)}</dd><dt>交接联系</dt><dd>${esc(order.contact || '暂无联系方式')}</dd></dl><div class="payment-summary"><span>订单金额</span>${price(order.price)}</div>${payment && order.status === 'pending' ? `<p class="notice-box">${returning ? '付款确认可能稍有延迟，请先刷新状态。' : '付款后请返回此页确认结果。'}请勿向个人提供银行卡密码或验证码。</p><div class="button-row"><button class="btn" data-action="${returning ? 'retry' : 'pay'}" data-id="${esc(order.id)}">${returning ? '我已付款，刷新状态' : '前往支付 ¥' + money(order.price)}</button>${returning ? `<button class="text-link" data-action="pay" data-id="${esc(order.id)}">重新前往支付</button>` : ''}<a class="btn secondary" href="${link('orders')}">稍后处理</a></div>` : `<div class="button-row">${orderActions(order, isBuyer)}<button class="btn secondary" data-action="retry">刷新状态</button></div>`}<p id="action-error" class="error" role="alert"></p></section><p class="small muted">交易金额以订单记录为准。付款完成后，请当面检查教材再确认收货。</p></div>`;
}
async function profile() {
  const user = await api.me();
  return `<section class="profile-banner panel"><span class="avatar" aria-hidden="true">${esc((user.name || '同学').slice(0, 1))}</span><div><span class="overline">MY CAMPUS BOOKS</span><h1>${esc(user.name || '同学')}，你好。</h1><p>让每一本书，都遇见下一位认真读它的人。</p></div></section><div class="profile-grid">${[['mine', '我的发布', '修改、下架和查看教材状态。'], ['orders', '我的订单', '查看购买、出售与交接进度。'], ['publish', '发布教材', '整理闲置，让知识继续流转。']].map(([page, title, text]) => `<a class="profile-tile" href="${link(page)}"><h2>${title} ↗</h2><p>${text}</p></a>`).join('')}</div><section class="panel account-panel"><h2>账号与使用</h2><p class="muted">用户编号：${esc(user.id)}</p><p class="muted">登录状态保留在当前标签页。退出后需要重新登录。</p><div class="button-row"><a class="btn secondary" href="${link('guide')}">使用指南</a><button class="btn danger" data-action="logout">退出登录</button></div></section>`;
}
function guide() {
  return `<div class="guide">${heading('让每次转手，都简单一点。', '校园书集 · 使用指南')}<section class="panel"><h2>01 登录，找到需要的教材</h2><p>使用已开通的姓名账号和初始学号密码登录。通过书名或课程搜索，结合分类、成色和价格排序找到教材。封面是文字示意，请打开详情确认版本、笔记和破损情况。</p><h2>02 如实发布，友善交流</h2><p>发布时填写教材、课程、成色、售价和联系方式。价格以元填写，最多两位小数。你可以编辑或下架自己在售的教材；下架后保留教材记录和历史留言。留言区可用来确认版本与交接位置。</p><h2>03 下单、付款与交接</h2><p>确认教材信息后提交订单，在“我的订单”中继续付款或取消待支付订单。付款完成后刷新订单确认状态。联系对方完成校内交接，检查教材后再确认收货。暂不提供预约、物流和退款功能。</p><h2>04 保护账号与个人信息</h2><p>不要向他人提供登录密码、付款验证码或银行卡密码。交易前核对金额和联系信息，遇到异常先暂停操作。退出登录后会返回登录页。</p><h2>常见问题</h2><details><summary>没有找到想要的教材？</summary><p>试试课程名、简短关键词，或者重置分类和成色筛选。</p></details><details><summary>为什么某些操作提示暂不可用？</summary><p>部分交易功能仍在完善中。操作失败不会显示为成功，请稍后重试。</p></details><details><summary>付款后仍显示待支付？</summary><p>付款确认可能延迟。先点击刷新状态，避免重复付款；仍有疑问时保留订单号并联系维护人。</p></details><div class="button-row"><a class="btn secondary" href="${state.user ? link('home') : '/login.html'}">${state.user ? '回到教材广场' : '前往登录'} →</a></div></section></div>`;
}

// 每次渲染分配编号，旧请求晚返回时不覆盖用户最新选择的页面。
async function render() {
  const version = ++renderVersion;
  refreshShell();
  app.setAttribute('aria-busy', 'true');
  app.innerHTML = '<div class="loading" role="status"><span class="loading-dot"></span>正在整理书架…</div>';
  try {
    const routes = { login: loginPage, home, book: detail, comments: historyComments, publish: editor, edit: editor, mine, orders, order: () => orderDetail(), pay: () => orderDetail(true), profile, guide };
    const html = await (routes[pageName] || (() => empty('没有找到这一页', '请从导航重新打开。')))();
    if (version !== renderVersion) return;
    app.innerHTML = html;
    refreshShell();
  } catch (error) {
    if (version !== renderVersion) return;
    app.innerHTML = errorBox(error);
    refreshShell();
  } finally {
    if (version === renderVersion) app.setAttribute('aria-busy', 'false');
  }
}
function confirmAction(title, message) {
  return new Promise(resolve => {
    const dialog = $('confirm-dialog');
    $('confirm-title').textContent = title;
    $('confirm-message').textContent = message;
    dialog.returnValue = '';
    dialog.addEventListener('close', () => resolve(dialog.returnValue === 'yes'), { once: true });
    dialog.showModal();
  });
}
// 禁用按钮和显示等待文字，避免用户连续点击重复提交。
async function busy(button, action, errorID) {
  if (!button || button.disabled) return;
  const text = button.textContent;
  button.disabled = true;
  button.textContent = '处理中…';
  if (errorID && $(errorID)) $(errorID).textContent = '';
  try { await action(); } catch (error) {
    if (errorID && $(errorID)) $(errorID).textContent = error.message;
    else notify(error.message, true);
    refreshShell();
  } finally {
    button.disabled = false;
    button.textContent = text;
  }
}

// 点击事件统一入口；只有带 data-action 的元素会进入这里。
document.addEventListener('click', event => {
  const button = event.target.closest('[data-action]');
  if (!button) return;
  event.preventDefault();
  const { action, id, value } = button.dataset;
  if (action === 'toggle-password') {
    const input = $('login-password');
    const hidden = input.type === 'password';
    input.type = hidden ? 'text' : 'password';
    button.textContent = hidden ? '隐藏' : '显示';
    button.setAttribute('aria-pressed', String(hidden));
    return;
  }
  if (action === 'retry') { start(); return; }
  if (action === 'category') { filter.category = value; filter.page = 1; updateSearch(filter); render(); return; }
  if (action === 'page') { filter.page = Number(value); updateSearch(filter); render(); window.scrollTo({ top: 300, behavior: 'smooth' }); return; }
  if (action === 'reset-filters') { filter = { keyword: '', category: '全部', condition: '全部', sort: 'new', page: 1 }; updateSearch(filter); render(); return; }
  if (action === 'mine-tab') { mineTab = value; updateSearch({ status: value }); render(); return; }
  if (action === 'order-role') { orderRole = value; orderTab = 'all'; updateSearch({ role: orderRole }); render(); return; }
  if (action === 'order-tab') { orderTab = value; updateSearch({ role: orderRole, status: value }); render(); return; }
  busy(button, async () => {
    if (action === 'logout') { logout(); location.assign('/login.html'); }
    if (action === 'buy') {
      if (!await confirmAction('确认购买这本教材？', '请先核对版次、成色和联系方式。确认后创建待支付订单，暂不扣款。')) return;
      const order = await api.order(id);
      if (!order?.id) throw new Error('未能读取订单，请先到“我的订单”确认，避免重复下单。');
      location.assign(link('pay', order.id));
    }
    if (action === 'remove') {
      if (!await confirmAction('下架这本教材？', '下架后不再出现在教材广场，历史留言仍保留。请确认没有正在进行的交易。')) return;
      await api.remove(id); await render(); notify('教材已下架。');
    }
    if (action === 'delete-comment') {
      if (!await confirmAction('删除这条留言？', '删除后无法恢复，请确认。')) return;
      await api.deleteComment(id); await render(); notify('留言已删除。');
    }
    if (action === 'cancel-order') {
      if (!await confirmAction('取消这笔订单？', '取消后不能继续支付这笔订单。')) return;
      await api.cancel(id); await render(); notify('订单已取消。');
    }
    if (action === 'complete-order') {
      if (!await confirmAction('确认收到教材？', '请在当面检查教材、完成交接后确认。')) return;
      await api.complete(id); await render(); notify('交易已完成，祝学习顺利。');
    }
    if (action === 'pay') {
      const result = await api.pay(id);
      const value = result?.payment_url || result?.checkout_url;
      if (!value) throw new Error('暂时无法打开付款页面，请稍后重试。');
      // 只接受 HTTP(S) 地址，避免执行接口返回的脚本或其他危险协议。
      const url = new URL(value, location.origin);
      if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) throw new Error('付款地址异常，请联系维护人。');
      location.assign(url.href);
    }
  }, 'action-error');
});

// 表单事件：校验后调用对应 API，失败时保留用户输入。
document.addEventListener('submit', event => {
  const form = event.target;
  if (!['search-form', 'login-form', 'book-form', 'comment-form'].includes(form.id)) return;
  event.preventDefault();
  const values = Object.fromEntries(new FormData(form));
  if (form.id === 'search-form') {
    filter.keyword = values.keyword.trim(); filter.page = 1;
    updateSearch(filter); render(); return;
  }
  const errorID = { 'login-form': 'login-error', 'book-form': 'form-error', 'comment-form': 'comment-error' }[form.id];
  busy(event.submitter || form.querySelector('[type="submit"]'), async () => {
    if (form.id === 'login-form') {
      if (!values.name.trim()) throw new Error('姓名不能为空。');
      await api.login(values.name.trim(), values.password);
      form.reset();
      location.assign(safeNext(params.get('next')));
    }
    if (form.id === 'comment-form') {
      const content = values.content.trim();
      if (!content) throw new Error('留言不能为空或只有空格。');
      await api.comment(form.dataset.id, content);
      commentDraft = ''; await render(); notify('留言已发布。');
    }
    if (form.id === 'book-form') {
      for (const key of ['title', 'course', 'description', 'contact']) values[key] = values[key].trim();
      if (!values.title || !values.course || !values.contact) throw new Error('教材名称、课程和联系方式不能为空。');
      // 先拆分元和小数，再转整数分，避免 19.99 * 100 的浮点误差。
      if (!/^\d+(\.\d{1,2})?$/.test(values.price.trim())) throw new Error('售价请输入正数，最多两位小数。');
      const [yuan, fraction = ''] = values.price.trim().split('.');
      const cents = Number(yuan) * 100 + Number(fraction.padEnd(2, '0'));
      if (!Number.isSafeInteger(cents) || cents <= 0) throw new Error('售价必须大于零，且不能超过有效范围。');
      const payload = { ...values, price: cents };
      const result = form.dataset.id ? await api.update(form.dataset.id, payload) : await api.create(payload);
      if (!result?.id) throw new Error('保存结果暂时无法读取，请到“我的发布”确认，避免重复提交。');
      savedDraft = null;
      location.assign(link('book', result.id));
    }
  }, errorID);
});

document.addEventListener('change', event => {
  if (event.target.id === 'condition-filter') filter.condition = event.target.value;
  else if (event.target.id === 'sort-filter') filter.sort = event.target.value;
  else return;
  filter.page = 1; updateSearch(filter); render();
});
document.addEventListener('input', event => {
  if (event.target.id === 'comment-content') {
    commentDraft = event.target.value;
    $('comment-count').textContent = `${commentDraft.length}/500`;
  }
  const form = event.target.closest('#book-form');
  if (form) {
    savedDraft = Object.fromEntries(new FormData(form));
    if (event.target.name === 'title') $('cover-preview').innerHTML = cover({ title: event.target.value }, 'preview-cover');
  }
});
window.addEventListener('session-expired', () => { if (pageName !== 'login') goLogin(); });

// 兼容旧版 #/book/12 等书签，统一转入新的独立 HTML 页面。
function migrateLegacyURL() {
  if (!location.hash.startsWith('#/')) return false;
  const [page, id] = location.hash.slice(2).split('/');
  location.replace(link(page || 'home', id));
  return true;
}
async function start() {
  if (migrateLegacyURL()) return;
  try {
    if (state.hasToken && !state.user) await api.restoreSession();
    restoreError = null;
  } catch (error) { restoreError = error; }
  if (!state.user && !['login', 'guide'].includes(pageName)) {
    if (state.hasToken && restoreError) {
      refreshShell();
      app.innerHTML = errorBox(restoreError, '暂时无法恢复登录');
      app.setAttribute('aria-busy', 'false');
      return;
    }
    goLogin(); return;
  }
  if (pageName === 'login' && state.user) {
    location.replace(safeNext(params.get('next'))); return;
  }
  await render();
}
start();
