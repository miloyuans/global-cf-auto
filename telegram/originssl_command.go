package telegram

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"DomainC/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/acm"
)

func (h *CommandHandler) handleOriginSSLCommand(args []string) {
	if len(args) < 3 {
		h.sendText(h.originSSLPromptText())
		return
	}

	domain := strings.TrimSpace(args[0])
	mode := strings.TrimSpace(args[1])

	if domain == "" {
		h.sendText(h.originSSLPromptText())
		return
	}
	awsAlias := strings.TrimSpace(args[2])
	if awsAlias == "" {
		h.sendText(h.originSSLPromptText())
		return
	}
	// 必须第二个参数是 "*"
	if mode != "*" {
		h.sendText("参数错误：必须使用 *\n\n" + h.originSSLPromptText())
		return
	}

	// 自动定位账号：domain 必须是某个账号下的 zone
	ctx := context.Background()
	acc, err := h.findAccountByDomain(ctx, domain)
	if err != nil {
		h.sendText(fmt.Sprintf("无法定位域名所属账号：%v\n\n%s", err, h.originSSLPromptText()))
		return
	}

	// 固定生成：裸域 + 通配
	hostnames := []string{domain, "*." + domain}

	// 创建 15 年 Origin CA 证书（你已在 client 里实现）
	cert, err := h.CFClient.CreateOriginCertificate(ctx, *acc, hostnames)
	if err != nil {
		h.sendText(fmt.Sprintf("创建源站证书失败: %v", err))
		return
	}

	target, ok := config.Cfg.AWSTargets[awsAlias]
	if !ok {
		h.sendText(fmt.Sprintf("未知 AWS 目标别名：%s\n\n%s", awsAlias, h.originSSLPromptText()))
		return
	}

	acmArn, err := importToACM(ctx, target, cert.CertificatePEM, cert.PrivateKeyPEM)
	if err != nil {
		h.sendText(fmt.Sprintf("证书已生成，但导入 ACM 失败（%s/%s）: %v", awsAlias, target.Region, err))
		return
	}
	h.sendText(fmt.Sprintf("CF源站证书生成并已导入 ACM：%s\nTarget: %s (%s)\nARN: %s\n账号：%s",
		domain, awsAlias, target.Region, acmArn, acc.Label))

	// 输出内容（证书 + 私钥 + CSR）
	var out bytes.Buffer
	out.WriteString("### Cloudflare Origin CA Certificate\n")
	out.WriteString(fmt.Sprintf("Account: %s\n", acc.Label))
	out.WriteString(fmt.Sprintf("Zone: %s\n", domain))
	out.WriteString(fmt.Sprintf("Hostnames: %s\n", strings.Join(hostnames, ", ")))
	if cert.ID != "" {
		out.WriteString(fmt.Sprintf("CertID: %s\n", cert.ID))
	}
	if !cert.ExpiresOn.IsZero() {
		out.WriteString(fmt.Sprintf("ExpiresOn: %s\n", cert.ExpiresOn.Format(time.RFC3339)))
	}
	out.WriteString("\n")

	out.WriteString("-----BEGIN CERTIFICATE-----\n")
	out.WriteString(strings.TrimSpace(cert.CertificatePEM))
	out.WriteString("\n-----END CERTIFICATE-----\n\n")

	out.WriteString("-----BEGIN PRIVATE KEY-----\n")
	out.WriteString(strings.TrimSpace(cert.PrivateKeyPEM))
	out.WriteString("\n-----END PRIVATE KEY-----\n\n")

	out.WriteString("-----BEGIN CERTIFICATE REQUEST-----\n")
	out.WriteString(strings.TrimSpace(cert.CSRPEM))
	out.WriteString("\n-----END CERTIFICATE REQUEST-----\n")

	// 文件名
	filename := fmt.Sprintf("origin-ca-%s-%s.pem", domain, time.Now().Format("20060102-150405"))
	filename = sanitizeFilename(filename)

	// 写临时文件并发送
	tmpFile, err := os.CreateTemp("", "origin-ca-*.pem")
	if err != nil {
		h.sendText(fmt.Sprintf("创建临时文件失败: %v", err))
		return
	}
	tmpPath := tmpFile.Name()

	// 用完即删
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	// 私钥文件尽量收紧权限
	// _ = os.Chmod(tmpPath, 0600)

	// if _, err := tmpFile.Write(out.Bytes()); err != nil {
	// 	h.sendText(fmt.Sprintf("写入临时文件失败: %v", err))
	// 	return
	// }
	// _ = tmpFile.Sync()

	// finalPath := filepath.Join(os.TempDir(), filename)
	// _ = os.Rename(tmpPath, finalPath)
	// tmpPath = finalPath

	// caption := "🔐 Cloudflare Origin CA 证书（含私钥）"
	// if !cert.ExpiresOn.IsZero() {
	// 	caption = fmt.Sprintf("🔐 Cloudflare Origin CA 证书（含私钥）\n到期：%s", cert.ExpiresOn.Format(time.RFC3339))
	// }

	// if err := h.Sender.SendDocumentPath(context.Background(), tmpPath, caption); err != nil {
	// 	h.sendText(fmt.Sprintf("发送证书文件失败: %v", err))
	// 	return
	// }

	// h.sendText(fmt.Sprintf("✅ 源站证书生成完成：%s（账号：%s）", domain, acc.Label))
}

// 提示文本
func (h *CommandHandler) originSSLPromptText() string {
	if len(h.Accounts) == 0 {
		return "未配置可用的 Cloudflare 账号，无法生成源站证书。"
	}

	var sb strings.Builder
	sb.WriteString("生成 Cloudflare Origin CA 源站证书（15年）。\n\n")
	sb.WriteString("命令必须带 \\*：\n")
	sb.WriteString("/ssl <主域名> \\* <aws-alias>\n\n")
	sb.WriteString("示例：\n")
	sb.WriteString("/ssl example.com \\* us-aws\n\n")
	sb.WriteString("说明：该命令固定签发 example.com + \\*.example.com\n\n")
	sb.WriteString("可用账号：\n")
	for _, a := range h.Accounts {
		if strings.TrimSpace(a.Label) == "" {
			continue
		}
		sb.WriteString("- " + a.Label + "\n")
	}
	sb.WriteString("\n可用 AWS 目标：\n")
	for name, t := range config.Cfg.AWSTargets {
		if strings.TrimSpace(name) == "" {
			continue
		}
		sb.WriteString(fmt.Sprintf("- %s (%s)\n", name, t.Region))
	}
	return sb.String()
}

// 自动定位域名所属账号：domain 必须精确匹配某个账号下的 zone.Name
// - 命中 0：域名不在任何账号
// - 命中 1：返回该账号
// - 命中 >1：歧义（一般不该发生，但必须阻止）
func (h *CommandHandler) findAccountByDomain(ctx context.Context, domain string) (*config.CF, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))
	if domain == "" {
		return nil, fmt.Errorf("domain 为空")
	}

	var matched []*config.CF
	for i := range h.Accounts {
		acc := &h.Accounts[i]

		zones, err := h.CFClient.ListZones(ctx, *acc)
		if err != nil {
			// 单账号失败不阻断，继续尝试其他账号
			continue
		}
		for _, z := range zones {
			if strings.EqualFold(strings.TrimSpace(z.Name), domain) {
				matched = append(matched, acc)
				break
			}
		}
	}

	if len(matched) == 0 {
		return nil, fmt.Errorf("域名 %s 不在任何 Cloudflare 账号中", domain)
	}
	if len(matched) > 1 {
		return nil, fmt.Errorf("域名 %s 同时存在于多个 Cloudflare 账号中（歧义），请先清理重复 zone", domain)
	}
	return matched[0], nil
}

// 简单文件名清洗（避免 OS/Telegram 不兼容字符）
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, " ", "_")
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, ":", "_")
	return name
}
func importToACM(ctx context.Context, target config.AWSTarget, certPEM, keyPEM string) (string, error) {
	if strings.TrimSpace(target.Region) == "" {
		return "", fmt.Errorf("aws target region 为空")
	}
	if strings.TrimSpace(target.Creds.AccessKeyID) == "" || strings.TrimSpace(target.Creds.SecretAccessKey) == "" {
		return "", fmt.Errorf("aws target creds 不完整")
	}

	cfg, err := awscfg.LoadDefaultConfig(
		ctx,
		awscfg.WithRegion(target.Region),
		awscfg.WithCredentialsProvider(
			aws.NewCredentialsCache(
				credentials.NewStaticCredentialsProvider(
					target.Creds.AccessKeyID,
					target.Creds.SecretAccessKey,
					target.Creds.SessionToken,
				),
			),
		),
	)
	if err != nil {
		return "", fmt.Errorf("load aws config: %w", err)
	}

	client := acm.NewFromConfig(cfg)

	certBody := []byte(strings.TrimSpace(certPEM) + "\n")
	privKey := []byte(strings.TrimSpace(keyPEM) + "\n")

	out, err := client.ImportCertificate(ctx, &acm.ImportCertificateInput{
		Certificate: certBody,
		PrivateKey:  privKey,
	})
	if err != nil {
		return "", fmt.Errorf("acm import certificate: %w", err)
	}
	return *out.CertificateArn, nil
}
