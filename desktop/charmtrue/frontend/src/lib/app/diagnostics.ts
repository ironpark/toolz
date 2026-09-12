import type { CertificateOverview, NetworkOverview, StorageOverview, SystemManagementOverview } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';

export type CheckStatus = 'pass' | 'warning' | 'critical' | 'unknown' | 'info';
export type CheckCategory = 'system' | 'tuning' | 'settings';
export interface DiagnosticCheck {
    id: string;
    category: CheckCategory;
    status: CheckStatus;
    title: string;
    detail: string;
    href: string;
    action?: 'compression_lz4' | 'sync_standard';
    target?: string;
    change?: string;
}
export type DiagnosticResults = [PromiseSettledResult<StorageOverview>, PromiseSettledResult<SystemManagementOverview>, PromiseSettledResult<NetworkOverview>, PromiseSettledResult<CertificateOverview>, PromiseSettledResult<boolean>];

export function canTuneDataset(id: string): boolean {
    const parts = id.split('/');
    return parts.length > 1 && parts.every(part => part !== '' && !part.startsWith('.') && !['ix-apps', 'ix-applications'].includes(part) && !/[@#]/.test(part));
}

export function buildDiagnosticChecks(results: DiagnosticResults): DiagnosticCheck[] {
    const checks: DiagnosticCheck[] = [];
    const add = (id: string, category: CheckCategory, status: CheckStatus, title: string, detail: string, href: string) => checks.push({ id, category, status, title, detail, href });
    const [storage, system, network, certificates, https] = results;
    const sources = [
        { result: storage, id: 'storage', label: '스토리지', href: '/storage' },
        { result: system, id: 'system', label: '시스템', href: '/system' },
        { result: network, id: 'network', label: '네트워크', href: '/network' },
        { result: certificates, id: 'certificates', label: '인증서', href: '/system' },
        { result: https, id: 'https', label: 'HTTPS 설정', href: '/system' },
    ];
    for (const source of sources) {
        if (source.result.status === 'rejected') {
            const reason = source.result.reason;
            add(`error-${source.id}`, 'system', 'unknown', `${source.label} 조회 실패`, `${reason instanceof Error ? reason.message : String(reason)} · 권한과 연결을 확인한 후 다시 점검하세요.`, source.href);
        }
    }
    if (storage.status === 'fulfilled') {
        const data = storage.value;
        if (!data.pools?.length) add('pools-empty', 'system', 'info', '스토리지 풀 없음', '점검할 풀이 없습니다. 저장소 구성을 확인하세요.', '/storage');
        for (const pool of data.pools ?? []) {
            add(`pool-${pool.id}`, 'system', pool.healthy && pool.status === 'ONLINE' ? 'pass' : 'critical', `${pool.name} 풀 상태`, `상태: ${pool.status || '알 수 없음'}. 이상이 있으면 디스크와 풀 상태를 확인하세요.`, '/storage');
            const usage = pool.size > 0 ? pool.allocated / pool.size * 100 : null;
            add(`space-${pool.id}`, 'system', usage === null ? 'unknown' : usage >= 90 ? 'critical' : usage >= 80 ? 'warning' : 'pass', `${pool.name} 공간 사용량`, usage === null ? '용량 정보를 확인할 수 없습니다.' : `${usage.toFixed(1)}% 사용 중 · 앱 점검 기준: 80% 주의, 90% 위험. 공간 확보 또는 확장을 검토하세요.`, '/storage');
        }
        add('snapshots', 'settings', data.snapshotCount > 0 ? 'info' : 'warning', '스냅샷 보호', data.snapshotCount > 0 ? `${data.snapshotCount}개 스냅샷이 있습니다. 개별 데이터셋의 주기·보존 정책과 별도 백업 여부는 추가 확인이 필요합니다.` : '조회된 스냅샷이 없습니다. 중요 데이터의 스냅샷과 별도 백업을 구성하세요.', '/datasets');
        for (const dataset of data.datasets ?? []) {
            if (dataset.type !== 'FILESYSTEM' || !canTuneDataset(dataset.id)) continue;
            if (dataset.locked) {
                add(`locked-${dataset.id}`, 'settings', 'info', `${dataset.id} 잠김`, '잠긴 데이터셋은 자동 튜닝에서 제외합니다.', '/datasets');
                continue;
            }
            if (dataset.compression.toUpperCase() === 'OFF') checks.push({ id: `compression-${dataset.id}`, category: 'tuning', status: 'info', title: `${dataset.id} 압축 최적화`, detail: '새로 쓰는 데이터에 LZ4 압축을 적용합니다. 압축률과 성능은 데이터에 따라 다르며 CPU 사용량이 늘 수 있습니다. 기존 데이터는 다시 압축하지 않습니다.', href: '/datasets', action: 'compression_lz4', target: dataset.id, change: 'compression: OFF → LZ4' });
            if (dataset.sync.toUpperCase() === 'DISABLED') checks.push({ id: `sync-${dataset.id}`, category: 'tuning', status: 'warning', title: `${dataset.id} 동기 쓰기 점검`, detail: '애플리케이션의 동기 쓰기 요청을 따르도록 복구합니다. 쓰기 성능이 낮아질 수 있습니다. 의도적으로 끈 설정인지 확인하세요.', href: '/datasets', action: 'sync_standard', target: dataset.id, change: 'sync: DISABLED → STANDARD' });
            if (!dataset.compression || !dataset.sync) add(`properties-${dataset.id}`, 'tuning', 'unknown', `${dataset.id} 튜닝 정보 부족`, '압축 또는 동기 쓰기 값을 읽지 못해 해당 설정의 자동 조치를 제안할 수 없습니다.', '/datasets');
        }
    }
    if (system.status === 'fulfilled') {
        const data = system.value;
        add('system-state', 'system', data.state === 'READY' ? 'pass' : data.state ? 'warning' : 'unknown', '시스템 준비 상태', data.state || '시스템 상태를 확인할 수 없습니다.', '/system');
        const stopped = (data.services ?? []).filter(service => service.enabled && service.state !== 'RUNNING');
        add('services', 'system', stopped.length ? 'warning' : 'pass', '자동 시작 서비스', stopped.length ? `${stopped.map(service => `${service.name} (${service.state})`).join(', ')} · 의도적으로 중지한 서비스인지 확인한 뒤 시스템 메뉴에서 시작하세요.` : '조회된 자동 시작 서비스가 모두 실행 중입니다.', '/system');
        add('updates', 'settings', data.update.available ? 'info' : data.update.status === 'UNKNOWN' || !data.update.status ? 'unknown' : 'info', '업데이트 상태', data.update.available ? `새 버전: ${data.update.version}. 백업과 호환성을 확인한 후 TrueNAS 웹 UI에서 업데이트하세요.` : `서버 보고 상태: ${data.update.status || '조회 불가'}. 최신 버전 여부는 TrueNAS 웹 UI에서 확인하세요.`, '/system');
    }
    if (network.status === 'fulfilled') {
        const data = network.value;
        add('pending-network', 'settings', data.pendingChanges || data.checkinRemaining > 0 ? 'warning' : 'pass', '네트워크 변경 확정', data.checkinRemaining > 0 ? `확정 대기 중이며 조회 시점에 ${data.checkinRemaining}초 남았습니다. 네트워크 메뉴에서 연결을 확인하고 확정하세요.` : data.pendingChanges ? '적용되지 않은 네트워크 변경이 있습니다. 적용 또는 취소가 필요합니다.' : '미확정 네트워크 변경이 없습니다.', '/network');
        add('dns', 'settings', data.configuration.nameServers?.some(Boolean) ? 'info' : 'warning', 'DNS 설정', data.configuration.nameServers?.some(Boolean) ? `설정된 DNS: ${data.configuration.nameServers.filter(Boolean).join(', ')}. 실제 이름 해석은 별도 확인이 필요합니다.` : '정적 DNS가 없습니다. DHCP로 DNS를 받는지 확인하세요.', '/network');
    }
    if (https.status === 'fulfilled') add('https', 'settings', https.value ? 'pass' : 'warning', 'HTTPS 리다이렉션', https.value ? 'HTTP 접속을 HTTPS로 전환하도록 설정되어 있습니다.' : 'HTTPS 접속이 가능한지 확인한 뒤 시스템 메뉴에서 리다이렉션을 활성화할 수 있습니다.', '/system');
    if (certificates.status === 'fulfilled') {
        const active = (certificates.value.certificates ?? []).find(cert => cert.id === certificates.value.uiCertificateId);
        add('ui-certificate', 'settings', !active ? 'unknown' : active.expired ? 'critical' : 'info', '웹 UI 인증서', !active ? '현재 웹 UI에 적용된 인증서를 찾지 못했습니다.' : active.expired ? `${active.name} 인증서가 만료되었습니다. 인증서를 갱신하거나 교체하세요.` : `${active.name} · 만료일: ${active.until || '확인 불가'}. 신뢰 체인과 접속 주소 일치 여부는 별도 확인이 필요합니다.`, '/system');
    }
    return checks;
}
