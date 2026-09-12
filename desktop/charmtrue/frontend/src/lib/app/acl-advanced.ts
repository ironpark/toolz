import type { FilesystemACLEntry } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';

export const nfsPermissions: Record<string, string> = {
    READ_DATA: '파일 읽기 / 폴더 목록 보기', WRITE_DATA: '파일 쓰기 / 파일 생성', APPEND_DATA: '내용 추가 / 하위 폴더 생성',
    EXECUTE: '파일 실행 / 폴더 탐색', DELETE: '이 항목 삭제', DELETE_CHILD: '하위 항목 삭제',
    READ_ATTRIBUTES: '속성 읽기', WRITE_ATTRIBUTES: '속성 변경', READ_NAMED_ATTRS: '확장 속성 읽기', WRITE_NAMED_ATTRS: '확장 속성 변경',
    READ_ACL: '권한 보기', WRITE_ACL: '권한 변경', WRITE_OWNER: '소유자 변경', SYNCHRONIZE: '동기화',
};
export const posixPermissions: Record<string, string> = { READ: '읽기', WRITE: '쓰기', EXECUTE: '실행 / 폴더 탐색' };
export const inheritanceFlags: Record<string, string> = {
    FILE_INHERIT: '새 파일에 상속', DIRECTORY_INHERIT: '새 폴더에 상속', NO_PROPAGATE_INHERIT: '바로 아래까지만 상속',
    INHERIT_ONLY: '하위 항목에만 적용', INHERITED: '상위에서 상속된 규칙으로 표시',
};
export const aclFlags: Record<string, string> = { protected: '상위의 상속 변경으로부터 보호', autoinherit: '상위 폴더의 권한 자동 상속', defaulted: '기본 규칙으로 생성된 ACL로 표시' };
export const permissionPresets: Record<string, string> = { READ: '읽기 전용', MODIFY: '읽기·쓰기', FULL_CONTROL: '전체 관리', TRAVERSE: '폴더 통과' };

// Match TrueNAS middleware utils/filesystem/acl.py BASIC masks, including SYNCHRONIZE.
export function expandedPermissions(entry: FilesystemACLEntry): Partial<Record<string, boolean>> {
    if (!entry.basicPerms) return { ...(entry.permissions ?? {}) };
    if (!(entry.basicPerms in permissionPresets)) throw new Error('알 수 없는 권한 프리셋입니다. JSON 옵션에서 확인하세요.');
    const read = ['READ_DATA', 'READ_NAMED_ATTRS', 'READ_ATTRIBUTES', 'READ_ACL', 'EXECUTE', 'SYNCHRONIZE'];
    const traverse = ['READ_NAMED_ATTRS', 'READ_ATTRIBUTES', 'EXECUTE', 'SYNCHRONIZE'];
    return Object.fromEntries(Object.keys(nfsPermissions).map(key => [key, entry.basicPerms === 'FULL_CONTROL' ||
        (entry.basicPerms === 'MODIFY' ? !['WRITE_ACL', 'WRITE_OWNER'].includes(key) : (entry.basicPerms === 'READ' ? read : traverse).includes(key))]));
}

export function expandedFlags(entry: FilesystemACLEntry): Partial<Record<string, boolean>> {
    if (!entry.basicFlags) return { ...(entry.flags ?? {}) };
    if (!['INHERIT', 'NOINHERIT'].includes(entry.basicFlags)) throw new Error('알 수 없는 상속 프리셋입니다. JSON 옵션에서 확인하세요.');
    return Object.fromEntries(Object.keys(inheritanceFlags).map(key => [key, entry.basicFlags === 'INHERIT' && ['FILE_INHERIT', 'DIRECTORY_INHERIT'].includes(key)]));
}

export function changeACLTarget(entry: FilesystemACLEntry, tag: string): FilesystemACLEntry {
    return tag === entry.tag ? entry : { ...entry, tag, who: '', id: 0, hasId: false };
}

export function newAdvancedEntry(type: string): FilesystemACLEntry {
    return { tag: 'GROUP', type: type === 'NFS4' ? 'ALLOW' : '', id: 0, hasId: false, who: '', default: false,
        basicPerms: type === 'NFS4' ? 'READ' : '', permissions: type === 'NFS4' ? {} : { READ: true, WRITE: false, EXECUTE: true },
        basicFlags: type === 'NFS4' ? 'NOINHERIT' : '', flags: {} };
}
