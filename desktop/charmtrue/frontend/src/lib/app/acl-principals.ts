import type { GroupInfo, UserInfo } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
import type { ACLPrincipal } from './acl-editor';

export const specialPrincipalHelp: Record<string, string> = {
    'owner@': '이 폴더를 소유한 사용자에게 적용합니다. 현재 로그인한 사용자와는 다를 수 있습니다.',
    USER_OBJ: '이 폴더를 소유한 사용자에게 적용합니다. 현재 로그인한 사용자와는 다를 수 있습니다.',
    'group@': '이 폴더의 소유 그룹에 속한 사용자에게 적용합니다.',
    GROUP_OBJ: '이 폴더의 소유 그룹에 속한 사용자에게 적용합니다. 마스크가 있으면 해당 상한의 제한을 받습니다.',
    OTHER: '소유자나 일치하는 사용자·그룹 규칙에 해당하지 않는 사용자에게 적용합니다. 모든 사용자를 뜻하지 않습니다.',
    'everyone@': '소유자와 소유 그룹을 포함한 모든 사용자에게 적용합니다. 다른 허용·거부 규칙도 함께 영향을 줍니다.',
    MASK: '지정 사용자·그룹과 소유 그룹에 허용할 수 있는 권한의 상한입니다. 소유자와 기타 사용자에는 적용되지 않습니다.',
};

export interface PrincipalCandidate extends ACLPrincipal { system: boolean; description: string }

// Use server metadata and reserved name/ID pairs, not a blanket low-ID cutoff.
const reserved: Record<string, number> = { root: 0, wheel: 0, apps: 568, nobody: 65534, nogroup: 65534, 'www-data': 33 };
const descriptions: Record<string, string> = {
    root: '시스템 최고 관리자 계정', wheel: '시스템 관리용 그룹',
    apps: '일부 앱에서 사용하는 실행 계정·그룹 · 앱의 실제 UID/GID 확인 필요',
    nobody: '권한이 제한된 비특권 계정', nogroup: '권한이 제한된 비특권 그룹',
    'www-data': '웹 서비스 실행용 계정·그룹',
    builtin_users: 'SMB 사용자들이 속하는 기본 그룹 · 부여 범위가 넓을 수 있음',
    builtin_administrators: '기본 관리자 그룹 · 실제 구성원 확인 필요',
    builtin_guests: '게스트 접근용 기본 그룹',
};

export function principalCandidates(users: UserInfo[], groups: GroupInfo[], type: 'USER' | 'GROUP'): PrincipalCandidate[] {
    const values = type === 'USER' ? users.map(user => ({ tag: type, id: user.uid, name: user.username, local: user.local, builtin: user.builtin, detail: user.fullName }))
        : groups.map(group => ({ tag: type, id: group.gid, name: group.name, local: group.local, builtin: group.builtin, detail: '' }));
    return values.map(item => {
        const known = item.builtin || item.local && reserved[item.name] === item.id;
        return { tag: item.tag, id: item.id, name: item.name, system: known,
            description: known && descriptions[item.name] ? descriptions[item.name] : item.detail || (item.builtin ? '시스템 기본 계정·그룹' : item.local ? '' : '디렉터리 서비스 계정·그룹') };
    });
}

export function filterPrincipals(candidates: PrincipalCandidate[], query: string, showSystem: boolean): PrincipalCandidate[] {
    const search = query.trim().toLowerCase();
    return candidates.filter(item => (showSystem || !item.system) && `${item.name} ${item.id} ${item.description}`.toLowerCase().includes(search));
}
