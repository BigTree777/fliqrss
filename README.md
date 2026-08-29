# fliqrss

RSS・Atomフィードからニュースを収集し, フリック操作で閲覧するWebアプリである.

## 構成

| 対象 | 採用技術 | 実行方法・役割 |
| --- | --- | --- |
| フロントエンド | TypeScript, React, Vite | Node.js上でViteを直接起動し, UIを配信する |
| バックエンド | Go | REST APIとRSS・Atomの定期収集を実行する |
| データベース | PostgreSQL | 記事, ソース, タグ, 閲覧状態を保存する |
| バックエンド実行環境 | Docker Compose | GoバックエンドとPostgreSQLを起動する |
| フロントエンド検証環境 | Node.js | Dockerへ含めず, VPS上でViteを直接起動する |

フロントエンドはバックエンド用Dockerイメージへ含めない.
以下はUIをスマートフォンから確認するための検証用デプロイ手順であり, 本番用の静的ファイル配信方法はUI確定後に決定する.

## 必要なソフトウェア

- Git
- Docker EngineとDocker Compose
- Node.js 24系とnpm

## VPSへの初回配置

リポジトリを`ubuntu`ユーザーで配置する.

```bash
cd /opt
sudo install -d -o ubuntu -g ubuntu /opt/fliqrss
git clone git@github.com:BigTree777/fliqrss.git /opt/fliqrss
cd /opt/fliqrss
```

### バックエンド

環境変数を作成し, `POSTGRES_PASSWORD`を十分に長い値へ変更する.
`CORS_ORIGIN`にはスマートフォンから開くURLのオリジンを指定する.

```bash
cp .env.example .env
chmod 600 .env
```

```env
POSTGRES_PASSWORD=change-this-password
BACKEND_PORT=8080
CORS_ORIGIN=https://news.example.com
FEED_REFRESH_INTERVAL=15m
```

バックエンドの公開先はVPS内の`127.0.0.1`に限定される.
フロントエンドは`BACKEND_PROXY_URL`を通してAPIへ接続する.

バックエンドとPostgreSQLを起動する.

```bash
docker compose up --build -d
docker compose ps
curl -fsS http://127.0.0.1:8080/api/v1/health
```

正常時は次のJSONを返す.

```json
{"data":{"status":"ok"}}
```

### フロントエンド

依存ライブラリを固定済みの`package-lock.json`からインストールする.

```bash
cd /opt/fliqrss/frontend
npm ci
cp .env.example .env.local
```

`.env.local`をUI検証用VPSに合わせて変更する.
`FRONTEND_ALLOWED_HOSTS`にはポート番号や`https://`を含めず, Cloudflareで使用するホスト名だけを指定する.
複数指定する場合は半角カンマで区切る.

```env
FRONTEND_HOST=0.0.0.0
FRONTEND_PORT=8443
FRONTEND_ALLOWED_HOSTS=news.example.com
BACKEND_PROXY_URL=http://127.0.0.1:8080
```

型検査後にViteを起動する.

```bash
npm run typecheck
npm run dev
```

別のSSH接続からVPS内の応答を確認する.

```bash
curl -I -H 'Host: news.example.com' http://127.0.0.1:8443/
curl -fsS -H 'Host: news.example.com' http://127.0.0.1:8443/api/v1/health
```

Cloudflareの既存設定を通して`https://news.example.com`を開き, スマートフォンで画面とAPI通信を確認する.

## フロントエンドの常駐化

SSH切断後もUI検証を継続する場合は, systemdでViteを`ubuntu`ユーザーとして起動する.
`npm`のパスは`command -v npm`で確認し, 異なる場合は`ExecStart`を変更する.

`/etc/systemd/system/fliqrss-frontend.service`を次の内容で作成する.

```ini
[Unit]
Description=fliqrss frontend for UI verification
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
User=ubuntu
Group=ubuntu
WorkingDirectory=/opt/fliqrss/frontend
ExecStart=/usr/bin/npm run dev
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

設定を反映して起動する.

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now fliqrss-frontend
sudo systemctl status fliqrss-frontend
```

ログは次のコマンドで確認する.

```bash
journalctl -u fliqrss-frontend -f
```

## 更新

ローカルでpushした変更をVPSへ反映する.

```bash
cd /opt/fliqrss
git pull --ff-only
docker compose up --build -d
cd frontend
npm ci
npm run typecheck
sudo systemctl restart fliqrss-frontend
```

起動後に状態を確認する.

```bash
cd /opt/fliqrss
docker compose ps
docker compose logs --since=5m backend
systemctl status fliqrss-frontend
```

## テスト

バックエンドとPostgreSQLの統合テストを実行する.

```bash
cd /opt/fliqrss
docker compose --profile test run --rm backend-test
```

フロントエンドの型検査と本番ビルドを確認する.

```bash
cd /opt/fliqrss/frontend
npm run typecheck
npm run build
```
