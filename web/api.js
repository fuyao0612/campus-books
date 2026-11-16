// 所有业务请求集中在本文件；页面通过 api 对象访问后端。
const TOKEN_KEY = 'campus-books-token';
let token = sessionStorage.getItem(TOKEN_KEY) || '';
let user = null;

export const state = {
  get user() { return user; },
  get hasToken() { return Boolean(token); },
};

// 保留状态码，让页面能区分登录失效、业务冲突和服务不可用。
export class ApiError extends Error {
  constructor(message, status = 0) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

export function logout() {
  token = '';
  user = null;
  sessionStorage.removeItem(TOKEN_KEY);
}

export async function request(path, { method = 'GET', body, query, auth = true } = {}) {
  const url = new URL('/api' + path, location.origin);
  for (const [key, value] of Object.entries(query || {})) {
    if (value !== undefined && value !== null && value !== '') url.searchParams.set(key, value);
  }
  const headers = { Accept: 'application/json' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';
  if (auth && token) headers.Authorization = `Bearer ${token}`;

  // 超时覆盖读取响应体的过程；finally 保证成功或失败都会清理计时器。
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 15000);
  let response;
  let payload;
  try {
    response = await fetch(url, {
      method, headers, signal: controller.signal,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    const text = await response.text();
    try { payload = text ? JSON.parse(text) : {}; } catch { payload = null; }
  } catch (error) {
    throw new ApiError(error.name === 'AbortError'
      ? '请求超时，请稍后重试。'
      : '暂时无法连接服务，请稍后重试。');
  } finally {
    clearTimeout(timeout);
  }

  if (!response.ok) {
    let message = payload?.message;
    // 未实现的路由可能返回文本 404，不能向用户展示 JSON 解析错误。
    if (!message) message = [404, 501].includes(response.status)
      ? '此功能暂不可用，请稍后再试。' : '操作未完成，请稍后重试。';
    if (response.status === 401 && auth) {
      logout();
      window.dispatchEvent(new Event('session-expired'));
      message = '登录已失效，请重新登录。';
    }
    throw new ApiError(message, response.status);
  }
  if (payload === null) throw new ApiError('服务返回的数据暂时无法读取，请重试。');
  return Object.hasOwn(payload, 'data') ? payload.data : payload;
}

export const api = {
  async login(name, password) {
    const result = await request('/login', { method: 'POST', body: { name, password }, auth: false });
    if (!result?.token || !result?.user?.id) throw new ApiError('登录信息不完整，请重试。');
    token = result.token;
    user = result.user;
    sessionStorage.setItem(TOKEN_KEY, token);
    return user;
  },
  async me() { user = await request('/me'); return user; },
  // 网络故障保留令牌，便于重试；只有 401 才清除登录状态。
  async restoreSession() { return token ? this.me() : null; },
  books(keyword = '') { return request('/books', { query: { keyword }, auth: false }); },
  detail(id) { return request(`/books/${encodeURIComponent(id)}`, { auth: false }); },
  mine(status = '') { return request('/me/books', { query: { status } }); },
  create(fields) { return request('/books', { method: 'POST', body: fields }); },
  update(id, fields) { return request(`/books/${encodeURIComponent(id)}`, { method: 'PUT', body: fields }); },
  remove(id) { return request(`/books/${encodeURIComponent(id)}/status`, { method: 'PATCH', body: { status: 'off_shelf' } }); },
  comments(id) { return request(`/books/${encodeURIComponent(id)}/comments`, { auth: false }); },
  comment(id, content) { return request(`/books/${encodeURIComponent(id)}/comments`, { method: 'POST', body: { content } }); },
  deleteComment(id) { return request(`/comments/${encodeURIComponent(id)}`, { method: 'DELETE' }); },
  order(bookId) { return request('/orders', { method: 'POST', body: { book_id: Number(bookId) } }); },
  orders(role = 'buy', status = '') { return request('/orders', { query: { role, status } }); },
  getOrder(id) { return request(`/orders/${encodeURIComponent(id)}`); },
  cancel(id) { return request(`/orders/${encodeURIComponent(id)}/cancel`, { method: 'POST', body: {} }); },
  pay(id) { return request(`/orders/${encodeURIComponent(id)}/pay`, { method: 'POST', body: {} }); },
  complete(id) { return request(`/orders/${encodeURIComponent(id)}/complete`, { method: 'POST', body: {} }); },
};
