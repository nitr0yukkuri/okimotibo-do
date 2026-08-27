# おきもちぼ〜ど

PC作業中の「今は話しかけてほしい／ほしくない」を、手のジェスチャーや表情から読み取り、同じルームの画面へリアルタイムに共有するプレゼンスボードです。

AI認識はブラウザ内のMediaPipeを基本とし、PC側で確定した状態をGoのWebSocket API経由でスマホ表示へ配信します。通常の認識経路ではカメラ映像をサーバーへ送信しません。

- 公開アプリ: https://okimotibo-public.vercel.app/
- 公開リポジトリ: https://github.com/nitr0yukkuri/okimotibo-do

## 解決する課題

作業中に話しかけられるかどうかは、相手の状況を見ながら推測するしかありません。おきもちぼ〜どは、本人の明示的な手ジェスチャーを中心に、表情を補助信号として使い、周囲から見える状態へ変換します。

想定する利用場所は、個人のデスク、研究室、学校、共有オフィス、リモートワークなどです。

## 現在の利用フロー

1. PCでGoogleアカウントにログインする。
2. PCのカメラに手のジェスチャーを向ける、またはPC画面のボタンを押す。
3. PC側で確定した状態を同じルームへ送信する。
4. スマホは同じアカウント・同じルームの状態を読み取り、周囲から見える表示として使う。
5. 状態はWebSocketの切断・再接続や画面復帰時にも、現在値APIから再取得する。

ログイン済みユーザーはSupabaseのroom_membersから所属ルームを取得します。スマホ画面は現在、表示専用です。スマホから状態を変更する操作は持たせず、PC側のカメラ認識または手動ボタンを状態の主な入力にしています。
## 状態の定義

| 入力 | ジェスチャー／表情 | APIの状態 | 画面上の意味 |
|---|---|---|---|
| 親指を上 | thumb_up | available | したい／ひま！ |
| 横向きの親指 | sideways_thumb | neutral | いいよ／反応可能 |
| 親指と小指 | shaka | neutral | いいよ／反応可能 |
| 親指を下 | thumb_down | busy | したくない／作業中 |
| 笑顔（補助） | smile | available | したい |
| 集中傾向（補助） | frown | busy | したくない |

顔のfrownは感情を断定するものではありません。眉・目・口など複数のBlendshapeから「集中している可能性」を補助的に判定しています。明示的な手ジェスチャーを優先します。

## 主な機能

### PC操作画面

- 手ジェスチャーによる状態更新
- 画面上の3状態ボタンによる手動更新
- カメラ映像のテスト表示
- 表情読み取りのON／OFF
- ChromeのDocument Picture-in-Pictureによる常時表示ミニパネル
- Google OAuthログイン
- ログアウトと再接続

### スマホ表示画面

- スマホまたは狭い画面幅では表示専用画面を表示
- PCから届いた状態を背景色・アイコン・文言で表示
- PWAとしてホーム画面へ追加可能
- スマホ側のカメラや画像は使用しない

### 同期

- 同じアカウントが同じルームを開くと、PCとスマホで同じ状態を共有
- PCのカメラ認識と手動ボタンはWebSocketで配信
- 手動ボタンの状態は無期限に維持し、顔認識より優先。明示的な手ジェスチャーでは変更できる
- スマホはWebSocketのstatus.changedを受信し、接続時・再接続時・画面復帰時に現在状態を再取得
- 同じユーザーの古い状態が新しい状態を上書きしないよう、sequenceとcapturedAtを検証

バックエンドにはQR／合言葉による一時ペアリングAPIもあります。ペアリングの有効期限は10分、合言葉は1回だけ使用でき、ペアリング先は読み取り専用です。現在の主導線は同一アカウントでのログインです。

## AI認識の仕組み

### 手の認識

ブラウザ内のMediaPipe Gesture Recognizerと、21点の手ランドマークを使うカスタム分類を組み合わせています。

- thumb_up／thumb_downはMediaPipeの標準分類を利用
- shakaは親指・人差し指・中指・薬指・小指の開閉をランドマークから判定
- sideways_thumbは親指の方向をランドマークから判定
- 握りこぶしは状態更新から除外
- 推論間隔は標準100ms（約10 FPS）
- 標準分類の最低信頼度は0.70
- 直近6サンプル中4サンプルの一致で状態を確定
- 一時的な未検出は1秒まで直前の状態を保持
- 手の認識後3秒間は顔認識で上書きしない

現在の親指の上下判定は、カメラ画像上のY方向を基準にしています。そのため、手首を大きく回転させた状態では横向き判定が不安定になる場合があります。

### 顔の認識

顔認識はMediaPipe Face LandmarkerのBlendshapeをブラウザ内で処理します。

- 初回の5サンプルを本人の基準値としてキャリブレーション
- smileは信頼度0.55以上
- frownは信頼度0.35以上
- 候補が約1.5秒継続したときに確定
- 確定後は4秒間ロック
- 表情がニュートラルなときは基準値を少しずつ更新
- 表情は手ジェスチャーがないときの補助入力

この認識は「本人が本当にその感情である」と断定するものではなく、画面の状態を切り替えるための入力信号です。

### 任意のPy-Feat v2 API

emotion-apiにはPy-Feat v2を使った画像ベースの表情APIを用意しています。ただし、現在の通常画面のAppはこのAPIを自動利用せず、MediaPipeのブラウザ内認識を使います。

Py-Featを明示的に有効化したライブラリ利用では、最大幅640pxに縮小したJPEGをAPIへ送信します。APIは画像を一時ファイルとして処理し、推論後に削除します。Py-Feat v2の学習済みモデルは研究・非商用利用向けです。

## アーキテクチャ

~~~
PCブラウザ
  ├─ カメラ
  │   ├─ MediaPipe Gesture Recognizer
  │   ├─ 手ランドマーク分類
  │   └─ MediaPipe Face Landmarker
  ├─ 手動ボタン
  └─ StateSocket
       │ recognition.update
       ▼
Go HTTP / WebSocket API
  ├─ 認証・ルーム権限確認
  ├─ sequence・時刻・信頼度の検証
  ├─ 現在状態の永続化
  └─ 同じルームへの status.changed 配信
       ├─ スマホ表示画面
       ├─ PCの別ウィンドウ
       └─ M5Stick試作クライアント

Supabase Auth / Postgres
  ├─ rooms
  ├─ room_members
  ├─ current_statuses
  └─ pairing_grants

任意のデモ経路:
  ブラウザ画像 → Python / Py-Feat v2 API
~~~

## データとプライバシー

通常のMediaPipe認識では、カメラ映像・顔画像・全フレームをサーバーへ送りません。WebSocketとSupabaseに保存・配信するのは、状態、ジェスチャー、表情の要約、信頼度、時刻などのメタデータです。

Supabaseのcurrent_statusesには現在状態だけを保存します。status_eventsテーブルは履歴用のスキーマとして存在しますが、現行APIは現在状態の更新を主に行います。

Py-Feat v2を明示的に使う場合だけ、縮小JPEGがPython APIへ送信されます。Go APIとSupabaseにはその画像を保存しません。

本番では次の秘密情報をフロントエンドへ含めないでください。

- SupabaseのSecret key
- Supabaseのservice role key
- バックエンド用の管理トークン

フロントエンドが使うのはSupabaseの公開キーと、ログイン後に取得したユーザーのアクセストークンです。

## ディレクトリ

~~~
frontend/       React + TypeScriptの画面、認識ライブラリ、PWA
backend/        Go HTTP / WebSocket API
emotion-api/    Python + Py-Feat v2の任意の表情API
supabase/       PostgresマイグレーションとRLS
hardware/       M5StickC Plus2の表示クライアント試作
render.yaml     Render Blueprint
~~~

## 必要な環境

- Node.js（frontendのpackage.jsonに定義したVite・TypeScript・Vitestを実行できるバージョン）
- Go 1.24以上
- Python 3.11（Py-Feat v2 APIを使う場合のみ）
- カメラとWebSocketを使えるブラウザ
- 本番のGoogleログインにはSupabase Authの設定

## セットアップ

リポジトリ直下で依存関係をインストールします。

~~~
npm ci
~~~

フロントエンドのテストとビルドは次で実行できます。

~~~
npm test
npm run build
~~~

### フロントエンドのローカル起動

~~~
npm run dev
~~~

通常は http://localhost:5173/ で開きます。Viteの開発プロキシが /api とWebSocketをlocalhost:8080へ転送します。

ローカル匿名モードで試す場合は、frontend/.env.localに次を設定します。

~~~dotenv
VITE_ANONYMOUS_MODE=true
VITE_ROOM_ID=local-room
VITE_USER_ID=local-user
~~~

匿名モードはローカル検証用です。本番ではSupabase Authとルーム権限を使用してください。

### Go APIのローカル起動

別ターミナルで次を実行します。

~~~powershell
cd backend
$env:ALLOW_ANONYMOUS="true"
go run ./cmd/server
~~~

匿名モードでは、フロントエンドから送られたuserIdをそのまま検証に使います。本番相当の認証を試す場合はALLOW_ANONYMOUSをfalseにし、SUPABASE_URLとSUPABASE_SECRET_KEYを設定してください。

### 任意のPython表情API

Py-Feat v2 APIを使うときだけ、Python環境を用意します。

~~~powershell
py -3.11 -m venv .venv
.\.venv\Scripts\Activate.ps1
pip install -r emotion-api\requirements.txt
$env:ALLOWED_ORIGINS="http://localhost:5173,http://127.0.0.1:5173"
python -m uvicorn app:app --app-dir emotion-api --host 127.0.0.1 --port 8000
~~~

ヘルスチェック:

~~~
GET http://127.0.0.1:8000/healthz
~~~

このAPIをフロントエンドから利用する場合は、useStateRecognitionにemotion.urlを明示的に渡します。通常のApp画面はこのオプションを渡していません。

## フロントエンドのライブラリ利用

画面を持たない認識・同期ライブラリとして利用する場合は、video要素を渡します。

~~~tsx
import { useRef } from "react";
import { useStateRecognition } from "@okimochi/recognition-client";

export function CameraRuntime() {
  const videoRef = useRef<HTMLVideoElement>(null);

  useStateRecognition(videoRef, {
    face: { enabled: true },
    socket: {
      url: "ws://localhost:8080/api/v1/ws",
      token: "SUPABASE_ACCESS_TOKEN",
      roomId: "ROOM_UUID",
      clientId: crypto.randomUUID(),
      userId: "USER_UUID"
    }
  });

  return <video ref={videoRef} hidden />;
}
~~~

本番ではWASMとMediaPipeモデルを同一オリジンに配置する構成も選べます。現行のデフォルトURLはCDNです。

## 認証・同期設定

### フロントエンド環境変数

frontend/.env.localまたはデプロイ環境に設定します。

| 変数 | 用途 |
|---|---|
| VITE_SUPABASE_URL | Supabase Project URL |
| VITE_SUPABASE_ANON_KEY | Supabaseの公開キー |
| VITE_WS_URL | WebSocket API URL。開発時は省略可能、本番はwss://を指定 |
| VITE_PUBLIC_APP_URL | QR／ペアリングURLを作るときの公開アプリURL |
| VITE_ROOM_ID | 未ログインのローカル検証や固定ルーム用。ログイン時は省略可能 |
| VITE_USER_ID | 未ログインのローカル検証用ユーザーID |
| VITE_ANONYMOUS_MODE | trueで匿名モードを有効化。ローカル・デモ用 |

ログイン済みでVITE_ROOM_IDを省略した場合、現在はroom_membersから取得した最初のルームを使います。複数ルームを選択するUIはありません。

### バックエンド環境変数

| 変数 | 用途 |
|---|---|
| PORT | HTTPポート。既定値8080 |
| ALLOWED_ORIGINS | 許可するOriginのカンマ区切り |
| ALLOW_ANONYMOUS | trueはローカル検証用。本番はfalse |
| SUPABASE_URL | Supabase Project URL |
| SUPABASE_SECRET_KEY | バックエンド専用のSecret key |
| STATUS_TTL | 自動認識状態の有効期間。0は無期限、最大24時間。手動状態は常に無期限 |

STATUS_TTLの既定値は15mです。自動認識状態が期限切れになると現在状態を返さず、画面側は初期状態（反応可能）に戻ります。手動ボタンで設定した状態は期限切れになりません。render.yamlは15mを設定しています。

## Go API

### HTTPエンドポイント

~~~
GET  /healthz
GET  /readyz
GET  /api/v1/rooms/{roomId}/status/{userId}
POST /api/v1/pairing
POST /api/v1/pairing/claim
GET  /api/v1/ws
~~~

### WebSocketの流れ

1. クライアントが8秒以内にclient.helloを送信
2. バックエンドがSupabaseアクセストークン、ルーム所属、または匿名設定を検証
3. 認証成功後にserver.readyを返す
4. PC側がrecognition.updateを送信
5. バックエンドが現在状態を保存し、同じルームへstatus.changedを配信
6. スマホや別クライアントが表示を更新

~~~
{
  "type": "recognition.update",
  "sequence": 1,
  "capturedAt": "2026-08-25T10:30:15.420Z",
  "status": "available",
  "source": "hand",
  "hand": {
    "gesture": "thumb_up",
    "confidence": 0.91,
    "handedness": "Right"
  },
  "face": null
}
~~~

sourceはhand、face、manual、noneを使います。手ジェスチャーと表情は状態との整合性をバックエンドでも検証します。

サーバーは次を検証します。

- メッセージサイズ16KiB以下
- sequenceが単調増加
- capturedAtが現在時刻から大きく外れていない
- 信頼度が0から1の範囲
- sourceとstatus・hand・faceの組み合わせ
- Supabase利用時のアクセストークンとroom_members
- ペアリングクライアントの読み取り専用制約

WebSocketは25秒ごとにpingを送り、フロントエンドは切断時に指数バックオフで再接続します。

## Supabase

マイグレーションは次の順に適用します。

1. supabase/migrations/001_initial.sql
2. supabase/migrations/002_manual_recognition_source.sql
3. supabase/migrations/003_pairing_grants.sql
4. supabase/migrations/004_manual_priority_and_expiry.sql

主なテーブル:

- rooms: ルーム
- room_members: ユーザーとルームの所属
- current_statuses: ユーザーごとの現在状態
- status_events: 履歴保存用のスキーマ
- pairing_grants: QR／合言葉ペアリング用の一時認証情報

RLSを有効にし、通常ユーザーは所属ルームを読み取れます。現在状態の書き込みとペアリング情報の操作は、バックエンドがservice roleで行います。

Googleログインを使う場合は、Supabase AuthでGoogle Providerを有効化し、公開アプリのURLをSite URLとRedirect URLに設定してください。Secret keyはブラウザへ公開しないでください。

## デプロイ

### フロントエンド

Vercelなどの静的ホスティングへ、次の設定でデプロイできます。

- Build command: npm ci && npm run build
- Output directory: frontend/dist
- VITE_SUPABASE_URL
- VITE_SUPABASE_ANON_KEY
- VITE_WS_URL
- VITE_PUBLIC_APP_URL

本番のVITE_WS_URLはTLS付きのwss:// URLを指定します。

### Go APIとPy-Feat API

render.yamlには次の3サービスを定義しています。

- okimochi-api: Go WebSocket／HTTP API
- okimochi-emotion-api: 任意のPy-Feat v2 API
- okimochi-frontend: 静的フロントエンド

Go APIをCloud Runへ配置することもできます。WebSocketの長時間接続を使うため、タイムアウト、Origin、SupabaseのSecret key、ヘルスチェックを環境に合わせて設定してください。

本番のバックエンドはALLOW_ANONYMOUS=falseにし、ALLOWED_ORIGINSに実際のフロントエンドOriginだけを指定してください。

## M5StickC Plus2試作

hardware/m5stick/main.inoに、M5StickC Plus2の画面へWebSocketで状態を表示するクライアント試作があります。

~~~
カメラ／手動操作
  → Go API
  → WebSocket
  → M5StickC Plus2の画面
~~~

現在のスケッチはローカル検証向けです。Wi-Fi、ホスト、ポート、トークンがソース内のプレースホルダーになっており、TLS付きWebSocketや本番向けの認証設定は別途必要です。M5Stickはアプリの必須構成ではなく、物理表示へ展開するための試作です。

## テスト

### 自動テスト

~~~
npm test
npm run build

cd backend
go test ./...

cd ../emotion-api
python -m pytest
~~~

Py-Featの依存関係を入れていない環境では、emotion-apiのテストは先にPython依存関係をインストールしてください。

### 手動確認

- 左手・右手、鏡像表示
- 親指を上・横・下、shaka、握りこぶし
- カメラの向きや手首の角度
- 顔だけ、手だけ、両方未検出
- 手の認識後に顔がすぐ上書きしないこと
- PCの手動操作がスマホへ反映されること
- 同一アカウントでの再ログインと状態復元
- WebSocket切断、再接続、画面復帰
- 異なるルームへ状態が漏れないこと
- ペアリングコードの10分期限、1回限り、読み取り専用

## 現在の制約

- 手の認識はカメラの向きや極端な遮蔽に影響される
- 親指の上下は画像のY方向を基準にしているため、手首を大きく回転させると不安定になる
- 顔認識は表情の補助信号であり、本人の感情を断定しない
- MediaPipeのモデルとWASMはデフォルトでCDNから取得する
- Py-Feat v2は任意経路で、通常のApp画面では使わない
- PWAとDocument Picture-in-Pictureの対応状況はブラウザに依存する
- M5Stickクライアントはローカル試作で、本番のTLS・認証・OTA更新までは整備していない
~~

