package domain

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// FileRepository 基于文件的域名仓库实现。
type FileRepository struct {
	sourcesPaths      []string
	expiringTarget    string
	failureTarget     string
	expiryCacheTarget string
}

func NewFileRepository(sources []string, expiringPath, failurePath, expiryCachePath string) *FileRepository {
	return &FileRepository{sourcesPaths: sources, expiringTarget: expiringPath, failureTarget: failurePath, expiryCacheTarget: expiryCachePath}
}

// LoadSources 读取配置的源文件，每行一个域名，忽略空行和注释。
func (r *FileRepository) LoadSources() ([]DomainSource, error) {
	var out []DomainSource
	for _, path := range r.sourcesPaths {
		file, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("无法打开域名文件 %s: %w", path, err)
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			parts := strings.Split(line, "|")
			domain := strings.TrimSpace(parts[0])
			if domain == "" {
				continue
			}

			source := strings.TrimSpace(path)
			if len(parts) >= 2 && strings.TrimSpace(parts[1]) != "" {
				source = strings.TrimSpace(parts[1])
			}

			expiry := ""
			if len(parts) >= 3 {
				expiry = strings.TrimSpace(parts[2])
			}

			out = append(out, DomainSource{Domain: domain, Source: source, Expiry: expiry})

		}

		if err := scanner.Err(); err != nil {
			file.Close()
			return nil, fmt.Errorf("读取域名文件 %s 出错: %w", path, err)
		}
		file.Close()
	}
	return out, nil
}

// SaveExpiring 将即将到期的域名写入指定文件，使用统一的分隔符和格式。
func (r *FileRepository) SaveExpiring(domains []DomainSource) error {
	file, err := os.Create(r.expiringTarget)
	if err != nil {
		return fmt.Errorf("创建到期缓存文件失败: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, ds := range domains {
		if _, err := writer.WriteString(
			fmt.Sprintf("%s|%s|%s\n", strings.TrimSpace(ds.Domain), strings.TrimSpace(ds.Source), strings.TrimSpace(ds.Expiry)),
		); err != nil {
			return fmt.Errorf("写入到期缓存失败: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("刷新到期缓存失败: %w", err)
	}
	return nil
}
func (r *FileRepository) SaveFailures(failures []FailureRecord) error {
	file, err := os.Create(r.failureTarget)
	if err != nil {
		return fmt.Errorf("创建失败记录文件失败: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, f := range failures {
		if _, err := writer.WriteString(
			fmt.Sprintf("%s|%s|%s\n", strings.TrimSpace(f.Domain), strings.TrimSpace(f.Source), strings.TrimSpace(f.Reason)),
		); err != nil {
			return fmt.Errorf("写入失败记录失败: %w", err)
		}
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("刷新失败记录文件失败: %w", err)
	}
	return nil
}

// LoadExpiryCache 读取缓存文件：domain|source|expiry
func (r *FileRepository) LoadExpiryCache() ([]DomainSource, error) {
	b, err := os.ReadFile(r.expiryCacheTarget)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取到期缓存文件失败: %w", err)
	}

	lines := strings.Split(string(b), "\n")
	out := make([]DomainSource, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 3 {
			continue
		}
		out = append(out, DomainSource{
			Domain: strings.TrimSpace(parts[0]),
			Source: strings.TrimSpace(parts[1]),
			Expiry: strings.TrimSpace(parts[2]),
		})
	}
	return out, nil
}

// SaveExpiryCache 覆盖写缓存文件：domain|source|expiry
func (r *FileRepository) SaveExpiryCache(domains []DomainSource) error {
	file, err := os.Create(r.expiryCacheTarget)
	if err != nil {
		return fmt.Errorf("创建到期缓存文件失败: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	for _, ds := range domains {
		if strings.TrimSpace(ds.Domain) == "" || strings.TrimSpace(ds.Source) == "" || strings.TrimSpace(ds.Expiry) == "" {
			continue
		}
		if _, err := writer.WriteString(
			fmt.Sprintf("%s|%s|%s\n",
				strings.TrimSpace(ds.Domain),
				strings.TrimSpace(ds.Source),
				strings.TrimSpace(ds.Expiry),
			),
		); err != nil {
			return fmt.Errorf("写入到期缓存失败: %w", err)
		}
	}
	if err := writer.Flush(); err != nil {
		return fmt.Errorf("刷新到期缓存失败: %w", err)
	}
	return nil
}
