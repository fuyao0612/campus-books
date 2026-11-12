// 所有网络请求集中在这里。接口路径与字段约定见 README.md。
const TOKEN_KEY='campus-books-token';
let token=sessionStorage.getItem(TOKEN_KEY)||'';
let user=null;
export const state={get user(){return user;}};

export function logout(){token='';user=null;sessionStorage.removeItem(TOKEN_KEY);}

export async function request(path,{method='GET',body,query,auth=true}={}){
  const url=new URL('/api'+path,location.origin);
  if(query)for(const [key,value] of Object.entries(query))if(value!==undefined&&value!==null&&value!=='')url.searchParams.set(key,value);
  const headers={Accept:'application/json'};
  if(body!==undefined)headers['Content-Type']='application/json';
  if(auth&&token)headers.Authorization=`Bearer ${token}`;
  let response;
  try{response=await fetch(url,{method,headers,body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(20000)});}
  catch(error){if(error.name==='TimeoutError')throw new Error('请求超时，请检查服务状态后重试。');throw new Error('无法连接 Go 服务，请确认后端已启动。');}
  const payload=await response.json().catch(()=>({message:'服务器响应格式不正确'}));
  if(response.status===401&&auth)logout();
  if(!response.ok)throw new Error(payload.message||`请求失败（${response.status}）`);
  return Object.hasOwn(payload,'data')?payload.data:payload;
}

export const api={
  async login(name,password){
    const result=await request('/login',{method:'POST',body:{name,password},auth:false});
    if(!result?.token||!result?.user)throw new Error('登录接口响应缺少 token 或 user 字段。');
    token=result.token;user=result.user;sessionStorage.setItem(TOKEN_KEY,token);return user;
  },
  async me(){user=await request('/me');return user;},
  async restoreSession(){if(!token)return null;try{return await this.me();}catch{logout();return null;}},
  books(keyword=''){return request('/books',{query:{keyword}});},
  detail(id){return request(`/books/${encodeURIComponent(id)}`);},
  mine(status=''){return request('/me/books',{query:{status}});},
  create(fields){return request('/books',{method:'POST',body:fields});},
  update(id,fields){return request(`/books/${encodeURIComponent(id)}`,{method:'PUT',body:fields});},
  remove(id){return request(`/books/${encodeURIComponent(id)}/status`,{method:'PATCH',body:{status:'off_shelf'}});},
  comments(bookId){return request(`/books/${encodeURIComponent(bookId)}/comments`);},
  comment(bookId,content){return request(`/books/${encodeURIComponent(bookId)}/comments`,{method:'POST',body:{content}});},
  deleteComment(commentId){return request(`/comments/${encodeURIComponent(commentId)}`,{method:'DELETE'});},
  order(bookId){return request('/orders',{method:'POST',body:{book_id:Number(bookId)}});},
  orders(role='buy',status=''){return request('/orders',{query:{role,status}});},
  getOrder(id){return request(`/orders/${encodeURIComponent(id)}`);},
  cancel(id){return request(`/orders/${encodeURIComponent(id)}/cancel`,{method:'POST',body:{}});},
  pay(id){return request(`/orders/${encodeURIComponent(id)}/pay`,{method:'POST',body:{}});},
  complete(id){return request(`/orders/${encodeURIComponent(id)}/complete`,{method:'POST',body:{}});},
};
