# tetra

ターミナルで動く、コード閲覧専用の TUI。AI コーディング時代の「書かない人」のために、画面に出すのはコードと diff だけに絞った viewer。

詳細は [Concept.md](Concept.md)。

## Install

```bash
go install github.com/gen-hiroto0119/tetra/cmd/tetra@latest
```

`~/go/bin` が PATH に無い場合:

```bash
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

バイナリだけ欲しいなら [Releases](https://github.com/gen-hiroto0119/tetra/releases) からダウンロード。

## Usage

```bash
cd <project>
tetra
```

### Keymap

| Key | Action |
|---|---|
| `↑` / `↓` (`k` / `j`) | 移動 / スクロール |
| `←` / `→` (`h` / `l`) | サイドバーモード切替 (files ⇄ changes) |
| `PgUp` / `Ctrl-B` | 1 ページ上 |
| `PgDn` / `Ctrl-F` / `Space` | 1 ページ下 |
| `g` / `Home` | 先頭 |
| `G` / `End` | 末尾 |
| `Enter` | 開く / ディレクトリ展開 |
| `Tab` | フォーカス切替 |
| `Shift+?` | ヘルプ |
| `q` / `Ctrl-C` | 終了 |

### Update

```bash
tetra update
```

## Config

`~/.config/tetra/config.toml` (任意):

```toml
theme = "auto"   # auto | dark | light
icons = true     # Nerd Font v3+ が必要
```

## License

[MIT](LICENSE)
