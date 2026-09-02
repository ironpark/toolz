export function formatBytes(bytes: number | undefined): string {
    if (!bytes || !Number.isFinite(bytes) || bytes <= 0) return '—';
    const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
    const exponent = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
    const value = bytes / 1024 ** exponent;
    return `${value.toFixed(value >= 10 ? 0 : 1)} ${units[exponent]}`;
}
