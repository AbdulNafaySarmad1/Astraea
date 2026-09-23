import { NextRequest, NextResponse } from 'next/server';
export async function GET(request: NextRequest) {
  const issuer=process.env.OIDC_ISSUER;
  const client=process.env.OIDC_CLIENT_ID;
  const publicURL=process.env.PUBLIC_URL;
  const destination=issuer&&client&&publicURL?new URL(`${issuer.replace(/\/$/,'')}/protocol/openid-connect/logout`):new URL('/auth/login',request.url);
  if(issuer&&client&&publicURL){destination.searchParams.set('client_id',client);destination.searchParams.set('post_logout_redirect_uri',new URL('/auth/login',publicURL).toString())}
  const response = NextResponse.redirect(destination);
  response.cookies.delete('nocturn_access'); response.cookies.delete('oidc_state'); response.cookies.delete('oidc_verifier');
  return response;
}
