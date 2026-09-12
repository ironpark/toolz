import assert from 'node:assert/strict';
import test from 'node:test';
import { formatTraffic, formatUptime } from './format.ts';

test('uptime floors fractional seconds and rolls over each unit', () => {
    assert.equal(formatUptime(0), '0시간 0분 0초');
    assert.equal(formatUptime(59.999), '0시간 0분 59초');
    assert.equal(formatUptime(60), '0시간 1분 0초');
    assert.equal(formatUptime(86400 + 3661.75), '1일 1시간 1분 1초');
    assert.equal(formatUptime(null), '—');
    assert.equal(formatUptime(NaN), '—');
});

test('traffic preserves zero and distinguishes unavailable readings', () => {
    assert.equal(formatTraffic(0), '0 Kbps');
    assert.equal(formatTraffic(null), '—');
    assert.equal(formatTraffic(1500), '1.5 Mbps');
    assert.equal(formatTraffic(1_500_000), '1.5 Gbps');
});
