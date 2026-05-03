> Detailed design for sus4 — the silent code viewer.

このドキュメントは [`Concept.md`](Concept.md) の続編として、**実装上の判断**を記述する。
「何を作るか」「なぜ作るか」は Concept.md を、「どう作るか」はこのドキュメントを参照する。

## 0. Scope of this document

- 対象は v0.1（MVP）と v0.2 までの実装方針。v0.3 以降は方向性のみ言及する。
- パッケージ構成、状態モデル、メッセージフロー、各コンポーネントの責務、外部システム（ファイルシステム / Git）との結合点を扱う。
- 具体的な API シグネチャや関数名は固定しない。実装中に最適形を選べる余地を残す。

## 1. Architecture Overview

`sus4` は Bubble Tea の **Elm Architecture** に素直に乗る。グローバルな可変状態は持たず、すべての UI 変化は `Msg → Update → Model → View` の 1 サイクルで起きる。

```
            ┌──────────────────────────────────────────┐
            │                  Program                 │
            │                                          │
   ┌────────┴────────┐                ┌────────────────┴───────────────┐
   │     Sources     │   tea.Msg      │             Update             │
   │ (key, fs, git,  │ ─────────────▶ │     (root) ─┬─ sidebar         │
   │  load results)  │                │             ├─ mainview        │
   └────────┬────────┘                │             └─ git/watcher refs│
            │                         └────────────────┬───────────────┘
            │                                          │
            │                                          ▼
            │                                   ┌──────────────┐
            └──────────────  cmd.Cmd  ◀───────  │   View       │
                                                │  (Lipgloss)  │
                                                └──────────────┘
```

並行性は Bubble Tea の `tea.Cmd` に閉じる。`fsnotify` のループや Git コマンドの実行はそれぞれ goroutine を持つが、**結果は必ず `tea.Msg` として戻し、Update 経由でしか Model を触らない**。これにより排他制御は事実上不要になる。

## 2. Package Layout

```
sus4/
├── cmd/
│   └── sus4/
│       └── main.go             # CLI 引数解析 → tea.Program 起動
├── internal/
│   ├── app/                    # ルート Model / Update / View
│   ├── sidebar/                # サイドバー（ファイルツリー / 変更一覧）
│   ├── mainview/               # メインビュー（ファイル / diff / コミット）
│   ├── filetree/               # ツリー構築・ナビゲーション・除外ルール
│   ├── git/                    # status / diff / show のラッパ
│   ├── watcher/                # fsnotify ラッパとデバウンス
│   ├── highlight/              # Chroma ラッパ・キャッシュ
│   ├── diffview/               # unified diff のパースとレンダリング
│   ├── keymap/                 # キーバインド定義（v0.3 で外部化）
│   └── theme/                  # 配色 / Lipgloss スタイル
└── go.mod
```

依存方向の原則：

- `app` は `sidebar` / `mainview` / `git` / `watcher` を知ってよい。
- `sidebar` と `mainview` は互いを知らない。`app` 経由でのみ協調する。
- `filetree` / `git` / `watcher` / `highlight` / `diffview` は **UI を知らない純粋ロジック層**として保つ。
- `theme` と `keymap` は最下層で、誰からでも参照される。

## 3. State Model

ルート Model はコンポーネント Model の集約と、外部リソースへのハンドルだけを持つ。

```go
type Model struct {
    // UI
    sidebar  sidebar.Model
    main     mainview.Model
    focus    Focus              // sidebarFocus | mainFocus
    width    int
    height   int
    helpOpen bool

    // 起動時の決定事項
    rootDir   string            // 基準ディレクトリ（CWD or 引数）
    initial   StartupTarget     // dir | file | commit

    // 外部リソースのハンドル（Cmd 経由で操作）
    repo      *git.Repo         // nil ならば非 git ディレクトリ
    watch     *watcher.Handle   // fsnotify 制御用

    // エラーバナー
    err       error
}
```

サイドバー / メインビューは **自分の表示範囲だけ** に責任を持つ。例えば `sidebar.Model` はカーソル位置とスクロール、現在のモード（tree / changes）を保持するが、ファイルの中身は知らない。中身は `mainview.Model` が `app` 経由で要求して保持する。

## 4. Message Catalog

メッセージは発生源ごとにファイルを分けて定義する。`Update` の switch を肥大化させないため、**メッセージは小さく、種類は多く** が方針。

| カテゴリ | 例 | 発生源 |
|---|---|---|
| Key | `tea.KeyMsg` | Bubble Tea |
| Layout | `tea.WindowSizeMsg` | Bubble Tea |
| File load | `FileLoadedMsg{path, content, lang}` / `FileLoadFailedMsg{path, err}` | 非同期 Cmd |
| Diff load | `DiffLoadedMsg{kind, hunks}` / `DiffLoadFailedMsg{...}` | 非同期 Cmd |
| Tree | `TreeBuiltMsg{root}` / `TreeRefreshMsg{path}` | 起動時 / fs イベント |
| Watcher | `FsEventMsg{path, op}` / `WatcherErrorMsg{err}` | fsnotify goroutine |
| Git | `GitStatusMsg{entries}` / `GitHeadChangedMsg{ref}` | git ポーリング / fsnotify |
| Highlight | `HighlightedMsg{path, lines}` | Chroma 非同期実行 |
| Internal | `tickMsg`（デバウンス用） | `tea.Tick` |

すべての非同期処理は `tea.Cmd` を返す関数として記述し、`Update` は **Cmd を返すだけで I/O を直接呼ばない**。

## 5. Startup Sequence

```
main.go
 └─ parse args
     ├─ no arg            → rootDir = cwd, initial = dir         [v0.1]
     ├─ <file>            → rootDir = file の親, initial = file  [v0.2]
     └─ <commit>          → rootDir = cwd, initial = commit      [v0.2]
 └─ NewModel(...)
     └─ Init() returns batched Cmds:
         - buildTreeCmd(rootDir)
         - openGitCmd(rootDir)               // 非 git なら nil repo を返す
         - startWatcherCmd(rootDir)
         - initial に応じて: loadFileCmd / loadDiffCmd / loadCommitCmd
 └─ tea.NewProgram(model).Run()
```

引数解析の分岐自体は v0.1 から構造として用意するが、`<file>` / `<commit>` 経路の本実装は v0.2 で行う。v0.1 では引数を受けても警告を出して `no arg` 経路にフォールバックする。

`Init` で投げる Cmd は **失敗が許容される**。Git が無いディレクトリでもアプリは起動する。サイドバーの「変更ファイル」モードはその場合「Not a git repository」を表示する。

## 6. Components

### 6.1 Sidebar

サイドバーは現在のモードを enum で持つ：

- `ModeTree`：再帰的にディレクトリを表示。展開状態を保持する。
- `ModeChanges`：`git status --porcelain` の結果を Modified / Added / Deleted / Untracked で並べる。

両モードで共通の動き：

- `↑/↓` で項目移動、`Enter` で「メインビューに開く」メッセージを `app` に投げる。
- `←/→` でモード切替（v0.1 では 2 モードのみだが、将来の拡張に備えて enum で扱う）。

サイドバーは **モードごとに別の slice を持ち、共通の interface（`Item { Label() string; Open() OpenIntent }`）でレンダリングする**。これにより新モード追加時の差分が小さくなる。

### 6.2 Main View

メインビューは現在の表示種別を enum で持つ：

- `ViewFile`：ファイル本文 + シンタックスハイライト。Bubbles の `viewport` を使用。
- `ViewDiff`：unified diff。`diffview` パッケージで前処理してから viewport に流し込む。
- `ViewCommit`：起動時 `<commit>` で渡されたコミットの diff。中身は `ViewDiff` と同じレンダラを使う。
- `ViewEmpty`：起動直後やファイル未選択時のプレースホルダ。

カーソル位置・スクロール位置は **(view kind, identifier) をキーとした LRU で保持** する。同じファイルに戻ったときに位置が戻る挙動を `Concept.md` の v0.1 要件として満たす。

### 6.3 Help Overlay

`?` でモーダル風のヘルプを開く。実装は本物のモーダルではなく、メインビューの上に `lipgloss.Place` で重ねる軽量実装で十分（v0.2 以降）。

## 7. File Watching

### 7.1 fsnotify wiring

- 起動時に `rootDir` 配下を **再帰的に Watch する**。`vendor` / `node_modules` / `.git` は Watch しない（v0.1 ハードコード）。
- 各イベントは `FsEventMsg` として `app` に届ける。
- サブディレクトリが新規作成された場合、watcher 側で自動的に `Add` する。

### 7.2 デバウンス

エディタは 1 回の保存で複数イベントを発火しがち（特に atomic save：CREATE → RENAME → REMOVE）。50ms のデバウンスをかけ、同一パスのイベントは最後の 1 回にまとめる。実装は watcher パッケージ内のリングバッファ + `time.AfterFunc`。

### 7.3 Atomic save 対応

`fsnotify` で `RENAME` を受けた場合、ファイル自体は新しい inode に置き換わっている。Watch 対象を再追加するために、デバウンス後に `Stat` し直し、存在すれば `watcher.Add(path)` を再発行する（v0.2 で正式対応、v0.1 は best-effort）。

### 7.4 Git の変化検知

`.git/HEAD` と `.git/index` を fsnotify で見る。変更を検知したら：

- HEAD：`GitHeadChangedMsg` を発行 → サイドバーの changes モードと、開いている commit ビューを再読込。
- index：`GitStatusMsg` を発行 → changes モードのリストを再構築。

## 8. Git Integration

v0.1 は **`os/exec` で `git` コマンドを呼ぶ** 方針を採る。理由：

- `go-git` はバイナリサイズと依存が重く、コンセプトの「軽い、静か」と衝突する。
- 必要な操作が `git status --porcelain=v1 -z`、`git diff`、`git show <commit>`、`git rev-parse HEAD` だけで足りる。
- ユーザーの `~/.gitconfig` がそのまま反映される（特に core.pager / color.* の扱い）。

呼び出し時の方針：

- 標準出力をパースする層を `internal/git` に閉じる。`app` は **Go の構造体しか見ない**。
- diff 取得時は `--no-color` を付け、色付けは `diffview` 側で行う（テーマ統一のため）。
- 大きいリポジトリでも UI が固まらないよう、Git 呼び出しは必ず `tea.Cmd` 経由（goroutine）で行う。

非 git ディレクトリでの起動は完全にサポートする。`repo == nil` のとき changes モードは無効化メッセージのみ表示する。

## 9. Syntax Highlighting

### 9.1 Chroma の使い方

- ファイル拡張子 + 先頭バイト数 KB から lexer を選ぶ（`lexers.Match` → `lexers.Analyse` のフォールバック）。
- フォーマッタは ANSI 256 色を基本、`COLORTERM=truecolor` を検出した場合のみ true color。
- スタイルは Lipgloss の `theme` パッケージから注入する。Chroma 標準スタイル名（例：`monokai`）を直接使わず、`theme.SyntaxStyle` の薄い変換層を挟む。

### 9.2 大きいファイル / バイナリ

- 1 MB を超えるファイルはハイライトせず、プレーン表示 + バナーを出す（v0.1）。
- バイナリ判定は先頭 8 KB に NUL バイトが含まれるかで簡易判定。バイナリは「Binary file」とだけ表示する。
- v0.3 でストリーミングハイライトを検討。

### 9.3 キャッシュ

`(path, mtime, size)` をキーに、ハイライト済みの行配列をメモリキャッシュ。同じファイルへの再フォーカス時に即時再表示できる。エントリ上限は 128（LRU）。

## 10. Diff Rendering

### 10.1 入力

- working tree の未コミット変更：`git diff --no-color`
- コミット指定：`git show --no-color <commit>`

### 10.2 パース

- `diff --git`、`@@` ヘッダ、`+ - 空白` 行を最小限に分類。
- バイナリ diff（`Binary files ... differ`）は専用行種別として保持。

### 10.3 表示

- unified 表示のみ（Non-Goals に明記）。
- 行頭の `+` `-` をスタイルで色分け。コンテキスト行は dim カラー。
- ハンクヘッダはセクション見出しとして強調。
- 折り返しは on（横スクロールなしの方針）。長行は次行にインデント付きで継続。

## 11. File Tree

- 構築は遅延展開：起動時にはルート直下のみ列挙。展開時に子をロード。
- 除外規則は v0.1 では固定（`.git`、`node_modules`、`vendor`）。Concept.md の MVP 要件と同じセット。
- v0.3 で `.gitignore` のフルパース（go-gitignore など）に切り替え。
- ファイル数が多いディレクトリは「+ N more」で打ち切り（しきい値はとりあえず 500）。

## 12. Keymap & Focus

```
focus = sidebarFocus | mainFocus
```

- `Tab`：フォーカス切替。
- 各フォーカスでの `↑/↓` は意味が異なる（項目移動 / 縦スクロール）。
- 共通キー（`q`、`?`）は focus に関係なくルート Update で先に拾う。
- `keymap` パッケージで **キー → 抽象アクション** のマップを持ち、コンポーネント側はアクションでだけ判断する。これにより v0.3 のキー再マップが安全。

## 13. Theme & Styling

- Lipgloss でスタイル定義をテーマパッケージに集約。
- 配色は基本 1 種（dark）。light は v0.3 以降。
- `NO_COLOR` 環境変数を尊重。
- 幅 60 桁未満の場合はサイドバーを縮小し、40 桁未満ではメインビューのみ表示するレイアウト降格モードを持つ（narrow terminal friendly）。

## 14. Performance Considerations

- 再描画は viewport 内の行のみ。Lipgloss のスタイル文字列は事前生成してキャッシュ。
- ハイライトとファイル読み込みは別 Cmd に分け、ハイライト前にプレーンテキストで先に描画する（体感速度の確保）。
- fsnotify のバーストはデバウンスで吸収。
- `git status` は連続呼び出しを 200ms 間隔で間引く。

## 15. Error Handling & Edge Cases

- ファイル読み込み失敗（permission / not found）はエラーバナーをメインビュー上部に出す。アプリは終了しない。
- シンボリックリンクはターゲットを 1 段だけ追う。ループ検出は inode 記録で行う（v0.2）。
- 非 UTF-8 ファイルは BOM 検出 + Shift_JIS / EUC-JP の簡易判定（v0.3、それまでは UTF-8 のみ表示し他は「Cannot decode」）。
- ターミナルが極端に小さい場合（10×3 など）は描画を止め、リサイズ待ちメッセージを出す。

## 16. Testing Strategy

| 種別 | 対象 | ツール |
|---|---|---|
| Unit | diff パース / git status パース / ツリー構築 / 除外ルール | 標準 testing |
| Snapshot | View の出力 | [`teatest`](https://github.com/charmbracelet/x/tree/main/exp/teatest) |
| Integration | 一時 git リポジトリでの起動〜操作シナリオ | testing + tempdir |
| Manual | 実マシンで `keifu` と組み合わせた使用感 | — |

CI（GitHub Actions）で `go vet` / `staticcheck` / `go test ./...` を回す。

## 17. Build & Distribution

- **GoReleaser**：darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64 をビルド。
- **Homebrew tap**：`gen-hiroto0119/tap`。
- **`go install github.com/gen-hiroto0119/sus4/cmd/sus4@latest`** をサポート。
- バイナリサイズは `-trimpath -ldflags="-s -w"` で最小化。

## 18. Open Questions

実装着手までに決めたい論点：

1. **MVP のサイドバー幅**：固定 30 桁か、画面幅の 30% か。
2. **viewport の選定**：Bubbles の `viewport` で行折り返し + ハイライトが快適か。重い場合は自前実装。
3. **コミット指定の解決**：`<commit>` は `git rev-parse` で SHA に正規化してから保持するか、生文字列で持って毎回解決するか。
4. **ファイルツリーの並び順**：ディレクトリ先 → ファイルの dotfile ありなし扱い。
5. **キャッシュサイズ**：ハイライト LRU の 128 は妥当か、メモリ上限で切るか。

これらは v0.1 の最初の実装イテレーションで実測しながら決める。

## 19. Out of Scope（再掲）

Concept.md の Non-Goals に加え、本ドキュメントで扱わないもの：

- マウス操作対応
- リモートリポジトリの操作
- 複数ペインの分割
- 検索（v0.3 の fuzzy find に切り出し）
- ロケール別ソート / 国際化メッセージ

## 20. Next Step

このドキュメントの確定後、次の作業は：

1. `cmd/sus4` と `internal/app` の最小骨格（空 Model + 起動だけ）を切る。
2. `internal/filetree` と `internal/git`（status / diff のみ）を **UI 抜きで** 単体テスト先行で書く。
3. `internal/app` から両者をつなぎ、サイドバーと最低限の main view を出す。
4. `internal/watcher` を統合し、自動追従を有効化する。

`v0.1` のリリースクライテリアは「Claude Code がファイルを書き換えると、`sus4` の表示が 1 秒以内に追従する」を満たすこと。
