import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import test from 'node:test';
import vm from 'node:vm';
import ts from 'typescript';

// Exercise asynchronous connection state without a live Wails server.
// Reactivity is not under test; the state shim retains ordinary field values.
function contextWith(api) {
    const source = readFileSync(new URL('../src/lib/app/context.svelte.ts', import.meta.url), 'utf8');
    const code = ts.transpileModule(source, {
        compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    }).outputText;
    const sandbox = {
        exports: {},
        $state: value => value,
        require: name => name === 'svelte' ? { getContext() {}, setContext() {} } : { TrueNASService: api },
    };
    vm.runInNewContext(code, sandbox);
    return new sandbox.exports.AppContext();
}

for (const mode of ['new', 'saved']) {
    test(`${mode} server closes immediately while background reads are pending`, async () => {
        const connection = { connected: true, endpoint: 'nas.local' };
        const pending = () => new Promise(() => {});
        const api = { Connect: async () => connection, ConnectSavedServer: async () => connection };
        for (const method of ['SavedServers', 'StorageOverview', 'SharingOverview', 'SystemManagementOverview', 'IdentityOverview', 'NetworkOverview', 'CertificateOverview']) api[method] = pending;
        const app = contextWith(api);
        app.modalOpen = true;
        const completion = mode === 'new'
            ? app.connect('nas.local', 'admin', 'test-password', true, true, true)
            : app.connectSavedServer('test-server');
        const result = await Promise.race([completion, new Promise(resolve => setTimeout(() => resolve('timeout'), 100))]);
        assert.equal(result, true);
        assert.equal(app.modalOpen, false);
        assert.equal(app.loading, false);
        assert.equal(app.connection.endpoint, 'nas.local');
        assert.equal(app.storageLoading, true);
    });
}

test('failed connection keeps the dialog open for correction', async () => {
    const app = contextWith({ Connect: async () => { throw new Error('Authentication failed'); } });
    app.modalOpen = true;
    assert.equal(await app.connect('nas.local', 'admin', 'wrong-password', true, true, true), false);
    assert.equal(app.modalOpen, true);
    assert.equal(app.loading, false);
    assert.ok(app.message);
});
