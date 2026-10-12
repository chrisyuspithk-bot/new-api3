# B2B2C Phase 1 — 月結發票功能測試／QA 報告

| 項目 | 內容 |
|------|------|
| 產品／分支 | `new-api` / `feat/b2b2c-phase1` |
| 功能 | 月結發票（Customer／Reseller／Admin rollup） |
| 測試類型 | API 自動化＋Playwright UI 截圖＋既有 B2B2C 回歸摘要 |
| 測試日期 | 2026-10-12（HKT） |
| 測試人員 | Grok Bot QA（executor） |
| 程式碼 SHA | `4e5a8f7e2e7127ed3b8c2e99a5bd770452acef08` |
| 遠端 | `https://github.com/chrisyuspithk-bot/new-api3.git` `feat/b2b2c-phase1` |
| 報告路徑 | `/workspace/b2b2c-pro-qa/REPORT-invoices.md`／`.pdf`（同步 `docs/b2b2c-pro-qa/`） |

---

## 1. 測試範圍

### 1.1 本報告新增（月結發票）
1. Admin：依曆月（Asia/Hong_Kong）從 consume／refund 日誌產生發票；列表／詳情／CSV  
2. Reseller：僅能看自己的 reseller 發票與所屬客戶 customer 發票  
3. Customer：僅能看自己的 customer 發票；錢包頁「Invoice History」  
4. 金額：USD = quota / `QuotaPerUnit`（500000）；含 charge／cost／profit  
5. 作用域隔離：Reseller 不可讀 admin_rollup；Customer 不可讀 reseller／他人發票  

### 1.2 既有 B2B2C 煙霧（摘要自先前專業 QA／retest）
公開註冊預設關閉、線上自助儲值、dual-leg 消費庫存、跨代理商隔離、停用凍結等 — 詳見 `REPORT.md`／`REPORT-retest.md`。本輪發票測試重用同一 SQLite QA 庫與帳號。

---

## 2. 測試環境

| 項目 | 值 |
|------|-----|
| OS | Linux（box） |
| Backend | `/tmp/new-api-b2b2c`（含 `web/dist` embed，commit `4e5a8f7e2`） |
| DB | SQLite `/tmp/b2b2c-pro-qa.db` |
| URL | `http://127.0.0.1:3310` |
| Redis | 未啟用 |
| 測試帳號 | `admin` / `reseller1`・`reseller2` / `cust1`・`cust1b`・`cust2` |
| 發票期間 | `2026-10`（HKT 曆月；測試前寫入 seed consume／refund 日誌） |

---

## 3. API／路由一覽（本功能）

| 角色 | 方法 | 路徑 | 說明 |
|------|------|------|------|
| Admin | `POST` | `/api/admin/invoices/generate` | `{period:"YYYY-MM"}` 產生／覆寫 |
| Admin | `GET` | `/api/admin/invoices` | 列表（period／scope／reseller_id／customer_id） |
| Admin | `GET` | `/api/admin/invoices/:id` | JSON 詳情；`?format=csv` 下載 |
| Reseller | `GET` | `/api/reseller/invoices` | 自己＋客戶 |
| Reseller | `GET` | `/api/reseller/invoices/:id` | 詳情／CSV |
| Customer | `GET` | `/api/user/invoices` | 自己的 customer 發票 |
| Customer | `GET` | `/api/user/invoices/:id` | 詳情／CSV |

**前端路由：** Admin `/invoices`；Reseller `/reseller/invoices`；Customer `/wallet`（Invoice History 卡片）。

---

## 4. 發票用例結果

| 編號 | 步驟 | 期望 | 實際 | 結果 |
|------|------|------|------|------|
| INV-01 | Admin 登入 | success | HTTP 200 success | **Pass** |
| INV-02 | `POST …/generate` period=`2026-10` | 產生 customer／reseller／admin_rollup | `customer_invoices=2`, `reseller_invoices=1`, `admin_rollup=true` | **Pass** |
| INV-02b | period=`2026-9`（非法） | 拒絕 | `period must be YYYY-MM` | **Pass** |
| INV-03 | Admin 列表 `period=2026-10` | 三種 scope | n=4；scopes=`admin_rollup,customer,reseller`；rollup charge≈$3.40 | **Pass** |
| INV-04 | Admin get 詳情 | invoice＋line_items | line_items 有模型明細；charge_usd 正確 | **Pass** |
| INV-05 | Admin `?format=csv` | CSV 含表頭 period | HTTP 200，表頭含 `period,scope,…` | **Pass** |
| INV-06 | Reseller1 登入 | success | success | **Pass** |
| INV-07 | Reseller 列表 | 無 admin_rollup | n=3；scopes=`customer,reseller` | **Pass** |
| INV-07b/c | Reseller get／CSV | success／200 | 皆成功 | **Pass** |
| INV-08 | Reseller 讀 admin_rollup id | 404 | `invoice not found` | **Pass** |
| INV-09 | Cust1 登入 | success | success | **Pass** |
| INV-10 | Customer 列表 | 僅自己 customer | n=1；customer_id=4；charge=$1.40 | **Pass** |
| INV-10b/c | Customer get／CSV | success／200 | 皆成功 | **Pass** |
| INV-11 | Customer 讀 reseller 發票 | 404 | 404 | **Pass** |
| INV-12 | Reseller2 讀 Reseller1 客戶發票 | 404 | 404 | **Pass** |
| INV-UI-01 | Admin UI `/invoices` | 列表可見三種 scope | 截圖 `admin-invoices.png` | **Pass** |
| INV-UI-02 | Reseller UI `/reseller/invoices` | 客戶＋Own | 截圖 `reseller1-invoices.png` | **Pass** |
| INV-UI-03 | Customer UI `/wallet` Invoice History | 顯示 2026-10 $1.40 | 截圖 `cust1-wallet-invoices.png` | **Pass** |

**本輪發票：Pass 18／Fail 0**（UI 以 API session 注入避開登入速率限制；API 帳密皆通）。

### 金額核對（seed 日誌）
- cust1：consume 500k＋250k − refund 50k = **700k quota** → **$1.40**；cost 200k＋100k−20k=280k → **$0.56**；profit **$0.84**  
- cust1b：consume 1,000k → **$2.00**；cost 400k → **$0.80**；profit **$1.20**  
- reseller／admin_rollup：charge **$3.40**；cost **$1.36**；profit **$2.04**  

與畫面／API 一致。

---

## 5. 既有 B2B2C 煙霧摘要（先前報告）

詳見同目錄 `REPORT.md`、`REPORT-retest.md`（2026-10-12）。重點結論：

| 主題 | 結果 |
|------|------|
| 公開／密碼註冊預設關閉；OAuth 新用戶註冊關閉 | Pass |
| BUG-01：`/sign-up` 在註冊關閉時導向 `/sign-in` | Pass（retest） |
| Admin Reseller CRUD／入帳／停用 | Pass |
| Reseller 客戶／加價／Token／兌換碼／日誌／毛利 | Pass |
| Customer 個人資料代理商卡片；錢包線上儲值 UI 未封鎖 | Pass |
| 消費 dual-leg：無代理商庫存拒絕；有庫存可結算；日誌帶 reseller 欄位 | Pass |
| 跨代理商隔離 404／403 | Pass |
| 單元測試 `go test ./model ./middleware ./controller ./router`（含 `invoice_test.go`） | Pass |

---

## 6. 截圖索引

| 檔名 | 說明 |
|------|------|
| `admin-invoices.png` | Admin `/invoices`：2026-10 四筆（rollup／cust1／cust1b／Reseller One） |
| `reseller1-invoices.png` | Reseller `/reseller/invoices`：兩客戶＋Own |
| `cust1-wallet-invoices.png` | Customer `/wallet`：Invoice History 2026-10 $1.40 |

（既有 B2B2C 截圖仍見本目錄其他 `*.png`。）

---

## 7. 缺陷與風險

| ID | 嚴重度 | 說明 | 狀態 |
|----|--------|------|------|
| — | — | 本輪發票 API／權限／金額／CSV／三端 UI 未發現阻擋缺陷 | 無 Open |
| NOTE-1 | 低 | 前端字串多為英文 `t('Invoices')` 鍵（與既有 Phase 1c 相同）；側欄在 zh-HK 下部分仍顯示英文 | 已知／非阻擋 |
| NOTE-2 | 低 | 登入 API 有 IP 速率限制；UI 自動化需換 IP 或注入 session（測試方法問題，非產品缺陷） | 已知 |
| NOTE-3 | 資訊 | 無用量月份仍會寫入零額 admin_rollup（便於 Admin 確認該月已跑過產生） | 設計行為 |

---

## 8. 結論

月結發票功能已合入 `feat/b2b2c-phase1`（SHA `4e5a8f7e2e7127ed3b8c2e99a5bd770452acef08`），Admin／Reseller／Customer 三端 API 與最小 UI 煙霧 **通過**；金額與 seed 日誌一致；角色隔離正確。建議後續可選：排程自動產生、PDF 發票下載、i18n 補齊。

**總評：Pass（可合併／可交付 Phase 1 發票範圍）**
