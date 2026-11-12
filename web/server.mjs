// 仅用于本地学习：提供静态页面，并将 /api 原样转发给 Go 服务。
// 不依赖 npm 包，不修改 Go 的接口或跨域配置。
import http from 'node:http';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const port = Number(process.env.WEB_PORT || 5174);
const backend = new URL('http://127.0.0.1:8080');
const files = new Map([
  ['/', ['index.html', 'text/html; charset=utf-8']],
  ['/index.html', ['index.html', 'text/html; charset=utf-8']],
  ['/app.js', ['app.js', 'text/javascript; charset=utf-8']],
  ['/api.js', ['api.js', 'text/javascript; charset=utf-8']],
  ['/style.css', ['style.css', 'text/css; charset=utf-8']],
]);

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url, 'http://127.0.0.1');
  if (url.pathname.startsWith('/api/')) {
    const headers = { ...req.headers, host: backend.host };
    delete headers.connection;
    const proxy = http.request({ hostname: backend.hostname, port: backend.port, method: req.method, path: url.pathname + url.search, headers }, (upstream) => {
      res.writeHead(upstream.statusCode, upstream.headers);
      upstream.pipe(res);
      upstream.on('error', () => res.destroy());
    });
    proxy.on('error', () => {
      if (res.headersSent) { res.destroy(); return; }
      res.writeHead(502, { 'Content-Type': 'application/json; charset=utf-8' });
      res.end(JSON.stringify({ message: '无法连接 Go 后端，请先在项目根目录运行 go run .' }));
    });
    proxy.setTimeout(15000, () => proxy.destroy());
    req.on('aborted', () => proxy.destroy());
    res.on('close', () => { if (!res.writableEnded) proxy.destroy(); });
    req.pipe(proxy);
    return;
  }
  if (!['GET', 'HEAD'].includes(req.method)) { res.writeHead(405); res.end(); return; }
  const file = files.get(url.pathname);
  if (!file) { res.writeHead(404); res.end('Not found'); return; }
  try {
    const content = await readFile(fileURLToPath(new URL(file[0], import.meta.url)));
    res.writeHead(200, { 'Content-Type': file[1], 'Cache-Control': 'no-store' });
    res.end(req.method === 'HEAD' ? undefined : content);
  } catch {
    res.writeHead(500); res.end('Unable to read frontend file');
  }
});
server.on('error', (error) => { console.error(`前端启动失败：${error.message}`); process.exitCode = 1; });
server.listen(port, '127.0.0.1', () => console.log(`校园书集：http://127.0.0.1:${port}\n后端转发地址：${backend.origin}\n按 Ctrl+C 停止`));
