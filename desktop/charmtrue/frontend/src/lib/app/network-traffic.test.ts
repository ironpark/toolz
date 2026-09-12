import assert from 'node:assert/strict';
import test from 'node:test';
import { appendTraffic, trafficPath, trafficSummary } from './network-traffic.ts';

test('traffic history rejects missing, stale, duplicate and invalid samples without zero filling', () => {
    const sample = { name: 'eth0', sampledAt: 1000, receivedKbps: 80, sentKbps: 40 };
    const history = appendTraffic({}, [sample], 1000);
    assert.deepEqual(history.eth0, [{ time: 1000, rx: 80, tx: 40 }]);
    assert.deepEqual(appendTraffic(history, [sample, { ...sample, sampledAt: 1005, sentKbps: null }, { ...sample, sampledAt: 1005, receivedKbps: NaN }, { ...sample, sampledAt: 900 }, { ...sample, sampledAt: 1100 }], 1005), history);
    assert.equal(appendTraffic(history, [{ ...sample, sampledAt: 1005, receivedKbps: 0, sentKbps: 0 }], 1005).eth0.length, 2);
    assert.equal(history.eth0.length, 1);
});

test('histories stay isolated by interface and expire after fifteen minutes', () => {
    const history = appendTraffic({ eth0: [{ time: 1, rx: 3, tx: 4 }, { time: 995, rx: 10, tx: 5 }] }, [{ name: 'bridge0', sampledAt: 1000, receivedKbps: 100, sentKbps: 200 }], 1000);
    assert.equal(history.eth0.length, 1);
    assert.equal(history.bridge0[0].rx, 100);
    assert.deepEqual(trafficSummary(history.eth0, 'rx'), { average: 10, peak: 10 });
    assert.deepEqual(trafficSummary([], 'tx'), { average: null, peak: null });
    assert.deepEqual(trafficSummary([{ time: 1, rx: 0, tx: 20 }, { time: 2, rx: 0, tx: 40 }], 'tx'), { average: 30, peak: 40 });
});

test('graph breaks across missing periods and clips points outside the selected window', () => {
    const path = trafficPath([{ time: 90, rx: 50, tx: 20 }, { time: 100, rx: 0, tx: 0 }, { time: 105, rx: 100, tx: 20 }, { time: 150, rx: 50, tx: 40 }], 'rx', 100, 160, 100);
    assert.equal((path.match(/M/g) ?? []).length, 2);
    assert.equal((path.match(/L/g) ?? []).length, 1);
    assert.ok(path.startsWith('M64.00,190.00'));
    assert.ok(!path.includes('NaN'));
});
