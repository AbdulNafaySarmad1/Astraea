import { NextRequest, NextResponse } from 'next/server';
import { createHash, randomBytes } from 'node:crypto';

export async function GET(request: NextRequest) {
  if (process.env.APP_MODE === 'demo') return NextResponse.redirect(new URL('/fleet', request.url));
  const issuer = process.env.OIDC_ISSUER;
  const client = process.env.OIDC_CLIENT_ID;
  const publicURL = process.env.PUBLIC_URL;
  if (!issuer || !client || !publicURL) return NextResponse.json({ error: 'OIDC is not configured' }, { status: 503 });
  const state = randomBytes(24).toString('base64url');
  const verifier = randomBytes(32).toString('base64url');
  const challenge = createHash('sha256').update(verifier).digest('base64url');
  const redirectUri = new URL('/auth/callback', publicURL).toString();
  const url = new URL(`${issuer.replace(/\/$/, '')}/protocol/openid-connect/auth`);
  url.searchParams.set('client_id', client); url.searchParams.set('redirect_uri', redirectUri);
  url.searchParams.set('response_type', 'code'); url.searchParams.set('scope', 'openid profile');
  url.searchParams.set('state', state); url.searchParams.set('code_challenge', challenge);
  url.searchParams.set('code_challenge_method', 'S256');
  const response = NextResponse.redirect(url);
  const options = { httpOnly: true, secure: process.env.NODE_ENV === 'production', sameSite: 'lax' as const, path: '/', maxAge: 600 };
  response.cookies.set('oidc_state', state, options); response.cookies.set('oidc_verifier', verifier, options);
  return response;
}
