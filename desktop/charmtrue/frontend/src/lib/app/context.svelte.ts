import { getContext, setContext } from 'svelte';
import { TrueNASService, type ACLTemplateInfo, type APIKeyMutation, type APIKeyMutationResult, type CertificateInstall, type CertificateOverview, type ConnectionInfo, type GeneratedCertificate, type SelfSignedCertificateRequest, type DatasetDeleteOptions, type DatasetMutation, type DatasetSnapshotMutation, type FilesystemACL, type FilesystemACLMutation, type GroupMutation, type IdentityOverview, type NetworkConfigurationMutation, type NetworkInterfaceMutation, type NetworkOverview, type RsyncTaskMutation, type SavedServer, type ShareMutation, type SharingOverview, type SMBShareACL, type StaticRouteMutation, type StorageOverview, type SystemManagementOverview, type UserMutation, type UserMutationResult } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
import type { View } from './types';

const APP_CONTEXT = Symbol('charmtrue-app');

export class AppContext {
    connection = $state<ConnectionInfo | null>(null);
    connectionVersion = $state(0);
    modalOpen = $state(false);
    loading = $state(false);
    message = $state('');
    savedServers = $state<SavedServer[]>([]);
    savedServersError = $state('');
    savedServersLoading = $state(false);
    storage = $state<StorageOverview | null>(null);
    storageLoading = $state(false);
    storageError = $state('');
    sharing = $state<SharingOverview | null>(null);
    sharingLoading = $state(false);
    sharingError = $state('');
    systemManagement = $state<SystemManagementOverview | null>(null);
    systemLoading = $state(false);
    systemError = $state('');
    identity = $state<IdentityOverview | null>(null);
    identityLoading = $state(false);
    identityError = $state('');
    network = $state<NetworkOverview | null>(null);
    certificates = $state<CertificateOverview | null>(null);
    certificatesLoading = $state(false);
    certificatesError = $state('');
    networkLoading = $state(false);
    networkError = $state('');

    openConnectModal(): void {
        this.message = '';
        this.modalOpen = true;
        void this.loadSavedServers();
    }

    closeConnectModal(): void {
        if (!this.loading) this.modalOpen = false;
    }

    async connect(endpoint: string, username: string, secret: string, allowPrivateCertificate: boolean, saveServer: boolean, saveCredential: boolean, authenticationMethod = 'password'): Promise<boolean> {
        this.loading = true;
        this.message = '';
        try {
            const connection = await TrueNASService.Connect(endpoint, username, secret, authenticationMethod, allowPrivateCertificate, saveServer, saveCredential);
            this.completeConnection(connection, saveServer);
            return true;
        } catch (error) {
            this.message = error instanceof Error ? error.message : String(error || '백엔드 서비스에 연결할 수 없습니다.');
            return false;
        } finally {
            this.loading = false;
        }
    }

    isRefreshing(view: View): boolean {
        if (view === 'settings' || view === 'diagnostics') return false;
        if (view === 'overview') return this.storageLoading || this.sharingLoading || this.systemLoading || this.identityLoading || this.networkLoading;
        if (view === 'storage' || view === 'datasets') return this.storageLoading;
        if (view === 'services') return this.sharingLoading;
        if (view === 'network') return this.networkLoading;
        if (view === 'identity') return this.identityLoading;
        return this.systemLoading || this.certificatesLoading;
    }

    async refreshView(view: View): Promise<void> {
        if (view === 'settings' || view === 'diagnostics') return;
        if (view === 'overview') {
            await Promise.all([this.refreshStorage(), this.refreshSharing(), this.refreshSystem(), this.refreshIdentity(), this.refreshNetwork()]);
        } else if (view === 'storage' || view === 'datasets') await this.refreshStorage();
        else if (view === 'services') await this.refreshSharing();
        else if (view === 'network') await this.refreshNetwork();
        else if (view === 'identity') await this.refreshIdentity();
        else await Promise.all([this.refreshSystem(), this.refreshCertificates()]);
    }

    async connectSavedServer(id: string): Promise<boolean> {
        this.loading = true;
        this.message = '';
        try {
            const connection = await TrueNASService.ConnectSavedServer(id);
            this.completeConnection(connection, true);
            return true;
        } catch (error) {
            this.message = error instanceof Error ? error.message : String(error || '저장된 로그인 정보로 연결하지 못했습니다.');
            return false;
        } finally {
            this.loading = false;
        }
    }

    private completeConnection(connection: ConnectionInfo, reloadSavedServers: boolean): void {
        this.connectionVersion++;
        this.storage = null;
        this.sharing = null;
        this.network = null;
        this.certificates = null;
        this.systemManagement = null;
        this.identity = null;
        this.storageLoading = this.sharingLoading = this.networkLoading = this.certificatesLoading = this.systemLoading = this.identityLoading = false;
        this.connection = connection;
        this.modalOpen = false;
        if (reloadSavedServers) void this.loadSavedServers();
        void Promise.all([this.refreshStorage(), this.refreshSharing(), this.refreshSystem(), this.refreshIdentity(), this.refreshNetwork(), this.refreshCertificates()]);
    }

    async loadSavedServers(): Promise<void> {
        this.savedServersLoading = true;
        this.savedServersError = '';
        try {
            this.savedServers = (await TrueNASService.SavedServers()) ?? [];
        } catch (error) {
            this.savedServersError = error instanceof Error ? error.message : String(error || '저장된 서버를 불러오지 못했습니다.');
        } finally {
            this.savedServersLoading = false;
        }
    }

    async deleteSavedServer(id: string): Promise<void> {
        this.savedServersError = '';
        try {
            await TrueNASService.DeleteSavedServer(id);
            await this.loadSavedServers();
        } catch (error) {
            this.savedServersError = error instanceof Error ? error.message : String(error || '저장된 서버를 삭제하지 못했습니다.');
        }
    }

    private async refreshResource<T>(section: 'storage' | 'sharing' | 'network' | 'certificates' | 'system' | 'identity', fetch: () => Promise<T>, apply: (value: T) => void): Promise<void> {
        const loadingKey = `${section}Loading` as const;
        const errorKey = `${section}Error` as const;
        if (!this.connection?.connected || this[loadingKey]) return;
        const version = this.connectionVersion;
        this[loadingKey] = true;
        this[errorKey] = '';
        try {
            const value = await fetch();
            if (version === this.connectionVersion) apply(value);
        } catch (error) {
            if (version === this.connectionVersion) this[errorKey] = error instanceof Error ? error.message : String(error);
        } finally {
            if (version === this.connectionVersion) this[loadingKey] = false;
        }
    }

    async refreshStorage(): Promise<void> {
        await this.refreshResource('storage', () => TrueNASService.StorageOverview(), value => { this.storage = value; });
    }

    async saveDataset(input: DatasetMutation): Promise<void> {
        this.storageError = '';
        try { await TrueNASService.SaveDataset(input); await this.refreshStorage(); }
        catch (error) { this.storageError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async deleteDataset(input: DatasetDeleteOptions): Promise<void> {
        this.storageError = '';
        try { await TrueNASService.DeleteDataset(input); await this.refreshStorage(); }
        catch (error) { this.storageError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async createDatasetSnapshot(input: DatasetSnapshotMutation): Promise<void> {
        this.storageError = '';
        try { await TrueNASService.CreateDatasetSnapshot(input); await this.refreshStorage(); }
        catch (error) { this.storageError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async setDatasetLocked(id: string, secret: string, lock: boolean, recursive: boolean, force: boolean): Promise<void> {
        this.storageError = '';
        try { await TrueNASService.SetDatasetLocked(id, secret, lock, recursive, force); await this.refreshStorage(); }
        catch (error) { this.storageError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async getFilesystemACL(path: string): Promise<FilesystemACL> {
        return TrueNASService.GetFilesystemACL(path);
    }

    async getACLTemplates(path: string): Promise<ACLTemplateInfo[]> {
        return (await TrueNASService.ACLTemplates(path)) ?? [];
    }

    async saveFilesystemACL(input: FilesystemACLMutation): Promise<void> {
        this.storageError = '';
        try { await TrueNASService.SaveFilesystemACL(input); await this.refreshStorage(); }
        catch (error) { this.storageError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async refreshSharing(): Promise<void> {
        await this.refreshResource('sharing', () => TrueNASService.SharingOverview(), value => { this.sharing = value; });
    }

    async setShareEnabled(protocol: string, id: number, enabled: boolean): Promise<void> {
        this.sharingError = '';
        try { await TrueNASService.SetShareEnabled(protocol, id, enabled); await this.refreshSharing(); }
        catch (error) { this.sharingError = error instanceof Error ? error.message : String(error); }
    }

    async deleteShare(protocol: string, id: number): Promise<void> {
        this.sharingError = '';
        try { await TrueNASService.DeleteShare(protocol, id); await this.refreshSharing(); }
        catch (error) { this.sharingError = error instanceof Error ? error.message : String(error); }
    }

    async runRsyncTask(id: number): Promise<void> {
        this.sharingError = '';
        try { await TrueNASService.RunRsyncTask(id); }
        catch (error) { this.sharingError = error instanceof Error ? error.message : String(error); }
    }

    async deleteRsyncTask(id: number): Promise<void> {
        this.sharingError = '';
        try { await TrueNASService.DeleteRsyncTask(id); await this.refreshSharing(); }
        catch (error) { this.sharingError = error instanceof Error ? error.message : String(error); }
    }

    async getSMBShareACL(shareName: string): Promise<SMBShareACL> {
        return TrueNASService.GetSMBShareACL(shareName);
    }

    async saveSMBShareACL(input: SMBShareACL): Promise<void> {
        await TrueNASService.SaveSMBShareACL(input);
    }

    async saveShare(input: ShareMutation): Promise<void> {
        this.sharingError = '';
        await TrueNASService.SaveShare(input);
        await this.refreshSharing();
    }

    async saveRsyncTask(input: RsyncTaskMutation): Promise<void> {
        this.sharingError = '';
        await TrueNASService.SaveRsyncTask(input);
        await this.refreshSharing();
    }

    async refreshNetwork(): Promise<void> {
        await this.refreshResource('network', () => TrueNASService.NetworkOverview(), value => { this.network = value; });
    }

    async saveNetworkConfiguration(input: NetworkConfigurationMutation): Promise<void> {
        this.networkError = '';
        try { await TrueNASService.SaveNetworkConfiguration(input); await this.refreshNetwork(); }
        catch (error) { this.networkError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async saveNetworkInterface(input: NetworkInterfaceMutation): Promise<void> {
        this.networkError = '';
        try { await TrueNASService.SaveNetworkInterface(input); await this.refreshNetwork(); }
        catch (error) { this.networkError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async deleteNetworkInterface(id: string): Promise<void> {
        this.networkError = '';
        try { await TrueNASService.DeleteNetworkInterface(id); await this.refreshNetwork(); }
        catch (error) { this.networkError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async commitNetworkChanges(timeout = 60): Promise<void> {
        this.networkError = '';
        try { await TrueNASService.CommitNetworkChanges(timeout); await this.refreshNetwork(); }
        catch (error) { this.networkError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async checkinNetworkChanges(): Promise<void> {
        this.networkError = '';
        try { await TrueNASService.CheckinNetworkChanges(); await this.refreshNetwork(); }
        catch (error) { this.networkError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async rollbackNetworkChanges(): Promise<void> {
        this.networkError = '';
        try { await TrueNASService.RollbackNetworkChanges(); await this.refreshNetwork(); }
        catch (error) { this.networkError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async saveStaticRoute(input: StaticRouteMutation): Promise<void> {
        this.networkError = '';
        try { await TrueNASService.SaveStaticRoute(input); await this.refreshNetwork(); }
        catch (error) { this.networkError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async deleteStaticRoute(id: number): Promise<void> {
        this.networkError = '';
        try { await TrueNASService.DeleteStaticRoute(id); await this.refreshNetwork(); }
        catch (error) { this.networkError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async refreshCertificates(): Promise<void> {
        await this.refreshResource('certificates', () => TrueNASService.CertificateOverview(), value => { this.certificates = value; });
    }

    async selfSignedCertificateDefaults(): Promise<SelfSignedCertificateRequest> {
        return TrueNASService.SelfSignedCertificateDefaults();
    }

    async generateSelfSignedCertificate(input: SelfSignedCertificateRequest): Promise<GeneratedCertificate> {
        return TrueNASService.GenerateSelfSignedCertificate(input);
    }

    async saveCertificateFiles(cert: GeneratedCertificate): Promise<string> {
        return TrueNASService.SaveCertificateFiles(cert);
    }

    async installCertificate(input: CertificateInstall): Promise<number> {
        this.certificatesError = '';
        const id = await TrueNASService.InstallCertificate(input);
        await this.refreshCertificates();
        return id;
    }

    async setUICertificate(id: number): Promise<void> {
        this.certificatesError = '';
        try { await TrueNASService.SetUICertificate(id); await this.refreshCertificates(); }
        catch (error) { this.certificatesError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async deleteCertificate(id: number): Promise<void> {
        this.certificatesError = '';
        try { await TrueNASService.DeleteCertificate(id); await this.refreshCertificates(); }
        catch (error) { this.certificatesError = error instanceof Error ? error.message : String(error); throw error; }
    }

    async refreshSystem(): Promise<void> {
        await this.refreshResource('system', () => TrueNASService.SystemManagementOverview(), value => { this.systemManagement = value; });
    }
    async controlSystemService(name:string,action:string):Promise<void>{this.systemError='';try{await TrueNASService.ControlSystemService(name,action);await this.refreshSystem()}catch(e){this.systemError=e instanceof Error?e.message:String(e)}}
    async setServiceStartup(name: string, automatic: boolean): Promise<void> {
        const version = this.connectionVersion;
        this.systemError = '';
        try {
            await TrueNASService.SetServiceStartup(name, automatic);
            if (version !== this.connectionVersion) return;
            const service = this.systemManagement?.services?.find(item => item.name === name);
            if (service) service.enabled = automatic;
            await this.refreshSystem();
        } catch (e) { if (version === this.connectionVersion) this.systemError = e instanceof Error ? e.message : String(e); }
    }
    async powerAction(action:string):Promise<void>{this.systemError='';try{await TrueNASService.PowerAction(action)}catch(e){this.systemError=e instanceof Error?e.message:String(e)}}
    async refreshIdentity(): Promise<void> {
        await this.refreshResource('identity', () => TrueNASService.IdentityOverview(), value => { this.identity = value; });
    }
    async deleteIdentity(kind:string,id:number):Promise<void>{this.identityError='';try{await TrueNASService.DeleteIdentity(kind,id);await this.refreshIdentity()}catch(e){this.identityError=e instanceof Error?e.message:String(e)}}
    async saveUser(input: UserMutation): Promise<UserMutationResult> { this.identityError=''; const result=await TrueNASService.SaveUser(input); await this.refreshIdentity(); return result; }
    async saveGroup(input: GroupMutation): Promise<void> { this.identityError=''; await TrueNASService.SaveGroup(input); await this.refreshIdentity(); }
    async saveAPIKey(input: APIKeyMutation): Promise<APIKeyMutationResult> { this.identityError=''; const result=await TrueNASService.SaveAPIKey(input); await this.refreshIdentity(); return result; }

    async restoreConnection(): Promise<void> {
        try {
            this.connection = await TrueNASService.CurrentConnection();
        } catch {
            // The Wails runtime may not be available in a standalone browser preview.
        }
        if (!this.connection?.connected) this.modalOpen = true;
        await this.loadSavedServers();
    }
}

export function createAppContext(): AppContext {
    return setContext(APP_CONTEXT, new AppContext());
}

export function getAppContext(): AppContext {
    return getContext<AppContext>(APP_CONTEXT);
}
