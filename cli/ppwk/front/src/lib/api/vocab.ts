import type { Action, Priority, Status } from './types';

/** 상태 이름은 한 곳에서만 옮긴다. 화면마다 적으면 반드시 어긋난다. */
export const statusLabel: Record<Status, string> = {
	open: '열림',
	claimed: '예약됨',
	working: '진행 중',
	blocked: '막힘',
	done: '완료',
	cancelled: '취소'
};

/** badge 의 variant 다. 진행 중과 막힘만 눈에 띄어야 한다. */
export const statusVariant: Record<Status, 'default' | 'secondary' | 'destructive' | 'outline'> = {
	open: 'outline',
	claimed: 'secondary',
	working: 'default',
	blocked: 'destructive',
	done: 'secondary',
	cancelled: 'outline'
};

export const priorityLabel: Record<Priority, string> = {
	high: '높음',
	med: '보통',
	low: '낮음',
	none: '백로그'
};

export const actionLabel: Record<Action, string> = {
	claim: '예약',
	start: '시작',
	done: '완료',
	block: '막기',
	unblock: '해제',
	release: '반납',
	cancel: '취소'
};

/**
 * allowed 는 이 상태에서 할 수 있는 전이다 (design §3.5).
 *
 * 서버가 최종 판정을 한다. 화면은 할 수 없는 버튼을 보여 주지 않기 위해서만
 * 이 표를 쓴다 — 눌러 보고 나서 안 된다고 듣는 것보다 낫다.
 */
export const allowed: Record<Status, Action[]> = {
	open: ['claim', 'start', 'cancel'],
	claimed: ['start', 'release', 'cancel'],
	working: ['done', 'block', 'cancel'],
	blocked: ['unblock', 'cancel'],
	done: [],
	cancelled: []
};

/** 되돌릴 수 없는 것은 확인을 받는다. */
export const destructive: Action[] = ['cancel'];

/** 상대 시각. 로컬 UI 라 정확한 시각보다 "얼마 전" 이 쓸모 있다. */
export function ago(iso: string): string {
	const then = new Date(iso).getTime();
	if (Number.isNaN(then)) return '';
	const seconds = Math.floor((Date.now() - then) / 1000);
	if (seconds < 60) return '방금';
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) return `${minutes}분 전`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}시간 전`;
	return `${Math.floor(hours / 24)}일 전`;
}
