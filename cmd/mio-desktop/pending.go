package main

import (
	"context"

	"github.com/curiselven/mio-money/internal/domain"
)

type PendingService struct {
	svc *domain.Service
}

func NewPendingService(svc *domain.Service) *PendingService {
	return &PendingService{svc: svc}
}

func (s *PendingService) List() ([]domain.PendingReview, error) {
	return s.svc.PendingReview(context.Background())
}

func (s *PendingService) Approve(snapshotID string) error {
	_, err := s.svc.ApproveAnomalous(context.Background(), snapshotID)
	return err
}

func (s *PendingService) Reject(snapshotID, reason string) error {
	_, err := s.svc.Reject(context.Background(), snapshotID, reason)
	return err
}
