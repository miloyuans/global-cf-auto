package domain

import (
	"context"
	"log"
	"strings"

	"DomainC/cfclient"
	"DomainC/config"
	"DomainC/tools"
)

type DomainSource struct {
	Domain string
	Source string
	Expiry string
	IsCF   bool
	Status string
	Paused bool
}

func DaysUntil(expiry string) (int, error) {
	return tools.DaysUntilExpiry(expiry)
}

func CheckWhois(domain string) (string, bool) {
	return tools.CheckWhois(domain)
}

func ParseExpiry(whois string) (string, bool) {
	return tools.ExtractExpiry(whois)
}

type Service struct {
	CF   cfclient.Client
	Repo Repository
}

func NewService(cf cfclient.Client, r Repository) *Service {
	if cf == nil {
		cf = cfclient.NewClient()
	}
	return &Service{CF: cf, Repo: r}
}

func (s *Service) collectCF(accounts []config.CF) ([]DomainSource, error) {
	var out []DomainSource
	for _, acc := range accounts {
		doms, e := s.CF.FetchAllDomains(context.Background(), acc)
		if e != nil {
			log.Printf("[%s] 获取域名失败: %v", acc.Label, e)
			continue
		}
		for _, d := range doms {
			out = append(out, DomainSource{
				Domain: d.Domain,
				Source: d.Source,
				IsCF:   d.IsCF,
				Status: d.Status,
				Paused: d.Paused,
			})
		}
	}
	return out, nil
}

func (s *Service) CollectPaused(accounts []config.CF) ([]DomainSource, error) {
	all, err := s.collectCF(accounts)
	if err != nil {
		return nil, err
	}
	var out []DomainSource
	for _, d := range all {
		if d.Paused {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *Service) CollectNonActive(accounts []config.CF) ([]DomainSource, error) {
	all, err := s.collectCF(accounts)
	if err != nil {
		return nil, err
	}
	var out []DomainSource
	for _, d := range all {
		if !d.Paused && strings.ToLower(d.Status) != "active" {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *Service) CollectActiveNotPaused(accounts []config.CF) ([]DomainSource, error) {
	all, err := s.collectCF(accounts)
	if err != nil {
		return nil, err
	}
	var out []DomainSource
	for _, d := range all {
		if !d.Paused && strings.ToLower(d.Status) == "active" {
			out = append(out, d)
		}
	}
	if s.Repo != nil {
		sources, err := s.Repo.LoadSources()
		if err != nil {
			return nil, err
		}
		out = append(out, sources...)
	}
	return out, nil
}
