export function formatBytes(bytes: number | undefined): string {
    if (!bytes || !Number.isFinite(bytes) || bytes <= 0) return '—';
    const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
    const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
    const value = bytes / 1024 ** exponent;
    return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[exponent]}`;
}

export function formatUptime(seconds: number | null | undefined): string {
    if (seconds == null || !Number.isFinite(seconds) || seconds < 0) return '—';
    const whole = Math.floor(seconds);
    const days = Math.floor(whole / 86400);
    const hours = Math.floor(whole % 86400 / 3600);
    const minutes = Math.floor(whole % 3600 / 60);
    const remaining = whole % 60;
    return `${days ? `${days}일 ` : ''}${hours}시간 ${minutes}분 ${remaining}초`;
}

export function formatTraffic(kbps: number | null | undefined): string {
    if (kbps == null || !Number.isFinite(kbps) || kbps < 0) return '—';
    if (kbps >= 1_000_000) return `${(kbps / 1_000_000).toFixed(1)} Gbps`;
    if (kbps >= 1_000) return `${(kbps / 1_000).toFixed(1)} Mbps`;
    return `${Math.round(kbps)} Kbps`;
}
