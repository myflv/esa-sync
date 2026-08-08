package main

import (
	"fmt"
	"log"
	"strings"

	dnsv2 "github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dns/v2"
	"github.com/huaweicloud/huaweicloud-sdk-go-v3/services/dns/v2/model"
)

// recordLimit 查询 zone / 记录集时的单页条数上限。
const recordLimit int32 = 100

// huaweiZoneID 返回 zone 的 ID；查到后缓存到 config 避免每轮重复查询。
func huaweiZoneID(c *Config, client *dnsv2.DnsClient) (string, error) {
	if c.ZoneID != "" {
		return c.ZoneID, nil
	}
	name := c.TargetDomain
	limit := recordLimit
	req := &model.ListPublicZonesRequest{Name: &name, Limit: &limit}
	resp, err := client.ListPublicZones(req)
	if err != nil {
		return "", err
	}
	if resp.Zones == nil || len(*resp.Zones) == 0 {
		return "", fmt.Errorf("找不到公网域名 %s（请确认已在华为云云解析添加）", c.TargetDomain)
	}
	id := *(*resp.Zones)[0].Id
	c.ZoneID = id // 缓存
	return id, nil
}

// recordTypePtr 返回 A 类型的指针（SDK 字段用）。
func recordTypePtr() *string {
	t := recordType
	return &t
}

// listRecordsetsOfName 查出 zone 下 fqdn 的全部 A 记录集。
func listRecordsetsOfName(client *dnsv2.DnsClient, zoneID, fqdn string) ([]model.ListRecordSets, error) {
	name := fqdn + "."
	limit := recordLimit
	req := &model.ListRecordSetsByZoneRequest{
		ZoneId: zoneID,
		Name:   &name,
		Type:   recordTypePtr(),
		Limit:  &limit,
	}
	resp, err := client.ListRecordSetsByZone(req)
	if err != nil {
		return nil, err
	}
	if resp.Recordsets == nil {
		return nil, nil
	}
	return *resp.Recordsets, nil
}

// syncHuaweiRecords 把 zone 下目标子域的 A 记录同步为目标 IP 集合。
// 逻辑：先删除该子域现有的全部 A 记录集，再按 wantIPs 全量新建。
func syncHuaweiRecords(c *Config, client *dnsv2.DnsClient, wantIPs []string) error {
	fqdn := c.fqdn()

	zoneID, err := huaweiZoneID(c, client)
	if err != nil {
		return err
	}

	// 1) 先删除现有全部匹配记录集。
	recordsets, err := listRecordsetsOfName(client, zoneID, fqdn)
	if err != nil {
		return err
	}
	for _, rs := range recordsets {
		if rs.Id == nil {
			continue
		}
		req := &model.DeleteRecordSetRequest{
			ZoneId:      zoneID,
			RecordsetId: *rs.Id,
		}
		if _, err := client.DeleteRecordSet(req); err != nil {
			log.Printf("删除记录失败: %v", err)
		} else {
			log.Printf("删除记录: %s", strings.Join(*rs.Records, " , "))
		}
	}

	// 2) 再全量新建（一个记录集放多个 IP 的记录数组）。
	ttl := int32(c.TTL)
	body := &model.CreateRecordSetRequestBody{
		Name:    fqdn + ".",
		Type:    recordType,
		Ttl:     &ttl,
		Records: wantIPs,
	}
	req := &model.CreateRecordSetRequest{ZoneId: zoneID, Body: body}
	if _, err := client.CreateRecordSet(req); err != nil {
		return err
	}
	log.Printf("新增记录: %s", strings.Join(wantIPs, " , "))
	return nil
}