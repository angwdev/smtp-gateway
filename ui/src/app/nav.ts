import HistoryIcon from '@mui/icons-material/HistoryOutlined';
import KeyIcon from '@mui/icons-material/KeyOutlined';
import MailIcon from '@mui/icons-material/MailOutlined';
import MoreIcon from '@mui/icons-material/MoreHorizOutlined';
import OverviewIcon from '@mui/icons-material/SpaceDashboardOutlined';
import TokenIcon from '@mui/icons-material/VpnKeyOutlined';
import type { SvgIconComponent } from '@mui/icons-material';

export interface NavItem { to: string; label: string; icon: SvgIconComponent; where: 'both' | 'rail' | 'tabs' }

export const NAV: NavItem[] = [
  { to: '/', label: 'Overview', icon: OverviewIcon, where: 'both' },
  { to: '/logins', label: 'Logins', icon: KeyIcon, where: 'both' },
  { to: '/messages', label: 'Messages', icon: MailIcon, where: 'both' },
  { to: '/audit', label: 'Audit', icon: HistoryIcon, where: 'both' },
  { to: '/tokens', label: 'API tokens', icon: TokenIcon, where: 'rail' },
  { to: '/more', label: 'More', icon: MoreIcon, where: 'tabs' },
];

// "/" is active only on itself; every other item is active on its subtree.
// The More tab also owns /tokens, which has no tab of its own.
export function isActive(item: NavItem, pathname: string): boolean {
  if (item.to === '/') return pathname === '/';
  if (item.to === '/more' && pathname.startsWith('/tokens')) return true;
  return pathname === item.to || pathname.startsWith(`${item.to}/`);
}
