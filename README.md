<div align="center">

# sus4

**A silent code viewer for the AI coding era.**

</div>

ターミナルで動く、書かない人のためのコード閲覧 TUI。AI コーディング時代の
「コードを観察するだけ」のニーズに振り切った viewer。

設計思想は [Concept.md](Concept.md)、内部設計は [Design.md](Design.md) を参照。

---

## Why

VSCode やその他のエディタは「書く人」のために作られている。書かない時間に
ノイズ (LSP / 診断 / サジェスト / 通知) を浴び続けるのは、AI に委ねる時代では
余計だ。`sus4` は **画面に出すのはコードと diff だけ** に絞った静かな TUI。

## Features (v0.1)

- 2 ペインレイアウト (左: ファイルツリー / 変更一覧、右: コード / diff)
- Chroma によるシンタックスハイライト + 行番号 gutter
- 自動追従: `fsnotify` でファイル / `.git/HEAD` / `.git/index` を監視し、
  Claude Code 等の書き換えを 1 秒以内に反映
- 長行の折り返し表示 (横スクロールなし)
- Material Design Nerd Font アイコン (フォント無しなら off に切替可)
- TOML 設定ファイル (`$XDG_CONFIG_HOME/sus4/config.toml`)
- Narrow terminal friendly: 60 桁未満ではメインビューのみのレイアウト降格

書かない原則は徹底。詳細な Non-Goals は [Concept.md](Concept.md#non-goals)。

## Install

```bash
go install github.com/gen-hiroto0119/sus4/cmd/sus4@latest
```

Nerd Font v3+ をターミナルで使っているとアイコンが正しく表示される。
おすすめは [Moralerspace HW NF](https://github.com/yuru7/moralerspace) や
JetBrainsMono Nerd Font。Nerd Font が無い環境では config で `icons = false` を
推奨。

## Usage

```bash
sus4              # カレントディレクトリで起動
sus4 <file>       # 直開き (v0.2 予定、v0.1 ではフォールバック)
sus4 <commit>     # コミット詳細 (v0.2 予定)
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

`$XDG_CONFIG_HOME/sus4/config.toml` (デフォルトは `~/.config/sus4/config.toml`)。
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
