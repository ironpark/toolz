import assert from 'node:assert/strict';
import test from 'node:test';
import { serviceMatches, servicePresentation } from './service-labels.ts';

test('service names use requested spelling and include explanations', () => {
    for (const [name, label] of Object.entries({ cifs: 'CIFS', ftp: 'FTP', iscsitarget: 'iSCSI Target', nfs: 'NFS', snmp: 'SNMP', ssh: 'SSH', ups: 'UPS', nvmet: 'NVMe Target' })) {
        assert.equal(servicePresentation(name).label, label);
        assert.ok(servicePresentation(name).description.length > 0);
    }
    assert.equal(servicePresentation('custom_service').label, 'custom_service');
});

test('search supports original identifiers, display names and Korean descriptions', () => {
    assert.ok(serviceMatches('iscsitarget', 'iscsi target'));
    assert.ok(serviceMatches('nvmet', 'nvmet'));
    assert.ok(serviceMatches('ups', ' 정전 '));
    assert.ok(serviceMatches('cifs', 'SMB'));
    assert.equal(serviceMatches('ssh', 'FTP'), false);
});
