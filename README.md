# おきもちぼーど Recognition API

カメラから手のジェスチャーと顔の動きをMediaPipeで読み取り、現在の状態をGo WebSocket APIでリアルタイム配信するヘッドレス実装です。UIは含みません。

## 状態の定義

| 手の形 | ジェスチャー | APIの状態 | 意味 |
|---|---|---|---|
| 親指を上 | `thumb_up` | `available` | 話しかけてよい |
| 親指と小指を伸ばす | `shaka` | `neutral` | 通常・反応可能 |
| 親指を下 | `thumb_down` | `busy` | 邪魔されたくない |

`thumb_up`と`thumb_down`はMediaPipe Gesture Recognizerの標準分類を利用します。標準分類にない`shaka`は21個の手ランドマークから指の開閉を判定します。

顔はFace LandmarkerのBlendshapeを使います。開始時に通常顔を約3秒学習し、そこからの眉・目・口の変化を組み合わせて`smile`、集中傾向（API上は互換性のため`frown`）、`surprised`、`neutral`として返します。手が確定している間は手を優先し、顔由来の状態変更は`smile`または集中傾向が約3秒続いた場合だけです。通常の無表情・驚き・顔未検出では状態を変更しません。最後に確定した状態は15分間有効で、同じ手の状態が続く場合は1分ごとのheartbeatで延長します。表情は本人の感情を断定するものではありません。

## アーキテクチャ

```text
Camera
  -> React / MediaPipe Tasks Vision
     -> 3 gesture classifier
     -> face blendshape classifier
     -> temporal stabilizer
  -> Go WebSocket API
     -> room broadcast
     -> Supabase Auth / Postgres
```

画像・映像・顔ランドマークはサーバーへ送りません。ブラウザ内で推論し、確定した状態と最小限のメタデータだけを送信します。

## ディレクトリ

```text
frontend/   Reactから利用するMediaPipe認識ライブラリ（画面なし）
backend/    Go HTTP/WebSocket API
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
- 顔は通常顔を30サンプル学習し、眉・目・口のうち複数の変化を合成
- 顔由来の状態変更は約3秒継続した場合だけ
- 顔の`neutral`や`surprised`では通常API状態を上書きしない
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
| `STATUS_TTL` | 最終状態の有効期間。既定値`15m`、最大`24h` |

Secret keyをReactへ含めてはいけません。Reactが使うのはSupabaseのpublishable keyとユーザーのaccess tokenだけです。

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
