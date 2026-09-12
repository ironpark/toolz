import assert from 'node:assert/strict';
import test from 'node:test';
import { changeACLTarget, expandedFlags, expandedPermissions, newAdvancedEntry, nfsPermissions } from './acl-advanced.ts';

test('permission presets expand without dropping synchronization or granting administration', () => {
    const entry = newAdvancedEntry('NFS4');
    const read = expandedPermissions(entry);
    assert.deepEqual(Object.keys(read).filter(key => read[key]).sort(), ['READ_DATA', 'READ_NAMED_ATTRS', 'READ_ATTRIBUTES', 'READ_ACL', 'EXECUTE', 'SYNCHRONIZE'].sort());
    assert.equal(entry.basicPerms, 'READ');
    assert.deepEqual(entry.permissions, {});
    const modify = expandedPermissions({ ...entry, basicPerms: 'MODIFY' });
    assert.equal(modify.WRITE_ACL, false); assert.equal(modify.WRITE_OWNER, false);
    assert.equal(Object.values(modify).filter(Boolean).length, 12);
    assert.equal(Object.values(expandedPermissions({ ...entry, basicPerms: 'FULL_CONTROL' })).filter(Boolean).length, Object.keys(nfsPermissions).length);
    const traverse = expandedPermissions({ ...entry, basicPerms: 'TRAVERSE' });
    assert.equal(traverse.READ_DATA, false); assert.equal(traverse.READ_ACL, false); assert.equal(traverse.EXECUTE, true);
});

test('custom permission and inheritance maps are copied, including unknown values', () => {
    const entry = { ...newAdvancedEntry('NFS4'), type: 'DENY', basicPerms: '', permissions: { READ_DATA: true, FUTURE_PERMISSION: true }, basicFlags: '', flags: { INHERITED: true, FUTURE_FLAG: false } };
    assert.deepEqual(expandedPermissions(entry), entry.permissions);
    assert.deepEqual(expandedFlags(entry), entry.flags);
    assert.notEqual(expandedPermissions(entry), entry.permissions);
    assert.notEqual(expandedFlags(entry), entry.flags);
    assert.equal(entry.type, 'DENY');
    assert.throws(() => expandedPermissions({ ...entry, basicPerms: 'UNKNOWN' }));
    assert.throws(() => expandedFlags({ ...entry, basicFlags: 'UNKNOWN' }));
});

test('inheritance expansion retains the preset scope', () => {
    const entry = newAdvancedEntry('NFS4');
    assert.equal(Object.values(expandedFlags(entry)).some(Boolean), false);
    assert.deepEqual(Object.entries(expandedFlags({ ...entry, basicFlags: 'INHERIT' })).filter(([, value]) => value).map(([key]) => key), ['FILE_INHERIT', 'DIRECTORY_INHERIT']);
});

test('changing target clears stale identity but retains permissions and DENY', () => {
    const entry = { ...newAdvancedEntry('NFS4'), type: 'DENY', who: 'family', id: 1000, hasId: true };
    assert.equal(changeACLTarget(entry, 'GROUP'), entry);
    const changed = changeACLTarget(entry, 'USER');
    assert.equal(changed.hasId, false); assert.equal(changed.who, ''); assert.equal(changed.id, 0);
    assert.equal(changed.type, 'DENY'); assert.equal(changed.basicPerms, 'READ');
    assert.equal(entry.who, 'family');
});

test('new rules require a target and do not inherit by default', () => {
    const nfs = newAdvancedEntry('NFS4'), posix = newAdvancedEntry('POSIX1E');
    assert.equal(nfs.hasId, false); assert.equal(nfs.who, ''); assert.equal(nfs.basicFlags, 'NOINHERIT');
    assert.equal(posix.default, false); assert.equal(posix.basicPerms, '');
    assert.deepEqual(posix.permissions, { READ: true, WRITE: false, EXECUTE: true });
});
