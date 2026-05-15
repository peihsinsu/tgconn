## 1. Storage package (`internal/storage/`)
- [x] 1.1 新增 `internal/storage/resolver.go`：實作 `EncodeCWD(cwd string) string`（`/` → `-`）
- [x] 1.2 實作 `Resolver.ProjectDir()`：回傳 `~/.tgconn/projects/<encoded>/`（用 `os.UserHomeDir`）
- [x] 1.3 實作 `Resolver.Subdir(name string) string`：回傳 `<projectDir>/<name>/`（tmp、cron 等共用）
- [x] 1.4 寫單元測試：`EncodeCWD` 對常見路徑、根目錄、含空白路徑的行為

## 2. Config 擴充 (`internal/config/config.go`) *(Part 2)*
- [ ] 2.1 `Config` 新增 `TmpRetentionHours`、`LogRetentionDays`、`HistoryMaxEntries`
- [ ] 2.2 `Load()` 讀取對應 viper key，套用預設值（24、30、100）
- [ ] 2.3 `Validate()` 加入非負值檢查，違反回傳清楚錯誤
- [ ] 2.4 config_test.go 補測試：預設值套用、負值拒絕

## 3. Recorder 依賴注入 (`internal/recorder/recorder.go`)
- [x] 3.1 移除 const `logDir` / `cronLogDir`，改為 `Recorder` struct field（`dir`、`cronDir`）
- [x] 3.2 `New(baseDir string) (*Recorder, error)`：caller 傳入 base，內部組合 `<base>` 與 `<base>/cron`
- [x] 3.3 更新 recorder_test.go 用 `t.TempDir()` 注入測試目錄

## 4. Migration 模組 (`internal/storage/migrate.go`)
- [x] 4.1 實作 `MigrateLegacy(legacyDir, targetDir string) (bool, error)`
- [x] 4.2 兩處都存在（且非僅麵包屑）→ 回傳明確錯誤
- [x] 4.3 主要路徑用 `os.Rename`；任何失敗 → fallback 用 copy-then-remove
- [x] 4.4 成功後在原 `<cwd>/.tgconn/` 建立 `MOVED_TO_<encoded>.txt`（含絕對路徑與 RFC3339 時間戳）
- [x] 4.5 單元測試：legacy 存在搬移成功、衝突案例、無 legacy 跳過、re-entrant、empty target 容忍、copy fallback

## 5. Cleanup 模組 (`internal/storage/cleanup.go`) *(Part 2)*
- [ ] 5.1 新增 `cleanup.go`：`RunStartup(cfg StartupConfig) Result`
- [ ] 5.2 實作 `cleanTmp(dir string, olderThan time.Duration) (removed int, freed int64, err error)`
- [ ] 5.3 實作 `cleanLogs(dir string, olderThan time.Duration) (removed, freed, err)`，0 表示 skip
- [ ] 5.4 實作 `compactHistory(dir string, maxEntries int) (compacted int, err error)`，0 表示 skip
- [ ] 5.5 實作 `Stats` struct 回傳清理摘要供 logging
- [ ] 5.6 cleanTmp 完成後刪除空 `<chatID>/` 子目錄
- [ ] 5.7 單元測試（用 `t.TempDir()` 構造各種檔案與 mtime）

## 6. Bot 整合 (`internal/bot/bot.go`)
- [x] 6.1 `New` 改為接收 `projectDir string`
- [x] 6.2 `downloader.New(api, filepath.Join(projectDir, "tmp"))`
- [x] 6.3 `cronjob.New(filepath.Join(projectDir, "cron"), ...)`
- [x] 6.4 移除剩餘的 `.tgconn` 字串字面值

## 7. Transcriber 中間檔處理 (`internal/transcriber/transcriber.go`)
- [x] 7.1 `.wav` 中間檔（whisper.cpp 路徑）— 既有 `cleanup()` 已 defer，確認保留
- [x] 7.2 `.txt` 輸出 — openai-whisper 與 whisper.cpp 兩條路徑都加 `defer os.Remove(outFile)`
- [x] 7.3 失敗路徑也走同一個 defer（defer 在 outFile 設定後立刻註冊，無論後續錯誤都會跑）
- [ ] 7.4 補測試覆蓋兩個 backend 的清理行為（subprocess 不易 mock，先略過；實機驗證涵蓋）

## 8. Connect 流程整合 (`cmd/connect.go`)
- [x] 8.1 取得 cwd → 解析 `storage.NewResolver().ProjectDir()`
- [x] 8.2 呼叫 `storage.MigrateLegacy(legacyDir, targetDir)`，失敗中止啟動
- [x] 8.3 `os.MkdirAll(targetDir, 0755)`
- [ ] 8.4 呼叫 `cleanup.RunStartup(...)`，log 清理摘要 *(Part 2)*
- [x] 8.5 `recorder.New(targetDir)` / `bot.New(..., projectDir)`
- [x] 8.6 啟動 log 中印出實際使用的 storage path

## 9. Clean 子指令 (`cmd/clean.go`) *(Part 2)*
- [ ] 9.1 新增 `cleanCmd`（cobra）：支援 `--tmp` / `--logs` / `--history` / `--all` / `--dry-run` / `--yes`
- [ ] 9.2 解析旗標 → 組出要處理的檔案清單（含大小）
- [ ] 9.3 `--dry-run`：印清單與總大小,不刪
- [ ] 9.4 `--history` 或 `--all` 且非 `--yes` 且非 `--dry-run`：互動 prompt `yes` 才刪
- [ ] 9.5 印出最終結果（刪除檔案數、釋放空間）
- [ ] 9.6 無旗標 → 印 usage 並 exit 非 0
- [ ] 9.7 註冊 `cleanCmd` 進 `rootCmd`

## 10. 文件與驗證
- [ ] 10.1 更新 `CLAUDE.md`：儲存路徑說明、retention 設定、`tgconn clean` 用法
- [ ] 10.2 `openspec validate centralize-storage --strict` 通過
- [x] 10.3 `go vet ./...` 通過（Part 1）
- [x] 10.4 `go test ./...` 全部通過（Part 1；既有 `TestLoad_DefaultTimeout` 失敗為 pre-existing 15m vs 2h 不一致，與本改動無關）
- [ ] 10.5 實機驗證：
  - [ ] 10.5.1 既有 `.tgconn/` 存在 → 啟動後資料搬到中央位置，麵包屑留存
  - [ ] 10.5.2 無 `.tgconn/` 啟動 → 直接用中央位置，不報錯
  - [ ] 10.5.3 跑 `tgconn clean --all --dry-run` → 只列不刪 *(Part 2)*
  - [ ] 10.5.4 跑 `tgconn clean --history` → 出現確認 prompt *(Part 2)*
  - [ ] 10.5.5 voice 轉錄完成後 `.wav` / `.txt` 確實消失
