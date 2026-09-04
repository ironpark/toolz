import { api, ApiError } from './client';
import type { Agent, ListEntry, PlanView, State, Status } from './types';
import { toast } from 'svelte-sonner';

/**
 * BoardStore 는 화면이 공유하는 보드 상태다.
 *
 * ref 변경 스트림을 하나만 열고 거기서 갱신을 받는다. 화면마다 따로 polling
 * 하면 같은 것을 여러 번 묻게 되고, 화면 사이의 표시가 어긋난다.
 */
class BoardStore {
	state = $state<State | null>(null);
	issues = $state<ListEntry[]>([]);
	plans = $state<PlanView[]>([]);
	agents = $state<Agent[]>([]);
	loading = $state(true);
	/** connected 는 변경 스트림이 살아 있는지다. */
	connected = $state(false);
	error = $state<string | null>(null);

	#stop: (() => void) | null = null;
	/** 이벤트가 몰아칠 때 한 번만 다시 읽기 위한 예약이다. */
	#pending: ReturnType<typeof setTimeout> | null = null;

	async start() {
		await this.refresh();
		this.#stop?.();
		this.#stop = api.events(() => this.#scheduleRefresh());
		this.connected = true;
	}

	stop() {
		this.#stop?.();
		this.#stop = null;
		this.connected = false;
	}

	/**
	 * 이벤트 하나마다 다시 읽지 않는다.
	 *
	 * done 하나가 ref 두 개를 건드리고(issues 삭제 + archive 생성), 에이전트
	 * 여럿이 동시에 움직이면 한 주기에 수십 개가 온다. 짧게 모아서 한 번만
	 * 읽는다.
	 */
	#scheduleRefresh() {
		if (this.#pending) return;
		this.#pending = setTimeout(() => {
			this.#pending = null;
			void this.refresh();
		}, 150);
	}

	async refresh() {
		try {
			const [state, issues, plans, agents] = await Promise.all([
				api.state(),
				api.issues({ all: true }),
				api.plans(),
				api.agents()
			]);
			this.state = state;
			this.issues = issues;
			this.plans = plans;
			this.agents = agents;
			this.error = null;
		} catch (err) {
			this.error = err instanceof Error ? err.message : String(err);
		} finally {
			this.loading = false;
		}
	}

	/** mine 은 이 세션이 쥔 것이다. */
	get mine() {
		const agent = this.state?.agent;
		if (!agent) return [];
		return this.issues.filter((i) => i.owner === agent && !isTerminal(i.status));
	}

	/**
	 * run 은 변경 작업을 감싼다.
	 *
	 * 실패를 조용히 삼키지 않는다. 특히 CAS 경쟁은 흔하고 정상적인 사건이라
	 * 사용자에게 "다시 하면 된다" 는 것을 알려야 한다.
	 */
	async run<T>(what: string, fn: () => Promise<T>): Promise<T | null> {
		try {
			const result = await fn();
			await this.refresh();
			return result;
		} catch (err) {
			if (err instanceof ApiError && err.retryable) {
				toast.warning(`${what}: 다른 에이전트가 먼저 가져갔습니다`, {
					description: '다시 시도하세요'
				});
			} else {
				toast.error(what, {
					description: err instanceof Error ? err.message : String(err)
				});
			}
			await this.refresh();
			return null;
		}
	}
}

export function isTerminal(status: Status) {
	return status === 'done' || status === 'cancelled';
}

export const board = new BoardStore();
