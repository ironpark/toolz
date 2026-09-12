import assert from 'node:assert/strict';
import test from 'node:test';
import { cleanPreset, readPresets, serializePresets } from './setup-presets.ts';

test('preset persistence allowlists non-secret settings and strips credentials at every level', () => {
    const p = { id: '1', name: ' Initial ', groups: [' family '], users: [{ name: ' alex ', fullName: '', group: 'family', smb: true, password: 'secret' }], enableSSH: true, adminUser: '', replaceAdminUser: 'new_admin', replacementPassword: 'secret', adminPassword: 'secret', passwords: { alex: 'secret' } };
    const text = serializePresets([p]);
    assert.equal(text.includes('secret'), false);
    assert.equal(text.includes('password'), false);
    assert.equal(readPresets(text)[0].name, 'Initial');
    assert.deepEqual(readPresets(text)[0].groups, ['family']);
    assert.equal(p.name, ' Initial ');
    assert.notEqual(cleanPreset(p).users, p.users);
});
test('invalid stored presets are rejected instead of silently erased', () => {
    assert.deepEqual(readPresets(null), []);
    assert.throws(() => readPresets('{'));
    assert.throws(() => readPresets('{}'));
    assert.throws(() => readPresets('[{"id":"bad"}]'));
});
