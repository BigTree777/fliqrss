# Backend

Go標準ライブラリだけで動作するfliqrssのREST APIである.
RSS 2.0とAtom 1.0のフィード登録, 形式の自動判定, 記事取込, 定期取得に対応する.
PostgreSQLへ記事, ソース, タグ, 閲覧状態を保存する.
初期状態は空であり, ダミーの記事, ソース, タグは登録しない.

## 起動

リポジトリのルートでDocker Composeを起動する.

```bash
cp .env.example .env
docker compose up --build
```

PostgreSQLのスキーマはバックエンド起動時に自動作成する.
データは`postgres-data`名前付きボリュームへ保存されるため, コンテナを再作成しても維持される.

標準では`http://localhost:8080`で待ち受ける.
Viteからのアクセスを許可するオリジンは`http://localhost:5173`である.
Goを直接起動する場合は, PostgreSQLの接続先を環境変数で指定する.

```bash
DATABASE_URL='postgres://fliqrss:fliqrss-dev@localhost:5432/fliqrss?sslmode=disable' go run ./cmd/server
```

`DATABASE_URL`を指定しない場合は, 開発とテスト用のインメモリ保存で起動する.
この場合のデータは再起動時に消える.

定期取得の間隔は`FEED_REFRESH_INTERVAL`で変更できる.
Goの時間表記を使用し, 標準値は`15m`である.

```bash
FEED_REFRESH_INTERVAL=30m go run ./cmd/server
```

## テスト

インメモリストアを使用する通常テストは, Goを直接実行できる環境で起動する.

```bash
go test ./...
```

PostgreSQLへの保存と再読込を含む全テストは, リポジトリのルートからDocker Composeで起動する.
テストごとに一時スキーマを作成するため, 開発用データには影響しない.

```bash
docker compose --profile test run --rm backend-test
```

## レスポンス形式

成功時は`data`, 失敗時は`error`を返す一般的なJSON形式を使用する.

```json
{
  "data": {}
}
```

```json
{
  "error": {
    "code": "not_found",
    "message": "requested resource was not found"
  }
}
```

## API

| メソッド | パス | 内容 |
| --- | --- | --- |
| GET | `/api/v1/health` | 動作確認 |
| GET | `/api/v1/articles` | 記事一覧 |
| GET | `/api/v1/articles/{id}` | 記事詳細 |
| PATCH | `/api/v1/articles/{id}/state` | 記事状態の変更 |
| POST | `/api/v1/articles/reset-skipped` | スキップ由来の既読を解除 |
| GET | `/api/v1/sources` | ソース一覧 |
| POST | `/api/v1/sources` | ソース追加 |
| POST | `/api/v1/sources/import-opml` | OPMLからソースを一括追加 |
| GET | `/api/v1/sources/export-opml` | ソースとタグをOPMLとして出力 |
| PATCH | `/api/v1/sources/{id}` | ソース名または取得状態の変更 |
| DELETE | `/api/v1/sources/{id}` | ソース削除 |
| POST | `/api/v1/sources/{id}/refresh` | ソースを再取得 |
| PUT | `/api/v1/sources/{id}/tags` | ソースのタグ割り当て |
| GET | `/api/v1/tags` | タグ一覧 |
| POST | `/api/v1/tags` | タグ追加 |
| PATCH | `/api/v1/tags/{id}` | タグ名変更 |
| DELETE | `/api/v1/tags/{id}` | タグ削除 |

記事一覧は`sourceId`, `tagId`, `state`で絞り込める.
`state`には`feed`, `all`, `favorite`, `saved`, `deleted`, `read`を指定できる.
省略時は`feed`となり, 未読かつ保存されておらず, ゴミ箱に入っていない記事だけを返す.

記事状態の変更では次のように`action`を指定する.

```json
{
  "action": "skip"
}
```

利用できる操作は`read`, `unread`, `skip`, `save`, `unsave`, `favorite`, `unfavorite`, `delete`, `restore`である.

## RSS・Atomの登録

ソース名とフィードURLを指定する.
ソース名を省略した場合はフィード内のタイトルを使用する.

```bash
curl -X POST \
  -H "Content-Type: application/json" \
  -d '{"name":"Example Feed","url":"https://example.com/feed.xml"}' \
  http://localhost:8080/api/v1/sources
```

登録時にフィードを取得し, RSS 2.0またはAtom 1.0を自動判定して記事を取り込む.
同じURLのソースは重複登録せず, 同じ記事を再取得した場合は既存記事を更新する.

登録済みソースを手動で再取得する.

```bash
curl -X POST http://localhost:8080/api/v1/sources/{id}/refresh
```

## 定期取得

バックエンド起動時に, 取得状態が有効なニュースソースを一度更新する.
その後は`FEED_REFRESH_INTERVAL`で指定した間隔を空けて更新する.
最大8件を並行取得し, 1件の取得失敗が他のソースやAPIサーバーを停止させることはない.
停止中のソースは定期取得の対象外であり, 取得結果とエラーはバックエンドのログへ記録する.

## OPMLの取込

OPML 1.0または2.0の`outline`要素から`xmlUrl`を読み取り, ニュースソースを一括追加する.
フォルダーとして使用されている親`outline`の`text`または`title`は, ソースのタグとして取り込む.

```bash
curl -X POST \
  -F "file=@subscriptions.opml" \
  http://localhost:8080/api/v1/sources/import-opml
```

結果には対象件数, 追加件数, 重複件数, 失敗件数, 新規タグ件数が含まれる.
同じOPML内の重複URLと登録済みURLは追加しない.
登録済みソースと複数タグはOPML 2.0の`category`属性を使って出力する.
出力したOPMLはそのまま再インポートできる.

```bash
curl -o fliqrss-subscriptions.opml \
  http://localhost:8080/api/v1/sources/export-opml
```

解析には次の制限を適用する.

- ファイルサイズは2 MiBまで
- `outline`要素は5,000個まで
- 階層の深さは12まで
- フィードは200件まで
- フィード取得は最大8件を並行処理する

取得処理には次の制限を適用する.

- HTTPとHTTPSだけを許可する
- 接続と取得を含めて12秒でタイムアウトする
- 展開後のレスポンスを5 MiBまでに制限する
- リダイレクトを5回までに制限する
- ループバック, プライベートIP, リンクローカルなどへの接続を拒否する
- URLにユーザー情報を含めることを禁止する

## 未実装

- RSS 1.0の解析
- 認証とユーザーごとの状態管理
- React静的ファイルの配信
