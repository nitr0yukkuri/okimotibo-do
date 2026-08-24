# おきもちぼーど 
カメラから手のジェスチャーと顔の表情をMediaPipeで読み取り、現在の状態をGo WebSocket APIでリアルタイム配信します。Py-Feat v2の表情APIはデモ用として利用できます。

## 状態の定義

| 手の形 | ジェスチャー | APIの状態 | 意味 |
|---|---|---|---|
| 親指を上 | `thumb_up` | `available` | 話しかけてよい |
| 親指と小指を伸ばす | `shaka` | `neutral` | 通常・反応可能 |
| 親指を下 | `thumb_down` | `busy` | 邪魔されたくない |

`thumb_up`と`thumb_down`はMediaPipe Gesture Recognizerの標準分類を利用します。標準分類にない`shaka`は21個の手ランドマークから指の開閉を判定します。

通常の顔認識はMediaPipe Face LandmarkerのBlendshapeを使います。笑顔または怒り寄りの表情が約2秒続いた場合に状態を変更し、変更後は4秒間固定します。それ以外の表情では状態を変更しません。手が検出されてから3秒間は手を優先します。最後に確定した状態は15分間有効です。表情は本人の感情を断定するものではありません。

## アーキテクチャ

```text
Camera
  -> React
     -> MediaPipe gesture classifier
     -> MediaPipe face classifier
     -> temporal stabilizer (hand priority)
  -> Go WebSocket API
     -> room broadcast
     -> Supabase Auth / Postgres

Demo only:
  React -> Python / Py-Feat v2 emotion API
```

通常のMediaPipe認識では画像をサーバーへ送りません。Py-Featのデモを有効にした場合だけ縮小JPEGをPython APIへ送り、APIは推論用の一時ファイルを処理直後に削除します。Go APIとSupabaseには画像を送りません。

## ディレクトリ

```text
frontend/   Reactから利用するMediaPipe認識ライブラリ（画面なし）
backend/    Go HTTP/WebSocket API
emotion-api/ Python / Py-Feat v2表情認識API
supabase/   PostgresマイグレーションとRLS
render.yaml Renderデプロイ設定
```

## フロントエンドライブラリ

### セットアップ

```bash
npm install
npm run build
npm test
```

### ローカル起動

リポジトリ直下で次を実行すると、フロントエンドを開発モードで起動できます。

```powershell
cd C:\src\okimotibo-do\2026-Team-02
npm run dev
```

通常は`http://localhost:5173/`で開きます。5173番ポートが使用中の場合は、ターミナルに表示された`Local`のURLを開いてください。終了するときは`Ctrl+C`を押します。

React側では表示用の`video`要素へのrefを渡します。ライブラリ自身はUIを描画しません。

```tsx
import { useRef } from "react";
import { useStateRecognition } from "@okimochi/recognition-client";

export function CameraRuntime() {
  const videoRef = useRef<HTMLVideoElement>(null);

  useStateRecognition(videoRef, {
    socket: {
      url: "ws://localhost:8080/api/v1/ws",
      token: "SUPABASE_ACCESS_TOKEN",
      roomId: "ROOM_UUID",
      clientId: crypto.randomUUID(),
      userId: "USER_UUID" // ローカル匿名モードで必要
    }
  });

  return <video ref={videoRef} hidden />;
}
```

本番ではモデルとWASMをCDNではなく同一オリジンに置き、`recognizer.wasmRoot`、`gestureModelUrl`、`faceModelUrl`で指定することを推奨します。

### 誤判定対策

- 推論は標準で10 FPS
- 信頼度`0.70`未満を除外
- 直近8フレーム中6フレームの一致で確定
- 一時的な未検出は3秒保持
- MediaPipeの顔判定は約2秒の継続で確定し、変更後は4秒間固定
- Py-Featのデモ判定は直近5回中3回の一致で確定し、`unknown`で候補をリセット
- 手の認識後3秒間は顔で状態を上書きしない
- 最終状態は既定で15分保持し、継続中の手ジェスチャーは1分ごとに有効期限を延長
- WebSocket送信は確定状態の変化時または1分ごとのheartbeatだけ
- 切断時は指数バックオフで再接続

## Go API

### ローカル起動

```powershell
cd backend
$env:ALLOW_ANONYMOUS="true"
go run ./cmd/server
```

本番では`ALLOW_ANONYMOUS=false`にして、以下を設定します。

| 環境変数 | 内容 |
|---|---|
| `PORT` | HTTPポート。既定値`8080` |
| `ALLOWED_ORIGINS` | 許可するOrigin。カンマ区切り |
| `ALLOW_ANONYMOUS` | ローカル開発時のみ`true` |
| `SUPABASE_URL` | Supabase Project URL |
| `SUPABASE_SECRET_KEY` | サーバー専用Secret key |
| `STATUS_TTL` | 最終状態の有効期間。`0`（既定値）は手動変更まで維持、指定時は最大`24h` |

Secret keyをReactへ含めてはいけません。Reactが使うのはSupabaseのpublishable keyとユーザーのaccess tokenだけです。

## Python表情API

Py-Feat v2の学習済みモデルは研究・非商用利用向けです。Python 3.11を使用します。

### 初回セットアップ

```powershell
cd C:\src\okimotibo-do\2026-Team-02
py -3.11 -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r emotion-api\requirements.txt
```

### 2回目以降の起動

リポジトリ直下で次を実行します。Py-Featはデモ用なので、利用するときだけ起動します。

```powershell
cd C:\src\okimotibo-do\2026-Team-02
$env:ALLOWED_ORIGINS="http://localhost:5173,http://127.0.0.1:5173,http://localhost:5176,http://127.0.0.1:5176"
.\.venv\Scripts\python.exe -m uvicorn app:app --app-dir emotion-api --host 127.0.0.1 --port 8000
```

起動後、`http://127.0.0.1:8000/healthz`を開いて応答を確認できます。終了するときは起動したターミナルで`Ctrl+C`を押します。

Pythonを直接入れずDockerで起動する場合:

```powershell
docker build -t okimochi-emotion .\emotion-api
docker run --rm -p 8000:8000 okimochi-emotion
```

初回推論時にPy-Featの学習済みモデルを取得するため時間がかかります。Reactは既定で`http://127.0.0.1:8000`へ接続します。変更する場合はフロントエンドの`.env.local`へ設定します。

```dotenv
VITE_EMOTION_API_URL=http://127.0.0.1:8000
```

Python APIの環境変数:

| 環境変数 | 内容 |
|---|---|
| `PORT` | HTTPポート。Dockerでは既定値`8000` |
| `ALLOWED_ORIGINS` | 許可するOrigin。カンマ区切り |
| `PYFEAT_DEVICE` | `cpu`または`cuda`。既定値`cpu` |
| `EMOTION_MIN_CONFIDENCE` | 赤・緑へ変更する最低信頼度。既定値`0.7` |

### HTTP

```text
GET /healthz
GET /readyz
GET /api/v1/rooms/{roomId}/status/{userId}
GET /api/v1/ws              WebSocket Upgrade
```

状態取得は本番モードでは`Authorization: Bearer <access-token>`が必要で、指定ルームのメンバーだけが取得できます。

### WebSocketプロトコル

接続直後、8秒以内に認証メッセージを送ります。

```json
{
  "type": "client.hello",
  "token": "SUPABASE_ACCESS_TOKEN",
  "roomId": "ROOM_UUID",
  "clientId": "BROWSER_INSTANCE_ID"
}
```

ローカル匿名モードでは`token`の代わりに`userId`を指定します。

認証後、サーバーが返します。

```json
{"type":"server.ready"}
```

状態更新は連番で送信します。

```json
{
  "type": "recognition.update",
  "sequence": 1,
  "capturedAt": "2026-07-19T10:30:15.420Z",
  "status": "available",
  "source": "hand",
  "hand": {
    "gesture": "thumb_up",
    "confidence": 0.91,
    "handedness": "Right"
  },
  "face": {
    "expression": "smile",
    "confidence": 0.76
  }
}
```

同じルームの接続へ`status.changed`が配信されます。メッセージ上限は16 KiB、古い連番、5分以上古い時刻、未来すぎる時刻、不正な信頼度は拒否されます。

各状態にはサーバー発行の`expiresAt`が含まれます。期限後、状態取得APIはその状態を返しません。

## Supabase

`supabase/migrations/001_initial.sql`をSupabase CLIまたはSQL Editorで適用します。

- `rooms`: ルーム
- `room_members`: 参加者とアクセス制御
- `current_statuses`: ルーム内ユーザーごとの最新状態
- `status_events`: 任意の履歴保存先

APIは`current_statuses`だけを更新します。映像、顔画像、全ランドマーク、全フレーム履歴は保存しません。

## デプロイ

### Render

リポジトリの`render.yaml`をBlueprintとして登録し、SupabaseとOriginの環境変数を設定します。WebSocket URLは`wss://<service>.onrender.com/api/v1/ws`です。

### Cloud Run

`backend/Dockerfile`をデプロイします。WebSocket接続はCloud Runのリクエストタイムアウト対象なので、タイムアウトを最大60分へ設定し、クライアントの自動再接続を有効にします。

### Vercel

Reactの配信先として利用できます。Go WebSocket APIは長時間接続との相性からRenderまたはCloud Runを推奨します。

## テスト観点

- 左手・右手、ブラウザの鏡像表示
- 手首を回転した状態と斜め方向
- 逆光、暗所、背景に手が写っている場合
- 指の一部が隠れた場合
- 状態を素早く切り替えた場合
- 顔のみ、手のみ、両方未検出
- WebSocket切断、再接続、重複連番
- 異なるルームへの配信漏れがないこと

ランドマーク規則だけでは手袋、強い遮蔽、極端な手首角度などに限界があります。実利用画像を収集できた段階で、匿名化したランドマークを使って`shaka`のカスタム分類器を学習するのが次の精度改善手段です。
