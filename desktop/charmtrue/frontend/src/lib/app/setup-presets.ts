import type { SetupPreset } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
export const presetStorageKey = 'charmtrue.setup-presets.v1';
export function cleanPreset(p: SetupPreset): SetupPreset {
    return { id: p.id, name: p.name.trim(), groups: (p.groups ?? []).map(name => name.trim()).filter(Boolean),
        users: (p.users ?? []).map(user => ({ name: user.name.trim(), fullName: user.fullName.trim(), group: user.group.trim(), smb: user.smb === true })), enableSSH: p.enableSSH === true, adminUser: p.adminUser.trim(), replaceAdminUser: (p.replaceAdminUser ?? '').trim() };
}
export function readPresets(text: string | null): SetupPreset[] {
    if (!text) return [];
    const values = JSON.parse(text);
    if (!Array.isArray(values)) throw new Error('프리셋 저장 형식이 올바르지 않습니다.');
    for (const p of values) {
        if (p?.replaceAdminUser !== undefined && typeof p.replaceAdminUser !== 'string') throw new Error('관리자 교체 설정이 올바르지 않습니다.');
        if (!p || typeof p.id !== 'string' || typeof p.name !== 'string' || typeof p.adminUser !== 'string' || typeof p.enableSSH !== 'boolean' || !Array.isArray(p.groups) || p.groups.some((g: unknown) => typeof g !== 'string') || !Array.isArray(p.users) || p.users.some((u: SetupPreset['users'] extends (infer T)[] ? T : any) => !u || !['name', 'fullName', 'group'].every(key => typeof u[key] === 'string') || typeof u.smb !== 'boolean')) throw new Error('저장된 프리셋 항목이 올바르지 않습니다.');
    }
    return values.map(cleanPreset);
}
export function serializePresets(presets: SetupPreset[]): string { return JSON.stringify(presets.map(cleanPreset)); }
