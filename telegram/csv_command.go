package telegram

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"DomainC/config"
)

func (h *CommandHandler) handleCSVCommand(args []string) {
	// 1) 用户只输入 /csv：提示可选账号
	if len(args) < 1 {
		h.sendText(h.csvPromptText())
		return
	}

	selector := strings.TrimSpace(args[0])
	if selector == "" {
		h.sendText(h.csvPromptText())
		return
	}

	// 2) 选择账号
	var targets []config.CF
	if strings.EqualFold(selector, "all") {
		targets = append(targets, h.Accounts...)
		if len(targets) == 0 {
			h.sendText("未配置可用的 Cloudflare 账号，无法导出。")
			return
		}
	} else {
		acc := h.getAccountByLabel(selector)
		if acc == nil {
			h.sendText(fmt.Sprintf("未找到账号 %s。\n\n%s", selector, h.csvPromptText()))
			return
		}
		targets = []config.CF{*acc}
	}

	// 3) 拉取数据并生成 CSV
	ctx := context.Background()
	csvBytes, filename, err := h.buildDNSExportCSV(ctx, targets)
	if err != nil {
		h.sendText(fmt.Sprintf("导出失败: %v", err))
		return
	}

	// 4) 写入临时文件并发送回群
	tmpFile, err := os.CreateTemp("", "dns-export-*.csv")
	if err != nil {
		h.sendText(fmt.Sprintf("创建临时文件失败: %v", err))
		return
	}
	tmpPath := tmpFile.Name()

	// 用完即删（如果你希望保留，去掉 os.Remove 这一行）
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmpFile.Write(csvBytes); err != nil {
		h.sendText(fmt.Sprintf("写入临时文件失败: %v", err))
		return
	}
	_ = tmpFile.Sync()

	finalPath := filepath.Join(os.TempDir(), filename)
	_ = os.Rename(tmpPath, finalPath)
	tmpPath = finalPath

	if err := h.Sender.SendDocumentPath(context.Background(), tmpPath, "📦 Cloudflare DNS 导出"); err != nil {
		h.sendText(fmt.Sprintf("发送导出文件失败: %v", err))
		return
	}

	h.sendText(fmt.Sprintf("✅ 导出完成：%s", filename))

}

// 提示文本：可导出的账号 + 示例
func (h *CommandHandler) csvPromptText() string {
	if len(h.Accounts) == 0 {
		return "未配置可用的 Cloudflare 账号，无法导出。"
	}

	var sb strings.Builder
	sb.WriteString("您想导出哪个账号？\n目前可以导出的账号：\n")
	for _, a := range h.Accounts {
		if strings.TrimSpace(a.Label) == "" {
			continue
		}
		sb.WriteString("- " + a.Label + "\n")
	}
	sb.WriteString("- all\n\n请输入：\n/csv all\n或者：\n/csv 账号标签")
	return sb.String()
}

// 按 Label 查账号（忽略大小写）
func (h *CommandHandler) getAccountByLabel(label string) *config.CF {
	for i := range h.Accounts {
		if strings.EqualFold(strings.TrimSpace(h.Accounts[i].Label), strings.TrimSpace(label)) {
			return &h.Accounts[i]
		}
	}
	return nil
}

func (h *CommandHandler) buildDNSExportCSV(ctx context.Context, accounts []config.CF) ([]byte, string, error) {
	// 文件名：dns-export-YYYYMMDD-HHMMSS.csv
	filename := fmt.Sprintf("dns-export-%s.csv", time.Now().Format("20060102-150405"))

	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)
	w.UseCRLF = false

	// Header
	if err := w.Write([]string{
		"所属账户",
		"主域名",
		"子域名",
		"解析类型",
		"解析地址",
		"是否代理",
		"Zone状态",
		"是否暂停",
	}); err != nil {
		return nil, "", err
	}

	for _, acc := range accounts {
		zones, err := h.CFClient.ListZones(ctx, acc)
		if err != nil {
			return nil, "", fmt.Errorf("列出账号 %s 的域名失败: %w", acc.Label, err)
		}

		for _, z := range zones {
			zonePaused := "否"
			if z.Paused {
				zonePaused = "是"
			}

			records, err := h.CFClient.ListDNSRecords(ctx, acc, z.Name)
			if err != nil {
				return nil, "", fmt.Errorf("获取 %s(%s) DNS 失败: %w", z.Name, acc.Label, err)
			}

			// 没有记录也写一行（保留 zone 维度信息）
			if len(records) == 0 {
				_ = w.Write([]string{
					acc.Label,
					z.Name,
					"",
					"",
					"",
					"",
					z.Status,
					zonePaused,
				})
				continue
			}

			for _, r := range records {
				proxied := "否"
				if r.Proxied != nil && *r.Proxied {
					proxied = "是"
				}

				subDomain := r.Name
				if subDomain == "@" || subDomain == z.Name {
					subDomain = z.Name
				}

				if err := w.Write([]string{
					acc.Label,  // 所属账户
					z.Name,     // 主域名
					subDomain,  // 子域名（完整 FQDN）
					r.Type,     // 解析类型
					r.Content,  // 解析地址
					proxied,    // 是否代理
					z.Status,   // Zone状态
					zonePaused, // 是否暂停
				}); err != nil {
					return nil, "", err
				}
			}
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", err
	}

	return buf.Bytes(), filename, nil
}
