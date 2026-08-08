# esa-sync

定时用 alidns DoH + ECS 查询上海电信 IP，同步到**华为云云解析 DNS** 的 A 记录（先删后增）。

## 前置：创建华为云 AK/SK 并授权

1. 在[华为云我的凭证 → 访问密钥](https://console.huaweicloud.com/iam/) 创建并下载 AK/SK 密钥对（妥善保存 SK，之后无法再查看）。
2. 给该密钥授予云解析 DNS 权限，自定义策略需要：
   - `dns:zone:list`
   - `dns:recordset:list`
   - `dns:recordset:create`
   - `dns:recordset:delete`

## 配置

复制模板并填写真实值（`config.example.json` 或仓库里的 `config.json`）：

```json
{
  "access_key": "你的华为云 Access Key (AK)",
  "secret_key": "你的华为云 Secret Access Key (SK)",
  "region": "cn-east-3",
  "target_domain": "你的华为云公网域名",
  "target_subdomain": "你的子域名",
  "source_domain": "esa的cname地址",
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

字段说明：

| 字段 | 说明 |
|---|---|
| `access_key` / `secret_key` | 华为云 AK/SK（不要提交到仓库） |
| `region` | 华为云区域，如 `cn-east-3` / `cn-north-4`（DNS 按区域鉴权） |
| `target_domain` | 华为云公网域名的根域名，如 `cname.100172.xyz` |
| `target_subdomain` | 要写入的子域，如 `esa`；空 = 根域 |
| `source_domain` | alidns DoH 查询的源域名/CNAME，如 `dev.myflv.cn.a1.initbb.com` |
| `subnets` | 探测子网列表，`ecs` 为 `edns_client_subnet` |

`zone_id` 无需配置：按 `target_domain` 自动查询并缓存。

## 同步逻辑

对每个子网：
1. 用 alidns DoH + ECS 查询 `source_domain`，得到电信 IP 列表（去重、保留前 `keep_top_n` 个）。
2. **先删除** `target_subdomain.target_domain` 下现有的全部 A 记录集。
3. **再全量新建**一条 A 记录集，多 IP 放入 records 数组。

日志示例：
```
[上海电信] 101.227.20.91 , 101.227.20.92 , 101.227.20.93 , 101.227.20.94
删除记录: 101.227.20.159 , 101.227.20.94 , 101.227.20.142 , 101.227.20.93
新增记录: 101.227.20.91 , 101.227.20.92 , 101.227.20.93 , 101.227.20.94
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
git tag v2.0.0
git push origin v2.0.0
```

镜像地址：`ghcr.io/<你的用户名>/<仓库名>:2.0.0`（也会打 `2.0`、`2`、`latest` tag）。
