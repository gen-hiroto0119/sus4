# Concept

> A silent code viewer for the AI coding era.

## What

ターミナルで動く、コード閲覧専用の TUI。**画面に出すのはコードと diff だけ**。
編集機能・LSP・補完・通知はそもそも持たない。

## Why

AI コーディングが普及して「自分では書かないが読む必要がある人」が増えた。
既存エディタ (VSCode 等) は「書く人」のために作られていて、書かない時間にも
LSP の波線・補完・通知が主張してくる。観察に集中したいとき、それはノイズ。

`tetra` は AI が書いている横で、人間は静かに観察する、という新しい開発スタイルに
振り切った道具。

## Naming

ギリシア語の τέτρα (= 4) から:

- **tetrachord** (4 音音階) — 解決しない、保留された連なり
- **tetrahedron** (四面体) — 最少の頂点で成立する安定形 (read-only の意匠)
- **第 4 の dev surface** — Editor / Terminal / Browser に並ぶ Viewer

何も提案せず、何も書き換えず、ただ静かに開いている。

## Core Features (v0.1)

- 2 ペインレイアウト (左: ファイルツリー / 変更一覧、右: コード / diff)
- シンタックスハイライト (chroma) と Markdown プレビュー (glamour)
- 変更マーカー: HEAD との diff を行頭の細バーで可視化 (add / mod / del)
- 自動追従: fsnotify でファイル / `.git/HEAD` / `.git/index` を監視
- ダーク / ライト / 自動 (`theme = "auto"` で OSC 11 検出)
- TOML 設定ファイル
- Material Design Nerd Font アイコン
- `tetra update` で `go install ...@latest` 実行

## Non-Goals

これらは **やらない**。制約ではなく設計上の選択:

- ファイル編集
- LSP / 診断 / 補完 / AI サジェスト
- Git 操作 (commit / push / branch)
- side-by-side diff (narrow terminal friendly のため unified のみ)
- 横スクロール (折り返しのみ)
- 通知 / ポップアップ / モーダル
- プラグインシステム
- マウス操作

## Architecture (要点)

- Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Elm Architecture)
- Watcher / View / Cmd の境界をはっきり分けて、global mutable state を持たない
- すべての I/O は `tea.Cmd` 経由 (goroutine) → 結果は `tea.Msg` で戻す
- 詳しい設計判断は git の commit message を参照 (永続化されてる)

### Performance ガード

「画面に出るのはコードと diff だけ、バックグラウンドで何も走らせない」を CPU レベルで満たすため:

- fs-event throttle (status 200ms / file reload 500ms)
- View() の revision-cache (毎フレーム再計算しない)
- git の `--no-optional-locks` (自分の `.git/index` 更新による自己フィードバック切り)
- CI bench gate (`internal/app/cpu_probe_test.go` が 5,000 ns/op 上限で監視)

## Distribution

```bash
go install github.com/gen-hiroto0119/tetra/cmd/tetra@latest
```

GitHub Releases から OS 別バイナリも配布。Homebrew は提供しない (homebrew-core の `tetra` (Cilium Tetragon) と名前衝突するため)。

## Roadmap

### v0.1 ✅ shipped
すべての Core Features 上記。

### v0.2
- `tetra <commit>` 起動対応 (コミット詳細表示)
- `tetra <file>` 起動対応 (ファイル直開き)
- atomic save / debounce の本格対応

### v0.3 以降
- カスタムキーバインド
- `.gitignore` フルパース
- ファイル名 fuzzy find

## Closing Thought

AI がコードを書く時代に必要なのは「書く道具の進化」ではなく「読む道具の進化」かもしれない。`tetra` はその仮説に対する一つの答え。
