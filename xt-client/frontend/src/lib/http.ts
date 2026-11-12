export class ApiError extends Error {
  readonly status: number;
  readonly code?: string;
  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
  }
}

export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {...init, headers: {'Content-Type': 'application/json', ...init?.headers}});
  if (!res.ok) {
    let message = `请求失败（${res.status}）`;
    let code: string | undefined;
    try {
      const body = await res.json();
      if (body?.error?.message) message = body.error.message;
      if (typeof body?.error?.code === 'string') code = body.error.code;
    } catch { /* keep status */ }
    throw new ApiError(message, res.status, code);
  }
  return res.json() as Promise<T>;
}