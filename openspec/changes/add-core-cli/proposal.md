## Why

tgconn 目前沒有任何實作。本提案定義核心 CLI 能力：啟動 Telegram bot、將訊息轉發給 LLM provider subprocess、將回應送回 Telegram。

## What Changes

- 新增 `telegram-connector` capability spec（核心功能）
- 建立 Go 專案骨架（module、package layout）
- 實作 `tgconn connect` 主要指令（含 `--provider`、`--debug`、`--allow-chat` flags）
- 實作 `tgconn config init` / `tgconn config show` 指令
- 實作 Provider 抽象介面與 Claude 實作（第一個 provider）
- Telegram long-polling 訊息接收與回應
- 安全管控：`--allow-chat` 白名單，預設拒絕所有來源

## Impact

- Affected specs: `telegram-connector` (新建)
- Affected code: 全新專案，無現有程式碼衝突
