export async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {...init, headers: {'Content-Type': 'application/json', ...init?.headers}});
  if (!res.ok) {
    let message = `请求失败（${res.status}）`;
    try {const body = await res.json(); if (body?.error?.message) message = body.error.message;} catch { /* keep status */ }
    throw new Error(message);
  }
  return res.json() as Promise<T>;
}
