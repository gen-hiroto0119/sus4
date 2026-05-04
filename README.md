<div align="center">

# tetra

**A silent code viewer for the AI coding era.**

</div>

ターミナルで動く、書かない人のためのコード閲覧 TUI。AI コーディング時代の
「コードを観察するだけ」のニーズに振り切った viewer。

設計思想は [Concept.md](Concept.md)、内部設計は [Design.md](Design.md) を参照。

---

## Why

VSCode やその他のエディタは「書く人」のために作られている。書かない時間に
ノイズ (LSP / 診断 / サジェスト / 通知) を浴び続けるのは、AI に委ねる時代では
余計だ。`tetra` は **画面に出すのはコードと diff だけ** に絞った静かな TUI。

## Features (v0.1)

- 2 ペインレイアウト (左: ファイルツリー / 変更一覧、右: コード / diff)
- Chroma によるシンタックスハイライト + 行番号 gutter
- ファイル閲覧中の変更マーカー (HEAD との diff を行頭の細バーで可視化: 追加 / 変更 / 削除)
- 自動追従: `fsnotify` でファイル / `.git/HEAD` / `.git/index` を監視し、
  Claude Code 等の書き換えを 1 秒以内に反映
- 長行の折り返し表示 (横スクロールなし)
- Material Design Nerd Font アイコン (フォント無しなら off に切替可)
- TOML 設定ファイル (`$XDG_CONFIG_HOME/tetra/config.toml`)
- Narrow terminal friendly: 60 桁未満ではメインビューのみのレイアウト降格

書かない原則は徹底。詳細な Non-Goals は [Concept.md](Concept.md#non-goals)。

## Install

```bash
go install github.com/gen-hiroto0119/tetra/cmd/tetra@latest
```

これでバイナリは `~/go/bin/tetra` に入る。`~/go/bin` が `$PATH` に
通っていない場合は `tetra` コマンドが見つからない — 以下を実行:

```bash
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc   # bash の人は ~/.bashrc
source ~/.zshrc                                       # 現セッションへ即反映
tetra --version                                       # 動作確認
```

`~/go/bin` がすでに通っている場合は `go install` だけで `tetra`
コマンドが使えるようになる。

バイナリだけ欲しい場合は
[Releases](https://github.com/gen-hiroto0119/tetra/releases)
ページから各 OS / arch のアーカイブを直接ダウンロードして任意の
PATH ディレクトリに置く (`/usr/local/bin/tetra` など)。

Homebrew は提供していません — homebrew-core の `tetra` (Cilium の
Tetragon CLI) と名前が衝突するため。

### Update

```bash
tetra update     # = go install github.com/gen-hiroto0119/tetra/cmd/tetra@latest
```

`go` が PATH に無い場合は失敗するので、その時はバイナリを
[Releases](https://github.com/gen-hiroto0119/tetra/releases) から
落として置き換えるか、Go をインストールしてから再実行。
特定 version に固定したいときは `go install ...@v0.1.7` のように
直接実行してください。

Nerd Font v3+ をターミナルで使っているとアイコンが正しく表示される。
おすすめは [Moralerspace HW NF](https://github.com/yuru7/moralerspace) や
JetBrainsMono Nerd Font。Nerd Font が無い環境では config で `icons = false` を
推奨。

## Usage

```bash
tetra              # カレントディレクトリで起動
tetra <file>       # 直開き (v0.2 予定、v0.1 ではフォールバック)
tetra <commit>     # コミット詳細 (v0.2 予定)
```

### Keymap

```
Tab        フォーカス切替 (sidebar ⇄ main)
←/→        sidebar モード切替 (files ⇄ changes)
↑/↓        項目移動 / 縦スクロール
Enter      選択項目を開く / ディレクトリ展開
Shift+?    ヘルプ
q          終了
```

## Config

`$XDG_CONFIG_HOME/tetra/config.toml` (デフォルトは `~/.config/tetra/config.toml`)。
ファイルが無くても起動する — その場合は default が当たる。

```toml
# 配色テーマ。v0.1 は "default" のみ。
theme = "default"

# 24-bit color の override。省略で COLORTERM 自動検出。
# true_color = false

# Nerd Font アイコン。Nerd Font v3+ 必須。
icons = true
```

`--config <path>` で別 path を指定可。

## Development

```bash
go build ./...
go test ./...
go vet ./...
```

主要依存:

- Go 1.26+
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) / [Lipgloss](https://github.com/charmbracelet/lipgloss)
- [charmbracelet/x/ansi](https://github.com/charmbracelet/x)
- [Chroma](https://github.com/alecthomas/chroma)
- [fsnotify](https://github.com/fsnotify/fsnotify)
- [BurntSushi/toml](https://github.com/BurntSushi/toml)

## License

[MIT](LICENSE)
