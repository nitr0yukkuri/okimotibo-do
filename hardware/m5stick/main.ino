#include <M5StickCPlus2.h>
#include <WiFi.h>
#include <WebSocketsClient.h>
#include <ArduinoJson.h>
#include <time.h>

const char *WIFI_SSID = "YOUR_WIFI_SSID";
const char *WIFI_PASSWORD = "YOUR_WIFI_PASSWORD";
const char *WS_HOST = "192.168.0.10";
// For the local kind NodePort demo. Use 443 with wss:// behind an Ingress in production.
const uint16_t WS_PORT = 30080;
const char *WS_PATH = "/api/v1/ws";
const char *WS_TOKEN = "YOUR_ACCESS_TOKEN";
const char *ROOM_ID = "YOUR_ROOM_ID";
const char *DEVICE_ID = "m5stick-01";

WebSocketsClient websocket;
String currentStatus = "offline";
String pendingManualStatus = "";
uint32_t lastReconnectAttempt = 0;
uint32_t buttonDownAt = 0;
uint32_t lastPendingAttempt = 0;
bool longPressSent = false;
uint64_t sequence = 0;
bool websocketReady = false;

const uint32_t LONG_PRESS_MS = 800;
const uint32_t MANUAL_LEASE_SECONDS = 30;

uint16_t statusColor(const String &status) {
  if (status == "busy" || status == "working") return TFT_RED;
  if (status == "available" || status == "対応可能") return TFT_YELLOW;
  if (status == "idle" || status == "free") return TFT_GREEN;
  return TFT_DARKGREY;
}

void renderStatus(const String &status) {
  currentStatus = status;
  M5.Lcd.fillScreen(TFT_BLACK);
  M5.Lcd.setTextDatum(MC_DATUM);
  M5.Lcd.setTextColor(statusColor(status), TFT_BLACK);
  M5.Lcd.drawString(status, 67, 90, 2);
  M5.Lcd.setTextColor(TFT_WHITE, TFT_BLACK);
  M5.Lcd.drawString(WiFi.isConnected() ? "connected" : "offline", 67, 115, 1);
}

void setOffline() { renderStatus("offline"); }

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
  Serial.printf("手動状態送信: %s (#%llu)\n", status.c_str(), sequence);
  return true;
}

void toggleManualStatus() {
  String next = currentStatus == "busy" ? "available" : "busy";
  if (!sendManualStatus(next)) {
    renderStatus(next);
  }
}

void onWebSocketEvent(WStype_t type, uint8_t *payload, size_t length) {
  switch (type) {
    case WStype_DISCONNECTED:
      websocketReady = false;
      setOffline();
      break;
    case WStype_CONNECTED: {
      websocketReady = false;
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
        if (pendingManualStatus.length() > 0) sendManualStatus(pendingManualStatus);
        return;
      }
      if (strcmp(type, "status.cleared") == 0) {
        renderStatus("unknown");
        return;
      }
      if (strcmp(type, "status.changed") != 0) return;
      JsonObject state = message["state"].as<JsonObject>();
      const char *status = state["status"] | "";
      if (status[0] != '\0') renderStatus(status);
      break;
    }
    default:
      break;
  }
}

void connectWiFi() {
  WiFi.mode(WIFI_STA);
  WiFi.begin(WIFI_SSID, WIFI_PASSWORD);
  M5.Lcd.fillScreen(TFT_BLACK);
  M5.Lcd.setTextDatum(MC_DATUM);
  M5.Lcd.setTextColor(TFT_WHITE, TFT_BLACK);
  M5.Lcd.drawString("Wi-Fi connecting...", 67, 90, 1);
  while (WiFi.status() != WL_CONNECTED) delay(250);
  configTime(0, 0, "pool.ntp.org", "time.nist.gov");
}

void connectWebSocket() {
  websocket.begin(WS_HOST, WS_PORT, WS_PATH);
  websocket.onEvent(onWebSocketEvent);
  websocket.setReconnectInterval(3000);
  websocket.enableHeartbeat(15000, 3000, 2);
}

void setup() {
  auto config = M5.config();
  M5.begin(config);
  M5.Lcd.setRotation(1);
  setOffline();
  connectWiFi();
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

  if (WiFi.status() != WL_CONNECTED && millis() - lastReconnectAttempt >= 3000) {
    lastReconnectAttempt = millis();
    WiFi.disconnect();
    WiFi.reconnect();
    setOffline();
  }
}
