# Backend architecture

## 結論

このBackendは、状態を中心にした小さなモジュールとして保つ。

```text
HTTP / WebSocket / IoT simulator
              |
              v
       API transport layer
              |
              v
       Application use cases
              |
              v
       Domain state machine
              |
              v
  repository / conditional-write ports
              |
              v
      Memory or Supabase store

Redis Pub/Sub は Pod 間への通知だけを担当する。
Kubernetes はプロセスの起動・接続先・監視・再起動を担当する。
```

これは「レイヤードを土台にしたOnion/Clean寄りの構成」で、状態変更の通知部分だけイベント駆動にしている。状態を全部イベントソーシングする構成や、状態ごとのマイクロサービスにはしていない。

## 状態の単位

1つの状態集約は `(roomId, userId)` である。同じユーザーの現在状態を1つだけ持ち、`clientId` はその状態を書いた端末を示す。

- `sequence`: 端末ごとの入力順。Heartbeatでは増やさない。
- `capturedAt`: 認識データを取得した時刻。
- `receivedAt`: Backendが受信した時刻。
- `expiresAt`: 状態を有効とみなす期限。
- `source`: `hand`, `face`, `manual`, `none`。

`sequence` と時刻は別の概念である。端末の再接続や複数端末を考えると、将来 `revision`（Backend側の集約版数）を追加する余地がある。

## 各層の責務

### `internal/domain`

プロダクトのルールだけを持つ。通信、JSON、Supabase、Redis、Kubernetesは知らない。

- 状態の形と入力値の検証
- 古いsequenceの拒否
- 古い時刻の状態の拒否
- 有効な手動状態が自動認識に上書きされることの防止
- Heartbeatによる手動Leaseの更新
- Lease期限切れ・切断時のクリアという遷移

ルールを変えるときは、まずここを変更し、`transition_test.go` に状態遷移のテストを追加する。

### `internal/application`

「何をするか」を調整する層である。現在状態を読み、Domainに遷移を依頼し、保存結果を確認する。

- `StatusService`: 認識更新、Heartbeat、現在状態の取得、期限切れ・切断の条件付きクリア
- `ExpiryScheduler`: `(roomId,userId)` ごとに期限タイマーを1本だけ管理
- 保存先は `StateRepository` ポート経由で利用する

ここはWebSocketのエラーコードやJSON形式を決めない。後からHTTP、WebSocket、IoTの別アダプターを追加しても、同じユースケースを使えるようにする。

### `internal/api`

外部との境界である。

- HTTP/WebSocketの受付
- 認証、Room権限、Pairingの確認
- JSONの厳格なデコード
- APIエラーコードへの変換
- 接続Hubへの配信
- Redis通知の送受信

APIは状態遷移のルールを直接決めない。APIで判断してよいのは「この接続が発行者か」「このメッセージ形式が正しいか」といった通信上の事項だけである。

### `internal/store`

保存の実装である。MemoryとSupabaseの違いを吸収する。

保存時にも条件付き更新・条件付き削除を必須にしている。これはDomainのルールの重複ではなく、複数Podや遅延したタイマーとの競合に対する最後の原子性ガードである。条件付き削除を持たない保存先へ無条件フォールバックはしない。

### Kubernetes

Kubernetesは業務ロジックを実装しない。

- `Deployment`: Backend/Redis/Emotion API Podの望ましい数と更新方法を宣言
- `Service`: PodのIPが変わっても名前で接続できるようにする
- `readinessProbe`: 通信を受けてよいかを確認
- `livenessProbe`: プロセスが生きているかを確認
- self-healing: Podが落ちたらDeploymentが作り直す

マニフェストは `k8s/base/` を共通部品にし、リポジトリ直下の `k8s/` はkind向けの匿名・単一Pod構成として使う。本番相当は `k8s/overlays/production/` で差分を重ねる。Production overlayでは、SupabaseとマネージドRedisを共有の正本・イベント基盤として用意したうえで、複数Pod、HPA、PDB、NetworkPolicy、TLS付きIngressを有効にする。ローカルRedisは永続化しないため、本番overlayには含めない。

状態遷移、ログアウト時のクリア、Heartbeat、例外処理はBackendの責務である。

## 代表的なフロー

### 認識状態の更新

```text
recognition.update
  -> APIがJSON/接続権限を確認
  -> Applicationが現在状態を取得
  -> Domain.ApplyRecognitionで遷移を判定
  -> Repositoryが保存（競合時は条件付きで拒否）
  -> 期限タイマーを同じユーザーの1本に差し替え
  -> Hubへ配信
  -> Redisへ通知（設定されている場合）
```

### Heartbeat

```text
status.heartbeat
  -> APIがLease秒数を検証
  -> Applicationが現在のmanual状態と所有clientを確認
  -> DomainがLease更新を生成
  -> Repositoryが読んだスナップショットと一致する場合だけ更新
```

Heartbeatは新しい認識結果ではないため、認識時の `capturedAt` は変えない。生存確認と認識データを混ぜないためである。

### 再接続

```text
接続をHubへ仮登録
  -> 現在状態を取得
  -> server.readyを先に送る
  -> 取得中に発生したイベントをその後に送る
```

これにより、初期スナップショットが新しいイベントを後から上書きする競合を防ぐ。保留キューが溢れた接続は、正しい順序を保証できないため切断する。

## 現在のイベント設計と今後

現在の正（source of truth）は `current_statuses` のスナップショットであり、Redis Pub/Subは再送保証のない通知である。Redisのイベントを状態の正として扱わない。

Supabaseの `status_events` は、受理された `current_statuses` の意味のある変更をDBトリガーで記録する。Heartbeatは認識状態を変えないため履歴イベントにしない。複数Podで確実な再送・監査・履歴表示が必要になった段階では、次の順番でOutboxを追加する。

1. 状態更新と履歴イベントを同じDBトランザクションで保存する。
2. 同じトランザクションでOutboxを作る。
3. Outbox workerがRedisや将来の外部通知へ再送する。
4. 受信側はevent idを使って重複を無害化する。

今の規模でいきなりイベントソーシングやメッセージブローカーを必須にしないのは、現在表示を速く取得でき、学習用のKubernetes構成も複雑にしすぎないためである。

## プロダクトとの適合

このプロダクトの価値は「AIが感情を当てること」ではなく、「今話しかけてよいかを周囲に伝えること」である。そのため優先すべき安定性は次の通り。

- 古い状態が残り続けない
- 端末切断時に新しい端末の状態を消さない
- 手動操作を自動認識が勝手に上書きしない
- 再接続後に表示が巻き戻らない
- 一時的なRedisやPodの問題でもDBの現在状態を基準に復元できる

今回のDomain/Application分離、条件付き保存、期限タイマー集約、再接続ゲートは、この優先順位に直接対応している。

## 本番化前の残課題

- `status_events` とOutboxのトランザクション保存
- Backend側 `revision` またはevent idの導入
- Redis停止時の監視・再接続・通知遅延の可視化
- Supabaseを使う複数Podでの統合テスト
- K8s本番Overlay（TLS入口、NetworkPolicy、PDB、HPA、Secret運用）
- Emotion APIをブラウザ直呼び出しからBackend経由にするかの方針決定

これらは今回の状態機械リファクタとは別の運用・配信拡張であり、まず現在の状態遷移を壊さないことを優先する。
