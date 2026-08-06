# esa-sync

定时用 alidns DoH + ECS 查询上海电信 IP，同步到 Cloudflare DNS 的 `esa.172100.xyz` A 记录。

## 配置

编辑 `config.json`：

```json
{
  "api_token": "你的 Cloudflare API Token (Zone.DNS Edit 权限)",
  "zone_id": "",
  "target_domain": "172100.xyz",
  "target_subdomain": "esa",
  "source_domain": "xin.myflv.cn.a1.initbb.com",
  "probe_base_url": "https://dns.alidns.com/resolve",
  "subnets": [
    { "region": "上海电信", "ecs": "116.228.0.0/24" }
  ],
  "keep_top_n": 4,
  "ttl": 300,
  "interval_sec": 300,
  "http_timeout_sec": 15
}
```

## 本地运行

```bash
go build -o esa-sync .
./esa-sync -once    # 跑一次
./esa-sync          # 循环，每 interval_sec 秒一次
```

## Docker 部署

```bash
# 用 GHCR 上的官方镜像（CI 打 tag 后才有）
docker compose up -d

# 或本地构建
docker compose build && docker compose up -d
```

日志查看：

```bash
docker compose logs -f
```

## 发布镜像（CI）

GitHub Actions workflow 在 `.github/workflows/docker-publish.yml`。
打 tag 自动触发构建并推送到 GHCR：

```bash
git tag v1.0.0
git push origin v1.0.0
```

镜像地址：`ghcr.io/<你的用户名>/<仓库名>:1.0.0`（也会打 `1.0`、`1`、`latest` tag）。
