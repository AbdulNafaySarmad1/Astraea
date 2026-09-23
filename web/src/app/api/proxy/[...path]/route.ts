import { NextRequest, NextResponse } from 'next/server';
async function forward(request: NextRequest, context: { params: Promise<{ path: string[] }> }) {
  const origin = request.headers.get('origin');
  if (request.method !== 'GET' && origin !== request.nextUrl.origin) return NextResponse.json({ error: 'Invalid request origin' }, { status: 403 });
  const token = request.cookies.get('nocturn_access')?.value;
  if (process.env.APP_MODE !== 'demo' && !token) return NextResponse.json({ error: 'Authentication required' }, { status: 401 });
  const { path } = await context.params;
  if (path.some(segment => segment === '..' || segment.includes('/'))) return NextResponse.json({ error: 'Invalid path' }, { status: 400 });
  const base = process.env.API_URL || 'http://127.0.0.1:8080';
  const url = `${base}/v1/${path.map(encodeURIComponent).join('/')}${request.nextUrl.search}`;
  const headers: Record<string, string> = process.env.APP_MODE === 'demo' ? { 'X-Demo-Actor': 'console' } : { Authorization: `Bearer ${token}` };
  if (request.method !== 'GET') headers['Content-Type'] = 'application/json';
  try {
    const response = await fetch(url, { method: request.method, headers, body: request.method === 'GET' ? undefined : await request.text(), cache: 'no-store', signal: AbortSignal.timeout(10000) });
    return new NextResponse(await response.text(), { status: response.status, headers: { 'Content-Type': 'application/json', 'Cache-Control': 'no-store' } });
  } catch { return NextResponse.json({ error: 'Control plane unavailable' }, { status: 503 }); }
}
export { forward as GET, forward as POST, forward as PUT };
