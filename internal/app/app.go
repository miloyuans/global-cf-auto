package app

import (
	"context"
	"errors"
	"log"
	"time"

	"DomainC/domain"
)

type DomainCollector interface {
	Collect(ctx context.Context) ([]domain.DomainSource, error)
}

type ExpiryChecker interface {
	Check(ctx context.Context, domains []domain.DomainSource) ([]domain.DomainSource, []domain.FailureRecord, error)
}

type Notifier interface {
	Notify(ctx context.Context, domains []domain.DomainSource) error
	NotifyFailures(ctx context.Context, failures []domain.FailureRecord) error
}

type Scheduler interface {
	ScheduleDaily(ctx context.Context, hour, min int, job func())
}

type App struct {
	Collector DomainCollector
	Checker   ExpiryChecker
	Notifier  Notifier
	Scheduler Scheduler
	AlertHour int
	AlertMin  int
}

func (a *App) Run(ctx context.Context) error {
	if a.Collector == nil || a.Checker == nil || a.Notifier == nil || a.Scheduler == nil {
		return ErrMissingDependencies
	}

	run := func() {
		domains, err := a.Collector.Collect(ctx)
		if err != nil {
			log.Printf("收集域名失败: %v", err)
			return
		}

		expiring, failures, err := a.Checker.Check(ctx, domains)
		if err != nil {
			log.Printf("检测到期失败: %v", err)
		}

		if len(expiring) > 0 {
			if err := a.Notifier.Notify(ctx, expiring); err != nil {
				log.Printf("发送通知失败: %v", err)
			}
		}

		if len(failures) > 0 {
			if err := a.Notifier.NotifyFailures(ctx, failures); err != nil {
				log.Printf("发送失败通知失败: %v", err)
			}
		}
	}

	// run()

	a.Scheduler.ScheduleDaily(ctx, a.AlertHour, a.AlertMin, func() {
		log.Printf("开始计划任务: %02d:%02d", a.AlertHour, a.AlertMin)
		run()
	})

	<-ctx.Done()
	return ctx.Err()
}

var ErrMissingDependencies = errors.New("missing dependencies")

// AlertDaysDuration 将配置天数转换为持续时间。
func AlertDaysDuration(days int) time.Duration {
	return time.Hour * 24 * time.Duration(days)
}
