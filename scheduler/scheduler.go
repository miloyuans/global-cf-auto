package scheduler

import (
	"context"
)

type DailyScheduler struct{}

func NewDailyScheduler() *DailyScheduler {
	return &DailyScheduler{}
}

// func (s *DailyScheduler) ScheduleDaily(ctx context.Context, hour, min int, job func()) {
// 	go func() {
// 		for {
// 			now := time.Now()
// 			next := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())
// 			if now.After(next) {
// 				next = next.Add(24 * time.Hour)
// 			}
// 			wait := time.Until(next)
// 			log.Printf("距离下次任务还有: %v", wait)

//				timer := time.NewTimer(wait)
//				select {
//				case <-ctx.Done():
//					timer.Stop()
//					return
//				case <-timer.C:
//				}
//				job()
//			}
//		}()
//	}
func (s *DailyScheduler) ScheduleDaily(ctx context.Context, hour, min int, job func()) {

}
