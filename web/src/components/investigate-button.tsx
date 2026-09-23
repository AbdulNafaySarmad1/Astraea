'use client';
import { useState } from 'react';
import { ScanSearch } from 'lucide-react';
export function InvestigateButton({tenant}:{tenant:string}){const [state,setState]=useState('');async function start(){setState('Queueing…');try{const response=await fetch(`/api/proxy/tenants/${tenant}/investigations`,{method:'POST',headers:{'Content-Type':'application/json'},body:'{}'});const result=await response.json();if(!response.ok)throw new Error(result.error);setState('Queued');window.location.reload()}catch(e){setState(e instanceof Error?e.message:'Failed')}}return <button className="button button-primary" onClick={start} disabled={!!state}><ScanSearch size={16}/>{state||'Start investigation'}</button>}
