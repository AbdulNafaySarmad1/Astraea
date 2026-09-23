import 'server-only';
import { cookies } from 'next/headers';
import type { components } from './api.generated';

export type Tenant = components['schemas']['Tenant'];
export type Overview = components['schemas']['Overview'];
export type Incident = components['schemas']['Incident'];
export type Action = components['schemas']['Action'];
export type AuditEvent = components['schemas']['AuditEvent'];
export type Notification = components['schemas']['Notification'];
export type MetricSeries = components['schemas']['MetricSeries'];
export type Investigation = components['schemas']['Investigation'];
export type Domain = components['schemas']['Domain'];
export type Connector = components['schemas']['Connector'];

const base = process.env.API_URL || 'http://127.0.0.1:8080';
export async function api<T>(path: string): Promise<{ data?: T; error?: string; status: number }> {
  const cookie = await cookies();
  const token = cookie.get('nocturn_access')?.value;
  if (process.env.APP_MODE !== 'demo' && !token) return { status: 401, error: 'Sign in to view this workspace.' };
  try {
    const response = await fetch(`${base}/v1${path}`, {
      headers: process.env.APP_MODE === 'demo' ? { 'X-Demo-Actor': 'console' } : { Authorization: `Bearer ${token}` },
      cache: 'no-store', signal: AbortSignal.timeout(8000)
    });
    const body = await response.json();
    if (!response.ok) return { status: response.status, error: body.error || 'Request failed.' };
    return { status: response.status, data: body as T };
  } catch { return { status: 503, error: 'Control plane is unavailable. Data may be stale.' }; }
}
