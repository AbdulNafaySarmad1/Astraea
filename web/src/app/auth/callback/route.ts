import { NextRequest, NextResponse } from 'next/server';
export async function GET(request: NextRequest) {
  const state = request.nextUrl.searchParams.get('state');
  const code = request.nextUrl.searchParams.get('code');
  const savedState = request.cookies.get('oidc_state')?.value;
  const verifier = request.cookies.get('oidc_verifier')?.value;
  if (!state || state !== savedState || !code || !verifier) return NextResponse.redirect(new URL('/auth/error', request.url));
  const issuer = process.env.OIDC_ISSUER;
  const client = process.env.OIDC_CLIENT_ID;
  const publicURL = process.env.PUBLIC_URL;
  if (!issuer || !client || !publicURL) return NextResponse.redirect(new URL('/auth/error', request.url));
  const redirectUri = new URL('/auth/callback', publicURL).toString();
  try {
    const response = await fetch(`${issuer.replace(/\/$/, '')}/protocol/openid-connect/token`, {
      method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({ grant_type: 'authorization_code', client_id: client, code, redirect_uri: redirectUri, code_verifier: verifier }),
      signal: AbortSignal.timeout(8000), cache: 'no-store'
    });
    if (!response.ok) throw new Error('token exchange failed');
    const token: { access_token: string; expires_in: number } = await response.json();
    const out = NextResponse.redirect(new URL('/fleet', request.url));
    out.cookies.set('nocturn_access', token.access_token, { httpOnly: true, secure: process.env.NODE_ENV === 'production', sameSite: 'lax', path: '/', maxAge: Math.min(token.expires_in, 3600) });
    out.cookies.delete('oidc_state'); out.cookies.delete('oidc_verifier');
    return out;
  } catch { return NextResponse.redirect(new URL('/auth/error', request.url)); }
}
