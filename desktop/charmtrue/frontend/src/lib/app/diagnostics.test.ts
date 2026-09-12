import assert from 'node:assert/strict';
import test from 'node:test';
import { buildDiagnosticChecks, canTuneDataset, type DiagnosticResults } from './diagnostics.ts';

const unavailable = () => Array.from({ length: 5 }, () => ({ status: 'rejected', reason: new Error('permission denied') })) as DiagnosticResults;

test('unavailable sources are never reported as healthy', () => {
    const checks = buildDiagnosticChecks(unavailable());
    assert.equal(checks.length, 5);
    assert.ok(checks.every(check => check.status === 'unknown' && !check.action));
});

test('pool thresholds and eligible tuning are detected without altering existing compression', () => {
    const results = unavailable();
    results[0] = { status: 'fulfilled', value: {
        pools: [{ id: 1, name: 'tank', healthy: true, status: 'ONLINE', size: 100, allocated: 90 }], snapshotCount: 0,
        datasets: [
            { id: 'tank/data', type: 'FILESYSTEM', locked: false, compression: 'off', sync: 'disabled' },
            { id: 'tank/archive', type: 'FILESYSTEM', locked: false, compression: 'zstd', sync: 'standard' },
            { id: 'tank/.system', type: 'FILESYSTEM', locked: false, compression: 'off', sync: 'disabled' },
            { id: 'tank/locked', type: 'FILESYSTEM', locked: true, compression: 'off', sync: 'disabled' },
        ],
    } } as DiagnosticResults[0];
    const checks = buildDiagnosticChecks(results);
    assert.equal(checks.find(check => check.id === 'space-1')?.status, 'critical');
    assert.equal(checks.find(check => check.id === 'snapshots')?.status, 'warning');
    assert.deepEqual(checks.filter(check => check.action).map(check => [check.target, check.action]), [['tank/data', 'compression_lz4'], ['tank/data', 'sync_standard']]);
});

test('system datasets and pool roots cannot be tuning candidates', () => {
    for (const id of ['tank', 'tank/.system/data', 'tank/ix-apps/data', 'tank/ix-applications', 'tank//data', 'tank/data@snap']) assert.equal(canTuneDataset(id), false, id);
    assert.equal(canTuneDataset('tank/user data'), true);
});
