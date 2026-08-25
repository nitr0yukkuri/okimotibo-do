export interface PairingSession {
  token: string;
  roomId: string;
  userId: string;
}

const storageKey = "okimochi-pairing-session";
const anonymousRoomKey = "okimochi-anonymous-room";
const anonymousUserKey = "okimochi-anonymous-user";

function isSafeId(value: string | null): value is string {
  return value !== null && /^[A-Za-z0-9._-]{1,128}$/.test(value);
}

function isPairingToken(value: string | null): value is string {
  return value !== null && /^pair_[A-Za-z0-9_-]{40,}$/.test(value);
}

function randomId(prefix: string): string {
  const id = typeof crypto.randomUUID === "function"
    ? crypto.randomUUID().replace(/-/g, "")
    : Math.random().toString(36).slice(2) + Date.now().toString(36);
  return `${prefix}-${id}`;
}

export function getAnonymousIdentity(): { roomId: string; userId: string } {
  try {
    const roomId = window.localStorage.getItem(anonymousRoomKey);
    const userId = window.localStorage.getItem(anonymousUserKey);
    if (isSafeId(roomId) && isSafeId(userId)) return { roomId, userId };
    const identity = { roomId: randomId("room"), userId: randomId("user") };
    window.localStorage.setItem(anonymousRoomKey, identity.roomId);
    window.localStorage.setItem(anonymousUserKey, identity.userId);
    return identity;
  } catch {
    return { roomId: randomId("room"), userId: randomId("user") };
  }
}

function readStoredPairing(): PairingSession | undefined {
  try {
    const raw = window.sessionStorage.getItem(storageKey);
    if (!raw) return undefined;
    const value = JSON.parse(raw) as Partial<PairingSession>;
    const token = value.token ?? null;
    const roomId = value.roomId ?? null;
    const userId = value.userId ?? null;
    if (isPairingToken(token) && isSafeId(roomId) && isSafeId(userId)) {
      return { token, roomId, userId };
    }
  } catch {
    // Storage can be unavailable in private browsing; the URL path still works.
  }
  return undefined;
}

export function readPairingSession(): PairingSession | undefined {
  const params = new URLSearchParams(window.location.search);
  const token = params.get("pair");
  const roomId = params.get("room");
  const userId = params.get("user");

  if (isPairingToken(token) && isSafeId(roomId) && isSafeId(userId)) {
    const pairing = { token, roomId, userId };
    try {
      window.sessionStorage.setItem(storageKey, JSON.stringify(pairing));
      const cleanUrl = `${window.location.pathname}${window.location.hash}`;
      window.history.replaceState({}, document.title, cleanUrl);
    } catch {
      // Keep the token in the URL if sessionStorage/history are unavailable.
    }
    return pairing;
  }

  return readStoredPairing();
}

export function clearPairingSession(): void {
  try {
    window.sessionStorage.removeItem(storageKey);
  } catch {
    // Ignore unavailable storage.
  }
}

export function pairingApiBase(websocketUrl?: string): string {
  const configured = websocketUrl?.trim() || import.meta.env.VITE_WS_URL?.trim();
  if (!configured) return window.location.origin;
  try {
    const url = new URL(configured.replace(/^ws/, "http"));
    const privateHost = url.hostname === "localhost" || url.hostname === "127.0.0.1" ||
      /^(10|192\.168|172\.(1[6-9]|2\d|3[0-1]))\./.test(url.hostname);
    // Viteの同一オリジンプロキシを使うと、スマホのトンネル経由でもlocalhostを誤参照しない。
    if (privateHost) return window.location.origin;
    url.pathname = url.pathname.replace(/\/api\/v1\/ws\/?$/, "");
    return url.toString().replace(/\/$/, "");
  } catch {
    return window.location.origin;
  }
}

export function buildPairingUrl(pairing: PairingSession): string {
  const configuredPublicUrl = import.meta.env.VITE_PUBLIC_APP_URL?.trim();
  let url: URL;
  try {
    url = new URL(configuredPublicUrl || window.location.href);
  } catch {
    url = new URL(window.location.href);
  }
  url.search = new URLSearchParams({
    pair: pairing.token,
    room: pairing.roomId,
    user: pairing.userId,
  }).toString();
  url.hash = "";
  return url.toString();
}
