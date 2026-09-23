import Link from 'next/link';
import { ArrowLeft } from 'lucide-react';
import { api, type Overview, type Domain, type Connector } from '@/lib/api';
import { DataState, PageHead } from '@/components/ui';
import { OnboardingControls } from '@/components/onboarding-controls';
export default async function Onboarding({params}:{params:Promise<{id:string}>}){const {id}=await params;const [result,domains,connectors]=await Promise.all([api<Overview>(`/tenants/${id}/overview`),api<{items:Domain[]}>(`/tenants/${id}/domains`),api<{items:Connector[]}>(`/tenants/${id}/connectors`)]);return <><Link className="backlink" href={`/customers/${id}`}><ArrowLeft size={15}/> Customer overview</Link><PageHead eyebrow="CUSTOMERS / ONBOARDING" title={`${result.data?.name||'Customer'} setup`} description="Domain control, connector identity, and explicit monitoring approval."/>{result.error?<DataState error={result.error}/>:<OnboardingControls tenant={id} overview={result.data!} initialDomains={domains.data?.items||[]} initialConnectors={connectors.data?.items||[]}/>}</>}
