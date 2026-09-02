export type View = 'overview' | 'storage' | 'datasets' | 'services' | 'network' | 'identity' | 'system';
export type AuthenticationMethod = 'api_key' | 'password';

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
];

export const pageTitles: Record<View, string> = Object.fromEntries(
    navigationItems.map(({ id, label }) => [id, label]),
) as Record<View, string>;

export function viewFromPath(pathname: string): View {
    const path = pathname.length > 1 ? pathname.replace(/\/$/, '') : pathname;
    return navigationItems.find(({ href }) => href === path)?.id ?? 'overview';
}

export function hrefForView(view: View): string {
    return navigationItems.find(({ id }) => id === view)?.href ?? '/';
}
