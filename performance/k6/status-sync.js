import { WebSocket } from 'k6/websockets';
import { Counter, Rate, Trend } from 'k6/metrics';

const wsUrl = __ENV.WS_URL || 'ws://127.0.0.1:8080/api/v1/ws';
const roomId = __ENV.ROOM_ID || 'k6-room';
const configuredRunId = __ENV.RUN_ID || '';
const configuredUserId = __ENV.USER_ID || '';
const publishToken = __ENV.PUBLISH_TOKEN || __ENV.TOKEN || '';
const viewerToken = __ENV.VIEWER_TOKEN || __ENV.TOKEN || '';
const viewerVUs = positiveInteger(__ENV.VIEWERS, 10);
const duration = __ENV.DURATION || '30s';
const publisherStart = __ENV.PUBLISHER_START || '3s';
const sessionDurationMs = positiveInteger(__ENV.SESSION_MS, 25000);
const publishIntervalMs = positiveInteger(__ENV.PUBLISH_INTERVAL_MS, 2000);
const heartbeatIntervalMs = positiveInteger(__ENV.HEARTBEAT_INTERVAL_MS, 10000);
const leaseSeconds = positiveInteger(__ENV.LEASE_SECONDS, 30);
const propagationP95BudgetMs = positiveInteger(__ENV.PROPAGATION_P95_MS, 1000);

if (!isLocalWebSocket(wsUrl) && __ENV.ALLOW_REMOTE !== 'true') {
  throw new Error(
    'リモート負荷試験を実行する場合は ALLOW_REMOTE=true を明示してください。' +
      '本番URLをデフォルトで叩かないための安全策です。',
  );
}

const wsReadyRate = new Rate('ws_ready_rate');
const viewerReceivedRate = new Rate('viewer_received_rate');
const serverErrorRate = new Rate('server_error_rate');
const statusPropagationMs = new Trend('status_propagation_ms', true);
const statusUpdatesSent = new Counter('status_updates_sent');
const heartbeatsSent = new Counter('heartbeats_sent');
const sequenceRegressions = new Counter('status_sequence_regressions');
const timestampRegressions = new Counter('status_timestamp_regressions');
const invalidPropagationTimestamps = new Counter('invalid_propagation_timestamps');

export const options = {
  scenarios: {
    viewers: {
      executor: 'per-vu-iterations',
      vus: viewerVUs,
      iterations: 1,
      maxDuration: duration,
      exec: 'viewer',
      gracefulStop: '5s',
    },
    publisher: {
      executor: 'per-vu-iterations',
      vus: 1,
      iterations: 1,
      maxDuration: duration,
      startTime: publisherStart,
      exec: 'publisher',
      gracefulStop: '5s',
    },
  },
  thresholds: {
    ws_ready_rate: ['rate>0.99'],
    viewer_received_rate: ['rate>0.99'],
    server_error_rate: ['rate<0.01'],
    status_propagation_ms: [`p(95)<${propagationP95BudgetMs}`],
    status_sequence_regressions: ['count<1'],
    status_timestamp_regressions: ['count<1'],
    invalid_propagation_timestamps: ['count<1'],
    status_updates_sent: ['count>0'],
  },
};

export function setup() {
  const runId = safeIdentifier(configuredRunId || `run-${Date.now()}-${Math.floor(Math.random() * 1000000)}`);
  return {
    runId,
    userId: configuredUserId || `k6-user-${runId}`,
  };
}

export function publisher(testData) {
  const clientId = `k6-${testData.runId}-publisher-${__VU}`;
  const ws = new WebSocket(wsUrl);
  let connected = false;
  let ready = false;
  let readyRecorded = false;
  let sequence = Date.now() * 1000 + __VU;
  let updateIntervalId;
  let heartbeatIntervalId;
  let closeTimeoutId;

  ws.addEventListener('open', () => {
    connected = true;
    ws.send(JSON.stringify(helloMessage(clientId, publishToken, testData.userId)));
  });

  ws.addEventListener('message', (event) => {
    const message = parseMessage(event.data);
    if (message === null) {
      serverErrorRate.add(true);
      return;
    }

    serverErrorRate.add(message.type === 'error');
    if (message.type === 'server.ready' && !ready) {
      ready = true;
      markReady(wsReadyRate, () => {
        readyRecorded = true;
      }, readyRecorded);

      sendStatusUpdate(ws, () => sequence++);
      updateIntervalId = setInterval(() => {
        if (connected && ready) {
          sendStatusUpdate(ws, () => sequence++);
        }
      }, publishIntervalMs);
      heartbeatIntervalId = setInterval(() => {
        if (connected && ready) {
          ws.send(JSON.stringify({ type: 'status.heartbeat', leaseSeconds }));
          heartbeatsSent.add(1);
        }
      }, heartbeatIntervalMs);
    }
  });

  ws.addEventListener('error', () => {
    connected = false;
    serverErrorRate.add(true);
  });

  ws.addEventListener('close', () => {
    connected = false;
    if (!readyRecorded) {
      wsReadyRate.add(false);
      readyRecorded = true;
    }
    clearInterval(updateIntervalId);
    clearInterval(heartbeatIntervalId);
    clearTimeout(closeTimeoutId);
  });

  closeTimeoutId = setTimeout(() => {
    connected = false;
    clearInterval(updateIntervalId);
    clearInterval(heartbeatIntervalId);
    ws.close();
  }, sessionDurationMs);
}

export function viewer(testData) {
  const clientId = `k6-${testData.runId}-viewer-${__VU}`;
  const ws = new WebSocket(wsUrl);
  let readyRecorded = false;
  let receivedStatus = false;
  let resultRecorded = false;
  let effectiveUserId = testData.userId;
  let latestSequence = 0;
  let latestReceivedAtMs = 0;
  let closeTimeoutId;

  ws.addEventListener('open', () => {
    ws.send(JSON.stringify(helloMessage(clientId, viewerToken, testData.userId)));
  });

  ws.addEventListener('message', (event) => {
    const message = parseMessage(event.data);
    if (message === null) {
      serverErrorRate.add(true);
      return;
    }

    serverErrorRate.add(message.type === 'error');
    if (message.type === 'server.ready') {
      if (message.userId) {
        effectiveUserId = message.userId;
      }
      markReady(wsReadyRate, () => {
        readyRecorded = true;
      }, readyRecorded);
      return;
    }

    if (message.type !== 'status.changed' || !message.state) {
      return;
    }
    if (message.state.userId !== effectiveUserId) {
      return;
    }

    receivedStatus = true;
    const sequence = Number(message.state.sequence);
    if (Number.isFinite(sequence)) {
      if (sequence < latestSequence) {
        sequenceRegressions.add(1);
      } else {
        latestSequence = sequence;
      }
    }

    const receivedAtMs = Date.parse(message.state.receivedAt);
    if (Number.isFinite(receivedAtMs)) {
      if (receivedAtMs < latestReceivedAtMs) {
        timestampRegressions.add(1);
      } else {
        latestReceivedAtMs = receivedAtMs;
      }
    }

    const capturedAtMs = Date.parse(message.state.capturedAt);
    const propagationMs = Date.now() - capturedAtMs;
    if (!Number.isFinite(capturedAtMs) || propagationMs < 0) {
      invalidPropagationTimestamps.add(1);
    } else {
      statusPropagationMs.add(propagationMs);
    }
  });

  ws.addEventListener('error', () => {
    serverErrorRate.add(true);
  });

  ws.addEventListener('close', () => {
    if (!readyRecorded) {
      wsReadyRate.add(false);
      readyRecorded = true;
    }
    recordViewerResult(() => {
      resultRecorded = true;
    }, resultRecorded, receivedStatus);
    clearTimeout(closeTimeoutId);
  });

  closeTimeoutId = setTimeout(() => {
    ws.close();
  }, sessionDurationMs);
}

function helloMessage(clientId, token, targetUserId) {
  const message = {
    type: 'client.hello',
    token,
    roomId,
    clientId,
  };
  // 匿名モードだけuserIdを送る。アクセストークン利用時はサーバー側の認証結果を使う。
  if (!token) {
    message.userId = targetUserId;
  }
  return message;
}

function sendStatusUpdate(ws, nextSequence) {
  const sequence = nextSequence();
  const statuses = ['available', 'neutral', 'busy'];
  ws.send(
    JSON.stringify({
      type: 'recognition.update',
      sequence,
      capturedAt: new Date().toISOString(),
      status: statuses[sequence % statuses.length],
      source: 'manual',
      leaseSeconds,
    }),
  );
  statusUpdatesSent.add(1);
}

function parseMessage(data) {
  if (typeof data !== 'string') {
    return null;
  }
  try {
    return JSON.parse(data);
  } catch (_) {
    return null;
  }
}

function markReady(metric, markRecorded, alreadyRecorded) {
  if (alreadyRecorded) {
    return;
  }
  metric.add(true);
  markRecorded();
}

function recordViewerResult(markRecorded, alreadyRecorded, received) {
  if (alreadyRecorded) {
    return;
  }
  viewerReceivedRate.add(received);
  markRecorded();
}

function positiveInteger(value, fallback) {
  const parsed = Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function isLocalWebSocket(value) {
  return /^wss?:\/\/(localhost|127\.0\.0\.1|\[::1\])(?::\d+)?(?:\/|$)/.test(value);
}

function safeIdentifier(value) {
  return String(value).replace(/[^A-Za-z0-9_.-]/g, '-').slice(0, 40);
}
