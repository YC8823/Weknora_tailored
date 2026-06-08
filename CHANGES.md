# WeKnora 改动记录

本文档记录基于官方 WeKnora 的本地定制改动，按时间倒序排列。
分支：`vision-augmented`（fork 自官方 `main`）

---

## [未发布] 推理期视觉增强扩展

**日期**：2026-06-04
**文件**：`internal/application/service/chat_pipeline/vision_augment.go`
**类型**：功能增强

### 背景

`PluginVisionAugment` 负责在对话命中知识库分块后，把图片注入给多模态 LLM。
改动前，该插件只处理 **相对路径** 引用（如 `figures/xxx.png`，MD 文件专用），
对 **`local://` 绝对路径**（PPT/base64-MD 摄入后的格式）显式跳过，
导致 PPT 图片在推理期无法作为视觉 token 发送给 LLM。

### 改动内容

**① 收集阶段拆分**：原来把 `local://` 和 `http://`/`data:` 统一 `continue` 丢弃；
现在 `local://` 单独收集到 `directLocalURLs` 切片，相对路径仍进 `knowledgeRefs` map。

**② 提前返回条件**：从 `len(knowledgeRefs) == 0` 改为
`len(knowledgeRefs) == 0 && len(directLocalURLs) == 0`，
避免仅有 PPT 图片时直接跳过。

**③ 知识库查询按需执行**：数据库 batch-fetch（用于相对路径转绝对路径）
包裹在 `if len(knowledgeRefs) > 0` 中，纯 PPT 场景不发无效查询。

**④ 直接追加**：新增 `directLocalURLs` 循环，将已有 `local://` URL 直接写入
`chatManage.Images`，与相对路径处理路径对称。

### 效果

| 格式 | 改动前推理期表现 | 改动后推理期表现 |
|---|---|---|
| MD（相对路径） | 图片发给 LLM ✓ | 不变 |
| PPT | 仅 OCR/Caption 文字 | 图片发给 LLM ✓ |
| MD（base64 内嵌） | 仅 OCR/Caption 文字 | 图片发给 LLM ✓ |

### 验证方式

```powershell
# 重建 app 镜像并热重启（仅 app 服务，不影响数据库/其他服务）
docker compose build app
docker compose up -d app

# 观察日志（PowerShell）
docker compose logs app | Select-String "VisionAugment"
# 命中含图片的 PPT 分块时应出现：
# VisionAugment images_augmented  added=N total_images=N
```

---

## [2026-06-04] 修复 Docker 构建（国内网络 Go 包 + DuckDB 扩展下载超时）

**文件**：`docker-compose.yml`、`.env`
**类型**：构建修复

从远程仓库拉取最新代码后重新构建，出现两处网络超时：

1. **Go 依赖下载超时**：`Dockerfile.app` 中声明了 `GOPROXY_ARG` build arg，但 `docker-compose.yml` 未传入，导致 Go 使用默认 `proxy.golang.org`，国内超时。
   - 修复：在 `docker-compose.yml` 的 app build args 加 `GOPROXY_ARG=${GOPROXY_ARG:-https://goproxy.cn,direct}`

2. **DuckDB spatial 扩展下载超时**：`cmd/download/duckdb/duckdb.go` 执行 `INSTALL spatial` 时从 `extensions.duckdb.org` 下载，国内无法访问。
   - 修复：在 `docker-compose.yml` 的 app build args 加 `HTTP_PROXY` / `HTTPS_PROXY`，透传宿主机代理给 Docker BuildKit；在 `.env` 中配置 `HTTP_PROXY=http://host.docker.internal:7890`

**前提**：宿主机代理客户端（如 Clash）需开启「允许局域网连接」。

---

## [2026-06-04] 摄入期支持 PPT/MD base64 图片入库

**提交**：`caefdfb`
**分支**：`vision-augmented`
**类型**：功能增强

### 背景

PPT 图片经 MarkItDown 解析后以 base64 Data URI 内嵌于 markdown，
官方流程中 `image_resolver.go` 的 `ResolveDataURIImages` 会将其解码并存入 FileService，
替换为 `local://` 稳定 URL，写入 chunk 的 `ImageInfo` 字段。

MD 文件若同样使用 base64 内嵌图片（`![alt](data:image/png;base64,...)`），
`SimpleFormatReader` 将原始内容直接作为 `MarkdownContent` 返回，
`ResolveDataURIImages` 同样会自动处理——**无需额外代码**，与 PPT 走相同管道。

### 关键结论

- `ResolveAndStore` 无论 `ImageRefs` 是否为空，都会先调用 `ResolveDataURIImages` 扫描内容里的 base64
- MD 使用 base64 内嵌图片即可复用 PPT 的完整摄入链路（存储 → chunk ImageInfo → 索引期 VLM OCR/Caption）

---

## [2026-06-03] 分块预览窗口支持图片显示

**提交**：`9d40b98`
**文件**：前端分块内容弹窗组件
**类型**：功能增强

前端 chunk 内容弹窗新增图片渲染支持，可预览分块中的 `local://` 图片引用。

---

## [2026-06-03] 修复 MD 文件相对路径图片在文档内容渲染器中的显示

**提交**：`c6e4350`
**文件**：前端 doc-content 渲染器
**类型**：Bug 修复

修复 `figures/xxx.png` 相对路径在文档内容渲染器中无法加载的问题，
改为根据知识条目的 `file_path` 动态拼接 `local://` 绝对路径。

---

## [2026-06-02] 新增 PluginVisionAugment（图文混合 RAG 推理）

**提交**：`32abd36`
**文件**：`internal/application/service/chat_pipeline/vision_augment.go`（新增）
**类型**：新功能

引入 `PluginVisionAugment` 插件，实现 MD 文件的推理期视觉增强：
- 从命中分块的内容中提取相对路径图片引用
- 根据知识条目的 `FilePath` 拼接 `local://` 绝对 URL
- 注入 `chatManage.Images`，由下游 `resolveImageURLForLLM` 转为 base64 发给视觉 LLM

---

## [2026-06-02] 修复 Docker 构建（国内网络环境）

**提交**：`0efcb7e` / `b64cb07` / `08e1964` / `ccd2e5c` / `828bdba`
**文件**：`docker/Dockerfile.app`
**类型**：构建修复

- 最终阶段改用官方预构建镜像作为 base，只替换编译好的 Go 二进制，绕过国内 apt 源问题
- 清华镜像源作为 apt fallback
- Python 包安装改用 `pip install uv`（替换 curl 安装方式）

---

## 改动原则

- 尽量最小化对官方代码的侵入，优先走现有管道的扩展点
- Go 后端改动只需重建 `app` 服务：`docker compose build app && docker compose up -d app`
- Python docreader 改动需重建 `docreader` 服务
- 前端改动需重建 `frontend` 服务
