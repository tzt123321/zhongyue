# 音乐播放器 Stream 401 问题 — 前端调试指南

## 🔍 问题概述

后端 stream 服务已支持 `?token=` 查询参数，前端 JS 也已修复，但用户反映音乐仍无法正常播放。本指南帮助用户排查浏览器端问题。

---

## 排查步骤（按优先级排序）

### 步骤 1：确认浏览器缓存（最高可能性）

**问题**：浏览器可能缓存了旧版 JS（未包含 token 修复）

**操作**：
1. 打开网站 `http://tianzhentao.space:8088`
2. 按 `Ctrl+Shift+R`（Windows/Linux）或 `Cmd+Shift+R`（Mac）强制刷新
3. 如果仍不行，打开 DevTools（F12）→ **Application** → **Storage** → **Clear site data** → 点击 "Clear site data" 按钮
4. 刷新页面后重试播放

**验证方法**：
- DevTools → **Network** 面板 → 找到 `index-CfhLJeKm.js` 请求
- 查看 **Size** 列：
  - 如果显示 `(from memory cache)` 或 `(from disk cache)` → 缓存未清理
  - 如果显示实际大小（如 209KB）→ 已下载新版

---

### 步骤 2：确认登录状态

**问题**：如果未登录，`localStorage` 中没有 `token`，stream 请求会 401

**操作**：
1. 在页面上确认是否能看到用户信息（如用户名、头像）
2. 打开 DevTools → **Console**
3. 输入以下命令并按回车：
   ```javascript
   localStorage.getItem("token")
   ```
4. 如果返回 `null` 或空字符串 → **未登录，需要先登录**
5. 如果返回一长串字符（JWT token）→ 已登录，继续下一步

---

### 步骤 3：检查 Service Worker 缓存

**问题**：Service Worker 可能缓存了旧版资源

**操作**：
1. DevTools → **Application** → **Service Workers**
2. 确认是否有 SW 在运行
3. 如果有，点击 **Unregister** 注销
4. 刷新页面后重试

---

### 步骤 4：捕获 Network 面板信息（最关键）

**操作**：
1. 打开 DevTools → **Network** 面板
2. 点击播放按钮
3. 在 Network 面板中过滤 `stream`
4. 找到 `/stream/xxx?token=...` 请求
5. **截图保存**，需要包含以下信息：
   - **Name** 列：请求路径
   - **Status** 列：HTTP 状态码（200/401/其他）
   - **Type** 列：请求类型
   - 点击该请求，查看右侧 **Headers** → **Request URL**（确认是否带 `?token=`）
   - 查看 **Response Headers** → `Content-Type`（应为 `audio/mpeg`）

---

### 步骤 5：检查 Console 错误日志

**操作**：
1. DevTools → **Console**
2. 点击播放按钮
3. 观察是否有红色错误信息
4. 特别关注：
   - `Load error:` 开头的日志（来自 Howler.js 的 onloaderror 回调）
   - `401 Unauthorized` 错误
   - CORS 相关错误
5. **截图保存**

---

### 步骤 6：验证 token 编码

**操作**：
1. Console 中执行：
   ```javascript
   const token = localStorage.getItem("token");
   console.log("Token:", token);
   console.log("Encoded:", encodeURIComponent(token));
   ```
2. 确认 token 中没有换行符或特殊空白字符
3. 确认 URL 中的 token 与 localStorage 中的一致

---

## 📋 需要提供给开发者的信息

请提供以下截图/信息：

1. **Network 面板截图**（包含 `/stream/` 请求）
   - 重点：Status 列、URL 是否带 `?token=`、Content-Type
2. **Console 截图**（包含任何错误日志）
3. **登录状态**：页面上是否能看到用户信息？
4. **Console 输出**：执行 `localStorage.getItem("token")` 的结果

---

## 🚀 快速修复尝试

如果以上排查太复杂，可以尝试以下一键修复：

1. **清除所有缓存**：
   - Chrome: `chrome://settings/clearBrowserData` → 选择 "Cached images and files" → Clear
   - 或 DevTools → Application → Storage → Clear site data

2. **无痕模式测试**：
   - 打开浏览器无痕/隐私模式
   - 访问 `http://tianzhentao.space:8088`
   - 登录后测试播放

3. **硬刷新**：
   - Windows/Linux: `Ctrl+Shift+R`
   - Mac: `Cmd+Shift+R`

---

## 🔧 技术细节（供参考）

- **正确行为**：播放时请求 URL 应为 `/stream/23?token=eyJhbG...`（带 token 参数）
- **错误行为**：请求 URL 为 `/stream/23`（无 token）→ 会返回 401
- **Service Worker**：当前部署了 `sw.js`，可能缓存旧版资源
- **JS 文件**：当前引用 `index-CfhLJeKm.js`（包含 token 修复）

---

## ❓ 如果以上都无效

请提供以下信息：
1. 浏览器类型和版本（如 Chrome 120, Firefox 121）
2. 操作系统（Windows/Mac/Linux）
3. Network 面板中 `/stream/` 请求的完整截图
4. Console 中的完整错误日志
