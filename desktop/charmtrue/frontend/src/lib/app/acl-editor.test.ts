import assert from 'node:assert/strict';
import test from 'node:test';
import type { FilesystemACLEntry } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
import { aclChanges, aclRole, aclWarnings, addACLGrant, cloneACL, stableACL, withACLRole, type EditableACL } from './acl-editor.ts';

function entry(tag: string, permissions = { READ: true, WRITE: false, EXECUTE: true }): FilesystemACLEntry {
    return { tag, type: '', id: 0, hasId: false, who: '', basicPerms: '', permissions, basicFlags: '', flags: {}, default: false };
}
function posix(): EditableACL {
    return { path: '/mnt/tank/data', aclType: 'POSIX1E', user: 'root', group: 'wheel', uid: 0, gid: 0, trivial: true, nfs41Flags: {}, entries: [entry('USER_OBJ', { READ: true, WRITE: true, EXECUTE: true }), entry('GROUP_OBJ'), entry('OTHER', { READ: false, WRITE: false, EXECUTE: false })] };
}
const family = { tag: 'GROUP' as const, name: 'family', id: 1000 };

test('POSIX no-access is recognized and clears read, write and traverse without changing scope', () => {
    const source = { ...entry('GROUP_OBJ'), default: true };
    const none = withACLRole(source, 'POSIX1E', 'NONE');
    assert.deepEqual(none.permissions, { READ: false, WRITE: false, EXECUTE: false });
    assert.equal(none.default, true);
    assert.equal(aclRole(none, 'POSIX1E'), 'NONE');
    assert.equal(aclRole(posix().entries[2], 'POSIX1E'), 'NONE');
    assert.equal(aclRole(withACLRole(none, 'POSIX1E', 'READ'), 'POSIX1E'), 'READ');
    assert.equal(source.permissions?.READ, true);
});

test('NFS no-access sends explicit false bits, not an unsupported BASIC preset or a DENY', () => {
    const source = { ...entry('group@'), type: 'ALLOW', basicPerms: 'MODIFY', basicFlags: 'INHERIT' };
    const none = withACLRole(source, 'NFS4', 'NONE');
    assert.equal(none.basicPerms, ''); assert.equal(none.type, 'ALLOW');
    assert.equal(none.basicFlags, 'INHERIT');
    assert.equal(Object.keys(none.permissions ?? {}).length, 14);
    assert.ok(Object.values(none.permissions ?? {}).every(value => value === false));
    assert.equal(aclRole(none, 'NFS4'), 'NONE');
    assert.equal(aclRole({ ...none, type: 'DENY' }, 'NFS4'), 'CUSTOM');
    assert.equal(aclRole({ ...none, basicFlags: '', flags: { INHERITED: true } }, 'NFS4'), 'CUSTOM');
    assert.equal(aclRole(withACLRole(none, 'NFS4', 'READ'), 'NFS4'), 'READ');
});

test('adding no-access keeps existing rules and warns about access granted elsewhere', () => {
    const original = posix();
    const after = addACLGrant(original, family, 'NONE', 'inherit');
    assert.deepEqual(after.entries.slice(0, 3), original.entries);
    assert.ok(after.entries.filter(item => item.tag === 'GROUP').every(item => aclRole(item, 'POSIX1E') === 'NONE'));
    assert.ok(aclWarnings(after).some(warning => warning.includes('접근 없음')));
});

test('POSIX read allows directory traversal and has no full-control alias', () => {
    const acl = addACLGrant(posix(), family, 'READ', 'folder');
    const grant = acl.entries.find(item => item.tag === 'GROUP')!;
    assert.deepEqual(grant.permissions, { READ: true, WRITE: false, EXECUTE: true });
    assert.equal(aclRole(grant, 'POSIX1E'), 'READ');
    assert.equal(aclRole(entry('USER', { READ: true, WRITE: false, EXECUTE: false }), 'POSIX1E'), 'CUSTOM');
    assert.throws(() => addACLGrant(posix(), family, 'FULL_CONTROL', 'folder'));
});

test('POSIX inheritance creates a complete default ACL without mutating source', () => {
    const original = posix(), before = stableACL(original);
    const after = addACLGrant(original, family, 'MODIFY', 'inherit');
    assert.equal(stableACL(original), before);
    assert.deepEqual(after.entries.filter(item => item.default).map(item => item.tag).sort(), ['GROUP', 'GROUP_OBJ', 'MASK', 'OTHER', 'USER_OBJ']);
    assert.equal(after.entries.find(item => item.tag === 'MASK' && item.default)?.permissions?.WRITE, true);
    assert.equal(after.entries.filter(item => item.tag === 'GROUP').length, 2);
});

test('existing POSIX masks remain intact and restrictions are reported', () => {
    const original = posix();
    original.entries.push(entry('MASK'));
    const after = addACLGrant(original, family, 'MODIFY', 'folder');
    assert.deepEqual(after.entries.find(item => item.tag === 'MASK'), original.entries[3]);
    assert.equal(aclWarnings(after).filter(warning => warning.includes('마스크')).length, 1);
});

test('NFSv4 DENY and custom flags survive adding a basic grant unchanged', () => {
    const original = posix(); original.aclType = 'NFS4';
    original.entries = [{ ...entry('USER'), id: 7, hasId: true, who: 'old-user', type: 'DENY', permissions: { DELETE: true }, flags: { INHERITED: true, FILE_INHERIT: true } }];
    const before = cloneACL(original);
    const after = addACLGrant(original, family, 'MODIFY', 'inherit');
    assert.deepEqual(original, before);
    assert.deepEqual(after.entries[0], before.entries[0]);
    assert.equal(aclRole(after.entries[0], 'NFS4'), 'CUSTOM');
    assert.equal(after.entries[1].basicFlags, 'INHERIT');
    assert.equal(after.entries[1].basicPerms, 'MODIFY');
    assert.equal(aclChanges(original, after).removed.length, 0);
    assert.equal(aclChanges(original, after).added.length, 1);
    assert.equal(aclWarnings(after).length, 1);
});

test('duplicate recipients are rejected and role edits preserve scope', () => {
    const original = posix(); original.aclType = 'NFS4'; original.entries = [];
    const acl = addACLGrant(original, family, 'READ', 'inherit');
    assert.throws(() => addACLGrant(acl, family, 'MODIFY', 'folder'));
    const modified = withACLRole(acl.entries[0], 'NFS4', 'FULL_CONTROL');
    assert.equal(modified.basicFlags, 'INHERIT');
    assert.equal(acl.entries[0].basicPerms, 'READ');
    assert.equal(modified.basicPerms, 'FULL_CONTROL');
});

test('diff respects duplicate rules and object key order', () => {
    const acl = posix(), next = cloneACL(acl);
    next.entries.push(cloneACL(acl.entries[0]));
    assert.equal(aclChanges(acl, next).added.length, 1);
    assert.equal(stableACL({ a: 1, b: 2 }), stableACL({ b: 2, a: 1 }));
});
