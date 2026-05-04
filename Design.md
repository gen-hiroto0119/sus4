> Detailed design for tetra — the silent code viewer.

このドキュメントは [`Concept.md`](Concept.md) の続編として、**実装上の判断**を記述する。
「何を作るか」「なぜ作るか」は Concept.md を、「どう作るか」はこのドキュメントを参照する。

## 0. Scope of this document

- 対象は v0.1（MVP）と v0.2 までの実装方針。v0.3 以降は方向性のみ言及する。
- パッケージ構成、状態モデル、メッセージフロー、各コンポーネントの責務、外部システム（ファイルシステム / Git）との結合点を扱う。
- 具体的な API シグネチャや関数名は固定しない。実装中に最適形を選べる余地を残す。

## 1. Architecture Overview

`tetra` は Bubble Tea の **Elm Architecture** に素直に乗る。グローバルな可変状態は持たず、すべての UI 変化は `Msg → Update → Model → View` の 1 サイクルで起きる。

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
tetra/
├── cmd/
│   └── tetra/
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

`Shift+?` (= bubbletea 的には `?` キー入力) でヘルプを開く。実装はモーダル
オーバーレイではなく、`m.helpOpen == true` のあいだ `View()` を完全に置き換える
**全画面takeover** 方式 (`internal/app/view.go: renderHelp`)。Lipgloss にレイヤ
合成が無いので、これがレイアウト計算 (m.width × m.height) を一致させたまま
実装するいちばん素直な道。ヘルプ表示中は `Quit` と `Help toggle` 以外のキーを
swallow する (update.go の `handleKey` 冒頭で分岐) — 裏に隠れているペインを
ユーザが意図せず駆動するのを防ぐため。v0.1 で shipped。

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
- すべての git 呼び出しは `--no-optional-locks` を先頭に付けて発行する
  （`internal/git/git.go: run`）。これは git の「副次的書き込み」を抑止し、
  特に `git status` がデフォルトで実行する `.git/index` の stat refresh を
  止める。これがないと、自分の `git status` 実行 → `.git/index` 更新 →
  watcher が変更を検知 → `gitMetaMsg` 発火 → 別の git 呼び出し、という
  自己フィードバックループが回って ~25% one core を fork-exec に持っていかれる。

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

### 10.4 ファイル閲覧中の変更マーカー (git gutter)

ファイル閲覧 (`ViewFile`) でも HEAD との差分を行番号 gutter の左に
細バー (`▎`) で表示する。VSCode / JetBrains の git gutter と同じ意図。

- パイプライン: `repo.DiffFile(path)` を非同期で叩き、出力を
  `diffview.Markers` で `map[newLineNo]ChangeKind` に変換、
  `mainview.SetMarkers` で渡す。
- 分類規則 (`diffview/markers.go`):
  - 純粋な `+` 連 → **ChangeAdd**
  - `-` 連 → 直後の `+` 連がそのファイル内ハンクに続けば **ChangeMod**
    (全 `+` 行に同じ分類)、無ければ **ChangeDel** を直後の生存行 (=
    新ファイルでも残っている文脈行) にスタンプ
- 色: `theme.DiffAdd` / `theme.DiffHunk` / `theme.DiffDel` を流用、
  別フィールドは作らない。
- 発火タイミング: ファイル open 時、対象ファイルの fs event 受領時、
  `gitMetaMsg` (HEAD/index 変化) 受領時。git diff の呼び出しは
  `gitCmdTimeout` で打ち切り、失敗 / 非 git / リポジトリ外パスは
  `cleared` メッセージで gutter 列ごと隠す。
- レンダラ側のルール: `m.fileMarkers == nil` のときは marker 列を
  描かず、行番号 + 1 セルだけ。`map{}` (空 map) なら列は出すが
  グリフは描かない (clean tree の安定表示)。
- 未対応 (v0.2 以降): untracked ファイルを「全行 add」で塗ること、
  staged-only モードへの切替、行単位の hover preview。

## 11. File Tree

- 構築は遅延展開：起動時にはルート直下のみ列挙。展開時に子をロード。
- 除外規則は v0.1 では **固定** で `internal/filetree/filetree.go: defaultExcludes`
  に列挙。VCS メタ (`.git`, `.hg`, `.svn`)、依存物 (`node_modules`, `vendor`)、
  ビルド/キャッシュ (`.next`, `.nuxt`, `.svelte-kit`, `.turbo`, `.parcel-cache`,
  `.cache`, `dist`, `build`, `out`, `target`, `coverage`)、Python 系
  (`__pycache__`, `.pytest_cache`, `.venv`, `venv`, `.tox`)、IDE
  (`.idea`, `.vscode`)。これらを watch から外し、tree レンダにも出さない
  ことで「busy なプロジェクトでも steady-state CPU が上がらない」状態を
  確保している（実測の主犯はこの除外漏れだった）。
- v0.3 で `.gitignore` のフルパース（go-gitignore など）に切り替え。
- ファイル数が多いディレクトリは「+ N more」で打ち切り（しきい値はとりあえず 500）。

## 12. Keymap & Focus

```
focus = sidebarFocus | mainFocus
```

- `Tab`：フォーカス切替。
- 各フォーカスでの `↑/↓` (vim 等価 `k/j`) は意味が異なる（項目移動 / 縦スクロール）。
- `←/→` (vim 等価 `h/l`) はサイドバーモード切替（files ⇄ changes）。
- ページ移動: `PgUp` / `Ctrl-B` / `PgDn` / `Ctrl-F` / **`Space`** (PgDn 同等)。
- ジャンプ: `g` / `Home` で先頭、`G` / `End` で末尾。
- 共通キー（`q` / `Ctrl-C` / `Shift+?` ）は focus に関係なくルート Update で先に拾う。
- `keymap` パッケージで **キー → 抽象アクション** のマップを持ち、
  コンポーネント側はアクションでだけ判断する。これにより v0.3 のキー再マップが安全。
- `Shift+?` の実体は bubbletea が emit する `?` 文字。printable な
  shifted character は別の `"shift+?"` イベントを生まないので、`keymap.go`
  の `case "?"` でカバーされる（コメントで明記）。

## 13. Theme & Styling

- Lipgloss でスタイル定義をテーマパッケージに集約 (`internal/theme`)。
- 配色は **dark / light の 2 種** を v0.1 で提供。`config.toml` の
  `theme = "auto" | "dark" | "default" | "light"` で選ぶ。"auto" (デフォルト)
  はターミナル背景を OSC 11 / termenv 経由で問い合わせ、結果に応じて
  Default (dark) / Light をその場で選択する。`Theme.IsDark` フラグを
  渡すことで chroma syntax style と glamour markdown render が
  light / dark に追従する。
- `NO_COLOR` 環境変数を尊重。
- レイアウト降格は **単一閾値** で行う（narrow terminal friendly）：
  - 幅 60 桁以上：サイドバー (30% 幅、clamp `[18, 40]`) + メインビュー。
  - 幅 60 桁未満：サイドバーを丸ごとドロップし、メインビューのみで描画。
  - これより細かい段階 (例：サイドバーだけ縮める中間段階) は意図的に持たない。
    実装値は `internal/app/view.go` の `mainOnlyWidth` / `sidebarMinWidth` /
    `sidebarMaxWidth` / `sidebarPercent` を参照。

## 14. Performance Considerations

「画面に出るのはコードと diff だけ、バックグラウンドで何も走らせない」
原則を **CPU 計測レベルで満たす** ために以下を多段で重ねる。すべて実機の
プロファイル / `internal/app/cpu_probe_test.go` で確認済み。

### 14.1 描画とイベント
- 再描画は viewport 内の行のみ。Lipgloss のスタイル文字列は事前生成してキャッシュ。
- ハイライトとファイル読み込みは別 Cmd に分け、ハイライト前にプレーンテキストで先に描画する（体感速度の確保）。
- fsnotify のバーストはデバウンスで吸収（`watcher` の 50ms ウィンドウ）。
- TAB は読み込み時に 4 スペースに展開し、`ansi.StringWidth` の 1-cell 計上と端末描画幅のズレを排除する。

### 14.2 throttle 階層 (`internal/app/update.go`)
- `statusThrottle = 200ms`: `git status` / `git diff` / structural tree refresh /
  gitMetaMsg 経由の markers 更新。
- `fileReloadThrottle = 500ms`: 開いている file 本体 + markers のリロード。
  chroma highlight が 50–200ms / call と重いので、status より広めに取る。
- 上記なしだと、build watcher が 10–20 Hz で書き換えるファイルを開くと
  CPU が一気に 200%+ に跳ねる。

### 14.3 View() の revision-cache (`internal/app/view.go`)
- bubbletea は Update のたび View() を呼ぶ。lipgloss の `applyBorder` /
  `ansi.StringWidth` は 1 frame で 230 KB / 1,222 alloc を吐くので、
  ナイーブに毎 frame 走らせると GC + alloc だけで CPU を喰う。
- `mainview.Model` / `sidebar.Model` に `revision int` を持たせ、状態を
  変える各 mutator で `revision++`。`app.viewCache` は (width, height, focus,
  helpOpen, sidebarRev, mainRev, errMsg) をキーに最後の View() string を
  記憶し、同じキーが来たら即返す。
- 結果: cpu_probe bench で **277 µs/op → 300 ns/op** (920×)。

### 14.4 git の自己フィードバック切り
- すべての git 呼び出しに `--no-optional-locks` を付ける（§8 参照）。
- それでも `gitMetaMsg` (HEAD/index 変化) → markers 再計算は無条件に
  fork すると振動するので、`maybeMarkersOnlyCmd` で `lastMarkersReq` の
  200ms throttle に通す。

### 14.5 CI bench gate (`internal/app/cpu_probe_test.go` + `.github/workflows/ci.yml`)
- `BenchmarkUpdateFsEventBurst` は temp git repo + 200 行の Go ファイルを
  作り、Open dispatch → Update(fsEventMsg) + View() のループを回す。
- CI で `go test -bench` を走らせ、ns/op が **5,000** を超えると workflow
  が fail する。「hot path に syscall を入れる / View を毎 frame 再計算する」
  系の regression が PR 段階で止まる。

## 15. Error Handling & Edge Cases

- ファイル読み込み失敗（permission / not found）はエラーバナーをメインビュー上部に出す。アプリは終了しない。
- シンボリックリンクはターゲットを 1 段だけ追う。ループ検出は inode 記録で行う（v0.2）。
- 非 UTF-8 ファイルは BOM 検出 + Shift_JIS / EUC-JP の簡易判定（v0.3、それまでは UTF-8 のみ表示し他は「Cannot decode」）。
- ターミナルが極端に小さい場合（10×3 など）は描画を止め、リサイズ待ちメッセージを出す。

## 16. Testing Strategy

| 種別 | 対象 | ツール |
|---|---|---|
| Unit | diff パース / git status パース / ツリー構築 / 除外ルール | 標準 testing |
| Integration | 一時 git リポジトリでの dispatch → Update → View 経路 | `internal/app/integration_test.go` (TestMarkersAppearForGoFile 等) |
| **CPU bench** | fs-event burst の steady-state cost | `internal/app/cpu_probe_test.go: BenchmarkUpdateFsEventBurst` |
| Snapshot | View の出力 | [`teatest`](https://github.com/charmbracelet/x/tree/main/exp/teatest)（v0.2 以降） |
| Manual | 実マシンでの使用感 | — |

CI（GitHub Actions, `.github/workflows/ci.yml`）で `go vet` / `go test ./...` /
`go build ./...` / **CPU bench gate** を回す。bench gate は
`BenchmarkUpdateFsEventBurst` の ns/op が 5,000 を超えると job が fail —
「hot path に syscall を入れる / View を毎 frame 再計算する」級の
regression が PR 時点で止まる仕掛け（§14.5 参照）。

## 17. Build & Distribution

- **GoReleaser**：darwin/amd64, darwin/arm64, linux/amd64, linux/arm64, windows/amd64 をビルド。
  タグ push (`v*`) で `.github/workflows/release.yml` が発火して GitHub
  Release を自動生成する。
- **`go install github.com/gen-hiroto0119/tetra/cmd/tetra@latest`** が
  正規の install 経路。`runtime/debug.ReadBuildInfo()` を併用するので
  `tetra --version` がタグ付き install で正しく version を出す。
- バイナリサイズは `-trimpath -ldflags="-s -w"` で最小化。
  goreleaser ビルド時のみ `-X main.version={{.Version}}` を加えて版表示。
- **`tetra update`** サブコマンド (`cmd/tetra/main.go: runUpdate`) は
  `go install ...@latest` を内部で `os/exec` 経由で実行する薄いラッパ。
  `go` が PATH に無い時は Releases ページに誘導するメッセージを出す。
  Concept §1「静か」の枠内 — 起動時の自動チェックは持たず、
  ユーザが明示的に叩いた時だけネットワークに行く。
- **Homebrew tap は提供しない**。`tetra` の名前は homebrew-core の
  cilium/tetragon CLI と `bin/tetra` レベルで衝突しており、回避策は
  formula 名 / binary 名 / alias 案内のいずれも UX を損ねる。
  この結論は本リポジトリの直近コミット履歴（`revert: drop Homebrew
  distribution, go-install-only`）に対応している。

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

1. `cmd/tetra` と `internal/app` の最小骨格（空 Model + 起動だけ）を切る。
2. `internal/filetree` と `internal/git`（status / diff のみ）を **UI 抜きで** 単体テスト先行で書く。
3. `internal/app` から両者をつなぎ、サイドバーと最低限の main view を出す。
4. `internal/watcher` を統合し、自動追従を有効化する。

`v0.1` のリリースクライテリアは「Claude Code がファイルを書き換えると、`tetra` の表示が 1 秒以内に追従する」を満たすこと。
