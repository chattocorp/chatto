// Minimal GraphQL HTTP client with cookie jar support.
// Each Client holds the session cookies and bearer token for one persona.

export type GqlError = {
  message: string;
  path?: (string | number)[];
  extensions?: Record<string, unknown>;
};

export type GqlResponse<T = unknown> = {
  data?: T | null;
  errors?: GqlError[];
};

export class Client {
  private cookies = new Map<string, string>();
  private bearer?: string;

  constructor(public endpoint: string, public label: string) {}

  setBearer(token: string) {
    this.bearer = token;
  }

  hasSession(): boolean {
    return this.cookies.size > 0 || !!this.bearer;
  }

  private cookieHeader(): string | undefined {
    if (this.cookies.size === 0) return undefined;
    return [...this.cookies.entries()].map(([k, v]) => `${k}=${v}`).join("; ");
  }

  private absorbSetCookie(headers: Headers) {
    // Node's fetch returns a flat Headers object that joins multiple Set-Cookie
    // with comma. We only need the first cookie per name, and Chatto only sets
    // one auth cookie ("session"), so the simplest split is fine for our use.
    const raw = headers.get("set-cookie");
    if (!raw) return;
    // Split on ", " only when followed by a Cookie name (heuristic but ok here).
    const parts = raw.split(/,(?=\s*[A-Za-z0-9_-]+=)/);
    for (const part of parts) {
      const [pair] = part.split(";");
      const eq = pair.indexOf("=");
      if (eq < 0) continue;
      const name = pair.slice(0, eq).trim();
      const value = pair.slice(eq + 1).trim();
      this.cookies.set(name, value);
    }
  }

  async http(path: string, init: RequestInit = {}): Promise<Response> {
    const headers = new Headers(init.headers);
    headers.set("content-type", "application/json");
    const cookie = this.cookieHeader();
    if (cookie) headers.set("cookie", cookie);
    if (this.bearer) headers.set("authorization", `Bearer ${this.bearer}`);
    const res = await fetch(this.endpoint + path, { ...init, headers });
    this.absorbSetCookie(res.headers);
    return res;
  }

  async query<T = unknown>(query: string, variables?: Record<string, unknown>): Promise<GqlResponse<T>> {
    const res = await this.http("/api/graphql", {
      method: "POST",
      body: JSON.stringify({ query, variables }),
    });
    if (!res.ok && res.status !== 200) {
      // GraphQL endpoints can return non-200 for transport errors; surface them.
      return { errors: [{ message: `HTTP ${res.status} ${res.statusText}` }] };
    }
    return (await res.json()) as GqlResponse<T>;
  }

  async login(login: string, password: string): Promise<void> {
    const res = await this.http("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ login, password }),
    });
    if (!res.ok) {
      const body = await res.text();
      throw new Error(`[${this.label}] login failed: HTTP ${res.status}: ${body}`);
    }
    const body = (await res.json()) as { token?: string };
    if (body.token) this.bearer = body.token;
  }
}
