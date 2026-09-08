#include <M5StickC.h>
#include <WiFi.h>
#include <WebSocketsClient.h>
#include <ArduinoJson.h>
#include <time.h>

const char *WIFI_SSID = "YOUR_WIFI_SSID";
const char *WIFI_PASSWORD = "YOUR_WIFI_PASSWORD";
const char *WS_HOST = "YOUR_BACKEND_LAN_IP";
// For the local kind NodePort demo. Use 443 with wss:// behind an Ingress in production.
const uint16_t WS_PORT = 8080;
const char *WS_PATH = "/api/v1/ws";
const char *WS_TOKEN = "";
const char *ROOM_ID = "room-1";
const char *DEVICE_ID = "m5stick-01";
// 空文字ならserver.readyで認証されたユーザーを対象にする。
// 固定する場合は、WS_TOKENで認証するユーザーIDと一致させる。
const char *TARGET_USER_ID = "";

WebSocketsClient websocket;
String currentStatus = "offline";
String pendingManualStatus = "";
String lastManualStatus = "";
String assignedUserId = "";
uint32_t lastReconnectAttempt = 0;
uint32_t buttonDownAt = 0;
uint32_t lastPendingAttempt = 0;
uint32_t lastLeaseHeartbeatAt = 0;
bool longPressSent = false;
uint64_t sequence = 0;
bool websocketReady = false;
bool manualLeaseActive = false;
bool backendTcpReachable = false;

const uint32_t LONG_PRESS_MS = 800;
const uint32_t MANUAL_LEASE_SECONDS = 30;
const uint32_t LEASE_HEARTBEAT_INTERVAL_MS = 10 * 1000;

uint16_t statusColor(const String &status) {
  if (status == "busy" || status == "working") return TFT_RED;
  if (status == "neutral") return TFT_YELLOW;
  if (status == "available" || status == "対応可能" || status == "idle" || status == "free") return TFT_GREEN;
  return TFT_DARKGREY;
}

bool isTargetUser(const char *userId) {
  if (userId == nullptr || userId[0] == '\0') return false;
  if (TARGET_USER_ID[0] != '\0') return strcmp(TARGET_USER_ID, userId) == 0;
  return assignedUserId.length() > 0 && assignedUserId.equals(userId);
}

bool isOwnManualState(JsonObject state) {
  const char *source = state["source"] | "";
  const char *clientId = state["clientId"] | "";
  return strcmp(source, "manual") == 0 && strcmp(clientId, DEVICE_ID) == 0;
}

void renderStatus(const String &status) {
  currentStatus = status;
  M5.Lcd.fillScreen(TFT_BLACK);
  M5.Lcd.setTextDatum(MC_DATUM);
  const int16_t centerX = M5.Lcd.width() / 2;
  const int16_t centerY = M5.Lcd.height() / 2 + 4;
  const uint16_t color = statusColor(status);

  // 状態は文字ではなく、色付きの丸で表示する。
  // 赤=したくない、黄色=いいよ、緑=したい、灰色=未接続/未確定。
  M5.Lcd.fillCircle(centerX, centerY, 27, color);
  M5.Lcd.drawCircle(centerX, centerY, 27, TFT_WHITE);
}

void setOffline() { renderStatus("offline"); }

void showWiFiFailure() {
  const int status = (int)WiFi.status();
  String detail;
  if (status == WL_NO_SSID_AVAIL) {
    detail = "SSID not found";
  } else if (status == WL_CONNECT_FAILED) {
    detail = "auth failed";
  } else if (status == WL_CONNECTION_LOST || status == WL_DISCONNECTED) {
    detail = "disconnected";
  } else {
    detail = String("code ") + String(status);
  }
  M5.Lcd.fillScreen(TFT_BLACK);
  M5.Lcd.setTextDatum(MC_DATUM);
  M5.Lcd.setTextColor(TFT_WHITE, TFT_BLACK);
  const int16_t centerX = M5.Lcd.width() / 2;
  M5.Lcd.drawString("OKIMOCHI", centerX, 14, 2);
  M5.Lcd.setTextColor(TFT_RED, TFT_BLACK);
  M5.Lcd.drawString("Wi-Fi offline", centerX, 33, 1);
  M5.Lcd.setTextColor(TFT_YELLOW, TFT_BLACK);
  M5.Lcd.drawString(detail, centerX, 50, 1);
  M5.Lcd.setTextColor(TFT_DARKGREY, TFT_BLACK);
  M5.Lcd.drawString("retrying", centerX, 68, 1);
}

void printWiFiDiagnostics(const char *phase) {
  String ip = WiFi.localIP().toString();
  Serial.printf("[WiFi] %s status=%d ip=%s rssi=%d\n", phase, (int)WiFi.status(), ip.c_str(), WiFi.RSSI());
}

String isoNow() {
  time_t now = time(nullptr);
  if (now < 1700000000) return "";
  struct tm utc;
  gmtime_r(&now, &utc);
  char value[32];
  strftime(value, sizeof(value), "%Y-%m-%dT%H:%M:%SZ", &utc);
  return String(value);
}

bool sendManualStatus(const String &status) {
  if (!websocketReady) {
    pendingManualStatus = status;
    return false;
  }
  String capturedAt = isoNow();
  if (capturedAt.length() == 0) {
    Serial.println("NTP時刻が未同期のため状態送信を保留");
    pendingManualStatus = status;
    return false;
  }

  JsonDocument update;
  update["type"] = "recognition.update";
  update["sequence"] = ++sequence;
  update["capturedAt"] = capturedAt;
  update["status"] = status;
  update["source"] = "manual";
  update["leaseSeconds"] = MANUAL_LEASE_SECONDS;
  String body;
  serializeJson(update, body);
  if (!websocket.sendTXT(body)) {
    pendingManualStatus = status;
    return false;
  }
  pendingManualStatus = "";
  lastManualStatus = status;
  manualLeaseActive = true;
  lastLeaseHeartbeatAt = millis();
  Serial.printf("手動状態送信: %s (#%llu)\n", status.c_str(), sequence);
  return true;
}

bool sendLeaseHeartbeat() {
  if (!websocketReady || !manualLeaseActive) return false;
  JsonDocument heartbeat;
  heartbeat["type"] = "status.heartbeat";
  heartbeat["leaseSeconds"] = MANUAL_LEASE_SECONDS;
  String body;
  serializeJson(heartbeat, body);
  if (!websocket.sendTXT(body)) {
    manualLeaseActive = false;
    return false;
  }
  lastLeaseHeartbeatAt = millis();
  return true;
}

void toggleManualStatus() {
  String next = currentStatus == "busy" ? "available" : "busy";
  sendManualStatus(next);
}

void onWebSocketEvent(WStype_t type, uint8_t *payload, size_t length) {
  switch (type) {
    case WStype_DISCONNECTED:
      if (manualLeaseActive && lastManualStatus.length() > 0) {
        pendingManualStatus = lastManualStatus;
      }
      websocketReady = false;
      manualLeaseActive = false;
      assignedUserId = "";
      if (WiFi.status() == WL_CONNECTED) {
        renderStatus(backendTcpReachable ? "ws fail" : "tcp fail");
      } else {
        showWiFiFailure();
      }
      break;
    case WStype_CONNECTED: {
      websocketReady = false;
      manualLeaseActive = false;
      assignedUserId = "";
      JsonDocument join;
      join["type"] = "client.hello";
      join["token"] = WS_TOKEN;
      join["roomId"] = ROOM_ID;
      join["clientId"] = DEVICE_ID;
      if (strcmp(WS_TOKEN, "YOUR_ACCESS_TOKEN") == 0 || WS_TOKEN[0] == '\0') {
        join["userId"] = DEVICE_ID;
      }
      String body;
      serializeJson(join, body);
      websocket.sendTXT(body);
      renderStatus(currentStatus == "offline" ? "unknown" : currentStatus);
      break;
    }
    case WStype_TEXT: {
      JsonDocument message;
      if (deserializeJson(message, payload, length)) return;
      const char *type = message["type"] | "";
      if (strcmp(type, "server.ready") == 0) {
        websocketReady = true;
        if (TARGET_USER_ID[0] != '\0') {
          assignedUserId = TARGET_USER_ID;
        } else {
          assignedUserId = String(message["userId"] | "");
        }
        JsonObject state = message["state"].as<JsonObject>();
        const char *stateUserId = state["userId"] | "";
        if (assignedUserId.length() == 0 && stateUserId[0] != '\0') {
          assignedUserId = stateUserId;
        }
        if (isTargetUser(stateUserId)) {
          const char *status = state["status"] | "";
          if (status[0] != '\0') renderStatus(status);
          manualLeaseActive = isOwnManualState(state);
          if (manualLeaseActive) {
            lastManualStatus = status;
            lastLeaseHeartbeatAt = millis();
          }
        } else {
          renderStatus("unknown");
        }
        if (pendingManualStatus.length() > 0) sendManualStatus(pendingManualStatus);
        return;
      }
      if (strcmp(type, "status.cleared") == 0) {
        const char *userId = message["userId"] | "";
        if (!isTargetUser(userId)) return;
        manualLeaseActive = false;
        renderStatus("unknown");
        return;
      }
      if (strcmp(type, "error") == 0) {
        const char *code = message["code"] | "";
        if (strcmp(code, "state_expired") == 0 || strcmp(code, "lease_owner_mismatch") == 0 ||
            strcmp(code, "stale_state") == 0 || strcmp(code, "validation_failed") == 0 ||
            strcmp(code, "read_only") == 0) {
          manualLeaseActive = false;
        }
        return;
      }
      if (strcmp(type, "status.changed") != 0) return;
      JsonObject state = message["state"].as<JsonObject>();
      const char *userId = state["userId"] | "";
      if (!isTargetUser(userId)) return;
      const char *status = state["status"] | "";
      if (status[0] == '\0') return;
      renderStatus(status);
      manualLeaseActive = isOwnManualState(state);
      if (manualLeaseActive) {
        lastManualStatus = status;
        lastLeaseHeartbeatAt = millis();
      }
      break;
    }
    default:
      break;
  }
}

void connectWiFi() {
  if (WIFI_SSID[0] == '\0' || strcmp(WIFI_SSID, "YOUR_WIFI_SSID") == 0) {
    Serial.println("[WiFi] SSID is not configured");
    setOffline();
    return;
  }

  Serial.printf("[WiFi] connecting to %s\n", WIFI_SSID);
  WiFi.mode(WIFI_STA);
  WiFi.begin(WIFI_SSID, WIFI_PASSWORD);
  M5.Lcd.fillScreen(TFT_BLACK);
  M5.Lcd.setTextDatum(MC_DATUM);
  M5.Lcd.setTextColor(TFT_WHITE, TFT_BLACK);
  M5.Lcd.drawString("OKIMOCHI", M5.Lcd.width() / 2, 18, 2);
  M5.Lcd.drawString("Wi-Fi connecting...", M5.Lcd.width() / 2, M5.Lcd.height() / 2, 1);
  const uint32_t startedAt = millis();
  while (WiFi.status() != WL_CONNECTED) {
    if (millis() - startedAt >= 15000) {
      printWiFiDiagnostics("timeout");
      showWiFiFailure();
      return;
    }
    delay(250);
  }
  printWiFiDiagnostics("connected");
  renderStatus("unknown");
  configTime(0, 0, "pool.ntp.org", "time.nist.gov");
}

void connectWebSocket() {
  websocket.begin(WS_HOST, WS_PORT, WS_PATH);
  websocket.onEvent(onWebSocketEvent);
  websocket.setReconnectInterval(3000);
  websocket.enableHeartbeat(15000, 3000, 2);
}

bool probeBackendTcp() {
  WiFiClient probe;
  const bool connected = probe.connect(WS_HOST, WS_PORT);
  Serial.printf("[TCP] %s:%u %s\n", WS_HOST, WS_PORT, connected ? "reachable" : "failed");
  probe.stop();
  return connected;
}

void setup() {
  Serial.begin(115200);
  delay(100);
  Serial.println("OKIMOCHI boot");
  M5.begin();
  M5.Lcd.setRotation(1);
  setOffline();
  connectWiFi();
  if (WiFi.status() == WL_CONNECTED) {
    backendTcpReachable = probeBackendTcp();
    renderStatus(backendTcpReachable ? "tcp ok" : "tcp fail");
    delay(800);
  }
  connectWebSocket();
}

void loop() {
  M5.update();
  websocket.loop();

  if (M5.BtnA.wasPressed()) {
    buttonDownAt = millis();
    longPressSent = false;
  }
  if (M5.BtnA.isPressed() && !longPressSent && millis() - buttonDownAt >= LONG_PRESS_MS) {
    longPressSent = true;
    toggleManualStatus();
  }

  if (websocketReady && pendingManualStatus.length() > 0 && millis() - lastPendingAttempt >= 1000) {
    lastPendingAttempt = millis();
    sendManualStatus(pendingManualStatus);
  }

  if (websocketReady && manualLeaseActive && millis() - lastLeaseHeartbeatAt >= LEASE_HEARTBEAT_INTERVAL_MS) {
    sendLeaseHeartbeat();
  }

  if (WiFi.status() != WL_CONNECTED && millis() - lastReconnectAttempt >= 3000) {
    lastReconnectAttempt = millis();
    WiFi.disconnect();
    WiFi.reconnect();
    showWiFiFailure();
  }
}
