/** Breaks circular deps: API client refreshes tokens via auth state. */

type Tokens = { access: string; refresh: string };

let getRefreshToken: () => string | null = () => null;
let applyTokens: (t: Tokens) => void = () => {};

export function registerAuthBridge(getRefresh: () => string | null, apply: (t: Tokens) => void) {
  getRefreshToken = getRefresh;
  applyTokens = apply;
}

export function getStoredRefreshToken() {
  return getRefreshToken();
}

export function storeRefreshedTokens(access: string, refresh: string) {
  applyTokens({ access, refresh });
}
