# Backend

Go標準ライブラリだけで動作するfliqrssのREST APIである.
RSS 2.0とAtom 1.0のフィード登録, 形式の自動判定, 記事取込に対応する.
現在はフロントエンド接続前のため, データをインメモリへ保存する.
サーバーを再起動すると記事状態, 追加したソース, タグは初期状態へ戻る.

## 起動

```bash
go run ./cmd/server
```

標準では`http://localhost:8080`で待ち受ける.
Viteからのアクセスを許可するオリジンは`http://localhost:5173`である.
設定を変更する場合は環境変数を指定する.

```bash
ADDR=:8081 CORS_ORIGIN=http://localhost:5174 go run ./cmd/server
```

## テスト

```bash
go test ./...
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

取得処理には次の制限を適用する.

- HTTPとHTTPSだけを許可する
- 接続と取得を含めて12秒でタイムアウトする
- 展開後のレスポンスを5 MiBまでに制限する
- リダイレクトを5回までに制限する
- ループバック, プライベートIP, リンクローカルなどへの接続を拒否する
- URLにユーザー情報を含めることを禁止する

## 未実装

- RSS 1.0の解析
- バックグラウンドでの定期収集
- PostgreSQLへの保存
- 認証とユーザーごとの状態管理
- React静的ファイルの配信
- Docker Composeと本番用Dockerfile
