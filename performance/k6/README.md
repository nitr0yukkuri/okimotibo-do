# k6 WebSocket負荷試験

`status-sync.js` は、同期バックエンドをカメラなしの仮想クライアントで検証するためのスモーク負荷試験です。

- publisher 1台が `recognition.update` を定期送信
- viewer 複数台が同じルーム・ユーザーの `status.changed` を受信
- `server.ready` の接続成功率を測定
- 状態配信の遅延、状態シーケンスの逆戻り、サーバーエラーを測定
- publisher は手動状態のLeaseをHeartbeatで延長

カメラ画像やMediaPipeの認識精度は測りません。WebSocket、状態保存、Pub/Sub、複数クライアントへの配信経路を測ります。

## ローカルで実行

先にバックエンドを匿名モードで起動します。

```powershell
cd backend
$env:ALLOW_ANONYMOUS="true"
go run ./cmd/server
```

別のターミナルでk6を実行します。

```powershell
k6 run performance/k6/status-sync.js
```

既定値は次のとおりです。

- WebSocket: `ws://127.0.0.1:8080/api/v1/ws`
- viewer: 10台
- 試験時間: 30秒
- publisher: 3秒後に開始
- 状態更新: 2秒ごと
- 手動Lease: 30秒、Heartbeat: 10秒ごと
- ユーザーID: 実行ごとに生成する専用ID

## 人数や時間を変える

```powershell
$env:VIEWERS="50"
$env:DURATION="2m"
$env:SESSION_MS="110000"
$env:PUBLISH_INTERVAL_MS="1000"
k6 run performance/k6/status-sync.js
```

`SESSION_MS` は `DURATION` より短くしてください。1回の接続を閉じた後に、各viewerが受信できたかを集計します。

## stagingで実行

リモートURLは誤実行防止のため、明示的に許可した場合だけ実行できます。

```powershell
$env:WS_URL="wss://staging.example.com/api/v1/ws"
$env:ALLOW_REMOTE="true"
$env:PUBLISH_TOKEN="<専用テストユーザーのアクセストークン>"
$env:VIEWER_TOKEN="<同じユーザーのアクセストークン、または専用ペアリングtoken>"
$env:ROOM_ID="k6-staging-room"
$env:USER_ID="k6-staging-user"
k6 run performance/k6/status-sync.js
```

本番URL、実ユーザーのroom、実利用中のアカウントでは実行しないでください。publisherが状態を更新し、状態配信の負荷を発生させます。Supabaseを使う環境では、専用テストユーザーと専用roomを用意してください。アクセストークンはシェル履歴やログへ出さないでください。

`VIEWER_TOKEN` にペアリングtokenを指定するとviewerは読み取り専用になります。匿名モードではviewerも認証上は書き込み可能ですが、スクリプトから更新は送信しません。

## 合格条件

既定のthresholdは次のとおりです。

- 接続して `server.ready` を受け取る率: 99%以上
- 全viewerが少なくとも1件の `status.changed` を受け取る率: 99%以上
- `status.changed` の配信遅延p95: 1秒未満
- サーバーerrorメッセージ率: 1%未満
- viewerで状態sequenceの逆戻り: 0件

遅延はpublisherが送信した`capturedAt`からviewerが受信した時刻までで計算します。テスト実行端末とサーバーの時計がずれている環境では正確な遅延にならないため、NTP同期済みの専用環境で測定してください。

負荷を上げる前に、まず10 viewer・30秒のローカル試験で接続、受信、thresholdが通ることを確認します。Redisや複数インスタンスの検証では、同じ試験をstagingで実行し、Redis停止などの障害注入は別の明示的な試験として行います。

参考: [Grafana k6 WebSockets](https://grafana.com/docs/k6/latest/javascript-api/k6-websockets/) / [Thresholds](https://grafana.com/docs/k6/latest/using-k6/thresholds/)
