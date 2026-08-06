package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// recordType 本工具管理的 DNS 记录类型（A 记录）。
const recordType = "A"

// dnsTypeA alidns DoH 响应中 A 记录的类型码。
const dnsTypeA = 1

// ptr 返回指向 bool 值的指针（SDK 字段用）。
func ptr(b bool) *bool { return &b }

// httpClient 返回带超时的共享 HTTP 客户端（probe 与 CF SDK 共用）。
func httpClient(c *Config) *http.Client {
	return &http.Client{Timeout: time.Duration(c.HTTPTimeoutSec) * time.Second}
}

// Subnet config: one telecom region.
type Subnet struct {
	Region string `json:"region"` // 显示名，仅用于日志
	ECS    string `json:"ecs"`    // alidns DoH 用的 edns_client_subnet
}

type Config struct {
	APIToken       string   `json:"api_token"`        // Cloudflare API Token (Zone.DNS Edit 权限)
	ZoneID         string   `json:"zone_id"`          // Cloudflare zone ID，留空则按 target_domain 自动查
	TargetDomain   string   `json:"target_domain"`    // Cloudflare zone 的根域名，如 100172.xyz
	TargetSubDomain string  `json:"target_subdomain"` // 要写入的主机记录/子域，如 esa；空=根域@
	SourceDomain   string   `json:"source_domain"`    // 查询源：xin.myflv.cn.a1.initbb.com
	ProbeBaseURL   string   `json:"probe_base_url"`
	Subnets        []Subnet `json:"subnets"`
	KeepTopN       int      `json:"keep_top_n"`
	TTL            int      `json:"ttl"`
	IntervalSec    int      `json:"interval_sec"`
	HTTPTimeoutSec int      `json:"http_timeout_sec"`
}

func loadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.ProbeBaseURL == "" {
		c.ProbeBaseURL = "https://dns.alidns.com/resolve"
	}
	if c.KeepTopN == 0 {
		c.KeepTopN = 4
	}
	if c.TTL == 0 {
		c.TTL = 300
	}
	if c.IntervalSec == 0 {
		c.IntervalSec = 300
	}
	if c.HTTPTimeoutSec == 0 {
		c.HTTPTimeoutSec = 15
	}
	if c.APIToken == "" || c.TargetDomain == "" || c.SourceDomain == "" {
		return nil, fmt.Errorf("config 必填项缺失: api_token/target_domain/source_domain 不能为空")
	}
	return &c, nil
}

// fqdn 返回要写入的完整主机名。
func (c *Config) fqdn() string {
	if c.TargetSubDomain == "" {
		return c.TargetDomain
	}
	return c.TargetSubDomain + "." + c.TargetDomain
}

// alidnsDoHProbe 用 alidns DoH + ECS 查询 source_domain，返回解析出的电信 IP 列表。
func alidnsDoHProbe(c *Config, subnet Subnet) ([]string, error) {
	u := fmt.Sprintf("%s?name=%s&type=%s&edns_client_subnet=%s",
		c.ProbeBaseURL, c.SourceDomain, recordType, subnet.ECS)

	resp, err := httpClient(c).Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("probe HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed struct {
		Answer []struct {
			Type int    `json:"type"`
			Data string `json:"data"`
		} `json:"Answer"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("probe 响应解析失败: %v (body=%s)", err, truncate(string(body), 300))
	}
	var ips []string
	for _, a := range parsed.Answer {
		if a.Type == dnsTypeA && isIPv4(a.Data) {
			ips = append(ips, a.Data)
		}
	}
	return ips, nil
}

// isIPv4 判断字符串是否为合法 IPv4。
func isIPv4(s string) bool {
	ip := net.ParseIP(strings.TrimSpace(s))
	return ip != nil && ip.To4() != nil
}

// toSet 把 IP 列表转为集合。
func toSet(in []string) map[string]bool {
	s := make(map[string]bool, len(in))
	for _, x := range in {
		s[x] = true
	}
	return s
}

// dedupePreserveOrder 去重并保留顺序。
func dedupePreserveOrder(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range in {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// truncate 截断字符串用于日志。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func main() {
	log.SetFlags(log.LstdFlags) // 日期+时间，docker logs 查看更清晰
	cfgPath := flag.String("config", "config.json", "配置文件路径")
	runOnce := flag.Bool("once", false, "只执行一次同步后退出（默认循环）")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	log.Printf("%s -> %s (间隔=%ds topN=%d)", cfg.SourceDomain, cfg.fqdn(), cfg.IntervalSec, cfg.KeepTopN)

	if *runOnce {
		if err := runOnceSync(cfg); err != nil {
			log.Fatalf("同步失败: %v", err)
		}
		return
	}

	// 循环模式：启动即跑一次，然后定时。
	if err := runOnceSync(cfg); err != nil {
		log.Printf("[错误] 首轮: %v", err)
	}
	ticker := time.NewTicker(time.Duration(cfg.IntervalSec) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if err := runOnceSync(cfg); err != nil {
			log.Printf("[错误] %v", err)
		}
	}
}
