import type { InterfaceTraffic } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';

export interface TrafficPoint { time: number; rx: number; tx: number }
export type TrafficHistory = Record<string, TrafficPoint[]>;
export function appendTraffic(history: TrafficHistory, samples: InterfaceTraffic[], now: number): TrafficHistory {
    const next: TrafficHistory = Object.fromEntries(Object.entries(history).map(([name, points]) => [name, points.filter(point => point.time >= now - 900)]));
    for (const sample of samples) {
        const { sampledAt: time, receivedKbps: rx, sentKbps: tx } = sample;
        if (time < now - 30 || time > now + 5 || rx == null || tx == null || !Number.isFinite(rx) || !Number.isFinite(tx) || rx < 0 || tx < 0) continue;
        const points = next[sample.name] ?? [];
        if (points.length && time <= points[points.length - 1].time) continue;
        next[sample.name] = [...points, { time, rx, tx }].slice(-900);
    }
    return next;
}
export function trafficSummary(points: TrafficPoint[], direction: 'rx' | 'tx'): { average: number | null; peak: number | null } {
    if (!points.length) return { average: null, peak: null };
    return { average: points.reduce((total, point) => total + point[direction], 0) / points.length, peak: Math.max(...points.map(point => point[direction])) };
}
export function trafficPath(points: TrafficPoint[], direction: 'rx' | 'tx', start: number, end: number, ceiling: number): string {
    let previous = 0;
    return points.filter(point => point.time >= start && point.time <= end).map(point => {
        const command = !previous || point.time - previous > 30 ? 'M' : 'L';
        previous = point.time;
        return `${command}${(64 + (point.time - start) / Math.max(1, end - start) * 716).toFixed(2)},${(190 - point[direction] / Math.max(1, ceiling) * 166).toFixed(2)}`;
    }).join(' ');
}
