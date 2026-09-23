import Link from 'next/link';
import { Activity, ArrowUpRight, Building2, CircleDashed, PlugZap, ShieldAlert } from 'lucide-react';
import { api, type Tenant } from '@/lib/api';
import { DataState, Kpi, PageHead, SectionTitle, Status, Time } from '@/components/ui';

export default async function Fleet() {
  const result = await api<{items:Tenant[]}>('/tenants'); const items=result.data?.items||[];
  const active=items.filter(x=>x.status==='active').length;
  const onboarding=items.filter(x=>x.status==='onboarding').length;
  const restricted=items.filter(x=>x.kill_switch).length;
  return <><PageHead eyebrow="OPERATIONS / FLEET" title="Fleet overview" description="Customer environments, readiness, and operational attention in one place." action={<Link className="button button-primary" href="/customers/new"><Building2 size={16}/> Add customer</Link>}/>
    <div className="notice-line"><span className="notice-icon"><CircleDashed size={16}/></span><div><strong>Live operational view</strong><span>Health is only current when approved telemetry has been received within the freshness window.</span></div><span className="notice-time">5 MIN FRESHNESS</span></div>
    <div className="kpi-row"><Kpi label="Registered customers" value={items.length} note="Across authorized fleet" icon={Building2}/><Kpi label="Active customers" value={active} note="Monitoring enabled separately" icon={Activity}/><Kpi label="In onboarding" value={onboarding} note="Awaiting required approvals" icon={PlugZap}/><Kpi label="Kill switches engaged" value={restricted} note="Operations held" icon={ShieldAlert}/></div>
    <SectionTitle title="Customer environments" subtitle="Select a customer to inspect component health and operational history." href="/customers" link="Open directory"/>
    {result.error?<DataState error={result.error}/>:items.length===0?<DataState empty title="No customers registered"/>:<div className="table-frame"><table><thead><tr><th>Customer</th><th>State</th><th>Isolation</th><th>Monitoring</th><th>Created</th><th aria-label="Open"/></tr></thead><tbody>{items.map(t=><tr key={t.id}><td><Link className="table-primary" href={`/customers/${t.id}`}>{t.name}</Link><span className="table-secondary">{t.slug}</span></td><td><Status value={t.status}/></td><td className="capitalize">{t.isolation_mode}</td><td><Status value={t.monitoring_approved?'active':'pending'}/></td><td><Time value={t.created_at}/></td><td><Link className="row-link" href={`/customers/${t.id}`} aria-label={`Open ${t.name}`}><ArrowUpRight size={17}/></Link></td></tr>)}</tbody></table></div>}
    <div className="bottom-note"><strong>Operational boundary</strong><span>Customer domains identify onboarding requests. Environment access begins only after authorization, verification, connector enrollment, and approval.</span></div>
  </>;
}
