package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudflare/cloudflare-go"
)

// newCFClient 用 API Token 构造 Cloudflare 官方 SDK 客户端。
func newCFClient(c *Config) (*cloudflare.API, error) {
	api, err := cloudflare.NewWithAPIToken(c.APIToken,
		cloudflare.HTTPClient(httpClient(c)),
	)
	if err != nil {
		return nil, err
	}
	return api, nil
}

// zoneContainer 返回 zone 级资源容器；zone_id 为空时按根域名自动查一次并缓存。
func zoneContainer(c *Config, api *cloudflare.API) (*cloudflare.ResourceContainer, error) {
	if c.ZoneID != "" {
		return cloudflare.ZoneIdentifier(c.ZoneID), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.HTTPTimeoutSec)*time.Second)
	defer cancel()
	zones, err := api.ListZones(ctx, c.TargetDomain)
	if err != nil {
		return nil, err
	}
	if len(zones) == 0 {
		return nil, fmt.Errorf("找不到 zone %s（请确认域名已添加到 Cloudflare）", c.TargetDomain)
	}
	c.ZoneID = zones[0].ID // 缓存，避免每轮重复查询
	return cloudflare.ZoneIdentifier(zones[0].ID), nil
}

// syncCFRecords 把 zone 下 esa 子域的所有 A 记录同步为目标 IP 集合。
// 策略：先删除 zone 下该子域全部现有 A 记录，再按 wantIPs 全量重建
// （依赖 DNS TTL 缓存兜底，避免中间空窗）。
func syncCFRecords(c *Config, sub Subnet, wantIPs []string) error {
	api, err := newCFClient(c)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.HTTPTimeoutSec)*time.Second)
	defer cancel()

	rc, err := zoneContainer(c, api)
	if err != nil {
		return err
	}

	recs, _, err := api.ListDNSRecords(ctx, rc, cloudflare.ListDNSRecordsParams{
		Type: "A",
		Name: c.fqdn(),
	})
	if err != nil {
		return err
	}

	// 1) 先删全部现有匹配记录。
	var deleted []string
	for _, r := range recs {
		if err := api.DeleteDNSRecord(ctx, rc, r.ID); err != nil {
			log.Printf("删除 %s 失败: %v", r.Content, err)
		} else {
			deleted = append(deleted, r.Content)
		}
	}
	if len(deleted) > 0 {
		log.Printf("删除 %s", strings.Join(deleted, " , "))
	}

	// 2) 再全量创建。
	comment := "esa-sync: " + sub.Region
	var created []string
	for _, ip := range wantIPs {
		_, err := api.CreateDNSRecord(ctx, rc, cloudflare.CreateDNSRecordParams{
			Type:    "A",
			Name:    c.fqdn(),
			Content: ip,
			TTL:     c.TTL,
			Proxied: ptr(false),
			Comment: comment,
		})
		if err != nil {
			log.Printf("新增 %s 失败: %v", ip, err)
		} else {
			created = append(created, ip)
		}
	}
	if len(created) > 0 {
		log.Printf("新增 %s", strings.Join(created, " , "))
	}
	return nil
}
