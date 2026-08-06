package main

import (
	"log"
	"strings"
)

// runOnceSync 执行一次完整的同步：
//   1. 对每个子网用 alidns DoH + ECS 查询 source_domain 得到上海电信 IP
//   2. 把 IP 列表同步到 Cloudflare zone 的 esa 子域（先删全部再全量重建）
func runOnceSync(c *Config) error {
	for _, sub := range c.Subnets {
		ips, err := alidnsDoHProbe(c, sub)
		if err != nil {
			log.Printf("[%s] 查询失败: %v", sub.Region, err)
			continue
		}
		ips = dedupePreserveOrder(ips)
		if len(ips) > c.KeepTopN {
			ips = ips[:c.KeepTopN]
		}
		if len(ips) == 0 {
			log.Printf("[%s] 无结果，跳过", sub.Region)
			continue
		}
		log.Printf("[%s] %s", sub.Region, strings.Join(ips, " , "))
		if err := syncCFRecords(c, sub, ips); err != nil {
			log.Printf("[%s] 同步失败: %v", sub.Region, err)
		}
	}
	return nil
}
