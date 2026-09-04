import type {
	Action, Agent, DecisionEntry, Finding, HistoryEvent, Issue,
	IssueDetail, ListEntry, NextResult, PlanView, RefEvent, State
} from './types';

/** 서버가 분류한 오류다. 화면은 kind 로 어조를 정한다. */
export class ApiError extends Error {
	constructor(
		readonly kind: string,
		message: string,
		readonly status: number
	) {
		super(message);
	}

	/** 경쟁에서 밀린 것은 사용자의 잘못이 아니다. 다시 하면 된다. */
	get retryable() {
		return this.kind === 'cas_conflict';
	}
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const response = await fetch(path, {
		...init,
		headers: init?.body ? { 'content-type': 'application/json' } : undefined
	});
	const body = await response.json().catch(() => null);
	if (!response.ok || !body?.ok) {
		const error = body?.error;
		throw new ApiError(
			error?.kind ?? 'internal',
			error?.message ?? `${response.status} ${response.statusText}`,
			response.status
		);
	}
	return body.data as T;
}

type QueryValue = string | boolean | string[] | number | undefined;

function query(params: Record<string, QueryValue>) {
	const search = new URLSearchParams();
	for (const [key, value] of Object.entries(params)) {
		if (value === undefined || value === false || value === '') continue;
		if (value === true) search.append(key, '');
		else if (Array.isArray(value)) value.forEach((v) => search.append(key, v));
		else search.append(key, String(value));
	}
	const s = search.toString();
	return s ? `?${s}` : '';
}

export interface IssueFilter extends Record<string, QueryValue> {
	status?: string[];
	priority?: string[];
	owner?: string;
	label?: string;
	plan?: string;
	phase?: string;
	unassigned?: boolean;
	mine?: boolean;
	all?: boolean;
	archived?: boolean;
}

export const api = {
	state: () => request<State>('/api/state'),

	issues: (filter: IssueFilter = {}) => request<ListEntry[]>(`/api/issues${query(filter)}`),

	issue: (id: string) => request<IssueDetail>(`/api/issues/${id}`),

	history: (id: string) => request<HistoryEvent[]>(`/api/issues/${id}/history`),

	add: (input: {
		title: string;
		body?: string;
		priority?: string;
		labels?: string[];
		depends_on?: string[];
		plan?: string;
		phase?: string;
		seq?: number;
	}) => request<Issue>('/api/issues', { method: 'POST', body: JSON.stringify(input) }),

	transition: (id: string, action: Action, input: { message?: string; on?: string } = {}) =>
		request<Issue>(`/api/issues/${id}/actions/${action}`, {
			method: 'POST',
			body: JSON.stringify(input)
		}),

	candidates: (plan?: string) => request<Issue[]>(`/api/next${query({ plan })}`),

	claimNext: (plan?: string) =>
		request<NextResult>(`/api/next${query({ plan })}`, { method: 'POST' }),

	plans: () => request<PlanView[]>('/api/plans'),
	plan: (id: string) => request<PlanView>(`/api/plans/${id}`),

	decisions: (all = false) => request<DecisionEntry[]>(`/api/decisions${query({ all })}`),

	agents: () => request<Agent[]>('/api/agents'),
	fsck: () => request<Finding[]>('/api/fsck'),

	/**
	 * events 는 ref 변경을 흘려보낸다.
	 *
	 * 화면이 주기적으로 전체를 다시 묻는 것보다 낫다 — 무엇이 바뀌었는지
	 * 알면 그때만 다시 읽으면 된다.
	 */
	events(onEvent: (event: RefEvent) => void): () => void {
		const source = new EventSource('/api/events');
		source.onmessage = (message) => {
			try {
				onEvent(JSON.parse(message.data) as RefEvent);
			} catch {
				// 형식이 어긋난 줄 하나가 스트림을 끊지 않는다.
			}
		};
		return () => source.close();
	}
};
