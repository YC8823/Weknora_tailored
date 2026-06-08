# 1 从github下载项目

weknora 原生框架：https://github.com/Tencent/WeKnora
weknora 改写框架（带图文混合视觉推理）：https://github.com/YC8823/Weknora_tailored/tree/vision-augmented

# 2 安装docker desktop


超时报错：docker desktop 中进入设置 --> docker engine, 修改配置文件

```bash
{
  "registry-mirrors": [
    "https://docker.1ms.run",
    "https://docker.m.daocloud.io"
  ],
...
}
```

# 2.1 构建镜像前的网络配置（国内环境必读）

从源码构建时，`docker compose build` 会下载 Go 依赖和 DuckDB spatial 扩展，均需访问境外域名，国内默认超时。

**已内置 Go 代理**：`docker-compose.yml` 默认使用 `goproxy.cn`，无需额外配置。

**DuckDB 扩展需要宿主机代理**：在 `.env` 中配置：

```bash
HTTP_PROXY=http://host.docker.internal:7890
HTTPS_PROXY=http://host.docker.internal:7890
```

将 `7890` 换成你本机代理的实际端口。确保代理客户端（Clash 等）已开启「允许局域网连接」。

> 如果只是 `docker compose up -d`（使用预构建镜像），无需此配置。

# 3 启动

```bash
cd WeKnora （或weknora_xxx）
cp .env.example .env   # 按需编辑 .env，详见文件内注释
docker compose up -d   # 启动核心服务
```

启动成功后访问 **http://localhost** 即可使用。
服务器访问：**http://10.30.45.2** 

# 4 注册账号


# 5 配置模型

# 5.1 本地部署：下载Ollama 

命令行：
```bash
Ollama pull model-name
```
推理模型：deepseek-r1:14b; deepseek-r1:8b
视觉模型：qwen3-vl:8b； qwen3.5:9b
嵌入模型：bge-m3:567k; qwen-embedding:8b; qwen-embedding:4b
重排序模型：目前只能远程访问

模型自定义参数：
推理模型：my-ds-16k （基于ds-r1-8b，定制为16k长上下文）

创建自定义参数模型：
```bash
ollama create my-deepseek-16k -f "C:\AI\model files customized\modelfile-xx(your customized model name).txt"
```
运行后Ollma创建一个新的模型标签，拥有在modelfile中定义的参数，可以在框架内直接引用