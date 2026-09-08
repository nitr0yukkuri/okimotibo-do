# M5StickC をおきもちぼーどにつなぐ

写真のオレンジ色の端末は **M5StickC** です。実機のUSBチップはFTDI、フラッシュは4MBでした。
`main.ino` はこの端末を、
おきもちぼーどの状態表示・状態変更リモコンとして使うスケッチです。

- 画面に `OKIMOCHI` と現在状態を表示
- Aボタンを0.8秒長押しすると `busy` / `available` を切り替え
- Wi-Fi経由でBackendへWebSocket送信
- 手動状態は30秒Leaseで保持し、10秒ごとにHeartbeatで延長
- PCやスマホで変更した状態も端末画面へ反映

## 1. Arduino IDEを準備する

M5Stack公式の [StickC Arduino手順](https://docs.m5stack.com/en/arduino/m5stickc/program) に沿って、次を準備します。

1. Arduino IDEをインストールする
2. ボードマネージャーでESP32のボード定義を入れる
3. ボードに `M5StickC` を選ぶ
4. ライブラリマネージャーで次を入れる
   - `M5StickC`
   - `WebSockets`（arduinoWebSockets / Markus Sattler）
   - `ArduinoJson`
5. Windowsで端末が `USB Serial Port` として認識されない場合は、FTDIのUSBドライバを入れる

USBケーブルは給電専用ではなく、データ通信できるものを使ってください。

## 2. スケッチを開く

Arduino IDEでこのファイルを開きます。

```text
hardware/m5stick/main.ino
```

ファイル上部の接続設定だけ、自分の環境に合わせて変更します。

```cpp
const char *WIFI_SSID = "自分のWi-Fi名";
const char *WIFI_PASSWORD = "自分のWi-Fiパスワード";
const char *WS_HOST = "Backendを起動しているPCのLAN IP";
const uint16_t WS_PORT = 8080; // go run ./cmd/server の場合
const char *WS_TOKEN = "";     // 匿名ローカル検証では空文字
const char *ROOM_ID = "room-1";
```

`WS_HOST` に `localhost` は指定しません。M5Stickから見たPCのアドレスが必要です。
WindowsではPowerShellで `ipconfig` を実行し、Wi-FiのIPv4アドレスを使います。

接続先がkindのNodePortの場合は、次のようにします。

```cpp
const char *WS_HOST = "192.168.x.x"; // kindを動かしているPCのLAN IP
const uint16_t WS_PORT = 30080;
```

Supabase認証を使う場合は `WS_TOKEN` にアクセストークンを入れ、
そのトークンのユーザーが `ROOM_ID` のメンバーであることを確認します。
アクセストークンやWi-Fiパスワードをコミットしないでください。

## 3. Backendを起動する

まずPC側で匿名ローカル検証用のBackendを起動します。

```powershell
cd C:\src\okimotibo-do\2026-Team-02\backend
$env:ALLOW_ANONYMOUS="true"
$env:SUPABASE_URL=""
$env:SUPABASE_SECRET_KEY=""
go run ./cmd/server
```

この場合はスケッチの `WS_TOKEN` を空文字、`ROOM_ID` を `room-1` にします。
PCで `go run ./cmd/server` を使う場合のポートは `8080` です。

## 4. 書き込む

端末をUSBで接続し、Arduino IDEで次を選びます。

1. `ツール` → `ボード` → `M5StickCPlus`
2. `ツール` → `ポート` → `COM番号`
3. 上向き矢印の「書き込み」を押す

COM番号は、Windowsの「デバイス マネージャー」→「ポート (COMとLPT)」で確認できます。
`USB Serial Port (COMx)` と表示され、FTDIのVID/PIDが `0403:6001` なら認識できています。

書き込み後、画面に `OKIMOCHI` が表示されます。しばらくするとWi-Fi接続状態が表示され、
接続できれば `connected` になります。

## 5. 動作確認

1. おきもちぼーどをPCで開く
2. M5StickのAボタンを0.8秒長押しする
3. M5Stickの画面が `busy` と `available` で切り替わることを確認
4. PCやスマホのおきもちぼーどにも同じ状態が反映されることを確認

`offline` のままなら、Wi-Fi名・パスワード、`WS_HOST`、Backendのポート、Windows Defender
ファイアウォール、M5StickとPCが同じネットワークにいるかを確認します。

## 参考

- [M5StickC公式Arduino手順](https://docs.m5stack.com/en/arduino/m5stickc/program)
- [M5Stack/M5StickC公式ライブラリ](https://github.com/m5stack/M5StickC)
