import Link from 'next/link';
import { Building2, ChevronRight } from 'lucide-react';
import { api, type Tenant } from '@/lib/api';
export async function TenantScope({section}:{section:'incidents'|'approvals'|'audit'|'settings'|'investigations'}){const response=await api<{items:Tenant[]}>('/tenants');return <div className="scope-list"><div className="scope-heading"><Building2 size={18}/><strong>Choose a customer environment</strong><span>Customer data is viewed within its own tenant boundary.</span></div>{response.data?.items.map(t=><Link key={t.id} href={`/${section}?tenant=${t.id}`}><span>{t.name}</span><ChevronRight size={17}/></Link>)}</div>}
