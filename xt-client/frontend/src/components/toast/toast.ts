export type ToastType = 'success' | 'error' | 'warning' | 'info' | 'loading';

export interface ToastAction {
  label: string;
  onClick: () => void;
}

export interface ToastOptions {
  id?: string;
  type?: ToastType;
  duration?: number;
  action?: ToastAction;
  onClose?: () => void;
}

export interface ToastItem {
  id: string;
  type: ToastType;
  message: string;
  action?: ToastAction;
  onClose?: () => void;
  createdAt: number;
  isLeaving?: boolean;
}

interface TimerState {
  timeoutId?: ReturnType<typeof setTimeout>;
  startTime: number;
  remainingTime: number;
}

const DEFAULT_DURATIONS: Record<ToastType, number> = {
  success: 3000,
  info: 3000,
  warning: 5000,
  error: 0,
  loading: 0,
};

const MAX_VISIBLE = 3;
const DEDUPE_WINDOW_MS = 1500;

class ToastManager {
  private toasts: ToastItem[] = [];
  private timers = new Map<string, TimerState>();
  private listeners = new Set<(toasts: ToastItem[]) => void>();
  private isHovered = false;
  private counter = 0;

  private notify() {
    const snapshot = [...this.toasts];
    for (const listener of this.listeners) {
      listener(snapshot);
    }
  }

  subscribe(listener: (toasts: ToastItem[]) => void) {
    this.listeners.add(listener);
    listener([...this.toasts]);
    return () => {
      this.listeners.delete(listener);
    };
  }

  getToasts(): ToastItem[] {
    return [...this.toasts];
  }

  private startTimer(id: string, duration: number) {
    this.clearTimer(id);
    if (duration <= 0) return;

    const timerState: TimerState = {
      startTime: Date.now(),
      remainingTime: duration,
    };

    if (!this.isHovered) {
      timerState.timeoutId = setTimeout(() => {
        this.dismiss(id);
      }, duration);
    }

    this.timers.set(id, timerState);
  }

  private clearTimer(id: string) {
    const state = this.timers.get(id);
    if (state?.timeoutId) {
      clearTimeout(state.timeoutId);
    }
    this.timers.delete(id);
  }

  pauseAll() {
    if (this.isHovered) return;
    this.isHovered = true;
    const now = Date.now();
    for (const [, state] of this.timers) {
      if (state.timeoutId) {
        clearTimeout(state.timeoutId);
        state.timeoutId = undefined;
        const elapsed = now - state.startTime;
        state.remainingTime = Math.max(0, state.remainingTime - elapsed);
      }
    }
  }

  resumeAll() {
    if (!this.isHovered) return;
    this.isHovered = false;
    const now = Date.now();
    for (const [id, state] of this.timers) {
      if (state.remainingTime > 0 && !state.timeoutId) {
        state.startTime = now;
        state.timeoutId = setTimeout(() => {
          this.dismiss(id);
        }, state.remainingTime);
      }
    }
  }

  show(type: ToastType, message: string, options?: ToastOptions): string {
    const existingIndex = this.toasts.findIndex(
      t => !t.isLeaving && t.type === type && t.message === message && Date.now() - t.createdAt < DEDUPE_WINDOW_MS
    );

    if (existingIndex !== -1) {
      const existing = this.toasts[existingIndex];
      const duration = options?.duration ?? (options?.action ? 0 : DEFAULT_DURATIONS[type]);
      this.startTimer(existing.id, duration);
      return existing.id;
    }

    const id = options?.id ?? `toast_${Date.now()}_${++this.counter}`;
    const duration = options?.duration ?? (options?.action ? 0 : DEFAULT_DURATIONS[type]);

    const item: ToastItem = {
      id,
      type,
      message,
      action: options?.action,
      onClose: options?.onClose,
      createdAt: Date.now(),
    };

    const activeToasts = this.toasts.filter(t => !t.isLeaving);
    if (activeToasts.length >= MAX_VISIBLE) {
      const oldest = activeToasts[0];
      this.dismiss(oldest.id);
    }

    this.toasts.push(item);
    this.startTimer(id, duration);
    this.notify();
    return id;
  }

  update(id: string, options: Partial<ToastOptions> & { message?: string }) {
    const index = this.toasts.findIndex(t => t.id === id);
    if (index === -1) return;

    const current = this.toasts[index];
    const updatedType = options.type ?? current.type;
    const updatedMessage = options.message ?? current.message;
    const updatedAction = options.action !== undefined ? options.action : current.action;

    this.toasts[index] = {
      ...current,
      type: updatedType,
      message: updatedMessage,
      action: updatedAction,
      onClose: options.onClose ?? current.onClose,
    };

    const duration = options.duration ?? (updatedAction ? 0 : DEFAULT_DURATIONS[updatedType]);
    this.startTimer(id, duration);
    this.notify();
  }

  dismiss(id?: string) {
    if (!id) {
      for (const t of this.toasts) {
        this.dismiss(t.id);
      }
      return;
    }

    const target = this.toasts.find(t => t.id === id);
    if (!target || target.isLeaving) return;

    target.isLeaving = true;
    this.clearTimer(id);
    this.notify();

    target.onClose?.();

    setTimeout(() => {
      this.toasts = this.toasts.filter(t => t.id !== id);
      this.notify();
    }, 200);
  }
}

export const toastManager = new ToastManager();

export const toast = {
  show: (message: string, options?: ToastOptions) => toastManager.show(options?.type ?? 'info', message, options),
  success: (message: string, options?: Omit<ToastOptions, 'type'>) => toastManager.show('success', message, options),
  error: (message: string, options?: Omit<ToastOptions, 'type'>) => toastManager.show('error', message, options),
  warning: (message: string, options?: Omit<ToastOptions, 'type'>) => toastManager.show('warning', message, options),
  info: (message: string, options?: Omit<ToastOptions, 'type'>) => toastManager.show('info', message, options),
  loading: (message: string, options?: Omit<ToastOptions, 'type'>) => toastManager.show('loading', message, options),
  dismiss: (id?: string) => toastManager.dismiss(id),
  update: (id: string, options: Partial<ToastOptions> & { message?: string }) => toastManager.update(id, options),
  promise: async <T>(
    promise: Promise<T>,
    messages: {
      loading: string;
      success: string | ((data: T) => string);
      error: string | ((err: unknown) => string);
    },
    options?: Omit<ToastOptions, 'type'>
  ): Promise<T> => {
    const id = toast.loading(messages.loading, options);
    try {
      const data = await promise;
      const msg = typeof messages.success === 'function' ? messages.success(data) : messages.success;
      toast.update(id, { type: 'success', message: msg });
      return data;
    } catch (err) {
      const msg = typeof messages.error === 'function' ? messages.error(err) : messages.error;
      toast.update(id, { type: 'error', message: msg });
      throw err;
    }
  },
};
