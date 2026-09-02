import { getContext, setContext } from 'svelte';
import { TrueNASService, type APIKeyMutation, type APIKeyMutationResult, type ConnectionInfo, type GroupMutation, type IdentityOverview, type NetworkConfigurationMutation, type NetworkInterfaceMutation, type NetworkOverview, type RsyncTaskMutation, type SavedServer, type ShareMutation, type SharingOverview, type StaticRouteMutation, type StorageOverview, type SystemManagementOverview, type UserMutation, type UserMutationResult } from '../../../bindings/github.com/ironpark/toolz/desktop/charmtrue';
import type { AuthenticationMethod } from './types';

const APP_CONTEXT = Symbol('charmtrue-app');

export class AppContext {
    connection = $state<ConnectionInfo | null>(null);
    modalOpen = $state(false);
    loading = $state(false);
    message = $state('');
    savedServers = $state<SavedServer[]>([]);
    savedServersError = $state('');
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

    async connect(endpoint: string, username: string, secret: string, authenticationMethod: AuthenticationMethod, allowPrivateCertificate: boolean, saveServer: boolean, saveCredential: boolean): Promise<void> {
        this.loading = true;
        this.message = '';
        try {
            const connection = await TrueNASService.Connect(endpoint, username, secret, authenticationMethod, allowPrivateCertificate, saveServer, saveCredential);
            await this.completeConnection(connection, saveServer);
        } catch (error) {
            this.message = error instanceof Error ? error.message : String(error || '백엔드 서비스에 연결할 수 없습니다.');
        } finally {
            this.loading = false;
        }
    }

    async connectSavedServer(id: string): Promise<void> {
        this.loading = true;
        this.message = '';
        try {
            const connection = await TrueNASService.ConnectSavedServer(id);
            await this.completeConnection(connection, true);
        } catch (error) {
            this.message = error instanceof Error ? error.message : String(error || '저장된 로그인 정보로 연결하지 못했습니다.');
        } finally {
            this.loading = false;
        }
    }

    private async completeConnection(connection: ConnectionInfo, reloadSavedServers: boolean): Promise<void> {
        this.connection = connection;
        this.modalOpen = false;
        if (reloadSavedServers) await this.loadSavedServers();
        await Promise.all([this.refreshStorage(), this.refreshSharing(), this.refreshSystem(), this.refreshIdentity(), this.refreshNetwork()]);
    }

    async loadSavedServers(): Promise<void> {
        this.savedServersError = '';
        try {
            this.savedServers = (await TrueNASService.SavedServers()) ?? [];
        } catch (error) {
            this.savedServersError = error instanceof Error ? error.message : String(error || '저장된 서버를 불러오지 못했습니다.');
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

    async refreshStorage(): Promise<void> {
        if (!this.connection?.connected || this.storageLoading) return;
        this.storageLoading = true;
        this.storageError = '';
        try {
            this.storage = await TrueNASService.StorageOverview();
        } catch (error) {
            this.storageError = error instanceof Error ? error.message : String(error || '스토리지 정보를 불러오지 못했습니다.');
        } finally {
            this.storageLoading = false;
        }
    }

    async refreshSharing(): Promise<void> {
        if (!this.connection?.connected || this.sharingLoading) return;
        this.sharingLoading = true; this.sharingError = '';
        try { this.sharing = await TrueNASService.SharingOverview(); }
        catch (error) { this.sharingError = error instanceof Error ? error.message : String(error || '공유 정보를 불러오지 못했습니다.'); }
        finally { this.sharingLoading = false; }
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
        if (!this.connection?.connected || this.networkLoading) return;
        this.networkLoading = true;
        this.networkError = '';
        try { this.network = await TrueNASService.NetworkOverview(); }
        catch (error) { this.networkError = error instanceof Error ? error.message : String(error || '네트워크 정보를 불러오지 못했습니다.'); }
        finally { this.networkLoading = false; }
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

    async refreshSystem(): Promise<void> { if(!this.connection?.connected||this.systemLoading)return;this.systemLoading=true;this.systemError='';try{this.systemManagement=await TrueNASService.SystemManagementOverview()}catch(e){this.systemError=e instanceof Error?e.message:String(e)}finally{this.systemLoading=false} }
    async controlSystemService(name:string,action:string):Promise<void>{this.systemError='';try{await TrueNASService.ControlSystemService(name,action);await this.refreshSystem()}catch(e){this.systemError=e instanceof Error?e.message:String(e)}}
    async powerAction(action:string):Promise<void>{this.systemError='';try{await TrueNASService.PowerAction(action)}catch(e){this.systemError=e instanceof Error?e.message:String(e)}}
    async refreshIdentity():Promise<void>{if(!this.connection?.connected||this.identityLoading)return;this.identityLoading=true;this.identityError='';try{this.identity=await TrueNASService.IdentityOverview()}catch(e){this.identityError=e instanceof Error?e.message:String(e)}finally{this.identityLoading=false}}
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
        await this.loadSavedServers();
    }
}

export function createAppContext(): AppContext {
    return setContext(APP_CONTEXT, new AppContext());
}

export function getAppContext(): AppContext {
    return getContext<AppContext>(APP_CONTEXT);
}
