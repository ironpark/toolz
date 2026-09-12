const services: Record<string, { label: string; description: string }> = {
    cifs: { label: 'CIFS', description: 'SMB 프로토콜로 Windows·macOS 등과 파일을 공유합니다.' },
    ftp: { label: 'FTP', description: 'FTP 클라이언트로 파일을 업로드·다운로드합니다.' },
    iscsitarget: { label: 'iSCSI Target', description: '네트워크를 통해 다른 장치에 블록 스토리지를 제공합니다.' },
    nfs: { label: 'NFS', description: 'Linux·Unix 등의 시스템에 네트워크 파일 공유를 제공합니다.' },
    snmp: { label: 'SNMP', description: '외부 모니터링 도구에 시스템 상태와 관리 정보를 제공합니다.' },
    ssh: { label: 'SSH', description: '암호화된 연결로 서버에 원격 접속하고 명령을 실행합니다.' },
    ups: { label: 'UPS', description: '무정전 전원장치를 감시하고 정전 시 안전한 종료를 돕습니다.' },
    nvmet: { label: 'NVMe Target', description: 'NVMe over Fabrics로 다른 장치에 블록 스토리지를 제공합니다.' },
};

export function servicePresentation(name: string): { label: string; description: string } {
    return services[name.toLowerCase()] ?? { label: name, description: 'TrueNAS에서 제공하는 시스템 서비스입니다.' };
}

export function serviceMatches(name: string, query: string): boolean {
    const { label, description } = servicePresentation(name);
    return `${name} ${label} ${description}`.toLowerCase().includes(query.trim().toLowerCase());
}
