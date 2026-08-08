package main

import (
	"log"
	"strings"

	dnsv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dns/v2"

	"github.com/huaweicloud/huaweicloud-sdk-go-v3/core/auth/basic"
	hcConfig "github.com/huaweicloud/huaweicloud-sdk-go-v3/core/config"
	dnsregion "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dns/v2/region"
)

// clientFor 构建华为云 DNS SDK 客户端（进程内仅构建一次）。
func clientFor(c *Config) (*dnsv2.DnsClient, error) {
	auth, err := basic.NewCredentialsBuilder().
		WithAk(c.AccessKey).
		WithSk(c.SecretKey).
		SafeBuild()
	if err != nil {
		return nil, err
	}

	ep, err := dnsregion.SafeValueOf(c.Region) // 区域级 DNS 端点；未知区域返回错误而非 panic
	if err != nil {
		return nil, err
	}

	hcClient, err := dnsv2.DnsClientBuilder().
		WithRegion(ep).
		WithCredential(auth).
		WithHttpConfig(hcConfig.DefaultHttpConfig().
			WithTimeout(timeoutFor(c))).
		SafeBuild()
	if err != nil {
		return nil, err
	}
	return dnsv2.NewDnsClient(hcClient), nil
}

// runOnceSync 执行一次完整的同步：
//  1. 对每个子网用 alidns DoH + ECS 查询 source_domain 得到上海电信 IP
//  2. 把 IP 列表同步到华为云 zone 的 esa 子域 A 记录
//
// 华为云客户端整轮只构建一次（AK/SK 与配置仅依赖 Config，进程内不变），
// 避免每子网/每轮重复初始化（含 IAM project 解析）。
func runOnceSync(c *Config, client *dnsv2.DnsClient) error {
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
		if err := syncHuaweiRecords(c, client, ips); err != nil {
			log.Printf("[%s] 同步失败: %v", sub.Region, err)
		}
	}
	return nil
}
