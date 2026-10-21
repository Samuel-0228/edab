import { writable } from 'svelte/store';
import type { AppRoute, Workflow } from './types';
import { api } from './client';

export interface ToastNotification {
  id: string;
  type: 'success' | 'error' | 'warning' | 'info';
  title?: string;
  message: string;
  timestamp: number;
  duration?: number;
}

// --- Active Route Management (Hash-based) ---
function getInitialRoute(): AppRoute {
  if (typeof window === 'undefined') return 'overview';
  const hash = window.location.hash.replace(/^#\/?/, '').split('?')[0];
  const validRoutes: AppRoute[] = [
    'overview',
    'workflows',
    'builder',
    'integrations',
    'credentials',
    'executions',
    'failures',
    'events',
    'webhooks',
    'logs',
    'schedules',
    'health',
    'settings'
  ];
  return (validRoutes.includes(hash as AppRoute) ? hash : 'overview') as AppRoute;
}

export const currentRoute = writable<AppRoute>(getInitialRoute());

export function navigate(route: AppRoute, queryParams?: Record<string, string>): void {
  let hash = `#/${route}`;
  if (queryParams) {
    const q = new URLSearchParams(queryParams).toString();
    if (q) hash += `?${q}`;
  }
  if (typeof window !== 'undefined') {
    window.location.hash = hash;
  }
  currentRoute.set(route);
}

if (typeof window !== 'undefined') {
  window.addEventListener('hashchange', () => {
    currentRoute.set(getInitialRoute());
  });
}

// --- Refresh Trigger ---
export const refreshTick = writable<number>(0);
export function triggerRefresh(): void {
  refreshTick.update((n) => n + 1);
}

// --- Engine Connection Status ---
export interface EngineConnectionStatus {
  online: boolean | null; // null = pending check, true = connected, false = unreachable
  lastChecked: number;
  error?: string;
}

export const engineConnection = writable<EngineConnectionStatus>({
  online: null,
  lastChecked: 0,
  error: undefined
});

export function setEngineOnline(): void {
  engineConnection.set({
    online: true,
    lastChecked: Date.now(),
    error: undefined
  });
}

export function setEngineOffline(errorMsg?: string): void {
  engineConnection.set({
    online: false,
    lastChecked: Date.now(),
    error: errorMsg || 'Kerob engine offline or unreachable'
  });
}

// Hook API status changes into store
if (typeof api !== 'undefined' && api && typeof api.onStatusChange === 'function') {
  api.onStatusChange((online, err) => {
    if (online) {
      setEngineOnline();
    } else {
      setEngineOffline(err);
    }
  });
}

// --- Notification Toasts ---
export const notifications = writable<ToastNotification[]>([]);

export function addToast(
  type: ToastNotification['type'],
  message: string,
  title?: string,
  duration = 4500
): void {
  const id = Math.random().toString(36).substring(2, 9);
  const toast: ToastNotification = {
    id,
    type,
    title,
    message,
    timestamp: Date.now(),
    duration
  };

  notifications.update((items) => [toast, ...items]);

  if (duration > 0) {
    setTimeout(() => {
      dismissToast(id);
    }, duration);
  }
}

export function dismissToast(id: string): void {
  notifications.update((items) => items.filter((item) => item.id !== id));
}

export const notifySuccess = (msg: string, title?: string) => addToast('success', msg, title);
export const notifyError = (msg: string, title?: string) => addToast('error', msg, title);
export const notifyWarning = (msg: string, title?: string) => addToast('warning', msg, title);
export const notifyInfo = (msg: string, title?: string) => addToast('info', msg, title);

// --- Global Modal States ---
export interface WorkflowBuilderState {
  open: boolean;
  mode: 'create' | 'edit';
  workflow?: Workflow | null;
  preselectedIntegration?: string;
}

export const workflowBuilderModal = writable<WorkflowBuilderState>({
  open: false,
  mode: 'create',
  workflow: null
});

export function openCreateWorkflow(preselectedIntegration = ''): void {
  workflowBuilderModal.set({
    open: true,
    mode: 'create',
    workflow: null,
    preselectedIntegration
  });
}

export function openEditWorkflow(workflow: Workflow): void {
  workflowBuilderModal.set({
    open: true,
    mode: 'edit',
    workflow
  });
}

export function closeWorkflowBuilder(): void {
  workflowBuilderModal.set({
    open: false,
    mode: 'create',
    workflow: null
  });
}

// Dispatch / Test Modal
export interface DispatchModalState {
  open: boolean;
  workflowId?: string;
  workflowName?: string;
}

export const dispatchModal = writable<DispatchModalState>({
  open: false
});

export function openDispatchModal(workflowId?: string, workflowName?: string): void {
  dispatchModal.set({
    open: true,
    workflowId,
    workflowName
  });
}

export function closeDispatchModal(): void {
  dispatchModal.set({
    open: false
  });
}

// Credential Modal
export interface CredentialModalState {
  open: boolean;
  preselectedIntegration?: string;
}

export const credentialModal = writable<CredentialModalState>({
  open: false
});

export function openAddCredentialModal(integration = ''): void {
  credentialModal.set({
    open: true,
    preselectedIntegration: integration
  });
}

export function closeAddCredentialModal(): void {
  credentialModal.set({ open: false });
}

// Webhook Modal
export const webhookModal = writable<boolean>(false);
export function openCreateWebhookModal(): void {
  webhookModal.set(true);
}
export function closeCreateWebhookModal(): void {
  webhookModal.set(false);
}

// Execution Detail Modal
export interface ExecutionDetailState {
  open: boolean;
  executionId?: string;
}

export const executionDetailModal = writable<ExecutionDetailState>({
  open: false
});

export function openExecutionDetail(executionId: string): void {
  executionDetailModal.set({
    open: true,
    executionId
  });
}

export function closeExecutionDetail(): void {
  executionDetailModal.set({ open: false });
}

// Tour Modal
export const tourModal = writable<boolean>(false);
export function openTourModal(): void {
  tourModal.set(true);
}
export function closeTourModal(): void {
  tourModal.set(false);
}

// Confirm Dialog
export interface ConfirmModalState {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  danger?: boolean;
  onConfirm?: () => Promise<void> | void;
}

export const confirmModal = writable<ConfirmModalState>({
  open: false,
  title: '',
  message: ''
});

export function showConfirm(options: {
  title: string;
  message: string;
  confirmLabel?: string;
  danger?: boolean;
  onConfirm: () => Promise<void> | void;
}): void {
  confirmModal.set({
    open: true,
    ...options
  });
}

export function closeConfirm(): void {
  confirmModal.set({
    open: false,
    title: '',
    message: ''
  });
}

// Live Status Summary Cache (for Sidebar badges)
export interface SummaryCounts {
  activeWorkflows: number;
  totalWorkflows: number;
  deadLetters: number;
  executionsTotal: number;
  uptimeSeconds: number;
}

export const summaryCounts = writable<SummaryCounts>({
  activeWorkflows: 0,
  totalWorkflows: 0,
  deadLetters: 0,
  executionsTotal: 0,
  uptimeSeconds: 0
});

// App configuration store
export const apiConfig = writable<{ token: string; baseUrl: string }>({
  token: typeof api !== 'undefined' && api ? api.getToken() : '',
  baseUrl: typeof api !== 'undefined' && api ? api.getBaseUrl() : ''
});
