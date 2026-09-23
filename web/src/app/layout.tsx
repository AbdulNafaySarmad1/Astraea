import type { Metadata } from 'next';
import Link from 'next/link';
import Image from 'next/image';
import { Activity, BellRing, BookOpenText, Building2, ClipboardCheck, Clock3, LayoutDashboard, LockKeyhole, ScrollText, Settings2, ShieldCheck, ScanSearch, LogOut } from 'lucide-react';
import './styles.css';

export const metadata: Metadata = {
  title: 'Nocturn Operations',
  description: 'AegisCore infrastructure operations',
  icons: {
    icon: [{ url: '/brand/nocturn-mark.png', type: 'image/png', sizes: '1254x1254' }],
    apple: [{ url: '/brand/nocturn-mark.png', type: 'image/png', sizes: '1254x1254' }],
  },
};
const nav = [
  { href: '/fleet', label: 'Fleet overview', icon: LayoutDashboard },
  { href: '/customers', label: 'Customers', icon: Building2 },
  { href: '/incidents', label: 'Incidents', icon: Activity },
  { href: '/investigations', label: 'Investigations', icon: ScanSearch },
  { href: '/approvals', label: 'Approvals', icon: ClipboardCheck },
  { href: '/audit', label: 'Audit trail', icon: ScrollText },
  { href: '/settings', label: 'Settings', icon: Settings2 },
];
export default function RootLayout({ children }: Readonly<{ children: React.ReactNode }>) {
  return <html lang="en"><body><div className="shell">
    <aside className="sidebar" aria-label="Primary navigation">
      <Link className="brand" href="/fleet"><span className="brand-mark"><Image className="brand-logo" src="/brand/nocturn-mark.png" width={38} height={38} alt="" priority /></span><span><strong>NOCTURN</strong><small>OPERATIONS</small></span></Link>
      <div className="workspace-label">WORKSPACE <span>01</span></div>
      <nav className="side-nav">{nav.map(({href,label,icon: Icon}) => <Link key={href} href={href}><Icon size={17} strokeWidth={1.8}/>{label}</Link>)}</nav>
      <div className="sidebar-bottom"><div className="sidebar-rule"/><a href="/settings#knowledge"><BookOpenText size={17}/> Runbooks & knowledge</a><div className="support"><ShieldCheck size={15}/><span>Policy gated operations</span></div></div>
    </aside>
    <div className="main-wrap"><header className="topbar"><div className="topbar-left"><span className="live-dot"/> CONTROL PLANE <span className="topbar-divider"/> <span className="topbar-muted">AegisCore / Operations</span></div><div className="topbar-right"><span className="utc"><Clock3 size={15}/> All times UTC</span><Link href="/settings#notifications" aria-label="Notification settings"><BellRing size={18}/></Link>{process.env.APP_MODE!=='demo'&&<Link href="/auth/logout" aria-label="Sign out"><LogOut size={18}/></Link>}<span className="avatar">NO</span></div></header>{process.env.APP_MODE==='demo'&&<div className="demo-ribbon">FICTIONAL DEMO DATA <span>·</span> No customer infrastructure or email provider is connected</div>}<main id="main-content">{children}</main><footer className="footer"><span>NOCTURN SYSTEMS <span className="footer-sep">/</span> CONTROL PLANE</span><span><LockKeyhole size={13}/> Access and operations are audited</span></footer></div>
  </div></body></html>;
}
