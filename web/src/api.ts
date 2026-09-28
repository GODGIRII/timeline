export class ApiError extends Error {
  constructor(public status: number, message: string) { super(message); }
}

export async function api<T>(path: string, method = 'GET', body?: unknown): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`/api${path}`, {
      method, credentials: 'same-origin',
      headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(12000),
    });
  } catch { throw new ApiError(0, 'We couldn’t reach the server. Check your connection and try again.'); }
  let data;
  try { data = await response.json(); }
  catch { throw new ApiError(0, 'The server returned an unexpected response. Please try again.'); }
  if (!response.ok) {
    if (response.status === 401 && !path.startsWith('/auth/')) window.dispatchEvent(new Event('session-expired'));
    const message = response.status === 409 && path.includes('/items/') ? 'This item changed since you opened it. Reload the latest version before saving.' : data.error || 'Something went wrong.';
    throw new ApiError(response.status, message);
  }
  return data as T;
}

export const messageOf = (error: unknown) => error instanceof Error ? error.message : 'Something went wrong. Please try again.';
