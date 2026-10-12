# B2B2C Phase 1 專業 QA 測試報告

| 項目 | 內容 |
|------|------|
| 產品／分支 | `new-api` / `feat/b2b2c-phase1`（commit `9d3295364` 及先前 phase1 提交） |
| 測試類型 | 手動＋API 自動化＋Playwright UI 截圖 |
| 測試日期 | 2026-10-12（HKT） |
| 測試人員 | Grok Bot QA（executor） |
| 報告路徑 | `/workspace/b2b2c-pro-qa/REPORT.md`（同步 `docs/b2b2c-pro-qa/`） |

---

## 1. 測試範圍

覆蓋 B2B2C Phase 1 最新行為（**公開／OAuth 註冊預設關閉**、**客戶／代理商可線上自助儲值**、**消費仍需代理商成本庫存 dual-leg**）：

1. 環境啟動（SQLite）與角色建立：Admin、Reseller×2、Customer×3；為 Reseller 入帳  
2. 登入／註冊（期望註冊被拒）  
3. Admin：Reseller CRUD／停用凍結  
4. Reseller：客戶、加價、Token、兌換碼、日誌、報表、錢包儲值 UI  
5. Customer：個人資料代理商卡片、錢包儲值 UI（不可被封鎖）  
6. 消費：無代理商庫存 → 拒絕；有庫存 → 可過 billing  
7. 跨代理商隔離（403／404）  
8. 停用代理商後客戶 Token 消費凍結  
9. OAuth／OIDC 新用戶註冊關閉（既有登入不受影響）  

**不在範圍**：真實支付閘道端到端（本環境未配置 Epay／Stripe 等金鑰）；上游真實模型推理品質。

---

## 2. 測試環境

| 項目 | 值 |
|------|-----|
| OS | Linux（box） |
| Backend | 自建 binary `/tmp/new-api-b2b2c`（含最新 `web/dist` embed） |
| Frontend | 內嵌 `web/dist`（測試前已 `bun run build` 重建，避免舊文案） |
| DB | SQLite `/tmp/b2b2c-pro-qa.db` |
| URL | `http://127.0.0.1:3310` |
| Redis | 未啟用 |
| 支付閘道 | 未配置（`enable_online_topup=false` 等屬環境配置，非 B2B2C 封鎖） |
| 測試帳號 | `admin` / `reseller1`・`reseller2` / `cust1`・`cust1b`・`cust2` |

---

## 3. 用例表

| 編號 | 步驟 | 期望 | 實際 | 結果 |
|------|------|------|------|------|
| TC01 | 全新 SQLite；`POST /api/setup` 建立 root Admin | 初始化成功，Admin 可登入 | `success=true`，建立 admin（role=100） | **Pass** |
| TC02 | `POST /api/user/login` admin | 取得 access_token／session | HTTP 200，JWT＋refresh cookie | **Pass** |
| TC03 | `GET /api/status`；`POST /api/user/register` | `register_enabled`／`password_register_enabled`=false；註冊失敗 | 旗標均 false；訊息 *New user registration has been disabled by administrator* | **Pass** |
| TC04 | Admin `POST /api/admin/resellers` ×2（groups=`default`） | 建立 reseller1／reseller2 | id=2、3 成功 | **Pass** |
| TC05 | Admin `POST …/credit`（quota／money_usd） | Reseller 庫存增加 | reseller1 quota 入帳成功（最終約 5e6） | **Pass** |
| TC06 | Reseller1 登入 | 登入成功，可進 reseller API | HTTP 200 | **Pass** |
| TC07 | Reseller1 建 cust1／cust1b；Reseller2 建 cust2 | 客戶建立，`reseller_id` 正確 | ids 4／5／6 | **Pass** |
| TC08 | Reseller1 存取 Reseller2 客戶；Reseller 呼叫 `/api/admin/resellers` | 客戶資源 404；Admin API 拒絕 | `customer not found`（404）；Admin 未授權 | **Pass** |
| TC09 | cust1 `GET /api/user/self`；UI `/profile` | 有 reseller 卡片（顯示名稱）；不洩漏成本／markup | API `online_topup_enabled=true`；UI「供應商: Brand One」＋正確自助儲值說明 | **Pass** |
| TC10 | cust1 `GET /api/user/topup/info`；試 `aff_transfer`／subscription pay；開 `/wallet` | 儲值 info 可讀且**未被** BlockResellerCustomer；aff／訂閱仍封鎖；錢包頁可開 | topup info 200；aff／subscription 403；錢包頁顯示「增加資金」區（閘道未配置屬環境） | **Pass** |
| TC11 | reseller quota=0；客戶有額度＋token＋渠道；`POST /v1/chat/completions` | 消費被拒（庫存不足） | HTTP 403，`Insufficient user quota, remaining quota: -`（對應 `resellerInsufficientError`，刻意不向客戶洩露代理商庫存細節） | **Pass** |
| TC12 | Admin 為 reseller1 入帳後再消費 | 不再因庫存拒絕；billing 預扣成功 | HTTP 200（經 mock 渠道至 httpbin）；reseller／customer quota 各扣 1 | **Pass** |
| TC13 | Admin `PUT` reseller status=disabled；客戶 token 再消費；再啟用 | 凍結（403）；啟用後恢復 | 403 *User has been banned*（TokenAuth 檢查代理商狀態）；再啟用成功 | **Pass** |
| TC14 | status `register_enabled=false`；未配置 OAuth 端點；程式碼 OAuth 新用戶分支 | OAuth／OIDC **新註冊**關閉；既有登入／綁定不受影響 | 旗標 false；`oauth.go` 有 `OAuthRegistrationDisabledError`；本環境無 OAuth client 故無法做真實現流，以 API＋碼徑驗證 | **Pass**（有限） |
| TC15 | Reseller：self／customers／tokens／model_markups／redemptions／logs／reports；UI 各頁 | API 與 UI 可用 | 全部 API 200；UI console／customers／markups／redemptions／logs／reports／wallet 正常 | **Pass** |
| TC16 | Admin 列表／詳情／新增抽屜；UI `/resellers` | CRUD UI 可用 | 列表含兩代理商；詳情／Add Reseller 抽屜截圖 OK | **Pass** |
| TC17 | 未登入開啟 `/sign-in` | 無註冊入口連結（因 register 關閉） | `a[href*="sign-up"]` 數量 = 0 | **Pass** |
| TC18 | 直接開啟 `/sign-up`（深鏈） | 應導向登入或顯示「已關閉註冊」，不應仍可填密碼註冊表單 | **仍顯示**「建立一個用戶」用戶名／密碼／確認密碼表單；API 會拒，但 UI 未閘道 | **Fail** |
| TC19 | Customer 開 `/reseller`、`/resellers` | 前端 403 禁止頁 | 導向 `/403`「禁止存取」 | **Pass** |

### 結果匯總

| Pass | Fail | Blocked |
|------|------|---------|
| 18 | 1 | 0 |

---

## 4. 截圖索引

路徑：`/workspace/b2b2c-pro-qa/`（及 `docs/b2b2c-pro-qa/`）

| 檔名 | 說明 |
|------|------|
| `public-login.png` | 登入頁；無註冊連結 |
| `public-register-page.png` | **缺陷**：註冊關閉時深鏈仍顯示註冊表單 |
| `admin-resellers-list.png` | Admin 代理商列表 |
| `admin-add-reseller-drawer.png` | 新增代理商抽屜 |
| `admin-reseller-detail.png` | 代理商詳情 |
| `admin-users.png` | 用戶管理 |
| `reseller1-console.png` | Reseller 總覽 |
| `reseller1-customers.png` | 客戶列表 |
| `reseller1-customer-detail.png` | 客戶詳情／tokens |
| `reseller1-markups.png` | 模型加價 |
| `reseller1-redemptions.png` | 兌換碼 |
| `reseller1-logs.png` | 使用日誌 |
| `reseller1-reports.png` | 毛利報表 |
| `reseller1-wallet-topup.png` | Reseller 錢包（可進儲值 UI） |
| `cust1-profile-reseller-card.png` | 客戶個人資料＋Reseller Contact 卡片 |
| `cust1-overview.png` | 概覽「Served by reseller… top up online」 |
| `cust1-wallet-topup.png` | 客戶錢包／增加資金（頁面未封鎖） |
| `cust1-forbidden-reseller.png` | 客戶進 Reseller 區 → 403 |
| `cust1-forbidden-admin.png` | 客戶進 Admin Resellers → 403 |

---

## 5. 缺陷列表

| ID | 嚴重度 | 標題 | 描述 | 建議 |
|----|--------|------|------|------|
| BUG-01 | **Major** | `/sign-up` 深鏈在註冊關閉時仍渲染密碼註冊表單 | `register_enabled`／`password_register_enabled` 預設 false；登入頁已正確隱藏註冊連結，但 `SignUp`／`SignUpForm` 未在 `password_register_enabled===false` 時隱藏表單或導向 `/sign-in`。API 會拒絕，屬 UI／產品一致性缺口，亦增加不必要的註冊嘗試與 rate-limit 壓力。 | 在 `sign-up` route 或 `SignUpForm` 讀取 status：若密碼註冊關閉則 redirect 至登入並 toast；OAuth 區塊僅在 `oauth_register_enabled` 時顯示。 |
| NOTE-01 | Info | 無庫存錯誤文案對客戶顯示為「用戶額度不足」 | `resellerInsufficientError` 使用 `MsgQuotaUserInsufficient`＋`Remaining: "-"`，屬刻意不洩露代理商庫存。行為正確；若產品希望代理商後台有更明確日誌即可（伺服器已有 log）。 | 可選：Reseller 控制台／日誌標註「成本庫存不足」。 |
| NOTE-02 | Info | 測試環境未配置支付閘道 | 錢包 UI 顯示「尚未啟用線上儲值」來自 `enable_online_topup=false`（無 Epay 等），**不是** B2B2C `BlockResellerCustomer`。客戶／代理商均可開啟錢包頁；`reseller.online_topup_enabled=true`。 | 配置閘道後再做支付 E2E。 |

未發現需緊急熱修的 **Critical** 計費／隔離／凍結缺陷；TC11–TC13、TC08 核心安全路徑通過。

---

## 6. 總結

B2B2C Phase 1 在 SQLite 環境下，**角色建模、庫存 dual-leg 計費、跨租戶隔離、停用凍結、註冊關閉（API）、自助儲值解禁（API＋錢包頁）** 均符合預期。消費無庫存拒絕、有庫存可結算已用真實 relay 路徑驗證。

主要缺口為 **前端註冊頁深鏈未跟隨 `RegisterEnabled` 預設關閉**（BUG-01，Major）。其餘為資訊性說明。建議合併前修復 BUG-01；無需為 Critical blocker 阻擋 phase1 其餘功能。

**建議給用戶的重點 Failures：**
1. **BUG-01（Major）**：`/sign-up` 在註冊關閉時仍顯示註冊表單 — 見 `public-register-page.png`  
2. 其餘關鍵路徑 **Pass**（含無庫存拒絕、有庫存成功、凍結、隔離、錢包未封鎖、代理商卡片）

---

*產生時間：2026-10-12 19:35 HKT*
