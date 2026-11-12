# dockman

`dockman` は、ターミナルから Docker リソースを操作する Go 製の TUI（Terminal User Interface）です。コンテナ、イメージ、ボリューム、ネットワーク、Compose プロジェクトを一つの画面から確認・操作できます。

> [!WARNING]
> コンテナやイメージの削除、未使用リソースのクリーンアップなど、Docker の状態を変更する操作を含みます。重要な環境では、対象を確認してから実行してください。

## 主な機能

- コンテナの一覧、検索、絞り込み、並べ替え、作成、起動、停止、再起動、削除
- コンテナの統計、ログ、対話ターミナルの表示
- Docker イメージの一覧、詳細表示、pull、削除
- ボリュームとネットワークの一覧、作成、詳細表示、削除
- コンテナのネットワーク接続・切断
- Compose プロジェクトの登録と主要な `docker compose` 操作
- Docker システム情報と未使用リソースのクリーンアップ
- Docker context の切り替え

## 必要環境

- Go 1.26 以上
- Docker Engine または Docker Desktop
- `docker` CLI（PATH から実行できること）
- Compose 機能を利用する場合は Docker Compose v2（`docker compose`）
- ANSI カラーと対話入力に対応したターミナル

起動前に Docker デーモンが動作していることを確認してください。

## インストールと実行

リポジトリを取得して直接実行する場合:

```sh
git clone <repository-url>
cd dockman
go run ./cmd/dockman
```

ローカルへビルドする場合:

```sh
go build -o dockman ./cmd/dockman
./dockman
```

Windows PowerShell では、ビルド後に次のように実行できます。

```powershell
go build -o dockman.exe ./cmd/dockman
.\dockman.exe
```

## 基本操作

- `↑` / `↓`: 項目やボタンを移動
- `Enter`: 選択・決定
- `Esc` / `Backspace`: 前の画面へ戻る
- `r`: 一覧を更新
- `q`: 対応する一覧画面から戻る、またはメインメニューで終了
- `Ctrl+C`: いつでも終了

画面固有の操作は、各画面の下部に表示されます。

### コンテナ操作

コンテナ一覧と詳細画面のどちらでも、選択中のコンテナに 1 キーで操作できます。

- `S`: 起動 / 停止の切り替え
- `R`: 再起動
- `l`: ログ表示
- `e`: シェル（`docker exec -it` で画面全体を渡すため、`vim` や `top` も動作します）
- `t`: dockman 内蔵ターミナル
- `D`: 削除（確認画面が表示されます）

一覧では `/` 検索、`f` 絞り込み、`s` 並べ替え、`d` 統計、`c` 作成、`x` Docker context 切り替えも使えます。ログ・ターミナル・削除画面を `Esc` で閉じると、開いた元の画面（一覧または詳細）に戻ります。

内蔵ターミナルでは `Esc` はコンテナ側に送られます。dockman に戻るには `Ctrl+]` を使用します。

### Compose 操作

Compose プロジェクトの詳細画面では、各操作に割り当てたキーで直接実行できます（`←→` と `Enter` でも選択できます）。

| キー | 操作 | キー | 操作 |
| --- | --- | --- | --- |
| `u` | Up | `U` | Up + Build |
| `d` | Down（確認あり） | `D` | Down + Orphans 削除（確認あり） |
| `s` | Stop | `S` | Start |
| `p` | Pull | `P` | Pause |
| `R` | Restart | `b` | Build |
| `l` | Logs | `c` | Containers |

Down 系の操作は `y` を押したときだけ実行され、それ以外のキーで取り消されます。

## 開発

依存関係を取得し、テストと静的解析を実行します。

```sh
go mod download
go test ./...
go vet ./...
```

ソースコードの構成:

```text
cmd/dockman/       エントリーポイント
internal/docker/   Docker Engine / CLI との通信
internal/model/    ドメインモデル
internal/service/  操作と検証のロジック
internal/ui/       Bubble Tea ベースの TUI
```

## データ保存

登録した Compose プロジェクトは、OS のユーザー設定ディレクトリ配下にある `dockman/compose-projects.json` に保存されます。認証情報や Docker の接続情報をこのリポジトリ内へ保存する必要はありません。

## ライセンス

Copyright 2026 peco2282

このプロジェクトは [Apache License 2.0](LICENSE) のもとで公開されています。
