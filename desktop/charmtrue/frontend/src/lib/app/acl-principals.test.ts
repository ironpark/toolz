import assert from 'node:assert/strict';
import test from 'node:test';
import type { GroupInfo, UserInfo } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
import { filterPrincipals, principalCandidates, specialPrincipalHelp } from './acl-principals.ts';

const group = (name: string, gid: number, builtin = false, local = true): GroupInfo => ({ id: gid, name, gid, builtin, local, userCount: 0, smb: false });

test('system candidates hidden by default and searchable when explicitly enabled', () => {
    const values = principalCandidates([], [group('family', 1000), group('daemon', 1, true), group('apps', 568), group('builtin_users', 545, true)], 'GROUP');
    assert.deepEqual(filterPrincipals(values, '', false).map(item => item.name), ['family']);
    assert.equal(filterPrincipals(values, '', true).length, 4);
    assert.equal(filterPrincipals(values, '568', false).length, 0);
    assert.equal(filterPrincipals(values, '568', true)[0].name, 'apps');
    assert.match(filterPrincipals(values, 'SMB', true)[0].description, /SMB/);
});

test('custom low IDs and directory identities are not mistaken for local system accounts', () => {
    const values = principalCandidates([], [group('family', 501), group('apps', 1500), group('root', 0, false, false)], 'GROUP');
    assert.equal(filterPrincipals(values, '', false).length, 3);
    assert.equal(values[1].description, '');
    assert.match(values[2].description, /디렉터리/);
});

test('user descriptions use full names and filters never mutate source identities', () => {
    const user = { uid: 1000, username: 'alex', fullName: '가족 관리자', local: true, builtin: false } as UserInfo;
    const before = JSON.stringify(user);
    const values = principalCandidates([user], [], 'USER');
    assert.equal(filterPrincipals(values, ' 가족 ', false)[0].name, 'alex');
    assert.equal(JSON.stringify(user), before);
});

test('special target explanations distinguish POSIX others from NFS everyone', () => {
    assert.match(specialPrincipalHelp.OTHER, /모든 사용자를 뜻하지/);
    assert.match(specialPrincipalHelp['everyone@'], /포함한 모든 사용자/);
    for (const tag of ['owner@', 'group@', 'USER_OBJ', 'GROUP_OBJ', 'MASK']) assert.ok(specialPrincipalHelp[tag]);
});
