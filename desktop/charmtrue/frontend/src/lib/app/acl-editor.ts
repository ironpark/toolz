import type { FilesystemACL, FilesystemACLEntry } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
import { nfsPermissions } from './acl-advanced.ts';

export type ACLRole = 'NONE' | 'READ' | 'MODIFY' | 'FULL_CONTROL';
export type ACLScope = 'folder' | 'inherit';
export type EditableACL = Omit<FilesystemACL, 'entries'> & { entries: FilesystemACLEntry[] };
export interface ACLPrincipal { tag: 'USER' | 'GROUP'; id: number; name: string }
export const roleLabels: Record<ACLRole, string> = { NONE: '접근 없음', READ: '읽기 전용', MODIFY: '읽기·쓰기', FULL_CONTROL: '전체 관리' };

export function cloneACL<T>(value: T): T { return JSON.parse(JSON.stringify(value)); }
export function editableACL(acl: FilesystemACL): EditableACL { return { ...cloneACL(acl), entries: cloneACL(acl.entries ?? []), nfs41Flags: cloneACL(acl.nfs41Flags ?? {}) }; }

export function stableACL(value: unknown): string {
    if (Array.isArray(value)) return '[' + value.map(stableACL).join(',') + ']';
    if (value && typeof value === 'object') return '{' + Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, item]) => JSON.stringify(key) + ':' + stableACL(item)).join(',') + '}';
    return JSON.stringify(value) ?? 'null';
}

export function aclRole(entry: FilesystemACLEntry, type: string): ACLRole | 'CUSTOM' {
    if (type === 'NFS4') {
        if (entry.type !== 'ALLOW' || !['NOINHERIT', 'INHERIT'].includes(entry.basicFlags) || Object.values(entry.flags ?? {}).some(Boolean)) return 'CUSTOM';
        if (!entry.basicPerms && Object.values(entry.permissions ?? {}).every(value => value === false)) return 'NONE';
        return ['READ', 'MODIFY', 'FULL_CONTROL'].includes(entry.basicPerms) ? entry.basicPerms as ACLRole : 'CUSTOM';
    }
    if (type !== 'POSIX1E' || entry.tag === 'MASK' || entry.basicPerms) return 'CUSTOM';
    const p = entry.permissions ?? {};
    if (Object.keys(p).some(key => !['READ', 'WRITE', 'EXECUTE'].includes(key))) return 'CUSTOM';
    if (p.READ === false && p.WRITE === false && p.EXECUTE === false) return 'NONE';
    if (p.READ === true && p.EXECUTE === true && p.WRITE === false) return 'READ';
    if (p.READ === true && p.EXECUTE === true && p.WRITE === true) return 'MODIFY';
    return 'CUSTOM';
}

export function withACLRole(entry: FilesystemACLEntry, type: string, role: ACLRole): FilesystemACLEntry {
    if (!(role in roleLabels) || !['NFS4', 'POSIX1E'].includes(type)) throw new Error('지원하지 않는 권한입니다.');
    if (type === 'POSIX1E' && role === 'FULL_CONTROL') throw new Error('POSIX ACL에서는 전체 관리 역할을 지원하지 않습니다.');
    const result = cloneACL(entry);
    if (type === 'NFS4') {
        result.basicPerms = role === 'NONE' ? '' : role;
        result.permissions = role === 'NONE' ? Object.fromEntries(Object.keys(nfsPermissions).map(key => [key, false])) : {};
    }
    else { result.basicPerms = ''; result.permissions = { READ: role !== 'NONE', WRITE: role === 'MODIFY', EXECUTE: role !== 'NONE' }; }
    return result;
}

export function principalLabel(entry: FilesystemACLEntry): string {
    const special: Record<string, string> = { 'owner@': '소유자', 'group@': '소유 그룹', 'everyone@': '모두', USER_OBJ: '소유자', GROUP_OBJ: '소유 그룹', OTHER: '기타 사용자', MASK: '그룹 권한 상한(마스크)' };
    return special[entry.tag] ?? `${entry.tag === 'GROUP' ? '그룹' : '사용자'} ${entry.who || (entry.hasId ? String(entry.id) : '미지정')}`;
}

export function entryDescription(entry: FilesystemACLEntry, type: string): string {
    const role = aclRole(entry, type);
    const scope = type === 'NFS4' ? entry.basicFlags === 'INHERIT' ? '현재 폴더 + 새 하위 항목' : entry.basicFlags === 'NOINHERIT' ? '현재 폴더' : '사용자 지정 상속' : entry.default ? '새 하위 항목의 기본 권한' : '현재 폴더';
    return `${principalLabel(entry)} · ${entry.type === 'DENY' ? '거부 · ' : ''}${role === 'CUSTOM' ? '사용자 지정' : roleLabels[role]} · ${scope}`;
}

// Only create a missing POSIX mask. Widening an existing mask could silently
// grant access through unrelated entries, so leave it intact and show a warning.
function ensurePOSIXMask(entries: FilesystemACLEntry[], isDefault: boolean): void {
    if (entries.some(entry => entry.default === isDefault && entry.tag === 'MASK')) return;
    const peers = entries.filter(entry => entry.default === isDefault && ['USER', 'GROUP', 'GROUP_OBJ'].includes(entry.tag));
    entries.push({ tag: 'MASK', type: '', id: 0, hasId: false, who: '', basicPerms: '', permissions: {
        READ: peers.some(entry => entry.permissions?.READ), WRITE: peers.some(entry => entry.permissions?.WRITE), EXECUTE: peers.some(entry => entry.permissions?.EXECUTE),
    }, basicFlags: '', flags: {}, default: isDefault });
}

export function addACLGrant(acl: FilesystemACL, principal: ACLPrincipal, role: ACLRole, scope: ACLScope): EditableACL {
    if (!Number.isInteger(principal.id) || principal.id < 0 || !principal.name) throw new Error('사용자 또는 그룹을 선택하세요.');
    if ((acl.entries ?? []).some(entry => entry.tag === principal.tag && (entry.hasId && entry.id === principal.id || entry.who === principal.name))) throw new Error('이미 등록된 대상입니다. 기존 항목을 수정하거나 고급 모드에서 규칙을 확인하세요.');
    const next = editableACL(acl);
    let entry: FilesystemACLEntry = { tag: principal.tag, type: acl.aclType === 'NFS4' ? 'ALLOW' : '', id: principal.id, hasId: true, who: principal.name, basicPerms: '', permissions: {}, basicFlags: acl.aclType === 'NFS4' ? scope === 'inherit' ? 'INHERIT' : 'NOINHERIT' : '', flags: {}, default: false };
    entry = withACLRole(entry, acl.aclType, role);
    next.entries.push(entry);
    if (acl.aclType === 'POSIX1E') {
        ensurePOSIXMask(next.entries, false);
        if (scope === 'inherit') {
            for (const tag of ['USER_OBJ', 'GROUP_OBJ', 'OTHER']) {
                if (next.entries.some(item => item.default && item.tag === tag)) continue;
                const base = next.entries.find(item => !item.default && item.tag === tag);
                if (!base) throw new Error('POSIX 기본 소유권 항목이 없습니다. 고급 모드에서 ACL을 확인하세요.');
                next.entries.push({ ...cloneACL(base), default: true });
            }
            next.entries.push({ ...cloneACL(entry), default: true });
            ensurePOSIXMask(next.entries, true);
        }
    }
    return next;
}

export function aclWarnings(acl: FilesystemACL): string[] {
    const warnings: string[] = [];
    const entries = acl.entries ?? [];
    if (entries.some(entry => aclRole(entry, acl.aclType) === 'NONE')) warnings.push('접근 없음은 이 규칙에서 권한을 부여하지 않는 설정입니다. 다른 사용자·그룹 규칙이나 관리자 권한에 의한 접근까지 차단하지는 않습니다.');
    if (entries.some(entry => entry.type === 'DENY')) warnings.push('거부 규칙이 있습니다. 추가한 허용 권한과 함께 실제 접근에 영향을 줄 수 있습니다.');
    if (acl.aclType === 'POSIX1E') {
        for (const defaults of [false, true]) {
            const mask = entries.find(entry => entry.tag === 'MASK' && entry.default === defaults);
            if (mask && entries.some(entry => entry.default === defaults && ['USER', 'GROUP', 'GROUP_OBJ'].includes(entry.tag) && Object.entries(entry.permissions ?? {}).some(([key, enabled]) => enabled && !mask.permissions?.[key]))) warnings.push(`${defaults ? '새 하위 항목의' : '현재 폴더의'} 마스크가 일부 사용자·그룹 권한을 제한합니다. 고급 모드에서 확인하세요.`);
        }
    }
    return warnings;
}

export function aclChanges(before: FilesystemACL, after: FilesystemACL): { added: FilesystemACLEntry[]; removed: FilesystemACLEntry[] } {
    const unmatched = [...(after.entries ?? [])];
    const removed = (before.entries ?? []).filter(entry => {
        const index = unmatched.findIndex(item => stableACL(item) === stableACL(entry));
        if (index < 0) return true;
        unmatched.splice(index, 1);
        return false;
    });
    return { added: unmatched, removed };
}
