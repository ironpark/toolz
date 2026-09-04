export type Status =
	| 'open'
	| 'claimed'
	| 'working'
	| 'blocked'
	| 'done'
	| 'cancelled';

export type Priority = 'high' | 'med' | 'low' | 'none';

export type Action =
	| 'claim'
	| 'start'
	| 'done'
	| 'block'
	| 'unblock'
	| 'release'
	| 'cancel';

export interface ListEntry {
	id: string;
	status: Status;
	owner?: string;
	priority: Priority;
	plan?: string;
	phase?: string;
	seq?: number;
	title: string;
	ref: string;
	commit: string;
}

export interface Issue {
	schema: number;
	id: string;
	title: string;
	status: Status;
	priority: Priority;
	labels?: string[];
	owner?: string;
	session?: string;
	plan?: string;
	phase?: string;
	seq?: number;
	depends_on?: string[];
	blocked_by?: string[];
	created_at: string;
	updated_at: string;
	updated_by: string;
}

export interface DecisionEntry {
	id: string;
	title: string;
	superseded_by?: string;
	ref: string;
	commit: string;
}

export interface IssueDetail {
	issue: Issue;
	body: string;
	ref: string;
	commit: string;
	archived: boolean;
	decisions: DecisionEntry[];
}

export interface HistoryEvent {
	commit: string;
	short: string;
	when: string;
	who: string;
	subject: string;
}

export interface Phase {
	id: string;
	title: string;
	gate: 'all_done' | 'any_done' | 'manual';
}

export interface PhaseView extends Phase {
	done: number;
	total: number;
	open: boolean;
	state: string;
	current: boolean;
	tasks: ListEntry[] | null;
}

export interface PlanView {
	plan: {
		id: string;
		title: string;
		status: string;
		priority: Priority;
		phases: Phase[];
		advanced_phases: string[];
	};
	phases: PhaseView[];
	done: number;
	total: number;
}

export interface Agent {
	agent: string;
	session: string;
	worktree: string;
	since: string;
	last_activity: string;
	hook_pid: number | null;
	alive: boolean;
}

export interface State {
	agent: string;
	session: string;
	worktree: string;
	schema: number;
	read_only: boolean;
}

export interface Finding {
	check: string;
	level: string;
	id?: string;
	message: string;
}

export interface RefEvent {
	ref: string;
	old?: string;
	new?: string;
	kind: 'created' | 'updated' | 'deleted';
	id?: string;
	status?: Status;
}

export interface NextResult {
	candidates: Issue[] | null;
	claimed?: Issue;
	attempts?: number;
}
