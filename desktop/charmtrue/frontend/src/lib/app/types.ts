export type View = 'overview' | 'storage' | 'datasets' | 'services' | 'network' | 'identity' | 'system' | 'settings' | 'diagnostics' | 'presets';
export interface NavigationItem {
    id: View;
    label: string;
    href: string;
}

export const navigationItems: NavigationItem[] = [
    { id: 'overview', label: '개요', href: '/' },
    { id: 'storage', label: '저장소', href: '/storage' },
    { id: 'datasets', label: '데이터셋', href: '/datasets' },
    { id: 'services', label: '서비스', href: '/services' },
    { id: 'network', label: '네트워크', href: '/network' },
    { id: 'identity', label: '계정 및 권한', href: '/identity' },
    { id: 'system', label: '시스템', href: '/system' },
    { id: 'diagnostics', label: '진단 및 해결', href: '/diagnostics' },
    { id: 'presets', label: '설정 프리셋', href: '/presets' },
];

export const pageTitles = {
    ...Object.fromEntries(navigationItems.map(({ id, label }) => [id, label])),
    settings: '앱 설정',
} as Record<View, string>;

export function viewFromPath(pathname: string): View {
    const path = pathname.length > 1 ? pathname.replace(/\/$/, '') : pathname;
    if (path === '/settings') return 'settings';
    return navigationItems.find(({ href }) => href === path)?.id ?? 'overview';
}

export function hrefForView(view: View): string {
    if (view === 'settings') return '/settings';
    return navigationItems.find(({ id }) => id === view)?.href ?? '/';
}
